package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	UpdateDNSRecord(ctx context.Context, token, zoneID, recordID, recordName, ip string, ttl int, proxied bool) error
}

type Service struct {
	Store    *db.Store
	Notifier TelegramNotifier
	DNS      DNSProvider
	Now      TimeNowFunc
}

type SwitchDecision struct {
	Group     db.Group
	Config    db.CloudflareConfig
	Current   db.NodeUsage
	Target    db.NodeUsage
	Reason    string
	Triggered bool
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
	if s.Notifier != nil {
		_ = s.Notifier.SendAdminMessage(ctx, thresholdMessage(decision))
		_ = s.Store.RecordNotification(ctx, "threshold", decision.Group.ID, thresholdMessage(decision), "sent", "")
	}
	policy, err := s.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if policy.NotifyOnly {
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
		if s.Notifier != nil {
			msg := fmt.Sprintf("没有可用切换节点\n\n分组：%s\n当前节点：%s\n触发原因：%s", group.Name, current.Name, reason)
			_ = s.Notifier.SendAdminMessage(ctx, msg)
			_ = s.Store.RecordNotification(ctx, "no_target", group.ID, msg, "sent", "")
		}
		return SwitchDecision{Group: group, Config: cfg, Current: current, Reason: reason, Triggered: false}, nil
	}
	return SwitchDecision{
		Group:     group,
		Config:    cfg,
		Current:   current,
		Target:    target,
		Reason:    reason,
		Triggered: true,
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
	record, err := s.DNS.LookupDNSRecord(ctx, cfg.APIToken, zoneID, cfg.RecordName)
	if err != nil {
		return db.NodeUsage{}, err
	}
	if recordID == "" || zoneID != cfg.ZoneID {
		_, _ = s.Store.CreateOrUpdateCloudflareConfig(ctx, cfg.GroupID, cfg.RecordName, record.ID, cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.PublicIP) == strings.TrimSpace(record.Content) {
			_ = s.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
			return node, nil
		}
	}
	return nodes[0], nil
}

func reasonForSwitch(current db.NodeUsage, policy db.Policy, now time.Time) string {
	if !current.Enabled {
		return "当前节点已禁用"
	}
	if !current.AutoSwitch {
		return "当前节点不参与自动切换"
	}
	if !current.Online && current.LastReportedAt.Valid {
		if now.Sub(current.LastReportedAt.Time) > time.Duration(policy.AgentOfflineSeconds)*time.Second {
			return "当前节点离线"
		}
	}
	if db.UsagePercent(current.UsedBytes, current.MonthlyQuotaBytes) >= float64(current.ThresholdPercent) {
		return "流量达到阈值"
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
		record, err := s.DNS.LookupDNSRecord(ctx, cfg.APIToken, zoneID, cfg.RecordName)
		if err != nil {
			return err
		}
		recordID = record.ID
	}
	err := s.DNS.UpdateDNSRecord(ctx, cfg.APIToken, zoneID, recordID, cfg.RecordName, decision.Target.PublicIP, cfg.TTL, cfg.Proxied)
	if err != nil {
		_ = s.Store.SetStatusNote(ctx, noteKeyDNSUpdate(decision.Group.ID), "❌ DNS 修改失败")
		_ = s.Store.SaveLastError(ctx, errorKeyDNSUpdate(decision.Group.ID), err.Error(), cfg.APIToken)
		_ = s.Store.RecordSwitchHistory(ctx, decision.Group.ID, decision.Current.ID, decision.Target.ID, cfg.RecordName, decision.Current.PublicIP, decision.Target.PublicIP, decision.Reason, "failed", err.Error(), cfg.APIToken)
		if s.Notifier != nil {
			msg := switchFailedMessage(decision, err)
			_ = s.Notifier.SendAdminMessage(ctx, msg)
			_ = s.Store.RecordNotification(ctx, "switch_failed", decision.Group.ID, msg, "sent", "")
		}
		return err
	}
	if err := s.Store.UpdateGroupCurrentNode(ctx, decision.Group.ID, decision.Target.ID); err != nil {
		return err
	}
	_ = s.Store.SetStatusNote(ctx, noteKeyDNSUpdate(decision.Group.ID), "✅ DNS 修改成功")
	_ = s.Store.ClearLastError(ctx, errorKeyDNSUpdate(decision.Group.ID))
	if err := s.Store.RecordSwitchHistory(ctx, decision.Group.ID, decision.Current.ID, decision.Target.ID, cfg.RecordName, decision.Current.PublicIP, decision.Target.PublicIP, decision.Reason, "success", ""); err != nil {
		return err
	}
	if s.Notifier != nil {
		msg := switchOKMessage(decision)
		_ = s.Notifier.SendAdminMessage(ctx, msg)
		_ = s.Store.RecordNotification(ctx, "switch_ok", decision.Group.ID, msg, "sent", "")
	}
	return nil
}

func inCooldown(group db.Group, now time.Time) bool {
	if !group.LastSwitchAt.Valid {
		return false
	}
	return now.Sub(group.LastSwitchAt.Time) < time.Duration(group.SwitchCooldownSeconds)*time.Second
}

func thresholdMessage(d SwitchDecision) string {
	return fmt.Sprintf(
		"⚠️ 流量阈值触发\n\n分组：%s\n当前节点：%s\n统计模式：%s\n月总量：%s\n已使用：%s\n阈值：%d%%\n当前 DNS：%s -> %s\n\n准备切换到：%s / %s",
		d.Group.Name,
		d.Current.Name,
		modeLabel(d.Current.TrafficMode),
		humanBytes(d.Current.MonthlyQuotaBytes),
		humanBytes(d.Current.UsedBytes),
		d.Current.ThresholdPercent,
		d.Config.RecordName,
		d.Current.PublicIP,
		d.Target.Name,
		d.Target.PublicIP,
	)
}

func switchOKMessage(d SwitchDecision) string {
	return fmt.Sprintf(
		"✅ DNS 已切换\n\n分组：%s\n域名：%s\n旧节点：%s / %s\n新节点：%s / %s\nCloudflare 确认：成功",
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
		"❌ DNS 切换失败\n\n分组：%s\n域名：%s\n旧节点：%s / %s\n目标节点：%s / %s\n原因：%s",
		d.Group.Name,
		d.Config.RecordName,
		d.Current.Name,
		d.Current.PublicIP,
		d.Target.Name,
		d.Target.PublicIP,
		err.Error(),
	)
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
