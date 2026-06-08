package master

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

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
	text += "自动切换：" + ternaryText(policy.AutoSwitchEnabled, "启用", "关闭") + "\n"
	text += "维护窗口：" + ternaryText(policy.MaintenanceMode, "开启（暂停自动切换）", "关闭") + "\n\n"
	text += "通知设置\n\n"
	text += "流量阈值通知：启用\n"
	text += "节点离线通知：启用\n"
	text += "DNS 切换通知：启用\n"
	text += "节点恢复通知：启用\n"
	text += "没有可用目标通知：启用\n\n"
	text += "这些默认值会用于新建节点。已有节点可以在节点详情中单独修改。"
	return c.sendMessageOrEdit(ctx, chatID, text, policyPanelMenu())
}

func (c *TelegramController) sendPolicyModeMenu(ctx context.Context, chatID int64) error {
	return c.sendMessageOrEdit(ctx, chatID, "请选择默认统计模式：", policyModeMenu())
}

func (c *TelegramController) sendPolicyValuePrompt(ctx context.Context, chatID int64, prefix, field string) error {
	text := ""
	switch field {
	case policyFieldQuota:
		text = prefix + "请发送默认月流量，例如：500GB、1TB、1000GB。"
	case policyFieldResetDay:
		text = prefix + "请发送默认重置日（1-28）。"
	default:
		text = prefix + "请发送默认阈值百分比，例如：80 或 80%。"
	}
	return c.sendPromptAndTrack(ctx, chatID, pendingPolicyValue, text, nil)
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
			return c.sendMessageOrEdit(ctx, chatID, "❌ 默认月流量格式错误，请重新发送。", nil)
		}
		policy.DefaultMonthlyQuotaBytes = bytes
	case policyFieldThreshold:
		value, parseErr := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(text), "%"))
		if parseErr != nil || value < 1 || value > 100 {
			c.setSession(chatID, pendingPolicyValue)
			return c.sendMessageOrEdit(ctx, chatID, "❌ 默认阈值必须在 1-100 之间，请重新发送。", nil)
		}
		policy.DefaultThresholdPercent = value
	case policyFieldResetDay:
		value, parseErr := strconv.Atoi(strings.TrimSpace(text))
		if parseErr != nil || value < 1 || value > 28 {
			c.setSession(chatID, pendingPolicyValue)
			return c.sendMessageOrEdit(ctx, chatID, "❌ 默认重置日必须在 1-28 之间，请重新发送。", nil)
		}
		policy.DefaultResetDay = value
	default:
		c.clearSession(chatID)
		return c.sendMessageOrEdit(ctx, chatID, "策略修改流程已失效，请重新选择。", policyPanelMenu())
	}
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	return c.sendMessageOrEdit(ctx, chatID, "✅ 策略已更新。", policySavedMenu())
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
	return c.sendMessageOrEdit(ctx, chatID, "✅ 默认统计模式已更新为："+modeLabel(policy.DefaultTrafficMode), policySavedMenu())
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
	return c.sendMessageOrEdit(ctx, chatID, "✅ 自动切换已"+ternaryText(policy.AutoSwitchEnabled, "启用", "关闭"), policySavedMenu())
}

func (c *TelegramController) togglePolicyMaintenanceMode(ctx context.Context, chatID int64) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	policy.MaintenanceMode = !policy.MaintenanceMode
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	msg := "✅ 维护窗口已关闭，自动切换恢复正常。"
	if policy.MaintenanceMode {
		msg = "✅ 维护窗口已开启，自动切换已暂停（手动切换仍可用）。"
	}
	return c.sendMessageOrEdit(ctx, chatID, msg, policySavedMenu())
}
