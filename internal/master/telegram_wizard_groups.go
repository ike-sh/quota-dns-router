package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/db"
)

func (c *TelegramController) sendGroupsPanel(ctx context.Context, chatID int64, prefix string) error {
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("%s📦 分组管理\n\n当前分组：%d\n", prefix, len(groups))
	if len(groups) == 0 {
		text += "\n还没有分组，请先创建分组。"
	} else {
		text += "\n点击下面的分组可查看详情、修改名称或进入该分组的 DNS/节点。"
	}
	text += "\n\n请选择操作："
	return c.sendMessageOrEdit(ctx, chatID, text, groupsPanelMenu(groups...))
}

func (c *TelegramController) ensureDefaultGroup(ctx context.Context) (db.Group, error) {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return db.Group{}, err
	}
	group, err := c.Store.CreateGroup(ctx, "default", policy.DefaultSwitchCooldownSecs)
	if err == nil {
		return group, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		return c.Store.GetGroupByName(ctx, "default")
	}
	return db.Group{}, err
}

func (c *TelegramController) sendGroupsStatus(ctx context.Context, chatID int64) error {
	items, err := BuildGroupDiagnostics(ctx, c.Store, timeNow(), c.DNS)
	if err != nil {
		return err
	}
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, FormatGroupDiagnostics(items), groupsPanelMenu(groups...))
}

func (c *TelegramController) sendGroupNamePrompt(ctx context.Context, chatID int64, prefix string) error {
	text := prefix + "请发送分组名，例如：\nhk\nsg\nus\n\n发送 /cancel 取消。"
	return c.sendPromptAndTrack(ctx, chatID, pendingGroupName, text, nil)
}

func (c *TelegramController) handleGroupNameInput(ctx context.Context, chatID int64, groupName string) error {
	groupName = strings.TrimSpace(groupName)
	if err := ValidateGroupName(groupName); err != nil {
		c.setSession(chatID, pendingGroupName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送分组名。", nil)
	}
	if groupID := c.currentSessionValue(chatID, sessionKeyGroupID); groupID != "" {
		if _, err := c.Store.GetGroupByID(ctx, groupID); err != nil {
			c.clearSession(chatID)
			return c.sendMessageOrEdit(ctx, chatID, "分组已失效，请重新选择。", groupsPanelMenu())
		}
		if err := c.Store.UpdateGroupName(ctx, groupID, groupName); err != nil {
			c.setSession(chatID, pendingGroupName)
			return c.sendMessageOrEdit(ctx, chatID, "❌ 修改分组名称失败："+err.Error()+"\n\n请换一个分组名重试。", nil)
		}
		c.completePrompt(ctx, chatID)
		c.clearSession(chatID)
		return c.sendGroupDetail(ctx, chatID, groupID, "✅ 分组名称已更新。\n\n")
	}
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if _, err := c.Store.CreateGroup(ctx, groupName, policy.DefaultSwitchCooldownSecs); err != nil {
		c.setSession(chatID, pendingGroupName)
		return c.sendMessageOrEdit(ctx, chatID, "❌ 创建分组失败："+err.Error()+"\n\n请换一个分组名重试。", nil)
	}
	c.completePrompt(ctx, chatID)
	c.clearSession(chatID)
	return c.sendMessageOrEdit(ctx, chatID, "✅ 分组已创建："+groupName+"\n\n下一步：", groupCreatedMenu())
}
