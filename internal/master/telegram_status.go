package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/telegram"
)

type TelegramStatus struct {
	TokenMasked     string
	TokenConfigured bool
	AdminID         int64
	AdminConfigured bool
	GetMe           string
	Webhook         string
	Polling         string
	BotUsername     string
	LastError       string
	NextSuggestion  []string
}

func BuildTelegramStatus(ctx context.Context, cfg config.MasterConfig) (TelegramStatus, error) {
	status := TelegramStatus{
		TokenMasked:     config.MaskSecret(cfg.TelegramToken),
		TokenConfigured: strings.TrimSpace(cfg.TelegramToken) != "",
		AdminID:         cfg.TelegramAdminID,
		AdminConfigured: cfg.TelegramAdminID != 0,
		GetMe:           "未检查",
		Webhook:         "未检查",
		Polling:         "未检查",
		LastError:       "无",
	}
	if !status.TokenConfigured {
		status.GetMe = "❌ Token 未配置"
		status.Polling = "❌ 不可用"
		status.LastError = "缺少 QDR_TELEGRAM_TOKEN"
		status.NextSuggestion = []string{"请在 /etc/quota-dns-router/master.env 中配置 QDR_TELEGRAM_TOKEN 后重启服务"}
		return status, nil
	}

	bot := telegram.NewBot(cfg.TelegramToken, cfg.TelegramAdminID, nil)
	me, err := bot.GetMe(ctx)
	if err != nil {
		status.GetMe = "❌ " + sanitizeStatusMessage(err.Error())
		status.Polling = "❌ 不可用"
		status.LastError = sanitizeStatusMessage(err.Error())
		status.NextSuggestion = []string{"请到 @BotFather 重新生成 Token，并更新 /etc/quota-dns-router/master.env 后重启服务"}
		return status, nil
	}
	status.BotUsername = me.Username
	if me.Username != "" {
		status.GetMe = "✅ @" + me.Username
	} else {
		status.GetMe = fmt.Sprintf("✅ bot_id=%d", me.ID)
	}

	webhook, err := bot.GetWebhookInfo(ctx)
	if err != nil {
		status.Webhook = "❌ " + sanitizeStatusMessage(err.Error())
		status.Polling = "⚠️ getMe 成功，Webhook 状态未确认"
		status.LastError = sanitizeStatusMessage(err.Error())
		status.NextSuggestion = []string{"请稍后重试 telegram-status，或检查服务器到 api.telegram.org 的网络"}
		return status, nil
	}
	if strings.TrimSpace(webhook.URL) == "" {
		status.Webhook = "✅ 未设置"
		status.Polling = "✅ 可用"
		return status, nil
	}
	status.Webhook = "⚠️ 已设置：" + webhook.URL
	status.Polling = "⚠️ Webhook 已设置，long polling 可能收不到更新"
	status.NextSuggestion = []string{"如需使用 long polling，请先在 Telegram Bot API 清理 webhook"}
	if webhook.LastErrorMessage != "" {
		status.LastError = sanitizeStatusMessage(webhook.LastErrorMessage)
	}
	return status, nil
}

func FormatTelegramStatus(status TelegramStatus) string {
	var b strings.Builder
	b.WriteString("Telegram 状态：\n")
	if status.TokenConfigured {
		b.WriteString("Token：已配置 " + status.TokenMasked + "\n")
	} else {
		b.WriteString("Token：未配置\n")
	}
	if status.AdminConfigured {
		b.WriteString(fmt.Sprintf("Admin ID：%d\n", status.AdminID))
	} else {
		b.WriteString("Admin ID：未配置\n")
	}
	b.WriteString("getMe：" + valueOrDash(status.GetMe) + "\n")
	b.WriteString("Webhook：" + valueOrDash(status.Webhook) + "\n")
	b.WriteString("Polling：" + valueOrDash(status.Polling) + "\n")
	b.WriteString("最近错误：" + valueOrDash(status.LastError) + "\n")
	if len(status.NextSuggestion) > 0 {
		b.WriteString("建议：" + strings.Join(status.NextSuggestion, "；") + "\n")
	}
	return b.String()
}
