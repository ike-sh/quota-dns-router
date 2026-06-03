package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"quota-dns-router-go/internal/config"
	rootmigrations "quota-dns-router-go/migrations"

	_ "modernc.org/sqlite"
)

const (
	TrafficModeRX   = "rx"
	TrafficModeTX   = "tx"
	TrafficModeBoth = "rx+tx"
)

type Store struct {
	db *sql.DB
}

type Policy struct {
	DefaultThresholdPercent    int
	DefaultMonthlyQuotaBytes   int64
	DefaultResetDay            int
	DefaultTrafficMode         string
	AgentReportIntervalSeconds int
	AgentOfflineSeconds        int
	OfflineNotifySeconds       int
	AutoSwitchEnabled          bool
	NotifyOnly                 bool
	DefaultSwitchCooldownSecs  int
	RepoInstallURL             string
}

type Group struct {
	ID                    string
	Name                  string
	SwitchCooldownSeconds int
	CurrentNodeID         sql.NullString
	LastSwitchAt          sql.NullTime
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CloudflareConfig struct {
	ID            string
	GroupID       string
	APIToken      string
	ZoneName      string
	ZoneID        string
	RecordName    string
	RecordID      string
	AllowOverride bool
	TTL           int
	Proxied       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Node struct {
	ID                    string
	AgentID               sql.NullString
	GroupID               string
	Name                  string
	PublicIP              string
	MonthlyQuotaBytes     int64
	ThresholdPercent      int
	ResetDay              int
	TrafficMode           string
	Enabled               bool
	AutoSwitch            bool
	Priority              int
	PreferredIface        string
	ReportIntervalSeconds int
	Online                bool
	LastReportedAt        sql.NullTime
	FirstSeenAt           sql.NullTime
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AgentReport struct {
	ID           string
	AgentID      string
	Hostname     string
	PublicIP     string
	Iface        string
	RXBytesTotal int64
	TXBytesTotal int64
	RXDelta      int64
	TXDelta      int64
	ReportedAt   time.Time
	AgentVersion string
	Status       string
}

type UsageSnapshot struct {
	CycleStart string
	RXBytes    int64
	TXBytes    int64
	UsedBytes  int64
}

type NodeUsage struct {
	Node
	UsageSnapshot
}

type JoinCodeResult struct {
	NodeID                string
	AgentID               string
	AgentToken            string
	NodeName              string
	GroupName             string
	PublicIPOverride      string
	Interface             string
	ReportIntervalSeconds int
}

type StatusSummary struct {
	Groups []GroupStatus
}

type GroupStatus struct {
	Group       Group
	DNSRecord   string
	CurrentNode string
	CurrentIP   string
	Nodes       []NodeStatus
}

type NodeStatus struct {
	Name         string
	PublicIP     string
	Online       bool
	UsagePercent float64
	TrafficMode  string
	Threshold    int
	LastReported string
	AutoSwitch   bool
	Enabled      bool
	Priority     int
}

type LastError struct {
	Key       string
	Message   string
	CreatedAt time.Time
}

type SwitchHistory struct {
	ID           string
	GroupID      string
	GroupName    string
	FromNodeID   string
	FromNodeName string
	ToNodeID     string
	ToNodeName   string
	RecordName   string
	OldIP        string
	NewIP        string
	Reason       string
	Status       string
	ErrorMessage string
	SwitchedAt   time.Time
}

type NodeWithGroup struct {
	Node
	GroupName string
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(rootmigrations.FS, ".")
	if err != nil {
		return err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := fs.ReadFile(rootmigrations.FS, filepath.Base(name))
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("执行 migration %s 失败: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES(?)`, name); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func NewID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("%s-%d", prefix, now)
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func NewSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func DefaultPolicy() Policy {
	return Policy{
		DefaultThresholdPercent:    80,
		DefaultMonthlyQuotaBytes:   1000 * 1024 * 1024 * 1024,
		DefaultResetDay:            1,
		DefaultTrafficMode:         TrafficModeBoth,
		AgentReportIntervalSeconds: 60,
		AgentOfflineSeconds:        300,
		OfflineNotifySeconds:       600,
		AutoSwitchEnabled:          true,
		NotifyOnly:                 false,
		DefaultSwitchCooldownSecs:  600,
		RepoInstallURL:             "https://raw.githubusercontent.com/OWNER/REPO/main/scripts/install-agent.sh",
	}
}

func (s *Store) SavePolicy(ctx context.Context, p Policy) error {
	settings := map[string]string{
		"default_threshold_percent":     strconv.Itoa(p.DefaultThresholdPercent),
		"default_monthly_quota_bytes":   strconv.FormatInt(p.DefaultMonthlyQuotaBytes, 10),
		"default_reset_day":             strconv.Itoa(p.DefaultResetDay),
		"default_traffic_mode":          p.DefaultTrafficMode,
		"agent_report_interval_seconds": strconv.Itoa(p.AgentReportIntervalSeconds),
		"agent_offline_seconds":         strconv.Itoa(p.AgentOfflineSeconds),
		"offline_notify_seconds":        strconv.Itoa(p.OfflineNotifySeconds),
		"auto_switch_enabled":           boolString(p.AutoSwitchEnabled),
		"notify_only":                   boolString(p.NotifyOnly),
		"default_switch_cooldown_secs":  strconv.Itoa(p.DefaultSwitchCooldownSecs),
		"repo_install_url":              p.RepoInstallURL,
	}
	for k, v := range settings {
		if err := s.SetSetting(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetPolicy(ctx context.Context) (Policy, error) {
	p := DefaultPolicy()
	if v, err := s.GetSetting(ctx, "default_threshold_percent"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.DefaultThresholdPercent = n
		}
	}
	if v, err := s.GetSetting(ctx, "default_monthly_quota_bytes"); err == nil && v != "" {
		if n, convErr := strconv.ParseInt(v, 10, 64); convErr == nil {
			p.DefaultMonthlyQuotaBytes = n
		}
	}
	if v, err := s.GetSetting(ctx, "default_reset_day"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.DefaultResetDay = n
		}
	}
	if v, err := s.GetSetting(ctx, "default_traffic_mode"); err == nil && v != "" {
		p.DefaultTrafficMode = v
	}
	if v, err := s.GetSetting(ctx, "agent_report_interval_seconds"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.AgentReportIntervalSeconds = n
		}
	}
	if v, err := s.GetSetting(ctx, "agent_offline_seconds"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.AgentOfflineSeconds = n
		}
	}
	if v, err := s.GetSetting(ctx, "offline_notify_seconds"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.OfflineNotifySeconds = n
		}
	}
	if v, err := s.GetSetting(ctx, "auto_switch_enabled"); err == nil && v != "" {
		p.AutoSwitchEnabled = v == "true"
	}
	if v, err := s.GetSetting(ctx, "notify_only"); err == nil && v != "" {
		p.NotifyOnly = v == "true"
	}
	if v, err := s.GetSetting(ctx, "default_switch_cooldown_secs"); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil {
			p.DefaultSwitchCooldownSecs = n
		}
	}
	if v, err := s.GetSetting(ctx, "repo_install_url"); err == nil && v != "" {
		p.RepoInstallURL = v
	}
	return p, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings(key, value, updated_at) VALUES(?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) SetStatusNote(ctx context.Context, key, value string) error {
	return s.SetSetting(ctx, "diag_note:"+key, value)
}

func (s *Store) GetStatusNote(ctx context.Context, key string) (string, error) {
	return s.GetSetting(ctx, "diag_note:"+key)
}

func (s *Store) SaveLastError(ctx context.Context, key, message string, secrets ...string) error {
	message = sanitizeDiagnosticMessage(message, secrets...)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO last_errors(key, message, created_at) VALUES(?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET message = excluded.message, created_at = CURRENT_TIMESTAMP
	`, key, message)
	return err
}

func (s *Store) ClearLastError(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM last_errors WHERE key = ?`, key)
	return err
}

func (s *Store) GetLastError(ctx context.Context, key string) (LastError, error) {
	var item LastError
	err := s.db.QueryRowContext(ctx, `
		SELECT key, message, created_at FROM last_errors WHERE key = ?
	`, key).Scan(&item.Key, &item.Message, &item.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LastError{}, nil
	}
	return item, err
}

func (s *Store) SetMasterPublicURL(ctx context.Context, value string) error {
	return s.SetSetting(ctx, "master_public_api_url", value)
}

func (s *Store) GetMasterPublicURL(ctx context.Context, fallback string) (string, error) {
	value, err := s.GetSetting(ctx, "master_public_api_url")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	return fallback, nil
}

func (s *Store) SaveCloudflareDefaults(ctx context.Context, token, zoneName, zoneID string) error {
	if err := s.SetSetting(ctx, "cloudflare_api_token", token); err != nil {
		return err
	}
	if err := s.SetSetting(ctx, "cloudflare_zone_name", zoneName); err != nil {
		return err
	}
	if err := s.SetSetting(ctx, "cloudflare_zone_id", zoneID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE cloudflare_configs
		SET api_token = ?, zone_name = ?, zone_id = ?, updated_at = CURRENT_TIMESTAMP
	`, token, zoneName, zoneID)
	return err
}

func (s *Store) GetCloudflareDefaults(ctx context.Context) (token, zoneName, zoneID string, err error) {
	token, err = s.GetSetting(ctx, "cloudflare_api_token")
	if err != nil {
		return "", "", "", err
	}
	zoneName, err = s.GetSetting(ctx, "cloudflare_zone_name")
	if err != nil {
		return "", "", "", err
	}
	zoneID, err = s.GetSetting(ctx, "cloudflare_zone_id")
	return token, zoneName, zoneID, err
}

func (s *Store) CreateGroup(ctx context.Context, name string, cooldownSeconds int) (Group, error) {
	g := Group{
		ID:                    NewID("grp"),
		Name:                  name,
		SwitchCooldownSeconds: cooldownSeconds,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO groups(id, name, switch_cooldown_seconds)
		VALUES(?, ?, ?)
	`, g.ID, g.Name, g.SwitchCooldownSeconds)
	if err != nil {
		return Group{}, err
	}
	return s.GetGroupByName(ctx, name)
}

func (s *Store) GetGroupByName(ctx context.Context, name string) (Group, error) {
	return scanGroup(s.db.QueryRowContext(ctx, `
		SELECT id, name, switch_cooldown_seconds, current_node_id, last_switch_at, created_at, updated_at
		FROM groups WHERE name = ?
	`, name))
}

func (s *Store) GetGroupByID(ctx context.Context, id string) (Group, error) {
	return scanGroup(s.db.QueryRowContext(ctx, `
		SELECT id, name, switch_cooldown_seconds, current_node_id, last_switch_at, created_at, updated_at
		FROM groups WHERE id = ?
	`, id))
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, switch_cooldown_seconds, current_node_id, last_switch_at, created_at, updated_at
		FROM groups ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.SwitchCooldownSeconds, &g.CurrentNodeID, &g.LastSwitchAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CountGroups(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM groups`).Scan(&count)
	return count, err
}

func (s *Store) CountNodes(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes`).Scan(&count)
	return count, err
}

func (s *Store) CountOnlineNodes(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes WHERE online = 1`).Scan(&count)
	return count, err
}

func (s *Store) CountCloudflareConfigs(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM cloudflare_configs`).Scan(&count)
	return count, err
}

func (s *Store) ListCloudflareConfigs(ctx context.Context) ([]CloudflareConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, api_token, zone_name, zone_id, record_name, record_id, allow_override, ttl, proxied, created_at, updated_at
		FROM cloudflare_configs ORDER BY record_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloudflareConfig
	for rows.Next() {
		var cfg CloudflareConfig
		if err := rows.Scan(&cfg.ID, &cfg.GroupID, &cfg.APIToken, &cfg.ZoneName, &cfg.ZoneID, &cfg.RecordName, &cfg.RecordID, &cfg.AllowOverride, &cfg.TTL, &cfg.Proxied, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *Store) CreateOrUpdateCloudflareConfig(ctx context.Context, groupID, recordName, recordID string, ttl int, proxied, allowOverride bool) (CloudflareConfig, error) {
	token, zoneName, zoneID, err := s.GetCloudflareDefaults(ctx)
	if err != nil {
		return CloudflareConfig{}, err
	}
	if token == "" || zoneName == "" {
		return CloudflareConfig{}, fmt.Errorf("请先配置 Cloudflare Token 和 Zone")
	}
	id := NewID("cf")
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO cloudflare_configs(id, group_id, api_token, zone_name, zone_id, record_name, record_id, allow_override, ttl, proxied)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET
			api_token = excluded.api_token,
			zone_name = excluded.zone_name,
			zone_id = excluded.zone_id,
			record_name = excluded.record_name,
			record_id = excluded.record_id,
			allow_override = excluded.allow_override,
			ttl = excluded.ttl,
			proxied = excluded.proxied,
			updated_at = CURRENT_TIMESTAMP
	`, id, groupID, token, zoneName, zoneID, recordName, recordID, boolInt(allowOverride), ttl, boolInt(proxied))
	if err != nil {
		return CloudflareConfig{}, err
	}
	return s.GetCloudflareConfigByGroupID(ctx, groupID)
}

func (s *Store) GetCloudflareConfigByGroupID(ctx context.Context, groupID string) (CloudflareConfig, error) {
	var cfg CloudflareConfig
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, api_token, zone_name, zone_id, record_name, record_id, allow_override, ttl, proxied, created_at, updated_at
		FROM cloudflare_configs WHERE group_id = ?
	`, groupID).Scan(
		&cfg.ID, &cfg.GroupID, &cfg.APIToken, &cfg.ZoneName, &cfg.ZoneID, &cfg.RecordName, &cfg.RecordID,
		&cfg.AllowOverride, &cfg.TTL, &cfg.Proxied, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	return cfg, err
}

func (s *Store) CreateNode(ctx context.Context, n Node) (Node, error) {
	if n.ID == "" {
		n.ID = NewID("node")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO nodes(
			id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
			traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, n.ID, nullStringValue(n.AgentID), n.GroupID, n.Name, n.PublicIP, n.MonthlyQuotaBytes, n.ThresholdPercent, n.ResetDay,
		n.TrafficMode, boolInt(n.Enabled), boolInt(n.AutoSwitch), n.Priority, n.PreferredIface, n.ReportIntervalSeconds, boolInt(n.Online))
	if err != nil {
		return Node{}, err
	}
	return s.GetNodeByName(ctx, n.Name)
}

func (s *Store) GetNodeByName(ctx context.Context, name string) (Node, error) {
	return scanNode(s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
		       traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online,
		       last_reported_at, first_seen_at, created_at, updated_at
		FROM nodes WHERE name = ?
	`, name))
}

func (s *Store) GetNodeByID(ctx context.Context, id string) (Node, error) {
	return scanNode(s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
		       traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online,
		       last_reported_at, first_seen_at, created_at, updated_at
		FROM nodes WHERE id = ?
	`, id))
}

func (s *Store) GetNodeByAgentID(ctx context.Context, agentID string) (Node, error) {
	return scanNode(s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
		       traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online,
		       last_reported_at, first_seen_at, created_at, updated_at
		FROM nodes WHERE agent_id = ?
	`, agentID))
}

func (s *Store) ListNodesByGroupID(ctx context.Context, groupID string) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
		       traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online,
		       last_reported_at, first_seen_at, created_at, updated_at
		FROM nodes WHERE group_id = ? ORDER BY priority ASC, name ASC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.AgentID, &n.GroupID, &n.Name, &n.PublicIP, &n.MonthlyQuotaBytes, &n.ThresholdPercent, &n.ResetDay,
			&n.TrafficMode, &n.Enabled, &n.AutoSwitch, &n.Priority, &n.PreferredIface, &n.ReportIntervalSeconds, &n.Online,
			&n.LastReportedAt, &n.FirstSeenAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListNodes(ctx context.Context) ([]NodeWithGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.agent_id, n.group_id, n.name, n.public_ip, n.monthly_quota_bytes, n.threshold_percent, n.reset_day,
		       n.traffic_mode, n.enabled, n.auto_switch, n.priority, n.preferred_iface, n.report_interval_seconds, n.online,
		       n.last_reported_at, n.first_seen_at, n.created_at, n.updated_at, g.name
		FROM nodes n
		JOIN groups g ON g.id = n.group_id
		ORDER BY g.name, n.priority ASC, n.name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeWithGroup
	for rows.Next() {
		var n NodeWithGroup
		if err := rows.Scan(&n.ID, &n.AgentID, &n.GroupID, &n.Name, &n.PublicIP, &n.MonthlyQuotaBytes, &n.ThresholdPercent, &n.ResetDay,
			&n.TrafficMode, &n.Enabled, &n.AutoSwitch, &n.Priority, &n.PreferredIface, &n.ReportIntervalSeconds, &n.Online,
			&n.LastReportedAt, &n.FirstSeenAt, &n.CreatedAt, &n.UpdatedAt, &n.GroupName); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) BindAgentToNode(ctx context.Context, nodeID, agentID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE nodes
		SET agent_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, agentID, nodeID)
	return err
}

func (s *Store) GenerateJoinCode(ctx context.Context, nodeID string, validFor time.Duration) (string, error) {
	code, _, err := s.GenerateJoinCodeWithExpiry(ctx, nodeID, validFor)
	return code, err
}

func (s *Store) GenerateJoinCodeWithExpiry(ctx context.Context, nodeID string, validFor time.Duration) (string, time.Time, error) {
	code, err := NewSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(validFor)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO join_codes(id, node_id, code_hash, expires_at)
		VALUES(?, ?, ?, ?)
	`, NewID("join"), nodeID, HashSecret(code), expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return code, expiresAt, nil
}

func (s *Store) RedeemJoinCode(ctx context.Context, code string) (JoinCodeResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return JoinCodeResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var joinID string
	var nodeID string
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT id, node_id, expires_at, used_at
		FROM join_codes WHERE code_hash = ?
	`, HashSecret(code)).Scan(&joinID, &nodeID, &expiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JoinCodeResult{}, fmt.Errorf("加入码无效")
		}
		return JoinCodeResult{}, err
	}
	if usedAt.Valid {
		return JoinCodeResult{}, fmt.Errorf("加入码已使用")
	}
	if time.Now().After(expiresAt) {
		return JoinCodeResult{}, fmt.Errorf("加入码已过期")
	}

	node, err := scanNodeTx(tx.QueryRowContext(ctx, `
		SELECT id, agent_id, group_id, name, public_ip, monthly_quota_bytes, threshold_percent, reset_day,
		       traffic_mode, enabled, auto_switch, priority, preferred_iface, report_interval_seconds, online,
		       last_reported_at, first_seen_at, created_at, updated_at
		FROM nodes WHERE id = ?
	`, nodeID))
	if err != nil {
		return JoinCodeResult{}, err
	}
	group, err := scanGroupTx(tx.QueryRowContext(ctx, `
		SELECT id, name, switch_cooldown_seconds, current_node_id, last_switch_at, created_at, updated_at
		FROM groups WHERE id = ?
	`, node.GroupID))
	if err != nil {
		return JoinCodeResult{}, err
	}
	agentID := node.AgentID.String
	if agentID == "" {
		agentID = NewID("agent")
		if _, err = tx.ExecContext(ctx, `
			UPDATE nodes SET agent_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, agentID, node.ID); err != nil {
			return JoinCodeResult{}, err
		}
	}
	token, err := NewSecret()
	if err != nil {
		return JoinCodeResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO agent_tokens(id, agent_id, token_hash)
		VALUES(?, ?, ?)
	`, NewID("atok"), agentID, HashSecret(token)); err != nil {
		return JoinCodeResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE join_codes SET used_at = CURRENT_TIMESTAMP WHERE id = ?
	`, joinID); err != nil {
		return JoinCodeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return JoinCodeResult{}, err
	}
	committed = true
	return JoinCodeResult{
		NodeID:                node.ID,
		AgentID:               agentID,
		AgentToken:            token,
		NodeName:              node.Name,
		GroupName:             group.Name,
		PublicIPOverride:      node.PublicIP,
		Interface:             node.PreferredIface,
		ReportIntervalSeconds: node.ReportIntervalSeconds,
	}, nil
}

func (s *Store) ValidateAgentToken(ctx context.Context, agentID, token string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM agent_tokens
		WHERE agent_id = ? AND token_hash = ? AND revoked_at IS NULL
	`, agentID, HashSecret(token)).Scan(&count)
	return count > 0, err
}

func (s *Store) RevokeAgentTokens(ctx context.Context, agentID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_tokens SET revoked_at = CURRENT_TIMESTAMP
		WHERE agent_id = ? AND revoked_at IS NULL
	`, agentID)
	return err
}

func (s *Store) SaveAgentReport(ctx context.Context, report AgentReport) error {
	node, err := s.GetNodeByAgentID(ctx, report.AgentID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if report.ID == "" {
		report.ID = NewID("rpt")
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO agent_reports(
			id, agent_id, hostname, public_ip, iface, rx_bytes_total, tx_bytes_total, rx_delta, tx_delta, reported_at, agent_version, status
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, report.ID, report.AgentID, report.Hostname, report.PublicIP, report.Iface, report.RXBytesTotal, report.TXBytesTotal, report.RXDelta, report.TXDelta, report.ReportedAt, report.AgentVersion, report.Status); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE nodes
		SET online = 1,
		    last_reported_at = ?,
		    first_seen_at = COALESCE(first_seen_at, ?),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, report.ReportedAt, report.ReportedAt, node.ID); err != nil {
		return err
	}
	cycle := BillingCycleStart(report.ReportedAt, node.ResetDay).Format("2006-01-02")
	usedDelta := UsageDeltaByMode(node.TrafficMode, report.RXDelta, report.TXDelta)
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO traffic_counters(id, node_id, cycle_start, rx_bytes, tx_bytes, used_bytes, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(node_id, cycle_start) DO UPDATE SET
			rx_bytes = traffic_counters.rx_bytes + excluded.rx_bytes,
			tx_bytes = traffic_counters.tx_bytes + excluded.tx_bytes,
			used_bytes = traffic_counters.used_bytes + excluded.used_bytes,
			updated_at = CURRENT_TIMESTAMP
	`, NewID("tc"), node.ID, cycle, report.RXDelta, report.TXDelta, usedDelta); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) MarkNodeOffline(ctx context.Context, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE nodes SET online = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, nodeID)
	return err
}

func (s *Store) GetUsageSnapshot(ctx context.Context, nodeID string, at time.Time, resetDay int) (UsageSnapshot, error) {
	cycle := BillingCycleStart(at, resetDay).Format("2006-01-02")
	var snap UsageSnapshot
	snap.CycleStart = cycle
	err := s.db.QueryRowContext(ctx, `
		SELECT cycle_start, rx_bytes, tx_bytes, used_bytes
		FROM traffic_counters WHERE node_id = ? AND cycle_start = ?
	`, nodeID, cycle).Scan(&snap.CycleStart, &snap.RXBytes, &snap.TXBytes, &snap.UsedBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return UsageSnapshot{CycleStart: cycle}, nil
	}
	return snap, err
}

func (s *Store) GetNodeUsage(ctx context.Context, node Node, at time.Time) (NodeUsage, error) {
	snap, err := s.GetUsageSnapshot(ctx, node.ID, at, node.ResetDay)
	if err != nil {
		return NodeUsage{}, err
	}
	return NodeUsage{Node: node, UsageSnapshot: snap}, nil
}

func (s *Store) ListNodeUsagesByGroup(ctx context.Context, groupID string, at time.Time) ([]NodeUsage, error) {
	nodes, err := s.ListNodesByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]NodeUsage, 0, len(nodes))
	for _, node := range nodes {
		usage, err := s.GetNodeUsage(ctx, node, at)
		if err != nil {
			return nil, err
		}
		out = append(out, usage)
	}
	return out, nil
}

func (s *Store) UpdateGroupCurrentNode(ctx context.Context, groupID, nodeID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE groups
		SET current_node_id = ?, last_switch_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, nodeID, groupID)
	return err
}

func (s *Store) RecordSwitchHistory(ctx context.Context, groupID, fromNodeID, toNodeID, recordName, oldIP, newIP, reason, status, errMsg string, secrets ...string) error {
	errMsg = sanitizeDiagnosticMessage(errMsg, secrets...)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dns_switch_history(
			id, group_id, from_node_id, to_node_id, record_name, old_ip, new_ip, reason, status, error_message
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, NewID("sw"), groupID, nullIfEmpty(fromNodeID), nullIfEmpty(toNodeID), recordName, nullIfEmpty(oldIP), nullIfEmpty(newIP), reason, status, nullIfEmpty(errMsg))
	return err
}

func (s *Store) GetLatestSwitchHistory(ctx context.Context) (SwitchHistory, error) {
	return s.getSwitchHistory(ctx, "")
}

func (s *Store) GetLatestFailedSwitchHistory(ctx context.Context) (SwitchHistory, error) {
	return s.getSwitchHistory(ctx, "WHERE lower(h.status) IN ('failed', 'failure', 'error')")
}

func (s *Store) getSwitchHistory(ctx context.Context, where string) (SwitchHistory, error) {
	query := `
		SELECT
			h.id,
			h.group_id,
			COALESCE(g.name, ''),
			COALESCE(h.from_node_id, ''),
			COALESCE(from_node.name, ''),
			COALESCE(h.to_node_id, ''),
			COALESCE(to_node.name, ''),
			h.record_name,
			COALESCE(h.old_ip, ''),
			COALESCE(h.new_ip, ''),
			h.reason,
			h.status,
			COALESCE(h.error_message, ''),
			h.switched_at
		FROM dns_switch_history h
		LEFT JOIN groups g ON g.id = h.group_id
		LEFT JOIN nodes from_node ON from_node.id = h.from_node_id
		LEFT JOIN nodes to_node ON to_node.id = h.to_node_id
		` + where + `
		ORDER BY h.switched_at DESC, h.id DESC
		LIMIT 1
	`
	var item SwitchHistory
	err := s.db.QueryRowContext(ctx, query).Scan(
		&item.ID,
		&item.GroupID,
		&item.GroupName,
		&item.FromNodeID,
		&item.FromNodeName,
		&item.ToNodeID,
		&item.ToNodeName,
		&item.RecordName,
		&item.OldIP,
		&item.NewIP,
		&item.Reason,
		&item.Status,
		&item.ErrorMessage,
		&item.SwitchedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SwitchHistory{}, nil
	}
	return item, err
}

func (s *Store) RecordNotification(ctx context.Context, kind, targetRef, message, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications(id, kind, target_ref, message, status, error_message)
		VALUES(?, ?, ?, ?, ?, ?)
	`, NewID("ntf"), kind, targetRef, message, status, nullIfEmpty(errMsg))
	return err
}

func (s *Store) BuildStatusSummary(ctx context.Context, at time.Time) (StatusSummary, error) {
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return StatusSummary{}, err
	}
	var summary StatusSummary
	for _, group := range groups {
		cfg, err := s.GetCloudflareConfigByGroupID(ctx, group.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return StatusSummary{}, err
		}
		usages, err := s.ListNodeUsagesByGroup(ctx, group.ID, at)
		if err != nil {
			return StatusSummary{}, err
		}
		gs := GroupStatus{
			Group:     group,
			DNSRecord: cfg.RecordName,
		}
		for _, usage := range usages {
			lastReported := "-"
			if usage.LastReportedAt.Valid {
				lastReported = usage.LastReportedAt.Time.Format(time.RFC3339)
			}
			ns := NodeStatus{
				Name:         usage.Name,
				PublicIP:     usage.PublicIP,
				Online:       usage.Online,
				UsagePercent: UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes),
				TrafficMode:  usage.TrafficMode,
				Threshold:    usage.ThresholdPercent,
				LastReported: lastReported,
				AutoSwitch:   usage.AutoSwitch,
				Enabled:      usage.Enabled,
				Priority:     usage.Priority,
			}
			if group.CurrentNodeID.Valid && group.CurrentNodeID.String == usage.ID {
				gs.CurrentNode = usage.Name
				gs.CurrentIP = usage.PublicIP
			}
			gs.Nodes = append(gs.Nodes, ns)
		}
		summary.Groups = append(summary.Groups, gs)
	}
	return summary, nil
}

func BillingCycleStart(now time.Time, resetDay int) time.Time {
	if resetDay < 1 {
		resetDay = 1
	}
	if resetDay > 28 {
		resetDay = 28
	}
	year, month, day := now.Date()
	loc := now.Location()
	if day < resetDay {
		month--
		if month < time.January {
			month = time.December
			year--
		}
	}
	return time.Date(year, month, resetDay, 0, 0, 0, 0, loc)
}

func UsageDeltaByMode(mode string, rx, tx int64) int64 {
	switch strings.ToLower(mode) {
	case TrafficModeRX:
		return rx
	case TrafficModeTX:
		return tx
	default:
		return rx + tx
	}
}

func UsagePercent(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(used) * 100 / float64(total)
}

func FormatPolicy(p Policy) string {
	return fmt.Sprintf(
		"阈值=%d%% 月总量=%s 重置日=%d 模式=%s 上报间隔=%ds 离线=%ds 通知=%ds 自动切换=%t 只通知=%t 冷却=%ds Repo=%s",
		p.DefaultThresholdPercent,
		formatBytes(p.DefaultMonthlyQuotaBytes),
		p.DefaultResetDay,
		p.DefaultTrafficMode,
		p.AgentReportIntervalSeconds,
		p.AgentOfflineSeconds,
		p.OfflineNotifySeconds,
		p.AutoSwitchEnabled,
		p.NotifyOnly,
		p.DefaultSwitchCooldownSecs,
		p.RepoInstallURL,
	)
}

func MaskedCloudflare(token string) string {
	return config.MaskSecret(token)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func nullStringValue(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func scanGroup(row *sql.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.SwitchCooldownSeconds, &g.CurrentNodeID, &g.LastSwitchAt, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func scanNode(row *sql.Row) (Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.AgentID, &n.GroupID, &n.Name, &n.PublicIP, &n.MonthlyQuotaBytes, &n.ThresholdPercent, &n.ResetDay,
		&n.TrafficMode, &n.Enabled, &n.AutoSwitch, &n.Priority, &n.PreferredIface, &n.ReportIntervalSeconds, &n.Online,
		&n.LastReportedAt, &n.FirstSeenAt, &n.CreatedAt, &n.UpdatedAt)
	return n, err
}

func scanGroupTx(row *sql.Row) (Group, error) {
	return scanGroup(row)
}

func scanNodeTx(row *sql.Row) (Node, error) {
	return scanNode(row)
}

func formatBytes(v int64) string {
	const gb = 1024 * 1024 * 1024
	return fmt.Sprintf("%.2fGB", float64(v)/gb)
}

func sanitizeDiagnosticMessage(message string, secrets ...string) string {
	message = strings.TrimSpace(message)
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, config.MaskSecret(secret))
	}
	return message
}
