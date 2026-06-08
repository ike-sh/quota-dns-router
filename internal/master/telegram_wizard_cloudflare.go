package master

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"quota-dns-router-go/internal/config"
)

func (c *TelegramController) isRoute53Provider() bool {
	return c.DNSProviderKind == "route53"
}

func (c *TelegramController) sendCloudflarePanel(ctx context.Context, chatID int64, prefix string) error {
	summary, err := BuildCloudflareSummary(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	text := prefix + dnsProviderPanelTitle(c.DNSProviderKind) + "\n\n"
	if c.isRoute53Provider() {
		text += "凭证：AWS 默认凭证链（环境变量 / IAM Role）\n"
	} else if summary.TokenConfigured {
		text += "Token：已配置 " + summary.TokenMasked + "\n"
	} else {
		text += "Token：未配置\n"
	}
	text += "Zone Name：" + valueOrDash(summary.ZoneName) + "\n"
	text += "Zone ID：" + valueOrDash(maskMiddle(summary.ZoneID, 4, 4)) + "\n\n请选择操作："
	return c.sendMessageOrEdit(ctx, chatID, text, dnsProviderPanelMenu(c.DNSProviderKind))
}

func dnsProviderPanelTitle(kind string) string {
	if strings.EqualFold(kind, "route53") {
		return "🌐 Route53 配置"
	}
	return "☁️ Cloudflare 配置"
}

func (c *TelegramController) sendCloudflareTokenPrompt(ctx context.Context, chatID int64, prefix string) error {
	text := prefix + "请发送 Cloudflare API Token。\n\n要求：\n- 需要 Zone Read 权限，用于查询 Zone\n- 需要 DNS Edit 权限，用于修改 A 记录\n- Token 只会脱敏显示，不会出现在日志中\n\n发送 /cancel 取消。"
	return c.sendPromptAndTrack(ctx, chatID, pendingCloudflareToken, text, nil)
}

func (c *TelegramController) showCloudflareZoneChoices(ctx context.Context, chatID int64, prefix string) error {
	token, _, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if c.isRoute53Provider() && strings.TrimSpace(token) == "" {
		_ = c.Store.SaveCloudflareDefaults(ctx, Route53PlaceholderToken, "", "")
		token = Route53PlaceholderToken
	}
	if strings.TrimSpace(token) == "" {
		return c.sendMessageOrEdit(ctx, chatID, prefix+"请先配置 Cloudflare Token。", dnsProviderNeedTokenMenu(c.DNSProviderKind))
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
		return c.sendMessageOrEdit(ctx, chatID, prefix+"查询 Zone 失败："+msg+"\n\n请重新发送 Cloudflare API Token，或发送 /cancel 取消。", cloudflareNeedTokenMenu())
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	meta := c.ensureSessionMeta(chatID)
	meta.Zones = zones
	c.setSession(chatID, pendingCloudflareZoneSelect)
	c.completePrompt(ctx, chatID)
	title := "请选择要管理的 Zone："
	if len(zones) == 1 {
		title = "检测到 1 个 Zone，是否使用这个 Zone？"
	}
	return c.sendMessageOrEdit(ctx, chatID, prefix+title, cloudflareZoneChoicesMenuForProvider(zones, c.DNSProviderKind))
}

func (c *TelegramController) sendCloudflareZoneNamePrompt(ctx context.Context, chatID int64, prefix string) error {
	return c.sendPromptAndTrack(ctx, chatID, pendingCloudflareZoneName, prefix+"请发送 Zone Name，例如：\nexample.com\n\n发送 /cancel 取消。", nil)
}

func (c *TelegramController) handleCloudflareZonePick(ctx context.Context, chatID int64, index int) error {
	meta := c.getSessionMeta(chatID)
	if meta == nil || index < 0 || index >= len(meta.Zones) {
		return c.sendMessageOrEdit(ctx, chatID, "Zone 选择已失效，请重新获取 Zone 列表。", cloudflarePanelMenu())
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
	return c.sendMessageOrEdit(ctx, chatID, fmt.Sprintf("✅ Cloudflare Zone 已保存\n\nZone Name：%s\nZone ID：%s\n\n下一步：配置 DNS A 记录", zone.Name, maskMiddle(zone.ID, 4, 4)), cloudflareSavedMenu())
}

func (c *TelegramController) handleCloudflareTokenInput(ctx context.Context, chatID int64, token string, messageID int64) error {
	token = strings.TrimSpace(token)
	if messageID > 0 {
		c.tryDeleteMessage(ctx, chatID, messageID)
	}
	if token == "" {
		c.setSession(chatID, pendingCloudflareToken)
		return c.sendMessageOrEdit(ctx, chatID, "❌ Token 不能为空，请重新发送 Cloudflare API Token。", nil)
	}
	_, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	if c.DNS == nil {
		c.completePrompt(ctx, chatID)
		c.beginFlow(chatID, pendingCloudflareZoneName, nil)
		return c.sendMessageOrEdit(ctx, chatID, "✅ Token 已保存："+config.MaskSecret(token)+"\n\n当前进程未配置 Cloudflare 客户端，请手动输入 Zone Name。", cloudflareZoneMenu())
	}
	zones, err := c.DNS.ListZones(ctx, token)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		c.setSession(chatID, pendingCloudflareToken)
		return c.sendMessageOrEdit(ctx, chatID, "✅ Token 已保存："+config.MaskSecret(token)+"\n\n查询 Zone 失败："+msg+"\n\n请重新发送 Cloudflare API Token，或发送 /cancel 取消。", nil)
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	meta := c.ensureSessionMeta(chatID)
	meta.Zones = zones
	c.setSession(chatID, pendingCloudflareZoneSelect)
	c.completePrompt(ctx, chatID)
	text := "✅ Token 已保存：" + config.MaskSecret(token) + "\n\n请选择要管理的 Zone："
	if len(zones) == 1 {
		text = "✅ Token 已保存：" + config.MaskSecret(token) + "\n\n检测到 1 个 Zone，是否使用这个 Zone？"
	}
	return c.sendMessageOrEdit(ctx, chatID, text, cloudflareZoneChoicesMenu(zones))
}

func (c *TelegramController) handleCloudflareZoneNameInput(ctx context.Context, chatID int64, zoneName string) error {
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" {
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ Zone Name 不能为空，请重新发送。", nil)
	}
	token, _, _, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		c.beginFlow(chatID, pendingCloudflareToken, nil)
		return c.sendMessageOrEdit(ctx, chatID, "请先配置 Cloudflare Token。", cloudflareNeedTokenMenu())
	}
	if c.DNS == nil {
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID。", nil)
	}
	zoneID, err := c.DNS.LookupZoneID(ctx, token, zoneName)
	if err != nil {
		msg := friendlyCloudflareError(err)
		_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
		_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
		c.setSession(chatID, pendingCloudflareZoneName)
		return c.sendMessageOrEdit(ctx, chatID, "查询 Zone 失败："+msg+"\n\n请重新发送 Zone Name，或发送 /cancel 取消。", nil)
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	return c.sendMessageOrEdit(ctx, chatID, fmt.Sprintf("✅ Cloudflare Zone 已保存\n\nZone Name：%s\nZone ID：%s\n\n下一步：配置 DNS A 记录", zoneName, maskMiddle(zoneID, 4, 4)), cloudflareSavedMenu())
}
