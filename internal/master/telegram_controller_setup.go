package master

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

func (c *TelegramController) sendMasterURLPromptWithPrefix(ctx context.Context, chatID int64, prefix string) error {
	suggested := c.ensureSuggestedMasterPublicURL(ctx)
	return c.sendPromptAndTrack(ctx, chatID, pendingMasterURL, prefix+masterURLHelp(suggested), masterURLPromptMenu(suggested))
}

func (c *TelegramController) ensureSuggestedMasterPublicURL(ctx context.Context) string {
	if c.Store != nil {
		if value, err := c.Store.GetSetting(ctx, settingSuggestedPublicAPIURL); err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	detectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := DetectPublicIPv4(detectCtx)
	if err != nil {
		return ""
	}
	suggested := SuggestedPublicAPIURLFromIP(ip)
	if suggested != "" && c.Store != nil {
		_ = c.Store.SetSetting(ctx, settingSuggestedPublicAPIURL, suggested)
	}
	return suggested
}

func masterURLHelp(suggested string) string {
	base := "请发送 Master 公网地址，例如：\nhttp://203.0.113.10:8080\nhttps://example.com"
	if strings.TrimSpace(suggested) == "" {
		return base + "\n\n也可以直接发送服务器公网 IP，系统会自动补全为 http://公网IP:8080。\n发送 /cancel 取消。"
	}
	return base + "\n\n检测到当前服务器公网地址：\n\n" + suggested + "\n\n你可以直接点击按钮使用，也可以手动发送其他地址。\n发送 /cancel 取消。"
}

func masterURLPromptMenu(suggested string) *telegram.ReplyMarkup {
	if strings.TrimSpace(suggested) == "" {
		return nil
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "使用当前公网地址", CallbackData: "use_suggested_master_url"}},
	}}
}

func (c *TelegramController) saveSuggestedMasterPublicURL(ctx context.Context, chatID int64) error {
	suggested := c.ensureSuggestedMasterPublicURL(ctx)
	if strings.TrimSpace(suggested) == "" {
		c.setSession(chatID, "master_url")
		return c.sendMessageOrEdit(ctx, chatID, "未能自动检测公网 IP，请手动发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	c.setSession(chatID, "master_url")
	return c.saveMasterPublicURL(ctx, chatID, suggested)
}

func (c *TelegramController) saveMasterPublicURL(ctx context.Context, chatID int64, raw string) error {
	value, normalized, err := NormalizeMasterPublicURLInput(raw)
	if err != nil {
		c.setSession(chatID, "master_url")
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	if err := c.Store.SetMasterPublicURL(ctx, value); err != nil {
		return err
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	msg := "✅ Master 公网地址已保存：\n" + value + "\n\n下一步：配置 Cloudflare"
	if normalized {
		msg = "检测到你输入的地址未带协议或端口，已自动补全为：\n" + value + "\n\n" + msg
	}
	return c.sendMessageOrEdit(ctx, chatID, msg, masterSavedMenu())
}

func (c *TelegramController) saveCloudflareConfig(ctx context.Context, chatID int64, token, zoneName, zoneID string) error {
	token = strings.TrimSpace(token)
	zoneName = strings.TrimSpace(zoneName)
	zoneID = strings.TrimSpace(zoneID)
	if token == "" || zoneName == "" {
		return c.sendMessageOrEdit(ctx, chatID, "Cloudflare Token 和 Zone Name 不能为空。", nil)
	}
	if zoneID == "" {
		if c.DNS == nil {
			return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID。", nil)
		}
		foundZoneID, err := c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			msg := friendlyCloudflareError(err)
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
			return c.sendMessageOrEdit(ctx, chatID, "自动查询 Zone ID 失败："+msg, nil)
		}
		zoneID = foundZoneID
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	return c.sendMessageOrEdit(ctx, chatID, "✅ Cloudflare 已保存：Token "+config.MaskSecret(token)+"，Zone "+zoneName+"，Zone ID "+maskMiddle(zoneID, 4, 4), cloudflareSavedMenu())
}

func (c *TelegramController) configureDNSRecord(ctx context.Context, group db.Group, recordName, recordID string, ttl int, proxied bool) (string, string, error) {
	token, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(zoneName) == "" {
		return "", "", fmt.Errorf("请先配置 Cloudflare Token 和 Zone")
	}
	if strings.TrimSpace(zoneID) == "" {
		if c.DNS == nil {
			return "", "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID")
		}
		zoneID, err = c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, friendlyCloudflareError(err), token)
			return "", "", fmt.Errorf("自动查询 Zone ID 失败：%w", err)
		}
		if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
			return "", "", err
		}
	}
	recordType := GroupDNSRecordType(ctx, c.Store, group.ID)
	currentIP := ""
	if strings.TrimSpace(recordID) == "" {
		if c.DNS == nil {
			return "", "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Record ID")
		}
		record, err := c.DNS.LookupDNSRecordWithType(ctx, token, zoneID, recordName, recordType)
		if err != nil {
			if any, anyErr := c.DNS.LookupDNSRecordAnyType(ctx, token, zoneID, recordName); anyErr == nil {
				msg := fmt.Sprintf("记录存在，但类型为 %s，不是 %s 记录", any.Type, recordType)
				_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型错误")
				_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
				return "", "", errors.New(msg)
			}
			msg := fmt.Sprintf("未找到 DNS %s 记录，请确认记录存在", recordType)
			_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
			return "", "", errors.New(msg)
		}
		recordID = record.ID
		currentIP = record.Content
		recordType = dnsRecordType(db.CloudflareConfig{RecordType: recordType}, record.Type)
	}
	cfg, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, recordName, recordID, recordType, ttl, proxied, true)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), err.Error(), token)
		return "", "", err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 配置已保存")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
	matchedNodeID := ""
	matchedNodeName := ""
	if currentIP != "" {
		nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
		if err == nil {
			for _, node := range nodes {
				if node.PublicIP == currentIP {
					_ = c.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
					matchedNodeID = node.ID
					matchedNodeName = node.Name
					break
				}
			}
		}
	}
	return formatDNSSavedMessage(group.Name, cfg.RecordName, currentIP, matchedNodeName, false), matchedNodeID, nil
}
