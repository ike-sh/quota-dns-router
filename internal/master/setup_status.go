package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
)

type SetupStatus struct {
	PublicAPIURL              string
	PublicURLConfigured       bool
	PublicURLWarning          string
	CloudflareTokenMasked     string
	CloudflareTokenConfigured bool
	ZoneName                  string
	ZoneID                    string
	DNSConfigCount            int
	GroupCount                int
	NodeCount                 int
	OnlineAgentCount          int
	AutoSwitchEnabled         bool
	NotifyOnly                bool
	Missing                   []string
}

func BuildSetupStatus(ctx context.Context, store *db.Store, fallbackPublicURL string) (SetupStatus, error) {
	publicURL, err := store.GetMasterPublicURL(ctx, fallbackPublicURL)
	if err != nil {
		return SetupStatus{}, err
	}
	token, zoneName, zoneID, err := store.GetCloudflareDefaults(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	dnsCount, err := store.CountCloudflareConfigs(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	groupCount, err := store.CountGroups(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	nodeCount, err := store.CountNodes(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	onlineCount, err := store.CountOnlineNodes(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	publicURLConfigured := false
	publicURLWarning := MasterPublicURLWarning(publicURL)
	if _, err := ValidateMasterPublicURL(publicURL); err == nil && !IsLocalMasterPublicURL(publicURL) {
		publicURLConfigured = true
	}

	status := SetupStatus{
		PublicAPIURL:              publicURL,
		PublicURLConfigured:       publicURLConfigured,
		PublicURLWarning:          publicURLWarning,
		CloudflareTokenMasked:     config.MaskSecret(token),
		CloudflareTokenConfigured: strings.TrimSpace(token) != "",
		ZoneName:                  zoneName,
		ZoneID:                    zoneID,
		DNSConfigCount:            dnsCount,
		GroupCount:                groupCount,
		NodeCount:                 nodeCount,
		OnlineAgentCount:          onlineCount,
		AutoSwitchEnabled:         policy.AutoSwitchEnabled,
		NotifyOnly:                policy.NotifyOnly,
	}
	status.Missing = MissingSetupItems(status)
	return status, nil
}

func MissingSetupItems(status SetupStatus) []string {
	var missing []string
	if !status.PublicURLConfigured {
		missing = append(missing, "Master 公网地址")
	}
	if !status.CloudflareTokenConfigured {
		missing = append(missing, "Cloudflare Token")
	}
	if strings.TrimSpace(status.ZoneName) == "" {
		missing = append(missing, "Zone Name")
	}
	if strings.TrimSpace(status.ZoneID) == "" {
		missing = append(missing, "Zone ID")
	}
	if status.DNSConfigCount == 0 {
		missing = append(missing, "DNS A 记录")
	}
	if status.GroupCount == 0 {
		missing = append(missing, "分组")
	}
	if status.NodeCount == 0 {
		missing = append(missing, "节点")
	}
	return missing
}

func AgentInstallMissingItems(status SetupStatus) []string {
	var missing []string
	if !status.PublicURLConfigured {
		missing = append(missing, "Master 公网地址")
	}
	if status.GroupCount == 0 {
		missing = append(missing, "分组")
	}
	if status.NodeCount == 0 {
		missing = append(missing, "节点")
	}
	return missing
}

func FormatSetupGuide(status SetupStatus) string {
	var b strings.Builder
	b.WriteString("初始化步骤：\n")
	b.WriteString(stepLine(status.PublicURLConfigured, "1) 配置 Master 公网地址", "/config_master_url"))
	b.WriteString(stepLine(status.CloudflareTokenConfigured, "2) 配置 Cloudflare Token", "/cf"))
	b.WriteString(stepLine(strings.TrimSpace(status.ZoneName) != "" && strings.TrimSpace(status.ZoneID) != "" && status.DNSConfigCount > 0, "3) 配置 Zone / DNS A 记录", "/dns set <分组名> <A记录>"))
	b.WriteString(stepLine(status.GroupCount > 0, "4) 添加分组", "/groups add <分组名>"))
	b.WriteString(stepLine(status.NodeCount > 0, "5) 添加节点", "/nodes add <节点名> <公网IP> <分组名> ..."))
	b.WriteString(stepLine(true, "6) 设置流量策略", "/policy set ..."))
	b.WriteString(stepLine(len(AgentInstallMissingItems(status)) == 0, "7) 生成 Agent 安装命令", "/agent install <节点名>"))
	if len(status.Missing) > 0 {
		b.WriteString("\n缺少配置：")
		b.WriteString(strings.Join(status.Missing, "、"))
		b.WriteString("\n")
	}
	return b.String()
}

func FormatStatusReport(status SetupStatus, summary db.StatusSummary, extras ...StatusReportExtras) string {
	var extra StatusReportExtras
	hasExtras := len(extras) > 0
	if hasExtras {
		extra = extras[0]
	}
	var b strings.Builder
	b.WriteString("📊 当前状态\n\n")
	b.WriteString(fmt.Sprintf("Master URL：%s %s\n", statusOK(status.PublicURLConfigured), valueOrDash(status.PublicAPIURL)))
	b.WriteString("Cloudflare：" + formatCloudflareStatusLine(status, extra.Cloudflare, hasExtras) + "\n")
	b.WriteString(fmt.Sprintf("DNS A 记录：%d\n", status.DNSConfigCount))
	b.WriteString(fmt.Sprintf("分组：%d\n", status.GroupCount))
	b.WriteString(fmt.Sprintf("节点：%d\n", status.NodeCount))
	b.WriteString(fmt.Sprintf("在线 Agent：%d\n", status.OnlineAgentCount))
	b.WriteString("自动切换：" + ternaryText(status.AutoSwitchEnabled, "启用", "关闭") + "\n")
	if status.NotifyOnly {
		b.WriteString("模式：只通知不切换\n")
	}
	if len(status.Missing) > 0 {
		b.WriteString("缺少配置：" + strings.Join(status.Missing, "、") + "\n")
	}

	if hasExtras {
		b.WriteString("\n最近切换：\n")
		b.WriteString(FormatRecentSwitchSummary(extra.RecentSwitch) + "\n")
		b.WriteString("\n最近失败：\n")
		b.WriteString(FormatRecentFailureSummary(extra.RecentFailure) + "\n")
		b.WriteString("\n当前风险：\n")
		b.WriteString(FormatStatusRiskSummary(extra.Risks) + "\n")
	}

	for _, g := range summary.Groups {
		b.WriteString(fmt.Sprintf("\n分组：%s DNS：%s 当前指向：%s %s\n", g.Group.Name, valueOrDash(g.DNSRecord), valueOrDash(g.CurrentNode), valueOrDash(g.CurrentIP)))
		for _, n := range g.Nodes {
			b.WriteString(fmt.Sprintf("- %s %s %.1f%%/%d%% online=%t enabled=%t auto=%t priority=%d last=%s\n", n.Name, n.PublicIP, n.UsagePercent, n.Threshold, n.Online, n.Enabled, n.AutoSwitch, n.Priority, n.LastReported))
		}
	}
	return b.String()
}

func formatCloudflareStatusLine(status SetupStatus, cf CloudflareSummary, hasCF bool) string {
	if !status.CloudflareTokenConfigured {
		return "❌ Token 未配置"
	}
	if hasCF && cf.Verified {
		return "✅ Token 已配置，Zone 已验证"
	}
	if strings.TrimSpace(status.ZoneName) == "" || strings.TrimSpace(status.ZoneID) == "" {
		return "⚠️ Token 已配置，Zone 未配置"
	}
	if hasCF {
		return "⚠️ Token 已配置，Zone 未验证"
	}
	return "✅ Token 已配置 " + status.CloudflareTokenMasked
}

func statusOK(ok bool) string {
	if ok {
		return "✅"
	}
	return "⚠️"
}

func stepLine(done bool, label, command string) string {
	mark := "[ ]"
	if done {
		mark = "[x]"
	}
	return fmt.Sprintf("%s %s：%s\n", mark, label, command)
}
