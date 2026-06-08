package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const DefaultMasterEnvPath = "/etc/quota-dns-router/master.env"

type MasterConfig struct {
	EnvPath               string
	ListenAddr            string
	PublicAPIURL          string
	DBPath                string
	DataDir               string
	LogDir                string
	TelegramToken         string
	TelegramAdminID       int64
	TelegramAdminIDs      []int64
	TelegramObserverIDs   []int64
	DNSProvider           string
	AWSRegion             string
	TelegramPollTimeout   time.Duration
	CheckInterval         time.Duration
	AgentOfflineAfter     time.Duration
	OfflineNotifyAfter    time.Duration
	DetectedPublicIP         string
	SuggestedPublicAPIURL    string
	AgentReportRetentionDays int
	StatusReadonlyToken      string
}

func LoadMaster(path string) (MasterConfig, error) {
	fileValues, err := LoadEnvFile(path)
	if err != nil {
		return MasterConfig{}, err
	}
	values := MergeEnv(fileValues)

	adminIDs, err := parseTelegramAdminIDs(values)
	if err != nil {
		return MasterConfig{}, err
	}
	observerIDs, err := parseTelegramObserverIDs(values)
	if err != nil {
		return MasterConfig{}, err
	}
	adminID := adminIDs[0]
	pollTimeout, err := getDuration(values, "QDR_TELEGRAM_POLL_TIMEOUT", 20*time.Second)
	if err != nil {
		return MasterConfig{}, err
	}
	checkInterval, err := getDuration(values, "QDR_CHECK_INTERVAL", time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}
	offlineAfter, err := getDuration(values, "QDR_AGENT_OFFLINE_AFTER", 5*time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}
	offlineNotifyAfter, err := getDuration(values, "QDR_OFFLINE_NOTIFY_AFTER", 10*time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}

	retentionDays, err := getInt(values, "QDR_AGENT_REPORT_RETENTION_DAYS", 30)
	if err != nil {
		return MasterConfig{}, err
	}
	if retentionDays < 1 {
		return MasterConfig{}, fmt.Errorf("QDR_AGENT_REPORT_RETENTION_DAYS 必须 >= 1")
	}

	cfg := MasterConfig{
		EnvPath:                  path,
		ListenAddr:               getString(values, "QDR_MASTER_LISTEN_ADDR", ":8080"),
		PublicAPIURL:               getString(values, "QDR_MASTER_PUBLIC_API_URL", "http://127.0.0.1:8080"),
		DBPath:                     getString(values, "QDR_MASTER_DB_PATH", "/var/lib/quota-dns-router/master.db"),
		DataDir:                    getString(values, "QDR_MASTER_DATA_DIR", "/var/lib/quota-dns-router"),
		LogDir:                     getString(values, "QDR_MASTER_LOG_DIR", "/var/log/quota-dns-router"),
		TelegramToken:              getString(values, "QDR_TELEGRAM_TOKEN", ""),
		TelegramAdminID:            adminID,
		TelegramAdminIDs:           adminIDs,
		TelegramObserverIDs:        observerIDs,
		DNSProvider:                getString(values, "QDR_DNS_PROVIDER", "cloudflare"),
		AWSRegion:                  getString(values, "QDR_AWS_REGION", "us-east-1"),
		TelegramPollTimeout:        pollTimeout,
		CheckInterval:              checkInterval,
		AgentOfflineAfter:          offlineAfter,
		OfflineNotifyAfter:         offlineNotifyAfter,
		DetectedPublicIP:           getString(values, "QDR_DETECTED_PUBLIC_IP", ""),
		SuggestedPublicAPIURL:      getString(values, "QDR_SUGGESTED_PUBLIC_API_URL", ""),
		AgentReportRetentionDays:   retentionDays,
		StatusReadonlyToken:        getString(values, "QDR_STATUS_READONLY_TOKEN", ""),
	}
	if cfg.TelegramToken == "" {
		return MasterConfig{}, fmt.Errorf("缺少 QDR_TELEGRAM_TOKEN")
	}
	return cfg, nil
}

func parseTelegramAdminIDs(values map[string]string) ([]int64, error) {
	if multi := strings.TrimSpace(getString(values, "QDR_TELEGRAM_ADMIN_IDS", "")); multi != "" {
		parts := strings.Split(multi, ",")
		ids := make([]int64, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("QDR_TELEGRAM_ADMIN_IDS 包含无效数字：%s", part)
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("QDR_TELEGRAM_ADMIN_IDS 不能为空")
		}
		return ids, nil
	}
	adminIDRaw := getString(values, "QDR_TELEGRAM_ADMIN_ID", "")
	if adminIDRaw == "" {
		return nil, fmt.Errorf("缺少 QDR_TELEGRAM_ADMIN_ID 或 QDR_TELEGRAM_ADMIN_IDS")
	}
	adminID, err := strconv.ParseInt(adminIDRaw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("QDR_TELEGRAM_ADMIN_ID 不是有效数字")
	}
	return []int64{adminID}, nil
}

func parseTelegramObserverIDs(values map[string]string) ([]int64, error) {
	multi := strings.TrimSpace(getString(values, "QDR_TELEGRAM_OBSERVER_IDS", ""))
	if multi == "" {
		return nil, nil
	}
	parts := strings.Split(multi, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("QDR_TELEGRAM_OBSERVER_IDS 包含无效数字：%s", part)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c MasterConfig) String() string {
	return fmt.Sprintf(
		"listen=%s public_api=%s db=%s data_dir=%s log_dir=%s admin_ids=%d telegram_token=%s",
		c.ListenAddr,
		c.PublicAPIURL,
		c.DBPath,
		c.DataDir,
		c.LogDir,
		len(c.TelegramAdminIDs),
		MaskSecret(c.TelegramToken),
	)
}
