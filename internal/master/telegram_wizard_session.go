package master

import (
	"strings"

	"quota-dns-router-go/internal/cloudflare"
)

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
