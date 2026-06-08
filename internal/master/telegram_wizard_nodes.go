package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"quota-dns-router-go/internal/db"
)

func (c *TelegramController) sendNodesPanel(ctx context.Context, chatID int64, prefix string) error {
	nodes, err := c.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("%s🖥 节点管理\n\n当前节点：%d\n\n请选择操作，或直接点开某个节点查看详情：", prefix, len(nodes))
	return c.sendMessageOrEdit(ctx, chatID, text, nodesPanelMenu(nodes))
}

func (c *TelegramController) sendNodesStatus(ctx context.Context, chatID int64) error {
	items, err := BuildNodeDiagnostics(ctx, c.Store, timeNow())
	if err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, FormatNodeDiagnostics(items), nodesPanelMenu(nil))
}

func (c *TelegramController) startNodeWizard(ctx context.Context, chatID int64, prefix string) error {
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return c.sendMessageOrEdit(ctx, chatID, prefix+"还没有分组，请先创建分组。", nodesNeedGroupMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, prefix+"请选择节点所属分组：", nodesGroupMenu(groups))
}

func (c *TelegramController) startNodeNamePrompt(ctx context.Context, chatID int64, groupID string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", nodesPanelMenu(nil))
	}
	c.beginFlow(chatID, pendingNodeName, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
		sessionKeyNodeFlow:  "create",
	})
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeName, "请发送节点名称，例如：\nhk-01\n\n发送 /cancel 取消。", nil)
}

func (c *TelegramController) handleNodeNameInput(ctx context.Context, chatID int64, nodeName string) error {
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" {
		c.setSession(chatID, pendingNodeName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 节点名称不能为空，请重新发送。", nil)
	}
	if _, err := c.Store.GetNodeByName(ctx, nodeName); err == nil {
		c.setSession(chatID, pendingNodeName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 节点名称已存在，请换一个名称。", nil)
	}
	c.setSessionValue(chatID, sessionKeyNodeName, nodeName)
	c.completePrompt(ctx, chatID)
	recordType := GroupDNSRecordType(ctx, c.Store, c.currentSessionValue(chatID, sessionKeyGroupID))
	c.setSession(chatID, pendingNodeIP)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeIP, nodeIPPrompt(recordType), nil)
}

func (c *TelegramController) handleNodeIPInput(ctx context.Context, chatID int64, ipText string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	ipText = strings.TrimSpace(ipText)
	recordType := GroupDNSRecordType(ctx, c.Store, c.currentSessionValue(chatID, sessionKeyGroupID))
	if err := ValidatePublicIP(ipText, recordType); err != nil {
		c.setSession(chatID, pendingNodeIP)
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送公网 IP。", nil)
	}
	c.setSessionValue(chatID, sessionKeyNodeIP, ipText)
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "default")
	c.setSessionValue(chatID, sessionKeyNodeQuota, strconv.FormatInt(policy.DefaultMonthlyQuotaBytes, 10))
	c.setSessionValue(chatID, sessionKeyNodeThreshold, strconv.Itoa(policy.DefaultThresholdPercent))
	c.setSessionValue(chatID, sessionKeyNodeTrafficMode, policy.DefaultTrafficMode)
	c.setSessionValue(chatID, sessionKeyNodeResetDay, strconv.Itoa(policy.DefaultResetDay))
	c.setSessionValue(chatID, sessionKeyNodePriority, strconv.Itoa(defaultNodePriority))
	c.completePrompt(ctx, chatID)
	c.setSession(chatID, pendingNodeConfirm)
	return c.sendMessageOrEdit(ctx, chatID, c.buildNodeConfirmText(chatID), nodeCreateConfirmMenu())
}

func (c *TelegramController) startNodePolicyPrompt(ctx context.Context, chatID int64) error {
	if c.currentSessionValue(chatID, sessionKeyNodeName) == "" || c.currentSessionValue(chatID, sessionKeyGroupID) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "节点配置流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeQuota)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeQuota, "请发送月流量，例如：500GB、1TB、1000GB。\n\n可直接点击默认值。", nodeQuotaMenu())
}

func (c *TelegramController) startNodePolicyEditWizard(ctx context.Context, chatID int64, nodeID string) error {
	return c.sendNodePolicyEditPanel(ctx, chatID, nodeID, "")
}

func (c *TelegramController) sendNodePolicyEditPanel(ctx context.Context, chatID int64, nodeID, prefix string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	text := prefix + fmt.Sprintf(
		"修改节点策略：%s\n\n当前：\n月流量：%s\n阈值：%d%%\n统计：%s\n重置日：%d\n优先级：%d\n启用：%t\n自动切换：%t\n\n请选择要修改的项目：",
		node.Name,
		formatNodeQuota(strconv.FormatInt(node.MonthlyQuotaBytes, 10)),
		node.ThresholdPercent,
		modeLabel(node.TrafficMode),
		node.ResetDay,
		node.Priority,
		node.Enabled,
		node.AutoSwitch,
	)
	return c.sendMessageOrEdit(ctx, chatID, text, nodePolicyEditMenu(node))
}

func (c *TelegramController) startNodePolicyFieldEdit(ctx context.Context, chatID int64, nodeID, field string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点所属分组不存在，请重新选择。", nodesPanelMenu(nil))
	}
	c.beginFlow(chatID, pendingNodeQuota, map[string]string{
		sessionKeyNodeFlow:         "edit_field",
		sessionKeyNodeID:           node.ID,
		sessionKeyGroupID:          group.ID,
		sessionKeyGroupName:        group.Name,
		sessionKeyNodeName:         node.Name,
		sessionKeyNodeIP:           node.PublicIP,
		sessionKeyNodeQuota:        strconv.FormatInt(node.MonthlyQuotaBytes, 10),
		sessionKeyNodeThreshold:    strconv.Itoa(node.ThresholdPercent),
		sessionKeyNodeTrafficMode:  node.TrafficMode,
		sessionKeyNodeResetDay:     strconv.Itoa(node.ResetDay),
		sessionKeyNodePriority:     strconv.Itoa(node.Priority),
		sessionKeyNodeEditField:    field,
		sessionKeyNodePolicySource: "custom",
	})
	switch field {
	case nodeEditFieldQuota:
		c.setSession(chatID, pendingNodeQuota)
		return c.sendPromptAndTrack(ctx, chatID, pendingNodeQuota, "请发送新的月流量，例如：500GB、1TB、1000GB。", nodeQuotaMenu())
	case nodeEditFieldThreshold:
		c.setSession(chatID, pendingNodeThreshold)
		return c.sendPromptAndTrack(ctx, chatID, pendingNodeThreshold, "请发送新的阈值百分比，例如：80 或 80%。", nodeThresholdMenu())
	case nodeEditFieldMode:
		c.setSession(chatID, pendingNodeModeSelect)
		return c.sendPromptAndTrack(ctx, chatID, pendingNodeModeSelect, "请选择新的统计模式：", c.nodeTrafficModeMenu(chatID))
	case nodeEditFieldResetDay:
		c.setSession(chatID, pendingNodeResetDay)
		return c.sendPromptAndTrack(ctx, chatID, pendingNodeResetDay, "请发送新的重置日（1-28）。", nodeResetDayMenu())
	case nodeEditFieldPriority:
		c.setSession(chatID, pendingNodePriority)
		return c.sendPromptAndTrack(ctx, chatID, pendingNodePriority, "请发送新的 priority。", nodePriorityMenu())
	default:
		c.clearSession(chatID)
		return c.sendMessageOrEdit(ctx, chatID, "节点策略修改项已失效，请重新选择。", nodesPanelMenu(nil))
	}
}

func (c *TelegramController) saveNodePolicyFieldEdit(ctx context.Context, chatID int64, successPrefix string) error {
	nodeID := c.currentSessionValue(chatID, sessionKeyNodeID)
	if nodeID == "" {
		return c.sendMessageOrEdit(ctx, chatID, "节点策略修改流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	quotaBytes, _ := strconv.ParseInt(c.currentSessionValue(chatID, sessionKeyNodeQuota), 10, 64)
	threshold, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeThreshold))
	resetDay, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeResetDay))
	priority, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodePriority))
	node.MonthlyQuotaBytes = quotaBytes
	node.ThresholdPercent = threshold
	node.ResetDay = resetDay
	node.TrafficMode = c.currentSessionValue(chatID, sessionKeyNodeTrafficMode)
	node.Priority = priority
	if err := ValidateNodeConfig(node, GroupDNSRecordType(ctx, c.Store, node.GroupID)); err != nil {
		field := c.currentSessionValue(chatID, sessionKeyNodeEditField)
		return c.startNodePolicyFieldEdit(ctx, chatID, nodeID, field)
	}
	if err := c.Store.UpdateNodePolicy(ctx, node); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "保存节点策略失败："+err.Error(), nodePolicyEditMenu(node))
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	return c.sendNodeDetail(ctx, chatID, node.ID, successPrefix)
}

func (c *TelegramController) toggleNodeEnabledAndShowPolicy(ctx context.Context, chatID int64, nodeID string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	node.Enabled = !node.Enabled
	if err := c.Store.SetNodeEnabled(ctx, node.ID, node.Enabled); err != nil {
		return err
	}
	return c.sendNodePolicyEditPanel(ctx, chatID, node.ID, "✅ 已更新节点启用状态。\n\n")
}

func (c *TelegramController) toggleNodeAutoSwitchAndShowPolicy(ctx context.Context, chatID int64, nodeID string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	node.AutoSwitch = !node.AutoSwitch
	if err := c.Store.SetNodeAutoSwitch(ctx, node.ID, node.AutoSwitch); err != nil {
		return err
	}
	return c.sendNodePolicyEditPanel(ctx, chatID, node.ID, "✅ 已更新自动切换状态。\n\n")
}

func (c *TelegramController) handleNodeQuotaValue(ctx context.Context, chatID int64, raw string) error {
	bytes, err := parseGB(raw)
	if err != nil || bytes <= 0 {
		c.setSession(chatID, pendingNodeQuota)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 流量总额格式错误，请发送类似 500GB、1TB、1000GB 的值。", nodeQuotaMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeQuota, strconv.FormatInt(bytes, 10))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.completePrompt(ctx, chatID)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit_field" {
		return c.saveNodePolicyFieldEdit(ctx, chatID, "✅ 月流量已更新。\n\n")
	}
	c.setSession(chatID, pendingNodeThreshold)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeThreshold, "请发送阈值百分比，例如：80 或 80%。\n\n可直接点击默认值。", nodeThresholdMenu())
}

func (c *TelegramController) handleNodeThresholdValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(raw), "%"))
	if err != nil || value < 1 || value > 100 {
		c.setSession(chatID, pendingNodeThreshold)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 阈值必须在 1-100 之间，请重新发送。", nodeThresholdMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeThreshold, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.completePrompt(ctx, chatID)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit_field" {
		return c.saveNodePolicyFieldEdit(ctx, chatID, "✅ 阈值已更新。\n\n")
	}
	c.setSession(chatID, pendingNodeModeSelect)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeModeSelect, "请选择统计模式：", c.nodeTrafficModeMenu(chatID))
}

func (c *TelegramController) handleNodeModeChoice(ctx context.Context, chatID int64, mode string) error {
	mode = normalizeMode(mode)
	c.setSessionValue(chatID, sessionKeyNodeTrafficMode, mode)
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.completePrompt(ctx, chatID)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit_field" {
		return c.saveNodePolicyFieldEdit(ctx, chatID, "✅ 统计模式已更新。\n\n")
	}
	c.setSession(chatID, pendingNodeResetDay)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeResetDay, "请发送重置日（1-28）。\n\n可直接点击默认值。", nodeResetDayMenu())
}

func (c *TelegramController) handleNodeResetDayValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > 28 {
		c.setSession(chatID, pendingNodeResetDay)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 重置日必须在 1-28 之间，请重新发送。", nodeResetDayMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeResetDay, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.completePrompt(ctx, chatID)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit_field" {
		return c.saveNodePolicyFieldEdit(ctx, chatID, "✅ 重置日已更新。\n\n")
	}
	c.setSession(chatID, pendingNodePriority)
	return c.sendPromptAndTrack(ctx, chatID, pendingNodePriority, "请发送 priority（默认 10）。\n\n可直接点击默认值。", nodePriorityMenu())
}

func (c *TelegramController) handleNodePriorityValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		c.setSession(chatID, pendingNodePriority)
		return c.sendMessageOrEdit(ctx, chatID, "❌ priority 不能小于 0，请重新发送。", nodePriorityMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodePriority, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit_field" {
		return c.saveNodePolicyFieldEdit(ctx, chatID, "✅ priority 已更新。\n\n")
	}
	c.setSession(chatID, pendingNodeConfirm)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
		return c.sendMessageOrEdit(ctx, chatID, c.buildNodeConfirmText(chatID), nodePolicyConfirmMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, c.buildNodeConfirmText(chatID), nodeCreateConfirmMenu())
}

func (c *TelegramController) buildNodeConfirmText(chatID int64) string {
	policyLines := fmt.Sprintf(
		"月流量：%s\n阈值：%s%%\n统计：%s\n重置日：%s\n优先级：%s",
		formatNodeQuota(c.currentSessionValue(chatID, sessionKeyNodeQuota)),
		c.currentSessionValue(chatID, sessionKeyNodeThreshold),
		modeLabel(c.currentSessionValue(chatID, sessionKeyNodeTrafficMode)),
		c.currentSessionValue(chatID, sessionKeyNodeResetDay),
		c.currentSessionValue(chatID, sessionKeyNodePriority),
	)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
		return fmt.Sprintf(
			"请确认节点策略：\n\n节点：%s\n分组：%s\n公网 IP：%s\n\n%s",
			c.currentSessionValue(chatID, sessionKeyNodeName),
			c.currentSessionValue(chatID, sessionKeyGroupName),
			c.currentSessionValue(chatID, sessionKeyNodeIP),
			policyLines,
		)
	}
	title := "流量策略："
	if c.currentSessionValue(chatID, sessionKeyNodePolicySource) != "custom" {
		title = "将使用默认流量策略："
	}
	return fmt.Sprintf(
		"请确认节点配置：\n\n节点：%s\n分组：%s\n公网 IP：%s\n\n%s\n%s",
		c.currentSessionValue(chatID, sessionKeyNodeName),
		c.currentSessionValue(chatID, sessionKeyGroupName),
		c.currentSessionValue(chatID, sessionKeyNodeIP),
		title,
		policyLines,
	)
}

func (c *TelegramController) handleNodeConfirm(ctx context.Context, chatID int64) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	if groupID == "" || c.currentSessionValue(chatID, sessionKeyGroupName) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "节点配置流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	quotaBytes, _ := strconv.ParseInt(c.currentSessionValue(chatID, sessionKeyNodeQuota), 10, 64)
	threshold, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeThreshold))
	resetDay, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeResetDay))
	priority, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodePriority))
	node := db.Node{
		GroupID:               groupID,
		Name:                  c.currentSessionValue(chatID, sessionKeyNodeName),
		PublicIP:              c.currentSessionValue(chatID, sessionKeyNodeIP),
		MonthlyQuotaBytes:     quotaBytes,
		ThresholdPercent:      threshold,
		ResetDay:              resetDay,
		TrafficMode:           c.currentSessionValue(chatID, sessionKeyNodeTrafficMode),
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              priority,
		PreferredIface:        "auto",
		ReportIntervalSeconds: policy.AgentReportIntervalSeconds,
	}
	if err := ValidateNodeConfig(node, GroupDNSRecordType(ctx, c.Store, groupID)); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新填写节点信息。", nodeCreateConfirmMenu())
	}
	created, err := c.Store.CreateNode(ctx, node)
	if err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 创建节点失败："+err.Error()+"\n\n请重新填写。", nodeCreateConfirmMenu())
	}
	c.clearSession(chatID)
	return c.sendNodeCreatedSummary(ctx, chatID, created)
}

func (c *TelegramController) handleNodeSavePolicy(ctx context.Context, chatID int64) error {
	nodeID := c.currentSessionValue(chatID, sessionKeyNodeID)
	if nodeID == "" {
		return c.sendMessageOrEdit(ctx, chatID, "节点策略修改流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	quotaBytes, _ := strconv.ParseInt(c.currentSessionValue(chatID, sessionKeyNodeQuota), 10, 64)
	threshold, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeThreshold))
	resetDay, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodeResetDay))
	priority, _ := strconv.Atoi(c.currentSessionValue(chatID, sessionKeyNodePriority))
	node.MonthlyQuotaBytes = quotaBytes
	node.ThresholdPercent = threshold
	node.ResetDay = resetDay
	node.TrafficMode = c.currentSessionValue(chatID, sessionKeyNodeTrafficMode)
	node.Priority = priority
	if err := ValidateNodeConfig(node, GroupDNSRecordType(ctx, c.Store, node.GroupID)); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新填写节点策略。", nodePolicyConfirmMenu())
	}
	if err := c.Store.UpdateNodePolicy(ctx, node); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 保存节点策略失败："+err.Error()+"\n\n请重新填写。", nodePolicyConfirmMenu())
	}
	c.clearSession(chatID)
	return c.sendNodeDetail(ctx, chatID, node.ID, "✅ 节点策略已更新。\n\n")
}

func (c *TelegramController) sendNodeDetail(ctx context.Context, chatID int64, nodeID, prefix string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点所属分组不存在，请重新选择。", nodesPanelMenu(nil))
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	usage, err := c.Store.GetNodeUsage(ctx, node, timeNow())
	if err != nil {
		return err
	}
	dnsMatch, err := c.nodeDNSMatches(ctx, group, node)
	if err != nil {
		return err
	}
	lastReportedText := "从未上报"
	hasReported := node.LastReportedAt.Valid
	if hasReported {
		lastReportedText = formatAge(timeNow().Sub(node.LastReportedAt.Time)) + "前"
	}
	online := nodeIsReachable(usage, policy, timeNow())
	agentStatus := "未安装 / 未上线"
	switch {
	case !hasReported:
		agentStatus = "未安装 / 未上线"
	case online:
		agentStatus = "在线（最后上报 " + lastReportedText + "）"
	default:
		agentStatus = "离线（最后上报 " + lastReportedText + "）"
	}
	text := prefix + fmt.Sprintf(
		"🖥 节点详情\n\n节点：%s\n分组：%s\n公网 IP：%s\nAgent：%s\nDNS 匹配：%s\n\n流量统计：\n统计模式：%s\n初始已用：%s\nAgent 增量：%s\n合计已用：%s / %s\n使用率：%.1f%%\n阈值：%d%%\n重置日：%d\n\n优先级：%d\n启用：%t\n自动切换：%t\n\n请选择操作：",
		node.Name,
		group.Name,
		node.PublicIP,
		agentStatus,
		ternaryText(dnsMatch, "是", "否"),
		modeLabel(node.TrafficMode),
		humanBytes(usage.TrafficOffsetBytes),
		humanBytes(usage.AgentUsedBytes),
		humanBytes(usage.UsedBytes),
		humanBytes(usage.MonthlyQuotaBytes),
		db.UsagePercent(usage.UsedBytes, usage.MonthlyQuotaBytes),
		node.ThresholdPercent,
		node.ResetDay,
		node.Priority,
		node.Enabled,
		node.AutoSwitch,
	)
	return c.sendMessageOrEdit(ctx, chatID, text, nodeDetailMenu(node, hasReported, online))
}

func (c *TelegramController) nodeDNSMatches(ctx context.Context, group db.Group, node db.Node) (bool, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if c.DNS != nil && strings.TrimSpace(cfg.ZoneID) != "" && strings.TrimSpace(cfg.APIToken) != "" && strings.TrimSpace(cfg.RecordName) != "" {
		record, err := c.DNS.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
		if err == nil {
			return strings.TrimSpace(record.Content) == strings.TrimSpace(node.PublicIP), nil
		}
	}
	if group.CurrentNodeID.Valid {
		return group.CurrentNodeID.String == node.ID, nil
	}
	return false, nil
}

func (c *TelegramController) toggleNodeEnabled(ctx context.Context, chatID int64, nodeID string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	node.Enabled = !node.Enabled
	if err := c.Store.SetNodeEnabled(ctx, node.ID, node.Enabled); err != nil {
		return err
	}
	return c.sendNodeDetail(ctx, chatID, node.ID, "✅ 节点状态已更新。\n\n")
}

func (c *TelegramController) toggleNodeAutoSwitch(ctx context.Context, chatID int64, nodeID string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	node.AutoSwitch = !node.AutoSwitch
	if err := c.Store.SetNodeAutoSwitch(ctx, node.ID, node.AutoSwitch); err != nil {
		return err
	}
	return c.sendNodeDetail(ctx, chatID, node.ID, "✅ 节点自动切换状态已更新。\n\n")
}

func (c *TelegramController) startNodeTrafficOffsetPrompt(ctx context.Context, chatID int64, nodeID string) error {
	if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	prefix := c.beginFlow(chatID, pendingNodeTrafficOffset, map[string]string{sessionKeyNodeID: nodeID})
	text := prefix + "请发送当前节点本月已用流量，例如：\n\n100GB\n350.5GB\n1TB\n\n这个值会作为本周期初始已用流量，加上 Agent 后续增量一起参与阈值判断。\n\n发送 /cancel 取消。"
	return c.sendPromptAndTrack(ctx, chatID, pendingNodeTrafficOffset, text, nodeTrafficOffsetPromptMenu(nodeID))
}

func (c *TelegramController) handleNodeTrafficOffsetInput(ctx context.Context, chatID int64, text string) error {
	nodeID := c.currentSessionValue(chatID, sessionKeyNodeID)
	if strings.TrimSpace(nodeID) == "" {
		c.clearSession(chatID)
		return c.sendMessageOrEdit(ctx, chatID, "流量校准流程已失效，请重新进入节点详情。", nodesPanelMenu(nil))
	}
	value, err := parseGB(text)
	if err != nil || value < 0 {
		c.setSession(chatID, pendingNodeTrafficOffset)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 已用流量格式错误，请发送类似 350GB、350.5GB 或 1TB 的值。", nodeTrafficOffsetPromptMenu(nodeID))
	}
	return c.saveNodeTrafficOffset(ctx, chatID, nodeID, value)
}

func (c *TelegramController) clearNodeTrafficOffset(ctx context.Context, chatID int64, nodeID string) error {
	return c.saveNodeTrafficOffset(ctx, chatID, nodeID, 0)
}

func (c *TelegramController) saveNodeTrafficOffset(ctx context.Context, chatID int64, nodeID string, bytes int64) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	cycle := db.BillingCycleStart(timeNow(), node.ResetDay).Format("2006-01-02")
	if bytes == 0 {
		cycle = ""
	}
	if err := c.Store.SetNodeTrafficOffset(ctx, node.ID, bytes, cycle); err != nil {
		c.setSession(chatID, pendingNodeTrafficOffset)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 保存当前已用流量失败："+err.Error(), nodeTrafficOffsetPromptMenu(node.ID))
	}
	updated, err := c.Store.GetNodeByID(ctx, node.ID)
	if err != nil {
		return err
	}
	usage, err := c.Store.GetNodeUsage(ctx, updated, timeNow())
	if err != nil {
		return err
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	return c.sendMessageOrEdit(ctx, chatID, formatNodeTrafficOffsetSavedMessage(updated, usage), nodeTrafficOffsetSavedMenu(updated.ID))
}

func (c *TelegramController) sendNodeTrafficHelp(ctx context.Context, chatID int64, nodeID string) error {
	if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	text := "流量统计说明：\n\nAgent 默认读取节点网卡流量，只能统计安装/首次上报后的增量。\n如果这台服务器本月已经用过流量，请点击“校准已用流量”，填入服务商面板显示的本月已用值。\n\n常用检查命令：\ncat /proc/net/dev\nip route get 192.0.2.1\nqdr-agent status\nqdr-agent config-check"
	return c.sendMessageOrEdit(ctx, chatID, text, nodeTrafficHelpMenu(nodeID))
}

func (c *TelegramController) sendAgentTroubleshooting(ctx context.Context, chatID int64, nodeID string) error {
	if nodeID != "" {
		if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
		}
	}
	return c.sendMessageOrEdit(ctx, chatID, "Agent 安装排查：\n\n请在 Agent 机器执行：\ndf -h\nqdr-agent version\nqdr-agent status\nqdr-agent config-check\nsystemctl status quota-dns-router-agent --no-pager -l\njournalctl -u quota-dns-router-agent -n 100 --no-pager\n\n如果 df -h 显示 / 已 100%，请先清理磁盘：\napt clean\njournalctl --vacuum-size=100M\ndocker system prune -af", nil)
}
