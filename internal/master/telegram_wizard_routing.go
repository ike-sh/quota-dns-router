package master

import (
	"context"
	"strconv"
	"strings"
)

func (c *TelegramController) handleWizardCallback(ctx context.Context, chatID int64, data string) (bool, error) {
	switch {
	case data == "status_refresh":
		return true, c.sendStatus(ctx, chatID)
	case data == "menu":
		return true, c.sendMenu(ctx, chatID)
	case data == "cf_token":
		if c.isRoute53Provider() {
			return true, c.sendMessageOrEdit(ctx, chatID, "Route53 使用 AWS 默认凭证链（环境变量或 IAM Role），无需配置 API Token。\n\n请直接选择 Hosted Zone。", dnsProviderPanelMenu(c.DNSProviderKind))
		}
		prefix := c.beginFlow(chatID, pendingCloudflareToken, nil)
		return true, c.sendCloudflareTokenPrompt(ctx, chatID, prefix)
	case data == "cf":
		return true, c.sendCloudflarePanel(ctx, chatID, c.replaceSession(chatID))
	case data == "cf_view":
		return true, c.sendCloudflarePanel(ctx, chatID, "")
	case data == "cf_select_zone":
		return true, c.showCloudflareZoneChoices(ctx, chatID, c.replaceSession(chatID))
	case data == "cf_zone_manual":
		prefix := c.beginFlow(chatID, pendingCloudflareZoneName, nil)
		return true, c.sendCloudflareZoneNamePrompt(ctx, chatID, prefix)
	case data == "cf_token_reset":
		prefix := c.beginFlow(chatID, pendingCloudflareToken, nil)
		return true, c.sendCloudflareTokenPrompt(ctx, chatID, prefix)
	case strings.HasPrefix(data, "cf_zone_pick:"):
		index, err := parseCallbackInt(data, "cf_zone_pick:")
		if err != nil {
			return true, c.sendMessageOrEdit(ctx, chatID, "Zone 选择已失效，请重新获取 Zone 列表。", cloudflareZoneMenu())
		}
		return true, c.handleCloudflareZonePick(ctx, chatID, index)
	case data == "dns":
		return true, c.sendDNSPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "dns_status":
		return true, c.sendDNSStatus(ctx, chatID)
	case strings.HasPrefix(data, "dns_view:"):
		groupID := strings.TrimPrefix(data, "dns_view:")
		return true, c.sendDNSDetail(ctx, chatID, groupID, "")
	case data == "dns_add":
		return true, c.startDNSWizard(ctx, chatID, c.replaceSession(chatID))
	case data == "dns_type:A":
		return true, c.handleDNSRecordTypeChoice(ctx, chatID, "A")
	case data == "dns_type:AAAA":
		return true, c.handleDNSRecordTypeChoice(ctx, chatID, "AAAA")
	case strings.HasPrefix(data, "dns_edit_name:"):
		groupID := strings.TrimPrefix(data, "dns_edit_name:")
		return true, c.startDNSRecordPrompt(ctx, chatID, groupID)
	case strings.HasPrefix(data, "dns_edit_ttl:"):
		groupID := strings.TrimPrefix(data, "dns_edit_ttl:")
		return true, c.sendDNSTTLMenu(ctx, chatID, groupID, "")
	case strings.HasPrefix(data, "dns_ttl_set:"):
		payload := strings.TrimPrefix(data, "dns_ttl_set:")
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "TTL 参数已失效，请重新选择。", nil)
		}
		ttl, err := strconv.Atoi(parts[1])
		if err != nil {
			return true, c.sendMessageOrEdit(ctx, chatID, "TTL 参数无效，请重新选择。", nil)
		}
		return true, c.updateDNSTTL(ctx, chatID, parts[0], ttl)
	case strings.HasPrefix(data, "dns_ttl_custom:"):
		groupID := strings.TrimPrefix(data, "dns_ttl_custom:")
		prefix := c.beginFlow(chatID, pendingDNSTTL, map[string]string{sessionKeyGroupID: groupID})
		return true, c.sendPromptAndTrack(ctx, chatID, pendingDNSTTL, prefix+"请发送新的 TTL，支持：60、120、300、1 或 auto。\n\n发送 /cancel 取消。", dnsTTLMenu(groupID))
	case strings.HasPrefix(data, "dns_edit_proxied:"):
		groupID := strings.TrimPrefix(data, "dns_edit_proxied:")
		return true, c.sendDNSProxiedMenu(ctx, chatID, groupID, "")
	case strings.HasPrefix(data, "dns_proxied:"):
		payload := strings.TrimPrefix(data, "dns_proxied:")
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "proxied 参数已失效，请重新选择。", nil)
		}
		return true, c.updateDNSProxied(ctx, chatID, parts[0], parseBool(parts[1], false))
	case strings.HasPrefix(data, "dns_repoint_menu:"):
		groupID := strings.TrimPrefix(data, "dns_repoint_menu:")
		return true, c.sendDNSRepointMenu(ctx, chatID, groupID, "")
	case strings.HasPrefix(data, "dns_repoint_pick:"):
		payload := strings.TrimPrefix(data, "dns_repoint_pick:")
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "DNS 切换参数已失效，请重新选择。", nil)
		}
		return true, c.sendDNSRepointConfirm(ctx, chatID, parts[0], parts[1], "")
	case strings.HasPrefix(data, "dns_repoint_do:"):
		payload := strings.TrimPrefix(data, "dns_repoint_do:")
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "DNS 切换参数已失效，请重新选择。", nil)
		}
		return true, c.handleDNSRepointSwitch(ctx, chatID, parts[0], parts[1])
	case data == "dns_new_group":
		prefix := c.beginFlow(chatID, pendingGroupName, nil)
		return true, c.sendGroupNamePrompt(ctx, chatID, prefix)
	case strings.HasPrefix(data, "dns_group:"):
		groupID := strings.TrimPrefix(data, "dns_group:")
		return true, c.startDNSRecordPrompt(ctx, chatID, groupID)
	case strings.HasPrefix(data, "dns_create:"):
		nodeID := strings.TrimPrefix(data, "dns_create:")
		return true, c.handleDNSCreateRecord(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "dns_repoint:"):
		nodeID := strings.TrimPrefix(data, "dns_repoint:")
		return true, c.handleDNSRepointToNode(ctx, chatID, nodeID)
	case data == "dns_keep_current":
		return true, c.handleDNSKeepCurrent(ctx, chatID)
	case data == "switch":
		return true, c.sendSwitchPanel(ctx, chatID, c.replaceSession(chatID))
	case strings.HasPrefix(data, "switch_group:"):
		groupID := strings.TrimPrefix(data, "switch_group:")
		return true, c.sendSwitchTargetMenu(ctx, chatID, groupID, "")
	case strings.HasPrefix(data, "switch_pick:"):
		parts := strings.SplitN(strings.TrimPrefix(data, "switch_pick:"), ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "手动切换参数已失效，请重新开始。", mainMenu())
		}
		return true, c.sendSwitchConfirm(ctx, chatID, parts[0], parts[1], "")
	case strings.HasPrefix(data, "switch_do:"):
		parts := strings.SplitN(strings.TrimPrefix(data, "switch_do:"), ":", 2)
		if len(parts) != 2 {
			return true, c.sendMessageOrEdit(ctx, chatID, "手动切换参数已失效，请重新开始。", mainMenu())
		}
		decision, err := c.buildManualSwitchDecision(ctx, parts[0], parts[1])
		if err != nil {
			return true, c.sendMessageOrEdit(ctx, chatID, err.Error(), mainMenu())
		}
		return true, c.executeManualSwitch(ctx, chatID, decision)
	case strings.HasPrefix(data, "switch_to_node:"):
		nodeID := strings.TrimPrefix(data, "switch_to_node:")
		node, err := c.Store.GetNodeByID(ctx, nodeID)
		if err != nil {
			return true, c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
		}
		return true, c.sendSwitchConfirm(ctx, chatID, node.GroupID, node.ID, "")
	case data == "groups":
		return true, c.sendGroupsPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "groups_status":
		return true, c.sendGroupsStatus(ctx, chatID)
	case strings.HasPrefix(data, "groups_view:"):
		groupID := strings.TrimPrefix(data, "groups_view:")
		return true, c.sendGroupDetail(ctx, chatID, groupID, "")
	case strings.HasPrefix(data, "groups_rename:"):
		groupID := strings.TrimPrefix(data, "groups_rename:")
		return true, c.startGroupRenamePrompt(ctx, chatID, groupID)
	case strings.HasPrefix(data, "groups_nodes:"):
		groupID := strings.TrimPrefix(data, "groups_nodes:")
		return true, c.sendGroupNodes(ctx, chatID, groupID, "")
	case data == "groups_new":
		prefix := c.beginFlow(chatID, pendingGroupName, nil)
		return true, c.sendGroupNamePrompt(ctx, chatID, prefix)
	case data == "nodes":
		return true, c.sendNodesPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "nodes_status":
		return true, c.sendNodesStatus(ctx, chatID)
	case data == "nodes_add":
		return true, c.startNodeWizard(ctx, chatID, c.replaceSession(chatID))
	case data == "nodes_restart":
		return true, c.startNodeWizard(ctx, chatID, c.replaceSession(chatID))
	case data == "nodes_customize_policy":
		return true, c.startNodePolicyPrompt(ctx, chatID)
	case data == "nodes_priority_default":
		return true, c.handleNodePriorityValue(ctx, chatID, strconv.Itoa(defaultNodePriority))
	case strings.HasPrefix(data, "nodes_group:"):
		groupID := strings.TrimPrefix(data, "nodes_group:")
		return true, c.startNodeNamePrompt(ctx, chatID, groupID)
	case strings.HasPrefix(data, "nodes_view:"):
		nodeID := strings.TrimPrefix(data, "nodes_view:")
		return true, c.sendNodeDetail(ctx, chatID, nodeID, "")
	case strings.HasPrefix(data, "nodes_edit_policy:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_policy:")
		return true, c.sendNodePolicyEditPanel(ctx, chatID, nodeID, "")
	case strings.HasPrefix(data, "nodes_edit_quota:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_quota:")
		return true, c.startNodePolicyFieldEdit(ctx, chatID, nodeID, nodeEditFieldQuota)
	case strings.HasPrefix(data, "nodes_edit_threshold:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_threshold:")
		return true, c.startNodePolicyFieldEdit(ctx, chatID, nodeID, nodeEditFieldThreshold)
	case strings.HasPrefix(data, "nodes_edit_mode:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_mode:")
		return true, c.startNodePolicyFieldEdit(ctx, chatID, nodeID, nodeEditFieldMode)
	case strings.HasPrefix(data, "nodes_edit_reset_day:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_reset_day:")
		return true, c.startNodePolicyFieldEdit(ctx, chatID, nodeID, nodeEditFieldResetDay)
	case strings.HasPrefix(data, "nodes_edit_priority:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_priority:")
		return true, c.startNodePolicyFieldEdit(ctx, chatID, nodeID, nodeEditFieldPriority)
	case strings.HasPrefix(data, "nodes_calibrate_traffic:"):
		nodeID := strings.TrimPrefix(data, "nodes_calibrate_traffic:")
		return true, c.startNodeTrafficOffsetPrompt(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_clear_traffic_offset:"):
		nodeID := strings.TrimPrefix(data, "nodes_clear_traffic_offset:")
		return true, c.clearNodeTrafficOffset(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_traffic_help:"):
		nodeID := strings.TrimPrefix(data, "nodes_traffic_help:")
		return true, c.sendNodeTrafficHelp(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_edit_toggle_enabled:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_toggle_enabled:")
		return true, c.toggleNodeEnabledAndShowPolicy(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_edit_toggle_auto:"):
		nodeID := strings.TrimPrefix(data, "nodes_edit_toggle_auto:")
		return true, c.toggleNodeAutoSwitchAndShowPolicy(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_toggle_enabled:"):
		nodeID := strings.TrimPrefix(data, "nodes_toggle_enabled:")
		return true, c.toggleNodeEnabled(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_toggle_auto:"):
		nodeID := strings.TrimPrefix(data, "nodes_toggle_auto:")
		return true, c.toggleNodeAutoSwitch(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "nodes_mode:"):
		mode := strings.TrimPrefix(data, "nodes_mode:")
		return true, c.handleNodeModeChoice(ctx, chatID, mode)
	case data == "nodes_quota_default":
		return true, c.handleNodeQuotaValue(ctx, chatID, "1000GB")
	case data == "nodes_threshold_default":
		return true, c.handleNodeThresholdValue(ctx, chatID, "80%")
	case data == "nodes_reset_day_default":
		return true, c.handleNodeResetDayValue(ctx, chatID, "1")
	case data == "nodes_confirm":
		return true, c.handleNodeConfirm(ctx, chatID)
	case data == "nodes_save_policy":
		return true, c.handleNodeSavePolicy(ctx, chatID)
	case data == "policy":
		return true, c.sendPolicyPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "policy_quota":
		prefix := c.beginFlow(chatID, pendingPolicyValue, map[string]string{sessionKeyPolicyField: policyFieldQuota})
		return true, c.sendPolicyValuePrompt(ctx, chatID, prefix, policyFieldQuota)
	case data == "policy_threshold":
		prefix := c.beginFlow(chatID, pendingPolicyValue, map[string]string{sessionKeyPolicyField: policyFieldThreshold})
		return true, c.sendPolicyValuePrompt(ctx, chatID, prefix, policyFieldThreshold)
	case data == "policy_reset_day":
		prefix := c.beginFlow(chatID, pendingPolicyValue, map[string]string{sessionKeyPolicyField: policyFieldResetDay})
		return true, c.sendPolicyValuePrompt(ctx, chatID, prefix, policyFieldResetDay)
	case data == "policy_mode":
		return true, c.sendPolicyModeMenu(ctx, chatID)
	case strings.HasPrefix(data, "policy_mode:"):
		mode := strings.TrimPrefix(data, "policy_mode:")
		return true, c.handlePolicyModeChoice(ctx, chatID, mode)
	case data == "policy_toggle_auto":
		return true, c.togglePolicyAutoSwitch(ctx, chatID)
	case data == "policy_toggle_maintenance":
		return true, c.togglePolicyMaintenanceMode(ctx, chatID)
	case strings.HasPrefix(data, "notify_"):
		return true, c.sendPolicyPanel(ctx, chatID, "通知细分开关当前默认启用，后续版本会提供独立开关字段。\n\n")
	case data == "agent":
		return true, c.sendAgentPanel(ctx, chatID, c.replaceSession(chatID))
	case strings.HasPrefix(data, "agent_node:"):
		nodeID := strings.TrimPrefix(data, "agent_node:")
		return true, c.sendAgentInstallCommand(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "agent_copy:"):
		nodeID := strings.TrimPrefix(data, "agent_copy:")
		return true, c.sendPureAgentInstallCommand(ctx, chatID, nodeID)
	case strings.HasPrefix(data, "agent_uninstall_copy:"):
		return true, c.sendPureAgentUninstallCommand(ctx, chatID)
	case strings.HasPrefix(data, "agent_troubleshoot:"):
		nodeID := strings.TrimPrefix(data, "agent_troubleshoot:")
		return true, c.sendAgentTroubleshooting(ctx, chatID, nodeID)
	}
	return false, nil
}

func (c *TelegramController) handlePendingInput(ctx context.Context, chatID int64, state, text string, messageID int64) error {
	switch state {
	case pendingMasterURL:
		return c.saveMasterPublicURL(ctx, chatID, text)
	case pendingCloudflareToken:
		return c.handleCloudflareTokenInput(ctx, chatID, text, messageID)
	case pendingCloudflareZoneName:
		return c.handleCloudflareZoneNameInput(ctx, chatID, text)
	case pendingCloudflareZoneSelect:
		return c.sendMessageOrEdit(ctx, chatID, "请点击 Zone 按钮，或选择“手动输入 Zone Name”。", cloudflareZoneChoicesMenu(c.sessionZones(chatID)))
	case pendingDNSRecordName:
		return c.handleDNSRecordNameInput(ctx, chatID, text)
	case pendingDNSTTL:
		return c.handleDNSTTLInput(ctx, chatID, text)
	case pendingDNSFixSelect:
		return c.sendMessageOrEdit(ctx, chatID, "请点击按钮选择 DNS 处理方式。", dnsFixMenu(nil))
	case pendingGroupName:
		return c.handleGroupNameInput(ctx, chatID, text)
	case pendingNodeName:
		return c.handleNodeNameInput(ctx, chatID, text)
	case pendingNodeIP:
		return c.handleNodeIPInput(ctx, chatID, text)
	case pendingNodeQuota:
		return c.handleNodeQuotaValue(ctx, chatID, text)
	case pendingNodeThreshold:
		return c.handleNodeThresholdValue(ctx, chatID, text)
	case pendingNodeModeSelect:
		return c.sendMessageOrEdit(ctx, chatID, "请点击统计模式按钮继续。", c.nodeTrafficModeMenu(chatID))
	case pendingNodeResetDay:
		return c.handleNodeResetDayValue(ctx, chatID, text)
	case pendingNodePriority:
		return c.handleNodePriorityValue(ctx, chatID, text)
	case pendingNodeTrafficOffset:
		return c.handleNodeTrafficOffsetInput(ctx, chatID, text)
	case pendingNodeConfirm:
		if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
			return c.sendMessageOrEdit(ctx, chatID, "请点击保存策略或取消。", nodePolicyConfirmMenu())
		}
		return c.sendMessageOrEdit(ctx, chatID, "请点击确认创建，或选择修改流量策略/重新填写。", nodeCreateConfirmMenu())
	case pendingPolicyValue:
		return c.handlePolicyValueInput(ctx, chatID, text)
	default:
		return c.sendMessageOrEdit(ctx, chatID, "当前流程已失效，请重新选择操作。", mainMenu())
	}
}
