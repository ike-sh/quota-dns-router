package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

const (
	pendingMasterURL            = "master_url"
	pendingCloudflareToken      = "cloudflare_token"
	pendingCloudflareZoneName   = "cloudflare_zone_name"
	pendingCloudflareZoneSelect = "cloudflare_zone_select"
	pendingDNSRecordName        = "dns_record_name"
	pendingDNSFixSelect         = "dns_fix_select"
	pendingGroupName            = "group_name"
	pendingNodeName             = "node_name"
	pendingNodeIP               = "node_ip"
	pendingNodeQuota            = "node_quota"
	pendingNodeThreshold        = "node_threshold"
	pendingNodeModeSelect       = "node_mode_select"
	pendingNodeResetDay         = "node_reset_day"
	pendingNodePriority         = "node_priority"
	pendingNodeConfirm          = "node_confirm"
	pendingPolicyValue          = "policy_value"
	sessionSwitchNotice         = "已切换到新的配置流程。"
	defaultNodePriority         = 10
	defaultDNSRecordTTL         = 120
	defaultDNSRecordProxied     = false
	policyFieldThreshold        = "threshold"
	policyFieldQuota            = "quota"
	policyFieldResetDay         = "reset_day"
	sessionKeyGroupID           = "group_id"
	sessionKeyGroupName         = "group_name"
	sessionKeyNodeID            = "node_id"
	sessionKeyNodeFlow          = "node_flow"
	sessionKeyNodePolicySource  = "node_policy_source"
	sessionKeyNodeName          = "node_name"
	sessionKeyNodeIP            = "node_ip"
	sessionKeyNodeQuota         = "node_quota"
	sessionKeyNodeThreshold     = "node_threshold"
	sessionKeyNodeTrafficMode   = "node_traffic_mode"
	sessionKeyNodeResetDay      = "node_reset_day"
	sessionKeyNodePriority      = "node_priority"
	sessionKeyRecordName        = "record_name"
	sessionKeyRecordID          = "record_id"
	sessionKeyCurrentIP         = "current_ip"
	sessionKeyZoneID            = "zone_id"
	sessionKeyPolicyField       = "policy_field"
)

type telegramSessionMeta struct {
	Data  map[string]string
	Zones []cloudflare.Zone
}

func (c *TelegramController) handleWizardCallback(ctx context.Context, chatID int64, data string) (bool, error) {
	switch {
	case data == "menu":
		return true, c.sendMenu(ctx, chatID)
	case data == "cf_token":
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
			return true, c.Bot.SendMessage(ctx, chatID, "Zone 选择已失效，请重新获取 Zone 列表。", cloudflareZoneMenu())
		}
		return true, c.handleCloudflareZonePick(ctx, chatID, index)
	case data == "dns":
		return true, c.sendDNSPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "dns_status":
		return true, c.sendDNSStatus(ctx, chatID)
	case data == "dns_add":
		return true, c.startDNSWizard(ctx, chatID, c.replaceSession(chatID))
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
	case data == "groups":
		return true, c.sendGroupsPanel(ctx, chatID, c.replaceSession(chatID))
	case data == "groups_status":
		return true, c.sendGroupsStatus(ctx, chatID)
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
		return true, c.startNodePolicyEditWizard(ctx, chatID, nodeID)
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
	case data == "agent":
		return true, c.sendAgentPanel(ctx, chatID, c.replaceSession(chatID))
	case strings.HasPrefix(data, "agent_node:"):
		nodeID := strings.TrimPrefix(data, "agent_node:")
		return true, c.sendAgentInstallCommand(ctx, chatID, nodeID)
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
		return c.Bot.SendMessage(ctx, chatID, "请点击 Zone 按钮，或选择“手动输入 Zone Name”。", cloudflareZoneChoicesMenu(c.sessionZones(chatID)))
	case pendingDNSRecordName:
		return c.handleDNSRecordNameInput(ctx, chatID, text)
	case pendingDNSFixSelect:
		return c.Bot.SendMessage(ctx, chatID, "请点击按钮选择 DNS 处理方式。", dnsFixMenu(nil))
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
		return c.Bot.SendMessage(ctx, chatID, "请点击统计模式按钮继续。", nodeTrafficModeMenu())
	case pendingNodeResetDay:
		return c.handleNodeResetDayValue(ctx, chatID, text)
	case pendingNodePriority:
		return c.handleNodePriorityValue(ctx, chatID, text)
	case pendingNodeConfirm:
		if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
			return c.Bot.SendMessage(ctx, chatID, "请点击保存策略或取消。", nodePolicyConfirmMenu())
		}
		return c.Bot.SendMessage(ctx, chatID, "请点击确认创建，或选择修改流量策略/重新填写。", nodeCreateConfirmMenu())
	case pendingPolicyValue:
		return c.handlePolicyValueInput(ctx, chatID, text)
	default:
		return c.Bot.SendMessage(ctx, chatID, "当前流程已失效，请重新选择操作。", mainMenu())
	}
}

func (c *TelegramController) beginFlow(chatID int64, state string, data map[string]string) string {
	prefix := c.replaceSession(chatID)
	c.setSession(chatID, state)
	meta := c.ensureSessionMeta(chatID)
	meta.Data = cloneStringMap(data)
	meta.Zones = nil
	return prefix
}

func (c *TelegramController) replaceSession(chatID int64) string {
	if c.sessions[chatID] == "" {
		return ""
	}
	c.clearSession(chatID)
	return sessionSwitchNotice + "\n\n"
}

func (c *TelegramController) ensureSessionMeta(chatID int64) *telegramSessionMeta {
	if c.sessionMeta == nil {
		c.sessionMeta = make(map[int64]*telegramSessionMeta)
	}
	meta := c.sessionMeta[chatID]
	if meta == nil {
		meta = &telegramSessionMeta{Data: make(map[string]string)}
		c.sessionMeta[chatID] = meta
	}
	if meta.Data == nil {
		meta.Data = make(map[string]string)
	}
	return meta
}

func (c *TelegramController) getSessionMeta(chatID int64) *telegramSessionMeta {
	if c.sessionMeta == nil {
		return nil
	}
	return c.sessionMeta[chatID]
}

func (c *TelegramController) sessionZones(chatID int64) []cloudflare.Zone {
	meta := c.getSessionMeta(chatID)
	if meta == nil || len(meta.Zones) == 0 {
		return nil
	}
	return append([]cloudflare.Zone(nil), meta.Zones...)
}

func (c *TelegramController) currentSessionValue(chatID int64, key string) string {
	meta := c.getSessionMeta(chatID)
	if meta == nil || meta.Data == nil {
		return ""
	}
	return strings.TrimSpace(meta.Data[key])
}

func (c *TelegramController) setSessionValue(chatID int64, key, value string) {
	meta := c.ensureSessionMeta(chatID)
	meta.Data[key] = value
}

func (c *TelegramController) sendCloudflarePanel(ctx context.Context, chatID int64, prefix string) error {
	summary, err := BuildCloudflareSummary(ctx, c.Store, nil)
	if err != nil {
		return err
	}
	text := prefix + "☁️ Cloudflare 配置\n\n"
	if summary.TokenConfigured {
		text += "Token：已配置 " + summary.TokenMasked + "\n"
	} else {
		text += "Token：未配置\n"
	}
	text += "Zone Name：" + valueOrDash(summary.ZoneName) + "\n"
	text += "Zone ID：" + valueOrDash(maskMiddle(summary.ZoneID, 4, 4)) + "\n\n请选择操作："
	return c.Bot.SendMessage(ctx, chatID, text, cloudflarePanelMenu())
}

func (c *TelegramController) sendCloudflareTokenPrompt(ctx context.Context, chatID int64, prefix string) error {
	text := prefix + "请发送 Cloudflare API Token。\n\n要求：\n- 需要 Zone Read 权限，用于查询 Zone\n- 需要 DNS Edit 权限，用于修改 A 记录\n- Token 只会脱敏显示，不会出现在日志中\n\n发送 /cancel 取消。"
	return c.Bot.SendMessage(ctx, chatID, text, nil)
}

func (c *TelegramController) showCloudflareZoneChoices(ctx context.Context, chatID int64, prefix string) error {
	token, _, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return c.Bot.SendMessage(ctx, chatID, prefix+"请先配置 Cloudflare Token。", cloudflareNeedTokenMenu())
	}
	if c.DNS == nil {
		prefix += "当前进程未配置 Cloudflare 客户端，请手动输入 Zone Name。\n\n"
		c.beginFlow(chatID, pendingCloudflareZoneName, nil)
		return c.sendCloudflareZoneNamePrompt(ctx, chatID, prefix)
	}
	zones, err := c.DNS.ListZones(ctx, token)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		c.beginFlow(chatID, pendingCloudflareToken, nil)
		return c.Bot.SendMessage(ctx, chatID, prefix+"查询 Zone 失败："+msg+"\n\n请重新发送 Cloudflare API Token，或发送 /cancel 取消。", cloudflareNeedTokenMenu())
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	meta := c.ensureSessionMeta(chatID)
	meta.Zones = zones
	c.setSession(chatID, pendingCloudflareZoneSelect)
	title := "请选择要管理的 Zone："
	if len(zones) == 1 {
		title = "检测到 1 个 Zone，是否使用这个 Zone？"
	}
	return c.Bot.SendMessage(ctx, chatID, prefix+title, cloudflareZoneChoicesMenu(zones))
}

func (c *TelegramController) sendCloudflareZoneNamePrompt(ctx context.Context, chatID int64, prefix string) error {
	return c.Bot.SendMessage(ctx, chatID, prefix+"请发送 Zone Name，例如：\nexample.com\n\n发送 /cancel 取消。", nil)
}

func (c *TelegramController) handleCloudflareZonePick(ctx context.Context, chatID int64, index int) error {
	meta := c.getSessionMeta(chatID)
	if meta == nil || index < 0 || index >= len(meta.Zones) {
		return c.Bot.SendMessage(ctx, chatID, "Zone 选择已失效，请重新获取 Zone 列表。", cloudflarePanelMenu())
	}
	zone := meta.Zones[index]
	token, _, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zone.Name, zone.ID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	c.clearSession(chatID)
	return c.Bot.SendMessage(ctx, chatID, fmt.Sprintf("✅ Cloudflare Zone 已保存\n\nZone Name：%s\nZone ID：%s\n\n下一步：配置 DNS A 记录", zone.Name, maskMiddle(zone.ID, 4, 4)), cloudflareSavedMenu())
}

func (c *TelegramController) handleCloudflareTokenInput(ctx context.Context, chatID int64, token string, messageID int64) error {
	token = strings.TrimSpace(token)
	if token == "" {
		c.setSession(chatID, pendingCloudflareToken)
		return c.Bot.SendMessage(ctx, chatID, "❌ Token 不能为空，请重新发送 Cloudflare API Token。", nil)
	}
	if messageID > 0 {
		c.tryDeleteMessage(ctx, chatID, messageID)
	}
	_, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	if c.DNS == nil {
		c.beginFlow(chatID, pendingCloudflareZoneName, nil)
		return c.Bot.SendMessage(ctx, chatID, "✅ Token 已保存："+config.MaskSecret(token)+"\n\n当前进程未配置 Cloudflare 客户端，请手动输入 Zone Name。", cloudflareZoneMenu())
	}
	zones, err := c.DNS.ListZones(ctx, token)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		c.setSession(chatID, pendingCloudflareToken)
		return c.Bot.SendMessage(ctx, chatID, "✅ Token 已保存："+config.MaskSecret(token)+"\n\n查询 Zone 失败："+msg+"\n\n请重新发送 Cloudflare API Token，或发送 /cancel 取消。", nil)
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	meta := c.ensureSessionMeta(chatID)
	meta.Zones = zones
	c.setSession(chatID, pendingCloudflareZoneSelect)
	text := "✅ Token 已保存：" + config.MaskSecret(token) + "\n\n请选择要管理的 Zone："
	if len(zones) == 1 {
		text = "✅ Token 已保存：" + config.MaskSecret(token) + "\n\n检测到 1 个 Zone，是否使用这个 Zone？"
	}
	return c.Bot.SendMessage(ctx, chatID, text, cloudflareZoneChoicesMenu(zones))
}

func (c *TelegramController) handleCloudflareZoneNameInput(ctx context.Context, chatID int64, zoneName string) error {
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" {
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.Bot.SendMessage(ctx, chatID, "❌ Zone Name 不能为空，请重新发送。", nil)
	}
	token, _, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		c.beginFlow(chatID, pendingCloudflareToken, nil)
		return c.Bot.SendMessage(ctx, chatID, "请先配置 Cloudflare Token。", cloudflareNeedTokenMenu())
	}
	if c.DNS == nil {
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID。", nil)
	}
	zoneID, err := c.DNS.LookupZoneID(ctx, token, zoneName)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.Bot.SendMessage(ctx, chatID, "查询 Zone 失败："+msg+"\n\n请重新发送 Zone Name，或发送 /cancel 取消。", nil)
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	c.clearSession(chatID)
	return c.Bot.SendMessage(ctx, chatID, fmt.Sprintf("✅ Cloudflare Zone 已保存\n\nZone Name：%s\nZone ID：%s\n\n下一步：配置 DNS A 记录", zoneName, maskMiddle(zoneID, 4, 4)), cloudflareSavedMenu())
}

func (c *TelegramController) sendDNSPanel(ctx context.Context, chatID int64, prefix string) error {
	_, zoneName, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	count, err := c.Store.CountCloudflareConfigs(ctx)
	if err != nil {
		return err
	}
	text := prefix + "🌐 DNS 配置\n\n"
	text += "当前 Zone：" + valueOrDash(zoneName) + "\n"
	text += fmt.Sprintf("已配置 DNS A 记录：%d\n", count)
	if count == 0 {
		text += "建议：先为分组添加第一条 DNS A 记录，再继续生成 Agent 安装命令。\n"
	}
	text += "\n请选择操作："
	return c.Bot.SendMessage(ctx, chatID, text, dnsPanelMenu())
}

func (c *TelegramController) sendDNSStatus(ctx context.Context, chatID int64) error {
	items, err := BuildDNSSummaries(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatDNSSummaries(items), dnsPanelMenu())
}

func (c *TelegramController) startDNSWizard(ctx context.Context, chatID int64, prefix string) error {
	token, zoneName, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(zoneName) == "" {
		return c.Bot.SendMessage(ctx, chatID, prefix+"请先完成 Cloudflare Token 和 Zone 配置。", cloudflareSavedMenu())
	}
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return c.Bot.SendMessage(ctx, chatID, prefix+"还没有分组。请先创建分组。", dnsNoGroupMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, prefix+"请选择要绑定 DNS 的分组：", dnsGroupMenu(groups))
}

func (c *TelegramController) startDNSRecordPrompt(ctx context.Context, chatID int64, groupID string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	_, zoneName, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	c.beginFlow(chatID, pendingDNSRecordName, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
	})
	text := "请发送 DNS A 记录名称，例如：\n"
	text += "hk." + zoneName + "\n"
	text += "或只输入子域名前缀：\n"
	text += "hk\n\n"
	text += "当前 Zone：" + valueOrDash(zoneName) + "\n\n发送 /cancel 取消。"
	return c.Bot.SendMessage(ctx, chatID, text, nil)
}

func (c *TelegramController) handleDNSRecordNameInput(ctx context.Context, chatID int64, text string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	if groupID == "" {
		return c.Bot.SendMessage(ctx, chatID, "分组信息已失效，请重新开始 DNS 配置。", dnsPanelMenu())
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "分组不存在，请重新开始 DNS 配置。", dnsPanelMenu())
	}
	token, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	recordName := normalizeDNSRecordName(text, zoneName)
	if recordName == "" {
		c.setSession(chatID, pendingDNSRecordName)
		return c.Bot.SendMessage(ctx, chatID, "❌ 记录名不能为空，请重新发送。", nil)
	}
	if strings.TrimSpace(zoneID) == "" && c.DNS != nil {
		foundZoneID, lookupErr := c.DNS.LookupZoneID(ctx, token, zoneName)
		if lookupErr != nil {
			c.setSession(chatID, pendingDNSRecordName)
			return c.Bot.SendMessage(ctx, chatID, "❌ 查询 Zone ID 失败："+friendlyCloudflareError(lookupErr), nil)
		}
		zoneID = foundZoneID
		if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
			return err
		}
	}
	c.setSessionValue(chatID, sessionKeyRecordName, recordName)
	c.setSessionValue(chatID, sessionKeyZoneID, zoneID)
	if c.DNS == nil || strings.TrimSpace(zoneID) == "" {
		c.setSession(chatID, pendingDNSRecordName)
		return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 DNS 记录。", nil)
	}
	record, err := c.DNS.LookupDNSRecord(ctx, token, zoneID, recordName)
	if err == nil {
		cfg, saveErr := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, defaultDNSRecordTTL, defaultDNSRecordProxied, true)
		if saveErr != nil {
			return saveErr
		}
		nodes, nodesErr := c.Store.ListNodesByGroupID(ctx, group.ID)
		matchedNodeID := ""
		matchedNodeName := ""
		if nodesErr == nil {
			for _, node := range nodes {
				if node.PublicIP == record.Content {
					_ = c.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
					matchedNodeID = node.ID
					matchedNodeName = node.Name
					break
				}
			}
		}
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 配置已保存")
		_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
		_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
		if matchedNodeID != "" {
			c.clearSession(chatID)
			text := formatDNSSavedMessage(group.Name, cfg.RecordName, record.Content, matchedNodeName, false)
			return c.Bot.SendMessage(ctx, chatID, text, dnsSavedMenu(matchedNodeID))
		}
		c.setSessionValue(chatID, sessionKeyRecordName, cfg.RecordName)
		c.setSessionValue(chatID, sessionKeyRecordID, record.ID)
		c.setSessionValue(chatID, sessionKeyCurrentIP, record.Content)
		c.setSessionValue(chatID, sessionKeyZoneID, zoneID)
		c.setSession(chatID, pendingDNSFixSelect)
		text := fmt.Sprintf("当前 DNS %s 解析到 %s，\n但没有匹配任何已配置节点。\n\n请选择：", cfg.RecordName, record.Content)
		return c.Bot.SendMessage(ctx, chatID, text, dnsFixMenu(nodes))
	}
	if any, anyErr := c.DNS.LookupDNSRecordAnyType(ctx, token, zoneID, recordName); anyErr == nil {
		msg := fmt.Sprintf("当前记录类型为 %s，不支持。请改为 A 记录后重试。", any.Type)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型错误")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
		c.setSession(chatID, pendingDNSRecordName)
		return c.Bot.SendMessage(ctx, chatID, msg, nil)
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		c.setSession(chatID, pendingDNSRecordName)
		return c.Bot.SendMessage(ctx, chatID, "DNS 记录不存在，但当前分组没有节点，无法确定初始 IP。\n\n请先添加节点。", dnsNeedNodeMenu())
	}
	c.setSession(chatID, pendingDNSRecordName)
	return c.Bot.SendMessage(ctx, chatID, "记录 "+recordName+" 不存在。\n\n请选择初始解析到哪个节点：", dnsNodeMenu(nodes))
}

func (c *TelegramController) handleDNSCreateRecord(ctx context.Context, chatID int64, nodeID string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	if groupID == "" || recordName == "" {
		return c.Bot.SendMessage(ctx, chatID, "DNS 创建流程已失效，请重新开始。", dnsPanelMenu())
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "分组不存在，请重新开始。", dnsPanelMenu())
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", dnsPanelMenu())
	}
	token, _, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if c.DNS == nil || strings.TrimSpace(zoneID) == "" {
		return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法创建 DNS 记录。", dnsPanelMenu())
	}
	record, err := c.DNS.CreateDNSRecord(ctx, token, zoneID, recordName, node.PublicIP, defaultDNSRecordTTL, defaultDNSRecordProxied)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "❌ DNS 创建失败")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), msg, token)
		c.setSession(chatID, pendingDNSRecordName)
		return c.Bot.SendMessage(ctx, chatID, "创建 DNS 记录失败："+msg, nil)
	}
	cfg, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, defaultDNSRecordTTL, defaultDNSRecordProxied, true)
	if err != nil {
		return err
	}
	_ = c.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 配置已保存")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
	c.clearSession(chatID)
	text := formatDNSSavedMessage(group.Name, cfg.RecordName, node.PublicIP, node.Name, true)
	return c.Bot.SendMessage(ctx, chatID, text, dnsSavedMenu(node.ID))
}

func (c *TelegramController) handleDNSRepointToNode(ctx context.Context, chatID int64, nodeID string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	recordID := c.currentSessionValue(chatID, sessionKeyRecordID)
	currentIP := c.currentSessionValue(chatID, sessionKeyCurrentIP)
	if groupID == "" || recordName == "" || recordID == "" {
		return c.Bot.SendMessage(ctx, chatID, "DNS 修正流程已失效，请重新开始。", dnsPanelMenu())
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", dnsPanelMenu())
	}
	if node.GroupID != groupID {
		return c.Bot.SendMessage(ctx, chatID, "节点不属于当前分组，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		return err
	}
	nodes, _ := c.Store.ListNodesByGroupID(ctx, groupID)
	if c.DNS == nil || strings.TrimSpace(cfg.ZoneID) == "" || strings.TrimSpace(cfg.APIToken) == "" {
		return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法修正 DNS 记录。", dnsPanelMenu())
	}
	if err := c.DNS.UpdateDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, recordID, recordName, node.PublicIP, cfg.TTL, cfg.Proxied); err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(groupID), "❌ DNS 修改失败")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(groupID), msg, cfg.APIToken)
		c.setSession(chatID, pendingDNSFixSelect)
		return c.Bot.SendMessage(ctx, chatID, "修正 DNS 记录失败："+msg, dnsFixMenu(nodes))
	}
	_ = c.Store.UpdateGroupCurrentNode(ctx, groupID, node.ID)
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(groupID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(groupID), "✅ DNS 修改成功")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(groupID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(groupID))
	c.clearSession(chatID)
	text := fmt.Sprintf("✅ DNS A 记录已更新\n\n域名：%s\n旧 IP：%s\n新 IP：%s\n匹配节点：%s\n\n下一步：安装 Agent", recordName, valueOrDash(currentIP), node.PublicIP, node.Name)
	return c.Bot.SendMessage(ctx, chatID, text, dnsSavedMenu(node.ID))
}

func (c *TelegramController) handleDNSKeepCurrent(ctx context.Context, chatID int64) error {
	groupName := c.currentSessionValue(chatID, sessionKeyGroupName)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	currentIP := c.currentSessionValue(chatID, sessionKeyCurrentIP)
	c.clearSession(chatID)
	return c.Bot.SendMessage(ctx, chatID, formatDNSSavedMessage(groupName, recordName, currentIP, "", false), dnsSavedMenu(""))
}

func (c *TelegramController) sendGroupsPanel(ctx context.Context, chatID int64, prefix string) error {
	count, err := c.Store.CountGroups(ctx)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("%s📦 分组管理\n\n当前分组：%d\n\n请选择操作：", prefix, count)
	return c.Bot.SendMessage(ctx, chatID, text, groupsPanelMenu())
}

func (c *TelegramController) sendGroupsStatus(ctx context.Context, chatID int64) error {
	items, err := BuildGroupDiagnostics(ctx, c.Store, timeNow(), c.DNS)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatGroupDiagnostics(items), groupsPanelMenu())
}

func (c *TelegramController) sendGroupNamePrompt(ctx context.Context, chatID int64, prefix string) error {
	text := prefix + "请发送分组名，例如：\nhk\nsg\nus\n\n发送 /cancel 取消。"
	return c.Bot.SendMessage(ctx, chatID, text, nil)
}

func (c *TelegramController) handleGroupNameInput(ctx context.Context, chatID int64, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	if err := ValidateGroupName(groupName); err != nil {
		c.setSession(chatID, pendingGroupName)
		return c.Bot.SendMessage(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送分组名。", nil)
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if _, err := c.Store.CreateGroup(ctx, groupName, policy.DefaultSwitchCooldownSecs); err != nil {
		c.setSession(chatID, pendingGroupName)
		return c.Bot.SendMessage(ctx, chatID, "❌ 创建分组失败："+err.Error()+"\n\n请换一个分组名重试。", nil)
	}
	c.clearSession(chatID)
	return c.Bot.SendMessage(ctx, chatID, "✅ 分组已创建："+groupName+"\n\n下一步：", groupCreatedMenu())
}

func (c *TelegramController) sendNodesPanel(ctx context.Context, chatID int64, prefix string) error {
	nodes, err := c.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("%s🖥 节点管理\n\n当前节点：%d\n\n请选择操作，或直接点开某个节点查看详情：", prefix, len(nodes))
	return c.Bot.SendMessage(ctx, chatID, text, nodesPanelMenu(nodes))
}

func (c *TelegramController) sendNodesStatus(ctx context.Context, chatID int64) error {
	items, err := BuildNodeDiagnostics(ctx, c.Store, timeNow())
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatNodeDiagnostics(items), nodesPanelMenu(nil))
}

func (c *TelegramController) startNodeWizard(ctx context.Context, chatID int64, prefix string) error {
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return c.Bot.SendMessage(ctx, chatID, prefix+"还没有分组，请先创建分组。", nodesNeedGroupMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, prefix+"请选择节点所属分组：", nodesGroupMenu(groups))
}

func (c *TelegramController) startNodeNamePrompt(ctx context.Context, chatID int64, groupID string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "分组不存在，请重新选择。", nodesPanelMenu(nil))
	}
	c.beginFlow(chatID, pendingNodeName, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
		sessionKeyNodeFlow:  "create",
	})
	return c.Bot.SendMessage(ctx, chatID, "请发送节点名称，例如：\nhk-01\n\n发送 /cancel 取消。", nil)
}

func (c *TelegramController) handleNodeNameInput(ctx context.Context, chatID int64, nodeName string) error {
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" {
		c.setSession(chatID, pendingNodeName)
		return c.Bot.SendMessage(ctx, chatID, "❌ 节点名称不能为空，请重新发送。", nil)
	}
	if _, err := c.Store.GetNodeByName(ctx, nodeName); err == nil {
		c.setSession(chatID, pendingNodeName)
		return c.Bot.SendMessage(ctx, chatID, "❌ 节点名称已存在，请换一个名称。", nil)
	}
	c.setSessionValue(chatID, sessionKeyNodeName, nodeName)
	c.setSession(chatID, pendingNodeIP)
	return c.Bot.SendMessage(ctx, chatID, "请发送节点公网 IP。\n\n要求：\n- 仅支持 IPv4\n- 不允许私网 IP、localhost、127.0.0.1\n\n发送 /cancel 取消。", nil)
}

func (c *TelegramController) handleNodeIPInput(ctx context.Context, chatID int64, ipText string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	ipText = strings.TrimSpace(ipText)
	if err := ValidatePublicIPv4(ipText); err != nil {
		c.setSession(chatID, pendingNodeIP)
		return c.Bot.SendMessage(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送公网 IPv4。", nil)
	}
	c.setSessionValue(chatID, sessionKeyNodeIP, ipText)
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "default")
	c.setSessionValue(chatID, sessionKeyNodeQuota, strconv.FormatInt(policy.DefaultMonthlyQuotaBytes, 10))
	c.setSessionValue(chatID, sessionKeyNodeThreshold, strconv.Itoa(policy.DefaultThresholdPercent))
	c.setSessionValue(chatID, sessionKeyNodeTrafficMode, policy.DefaultTrafficMode)
	c.setSessionValue(chatID, sessionKeyNodeResetDay, strconv.Itoa(policy.DefaultResetDay))
	c.setSessionValue(chatID, sessionKeyNodePriority, strconv.Itoa(defaultNodePriority))
	c.setSession(chatID, pendingNodeConfirm)
	return c.Bot.SendMessage(ctx, chatID, c.buildNodeConfirmText(chatID), nodeCreateConfirmMenu())
}

func (c *TelegramController) startNodePolicyPrompt(ctx context.Context, chatID int64) error {
	if c.currentSessionValue(chatID, sessionKeyNodeName) == "" || c.currentSessionValue(chatID, sessionKeyGroupID) == "" {
		return c.Bot.SendMessage(ctx, chatID, "节点配置流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeQuota)
	return c.Bot.SendMessage(ctx, chatID, "请发送月流量，例如：500GB、1TB、1000GB。\n\n可直接点击默认值。", nodeQuotaMenu())
}

func (c *TelegramController) startNodePolicyEditWizard(ctx context.Context, chatID int64, nodeID string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点所属分组不存在，请重新选择。", nodesPanelMenu(nil))
	}
	c.beginFlow(chatID, pendingNodeQuota, map[string]string{
		sessionKeyNodeFlow:         "edit",
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
		sessionKeyNodePolicySource: "custom",
	})
	return c.Bot.SendMessage(ctx, chatID, "请发送新的月流量，例如：500GB、1TB、1000GB。", nodeQuotaMenu())
}

func (c *TelegramController) handleNodeQuotaValue(ctx context.Context, chatID int64, raw string) error {
	bytes, err := parseGB(raw)
	if err != nil || bytes <= 0 {
		c.setSession(chatID, pendingNodeQuota)
		return c.Bot.SendMessage(ctx, chatID, "❌ 流量总额格式错误，请发送类似 500GB、1TB、1000GB 的值。", nodeQuotaMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeQuota, strconv.FormatInt(bytes, 10))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeThreshold)
	return c.Bot.SendMessage(ctx, chatID, "请发送阈值百分比，例如：80 或 80%。\n\n可直接点击默认值。", nodeThresholdMenu())
}

func (c *TelegramController) handleNodeThresholdValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(raw), "%"))
	if err != nil || value < 1 || value > 100 {
		c.setSession(chatID, pendingNodeThreshold)
		return c.Bot.SendMessage(ctx, chatID, "❌ 阈值必须在 1-100 之间，请重新发送。", nodeThresholdMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeThreshold, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeModeSelect)
	return c.Bot.SendMessage(ctx, chatID, "请选择统计模式：", nodeTrafficModeMenu())
}

func (c *TelegramController) handleNodeModeChoice(ctx context.Context, chatID int64, mode string) error {
	mode = normalizeMode(mode)
	c.setSessionValue(chatID, sessionKeyNodeTrafficMode, mode)
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeResetDay)
	return c.Bot.SendMessage(ctx, chatID, "请发送重置日（1-28）。\n\n可直接点击默认值。", nodeResetDayMenu())
}

func (c *TelegramController) handleNodeResetDayValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > 28 {
		c.setSession(chatID, pendingNodeResetDay)
		return c.Bot.SendMessage(ctx, chatID, "❌ 重置日必须在 1-28 之间，请重新发送。", nodeResetDayMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodeResetDay, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodePriority)
	return c.Bot.SendMessage(ctx, chatID, "请发送 priority（默认 10）。\n\n可直接点击默认值。", nodePriorityMenu())
}

func (c *TelegramController) handleNodePriorityValue(ctx context.Context, chatID int64, raw string) error {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		c.setSession(chatID, pendingNodePriority)
		return c.Bot.SendMessage(ctx, chatID, "❌ priority 不能小于 0，请重新发送。", nodePriorityMenu())
	}
	c.setSessionValue(chatID, sessionKeyNodePriority, strconv.Itoa(value))
	c.setSessionValue(chatID, sessionKeyNodePolicySource, "custom")
	c.setSession(chatID, pendingNodeConfirm)
	if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
		return c.Bot.SendMessage(ctx, chatID, c.buildNodeConfirmText(chatID), nodePolicyConfirmMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, c.buildNodeConfirmText(chatID), nodeCreateConfirmMenu())
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
		return c.Bot.SendMessage(ctx, chatID, "节点配置流程已失效，请重新开始。", nodesPanelMenu(nil))
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
	if err := ValidateNodeConfig(node); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.Bot.SendMessage(ctx, chatID, "❌ "+err.Error()+"\n\n请重新填写节点信息。", nodeCreateConfirmMenu())
	}
	created, err := c.Store.CreateNode(ctx, node)
	if err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.Bot.SendMessage(ctx, chatID, "❌ 创建节点失败："+err.Error()+"\n\n请重新填写。", nodeCreateConfirmMenu())
	}
	c.clearSession(chatID)
	return c.sendNodeCreatedSummary(ctx, chatID, created)
}

func (c *TelegramController) handleNodeSavePolicy(ctx context.Context, chatID int64) error {
	nodeID := c.currentSessionValue(chatID, sessionKeyNodeID)
	if nodeID == "" {
		return c.Bot.SendMessage(ctx, chatID, "节点策略修改流程已失效，请重新开始。", nodesPanelMenu(nil))
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
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
	if err := ValidateNodeConfig(node); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.Bot.SendMessage(ctx, chatID, "❌ "+err.Error()+"\n\n请重新填写节点策略。", nodePolicyConfirmMenu())
	}
	if err := c.Store.UpdateNodePolicy(ctx, node); err != nil {
		c.setSession(chatID, pendingNodeConfirm)
		return c.Bot.SendMessage(ctx, chatID, "❌ 保存节点策略失败："+err.Error()+"\n\n请重新填写。", nodePolicyConfirmMenu())
	}
	c.clearSession(chatID)
	return c.sendNodeDetail(ctx, chatID, node.ID, "✅ 节点策略已更新。\n\n")
}

func (c *TelegramController) sendNodeDetail(ctx context.Context, chatID int64, nodeID, prefix string) error {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点所属分组不存在，请重新选择。", nodesPanelMenu(nil))
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
		"🖥 节点详情\n\n节点：%s\n分组：%s\n公网 IP：%s\nAgent：%s\nDNS 匹配：%s\n月流量：%s\n阈值：%d%%\n统计：%s\n重置日：%d\n优先级：%d\n启用：%t\n自动切换：%t\n\n请选择操作：",
		node.Name,
		group.Name,
		node.PublicIP,
		agentStatus,
		ternaryText(dnsMatch, "是", "否"),
		formatNodeQuota(strconv.FormatInt(node.MonthlyQuotaBytes, 10)),
		node.ThresholdPercent,
		modeLabel(node.TrafficMode),
		node.ResetDay,
		node.Priority,
		node.Enabled,
		node.AutoSwitch,
	)
	return c.Bot.SendMessage(ctx, chatID, text, nodeDetailMenu(node, hasReported, online))
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
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
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
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
	}
	node.AutoSwitch = !node.AutoSwitch
	if err := c.Store.SetNodeAutoSwitch(ctx, node.ID, node.AutoSwitch); err != nil {
		return err
	}
	return c.sendNodeDetail(ctx, chatID, node.ID, "✅ 节点自动切换状态已更新。\n\n")
}

func (c *TelegramController) sendAgentTroubleshooting(ctx context.Context, chatID int64, nodeID string) error {
	if nodeID != "" {
		if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
			return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", nodesPanelMenu(nil))
		}
	}
	return c.Bot.SendMessage(ctx, chatID, "Agent 安装排查：\n\n请在 Agent 机器执行：\ndf -h\nqdr-agent version\nqdr-agent status\nqdr-agent config-check\nsystemctl status quota-dns-router-agent --no-pager -l\njournalctl -u quota-dns-router-agent -n 100 --no-pager\n\n如果 df -h 显示 / 已 100%，请先清理磁盘：\napt clean\njournalctl --vacuum-size=100M\ndocker system prune -af", nil)
}

func (c *TelegramController) sendPolicyPanel(ctx context.Context, chatID int64, prefix string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	text := prefix + "⚙️ 默认流量策略\n\n"
	text += "默认月流量：" + formatNodeQuota(strconv.FormatInt(policy.DefaultMonthlyQuotaBytes, 10)) + "\n"
	text += fmt.Sprintf("默认阈值：%d%%\n", policy.DefaultThresholdPercent)
	text += "默认统计模式：" + modeLabel(policy.DefaultTrafficMode) + "\n"
	text += fmt.Sprintf("默认重置日：%d\n", policy.DefaultResetDay)
	text += fmt.Sprintf("默认优先级：%d\n", defaultNodePriority)
	text += "自动切换：" + ternaryText(policy.AutoSwitchEnabled, "启用", "关闭") + "\n\n"
	text += "这些默认值会用于新建节点。已有节点可以在节点详情中单独修改。"
	return c.Bot.SendMessage(ctx, chatID, text, policyPanelMenu())
}

func (c *TelegramController) sendPolicyModeMenu(ctx context.Context, chatID int64) error {
	return c.Bot.SendMessage(ctx, chatID, "请选择默认统计模式：", policyModeMenu())
}

func (c *TelegramController) sendPolicyValuePrompt(ctx context.Context, chatID int64, prefix, field string) error {
	switch field {
	case policyFieldQuota:
		return c.Bot.SendMessage(ctx, chatID, prefix+"请发送默认月流量，例如：500GB、1TB、1000GB。", nil)
	case policyFieldResetDay:
		return c.Bot.SendMessage(ctx, chatID, prefix+"请发送默认重置日（1-28）。", nil)
	default:
		return c.Bot.SendMessage(ctx, chatID, prefix+"请发送默认阈值百分比，例如：80 或 80%。", nil)
	}
}

func (c *TelegramController) handlePolicyValueInput(ctx context.Context, chatID int64, text string) error {
	field := c.currentSessionValue(chatID, sessionKeyPolicyField)
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	switch field {
	case policyFieldQuota:
		bytes, parseErr := parseGB(text)
		if parseErr != nil || bytes <= 0 {
			c.setSession(chatID, pendingPolicyValue)
			return c.Bot.SendMessage(ctx, chatID, "❌ 默认月流量格式错误，请重新发送。", nil)
		}
		policy.DefaultMonthlyQuotaBytes = bytes
	case policyFieldThreshold:
		value, parseErr := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(text), "%"))
		if parseErr != nil || value < 1 || value > 100 {
			c.setSession(chatID, pendingPolicyValue)
			return c.Bot.SendMessage(ctx, chatID, "❌ 默认阈值必须在 1-100 之间，请重新发送。", nil)
		}
		policy.DefaultThresholdPercent = value
	case policyFieldResetDay:
		value, parseErr := strconv.Atoi(strings.TrimSpace(text))
		if parseErr != nil || value < 1 || value > 28 {
			c.setSession(chatID, pendingPolicyValue)
			return c.Bot.SendMessage(ctx, chatID, "❌ 默认重置日必须在 1-28 之间，请重新发送。", nil)
		}
		policy.DefaultResetDay = value
	default:
		c.clearSession(chatID)
		return c.Bot.SendMessage(ctx, chatID, "策略修改流程已失效，请重新选择。", policyPanelMenu())
	}
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	c.clearSession(chatID)
	return c.Bot.SendMessage(ctx, chatID, "✅ 策略已更新。", policySavedMenu())
}

func (c *TelegramController) handlePolicyModeChoice(ctx context.Context, chatID int64, mode string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	policy.DefaultTrafficMode = normalizeMode(mode)
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, "✅ 默认统计模式已更新为："+modeLabel(policy.DefaultTrafficMode), policySavedMenu())
}

func (c *TelegramController) togglePolicyAutoSwitch(ctx context.Context, chatID int64) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	policy.AutoSwitchEnabled = !policy.AutoSwitchEnabled
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, "✅ 自动切换已"+ternaryText(policy.AutoSwitchEnabled, "启用", "关闭"), policySavedMenu())
}

func (c *TelegramController) sendAgentPanel(ctx context.Context, chatID int64, prefix string) error {
	nodes, err := c.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return c.Bot.SendMessage(ctx, chatID, prefix+"🤖 Agent 安装\n\n还没有节点，请先添加节点。", agentNeedNodeMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, prefix+"🤖 Agent 安装\n\n请选择要安装 Agent 的节点：", agentNodeMenu(nodes))
}

func (c *TelegramController) sendAgentInstallCommand(ctx context.Context, chatID int64, nodeID string) error {
	if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
		return c.Bot.SendMessage(ctx, chatID, "节点不存在，请重新选择。", agentPanelMenu())
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	preview, err := c.buildAgentInstallPreview(ctx, nodeID, policy)
	if err != nil {
		return err
	}
	if len(preview.Missing) > 0 {
		return c.Bot.SendMessage(ctx, chatID, "生成 Agent 安装命令前还缺少："+strings.Join(preview.Missing, "、"), setupMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, formatAgentInstallMessage(preview), agentCommandMenu(preview.DNSReady))
}

func (c *TelegramController) tryDeleteMessage(ctx context.Context, chatID, messageID int64) {
	_ = c.Bot.DeleteMessage(ctx, chatID, messageID)
}

func cloudflarePanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置/更新 Token", CallbackData: "cf_token"}},
		{{Text: "选择 Zone", CallbackData: "cf_select_zone"}},
		{{Text: "查看当前配置", CallbackData: "cf_view"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareNeedTokenMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置/更新 Token", CallbackData: "cf_token"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareZoneMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "手动输入 Zone Name", CallbackData: "cf_zone_manual"}},
		{{Text: "重新输入 Token", CallbackData: "cf_token_reset"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareZoneChoicesMenu(zones []cloudflare.Zone) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(zones)+2)
	for i, zone := range zones {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: zone.Name, CallbackData: fmt.Sprintf("cf_zone_pick:%d", i)}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "手动输入 Zone Name", CallbackData: "cf_zone_manual"}},
		[]telegram.InlineKeyboardButton{{Text: "重新输入 Token", CallbackData: "cf_token_reset"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func cloudflareSavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置 DNS", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加 DNS A 记录", CallbackData: "dns_add"}},
		{{Text: "查看 DNS 状态", CallbackData: "dns_status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsNoGroupMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "创建分组", CallbackData: "groups_new"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsNeedNodeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsSavedMenu(nodeID string) *telegram.ReplyMarkup {
	callback := "agent"
	if strings.TrimSpace(nodeID) != "" {
		callback = "agent_node:" + nodeID
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "Agent 安装", CallbackData: callback}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsFixMenu(nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+2)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         "改为指向节点 " + node.Name + " / " + node.PublicIP,
			CallbackData: "dns_repoint:" + node.ID,
		}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "保留当前 DNS", CallbackData: "dns_keep_current"}},
		[]telegram.InlineKeyboardButton{{Text: "返回", CallbackData: "dns"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsGroupMenu(groups []db.Group) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(groups)+1)
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "dns_group:" + group.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "新建分组", CallbackData: "groups_new"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsNodeMenu(nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+1)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name + " / " + node.PublicIP, CallbackData: "dns_create:" + node.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "取消", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func groupsPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "新建分组", CallbackData: "groups_new"}},
		{{Text: "查看分组状态", CallbackData: "groups_status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func groupCreatedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "配置 DNS", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodesPanelMenu(nodes []db.NodeWithGroup) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "查看节点状态", CallbackData: "nodes_status"}},
	}
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         node.Name + " / " + node.GroupName,
			CallbackData: "nodes_view:" + node.ID,
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodesNeedGroupMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "创建分组", CallbackData: "groups_new"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodesGroupMenu(groups []db.Group) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "nodes_group:" + group.ID}})
	}
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodeQuotaMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1000GB", CallbackData: "nodes_quota_default"}},
	}}
}

func nodeThresholdMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "80%", CallbackData: "nodes_threshold_default"}},
	}}
}

func nodeTrafficModeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "RX 下行", CallbackData: "nodes_mode:rx"}},
		{{Text: "TX 上行", CallbackData: "nodes_mode:tx"}},
		{{Text: "RX+TX 双向", CallbackData: "nodes_mode:both"}},
	}}
}

func nodeResetDayMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1", CallbackData: "nodes_reset_day_default"}},
	}}
}

func nodePriorityMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "10", CallbackData: "nodes_priority_default"}},
	}}
}

func nodeCreateConfirmMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "确认创建", CallbackData: "nodes_confirm"}},
		{{Text: "修改流量策略", CallbackData: "nodes_customize_policy"}},
		{{Text: "重新填写", CallbackData: "nodes_restart"}},
		{{Text: "取消", CallbackData: "menu"}},
	}}
}

func nodePolicyConfirmMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "保存策略", CallbackData: "nodes_save_policy"}},
		{{Text: "取消", CallbackData: "menu"}},
	}}
}

func nodeCreatedMenu(nodeID string, hasDNS bool) *telegram.ReplyMarkup {
	if !hasDNS {
		return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "配置 DNS", CallbackData: "dns"}},
			{{Text: "继续生成 Agent 命令", CallbackData: "agent_node:" + nodeID}},
			{{Text: "当前状态", CallbackData: "status"}},
			{{Text: "返回主菜单", CallbackData: "menu"}},
		}}
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "生成 Agent 安装命令", CallbackData: "agent_node:" + nodeID}},
		{{Text: "查看节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func policyPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "修改默认月流量", CallbackData: "policy_quota"}},
		{{Text: "修改默认阈值", CallbackData: "policy_threshold"}},
		{{Text: "修改统计模式", CallbackData: "policy_mode"}},
		{{Text: "修改默认重置日", CallbackData: "policy_reset_day"}},
		{{Text: "开启/关闭自动切换", CallbackData: "policy_toggle_auto"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func policyModeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "RX 下行", CallbackData: "policy_mode:rx"}},
		{{Text: "TX 上行", CallbackData: "policy_mode:tx"}},
		{{Text: "RX+TX 双向", CallbackData: "policy_mode:both"}},
	}}
}

func policySavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "流量策略", CallbackData: "policy"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentNeedNodeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentNodeMenu(nodes []db.NodeWithGroup) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+1)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name, CallbackData: "agent_node:" + node.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func agentCommandMenu(hasDNS bool) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, 3)
	if !hasDNS {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "配置 DNS", CallbackData: "dns"}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "当前状态", CallbackData: "status"}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodeDetailMenu(node db.Node, hasReported, online bool) *telegram.ReplyMarkup {
	installText := "生成 Agent 安装命令"
	if !hasReported || !online {
		installText = "重新生成 Agent 安装命令"
	}
	enabledText := "禁用节点"
	if !node.Enabled {
		enabledText = "启用节点"
	}
	autoText := "关闭自动切换"
	if !node.AutoSwitch {
		autoText = "开启自动切换"
	}
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: installText, CallbackData: "agent_node:" + node.ID}},
		{{Text: "修改节点策略", CallbackData: "nodes_edit_policy:" + node.ID}},
		{{Text: enabledText, CallbackData: "nodes_toggle_enabled:" + node.ID}},
		{{Text: autoText, CallbackData: "nodes_toggle_auto:" + node.ID}},
	}
	if !hasReported || !online {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "查看安装排查", CallbackData: "agent_troubleshoot:" + node.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回节点列表", CallbackData: "nodes"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func masterSavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置 Cloudflare", CallbackData: "cf"}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseCallbackInt(value, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(value, prefix))
}

func normalizeDNSRecordName(raw, zoneName string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, ".") && strings.TrimSpace(zoneName) != "" {
		return raw + "." + zoneName
	}
	return raw
}

func formatNodeQuota(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	const (
		gb = 1024 * 1024 * 1024
		tb = 1024 * gb
	)
	if value%tb == 0 {
		return fmt.Sprintf("%dTB", value/tb)
	}
	if value%gb == 0 {
		return fmt.Sprintf("%dGB", value/gb)
	}
	return raw
}

func maskMiddle(value string, prefixLen, suffixLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= prefixLen+suffixLen {
		return config.MaskSecret(value)
	}
	return value[:prefixLen] + "****" + value[len(value)-suffixLen:]
}

func ValidatePublicIPv4(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("公网 IPv4 不能为空")
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("公网 IPv4 无效")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("请填写可公网访问的 IPv4，不能使用私网地址、localhost 或 127.0.0.1")
	}
	return nil
}

func timeNow() time.Time {
	return time.Now()
}
