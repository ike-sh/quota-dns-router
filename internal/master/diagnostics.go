package master

import (
	"context"
	"fmt"
	"strings"
	"time"

	"quota-dns-router-go/internal/db"
)

const (
	errorKeyCloudflareZone  = "cloudflare_zone_lookup"
	errorKeyAgentInstall    = "agent_install_command"
	errorKeyAgentReportAuth = "agent_report_auth"
	errorKeyNotification    = "notification_delivery"
	noteKeyCloudflareZone   = "cloudflare_zone_result"
)

type CloudflareSummary struct {
	TokenMasked     string
	TokenConfigured bool
	ZoneName        string
	ZoneID          string
	Status          string
	LastResult      string
	LastError       string
	NextSuggestion  []string
	Verified        bool
}

type DNSSummary struct {
	GroupName       string
	GroupID         string
	RecordName      string
	RecordID        string
	RecordType      string
	Pending         bool
	CurrentIP       string
	MatchedNodeName string
	Proxied         bool
	TTL             int
	Status          string
	LastResult      string
	LastError       string
	NextSuggestion  []string
	IPMatchesNode   bool
}

type NodeDiagnostic struct {
	Name               string
	GroupName          string
	PublicIP           string
	Online             bool
	HasReported        bool
	LastReportedText   string
	TrafficMode        string
	TrafficOffsetBytes int64
	AgentUsedBytes     int64
	UsedBytes          int64
	MonthlyQuotaBytes  int64
	UsagePercent       float64
	ThresholdPercent   int
	ResetDay           int
	Enabled            bool
	AutoSwitch         bool
	Priority           int
	ReachedThreshold   bool
	EligibleTarget     bool
}

type GroupDiagnostic struct {
	Name                 string
	DNSRecord            string
	DNSPending           bool
	CurrentIP            string
	CurrentNode          string
	NodeCount            int
	AvailableTargetCount int
	AutoSwitchEnabled    bool
	LastSwitchText       string
	CooldownRemaining    string
	Status               string
}

func noteKeyDNSLookup(groupID string) string  { return "dns_lookup_result:" + groupID }
func noteKeyDNSUpdate(groupID string) string  { return "dns_update_result:" + groupID }
func errorKeyDNSLookup(groupID string) string { return "dns_lookup:" + groupID }
func errorKeyDNSUpdate(groupID string) string { return "dns_update:" + groupID }

func BuildCloudflareSummary(ctx context.Context, store *db.Store, dns DNSProvider) (CloudflareSummary, error) {
	token, zoneName, zoneID, err := store.GetCloudflareDefaults(ctx)
	if err != nil {
		return CloudflareSummary{}, err
	}
	lastResult, _ := store.GetStatusNote(ctx, noteKeyCloudflareZone)
	lastErr, _ := store.GetLastError(ctx, errorKeyCloudflareZone)
	summary := CloudflareSummary{
		TokenMasked:     db.MaskedCloudflare(token),
		TokenConfigured: strings.TrimSpace(token) != "",
		ZoneName:        zoneName,
		ZoneID:          zoneID,
		LastResult:      valueOrDash(lastResult),
		LastError:       valueOrDash(lastErr.Message),
	}
	if !summary.TokenConfigured {
		summary.Status = "❌ Token 未配置"
		summary.NextSuggestion = []string{"执行 /cf <token> <zone_name>", "确认 Token 具备 Zone.Zone Read 权限"}
		return summary, nil
	}
	if strings.TrimSpace(zoneName) == "" {
		summary.Status = "❌ Zone Name 未配置"
		summary.NextSuggestion = []string{"执行 /cf <token> <zone_name>", "Zone Name 请填写根域名，例如 example.com"}
		return summary, nil
	}
	if dns == nil {
		summary.Status = "ℹ️ 已配置，未实时验证"
		summary.NextSuggestion = []string{"使用 /dns set 配置 DNS A 记录"}
		return summary, nil
	}
	foundZoneID, err := dns.LookupZoneID(ctx, token, zoneName)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		summary.Status = "❌ Zone 查询失败"
		summary.LastResult = "❌ Zone 查询失败"
		summary.LastError = msg
		summary.NextSuggestion = []string{
			"确认 Token 有 Zone.Zone Read",
			"确认 Zone Name 是根域名，例如 example.com",
			"重新执行 /cf <token> <zone_name>",
		}
		return summary, nil
	}
	if foundZoneID != zoneID {
		zoneID = foundZoneID
		_ = store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID)
	}
	_ = store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = store.ClearLastError(ctx, errorKeyCloudflareZone)
	summary.ZoneID = zoneID
	summary.Verified = true
	summary.Status = "✅ Zone 已验证"
	summary.LastResult = "✅ Zone 已验证"
	summary.LastError = "-"
	summary.NextSuggestion = []string{"使用 /dns set 配置 DNS A 记录"}
	return summary, nil
}

func BuildDNSSummaries(ctx context.Context, store *db.Store, dns DNSProvider) ([]DNSSummary, error) {
	groups, err := store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	var out []DNSSummary
	for _, group := range groups {
		cfg, err := store.GetCloudflareConfigByGroupID(ctx, group.ID)
		if err != nil {
			continue
		}
		nodes, err := store.ListNodesByGroupID(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		recordType := dnsRecordType(cfg, "")
		summary := DNSSummary{
			GroupName:      group.Name,
			GroupID:        group.ID,
			RecordName:     cfg.RecordName,
			RecordID:       cfg.RecordID,
			RecordType:     recordType,
			Pending:        strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) == "",
			Proxied:        cfg.Proxied,
			TTL:            cfg.TTL,
			LastResult:     valueOrDash(joinNotes(mustStatusNote(ctx, store, noteKeyDNSLookup(group.ID)), mustStatusNote(ctx, store, noteKeyDNSUpdate(group.ID)))),
			LastError:      valueOrDash(firstNonEmpty(mustLastError(ctx, store, errorKeyDNSUpdate(group.ID)), mustLastError(ctx, store, errorKeyDNSLookup(group.ID)))),
			Status:         "ℹ️ 已配置，未实时查询",
			NextSuggestion: []string{"执行 /dns set " + group.Name + " " + cfg.RecordName},
		}
		if dns != nil && cfg.ZoneID != "" && cfg.RecordName != "" {
			rec, aErr := dns.LookupDNSRecordWithType(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName, recordType)
			if aErr != nil {
				if summary.Pending {
					summary.Status = "⏳ 待绑定节点"
					summary.LastResult = valueOrDash(firstNonEmpty(mustStatusNote(ctx, store, noteKeyDNSUpdate(group.ID)), "⏳ 已保存记录名，等待绑定节点"))
					summary.LastError = "-"
					summary.NextSuggestion = []string{"先添加节点，再在 DNS 面板把记录绑定到节点"}
					out = append(out, summary)
					continue
				}
				any, anyErr := dns.LookupDNSRecordAnyType(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
				if anyErr != nil {
					msg := fmt.Sprintf("未找到 DNS %s 记录，请确认记录存在", recordType)
					_ = store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 查询失败")
					_ = store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, cfg.APIToken)
					summary.Status = "❌ DNS 记录不存在"
					summary.LastResult = "❌ DNS 查询失败"
					summary.LastError = msg
					summary.NextSuggestion = []string{"确认 Record Name 正确", "确认 Cloudflare 中已存在该记录", "重新执行 /dns set"}
				} else {
					msg := fmt.Sprintf("记录存在，但类型为 %s，不是 %s 记录", any.Type, recordType)
					_ = store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型错误")
					_ = store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, cfg.APIToken)
					summary.Status = fmt.Sprintf("❌ 记录不是 %s 记录", recordType)
					summary.LastResult = "❌ DNS 记录类型错误"
					summary.LastError = msg
					summary.NextSuggestion = []string{fmt.Sprintf("请改为 %s 记录", recordType), "重新执行 /dns set"}
				}
			} else {
				if summary.Pending {
					summary.Pending = false
					summary.RecordID = rec.ID
					_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, rec.Name, rec.ID, rec.Type, normalizeDNSTTLValue(rec.TTL), rec.Proxied, cfg.AllowOverride)
				}
				summary.TTL = normalizeDNSTTLValue(rec.TTL)
				summary.Proxied = rec.Proxied
				summary.CurrentIP = rec.Content
				summary.RecordID = rec.ID
				summary.RecordType = rec.Type
				_ = store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
				_ = store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
				for _, node := range nodes {
					if node.PublicIP == rec.Content {
						summary.MatchedNodeName = node.Name
						summary.IPMatchesNode = true
						break
					}
				}
				if summary.MatchedNodeName == "" {
					summary.Status = "⚠️ DNS IP 未匹配任何节点"
					summary.LastResult = "✅ DNS 记录查询成功"
					summary.LastError = "-"
					summary.NextSuggestion = []string{fmt.Sprintf("确认当前 %s 记录值与某个节点公网 IP 一致", recordType), "自动切换在 DNS IP 匹配节点时更可靠"}
				} else {
					summary.Status = "✅ DNS 记录正常"
					summary.LastResult = "✅ DNS 记录查询成功"
					summary.LastError = "-"
					summary.NextSuggestion = []string{"如需验收切换，可调整阈值或构造验收流量"}
				}
			}
		}
		if summary.Pending {
			summary.Status = "⏳ 待绑定节点"
			summary.LastResult = valueOrDash(firstNonEmpty(mustStatusNote(ctx, store, noteKeyDNSUpdate(group.ID)), "⏳ 已保存记录名，等待绑定节点"))
			summary.LastError = "-"
			summary.NextSuggestion = []string{"先添加节点，再在 DNS 面板把记录绑定到节点"}
		}
		out = append(out, summary)
	}
	return out, nil
}

func BuildNodeDiagnostics(ctx context.Context, store *db.Store, now time.Time) ([]NodeDiagnostic, error) {
	groups, err := store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	var out []NodeDiagnostic
	for _, group := range groups {
		usages, err := store.ListNodeUsagesByGroup(ctx, group.ID, now)
		if err != nil {
			return nil, err
		}
		for _, usage := range usages {
			lastText := "从未上报"
			hasReported := usage.LastReportedAt.Valid
			if hasReported {
				age := now.Sub(usage.LastReportedAt.Time)
				lastText = formatAge(age) + "前"
			}
			reached := db.UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes) >= float64(usage.ThresholdPercent)
			eligible := usage.Enabled && usage.AutoSwitch && !reached && nodeIsReachable(usage, policy, now) && (!group.CurrentNodeID.Valid || usage.ID != group.CurrentNodeID.String)
			out = append(out, NodeDiagnostic{
				Name:               usage.Name,
				GroupName:          group.Name,
				PublicIP:           usage.PublicIP,
				Online:             nodeIsReachable(usage, policy, now),
				HasReported:        hasReported,
				LastReportedText:   lastText,
				TrafficMode:        usage.TrafficMode,
				TrafficOffsetBytes: usage.TrafficOffsetBytes,
				AgentUsedBytes:     usage.AgentUsedBytes,
				UsedBytes:          usage.UsedBytes,
				MonthlyQuotaBytes:  usage.MonthlyQuotaBytes,
				UsagePercent:       db.UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes),
				ThresholdPercent:   usage.ThresholdPercent,
				ResetDay:           usage.ResetDay,
				Enabled:            usage.Enabled,
				AutoSwitch:         usage.AutoSwitch,
				Priority:           usage.Priority,
				ReachedThreshold:   reached,
				EligibleTarget:     eligible,
			})
		}
	}
	return out, nil
}

func BuildGroupDiagnostics(ctx context.Context, store *db.Store, now time.Time, dns DNSProvider) ([]GroupDiagnostic, error) {
	groups, err := store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	var out []GroupDiagnostic
	for _, group := range groups {
		cfg, _ := store.GetCloudflareConfigByGroupID(ctx, group.ID)
		usages, err := store.ListNodeUsagesByGroup(ctx, group.ID, now)
		if err != nil {
			return nil, err
		}
		hasReported := false
		currentNode := "-"
		currentIP := "-"
		dnsPending := strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) == ""
		if dns != nil && cfg.ZoneID != "" && cfg.RecordName != "" {
			if rec, err := dns.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName); err == nil {
				if dnsPending {
					dnsPending = false
					cfg.RecordID = rec.ID
					_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, rec.Name, rec.ID, rec.Type, cfg.TTL, cfg.Proxied, cfg.AllowOverride)
				}
				currentIP = rec.Content
				for _, usage := range usages {
					if usage.PublicIP == rec.Content {
						currentNode = usage.Name
						break
					}
				}
			}
		}
		availableTargets := 0
		for _, usage := range usages {
			if usage.LastReportedAt.Valid {
				hasReported = true
			}
			if currentNode == "-" && group.CurrentNodeID.Valid && usage.ID == group.CurrentNodeID.String {
				currentNode = usage.Name
				currentIP = usage.PublicIP
				continue
			}
			if usage.Enabled && usage.AutoSwitch && !thresholdReached(usage) && nodeIsReachable(usage, policy, now) {
				availableTargets++
			}
		}
		autoSwitch := policy.AutoSwitchEnabled
		cooldownRemaining := "无"
		if inCooldown(group, now) {
			remain := time.Duration(group.SwitchCooldownSeconds)*time.Second - now.Sub(group.LastSwitchAt.Time)
			if remain < 0 {
				remain = 0
			}
			cooldownRemaining = remain.Truncate(time.Second).String()
		}
		lastSwitchText := "无"
		if group.LastSwitchAt.Valid {
			lastSwitchText = group.LastSwitchAt.Time.Format(time.RFC3339)
		}
		status := "✅ 可自动切换"
		if strings.TrimSpace(cfg.RecordName) == "" {
			status = "⏳ 待配置 DNS A 记录"
		} else if dnsPending {
			status = "⏳ DNS 已保存，等待绑定节点"
			currentNode = "待绑定"
		} else if !autoSwitch {
			status = "⚠️ 全局自动切换未启用"
		} else if availableTargets == 0 {
			if hasReported {
				status = "⚠️ 没有可用切换目标"
			} else {
				status = "⏳ 等待 Agent 首次上线"
			}
		}
		out = append(out, GroupDiagnostic{
			Name:                 group.Name,
			DNSRecord:            cfg.RecordName,
			DNSPending:           dnsPending,
			CurrentIP:            currentIP,
			CurrentNode:          currentNode,
			NodeCount:            len(usages),
			AvailableTargetCount: availableTargets,
			AutoSwitchEnabled:    autoSwitch,
			LastSwitchText:       lastSwitchText,
			CooldownRemaining:    cooldownRemaining,
			Status:               status,
		})
	}
	return out, nil
}

func friendlyCloudflareError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "403"):
		return "Cloudflare 返回 403，请检查 Token 权限是否包含 Zone.Zone Read 或 Zone.DNS Edit"
	case strings.Contains(msg, "未找到 Zone"):
		return "未找到 Zone，请确认 Zone Name 使用根域名，例如 example.com"
	default:
		return msg
	}
}

func thresholdReached(usage db.NodeUsage) bool {
	return db.UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes) >= float64(usage.ThresholdPercent)
}

func nodeIsReachable(usage db.NodeUsage, policy db.Policy, now time.Time) bool {
	if usage.Online {
		return true
	}
	if !usage.LastReportedAt.Valid {
		return false
	}
	return now.Sub(usage.LastReportedAt.Time) <= time.Duration(policy.AgentOfflineSeconds)*time.Second
}

func formatAge(age time.Duration) string {
	if age < time.Minute {
		return fmt.Sprintf("%d 秒", int(age.Seconds()))
	}
	if age < time.Hour {
		return fmt.Sprintf("%d 分钟", int(age.Minutes()))
	}
	return fmt.Sprintf("%d 小时", int(age.Hours()))
}

func mustStatusNote(ctx context.Context, store *db.Store, key string) string {
	value, _ := store.GetStatusNote(ctx, key)
	return value
}

func mustLastError(ctx context.Context, store *db.Store, key string) string {
	value, _ := store.GetLastError(ctx, key)
	return value.Message
}

func joinNotes(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left == "" && right == "":
		return ""
	case left == "":
		return right
	case right == "":
		return left
	default:
		return left + "；" + right
	}
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return item
		}
	}
	return ""
}

func FormatCloudflareSummary(summary CloudflareSummary) string {
	var b strings.Builder
	b.WriteString("☁️ Cloudflare 配置\n\n")
	if summary.TokenConfigured {
		b.WriteString("Token：已配置 " + summary.TokenMasked + "\n")
	} else {
		b.WriteString("Token：未配置\n")
	}
	b.WriteString("Zone Name：" + valueOrDash(summary.ZoneName) + "\n")
	b.WriteString("Zone ID：" + valueOrDash(summary.ZoneID) + "\n")
	b.WriteString("状态：" + valueOrDash(summary.Status) + "\n")
	b.WriteString("最近一次 Zone 查询结果：" + valueOrDash(summary.LastResult) + "\n")
	if summary.LastError != "-" && summary.LastError != "" {
		b.WriteString("最近错误：" + summary.LastError + "\n")
	}
	if len(summary.NextSuggestion) > 0 {
		b.WriteString("\n建议：\n")
		for i, item := range summary.NextSuggestion {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
		}
	}
	return strings.TrimSpace(b.String())
}

func FormatDNSSummaries(items []DNSSummary) string {
	if len(items) == 0 {
		return "🌐 DNS 配置\n\n尚未配置任何分组的 DNS 记录。\n下一步：点击 DNS 面板里的“添加 DNS 记录”；没有分组时会自动创建 default。"
	}
	var b strings.Builder
	b.WriteString("🌐 DNS 配置\n")
	for _, item := range items {
		b.WriteString("\n\n")
		b.WriteString(item.GroupName + "\n")
		b.WriteString("域名：" + valueOrDash(item.RecordName) + "\n")
		b.WriteString("记录类型：" + formatDNSRecordType(item.RecordType) + "\n")
		b.WriteString("Record ID：" + valueOrDash(item.RecordID) + "\n")
		b.WriteString(formatDNSCurrentRecordLine(item.RecordType, item.CurrentIP) + "\n")
		b.WriteString("匹配节点：" + valueOrDash(item.MatchedNodeName) + "\n")
		b.WriteString(fmt.Sprintf("proxied：%t\n", item.Proxied))
		b.WriteString("TTL：" + formatDNSTTL(item.TTL) + "\n")
		b.WriteString("状态：" + valueOrDash(item.Status) + "\n")
		b.WriteString("最近一次查询/修改结果：" + valueOrDash(item.LastResult) + "\n")
		if item.LastError != "-" && item.LastError != "" {
			b.WriteString("最近错误：" + item.LastError + "\n")
		}
		if item.Proxied {
			b.WriteString("提示：proxied=true 可能看到的是 Cloudflare 代理 IP，不一定等于源站 IP。\n")
		}
		if len(item.NextSuggestion) > 0 {
			b.WriteString("建议：" + strings.Join(item.NextSuggestion, "；") + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func FormatNodeDiagnostics(items []NodeDiagnostic) string {
	if len(items) == 0 {
		return "🖥 节点列表\n\n暂无节点。下一步：执行 /nodes add <节点名> <公网IP> <分组名> ..."
	}
	var b strings.Builder
	b.WriteString("🖥 节点列表\n")
	for _, item := range items {
		b.WriteString("\n\n" + item.Name + "\n")
		b.WriteString("IP：" + item.PublicIP + "\n")
		b.WriteString("分组：" + item.GroupName + "\n")
		switch {
		case !item.HasReported:
			b.WriteString("Agent：🟡 未安装 / 未上线\n")
		case item.Online:
			b.WriteString("Agent：✅ 在线，最后上报 " + item.LastReportedText + "\n")
		default:
			b.WriteString("Agent：⚠️ 离线，最后上报 " + item.LastReportedText + "\n")
		}
		b.WriteString("统计：" + modeLabel(item.TrafficMode) + "\n")
		b.WriteString("已用：" + humanBytes(item.UsedBytes) + " / " + humanBytes(item.MonthlyQuotaBytes) + "\n")
		if item.TrafficOffsetBytes > 0 {
			b.WriteString("其中：初始 " + humanBytes(item.TrafficOffsetBytes) + " + Agent 增量 " + humanBytes(item.AgentUsedBytes) + "\n")
		}
		b.WriteString(fmt.Sprintf("使用率：%.1f%%\n", item.UsagePercent))
		b.WriteString(fmt.Sprintf("阈值：%d%%\n", item.ThresholdPercent))
		b.WriteString(fmt.Sprintf("重置日：%d\n", item.ResetDay))
		b.WriteString(fmt.Sprintf("enabled：%t\n", item.Enabled))
		b.WriteString(fmt.Sprintf("auto_switch：%t\n", item.AutoSwitch))
		b.WriteString(fmt.Sprintf("priority：%d\n", item.Priority))
		if item.ReachedThreshold {
			b.WriteString("状态：⚠️ 已达到阈值\n")
		} else if !item.HasReported {
			b.WriteString("状态：🟡 待安装 Agent\n")
		} else if !item.Online {
			b.WriteString("状态：⚠️ Agent 离线\n")
		} else {
			b.WriteString("状态：✅ 可用\n")
		}
		if item.EligibleTarget {
			b.WriteString("可切换目标：是\n")
		} else {
			b.WriteString("可切换目标：否\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func FormatGroupDiagnostics(items []GroupDiagnostic) string {
	if len(items) == 0 {
		return "📦 分组列表\n\n暂无分组。下一步：执行 /groups add <分组名>"
	}
	var b strings.Builder
	b.WriteString("📦 分组列表\n")
	for _, item := range items {
		b.WriteString("\n\n" + item.Name + "\n")
		b.WriteString("DNS：" + valueOrDash(item.DNSRecord) + "\n")
		b.WriteString("当前 IP：" + valueOrDash(item.CurrentIP) + "\n")
		b.WriteString("当前节点：" + valueOrDash(item.CurrentNode) + "\n")
		b.WriteString(fmt.Sprintf("节点数量：%d\n", item.NodeCount))
		b.WriteString(fmt.Sprintf("可用切换目标：%d\n", item.AvailableTargetCount))
		b.WriteString(fmt.Sprintf("自动切换：%s\n", ternaryText(item.AutoSwitchEnabled, "启用", "关闭")))
		b.WriteString("最近一次切换：" + item.LastSwitchText + "\n")
		b.WriteString("Cooldown：" + item.CooldownRemaining + "\n")
		b.WriteString("状态：" + item.Status + "\n")
	}
	return strings.TrimSpace(b.String())
}

func ternaryText(ok bool, a, b string) string {
	if ok {
		return a
	}
	return b
}
