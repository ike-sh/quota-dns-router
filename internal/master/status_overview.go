package master

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"quota-dns-router-go/internal/db"
)

const maxStatusRisks = 8

type StatusOverview struct {
	Setup         SetupStatus
	Summary       db.StatusSummary
	Cloudflare    CloudflareSummary
	DNS           []DNSSummary
	Groups        []GroupDiagnostic
	Nodes         []NodeDiagnostic
	RecentSwitch  StatusSwitchSummary
	RecentFailure StatusFailureSummary
	Risks         StatusRiskSummary
}

type StatusReportExtras struct {
	Cloudflare    CloudflareSummary
	RecentSwitch  StatusSwitchSummary
	RecentFailure StatusFailureSummary
	Risks         StatusRiskSummary
}

type StatusSwitchSummary struct {
	Has          bool
	GroupName    string
	RecordName   string
	OldIP        string
	NewIP        string
	OldNode      string
	NewNode      string
	SwitchedAt   time.Time
	Status       string
	ErrorMessage string
}

type StatusFailureSummary struct {
	Has        bool
	Kind       string
	GroupName  string
	Message    string
	OccurredAt time.Time
	Priority   int
}

type StatusRiskInput struct {
	Setup      SetupStatus
	Cloudflare CloudflareSummary
	DNS        []DNSSummary
	Groups     []GroupDiagnostic
	Nodes      []NodeDiagnostic
}

type StatusRiskSummary struct {
	Items  []string
	Hidden int
}

func BuildStatusOverview(ctx context.Context, store *db.Store, fallbackPublicURL string, dns DNSProvider, now time.Time) (StatusOverview, error) {
	if now.IsZero() {
		now = time.Now()
	}
	setup, err := BuildSetupStatus(ctx, store, fallbackPublicURL)
	if err != nil {
		return StatusOverview{}, err
	}
	summary, err := store.BuildStatusSummary(ctx, now)
	if err != nil {
		return StatusOverview{}, err
	}
	cf, err := BuildCloudflareSummary(ctx, store, dns)
	if err != nil {
		return StatusOverview{}, err
	}
	dnsItems, err := BuildDNSSummaries(ctx, store, dns)
	if err != nil {
		return StatusOverview{}, err
	}
	groups, err := BuildGroupDiagnostics(ctx, store, now, dns)
	if err != nil {
		return StatusOverview{}, err
	}
	nodes, err := BuildNodeDiagnostics(ctx, store, now)
	if err != nil {
		return StatusOverview{}, err
	}
	recentSwitch, err := BuildRecentSwitchSummary(ctx, store)
	if err != nil {
		return StatusOverview{}, err
	}
	recentFailure, err := BuildLatestFailureSummary(ctx, store)
	if err != nil {
		return StatusOverview{}, err
	}
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup:      setup,
		Cloudflare: cf,
		DNS:        dnsItems,
		Groups:     groups,
		Nodes:      nodes,
	})
	return StatusOverview{
		Setup:         setup,
		Summary:       summary,
		Cloudflare:    cf,
		DNS:           dnsItems,
		Groups:        groups,
		Nodes:         nodes,
		RecentSwitch:  recentSwitch,
		RecentFailure: recentFailure,
		Risks:         risks,
	}, nil
}

func (o StatusOverview) ReportExtras() StatusReportExtras {
	return StatusReportExtras{
		Cloudflare:    o.Cloudflare,
		RecentSwitch:  o.RecentSwitch,
		RecentFailure: o.RecentFailure,
		Risks:         o.Risks,
	}
}

func BuildRecentSwitchSummary(ctx context.Context, store *db.Store) (StatusSwitchSummary, error) {
	item, err := store.GetLatestSwitchHistory(ctx)
	if err != nil {
		return StatusSwitchSummary{}, err
	}
	return switchSummaryFromHistory(item), nil
}

func BuildLatestFailureSummary(ctx context.Context, store *db.Store) (StatusFailureSummary, error) {
	var candidates []StatusFailureSummary
	addLastError := func(key, kind, groupName string, priority int) {
		item, err := store.GetLastError(ctx, key)
		if err != nil || strings.TrimSpace(item.Message) == "" {
			return
		}
		candidates = append(candidates, StatusFailureSummary{
			Has:        true,
			Kind:       kind,
			GroupName:  groupName,
			Message:    sanitizeStatusMessage(item.Message),
			OccurredAt: item.CreatedAt,
			Priority:   priority,
		})
	}

	failedSwitch, err := store.GetLatestFailedSwitchHistory(ctx)
	if err != nil {
		return StatusFailureSummary{}, err
	}
	if failedSwitch.ID != "" {
		message := failedSwitch.ErrorMessage
		if strings.TrimSpace(message) == "" {
			message = "原因未记录"
		}
		candidates = append(candidates, StatusFailureSummary{
			Has:        true,
			Kind:       "DNS 修改失败",
			GroupName:  failedSwitch.GroupName,
			Message:    sanitizeStatusMessage(message),
			OccurredAt: failedSwitch.SwitchedAt,
			Priority:   1,
		})
	}

	addLastError(errorKeyCloudflareZone, "Cloudflare Zone 查询失败", "", 2)
	groups, err := store.ListGroups(ctx)
	if err != nil {
		return StatusFailureSummary{}, err
	}
	for _, group := range groups {
		addLastError(errorKeyDNSUpdate(group.ID), "DNS 修改失败", group.Name, 1)
		addLastError(errorKeyDNSLookup(group.ID), "DNS 查询失败", group.Name, 3)
	}
	addLastError(errorKeyAgentInstall, "Agent install/join 生成失败", "", 4)
	addLastError(errorKeyAgentReportAuth, "Agent 上报鉴权失败", "", 5)

	if len(candidates) == 0 {
		return StatusFailureSummary{}, nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].OccurredAt.Equal(candidates[j].OccurredAt) {
			return candidates[i].OccurredAt.After(candidates[j].OccurredAt)
		}
		return candidates[i].Priority < candidates[j].Priority
	})
	return candidates[0], nil
}

func BuildStatusRiskSummary(input StatusRiskInput) StatusRiskSummary {
	var items []statusRiskItem
	seen := make(map[string]bool)
	add := func(priority int, message string) {
		message = strings.TrimSpace(message)
		if message == "" || seen[message] {
			return
		}
		seen[message] = true
		items = append(items, statusRiskItem{priority: priority, message: message})
	}

	if MasterPublicURLWarning(input.Setup.PublicAPIURL) != "" || input.Setup.PublicURLWarning != "" {
		add(1, "⚠️ Master Public API URL 仍是本机地址")
	}
	if !input.Setup.CloudflareTokenConfigured {
		add(2, "⚠️ Cloudflare Token 未配置")
	}
	if input.Setup.CloudflareTokenConfigured && zoneNeedsVerification(input.Setup, input.Cloudflare) {
		add(2, "⚠️ Zone 未验证")
	}
	if input.Setup.DNSConfigCount == 0 {
		add(2, "⚠️ 没有 DNS A 记录")
	}
	if !isBlankOrDash(input.Cloudflare.LastError) {
		add(2, "⚠️ Cloudflare 配置错误："+sanitizeStatusMessage(input.Cloudflare.LastError))
	}
	for _, item := range input.DNS {
		if !isBlankOrDash(item.LastError) {
			add(2, fmt.Sprintf("⚠️ DNS 配置错误[%s]：%s", valueOrDash(item.GroupName), sanitizeStatusMessage(item.LastError)))
		}
		if !isBlankOrDash(item.CurrentIP) && !item.IPMatchesNode {
			add(3, fmt.Sprintf("⚠️ %s DNS 当前 IP %s 不匹配任何节点", valueOrDash(item.GroupName), item.CurrentIP))
		}
	}
	for _, group := range input.Groups {
		if group.NodeCount > 0 && group.AvailableTargetCount == 0 {
			add(4, fmt.Sprintf("⚠️ %s 没有可用切换目标", valueOrDash(group.Name)))
		}
	}
	if !input.Setup.AutoSwitchEnabled {
		add(5, "⚠️ 自动切换关闭")
	}
	for _, node := range input.Nodes {
		if node.ReachedThreshold {
			add(6, fmt.Sprintf("⚠️ %s 已达到阈值", valueOrDash(node.Name)))
		}
	}
	for _, node := range input.Nodes {
		if !node.Online {
			add(7, fmt.Sprintf("⚠️ %s Agent 离线", valueOrDash(node.Name)))
		}
	}
	for _, group := range input.Groups {
		if isCooldownText(group.CooldownRemaining) {
			add(8, fmt.Sprintf("⚠️ %s 当前处于 cooldown（%s）", valueOrDash(group.Name), group.CooldownRemaining))
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		return items[i].message < items[j].message
	})
	limit := maxStatusRisks
	if len(items) < limit {
		limit = len(items)
	}
	out := StatusRiskSummary{Items: make([]string, 0, limit)}
	for _, item := range items[:limit] {
		out.Items = append(out.Items, item.message)
	}
	if len(items) > maxStatusRisks {
		out.Hidden = len(items) - maxStatusRisks
	}
	return out
}

func FormatRecentSwitchSummary(summary StatusSwitchSummary) string {
	if !summary.Has {
		return "暂无"
	}
	icon := "✅"
	if strings.EqualFold(summary.Status, "failed") || strings.EqualFold(summary.Status, "failure") || strings.EqualFold(summary.Status, "error") {
		icon = "❌"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %s\n", icon, formatStatusTime(summary.SwitchedAt)))
	b.WriteString("分组：" + valueOrDash(summary.GroupName) + "\n")
	b.WriteString("域名：" + valueOrDash(summary.RecordName) + "\n")
	b.WriteString(fmt.Sprintf("%s / %s -> %s / %s", valueOrDash(summary.OldNode), valueOrDash(summary.OldIP), valueOrDash(summary.NewNode), valueOrDash(summary.NewIP)))
	if summary.ErrorMessage != "" {
		b.WriteString("\n失败原因：" + sanitizeStatusMessage(summary.ErrorMessage))
	}
	return b.String()
}

func FormatRecentFailureSummary(summary StatusFailureSummary) string {
	if !summary.Has {
		return "无"
	}
	kind := summary.Kind
	if summary.GroupName != "" {
		kind += "[" + summary.GroupName + "]"
	}
	return "❌ " + kind + "：" + sanitizeStatusMessage(summary.Message)
}

func FormatStatusRiskSummary(summary StatusRiskSummary) string {
	if len(summary.Items) == 0 {
		return "无"
	}
	lines := append([]string{}, summary.Items...)
	if summary.Hidden > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ 还有 %d 条风险未显示", summary.Hidden))
	}
	return strings.Join(lines, "\n")
}

type statusRiskItem struct {
	priority int
	message  string
}

func switchSummaryFromHistory(item db.SwitchHistory) StatusSwitchSummary {
	if item.ID == "" {
		return StatusSwitchSummary{}
	}
	return StatusSwitchSummary{
		Has:          true,
		GroupName:    firstNonEmpty(item.GroupName, item.GroupID),
		RecordName:   item.RecordName,
		OldIP:        item.OldIP,
		NewIP:        item.NewIP,
		OldNode:      firstNonEmpty(item.FromNodeName, item.FromNodeID),
		NewNode:      firstNonEmpty(item.ToNodeName, item.ToNodeID),
		SwitchedAt:   item.SwitchedAt,
		Status:       item.Status,
		ErrorMessage: sanitizeStatusMessage(item.ErrorMessage),
	}
}

func zoneNeedsVerification(status SetupStatus, cf CloudflareSummary) bool {
	if strings.TrimSpace(status.ZoneName) == "" || strings.TrimSpace(status.ZoneID) == "" {
		return true
	}
	if cf.Verified {
		return false
	}
	return !isBlankOrDash(cf.Status)
}

func isCooldownText(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "-" && value != "无" && value != "0s"
}

func isBlankOrDash(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "-"
}

func formatStatusTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func sanitizeStatusMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	for _, pattern := range sensitiveStatusPatterns {
		message = pattern.ReplaceAllString(message, "${1}[已脱敏]")
	}
	return message
}

var sensitiveStatusPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(token\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(api[_-]?token\s*[:=]\s*)[^\s,;]+`),
}
