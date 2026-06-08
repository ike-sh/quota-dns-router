package master

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

func (c *TelegramController) startGroupRenamePrompt(ctx context.Context, chatID int64, groupID string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", groupsPanelMenu())
	}
	prefix := c.beginFlow(chatID, pendingGroupName, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
	})
	text := prefix + "请发送新的分组名。\n\n"
	text += "当前分组：" + group.Name + "\n\n"
	text += "发送 /cancel 取消。"
	return c.sendPromptAndTrack(ctx, chatID, pendingGroupName, text, nil)
}

func (c *TelegramController) sendGroupDetail(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", groupsPanelMenu())
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	diagnostics, err := BuildGroupDiagnostics(ctx, c.Store, timeNow(), c.DNS)
	if err != nil {
		return err
	}
	detail := GroupDiagnostic{
		Name:                 group.Name,
		CurrentNode:          "-",
		CurrentIP:            "-",
		DNSRecord:            "",
		NodeCount:            len(nodes),
		AvailableTargetCount: 0,
		Status:               "⏳ 待配置",
	}
	for _, item := range diagnostics {
		if item.Name == group.Name {
			detail = item
			break
		}
	}
	if strings.TrimSpace(detail.DNSRecord) == "" {
		if cfg, cfgErr := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID); cfgErr == nil {
			detail.DNSRecord = cfg.RecordName
		}
	}
	text := prefix + fmt.Sprintf(
		"📦 分组详情\n\n分组：%s\n绑定 DNS：%s\n节点数量：%d\n当前指向节点：%s\n可用切换目标：%d\n\n请选择操作：",
		group.Name,
		valueOrDash(detail.DNSRecord),
		len(nodes),
		valueOrDash(detail.CurrentNode),
		detail.AvailableTargetCount,
	)
	return c.sendMessageOrEdit(ctx, chatID, text, groupDetailMenu(group.ID))
}

func (c *TelegramController) sendGroupNodes(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", groupsPanelMenu())
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	text := prefix + "🖥 分组节点\n\n"
	text += "分组：" + group.Name + "\n"
	text += fmt.Sprintf("节点数量：%d\n\n", len(nodes))
	if len(nodes) == 0 {
		text += "当前分组还没有节点。"
	} else {
		text += "请选择要查看的节点："
	}
	return c.sendMessageOrEdit(ctx, chatID, text, groupNodesMenu(group.ID, nodes))
}

func (c *TelegramController) sendDNSDetail(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.startDNSRecordPromptWithPrefix(ctx, chatID, group, prefix+"当前分组还没有 DNS 记录，请先配置。\n\n")
		}
		return err
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return c.startDNSRecordPromptWithPrefix(ctx, chatID, group, prefix+"当前分组还没有 DNS 记录，请先配置。\n\n")
	}
	summary, err := c.findDNSSummaryByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	currentIP := summary.CurrentIP
	matchedNode := summary.MatchedNodeName
	if matchedNode == "" && group.CurrentNodeID.Valid {
		if node, nodeErr := c.Store.GetNodeByID(ctx, group.CurrentNodeID.String); nodeErr == nil {
			matchedNode = node.Name
			if strings.TrimSpace(currentIP) == "" {
				currentIP = node.PublicIP
			}
		}
	}
	recordType := dnsRecordType(cfg, "")
	text := prefix + "🌐 DNS 详情\n\n"
	text += "分组：" + group.Name + "\n"
	text += "域名：" + valueOrDash(cfg.RecordName) + "\n"
	text += "记录类型：" + recordType + "\n"
	text += "Record ID：" + valueOrDash(cfg.RecordID) + "\n"
	text += formatDNSCurrentRecordLine(recordType, currentIP) + "\n"
	text += "匹配节点：" + valueOrDash(matchedNode) + "\n"
	text += fmt.Sprintf("proxied：%t\n", cfg.Proxied)
	text += "TTL：" + formatDNSTTL(cfg.TTL)
	if summary.Pending {
		text += "\n状态：待绑定节点"
	}
	if cfg.Proxied {
		text += "\n提示：proxied=true 时，Cloudflare 可能会自动处理 TTL。"
	}
	text += "\n\n请选择操作："
	return c.sendMessageOrEdit(ctx, chatID, text, dnsDetailMenu(group.ID))
}

func (c *TelegramController) sendDNSTTLMenu(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.startDNSRecordPromptWithPrefix(ctx, chatID, group, prefix+"当前分组还没有 DNS 记录，请先配置。\n\n")
		}
		return err
	}
	text := prefix + fmt.Sprintf(
		"请选择新的 TTL。\n\n域名：%s\n当前 TTL：%s",
		valueOrDash(cfg.RecordName),
		formatDNSTTL(cfg.TTL),
	)
	if cfg.Proxied {
		text += "\n提示：proxied=true 时，Cloudflare 可能会自动处理 TTL。"
	}
	return c.sendMessageOrEdit(ctx, chatID, text, dnsTTLMenu(group.ID))
}

func (c *TelegramController) handleDNSTTLInput(ctx context.Context, chatID int64, raw string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	if groupID == "" {
		return c.sendMessageOrEdit(ctx, chatID, "TTL 修改流程已失效，请重新选择。", dnsPanelMenu())
	}
	value := strings.TrimSpace(strings.ToLower(raw))
	ttl := 0
	switch value {
	case "auto":
		ttl = 1
	default:
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			c.setSession(chatID, pendingDNSTTL)
			return c.sendMessageOrEdit(ctx, chatID, "❌ TTL 无效，请发送 60、120、300、1 或 auto。", dnsTTLMenu(groupID))
		}
		ttl = n
	}
	return c.updateDNSTTL(ctx, chatID, groupID, ttl)
}

func (c *TelegramController) updateDNSTTL(ctx context.Context, chatID int64, groupID string, ttl int) error {
	group, cfg, err := c.saveDNSOptions(ctx, groupID, ttl, nil)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "TTL 更新失败："+friendlyCloudflareError(err), dnsTTLMenu(groupID))
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	prefix := fmt.Sprintf("✅ TTL 已更新\n\n域名：%s\nTTL：%s", valueOrDash(cfg.RecordName), formatDNSTTL(cfg.TTL))
	if strings.TrimSpace(cfg.RecordID) == "" {
		prefix += "\n状态：当前记录仍待绑定节点，绑定后生效。"
	}
	return c.sendDNSDetail(ctx, chatID, group.ID, prefix+"\n\n")
}

func (c *TelegramController) sendDNSProxiedMenu(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.startDNSRecordPromptWithPrefix(ctx, chatID, group, prefix+"当前分组还没有 DNS 记录，请先配置。\n\n")
		}
		return err
	}
	text := prefix + fmt.Sprintf(
		"请选择 proxied 设置。\n\n域名：%s\n当前 proxied：%t",
		valueOrDash(cfg.RecordName),
		cfg.Proxied,
	)
	return c.sendMessageOrEdit(ctx, chatID, text, dnsProxiedMenu(group.ID, cfg.Proxied))
}

func (c *TelegramController) updateDNSProxied(ctx context.Context, chatID int64, groupID string, proxied bool) error {
	group, cfg, err := c.saveDNSOptions(ctx, groupID, 0, &proxied)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "proxied 更新失败："+friendlyCloudflareError(err), dnsProxiedMenu(groupID, proxied))
	}
	prefix := fmt.Sprintf("✅ proxied 已更新\n\n域名：%s\nproxied：%t", valueOrDash(cfg.RecordName), cfg.Proxied)
	if strings.TrimSpace(cfg.RecordID) == "" {
		prefix += "\n状态：当前记录仍待绑定节点，绑定后生效。"
	}
	return c.sendDNSDetail(ctx, chatID, group.ID, prefix+"\n\n")
}

func (c *TelegramController) sendDNSRepointMenu(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil || strings.TrimSpace(cfg.RecordName) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "当前分组还没有 DNS 记录，请先配置。", dnsPanelMenu())
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return c.sendMessageOrEdit(ctx, chatID, "当前分组还没有节点，请先添加节点。", dnsNeedNodeMenu())
	}
	text := prefix + fmt.Sprintf("请选择要让 %s 指向的节点：", cfg.RecordName)
	return c.sendMessageOrEdit(ctx, chatID, text, dnsRepointMenu(group.ID, nodes))
}

func (c *TelegramController) sendDNSRepointConfirm(ctx context.Context, chatID int64, groupID, nodeID, prefix string) error {
	decision, err := c.buildManualSwitchDecision(ctx, groupID, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, err.Error(), dnsDetailMenu(groupID))
	}
	text := prefix + fmt.Sprintf(
		"请确认改为指向该节点：\n\n域名：%s\n当前节点：%s\n当前 IP：%s\n目标节点：%s\n目标 IP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Current.Name),
		valueOrDash(decision.Current.PublicIP),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
	return c.sendMessageOrEdit(ctx, chatID, text, dnsRepointConfirmMenu(groupID, nodeID))
}

func (c *TelegramController) handleDNSRepointSwitch(ctx context.Context, chatID int64, groupID, nodeID string) error {
	decision, err := c.buildManualSwitchDecision(ctx, groupID, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, err.Error(), dnsDetailMenu(groupID))
	}
	if decision.Current.ID != "" && decision.Current.ID == decision.Target.ID {
		return c.sendDNSDetail(ctx, chatID, groupID, "当前 DNS 已经指向这个节点。\n\n")
	}
	service := Service{Store: c.Store, DNS: c.DNS, Now: timeNow}
	if err := service.ExecuteSwitch(ctx, decision); err != nil {
		return c.sendDNSDetail(ctx, chatID, groupID, "❌ 改为指向节点失败："+friendlyCloudflareError(err)+"\n\n")
	}
	return c.sendDNSDetail(ctx, chatID, groupID, "✅ DNS 已改为指向目标节点。\n\n")
}

func (c *TelegramController) saveDNSOptions(ctx context.Context, groupID string, ttl int, proxied *bool) (db.Group, db.CloudflareConfig, error) {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return db.Group{}, db.CloudflareConfig{}, err
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		return db.Group{}, db.CloudflareConfig{}, err
	}
	nextTTL := normalizeDNSTTLValue(cfg.TTL)
	if ttl > 0 {
		nextTTL = normalizeDNSTTLValue(ttl)
	}
	nextProxied := cfg.Proxied
	if proxied != nil {
		nextProxied = *proxied
	}
	if strings.TrimSpace(cfg.RecordID) != "" && c.DNS != nil && strings.TrimSpace(cfg.ZoneID) != "" && strings.TrimSpace(cfg.APIToken) != "" {
		currentIP, ipErr := c.lookupDNSCurrentIP(ctx, group, cfg)
		if ipErr != nil {
			return db.Group{}, db.CloudflareConfig{}, ipErr
		}
		if strings.TrimSpace(currentIP) == "" {
			return db.Group{}, db.CloudflareConfig{}, fmt.Errorf("未能确定当前 DNS 指向 IP，请稍后重试")
		}
		if err := c.DNS.UpdateDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordID, cfg.RecordName, currentIP, nextTTL, nextProxied); err != nil {
			_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "❌ DNS 修改失败")
			_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), friendlyCloudflareError(err), cfg.APIToken)
			return db.Group{}, db.CloudflareConfig{}, err
		}
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 修改成功")
		_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
	}
	updated, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, cfg.RecordName, cfg.RecordID, dnsRecordType(cfg, ""), nextTTL, nextProxied, cfg.AllowOverride)
	if err != nil {
		return db.Group{}, db.CloudflareConfig{}, err
	}
	return group, updated, nil
}

func (c *TelegramController) lookupDNSCurrentIP(ctx context.Context, group db.Group, cfg db.CloudflareConfig) (string, error) {
	if c.DNS != nil && strings.TrimSpace(cfg.ZoneID) != "" && strings.TrimSpace(cfg.APIToken) != "" && strings.TrimSpace(cfg.RecordName) != "" {
		record, err := lookupGroupDNSRecord(ctx, c.DNS, cfg)
		if err == nil {
			return record.Content, nil
		}
	}
	if group.CurrentNodeID.Valid {
		node, err := c.Store.GetNodeByID(ctx, group.CurrentNodeID.String)
		if err == nil {
			return node.PublicIP, nil
		}
	}
	return "", nil
}

func (c *TelegramController) findDNSSummaryByGroupID(ctx context.Context, groupID string) (DNSSummary, error) {
	items, err := BuildDNSSummaries(ctx, c.Store, c.DNS)
	if err != nil {
		return DNSSummary{}, err
	}
	for _, item := range items {
		if item.GroupID == groupID {
			return item, nil
		}
	}
	return DNSSummary{GroupID: groupID}, nil
}

func normalizeDNSTTLValue(ttl int) int {
	switch {
	case ttl == 1:
		return 1
	case ttl <= 0:
		return defaultDNSRecordTTL
	default:
		return ttl
	}
}

func formatDNSTTL(ttl int) string {
	ttl = normalizeDNSTTLValue(ttl)
	if ttl == 1 {
		return "自动"
	}
	return strconv.Itoa(ttl)
}

func (c *TelegramController) nodeTrafficModeMenu(chatID int64) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "RX 下行", CallbackData: "nodes_mode:rx"}},
		{{Text: "TX 上行", CallbackData: "nodes_mode:tx"}},
		{{Text: "RX+TX 双向", CallbackData: "nodes_mode:both"}},
	}
	if nodeID := c.currentSessionValue(chatID, sessionKeyNodeID); strings.TrimSpace(nodeID) != "" {
		rows = append(rows,
			[]telegram.InlineKeyboardButton{{Text: "返回节点策略", CallbackData: "nodes_edit_policy:" + nodeID}},
			[]telegram.InlineKeyboardButton{{Text: "返回节点详情", CallbackData: "nodes_view:" + nodeID}},
		)
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func groupDetailMenu(groupID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "修改分组名称", CallbackData: "groups_rename:" + groupID}},
		{{Text: "配置 DNS", CallbackData: "dns_view:" + groupID}},
		{{Text: "查看节点", CallbackData: "groups_nodes:" + groupID}},
		{{Text: "返回分组列表", CallbackData: "groups"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func groupNodesMenu(groupID string, nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+3)
	if len(nodes) == 0 {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "添加节点", CallbackData: "nodes_add"}})
	}
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name + " / " + node.PublicIP, CallbackData: "nodes_view:" + node.ID}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "返回分组详情", CallbackData: "groups_view:" + groupID}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsDetailMenu(groupID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "修改域名", CallbackData: "dns_edit_name:" + groupID}},
		{{Text: "修改 TTL", CallbackData: "dns_edit_ttl:" + groupID}},
		{{Text: "修改 proxied", CallbackData: "dns_edit_proxied:" + groupID}},
		{{Text: "改为指向某个节点", CallbackData: "dns_repoint_menu:" + groupID}},
		{{Text: "手动切换", CallbackData: "switch_group:" + groupID}},
		{{Text: "返回 DNS 列表", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsTTLMenu(groupID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "自动 TTL", CallbackData: "dns_ttl_set:" + groupID + ":1"}},
		{{Text: "60 秒", CallbackData: "dns_ttl_set:" + groupID + ":60"}},
		{{Text: "120 秒", CallbackData: "dns_ttl_set:" + groupID + ":120"}},
		{{Text: "300 秒", CallbackData: "dns_ttl_set:" + groupID + ":300"}},
		{{Text: "自定义", CallbackData: "dns_ttl_custom:" + groupID}},
		{{Text: "返回 DNS 详情", CallbackData: "dns_view:" + groupID}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsProxiedMenu(groupID string, current bool) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "proxied=false", CallbackData: "dns_proxied:" + groupID + ":false"}},
		{{Text: "proxied=true", CallbackData: "dns_proxied:" + groupID + ":true"}},
		{{Text: "返回 DNS 详情", CallbackData: "dns_view:" + groupID}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}
	if current {
		rows[1][0].Text = "proxied=true（当前）"
	} else {
		rows[0][0].Text = "proxied=false（当前）"
	}
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsRepointMenu(groupID string, nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+2)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         node.Name + " / " + node.PublicIP,
			CallbackData: "dns_repoint_pick:" + groupID + ":" + node.ID,
		}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "返回 DNS 详情", CallbackData: "dns_view:" + groupID}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsRepointConfirmMenu(groupID, nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "确认修改", CallbackData: "dns_repoint_do:" + groupID + ":" + nodeID}},
		{{Text: "返回节点列表", CallbackData: "dns_repoint_menu:" + groupID}},
		{{Text: "返回 DNS 详情", CallbackData: "dns_view:" + groupID}},
	}}
}
