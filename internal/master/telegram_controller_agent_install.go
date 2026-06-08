package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/version"
)

func (c *TelegramController) createAgentInstallCommand(ctx context.Context, nodeID string, policy db.Policy) (string, time.Time, []string, error) {
	preview, err := c.buildAgentInstallPreview(ctx, nodeID, policy)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	return preview.Command, preview.ExpiresAt, preview.Missing, nil
}

func (c *TelegramController) getAgentInstallPreview(ctx context.Context, nodeID string, policy db.Policy, refresh bool) (agentInstallPreview, error) {
	base, err := c.buildAgentInstallPreviewBase(ctx, nodeID)
	if err != nil {
		return agentInstallPreview{}, err
	}
	if len(base.Missing) > 0 {
		if refresh {
			c.clearAgentInstallPreview(nodeID)
		}
		return base, nil
	}
	if !refresh {
		if cached, ok := c.cachedAgentInstallPreview(nodeID); ok {
			return cached, nil
		}
	}
	preview, err := c.finalizeAgentInstallPreview(ctx, base, nodeID, policy)
	if err != nil {
		return agentInstallPreview{}, err
	}
	c.storeAgentInstallPreview(nodeID, preview)
	return preview, nil
}

func (c *TelegramController) buildAgentInstallPreview(ctx context.Context, nodeID string, policy db.Policy) (agentInstallPreview, error) {
	base, err := c.buildAgentInstallPreviewBase(ctx, nodeID)
	if err != nil {
		return agentInstallPreview{}, err
	}
	if len(base.Missing) > 0 {
		return base, nil
	}
	return c.finalizeAgentInstallPreview(ctx, base, nodeID, policy)
}

func (c *TelegramController) buildAgentInstallPreviewBase(ctx context.Context, nodeID string) (agentInstallPreview, error) {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "节点不存在")
		return agentInstallPreview{}, err
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "节点所属分组不存在")
		return agentInstallPreview{}, err
	}
	status, err := BuildSetupStatus(ctx, c.Store, c.PublicAPIURL)
	if err != nil {
		return agentInstallPreview{}, err
	}
	preview := agentInstallPreview{
		Node:  node,
		Group: group,
	}
	missing := AgentInstallMissingItems(status)
	if len(missing) > 0 {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成 Agent 安装命令前缺少："+strings.Join(missing, "、"))
		preview.Missing = missing
		return preview, nil
	}
	publicURL, err := ValidateMasterPublicURL(status.PublicAPIURL)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return agentInstallPreview{}, err
	}
	preview.PublicURL = publicURL
	preview.DNSReady, preview.WarningLines, err = c.buildAgentInstallWarnings(ctx, group)
	if err != nil {
		return agentInstallPreview{}, err
	}
	_ = c.Store.ClearLastError(ctx, errorKeyAgentInstall)
	return preview, nil
}

func (c *TelegramController) finalizeAgentInstallPreview(ctx context.Context, preview agentInstallPreview, nodeID string, policy db.Policy) (agentInstallPreview, error) {
	code, expiresAt, err := c.Store.GenerateJoinCodeWithExpiry(ctx, nodeID, 30*time.Minute)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成加入码失败")
		return agentInstallPreview{}, err
	}
	command, err := BuildAgentInstallCommand(preview.PublicURL, installURL(policy), code)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return agentInstallPreview{}, err
	}
	preview.Command = command
	preview.ExpiresAt = expiresAt
	_ = c.Store.ClearLastError(ctx, errorKeyAgentInstall)
	return preview, nil
}

func (c *TelegramController) buildAgentInstallWarnings(ctx context.Context, group db.Group) (bool, []string, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, []string{
				"⚠️ 当前分组还没有 DNS A 记录。",
				"Agent 可以先安装，但 DNS 自动切换不会生效。",
				"建议先完成 DNS 配置。",
			}, nil
		}
		return false, nil, err
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return false, []string{
			"⚠️ 当前分组还没有 DNS A 记录。",
			"Agent 可以先安装，但 DNS 自动切换不会生效。",
			"建议先完成 DNS 配置。",
		}, nil
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		if c.DNS != nil && strings.TrimSpace(cfg.ZoneID) != "" && strings.TrimSpace(cfg.APIToken) != "" {
			record, lookupErr := lookupGroupDNSRecord(ctx, c.DNS, cfg)
			if lookupErr == nil {
				cfg.RecordID = record.ID
				_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, dnsRecordType(cfg, record.Type), cfg.TTL, cfg.Proxied, cfg.AllowOverride)
			}
		}
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return false, []string{
			fmt.Sprintf("⚠️ 当前分组的 DNS 记录 %s 还处于待绑定状态。", cfg.RecordName),
			"请先在 DNS 面板把记录绑定到某个节点，再生成最终安装命令。",
		}, nil
	}
	if c.DNS == nil || strings.TrimSpace(cfg.ZoneID) == "" || strings.TrimSpace(cfg.APIToken) == "" {
		return true, nil, nil
	}
	record, err := lookupGroupDNSRecord(ctx, c.DNS, cfg)
	if err != nil {
		return true, nil, nil
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return true, nil, err
	}
	for _, node := range nodes {
		if node.PublicIP == record.Content {
			return true, nil, nil
		}
	}
	return true, []string{
		fmt.Sprintf("⚠️ 当前 DNS %s 解析到 %s，但没有匹配任何已配置节点。", cfg.RecordName, record.Content),
		"自动切换前建议先把 DNS 指向某个已配置节点。",
	}, nil
}

func formatAgentInstallMessage(preview agentInstallPreview) string {
	var b strings.Builder
	b.WriteString("🤖 Agent 安装命令预览\n\n")
	b.WriteString("节点：" + preview.Node.Name + "\n")
	b.WriteString("分组：" + preview.Group.Name + "\n")
	b.WriteString("Master：" + preview.PublicURL + "\n")
	if len(preview.WarningLines) > 0 {
		b.WriteString("\n")
		for _, line := range preview.WarningLines {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n请点击下面按钮复制安装命令。\n")
	b.WriteString("如果客户端不支持复制按钮，可点击“显示纯安装命令”。\n\n")
	b.WriteString("join code 有效期：30 分钟")
	if !preview.ExpiresAt.IsZero() {
		b.WriteString("（到 " + preview.ExpiresAt.Local().Format("2006-01-02 15:04:05") + "）")
	}
	return strings.TrimSpace(b.String())
}

func formatDNSSavedMessage(groupName, recordName, currentIP, matchedNodeName string, created bool) string {
	title := "✅ DNS A 记录已保存"
	if created {
		title = "✅ DNS A 记录已创建"
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString("分组：" + groupName + "\n")
	b.WriteString("域名：" + valueOrDash(recordName) + "\n")
	b.WriteString("当前 IP：" + valueOrDash(currentIP) + "\n")
	b.WriteString("匹配节点：" + valueOrDash(matchedNodeName) + "\n\n")
	b.WriteString("下一步：生成 Agent 安装命令")
	return b.String()
}

func formatDNSPendingMessage(groupName, recordName string) string {
	return fmt.Sprintf(
		"⏳ DNS A 记录已保存为待绑定\n\n分组：%s\n域名：%s\n状态：还没有节点，稍后可在 DNS 面板中选择节点并创建记录。\n\n下一步：添加节点",
		groupName,
		valueOrDash(recordName),
	)
}

func manualSwitchSuccessMessage(decision SwitchDecision) string {
	return fmt.Sprintf(
		"✅ 手动切换完成\n\n域名：%s\n旧节点：%s\n旧 IP：%s\n新节点：%s\n新 IP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Current.Name),
		valueOrDash(decision.Current.PublicIP),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
}

func manualSwitchAlreadyOnTargetMessage(decision SwitchDecision) string {
	return fmt.Sprintf(
		"当前 DNS 已经指向目标节点。\n\n域名：%s\n节点：%s\nIP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
}

func formatNodeTrafficOffsetSavedMessage(node db.Node, usage db.NodeUsage) string {
	return fmt.Sprintf(
		"✅ 当前已用流量已校准\n\n节点：%s\n初始已用：%s\nAgent 增量：%s\n合计已用：%s / %s\n使用率：%.1f%%",
		node.Name,
		humanBytes(usage.TrafficOffsetBytes),
		humanBytes(usage.AgentUsedBytes),
		humanBytes(usage.UsedBytes),
		humanBytes(usage.MonthlyQuotaBytes),
		db.UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes),
	)
}

func agentUninstallCommand() string {
	return "bash <(curl -fsSL " + version.DefaultUninstallAgentURL() + ") --yes"
}

func (c *TelegramController) sendNodeCreatedSummary(ctx context.Context, chatID int64, node db.Node) error {
	hasDNS, err := c.groupHasDNSConfig(ctx, node.GroupID)
	if err != nil {
		return err
	}
	if !hasDNS {
		pending, pendingErr := c.groupHasPendingDNSConfig(ctx, node.GroupID)
		if pendingErr != nil {
			return pendingErr
		}
		if pending {
			return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：当前分组已有待绑定的 DNS A 记录，请先到 DNS 面板绑定到这个节点。", nodeCreatedMenu(node.ID, false))
		}
		return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：当前分组还没有绑定 DNS A 记录，请先配置 DNS。", nodeCreatedMenu(node.ID, false))
	}
	return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：生成 Agent 安装命令。", nodeCreatedMenu(node.ID, true))
}

func (c *TelegramController) groupHasDNSConfig(ctx context.Context, groupID string) (bool, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) != "", nil
}

func (c *TelegramController) groupHasPendingDNSConfig(ctx context.Context, groupID string) (bool, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) == "", nil
}
