package master

import (
	"context"
	"strings"
)

func (c *TelegramController) sendAgentPanel(ctx context.Context, chatID int64, prefix string) error {
	nodes, err := c.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return c.sendMessageOrEdit(ctx, chatID, prefix+"🤖 Agent 安装\n\n还没有节点，请先添加节点。", agentNeedNodeMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, prefix+"🤖 Agent 安装\n\n请选择要安装 Agent 的节点：", agentNodeMenu(nodes))
}

func (c *TelegramController) sendAgentInstallCommand(ctx context.Context, chatID int64, nodeID string) error {
	if _, err := c.Store.GetNodeByID(ctx, nodeID); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "节点不存在，请重新选择。", agentPanelMenu())
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	preview, err := c.getAgentInstallPreview(ctx, nodeID, policy, true)
	if err != nil {
		return err
	}
	if len(preview.Missing) > 0 {
		return c.sendMessageOrEdit(ctx, chatID, "生成 Agent 安装命令前还缺少："+strings.Join(preview.Missing, "、"), setupMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, formatAgentInstallMessage(preview), agentCommandMenu(nodeID, preview.DNSReady, preview.Command))
}

func (c *TelegramController) sendPureAgentInstallCommand(ctx context.Context, chatID int64, nodeID string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	preview, err := c.getAgentInstallPreview(ctx, nodeID, policy, false)
	if err != nil {
		return err
	}
	if len(preview.Missing) > 0 {
		return c.sendMessageOrEdit(ctx, chatID, "生成 Agent 安装命令前还缺少："+strings.Join(preview.Missing, "、"), setupMenu())
	}
	return c.Bot.SendMessage(ctx, chatID, preview.Command, nil)
}

func (c *TelegramController) sendPureAgentUninstallCommand(ctx context.Context, chatID int64) error {
	return c.Bot.SendMessage(ctx, chatID, agentUninstallCommand(), nil)
}

func (c *TelegramController) tryDeleteMessage(ctx context.Context, chatID, messageID int64) {
	_ = c.Bot.DeleteMessage(ctx, chatID, messageID)
}
