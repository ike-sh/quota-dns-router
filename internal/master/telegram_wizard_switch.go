package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/db"
)

func (c *TelegramController) sendSwitchPanel(ctx context.Context, chatID int64, prefix string) error {
	groups, err := c.Store.ListGroups(ctx)
	if err != nil {
		return err
	}
	var ready []db.Group
	for _, group := range groups {
		cfg, cfgErr := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
		if cfgErr == nil && strings.TrimSpace(cfg.RecordName) != "" {
			ready = append(ready, group)
		}
	}
	if len(ready) == 0 {
		return c.sendMessageOrEdit(ctx, chatID, prefix+"还没有可手动切换的 DNS 记录，请先完成 DNS 配置。", dnsPanelMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, prefix+"🔀 手动切换\n\n请选择要切换的分组：", switchGroupMenu(ready))
}

func (c *TelegramController) sendSwitchTargetMenu(ctx context.Context, chatID int64, groupID, prefix string) error {
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请重新选择。", mainMenu())
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil || strings.TrimSpace(cfg.RecordName) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "当前分组还没有 DNS A 记录，请先完成 DNS 配置。", dnsPanelMenu())
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return c.sendMessageOrEdit(ctx, chatID, "当前分组的 DNS 记录还处于待绑定状态，请先在 DNS 面板绑定到节点。", dnsPanelMenu())
	}
	nodes, err := c.Store.ListNodes(ctx)
	if err != nil {
		return err
	}
	var groupNodes []db.NodeWithGroup
	for _, node := range nodes {
		if node.GroupID == group.ID {
			groupNodes = append(groupNodes, node)
		}
	}
	if len(groupNodes) == 0 {
		return c.sendMessageOrEdit(ctx, chatID, "当前分组还没有节点，请先添加节点。", dnsNeedNodeMenu())
	}
	currentNode := "-"
	currentIP := "-"
	usages, usageErr := c.Store.ListNodeUsagesByGroup(ctx, group.ID, timeNow())
	if usageErr == nil && len(usages) > 0 && c.DNS != nil {
		service := Service{Store: c.Store, DNS: c.DNS, Now: timeNow}
		if current, resolveErr := service.ResolveCurrentNode(ctx, group, cfg, usages); resolveErr == nil {
			currentNode = current.Name
			currentIP = current.PublicIP
		}
	}
	text := fmt.Sprintf("%s🔀 手动切换\n\n当前 DNS：%s -> %s / %s\n\n请选择要切换到的节点：", prefix, cfg.RecordName, valueOrDash(currentNode), valueOrDash(currentIP))
	return c.sendMessageOrEdit(ctx, chatID, text, switchTargetMenu(group.ID, groupNodes))
}

func (c *TelegramController) sendSwitchConfirm(ctx context.Context, chatID int64, groupID, nodeID, prefix string) error {
	decision, err := c.buildManualSwitchDecision(ctx, groupID, nodeID)
	if err != nil {
		return c.sendMessageOrEdit(ctx, chatID, err.Error(), mainMenu())
	}
	text := prefix + fmt.Sprintf(
		"请确认手动切换：\n\n域名：%s\n旧节点：%s\n旧 IP：%s\n新节点：%s\n新 IP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Current.Name),
		valueOrDash(decision.Current.PublicIP),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
	if decision.Current.ID != "" && decision.Current.ID == decision.Target.ID {
		text = "当前 DNS 已经指向这个节点，无需再次切换。"
		return c.sendMessageOrEdit(ctx, chatID, text, manualSwitchDoneMenu(decision.Target.ID))
	}
	return c.sendMessageOrEdit(ctx, chatID, text, manualSwitchConfirmMenu(groupID, nodeID))
}
