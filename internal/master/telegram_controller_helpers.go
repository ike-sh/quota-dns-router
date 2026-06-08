package master

import (
	"strconv"
	"strings"

	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

func mainMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "初始化向导", CallbackData: "setup"}},
		{{Text: "配置 Master 公网地址", CallbackData: "master_url"}},
		{{Text: "当前状态", CallbackData: "status"}, {Text: "DNS 配置", CallbackData: "dns"}},
		{{Text: "节点管理", CallbackData: "nodes"}, {Text: "分组管理", CallbackData: "groups"}},
		{{Text: "流量策略", CallbackData: "policy"}, {Text: "Cloudflare 配置", CallbackData: "cf"}},
		{{Text: "Agent 安装", CallbackData: "agent"}, {Text: "手动切换", CallbackData: "switch"}},
		{{Text: "帮助", CallbackData: "help"}},
	}}
}

func statusMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "刷新状态", CallbackData: "status_refresh"}},
		{{Text: "DNS 配置", CallbackData: "dns"}},
		{{Text: "节点管理", CallbackData: "nodes"}},
		{{Text: "返回主菜单", CallbackData: "menu"}},
	}}
}

func helpText() string {
	return `/start /menu 打开菜单
/status 查看状态
/setup 查看初始化流程
/config_master_url 配置 Master 公网地址
/cf <Token> <Zone Name> [Zone ID]
/groups add <分组名>
/groups rename <原分组名> <新分组名>
/dns set <分组名> <A记录> [ttl=60|1(auto)] [proxied=false] [record_id=xxx]
/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=10] [enabled=true] [auto_switch=true] [iface=auto]
/agent install <节点名>
/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false repo=<install-agent-url>
/switch <分组名> <节点名>
/cancel 取消当前向导

推荐优先使用按钮和向导，以上带参数命令适合作为高级用法。`
}

func groupsHelp() string {
	return "分组命令：/groups add <分组名>；/groups rename <原分组名> <新分组名>"
}
func nodesHelp() string {
	return "节点命令：/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=10] [enabled=true] [auto_switch=true] [iface=auto]"
}
func dnsHelp() string {
	return "DNS 命令：/dns set <分组名> <A记录> [ttl=60|1(auto)] [proxied=false] [record_id=xxx]"
}
func cfHelp() string {
	return "Cloudflare：发送 /cf <Token> <Zone Name> [Zone ID]。\n如果不提供 Zone ID，程序会自动查询。"
}
func agentHelp() string { return "Agent 安装：/agent install <节点名>" }
func policyHelp() string {
	return "策略命令：/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false repo=<install-agent-url>"
}

func setupMenu() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1. 配置 Master 公网地址", CallbackData: "master_url"}},
		{{Text: "2. Cloudflare 配置", CallbackData: "cf"}},
		{{Text: "3. DNS 配置", CallbackData: "dns"}},
		{{Text: "4. 分组管理", CallbackData: "groups"}, {Text: "5. 节点管理", CallbackData: "nodes"}},
		{{Text: "6. Agent 安装", CallbackData: "agent"}},
		{{Text: "7. 当前状态", CallbackData: "status"}},
		{{Text: "流量策略", CallbackData: "policy"}},
	}}
}

func parseGB(v string) (int64, error) {
	v = strings.TrimSpace(strings.ToUpper(strings.TrimSuffix(v, "B")))
	multiplier := float64(1)
	if strings.HasSuffix(v, "G") {
		multiplier = 1024 * 1024 * 1024
		v = strings.TrimSuffix(v, "G")
	} else if strings.HasSuffix(v, "M") {
		multiplier = 1024 * 1024
		v = strings.TrimSuffix(v, "M")
	} else if strings.HasSuffix(v, "T") {
		multiplier = 1024 * 1024 * 1024 * 1024
		v = strings.TrimSuffix(v, "T")
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, err
	}
	return int64(n * multiplier), nil
}

func parseBool(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func normalizeMode(v string) string {
	switch strings.ToLower(v) {
	case "rx":
		return db.TrafficModeRX
	case "tx":
		return db.TrafficModeTX
	default:
		return db.TrafficModeBoth
	}
}

func installURL(policy db.Policy) string {
	if policy.RepoInstallURL == "" {
		return "https://raw.githubusercontent.com/ike-sh/quota-dns-router/v0.1.0/scripts/install-agent.sh"
	}
	return policy.RepoInstallURL
}
