package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/db"
)

type TimeNowFunc func() time.Time

type TelegramNotifier interface {
	SendAdminMessage(ctx context.Context, text string) error
}

type DNSProvider interface {
	ListZones(ctx context.Context, token string) ([]cloudflare.Zone, error)
	LookupZoneID(ctx context.Context, token, zoneName string) (string, error)
	LookupDNSRecord(ctx context.Context, token, zoneID, recordName string) (cloudflare.DNSRecord, error)
	LookupDNSRecordAnyType(ctx context.Context, token, zoneID, recordName string) (cloudflare.DNSRecord, error)
	CreateDNSRecord(ctx context.Context, token, zoneID, recordName, ip string, ttl int, proxied bool) (cloudflare.DNSRecord, error)
	CreateDNSRecordWithType(ctx context.Context, token, zoneID, recordName, ip, recordType string, ttl int, proxied bool) (cloudflare.DNSRecord, error)
	UpdateDNSRecord(ctx context.Context, token, zoneID, recordID, recordName, ip string, ttl int, proxied bool) error
	UpdateDNSRecordWithType(ctx context.Context, token, zoneID, recordID, recordName, ip, recordType string, ttl int, proxied bool) error
	LookupDNSRecordWithType(ctx context.Context, token, zoneID, recordName, recordType string) (cloudflare.DNSRecord, error)
}

func dnsRecordType(cfg db.CloudflareConfig, discovered string) string {
	if discovered != "" {
		return discovered
	}
	if cfg.RecordType != "" {
		return cfg.RecordType
	}
	return "A"
}

type Service struct {
	Store    *db.Store
	Notifier TelegramNotifier
	DNS      DNSProvider
	Now      TimeNowFunc
}

type SwitchDecision struct {
	Group       db.Group
	Config      db.CloudflareConfig
	Current     db.NodeUsage
	Target      db.NodeUsage
	TriggerType string
	Reason      string
	Triggered   bool
}

const (
	switchReasonThreshold = "流量达到阈值"
	switchReasonOffline   = "当前节点离线"
	switchReasonDisabled  = "当前节点已禁用"
	switchReasonNoAuto    = "当前节点不参与自动切换"
)

type CurrentNodeUnresolvedError struct {
	GroupID    string
	RecordName string
	DNSIP      string
}

func (e *CurrentNodeUnresolvedError) Error() string {
	return fmt.Sprintf("DNS 记录 %s 当前指向 %s，未匹配任何节点", e.RecordName, e.DNSIP)
}

func NewService(store *db.Store, notifier TelegramNotifier, dns DNSProvider) *Service {
	return &Service{
		Store:    store,
		Notifier: notifier,
		DNS:      dns,
		Now:      time.Now,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) EvaluateAndSwitchAll(ctx context.Context) error {
	groups, err := s.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if err := s.HandleGroup(ctx, group.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HandleGroup(ctx context.Context, groupID string) error {
	decision, err := s.BuildDecision(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !decision.Triggered {
		return nil
	}
	if decision.TriggerType == db.SwitchTriggerThreshold {
		_, _ = s.notifyOnce(ctx, "threshold", thresholdNotificationTarget(decision.Current), thresholdMessage(decision), "")
	}
	policy, err := s.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if policy.NotifyOnly {
		return nil
	}
	if policy.MaintenanceMode {
		slog.Info("auto switch skipped: maintenance mode enabled",
			"group", decision.Group.Name,
			"reason", decision.Reason,
		)
		return nil
	}
	return s.ExecuteSwitch(ctx, decision)
}

func (s *Service) BuildDecision(ctx context.Context, groupID string) (SwitchDecision, error) {
	group, err := s.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return SwitchDecision{}, err
	}
	cfg, err := s.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		return SwitchDecision{}, err
	}
	nodes, err := s.Store.ListNodeUsagesByGroup(ctx, groupID, s.now())
	if err != nil {
		return SwitchDecision{}, err
	}
	if len(nodes) == 0 {
		return SwitchDecision{}, sql.ErrNoRows
	}
	current, err := s.ResolveCurrentNode(ctx, group, cfg, nodes)
	if err != nil {
		var unresolved *CurrentNodeUnresolvedError
		if errors.As(err, &unresolved) {
			msg := fmt.Sprintf(
				"⚠️ 分组 %s：DNS 记录 %s 当前指向 %s，但未匹配任何已配置节点。自动切换已暂停，请在 Telegram 中检查 DNS 或节点 IP。",
				group.Name,
				unresolved.RecordName,
				unresolved.DNSIP,
			)
			_, _ = s.notifyOnce(ctx, "dns_unmatched", unresolved.GroupID+":"+unresolved.DNSIP, msg, "")
			return SwitchDecision{Group: group, Config: cfg}, nil
		}
		return SwitchDecision{}, err
	}
	policy, err := s.Store.GetPolicy(ctx)
	if err != nil {
		return SwitchDecision{}, err
	}
	if inCooldown(group, s.now()) {
		return SwitchDecision{Group: group, Config: cfg, Current: current}, nil
	}
	reason := reasonForSwitch(current, policy, s.now())
	if reason == "" {
		return SwitchDecision{Group: group, Config: cfg, Current: current}, nil
	}
	target, ok := SelectTarget(current.ID, nodes, policy, s.now())
	if !ok {
		msg := noTargetMessage(group, current, reason, len(nodes), 0)
		_, _ = s.notifyOnce(ctx, "no_target", noTargetNotificationTarget(group.ID, current, reason), msg, "")
		return SwitchDecision{Group: group, Config: cfg, Current: current, Reason: reason, Triggered: false}, nil
	}
	return SwitchDecision{
		Group:       group,
		Config:      cfg,
		Current:     current,
		Target:      target,
		TriggerType: switchTriggerTypeForReason(reason),
		Reason:      reason,
		Triggered:   true,
	}, nil
}

func (s *Service) ResolveCurrentNode(ctx context.Context, group db.Group, cfg db.CloudflareConfig, nodes []db.NodeUsage) (db.NodeUsage, error) {
	if group.CurrentNodeID.Valid {
		for _, node := range nodes {
			if node.ID == group.CurrentNodeID.String {
				return node, nil
			}
		}
	}
	recordID := cfg.RecordID
	zoneID := cfg.ZoneID
	if zoneID == "" {
		id, err := s.DNS.LookupZoneID(ctx, cfg.APIToken, cfg.ZoneName)
		if err != nil {
			return db.NodeUsage{}, err
		}
		zoneID = id
	}
	record, err := s.DNS.LookupDNSRecordWithType(ctx, cfg.APIToken, zoneID, cfg.RecordName, dnsRecordType(cfg, ""))
	if err != nil {
		return db.NodeUsage{}, err
	}
	if recordID == "" || zoneID != cfg.ZoneID {
		_, _ = s.Store.CreateOrUpdateCloudflareConfig(ctx, cfg.GroupID, cfg.RecordName, record.ID, dnsRecordType(cfg, record.Type), cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.PublicIP) == strings.TrimSpace(record.Content) {
			_ = s.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
			return node, nil
		}
	}
	return db.NodeUsage{}, &CurrentNodeUnresolvedError{
		GroupID:    group.ID,
		RecordName: cfg.RecordName,
		DNSIP:      strings.TrimSpace(record.Content),
	}
}

func reasonForSwitch(current db.NodeUsage, policy db.Policy, now time.Time) string {
	if !current.Enabled {
		return switchReasonDisabled
	}
	if !current.AutoSwitch {
		return switchReasonNoAuto
	}
	if !current.Online && current.LastReportedAt.Valid {
		if now.Sub(current.LastReportedAt.Time) > time.Duration(policy.AgentOfflineSeconds)*time.Second {
			return switchReasonOffline
		}
	}
	if db.UsagePercent(current.UsedBytes, current.MonthlyQuotaBytes) >= float64(current.ThresholdPercent) {
		return switchReasonThreshold
	}
	return ""
}

func SelectTarget(currentID string, nodes []db.NodeUsage, policy db.Policy, now time.Time) (db.NodeUsage, bool) {
	candidates := make([]db.NodeUsage, 0, len(nodes))
	for _, node := range nodes {
		if node.ID == currentID || !node.Enabled || !node.AutoSwitch {
			continue
		}
		if !node.Online && node.LastReportedAt.Valid && now.Sub(node.LastReportedAt.Time) > time.Duration(policy.AgentOfflineSeconds)*time.Second {
			continue
		}
		if db.UsagePercent(node.UsedBytes, node.MonthlyQuotaBytes) >= float64(node.ThresholdPercent) {
			continue
		}
		candidates = append(candidates, node)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority < candidates[j].Priority
		}
		left := db.UsagePercent(candidates[i].UsedBytes, candidates[i].MonthlyQuotaBytes)
		right := db.UsagePercent(candidates[j].UsedBytes, candidates[j].MonthlyQuotaBytes)
		if left != right {
			return left < right
		}
		return candidates[i].Name < candidates[j].Name
	})
	if len(candidates) == 0 {
		return db.NodeUsage{}, false
	}
	return candidates[0], true
}

func (s *Service) ExecuteSwitch(ctx context.Context, decision SwitchDecision) error {
	cfg := decision.Config
	zoneID := cfg.ZoneID
	if zoneID == "" {
		id, err := s.DNS.LookupZoneID(ctx, cfg.APIToken, cfg.ZoneName)
		if err != nil {
			return err
		}
		zoneID = id
	}
	recordID := cfg.RecordID
	if recordID == "" {
		record, err := s.DNS.LookupDNSRecordWithType(ctx, cfg.APIToken, zoneID, cfg.RecordName, dnsRecordType(cfg, ""))
		if err != nil {
			return err
		}
		recordID = record.ID
	}
	err := s.DNS.UpdateDNSRecordWithType(ctx, cfg.APIToken, zoneID, recordID, cfg.RecordName, decision.Target.PublicIP, dnsRecordType(cfg, ""), cfg.TTL, cfg.Proxied)
	if err != nil {
		slog.Error("dns switch failed",
			"group", decision.Group.Name,
			"record", cfg.RecordName,
			"from", decision.Current.Name,
			"to", decision.Target.Name,
			"trigger", decision.TriggerType,
			"error", err,
		)
		_ = s.Store.SetStatusNote(ctx, noteKeyDNSUpdate(decision.Group.ID), "❌ DNS 修改失败")
		_ = s.Store.SaveLastError(ctx, errorKeyDNSUpdate(decision.Group.ID), err.Error(), cfg.APIToken)
		_ = s.Store.RecordSwitchHistory(ctx, decision.Group.ID, decision.Current.ID, decision.Target.ID, cfg.RecordName, decision.Current.PublicIP, decision.Target.PublicIP, decision.TriggerType, decision.Reason, "failed", err.Error(), cfg.APIToken)
		msg := switchFailedMessage(decision, err)
		_ = s.notify(ctx, "switch_failed", decision.Group.ID, msg, sanitizedSwitchError(err))
		return err
	}
	if err := s.Store.UpdateGroupCurrentNode(ctx, decision.Group.ID, decision.Target.ID); err != nil {
		return err
	}
	_ = s.Store.SetStatusNote(ctx, noteKeyDNSUpdate(decision.Group.ID), "✅ DNS 修改成功")
	_ = s.Store.ClearLastError(ctx, errorKeyDNSUpdate(decision.Group.ID))
	if err := s.Store.RecordSwitchHistory(ctx, decision.Group.ID, decision.Current.ID, decision.Target.ID, cfg.RecordName, decision.Current.PublicIP, decision.Target.PublicIP, decision.TriggerType, decision.Reason, "success", ""); err != nil {
		return err
	}
	msg := switchOKMessage(decision)
	_ = s.notify(ctx, "switch_ok", decision.Group.ID, msg, "")
	slog.Info("dns switch succeeded",
		"group", decision.Group.Name,
		"record", cfg.RecordName,
		"from", decision.Current.Name,
		"to", decision.Target.Name,
		"old_ip", decision.Current.PublicIP,
		"new_ip", decision.Target.PublicIP,
		"trigger", decision.TriggerType,
	)
	return nil
}

func (s *Service) CheckOfflineNodes(ctx context.Context) error {
	policy, err := s.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	nodes, err := s.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, node := range nodes {
		if !node.LastReportedAt.Valid {
			continue
		}
		if now.Sub(node.LastReportedAt.Time) <= time.Duration(policy.AgentOfflineSeconds)*time.Second {
			continue
		}
		if node.Online {
			_ = s.Store.MarkNodeOffline(ctx, node.ID)
			node.Online = false
		}
		cfg, _ := s.Store.GetCloudflareConfigByGroupID(ctx, node.GroupID)
		msg := offlineMessage(node, cfg, policy, now)
		target := offlineNotificationTarget(node.ID, node.LastReportedAt.Time)
		_, _ = s.notifyOnce(ctx, "offline", target, msg, "")
	}
	return nil
}

func (s *Service) HandleAgentRecovery(ctx context.Context, previous, current db.Node) error {
	if !previous.LastReportedAt.Valid {
		return nil
	}
	policy, err := s.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if previous.Online && s.now().Sub(previous.LastReportedAt.Time) <= time.Duration(policy.AgentOfflineSeconds)*time.Second {
		return nil
	}
	group, err := s.Store.GetGroupByID(ctx, current.GroupID)
	if err != nil {
		return err
	}
	target := offlineNotificationTarget(previous.ID, previous.LastReportedAt.Time)
	hadOfflineNotification, err := s.Store.HasNotification(ctx, "offline", target)
	if err != nil || !hadOfflineNotification {
		return err
	}
	msg := recoveredMessage(current, group)
	_, _ = s.notifyOnce(ctx, "recovered", target, msg, "")
	return nil
}

func (s *Service) notify(ctx context.Context, kind, targetRef, message, errMsg string) error {
	status := "sent"
	if s.Notifier != nil {
		if err := s.Notifier.SendAdminMessage(ctx, message); err != nil {
			status = "failed"
			if errMsg == "" {
				errMsg = err.Error()
			}
			errMsg = sanitizeStatusMessage(errMsg)
			_ = s.Store.SaveLastError(ctx, errorKeyNotification, errMsg)
		} else {
			_ = s.Store.ClearLastError(ctx, errorKeyNotification)
		}
	}
	return s.Store.RecordNotification(ctx, kind, targetRef, message, status, errMsg)
}

func (s *Service) notifyOnce(ctx context.Context, kind, targetRef, message, errMsg string) (bool, error) {
	inserted, err := s.Store.RecordNotificationOnce(ctx, kind, targetRef, message, "pending", errMsg)
	if err != nil || !inserted {
		return inserted, err
	}
	status := "sent"
	if s.Notifier != nil {
		if err := s.Notifier.SendAdminMessage(ctx, message); err != nil {
			status = "failed"
			if errMsg == "" {
				errMsg = err.Error()
			}
			errMsg = sanitizeStatusMessage(errMsg)
			_ = s.Store.SaveLastError(ctx, errorKeyNotification, errMsg)
		} else {
			_ = s.Store.ClearLastError(ctx, errorKeyNotification)
		}
	}
	if err := s.Store.UpdateNotificationStatus(ctx, kind, targetRef, status, errMsg); err != nil {
		return true, err
	}
	return true, nil
}

func inCooldown(group db.Group, now time.Time) bool {
	if !group.LastSwitchAt.Valid {
		return false
	}
	return now.Sub(group.LastSwitchAt.Time) < time.Duration(group.SwitchCooldownSeconds)*time.Second
}

func thresholdMessage(d SwitchDecision) string {
	usagePercent := db.UsagePercent(d.Current.UsedBytes, d.Current.MonthlyQuotaBytes)
	switchNote := "自动切换：准备选择可用节点"
	if strings.TrimSpace(d.Target.ID) != "" {
		switchNote = fmt.Sprintf("自动切换：准备切换到 %s / %s", d.Target.Name, d.Target.PublicIP)
	}
	return fmt.Sprintf(
		"⚠️ 流量阈值触发\n\n分组：%s\n节点：%s\n统计模式：%s\n月总量：%s\n已使用：%s\n使用率：%.1f%%\n阈值：%d%%\nDNS：%s\n当前解析 IP：%s\n\n%s",
		d.Group.Name,
		d.Current.Name,
		modeLabel(d.Current.TrafficMode),
		humanBytes(d.Current.MonthlyQuotaBytes),
		humanBytes(d.Current.UsedBytes),
		usagePercent,
		d.Current.ThresholdPercent,
		d.Config.RecordName,
		d.Current.PublicIP,
		switchNote,
	)
}

func switchOKMessage(d SwitchDecision) string {
	return fmt.Sprintf(
		"✅ DNS 自动切换成功\n\n触发原因：%s\n分组：%s\n域名：%s\n旧节点：%s / %s\n新节点：%s / %s\nCloudflare：已确认",
		switchReasonLabel(d),
		d.Group.Name,
		d.Config.RecordName,
		d.Current.Name,
		d.Current.PublicIP,
		d.Target.Name,
		d.Target.PublicIP,
	)
}

func switchFailedMessage(d SwitchDecision, err error) string {
	return fmt.Sprintf(
		"❌ DNS 自动切换失败\n\n触发原因：%s\n分组：%s\n域名：%s\n目标节点：%s / %s\n错误：%s",
		switchReasonLabel(d),
		d.Group.Name,
		d.Config.RecordName,
		d.Target.Name,
		d.Target.PublicIP,
		sanitizedSwitchError(err),
	)
}

func switchTriggerTypeForReason(reason string) string {
	switch strings.TrimSpace(reason) {
	case switchReasonOffline:
		return db.SwitchTriggerOffline
	case switchReasonDisabled, switchReasonNoAuto:
		return db.SwitchTriggerDisabled
	default:
		return db.SwitchTriggerThreshold
	}
}

func noTargetMessage(group db.Group, current db.NodeUsage, reason string, nodeCount, availableTargets int) string {
	return fmt.Sprintf(
		"⚠️ 没有可用切换目标\n\n分组：%s\n当前节点：%s\n原因：%s\n节点总数：%d\n可用目标：%d\n\n请添加新节点或调整节点策略。",
		group.Name,
		current.Name,
		noTargetReasonLabel(reason),
		nodeCount,
		availableTargets,
	)
}

func offlineMessage(node db.NodeWithGroup, cfg db.CloudflareConfig, policy db.Policy, now time.Time) string {
	offlineMinutes := int(now.Sub(node.LastReportedAt.Time).Minutes())
	if offlineMinutes < 1 {
		offlineMinutes = 1
	}
	return fmt.Sprintf(
		"🔴 节点离线\n\n节点：%s\n分组：%s\n最后上报：%s\n离线时长：%d 分钟\n当前 DNS：%s -> %s\n\n自动切换：%s",
		node.Name,
		node.GroupName,
		node.LastReportedAt.Time.Local().Format("2006-01-02 15:04:05"),
		offlineMinutes,
		valueOrDash(cfg.RecordName),
		node.PublicIP,
		ternaryText(policy.AutoSwitchEnabled && node.AutoSwitch, "启用", "关闭"),
	)
}

func recoveredMessage(node db.Node, group db.Group) string {
	return fmt.Sprintf(
		"🟢 节点恢复在线\n\n节点：%s\n分组：%s\n当前公网 IP：%s",
		node.Name,
		group.Name,
		node.PublicIP,
	)
}

func thresholdNotificationTarget(node db.NodeUsage) string {
	return node.ID + ":" + node.CycleStart
}

func noTargetNotificationTarget(groupID string, current db.NodeUsage, reason string) string {
	target := groupID + ":" + current.ID + ":" + switchTriggerTypeForReason(reason)
	switch switchTriggerTypeForReason(reason) {
	case db.SwitchTriggerThreshold:
		return target + ":" + current.CycleStart
	case db.SwitchTriggerOffline:
		if current.LastReportedAt.Valid {
			return target + ":" + current.LastReportedAt.Time.UTC().Format(time.RFC3339Nano)
		}
	}
	return target
}

func offlineNotificationTarget(nodeID string, lastReportedAt time.Time) string {
	return nodeID + ":" + lastReportedAt.UTC().Format(time.RFC3339Nano)
}

func switchReasonLabel(d SwitchDecision) string {
	switch d.TriggerType {
	case db.SwitchTriggerOffline:
		return "节点离线"
	case db.SwitchTriggerManual:
		return "手动切换"
	case db.SwitchTriggerDisabled:
		return "节点禁用"
	default:
		return "流量阈值"
	}
}

func noTargetReasonLabel(reason string) string {
	switch strings.TrimSpace(reason) {
	case switchReasonThreshold:
		return "当前节点达到流量阈值"
	case switchReasonOffline:
		return "当前节点离线"
	default:
		return reason
	}
}

func sanitizedSwitchError(err error) string {
	return sanitizeStatusMessage(friendlyCloudflareError(err))
}

func humanBytes(v int64) string {
	const gb = 1024 * 1024 * 1024
	return fmt.Sprintf("%.2fGB", float64(v)/gb)
}

func modeLabel(mode string) string {
	switch mode {
	case db.TrafficModeRX:
		return "单向 RX"
	case db.TrafficModeTX:
		return "单向 TX"
	default:
		return "双向 RX+TX"
	}
}
