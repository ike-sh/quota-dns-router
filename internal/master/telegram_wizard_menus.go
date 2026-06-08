package master

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

func dnsProviderPanelMenu(kind string) *telegram.ReplyMarkup {
	if strings.EqualFold(kind, "route53") {
		return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "选择 Hosted Zone", CallbackData: "cf_select_zone"}},
			{{Text: "查看当前配置", CallbackData: "cf_view"}},
			{{Text: "返回主菜单", CallbackData: "menu"}},
		}}
	}
	return cloudflarePanelMenu()
}

func dnsProviderNeedTokenMenu(kind string) *telegram.ReplyMarkup {
	if strings.EqualFold(kind, "route53") {
		return dnsProviderPanelMenu(kind)
	}
	return cloudflareNeedTokenMenu()
}

func cloudflarePanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置/更新 Token", CallbackData: "cf_token"}},
		{{Text: "选择 Zone", CallbackData: "cf_select_zone"}},
		{{Text: "查看当前配置", CallbackData: "cf_view"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareNeedTokenMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置/更新 Token", CallbackData: "cf_token"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareZoneMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "手动输入 Zone Name", CallbackData: "cf_zone_manual"}},
		{{Text: "重新输入 Token", CallbackData: "cf_token_reset"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloudflareZoneChoicesMenu(zones []cloudflare.Zone) *telegram.ReplyMarkup {
	return cloudflareZoneChoicesMenuForProvider(zones, "")
}

func cloudflareZoneChoicesMenuForProvider(zones []cloudflare.Zone, providerKind string) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(zones)+3)
	for i, zone := range zones {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: zone.Name, CallbackData: fmt.Sprintf("cf_zone_pick:%d", i)}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "手动输入 Zone Name", CallbackData: "cf_zone_manual"}})
	if !strings.EqualFold(providerKind, "route53") {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "重新输入 Token", CallbackData: "cf_token_reset"}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func cloudflareSavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置 DNS", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsRecordTypeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "A 记录（IPv4）", CallbackData: "dns_type:A"}},
		{{Text: "AAAA 记录（IPv6）", CallbackData: "dns_type:AAAA"}},
		{{Text: "返回 DNS 列表", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsPanelMenu(items ...DNSSummary) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "添加 DNS 记录", CallbackData: "dns_add"}},
		{{Text: "查看 DNS 状态", CallbackData: "dns_status"}},
	}
	for _, item := range items {
		label := item.GroupName
		if strings.TrimSpace(item.RecordName) != "" {
			label += " / " + item.RecordName
		}
		if item.Pending {
			label += "（待绑定）"
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: label, CallbackData: "dns_view:" + item.GroupID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsNoGroupMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "创建分组", CallbackData: "groups_new"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsNeedNodeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsPendingMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "查看 DNS 状态", CallbackData: "dns_status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsSavedMenu(nodeID string) *telegram.ReplyMarkup {
	callback := "agent"
	if strings.TrimSpace(nodeID) != "" {
		callback = "agent_node:" + nodeID
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "Agent 安装", CallbackData: callback}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func dnsFixMenu(nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+2)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         "改为指向节点 " + node.Name + " / " + node.PublicIP,
			CallbackData: "dns_repoint:" + node.ID,
		}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "保留当前 DNS", CallbackData: "dns_keep_current"}},
		[]telegram.InlineKeyboardButton{{Text: "返回", CallbackData: "dns"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsGroupMenu(groups []db.Group) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(groups)+2)
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "dns_group:" + group.ID}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "新建分组", CallbackData: "groups_new"}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func dnsNodeMenu(nodes []db.Node) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+1)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name + " / " + node.PublicIP, CallbackData: "dns_create:" + node.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回 DNS 列表", CallbackData: "dns"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func switchGroupMenu(groups []db.Group) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(groups)+1)
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "switch_group:" + group.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func switchTargetMenu(groupID string, nodes []db.NodeWithGroup) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+2)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name + " / " + node.PublicIP, CallbackData: "switch_pick:" + groupID + ":" + node.ID}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "返回分组列表", CallbackData: "switch"}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func manualSwitchConfirmMenu(groupID, nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "确认切换", CallbackData: "switch_do:" + groupID + ":" + nodeID}},
		{{Text: "返回节点列表", CallbackData: "switch_group:" + groupID}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func manualSwitchDoneMenu(nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "继续手动切换", CallbackData: "switch"}},
		{{Text: "查看节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func groupsPanelMenu(groups ...db.Group) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "新建分组", CallbackData: "groups_new"}},
		{{Text: "查看分组状态", CallbackData: "groups_status"}},
	}
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "groups_view:" + group.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func groupCreatedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "配置 DNS", CallbackData: "dns"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodesPanelMenu(nodes []db.NodeWithGroup) *telegram.ReplyMarkup {
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "查看节点状态", CallbackData: "nodes_status"}},
	}
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         node.Name + " / " + node.GroupName,
			CallbackData: "nodes_view:" + node.ID,
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodesNeedGroupMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "创建分组", CallbackData: "groups_new"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodesGroupMenu(groups []db.Group) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(groups)+1)
	for _, group := range groups {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: group.Name, CallbackData: "nodes_group:" + group.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodeQuotaMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1000GB", CallbackData: "nodes_quota_default"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodeThresholdMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "80%", CallbackData: "nodes_threshold_default"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodeResetDayMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1", CallbackData: "nodes_reset_day_default"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodePriorityMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "10", CallbackData: "nodes_priority_default"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodeCreateConfirmMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "确认创建", CallbackData: "nodes_confirm"}},
		{{Text: "修改流量策略", CallbackData: "nodes_customize_policy"}},
		{{Text: "重新填写", CallbackData: "nodes_restart"}},
		{{Text: "取消", CallbackData: "menu"}},
	}}
}

func nodePolicyConfirmMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "保存策略", CallbackData: "nodes_save_policy"}},
		{{Text: "取消", CallbackData: "menu"}},
	}}
}

func nodeCreatedMenu(nodeID string, hasDNS bool) *telegram.ReplyMarkup {
	if !hasDNS {
		return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "配置 DNS", CallbackData: "dns"}},
			{{Text: "继续生成 Agent 命令", CallbackData: "agent_node:" + nodeID}},
			{{Text: "当前状态", CallbackData: "status"}},
			{{Text: "返回主菜单", CallbackData: "menu"}},
		}}
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "生成 Agent 安装命令", CallbackData: "agent_node:" + nodeID}},
		{{Text: "查看节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func policyPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "修改默认月流量", CallbackData: "policy_quota"}},
		{{Text: "修改默认阈值", CallbackData: "policy_threshold"}},
		{{Text: "修改统计模式", CallbackData: "policy_mode"}},
		{{Text: "修改默认重置日", CallbackData: "policy_reset_day"}},
		{{Text: "开启/关闭自动切换", CallbackData: "policy_toggle_auto"}},
		{{Text: "开启/关闭维护窗口", CallbackData: "policy_toggle_maintenance"}},
		{{Text: "开关流量阈值通知", CallbackData: "notify_threshold"}},
		{{Text: "开关离线通知", CallbackData: "notify_offline"}},
		{{Text: "开关切换通知", CallbackData: "notify_switch"}},
		{{Text: "开关恢复通知", CallbackData: "notify_recovered"}},
		{{Text: "开关无目标通知", CallbackData: "notify_no_target"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func policyModeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "RX 下行", CallbackData: "policy_mode:rx"}},
		{{Text: "TX 上行", CallbackData: "policy_mode:tx"}},
		{{Text: "RX+TX 双向", CallbackData: "policy_mode:both"}},
		{{Text: "返回策略设置", CallbackData: "policy"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func policySavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "流量策略", CallbackData: "policy"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentPanelMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentNeedNodeMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "添加节点", CallbackData: "nodes_add"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func agentNodeMenu(nodes []db.NodeWithGroup) *telegram.ReplyMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(nodes)+1)
	for _, node := range nodes {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: node.Name, CallbackData: "agent_node:" + node.ID}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}})
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func agentCommandMenu(nodeID string, hasDNS bool, installCommand string) *telegram.ReplyMarkup {
	uninstallCommand := agentUninstallCommand()
	rows := make([][]telegram.InlineKeyboardButton, 0, 8)
	if !hasDNS {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "配置 DNS", CallbackData: "dns"}})
	}
	if copyText := copyTextButton(installCommand); copyText != nil {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "复制安装命令", CopyText: copyText}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "显示纯安装命令", CallbackData: "agent_copy:" + nodeID}})
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "重新生成命令", CallbackData: "agent_node:" + nodeID}})
	if copyText := copyTextButton(uninstallCommand); copyText != nil {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "复制卸载命令", CopyText: copyText}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "显示纯卸载命令", CallbackData: "agent_uninstall_copy:" + nodeID}},
		[]telegram.InlineKeyboardButton{{Text: "安装排查", CallbackData: "agent_troubleshoot:" + nodeID}},
		[]telegram.InlineKeyboardButton{{Text: "返回节点详情", CallbackData: "nodes_view:" + nodeID}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func copyTextButton(text string) *telegram.CopyTextButton {
	text = strings.TrimSpace(text)
	if text == "" || len([]rune(text)) > 256 {
		return nil
	}
	return &telegram.CopyTextButton{Text: text}
}

func nodeDetailMenu(node db.Node, hasReported, online bool) *telegram.ReplyMarkup {
	installText := "生成 Agent 安装命令"
	if !hasReported || !online {
		installText = "重新生成 Agent 安装命令"
	}
	enabledText := "禁用节点"
	if !node.Enabled {
		enabledText = "启用节点"
	}
	autoText := "关闭自动切换"
	if !node.AutoSwitch {
		autoText = "开启自动切换"
	}
	rows := [][]telegram.InlineKeyboardButton{
		{{Text: "手动切换到此节点", CallbackData: "switch_to_node:" + node.ID}},
		{{Text: installText, CallbackData: "agent_node:" + node.ID}},
		{{Text: "校准已用流量", CallbackData: "nodes_calibrate_traffic:" + node.ID}},
		{{Text: "流量统计说明", CallbackData: "nodes_traffic_help:" + node.ID}},
		{{Text: "修改节点策略", CallbackData: "nodes_edit_policy:" + node.ID}},
		{{Text: enabledText, CallbackData: "nodes_toggle_enabled:" + node.ID}},
		{{Text: autoText, CallbackData: "nodes_toggle_auto:" + node.ID}},
	}
	if !hasReported || !online {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "查看安装排查", CallbackData: "agent_troubleshoot:" + node.ID}})
	}
	rows = append(rows,
		[]telegram.InlineKeyboardButton{{Text: "返回节点列表", CallbackData: "nodes"}},
		[]telegram.InlineKeyboardButton{{Text: "返回主菜单", CallbackData: "menu"}},
	)
	return &telegram.ReplyMarkup{InlineKeyboard: rows}
}

func nodeTrafficOffsetPromptMenu(nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "清零初始已用流量", CallbackData: "nodes_clear_traffic_offset:" + nodeID}},
		{{Text: "返回节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodeTrafficOffsetSavedMenu(nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "返回节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodeTrafficHelpMenu(nodeID string) *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "校准已用流量", CallbackData: "nodes_calibrate_traffic:" + nodeID}},
		{{Text: "返回节点详情", CallbackData: "nodes_view:" + nodeID}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func nodePolicyEditMenu(node db.Node) *telegram.ReplyMarkup {
	autoText := "关闭自动切换"
	if !node.AutoSwitch {
		autoText = "开启自动切换"
	}
	enableText := "禁用节点"
	if !node.Enabled {
		enableText = "启用节点"
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "月流量总额", CallbackData: "nodes_edit_quota:" + node.ID}},
		{{Text: "阈值百分比", CallbackData: "nodes_edit_threshold:" + node.ID}},
		{{Text: "统计模式", CallbackData: "nodes_edit_mode:" + node.ID}},
		{{Text: "重置日", CallbackData: "nodes_edit_reset_day:" + node.ID}},
		{{Text: "优先级", CallbackData: "nodes_edit_priority:" + node.ID}},
		{{Text: enableText, CallbackData: "nodes_edit_toggle_enabled:" + node.ID}},
		{{Text: autoText, CallbackData: "nodes_edit_toggle_auto:" + node.ID}},
		{{Text: "返回节点详情", CallbackData: "nodes_view:" + node.ID}},
	}}
}

func masterSavedMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置 Cloudflare", CallbackData: "cf"}},
		{{Text: "当前状态", CallbackData: "status"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseCallbackInt(value, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(value, prefix))
}

func normalizeDNSRecordName(raw, zoneName string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, ".") && strings.TrimSpace(zoneName) != "" {
		return raw + "." + zoneName
	}
	return raw
}

func formatNodeQuota(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	const (
		gb = 1024 * 1024 * 1024
		tb = 1024 * gb
	)
	if value%tb == 0 {
		return fmt.Sprintf("%dTB", value/tb)
	}
	if value%gb == 0 {
		return fmt.Sprintf("%dGB", value/gb)
	}
	return raw
}

func maskMiddle(value string, prefixLen, suffixLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= prefixLen+suffixLen {
		return config.MaskSecret(value)
	}
	return value[:prefixLen] + "****" + value[len(value)-suffixLen:]
}

func ValidatePublicIPv4(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("公网 IPv4 不能为空")
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("公网 IPv4 无效")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("请填写可公网访问的 IPv4，不能使用私网地址、localhost 或 127.0.0.1")
	}
	return nil
}

func timeNow() time.Time {
	return time.Now()
}
