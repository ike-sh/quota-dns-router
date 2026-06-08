package master

import (
	"context"
	"errors"
	"strings"

	"quota-dns-router-go/internal/telegram"
)

func (c *TelegramController) withCallbackMessage(chatID, messageID int64, fn func() error) error {
	prevChat := c.callbackChat
	prevMsg := c.callbackMsg
	c.callbackChat = chatID
	c.callbackMsg = messageID
	defer func() {
		c.callbackChat = prevChat
		c.callbackMsg = prevMsg
	}()
	return fn()
}

func (c *TelegramController) sendMessageOrEdit(ctx context.Context, chatID int64, text string, markup *telegram.ReplyMarkup) error {
	_, err := c.sendMessageOrEditWithID(ctx, chatID, text, markup)
	return err
}

func (c *TelegramController) sendMessageOrEditWithID(ctx context.Context, chatID int64, text string, markup *telegram.ReplyMarkup) (int64, error) {
	if c.callbackChat == chatID && c.callbackMsg > 0 {
		err := c.Bot.EditMessageText(ctx, chatID, c.callbackMsg, text, markup)
		if err == nil {
			return c.callbackMsg, nil
		}
		var apiErr telegram.APIError
		if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Description), "message is not modified") {
			return c.callbackMsg, nil
		}
	}
	msg, err := c.Bot.SendMessageWithResult(ctx, chatID, text, markup)
	if err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

func (c *TelegramController) sendPromptAndTrack(ctx context.Context, chatID int64, state, text string, markup *telegram.ReplyMarkup) error {
	c.cleanupPrompt(ctx, chatID, "⛔ 已切换到新的配置流程")
	messageID, err := c.sendMessageOrEditWithID(ctx, chatID, text, markup)
	if err != nil {
		return err
	}
	c.setPrompt(chatID, state, messageID)
	return nil
}

func (c *TelegramController) cleanupPrompt(ctx context.Context, chatID int64, completedText string) {
	prompt := c.prompt(chatID)
	if prompt == nil {
		return
	}
	defer c.clearPrompt(chatID)
	if prompt.MessageID <= 0 {
		return
	}
	if err := c.Bot.DeleteMessage(ctx, chatID, prompt.MessageID); err == nil {
		return
	}
	if strings.TrimSpace(completedText) == "" {
		return
	}
	err := c.Bot.EditMessageText(ctx, chatID, prompt.MessageID, completedText, nil)
	if err == nil {
		return
	}
	var apiErr telegram.APIError
	if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Description), "message is not modified") {
		return
	}
}

func (c *TelegramController) completePrompt(ctx context.Context, chatID int64) {
	prompt := c.prompt(chatID)
	if prompt == nil {
		return
	}
	cleanupText := c.promptCompletionText(chatID, prompt.State)
	c.cleanupPrompt(ctx, chatID, cleanupText)
}

func (c *TelegramController) cancelPrompt(ctx context.Context, chatID int64) {
	c.cleanupPrompt(ctx, chatID, "⛔ 已取消当前配置")
}

func (c *TelegramController) promptCompletionText(chatID int64, state string) string {
	switch state {
	case pendingMasterURL:
		return "✅ 已完成：Master 公网地址"
	case pendingCloudflareToken:
		return "✅ 已完成：Cloudflare Token"
	case pendingCloudflareZoneName:
		return "✅ 已完成：Zone Name"
	case pendingDNSRecordName:
		return "✅ 已完成：DNS A 记录名称"
	case pendingDNSTTL:
		return "✅ 已完成：TTL"
	case pendingGroupName:
		return "✅ 已完成：分组名称"
	case pendingNodeName:
		return "✅ 已完成：节点名称"
	case pendingNodeIP:
		return "✅ 已完成：节点公网 IP"
	case pendingNodeQuota:
		return "✅ 已完成：默认月流量"
	case pendingNodeThreshold:
		return "✅ 已完成：默认阈值"
	case pendingNodeModeSelect:
		return "✅ 已完成：统计模式"
	case pendingNodeResetDay:
		return "✅ 已完成：默认重置日"
	case pendingNodePriority:
		return "✅ 已完成：节点 priority"
	case pendingNodeConfirm:
		if c.currentSessionValue(chatID, sessionKeyNodeFlow) == "edit" {
			return "✅ 已完成：节点策略"
		}
		return "✅ 已完成：节点创建"
	case pendingPolicyValue:
		switch c.currentSessionValue(chatID, sessionKeyPolicyField) {
		case policyFieldQuota:
			return "✅ 已完成：默认月流量"
		case policyFieldThreshold:
			return "✅ 已完成：默认阈值"
		case policyFieldResetDay:
			return "✅ 已完成：默认重置日"
		default:
			return "✅ 已完成：默认策略值"
		}
	default:
		return "✅ 已完成"
	}
}
