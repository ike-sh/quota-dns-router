package master

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

type TelegramController struct {
	Bot          *telegram.Bot
	Store        *db.Store
	PublicAPIURL string
	PollTimeout  time.Duration
	DNS          DNSProvider
	sessions     map[int64]string
}

func NewTelegramController(bot *telegram.Bot, store *db.Store, publicAPIURL string, pollTimeout time.Duration, dns DNSProvider) *TelegramController {
	return &TelegramController{
		Bot:          bot,
		Store:        store,
		PublicAPIURL: publicAPIURL,
		PollTimeout:  pollTimeout,
		DNS:          dns,
		sessions:     make(map[int64]string),
	}
}

func (c *TelegramController) setSession(chatID int64, state string) {
	if c.sessions == nil {
		c.sessions = make(map[int64]string)
	}
	c.sessions[chatID] = state
}

func (c *TelegramController) clearSession(chatID int64) {
	if c.sessions != nil {
		delete(c.sessions, chatID)
	}
}

func (c *TelegramController) Run(ctx context.Context) error {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := c.Bot.GetUpdates(ctx, offset, c.PollTimeout)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			if err := c.handleUpdate(ctx, update); err != nil {
				_ = c.Bot.SendAdminMessage(ctx, "处理命令失败："+err.Error())
			}
		}
	}
}

func (c *TelegramController) handleUpdate(ctx context.Context, update telegram.Update) error {
	if update.Message != nil {
		if !c.Bot.IsAdmin(update.Message.From.ID) {
			return c.Bot.SendMessage(ctx, update.Message.Chat.ID, "无权限操作。", nil)
		}
		return c.handleText(ctx, update.Message.Chat.ID, strings.TrimSpace(update.Message.Text))
	}
	if update.CallbackQuery != nil {
		if !c.Bot.IsAdmin(update.CallbackQuery.From.ID) {
			_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "无权限")
			return nil
		}
		_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "已选择")
		return c.handleCallback(ctx, update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data)
	}
	return nil
}

func (c *TelegramController) handleCallback(ctx context.Context, chatID int64, data string) error {
	switch data {
	case "setup":
		return c.sendSetup(ctx, chatID)
	case "status":
		return c.sendStatus(ctx, chatID)
	case "master_url":
		c.setSession(chatID, "master_url")
		return c.sendMasterURLPrompt(ctx, chatID)
	case "use_suggested_master_url":
		return c.saveSuggestedMasterPublicURL(ctx, chatID)
	case "cf":
		return c.sendCloudflareSummary(ctx, chatID)
	case "dns":
		return c.sendDNSSummary(ctx, chatID)
	case "groups":
		return c.sendGroups(ctx, chatID)
	case "nodes":
		return c.sendNodes(ctx, chatID)
	case "policy":
		return c.Bot.SendMessage(ctx, chatID, policyHelp(), nil)
	case "agent":
		return c.Bot.SendMessage(ctx, chatID, agentHelp(), nil)
	case "switch":
		return c.Bot.SendMessage(ctx, chatID, "手动切换：/switch <分组名> <节点名>", nil)
	case "help":
		return c.Bot.SendMessage(ctx, chatID, helpText(), nil)
	default:
		return c.sendMenu(ctx, chatID)
	}
}

func (c *TelegramController) handleText(ctx context.Context, chatID int64, text string) error {
	if text == "/cancel" {
		c.clearSession(chatID)
		return c.Bot.SendMessage(ctx, chatID, "已取消当前配置。", nil)
	}
	if state := c.sessions[chatID]; state != "" && !strings.HasPrefix(text, "/") {
		return c.handleSession(ctx, chatID, state, text)
	}
	if text == "" {
		return c.sendMenu(ctx, chatID)
	}
	parts := strings.Fields(text)
	switch parts[0] {
	case "/start":
		return c.sendSetup(ctx, chatID)
	case "/menu":
		return c.sendMenu(ctx, chatID)
	case "/help":
		return c.Bot.SendMessage(ctx, chatID, helpText(), nil)
	case "/status":
		return c.sendStatus(ctx, chatID)
	case "/setup":
		return c.sendSetup(ctx, chatID)
	case "/config_master_url":
		if len(parts) >= 2 {
			c.setSession(chatID, "master_url")
			return c.saveMasterPublicURL(ctx, chatID, parts[1])
		}
		c.setSession(chatID, "master_url")
		return c.sendMasterURLPrompt(ctx, chatID)
	case "/cf":
		if len(parts) >= 3 {
			zoneID := ""
			if len(parts) >= 4 {
				zoneID = parts[3]
			}
			return c.saveCloudflareConfig(ctx, chatID, parts[1], parts[2], zoneID)
		}
		return c.sendCloudflareSummary(ctx, chatID)
	case "/groups":
		return c.handleGroupsCommand(ctx, chatID, parts)
	case "/nodes":
		return c.handleNodesCommand(ctx, chatID, parts)
	case "/dns":
		return c.handleDNSCommand(ctx, chatID, parts)
	case "/policy":
		return c.handlePolicyCommand(ctx, chatID, parts)
	case "/agent":
		return c.handleAgentCommand(ctx, chatID, parts)
	case "/switch":
		return c.handleSwitchCommand(ctx, chatID, parts)
	default:
		return c.Bot.SendMessage(ctx, chatID, "未知命令。发送 /help 查看用法。", nil)
	}
}

func (c *TelegramController) handleSession(ctx context.Context, chatID int64, state, text string) error {
	switch state {
	case "master_url":
		return c.saveMasterPublicURL(ctx, chatID, text)
	case "cf":
		parts := strings.Fields(text)
		if len(parts) < 2 {
			return c.Bot.SendMessage(ctx, chatID, "格式错误，请发送：<Cloudflare Token> <Zone Name> [Zone ID]", nil)
		}
		zoneID := ""
		if len(parts) >= 3 {
			zoneID = parts[2]
		}
		return c.saveCloudflareConfig(ctx, chatID, parts[0], parts[1], zoneID)
	default:
		return nil
	}
}

func (c *TelegramController) handleGroupsCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendGroups(ctx, chatID)
	}
	if len(parts) >= 3 && parts[1] == "add" {
		if err := ValidateGroupName(parts[2]); err != nil {
			return c.Bot.SendMessage(ctx, chatID, err.Error(), nil)
		}
		policy, _ := c.Store.GetPolicy(ctx)
		_, err := c.Store.CreateGroup(ctx, parts[2], policy.DefaultSwitchCooldownSecs)
		if err != nil {
			return err
		}
		return c.Bot.SendMessage(ctx, chatID, "分组已创建："+parts[2], nil)
	}
	return c.Bot.SendMessage(ctx, chatID, groupsHelp(), nil)
}

func (c *TelegramController) handleNodesCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendNodes(ctx, chatID)
	}
	if len(parts) >= 5 && parts[1] == "add" {
		group, err := c.Store.GetGroupByName(ctx, parts[4])
		if err != nil {
			return c.Bot.SendMessage(ctx, chatID, "分组不存在，请先 /groups add "+parts[4], nil)
		}
		policy, _ := c.Store.GetPolicy(ctx)
		node := db.Node{
			GroupID:               group.ID,
			Name:                  parts[2],
			PublicIP:              parts[3],
			MonthlyQuotaBytes:     policy.DefaultMonthlyQuotaBytes,
			ThresholdPercent:      policy.DefaultThresholdPercent,
			ResetDay:              policy.DefaultResetDay,
			TrafficMode:           policy.DefaultTrafficMode,
			Enabled:               true,
			AutoSwitch:            true,
			Priority:              100,
			PreferredIface:        "auto",
			ReportIntervalSeconds: policy.AgentReportIntervalSeconds,
		}
		for _, item := range parts[5:] {
			k, v, ok := strings.Cut(item, "=")
			if !ok {
				continue
			}
			switch k {
			case "quota":
				if bytes, err := parseGB(v); err == nil {
					node.MonthlyQuotaBytes = bytes
				}
			case "threshold":
				if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil {
					node.ThresholdPercent = n
				}
			case "reset_day":
				if n, err := strconv.Atoi(v); err == nil {
					node.ResetDay = n
				}
			case "mode":
				node.TrafficMode = normalizeMode(v)
			case "priority":
				if n, err := strconv.Atoi(v); err == nil {
					node.Priority = n
				}
			case "iface":
				node.PreferredIface = v
			case "enabled":
				node.Enabled = parseBool(v, true)
			case "auto_switch":
				node.AutoSwitch = parseBool(v, true)
			}
		}
		if err := ValidateNodeConfig(node); err != nil {
			return c.Bot.SendMessage(ctx, chatID, err.Error(), nil)
		}
		created, err := c.Store.CreateNode(ctx, node)
		if err != nil {
			return err
		}
		command, expiresAt, missing, err := c.createAgentInstallCommand(ctx, created.ID, policy)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return c.Bot.SendMessage(ctx, chatID, "节点已创建："+created.Name+"\n\n生成 Agent 安装命令前还缺少："+strings.Join(missing, "、"), setupMenu())
		}
		return c.Bot.SendMessage(ctx, chatID, fmt.Sprintf("节点已创建：%s\n加入码过期时间：%s\n安装命令：\n%s", created.Name, expiresAt.Format(time.RFC3339), command), nil)
	}
	return c.Bot.SendMessage(ctx, chatID, nodesHelp(), nil)
}

func (c *TelegramController) handleDNSCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendDNSSummary(ctx, chatID)
	}
	if len(parts) >= 4 && parts[1] == "set" {
		group, err := c.Store.GetGroupByName(ctx, parts[2])
		if err != nil {
			return err
		}
		ttl := 120
		proxied := false
		recordID := ""
		for _, item := range parts[4:] {
			k, v, ok := strings.Cut(item, "=")
			if !ok {
				continue
			}
			switch k {
			case "ttl":
				if n, err := strconv.Atoi(v); err == nil {
					ttl = n
				}
			case "proxied":
				proxied = parseBool(v, false)
			case "record_id":
				recordID = v
			}
		}
		msg, err := c.configureDNSRecord(ctx, group, parts[3], recordID, ttl, proxied)
		if err != nil {
			return c.Bot.SendMessage(ctx, chatID, err.Error(), nil)
		}
		return c.Bot.SendMessage(ctx, chatID, msg, nil)
	}
	return c.Bot.SendMessage(ctx, chatID, dnsHelp(), nil)
}

func (c *TelegramController) handlePolicyCommand(ctx context.Context, chatID int64, parts []string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if len(parts) == 1 {
		return c.Bot.SendMessage(ctx, chatID, db.FormatPolicy(policy)+"\n\n"+policyHelp(), nil)
	}
	if len(parts) < 3 || parts[1] != "set" {
		return c.Bot.SendMessage(ctx, chatID, policyHelp(), nil)
	}
	for _, item := range parts[2:] {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		switch k {
		case "threshold":
			if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil {
				policy.DefaultThresholdPercent = n
			}
		case "quota":
			if n, err := parseGB(v); err == nil {
				policy.DefaultMonthlyQuotaBytes = n
			}
		case "reset_day":
			if n, err := strconv.Atoi(v); err == nil {
				policy.DefaultResetDay = n
			}
		case "mode":
			policy.DefaultTrafficMode = normalizeMode(v)
		case "offline":
			if n, err := strconv.Atoi(v); err == nil {
				policy.AgentOfflineSeconds = n
			}
		case "auto_switch":
			policy.AutoSwitchEnabled = parseBool(v, true)
		case "notify_only":
			policy.NotifyOnly = parseBool(v, false)
		case "repo":
			policy.RepoInstallURL = v
		}
	}
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, "策略已更新。\n"+db.FormatPolicy(policy), nil)
}

func (c *TelegramController) handleAgentCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) >= 3 && parts[1] == "install" {
		node, err := c.Store.GetNodeByName(ctx, parts[2])
		if err != nil {
			return err
		}
		policy, _ := c.Store.GetPolicy(ctx)
		command, expiresAt, missing, err := c.createAgentInstallCommand(ctx, node.ID, policy)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return c.Bot.SendMessage(ctx, chatID, "生成 Agent 安装命令前还缺少："+strings.Join(missing, "、"), setupMenu())
		}
		return c.Bot.SendMessage(ctx, chatID, fmt.Sprintf("Agent 安装命令：\n%s\n\n加入码过期时间：%s", command, expiresAt.Format(time.RFC3339)), nil)
	}
	return c.Bot.SendMessage(ctx, chatID, agentHelp(), nil)
}

func (c *TelegramController) handleSwitchCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) < 3 {
		return c.Bot.SendMessage(ctx, chatID, "手动切换：/switch <分组名> <节点名>", nil)
	}
	group, err := c.Store.GetGroupByName(ctx, parts[1])
	if err != nil {
		return err
	}
	target, err := c.Store.GetNodeByName(ctx, parts[2])
	if err != nil {
		return err
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		return err
	}
	nodes, err := c.Store.ListNodeUsagesByGroup(ctx, group.ID, time.Now())
	if err != nil {
		return err
	}
	var current db.NodeUsage
	if group.CurrentNodeID.Valid {
		for _, n := range nodes {
			if n.ID == group.CurrentNodeID.String {
				current = n
				break
			}
		}
	}
	if current.ID == "" {
		current.Node = db.Node{Name: "-", PublicIP: "-"}
	}
	targetUsage, err := c.Store.GetNodeUsage(ctx, target, time.Now())
	if err != nil {
		return err
	}
	service := Service{Store: c.Store, Notifier: c.Bot, DNS: c.DNS, Now: time.Now}
	if service.DNS == nil {
		return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 DNS 客户端，无法手动切换。", nil)
	}
	return service.ExecuteSwitch(ctx, SwitchDecision{Group: group, Config: cfg, Current: current, Target: targetUsage, Reason: "手动切换", Triggered: true})
}

func (c *TelegramController) sendMenu(ctx context.Context, chatID int64) error {
	return c.Bot.SendMessage(ctx, chatID, "quota-dns-router 已启动。请选择操作：", mainMenu())
}

func (c *TelegramController) sendStatus(ctx context.Context, chatID int64) error {
	overview, err := BuildStatusOverview(ctx, c.Store, c.PublicAPIURL, c.DNS, time.Now())
	if err != nil {
		return err
	}
	msg := FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	return c.Bot.SendMessage(ctx, chatID, strings.TrimSpace(msg), nil)
}

func (c *TelegramController) sendSetup(ctx context.Context, chatID int64) error {
	status, err := BuildSetupStatus(ctx, c.Store, c.PublicAPIURL)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatSetupGuide(status), setupMenu())
}

func (c *TelegramController) sendGroups(ctx context.Context, chatID int64) error {
	groups, err := BuildGroupDiagnostics(ctx, c.Store, time.Now(), c.DNS)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatGroupDiagnostics(groups), nil)
}

func (c *TelegramController) sendNodes(ctx context.Context, chatID int64) error {
	nodes, err := BuildNodeDiagnostics(ctx, c.Store, time.Now())
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatNodeDiagnostics(nodes), nil)
}

func (c *TelegramController) sendCloudflareSummary(ctx context.Context, chatID int64) error {
	summary, err := BuildCloudflareSummary(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatCloudflareSummary(summary), nil)
}

func (c *TelegramController) sendDNSSummary(ctx context.Context, chatID int64) error {
	items, err := BuildDNSSummaries(ctx, c.Store, c.DNS)
	if err != nil {
		return err
	}
	return c.Bot.SendMessage(ctx, chatID, FormatDNSSummaries(items), nil)
}

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

func helpText() string {
	return `/start /menu 打开菜单
/status 查看状态
/setup 查看初始化流程
/config_master_url 配置 Master 公网地址
/cf <Token> <Zone Name> [Zone ID]
/groups add <分组名>
/dns set <分组名> <A记录> [ttl=120] [proxied=false] [record_id=xxx]
/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=100] [enabled=true] [auto_switch=true] [iface=auto]
/agent install <节点名>
/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false repo=<install-agent-url>
/switch <分组名> <节点名>`
}

func groupsHelp() string { return "分组命令：/groups add <分组名>" }
func nodesHelp() string {
	return "节点命令：/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=100] [enabled=true] [auto_switch=true] [iface=auto]"
}
func dnsHelp() string {
	return "DNS 命令：/dns set <分组名> <A记录> [ttl=120] [proxied=false] [record_id=xxx]"
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
		{{Text: "2. Cloudflare 配置", CallbackData: "cf"}, {Text: "3. DNS 配置", CallbackData: "dns"}},
		{{Text: "4. 分组管理", CallbackData: "groups"}, {Text: "5. 节点管理", CallbackData: "nodes"}},
		{{Text: "6. 流量策略", CallbackData: "policy"}, {Text: "7. Agent 安装", CallbackData: "agent"}},
		{{Text: "当前状态", CallbackData: "status"}},
	}}
}

func parseGB(v string) (int64, error) {
	v = strings.TrimSpace(strings.ToUpper(strings.TrimSuffix(v, "B")))
	multiplier := int64(1)
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
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return n * multiplier, nil
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
		return "https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/install-agent.sh"
	}
	return policy.RepoInstallURL
}

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

const settingSuggestedPublicAPIURL = "suggested_public_api_url"

func masterURLButton() *telegram.ReplyMarkup {
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "配置 Master 公网地址", CallbackData: "master_url"}},
	}}
}

func (c *TelegramController) sendMasterURLPrompt(ctx context.Context, chatID int64) error {
	suggested := c.ensureSuggestedMasterPublicURL(ctx)
	return c.Bot.SendMessage(ctx, chatID, masterURLHelp(suggested), masterURLPromptMenu(suggested))
}

func (c *TelegramController) ensureSuggestedMasterPublicURL(ctx context.Context) string {
	if c.Store != nil {
		if value, err := c.Store.GetSetting(ctx, settingSuggestedPublicAPIURL); err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	detectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := DetectPublicIPv4(detectCtx)
	if err != nil {
		return ""
	}
	suggested := SuggestedPublicAPIURLFromIP(ip)
	if suggested != "" && c.Store != nil {
		_ = c.Store.SetSetting(ctx, settingSuggestedPublicAPIURL, suggested)
	}
	return suggested
}

func masterURLHelp(suggested string) string {
	base := "请发送 Master 公网地址，例如：\nhttp://1.2.3.4:8080\nhttps://domain.example.com"
	if strings.TrimSpace(suggested) == "" {
		return base + "\n\n也可以直接发送服务器公网 IP，系统会自动补全为 http://公网IP:8080。\n发送 /cancel 取消。"
	}
	return base + "\n\n检测到当前服务器公网地址：\n\n" + suggested + "\n\n你可以直接点击按钮使用，也可以手动发送其他地址。\n发送 /cancel 取消。"
}

func masterURLPromptMenu(suggested string) *telegram.ReplyMarkup {
	if strings.TrimSpace(suggested) == "" {
		return nil
	}
	return &telegram.ReplyMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "使用当前公网地址", CallbackData: "use_suggested_master_url"}},
	}}
}

func (c *TelegramController) saveSuggestedMasterPublicURL(ctx context.Context, chatID int64) error {
	suggested := c.ensureSuggestedMasterPublicURL(ctx)
	if strings.TrimSpace(suggested) == "" {
		c.setSession(chatID, "master_url")
		return c.Bot.SendMessage(ctx, chatID, "未能自动检测公网 IP，请手动发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	c.setSession(chatID, "master_url")
	return c.saveMasterPublicURL(ctx, chatID, suggested)
}

func (c *TelegramController) saveMasterPublicURL(ctx context.Context, chatID int64, raw string) error {
	value, normalized, err := NormalizeMasterPublicURLInput(raw)
	if err != nil {
		c.setSession(chatID, "master_url")
		return c.Bot.SendMessage(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	if err := c.Store.SetMasterPublicURL(ctx, value); err != nil {
		return err
	}
	c.clearSession(chatID)
	msg := "✅ Master 公网地址已保存：\n" + value + "\n\n下一步：配置 Cloudflare：/cf"
	if normalized {
		msg = "检测到你输入的地址未带协议或端口，已自动补全为：\n" + value + "\n\n" + msg
	}
	return c.Bot.SendMessage(ctx, chatID, msg, nil)
}

func (c *TelegramController) saveCloudflareConfig(ctx context.Context, chatID int64, token, zoneName, zoneID string) error {
	token = strings.TrimSpace(token)
	zoneName = strings.TrimSpace(zoneName)
	zoneID = strings.TrimSpace(zoneID)
	if token == "" || zoneName == "" {
		return c.Bot.SendMessage(ctx, chatID, "Cloudflare Token 和 Zone Name 不能为空。", nil)
	}
	if zoneID == "" {
		if c.DNS == nil {
			return c.Bot.SendMessage(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID。", nil)
		}
		foundZoneID, err := c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			msg := friendlyCloudflareError(err)
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
			return c.Bot.SendMessage(ctx, chatID, "自动查询 Zone ID 失败："+msg, nil)
		}
		zoneID = foundZoneID
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	return c.Bot.SendMessage(ctx, chatID, "Cloudflare 已保存：Token "+config.MaskSecret(token)+"，Zone "+zoneName+"，Zone ID "+zoneID, nil)
}

func (c *TelegramController) configureDNSRecord(ctx context.Context, group db.Group, recordName, recordID string, ttl int, proxied bool) (string, error) {
	token, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(zoneName) == "" {
		return "", fmt.Errorf("请先配置 Cloudflare Token 和 Zone")
	}
	if strings.TrimSpace(zoneID) == "" {
		if c.DNS == nil {
			return "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID")
		}
		zoneID, err = c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, friendlyCloudflareError(err), token)
			return "", fmt.Errorf("自动查询 Zone ID 失败：%w", err)
		}
		if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
			return "", err
		}
	}
	currentIP := ""
	if strings.TrimSpace(recordID) == "" {
		if c.DNS == nil {
			return "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Record ID")
		}
		record, err := c.DNS.LookupDNSRecord(ctx, token, zoneID, recordName)
		if err != nil {
			if any, anyErr := c.DNS.LookupDNSRecordAnyType(ctx, token, zoneID, recordName); anyErr == nil {
				msg := fmt.Sprintf("记录存在，但类型为 %s，不是 A 记录", any.Type)
				_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型错误")
				_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
				return "", errors.New(msg)
			}
			msg := "未找到 DNS A 记录，请确认记录存在"
			_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
			return "", errors.New(msg)
		}
		recordID = record.ID
		currentIP = record.Content
	}
	cfg, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, recordName, recordID, ttl, proxied, true)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), err.Error(), token)
		return "", err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 配置已保存")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
	if currentIP != "" {
		nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
		if err == nil {
			for _, node := range nodes {
				if node.PublicIP == currentIP {
					_ = c.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
					break
				}
			}
		}
	}
	msg := fmt.Sprintf("DNS 已配置：%s\nZone：%s / %s\nRecord ID：%s\n当前 A 记录 IP：%s", cfg.RecordName, cfg.ZoneName, valueOrDash(cfg.ZoneID), valueOrDash(cfg.RecordID), valueOrDash(currentIP))
	if proxied {
		msg += "\n提示：proxied=true 可能影响真实源 IP 判断或连通性验证。"
	}
	return msg, nil
}

func (c *TelegramController) createAgentInstallCommand(ctx context.Context, nodeID string, policy db.Policy) (string, time.Time, []string, error) {
	status, err := BuildSetupStatus(ctx, c.Store, c.PublicAPIURL)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	missing := AgentInstallMissingItems(status)
	if len(missing) > 0 {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成 Agent 安装命令前缺少："+strings.Join(missing, "、"))
		return "", time.Time{}, missing, nil
	}
	publicURL, err := ValidateMasterPublicURL(status.PublicAPIURL)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return "", time.Time{}, nil, err
	}
	code, expiresAt, err := c.Store.GenerateJoinCodeWithExpiry(ctx, nodeID, 24*time.Hour)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成加入码失败")
		return "", time.Time{}, nil, err
	}
	command, err := BuildAgentInstallCommand(publicURL, installURL(policy), code)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return "", time.Time{}, nil, err
	}
	_ = c.Store.ClearLastError(ctx, errorKeyAgentInstall)
	return command, expiresAt, nil, nil
}
