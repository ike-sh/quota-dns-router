package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/db"
)

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
	} else {
		text += "点击下面的 DNS 记录可查看详情并修改域名、TTL、proxied 或指向节点。\n"
	}
	text += "\n请选择操作："
	items, err := BuildDNSSummaries(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, text, dnsPanelMenu(items...))
}

func (c *TelegramController) sendDNSStatus(ctx context.Context, chatID int64) error {
	items, err := BuildDNSSummaries(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, FormatDNSSummaries(items), dnsPanelMenu(items...))
}

func (c *TelegramController) startDNSWizard(ctx context.Context, chatID int64, prefix string) error {
	token, zoneName, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(zoneName) == "" {
		return c.sendMessageOrEdit(ctx, chatID, prefix+"请先完成 Cloudflare Token 和 Zone 配置。", cloudflareSavedMenu())
	}
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		group, groupErr := c.ensureDefaultGroup(ctx)
		if groupErr != nil {
			return groupErr
		}
		return c.startDNSRecordPromptWithPrefix(ctx, chatID, group, prefix+"已自动创建默认分组 default。\n\n")
	}
	return c.sendMessageOrEdit(ctx, chatID, prefix+"请选择要绑定 DNS 的分组：", dnsGroupMenu(groups))
}

func (c *TelegramController) startDNSRecordPrompt(ctx context.Context, chatID int64, groupID string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", dnsPanelMenu())
	}
	c.beginFlow(chatID, pendingDNSRecordName, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
	})
	if cfg, cfgErr := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID); cfgErr == nil {
		c.setSessionValue(chatID, sessionKeyRecordType, dnsRecordType(cfg, ""))
	}
	return c.startDNSRecordNamePromptAfterType(ctx, chatID, "")
}

func (c *TelegramController) startDNSRecordPromptWithPrefix(ctx context.Context, chatID int64, group db.Group, prefix string) error {
	c.beginFlow(chatID, pendingDNSTypeSelect, map[string]string{
		sessionKeyGroupID:   group.ID,
		sessionKeyGroupName: group.Name,
	})
	text := prefix + "请选择要绑定的 DNS 记录类型：\n\n"
	text += "当前分组：" + group.Name + "\n"
	text += "A 记录用于 IPv4，AAAA 记录用于 IPv6。"
	return c.sendMessageOrEdit(ctx, chatID, text, dnsRecordTypeMenu())
}

func (c *TelegramController) startDNSRecordNamePromptAfterType(ctx context.Context, chatID int64, prefix string) error {
	_, zoneName, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	groupName := c.currentSessionValue(chatID, sessionKeyGroupName)
	recordType := c.currentSessionValue(chatID, sessionKeyRecordType)
	if recordType == "" {
		recordType = "A"
	}
	c.setSession(chatID, pendingDNSRecordName)
	text := fmt.Sprintf("请发送 DNS %s 记录名称，例如：\n", recordType)
	text += "hk.example.com\n\n"
	text += "也可以只输入子域名前缀，例如：\n"
	text += "hk\n\n"
	text += "当前分组：" + valueOrDash(groupName) + "\n"
	text += "记录类型：" + recordType + "\n"
	text += "当前 Zone：" + valueOrDash(zoneName) + "\n\n发送 /cancel 取消。"
	return c.sendPromptAndTrack(ctx, chatID, pendingDNSRecordName, prefix+text, nil)
}

func (c *TelegramController) handleDNSRecordTypeChoice(ctx context.Context, chatID int64, recordType string) error {
	if recordType != "A" && recordType != "AAAA" {
		return c.sendMessageOrEdit(ctx, chatID, "不支持的记录类型。", dnsRecordTypeMenu())
	}
	c.setSessionValue(chatID, sessionKeyRecordType, recordType)
	return c.startDNSRecordNamePromptAfterType(ctx, chatID, "")
}

func (c *TelegramController) handleDNSRecordNameInput(ctx context.Context, chatID int64, text string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	if groupID == "" {
		return c.sendMessageOrEdit(ctx, chatID, "分组信息已失效，请重新开始 DNS 配置。", dnsPanelMenu())
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新开始 DNS 配置。", dnsPanelMenu())
	}
	token, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	ttl := defaultDNSRecordTTL
	proxied := defaultDNSRecordProxied
	if existing, cfgErr := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID); cfgErr == nil {
		ttl = normalizeDNSTTLValue(existing.TTL)
		proxied = existing.Proxied
	}
	recordName := normalizeDNSRecordName(text, zoneName)
	if recordName == "" {
		c.setSession(chatID, pendingDNSRecordName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 记录名不能为空，请重新发送。", nil)
	}
	if strings.TrimSpace(zoneID) == "" && c.DNS != nil {
		foundZoneID, lookupErr := c.DNS.LookupZoneID(ctx, token, zoneName)
		if lookupErr != nil {
			c.setSession(chatID, pendingDNSRecordName)
			return c.sendMessageOrEdit(ctx, chatID, "❌ 查询 Zone ID 失败："+friendlyCloudflareError(lookupErr), nil)
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
		return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 DNS 记录。", nil)
	}
	recordType := c.currentSessionValue(chatID, sessionKeyRecordType)
	if recordType == "" {
		recordType = "A"
	}
	record, err := c.DNS.LookupDNSRecordWithType(ctx, token, zoneID, recordName, recordType)
	if err == nil {
		cfg, saveErr := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, record.Type, normalizeDNSTTLValue(record.TTL), record.Proxied, true)
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
			c.completePrompt(ctx, chatID)
			c.clearSession(chatID)
			text := formatDNSSavedMessage(group.Name, cfg.RecordName, record.Content, matchedNodeName, false)
			return c.sendMessageOrEdit(ctx, chatID, text, dnsSavedMenu(matchedNodeID))
		}
		c.setSessionValue(chatID, sessionKeyRecordName, cfg.RecordName)
		c.setSessionValue(chatID, sessionKeyRecordID, record.ID)
		c.setSessionValue(chatID, sessionKeyCurrentIP, record.Content)
		c.setSessionValue(chatID, sessionKeyZoneID, zoneID)
		c.setSession(chatID, pendingDNSFixSelect)
		c.completePrompt(ctx, chatID)
		text := fmt.Sprintf("当前 DNS %s 解析到 %s，\n但没有匹配任何已配置节点。\n\n请选择：", cfg.RecordName, record.Content)
		return c.sendMessageOrEdit(ctx, chatID, text, dnsFixMenu(nodes))
	}
	if any, anyErr := c.DNS.LookupDNSRecordAnyType(ctx, token, zoneID, recordName); anyErr == nil && any.Type != recordType {
		msg := fmt.Sprintf("已存在 %s 记录，但你选择的是 %s。请重新选择记录类型或更换记录名。", any.Type, recordType)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型不匹配")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
		c.setSession(chatID, pendingDNSRecordName)
		return c.sendMessageOrEdit(ctx, chatID, msg, dnsRecordTypeMenu())
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		cfg, saveErr := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, recordName, "", recordType, ttl, proxied, true)
		if saveErr != nil {
			return saveErr
		}
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "⏳ 已保存记录名，等待绑定节点")
		_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
		_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
		c.completePrompt(ctx, chatID)
		c.clearSession(chatID)
		return c.sendMessageOrEdit(ctx, chatID, formatDNSPendingMessage(group.Name, cfg.RecordName), dnsPendingMenu())
	}
	c.completePrompt(ctx, chatID)
	c.setSession(chatID, pendingDNSRecordName)
	return c.sendMessageOrEdit(ctx, chatID, "记录 "+recordName+" 不存在。\n\n请选择初始解析到哪个节点：", dnsNodeMenu(nodes))
}

func (c *TelegramController) handleDNSCreateRecord(ctx context.Context, chatID int64, nodeID string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	if groupID == "" || recordName == "" {
		return c.sendMessageOrEdit(ctx, chatID, "DNS 创建流程已失效，请重新开始。", dnsPanelMenu())
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新开始。", dnsPanelMenu())
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", dnsPanelMenu())
	}
	token, _, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	ttl := defaultDNSRecordTTL
	proxied := defaultDNSRecordProxied
	if existing, cfgErr := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID); cfgErr == nil {
		ttl = normalizeDNSTTLValue(existing.TTL)
		proxied = existing.Proxied
	}
	if c.DNS == nil || strings.TrimSpace(zoneID) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法创建 DNS 记录。", dnsPanelMenu())
	}
	recordType := c.currentSessionValue(chatID, sessionKeyRecordType)
	if recordType == "" {
		recordType = "A"
	}
	record, err := c.DNS.CreateDNSRecordWithType(ctx, token, zoneID, recordName, node.PublicIP, recordType, ttl, proxied)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "❌ DNS 创建失败")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), msg, token)
		c.setSession(chatID, pendingDNSRecordName)
		return c.sendMessageOrEdit(ctx, chatID, "创建 DNS 记录失败："+msg, nil)
	}
	cfg, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, record.Type, normalizeDNSTTLValue(record.TTL), record.Proxied, true)
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
	return c.sendMessageOrEdit(ctx, chatID, text, dnsSavedMenu(node.ID))
}

func (c *TelegramController) handleDNSRepointToNode(ctx context.Context, chatID int64, nodeID string) error {
	groupID := c.currentSessionValue(chatID, sessionKeyGroupID)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	recordID := c.currentSessionValue(chatID, sessionKeyRecordID)
	currentIP := c.currentSessionValue(chatID, sessionKeyCurrentIP)
	if groupID == "" || recordName == "" || recordID == "" {
		return c.sendMessageOrEdit(ctx, chatID, "DNS 修正流程已失效，请重新开始。", dnsPanelMenu())
	}
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", dnsPanelMenu())
	}
	if node.GroupID != groupID {
		return c.sendMessageOrEdit(ctx, chatID, "节点不属于当前分组，请重新选择。", dnsPanelMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		return err
	}
	nodes, _ := c.Store.ListNodesByGroupID(ctx, groupID)
	if c.DNS == nil || strings.TrimSpace(cfg.ZoneID) == "" || strings.TrimSpace(cfg.APIToken) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法修正 DNS 记录。", dnsPanelMenu())
	}
	if err := c.DNS.UpdateDNSRecordWithType(ctx, cfg.APIToken, cfg.ZoneID, recordID, recordName, node.PublicIP, dnsRecordType(cfg, ""), cfg.TTL, cfg.Proxied); err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(groupID), "❌ DNS 修改失败")
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(groupID), msg, cfg.APIToken)
		c.setSession(chatID, pendingDNSFixSelect)
		return c.sendMessageOrEdit(ctx, chatID, "修正 DNS 记录失败："+msg, dnsFixMenu(nodes))
	}
	_ = c.Store.UpdateGroupCurrentNode(ctx, groupID, node.ID)
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(groupID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(groupID), "✅ DNS 修改成功")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(groupID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(groupID))
	c.clearSession(chatID)
	text := fmt.Sprintf("✅ DNS A 记录已更新\n\n域名：%s\n旧 IP：%s\n新 IP：%s\n匹配节点：%s\n\n下一步：安装 Agent", recordName, valueOrDash(currentIP), node.PublicIP, node.Name)
	return c.sendMessageOrEdit(ctx, chatID, text, dnsSavedMenu(node.ID))
}

func (c *TelegramController) handleDNSKeepCurrent(ctx context.Context, chatID int64) error {
	groupName := c.currentSessionValue(chatID, sessionKeyGroupName)
	recordName := c.currentSessionValue(chatID, sessionKeyRecordName)
	currentIP := c.currentSessionValue(chatID, sessionKeyCurrentIP)
	c.clearSession(chatID)
	return c.sendMessageOrEdit(ctx, chatID, formatDNSSavedMessage(groupName, recordName, currentIP, "", false), dnsSavedMenu(""))
}
