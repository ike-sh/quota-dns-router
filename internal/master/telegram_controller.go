package master

import (
	"context"
	"database/sql"
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
	sessionMeta  map[int64]*telegramSessionMeta
	callbackChat int64
	callbackMsg  int64
}

type agentInstallPreview struct {
	Node         db.Node
	Group        db.Group
	PublicURL    string
	Command      string
	ExpiresAt    time.Time
	Missing      []string
	DNSReady     bool
	WarningLines []string
}

func NewTelegramController(bot *telegram.Bot, store *db.Store, publicAPIURL string, pollTimeout time.Duration, dns DNSProvider) *TelegramController {
	return &TelegramController{
		Bot:          bot,
		Store:        store,
		PublicAPIURL: publicAPIURL,
		PollTimeout:  pollTimeout,
		DNS:          dns,
		sessions:     make(map[int64]string),
		sessionMeta:  make(map[int64]*telegramSessionMeta),
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
	if c.sessionMeta != nil {
		delete(c.sessionMeta, chatID)
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
		return c.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		if !c.Bot.IsAdmin(update.CallbackQuery.From.ID) {
			_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "无权限")
			return nil
		}
		_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "已选择")
		return c.withCallbackMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, func() error {
			return c.handleCallback(ctx, update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data)
		})
	}
	return nil
}

func (c *TelegramController) handleMessage(ctx context.Context, message *telegram.Message) error {
	if message == nil {
		return nil
	}
	return c.handleTextMessage(ctx, message.Chat.ID, strings.TrimSpace(message.Text), message.MessageID)
}

func (c *TelegramController) handleCallback(ctx context.Context, chatID int64, data string) error {
	if handled, err := c.handleWizardCallback(ctx, chatID, data); handled {
		return err
	}
	switch data {
	case "setup":
		return c.sendSetup(ctx, chatID)
	case "status":
		return c.sendStatus(ctx, chatID)
	case "master_url":
		return c.sendMasterURLPromptWithPrefix(ctx, chatID, c.beginFlow(chatID, pendingMasterURL, nil))
	case "use_suggested_master_url":
		return c.saveSuggestedMasterPublicURL(ctx, chatID)
	case "cf":
		return c.sendCloudflarePanel(ctx, chatID, c.replaceSession(chatID))
	case "dns":
		return c.sendDNSPanel(ctx, chatID, c.replaceSession(chatID))
	case "groups":
		return c.sendGroupsPanel(ctx, chatID, c.replaceSession(chatID))
	case "nodes":
		return c.sendNodesPanel(ctx, chatID, c.replaceSession(chatID))
	case "policy":
		return c.sendPolicyPanel(ctx, chatID, c.replaceSession(chatID))
	case "agent":
		return c.sendAgentPanel(ctx, chatID, c.replaceSession(chatID))
	case "switch":
		return c.sendMessageOrEdit(ctx, chatID, "手动切换：/switch <分组名> <节点名>", nil)
	case "help":
		return c.sendMessageOrEdit(ctx, chatID, helpText(), nil)
	default:
		return c.sendMenu(ctx, chatID)
	}
}

func (c *TelegramController) handleText(ctx context.Context, chatID int64, text string) error {
	return c.handleTextMessage(ctx, chatID, text, 0)
}

func (c *TelegramController) handleTextMessage(ctx context.Context, chatID int64, text string, messageID int64) error {
	if text == "/cancel" {
		c.clearSession(chatID)
		return c.sendMessageOrEdit(ctx, chatID, "已取消当前配置。", nil)
	}
	if state := c.sessions[chatID]; state != "" && !strings.HasPrefix(text, "/") {
		return c.handlePendingInput(ctx, chatID, state, text, messageID)
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
		return c.sendMessageOrEdit(ctx, chatID, helpText(), nil)
	case "/status":
		return c.sendStatus(ctx, chatID)
	case "/setup":
		return c.sendSetup(ctx, chatID)
	case "/config_master_url":
		if len(parts) >= 2 {
			c.beginFlow(chatID, pendingMasterURL, nil)
			return c.saveMasterPublicURL(ctx, chatID, parts[1])
		}
		return c.sendMasterURLPromptWithPrefix(ctx, chatID, c.beginFlow(chatID, pendingMasterURL, nil))
	case "/cf":
		if len(parts) >= 3 {
			if messageID > 0 {
				c.tryDeleteMessage(ctx, chatID, messageID)
			}
			zoneID := ""
			if len(parts) >= 4 {
				zoneID = parts[3]
			}
			return c.saveCloudflareConfig(ctx, chatID, parts[1], parts[2], zoneID)
		}
		return c.sendCloudflarePanel(ctx, chatID, c.replaceSession(chatID))
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
		return c.sendMessageOrEdit(ctx, chatID, "未知命令。发送 /help 查看用法。", nil)
	}
}

func (c *TelegramController) handleSession(ctx context.Context, chatID int64, state, text string) error {
	return c.handlePendingInput(ctx, chatID, state, text, 0)
}

func (c *TelegramController) handleGroupsCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendGroupsPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) >= 3 && parts[1] == "add" {
		if err := ValidateGroupName(parts[2]); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		policy, _ := c.Store.GetPolicy(ctx)
		_, err := c.Store.CreateGroup(ctx, parts[2], policy.DefaultSwitchCooldownSecs)
		if err != nil {
			return err
		}
		return c.sendMessageOrEdit(ctx, chatID, "✅ 分组已创建："+parts[2]+"\n\n下一步：", groupCreatedMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, groupsHelp(), nil)
}

func (c *TelegramController) handleNodesCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendNodesPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) >= 5 && parts[1] == "add" {
		group, err := c.Store.GetGroupByName(ctx, parts[4])
		if err != nil {
			return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请先 /groups add "+parts[4], nil)
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
			Priority:              defaultNodePriority,
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
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		created, err := c.Store.CreateNode(ctx, node)
		if err != nil {
			return err
		}
		return c.sendNodeCreatedSummary(ctx, chatID, created)
	}
	return c.sendMessageOrEdit(ctx, chatID, nodesHelp(), nil)
}

func (c *TelegramController) handleDNSCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendDNSPanel(ctx, chatID, c.replaceSession(chatID))
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
		msg, matchedNodeID, err := c.configureDNSRecord(ctx, group, parts[3], recordID, ttl, proxied)
		if err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		return c.sendMessageOrEdit(ctx, chatID, msg, dnsSavedMenu(matchedNodeID))
	}
	return c.sendMessageOrEdit(ctx, chatID, dnsHelp(), nil)
}

func (c *TelegramController) handlePolicyCommand(ctx context.Context, chatID int64, parts []string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if len(parts) == 1 {
		return c.sendPolicyPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) < 3 || parts[1] != "set" {
		return c.sendMessageOrEdit(ctx, chatID, policyHelp(), nil)
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
	return c.sendMessageOrEdit(ctx, chatID, "策略已更新。\n"+db.FormatPolicy(policy), policySavedMenu())
}

func (c *TelegramController) handleAgentCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) >= 3 && parts[1] == "install" {
		node, err := c.Store.GetNodeByName(ctx, parts[2])
		if err != nil {
			return err
		}
		return c.sendAgentInstallCommand(ctx, chatID, node.ID)
	}
	return c.sendAgentPanel(ctx, chatID, c.replaceSession(chatID))
}

func (c *TelegramController) handleSwitchCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) < 3 {
		return c.sendMessageOrEdit(ctx, chatID, "手动切换：/switch <分组名> <节点名>", nil)
	}
	group, err := c.Store.GetGroupByName(ctx, parts[1])
	if err != nil {
		return err
	}
	target, err := c.Store.GetNodeByName(ctx, parts[2])
	if err != nil {
		return err
	}
	decision, err := c.buildManualSwitchDecision(ctx, group.ID, target.ID)
	if err != nil {
		return err
	}
	return c.executeManualSwitch(ctx, chatID, decision)
}

func (c *TelegramController) buildManualSwitchDecision(ctx context.Context, groupID, nodeID string) (SwitchDecision, error) {
	if c.DNS == nil {
		return SwitchDecision{}, errors.New("当前进程未配置 DNS 客户端，无法手动切换。")
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return SwitchDecision{}, err
	}
	targetNode, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return SwitchDecision{}, err
	}
	if targetNode.GroupID != group.ID {
		return SwitchDecision{}, errors.New("目标节点不属于该分组。")
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		return SwitchDecision{}, err
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return SwitchDecision{}, errors.New("当前分组还没有 DNS A 记录，请先完成 DNS 配置。")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		zoneID, lookupErr := c.DNS.LookupZoneID(ctx, cfg.APIToken, cfg.ZoneName)
		if lookupErr != nil {
			return SwitchDecision{}, lookupErr
		}
		cfg.ZoneID = zoneID
		_ = c.Store.SaveCloudflareDefaults(ctx, cfg.APIToken, cfg.ZoneName, zoneID)
		_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, cfg.RecordName, cfg.RecordID, cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		record, lookupErr := c.DNS.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
		if lookupErr != nil {
			return SwitchDecision{}, errors.New("当前 DNS 记录还处于待绑定状态，请先在 DNS 面板绑定到节点。")
		}
		cfg.RecordID = record.ID
		_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	nodes, err := c.Store.ListNodeUsagesByGroup(ctx, group.ID, time.Now())
	if err != nil {
		return SwitchDecision{}, err
	}
	var target db.NodeUsage
	for _, item := range nodes {
		if item.ID == targetNode.ID {
			target = item
			break
		}
	}
	if target.ID == "" {
		target, err = c.Store.GetNodeUsage(ctx, targetNode, time.Now())
		if err != nil {
			return SwitchDecision{}, err
		}
	}
	current := db.NodeUsage{Node: db.Node{Name: "-", PublicIP: "-"}}
	service := Service{Store: c.Store, DNS: c.DNS, Now: time.Now}
	if len(nodes) > 0 {
		if resolved, resolveErr := service.ResolveCurrentNode(ctx, group, cfg, nodes); resolveErr == nil {
			current = resolved
		}
	}
	return SwitchDecision{
		Group:       group,
		Config:      cfg,
		Current:     current,
		Target:      target,
		TriggerType: db.SwitchTriggerManual,
		Reason:      "手动切换",
		Triggered:   true,
	}, nil
}

func (c *TelegramController) executeManualSwitch(ctx context.Context, chatID int64, decision SwitchDecision) error {
	if decision.Current.ID != "" && decision.Current.ID == decision.Target.ID {
		return c.sendMessageOrEdit(ctx, chatID, manualSwitchAlreadyOnTargetMessage(decision), manualSwitchDoneMenu(decision.Target.ID))
	}
	service := Service{Store: c.Store, DNS: c.DNS, Now: time.Now}
	if err := service.ExecuteSwitch(ctx, decision); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "手动切换失败："+friendlyCloudflareError(err), manualSwitchDoneMenu(decision.Target.ID))
	}
	return c.sendMessageOrEdit(ctx, chatID, manualSwitchSuccessMessage(decision), manualSwitchDoneMenu(decision.Target.ID))
}

func (c *TelegramController) sendMenu(ctx context.Context, chatID int64) error {
	return c.sendMessageOrEdit(ctx, chatID, "quota-dns-router 已启动。请选择操作：", mainMenu())
}

func (c *TelegramController) sendStatus(ctx context.Context, chatID int64) error {
	overview, err := BuildStatusOverview(ctx, c.Store, c.PublicAPIURL, c.DNS, time.Now())
	if err != nil {
		return err
	}
	msg := FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	return c.sendMessageOrEdit(ctx, chatID, strings.TrimSpace(msg), nil)
}

func (c *TelegramController) sendSetup(ctx context.Context, chatID int64) error {
	status, err := BuildSetupStatus(ctx, c.Store, c.PublicAPIURL)
	if err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, FormatSetupGuide(status), setupMenu())
}

func (c *TelegramController) sendGroups(ctx context.Context, chatID int64) error {
	return c.sendGroupsPanel(ctx, chatID, "")
}

func (c *TelegramController) sendNodes(ctx context.Context, chatID int64) error {
	return c.sendNodesPanel(ctx, chatID, "")
}

func (c *TelegramController) sendCloudflareSummary(ctx context.Context, chatID int64) error {
	return c.sendCloudflarePanel(ctx, chatID, "")
}

func (c *TelegramController) sendDNSSummary(ctx context.Context, chatID int64) error {
	return c.sendDNSPanel(ctx, chatID, "")
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
/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=10] [enabled=true] [auto_switch=true] [iface=auto]
/agent install <节点名>
/policy set threshold=80 quota=1000GB reset_day=1 mode=both offline=300 auto_switch=true notify_only=false repo=<install-agent-url>
/switch <分组名> <节点名>
/cancel 取消当前向导

推荐优先使用按钮和向导，以上带参数命令适合作为高级用法。`
}

func groupsHelp() string { return "分组命令：/groups add <分组名>" }
func nodesHelp() string {
	return "节点命令：/nodes add <节点名> <公网IP> <分组名> [quota=1000GB] [threshold=80] [reset_day=1] [mode=rx|tx|both] [priority=10] [enabled=true] [auto_switch=true] [iface=auto]"
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
	if c.callbackChat == chatID && c.callbackMsg > 0 {
		err := c.Bot.EditMessageText(ctx, chatID, c.callbackMsg, text, markup)
		if err == nil {
			return nil
		}
		var apiErr telegram.APIError
		if errors.As(err, &apiErr) && strings.Contains(strings.ToLower(apiErr.Description), "message is not modified") {
			return nil
		}
	}
	return c.Bot.SendMessage(ctx, chatID, text, markup)
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
	return c.sendMasterURLPromptWithPrefix(ctx, chatID, "")
}

func (c *TelegramController) sendMasterURLPromptWithPrefix(ctx context.Context, chatID int64, prefix string) error {
	suggested := c.ensureSuggestedMasterPublicURL(ctx)
	return c.sendMessageOrEdit(ctx, chatID, prefix+masterURLHelp(suggested), masterURLPromptMenu(suggested))
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
		return c.sendMessageOrEdit(ctx, chatID, "未能自动检测公网 IP，请手动发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	c.setSession(chatID, "master_url")
	return c.saveMasterPublicURL(ctx, chatID, suggested)
}

func (c *TelegramController) saveMasterPublicURL(ctx context.Context, chatID int64, raw string) error {
	value, normalized, err := NormalizeMasterPublicURLInput(raw)
	if err != nil {
		c.setSession(chatID, "master_url")
		return c.sendMessageOrEdit(ctx, chatID, "❌ "+err.Error()+"\n\n请重新发送 Master 公网地址，或发送 /cancel 取消。", nil)
	}
	if err := c.Store.SetMasterPublicURL(ctx, value); err != nil {
		return err
	}
	c.clearSession(chatID)
	msg := "✅ Master 公网地址已保存：\n" + value + "\n\n下一步：配置 Cloudflare"
	if normalized {
		msg = "检测到你输入的地址未带协议或端口，已自动补全为：\n" + value + "\n\n" + msg
	}
	return c.sendMessageOrEdit(ctx, chatID, msg, masterSavedMenu())
}

func (c *TelegramController) saveCloudflareConfig(ctx context.Context, chatID int64, token, zoneName, zoneID string) error {
	token = strings.TrimSpace(token)
	zoneName = strings.TrimSpace(zoneName)
	zoneID = strings.TrimSpace(zoneID)
	if token == "" || zoneName == "" {
		return c.sendMessageOrEdit(ctx, chatID, "Cloudflare Token 和 Zone Name 不能为空。", nil)
	}
	if zoneID == "" {
		if c.DNS == nil {
			return c.sendMessageOrEdit(ctx, chatID, "当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID。", nil)
		}
		foundZoneID, err := c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			msg := friendlyCloudflareError(err)
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, msg, token)
			return c.sendMessageOrEdit(ctx, chatID, "自动查询 Zone ID 失败："+msg, nil)
		}
		zoneID = foundZoneID
	}
	if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
		return err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "✅ Zone 已验证")
	_ = c.Store.ClearLastError(ctx, errorKeyCloudflareZone)
	return c.sendMessageOrEdit(ctx, chatID, "✅ Cloudflare 已保存：Token "+config.MaskSecret(token)+"，Zone "+zoneName+"，Zone ID "+zoneID, cloudflareSavedMenu())
}

func (c *TelegramController) configureDNSRecord(ctx context.Context, group db.Group, recordName, recordID string, ttl int, proxied bool) (string, string, error) {
	token, zoneName, zoneID, err := c.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(zoneName) == "" {
		return "", "", fmt.Errorf("请先配置 Cloudflare Token 和 Zone")
	}
	if strings.TrimSpace(zoneID) == "" {
		if c.DNS == nil {
			return "", "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Zone ID")
		}
		zoneID, err = c.DNS.LookupZoneID(ctx, token, zoneName)
		if err != nil {
			_ = c.Store.SetStatusNote(ctx, noteKeyCloudflareZone, "❌ Zone 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyCloudflareZone, friendlyCloudflareError(err), token)
			return "", "", fmt.Errorf("自动查询 Zone ID 失败：%w", err)
		}
		if err := c.Store.SaveCloudflareDefaults(ctx, token, zoneName, zoneID); err != nil {
			return "", "", err
		}
	}
	currentIP := ""
	if strings.TrimSpace(recordID) == "" {
		if c.DNS == nil {
			return "", "", fmt.Errorf("当前进程未配置 Cloudflare 客户端，无法自动查询 Record ID")
		}
		record, err := c.DNS.LookupDNSRecord(ctx, token, zoneID, recordName)
		if err != nil {
			if any, anyErr := c.DNS.LookupDNSRecordAnyType(ctx, token, zoneID, recordName); anyErr == nil {
				msg := fmt.Sprintf("记录存在，但类型为 %s，不是 A 记录", any.Type)
				_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 记录类型错误")
				_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
				return "", "", errors.New(msg)
			}
			msg := "未找到 DNS A 记录，请确认记录存在"
			_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "❌ DNS 查询失败")
			_ = c.Store.SaveLastError(ctx, errorKeyDNSLookup(group.ID), msg, token)
			return "", "", errors.New(msg)
		}
		recordID = record.ID
		currentIP = record.Content
	}
	cfg, err := c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, recordName, recordID, ttl, proxied, true)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyDNSUpdate(group.ID), err.Error(), token)
		return "", "", err
	}
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSLookup(group.ID), "✅ DNS 记录查询成功")
	_ = c.Store.SetStatusNote(ctx, noteKeyDNSUpdate(group.ID), "✅ DNS 配置已保存")
	_ = c.Store.ClearLastError(ctx, errorKeyDNSLookup(group.ID))
	_ = c.Store.ClearLastError(ctx, errorKeyDNSUpdate(group.ID))
	matchedNodeID := ""
	matchedNodeName := ""
	if currentIP != "" {
		nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
		if err == nil {
			for _, node := range nodes {
				if node.PublicIP == currentIP {
					_ = c.Store.UpdateGroupCurrentNode(ctx, group.ID, node.ID)
					matchedNodeID = node.ID
					matchedNodeName = node.Name
					break
				}
			}
		}
	}
	return formatDNSSavedMessage(group.Name, cfg.RecordName, currentIP, matchedNodeName, false), matchedNodeID, nil
}

func (c *TelegramController) createAgentInstallCommand(ctx context.Context, nodeID string, policy db.Policy) (string, time.Time, []string, error) {
	preview, err := c.buildAgentInstallPreview(ctx, nodeID, policy)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	return preview.Command, preview.ExpiresAt, preview.Missing, nil
}

func (c *TelegramController) buildAgentInstallPreview(ctx context.Context, nodeID string, policy db.Policy) (agentInstallPreview, error) {
	node, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "节点不存在")
		return agentInstallPreview{}, err
	}
	group, err := c.Store.GetGroupByID(ctx, node.GroupID)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "节点所属分组不存在")
		return agentInstallPreview{}, err
	}
	status, err := BuildSetupStatus(ctx, c.Store, c.PublicAPIURL)
	if err != nil {
		return agentInstallPreview{}, err
	}
	preview := agentInstallPreview{
		Node:  node,
		Group: group,
	}
	missing := AgentInstallMissingItems(status)
	if len(missing) > 0 {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成 Agent 安装命令前缺少："+strings.Join(missing, "、"))
		preview.Missing = missing
		return preview, nil
	}
	publicURL, err := ValidateMasterPublicURL(status.PublicAPIURL)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return agentInstallPreview{}, err
	}
	preview.PublicURL = publicURL
	preview.DNSReady, preview.WarningLines, err = c.buildAgentInstallWarnings(ctx, group)
	if err != nil {
		return agentInstallPreview{}, err
	}
	code, expiresAt, err := c.Store.GenerateJoinCodeWithExpiry(ctx, nodeID, 30*time.Minute)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, "生成加入码失败")
		return agentInstallPreview{}, err
	}
	command, err := BuildAgentInstallCommand(publicURL, installURL(policy), code)
	if err != nil {
		_ = c.Store.SaveLastError(ctx, errorKeyAgentInstall, err.Error())
		return agentInstallPreview{}, err
	}
	preview.Command = command
	preview.ExpiresAt = expiresAt
	_ = c.Store.ClearLastError(ctx, errorKeyAgentInstall)
	return preview, nil
}

func (c *TelegramController) buildAgentInstallWarnings(ctx context.Context, group db.Group) (bool, []string, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, []string{
				"⚠️ 当前分组还没有 DNS A 记录。",
				"Agent 可以先安装，但 DNS 自动切换不会生效。",
				"建议先完成 DNS 配置。",
			}, nil
		}
		return false, nil, err
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return false, []string{
			"⚠️ 当前分组还没有 DNS A 记录。",
			"Agent 可以先安装，但 DNS 自动切换不会生效。",
			"建议先完成 DNS 配置。",
		}, nil
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		if c.DNS != nil && strings.TrimSpace(cfg.ZoneID) != "" && strings.TrimSpace(cfg.APIToken) != "" {
			record, lookupErr := c.DNS.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
			if lookupErr == nil {
				cfg.RecordID = record.ID
				_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, cfg.TTL, cfg.Proxied, cfg.AllowOverride)
			}
		}
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return false, []string{
			fmt.Sprintf("⚠️ 当前分组的 DNS 记录 %s 还处于待绑定状态。", cfg.RecordName),
			"请先在 DNS 面板把记录绑定到某个节点，再生成最终安装命令。",
		}, nil
	}
	if c.DNS == nil || strings.TrimSpace(cfg.ZoneID) == "" || strings.TrimSpace(cfg.APIToken) == "" {
		return true, nil, nil
	}
	record, err := c.DNS.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
	if err != nil {
		return true, nil, nil
	}
	nodes, err := c.Store.ListNodesByGroupID(ctx, group.ID)
	if err != nil {
		return true, nil, err
	}
	for _, node := range nodes {
		if node.PublicIP == record.Content {
			return true, nil, nil
		}
	}
	return true, []string{
		fmt.Sprintf("⚠️ 当前 DNS %s 解析到 %s，但没有匹配任何已配置节点。", cfg.RecordName, record.Content),
		"自动切换前建议先把 DNS 指向某个已配置节点。",
	}, nil
}

func formatAgentInstallMessage(preview agentInstallPreview) string {
	var b strings.Builder
	b.WriteString("🤖 Agent 安装命令预览\n\n")
	b.WriteString("节点：" + preview.Node.Name + "\n")
	b.WriteString("分组：" + preview.Group.Name + "\n")
	b.WriteString("Master：" + preview.PublicURL + "\n")
	if len(preview.WarningLines) > 0 {
		b.WriteString("\n")
		for _, line := range preview.WarningLines {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n请在节点 " + preview.Node.Name + " 上执行下面命令：\n\n")
	b.WriteString(preview.Command + "\n\n")
	b.WriteString("join code 有效期：30 分钟")
	if !preview.ExpiresAt.IsZero() {
		b.WriteString("（到 " + preview.ExpiresAt.Local().Format("2006-01-02 15:04:05") + "）")
	}
	b.WriteString("\n\nAgent 卸载命令：\n")
	b.WriteString(agentUninstallCommand())
	return strings.TrimSpace(b.String())
}

func formatDNSSavedMessage(groupName, recordName, currentIP, matchedNodeName string, created bool) string {
	title := "✅ DNS A 记录已保存"
	if created {
		title = "✅ DNS A 记录已创建"
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	b.WriteString("分组：" + groupName + "\n")
	b.WriteString("域名：" + valueOrDash(recordName) + "\n")
	b.WriteString("当前 IP：" + valueOrDash(currentIP) + "\n")
	b.WriteString("匹配节点：" + valueOrDash(matchedNodeName) + "\n\n")
	b.WriteString("下一步：生成 Agent 安装命令")
	return b.String()
}

func formatDNSPendingMessage(groupName, recordName string) string {
	return fmt.Sprintf(
		"⏳ DNS A 记录已保存为待绑定\n\n分组：%s\n域名：%s\n状态：还没有节点，稍后可在 DNS 面板中选择节点并创建记录。\n\n下一步：添加节点",
		groupName,
		valueOrDash(recordName),
	)
}

func manualSwitchSuccessMessage(decision SwitchDecision) string {
	return fmt.Sprintf(
		"✅ 手动切换完成\n\n域名：%s\n旧节点：%s\n旧 IP：%s\n新节点：%s\n新 IP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Current.Name),
		valueOrDash(decision.Current.PublicIP),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
}

func manualSwitchAlreadyOnTargetMessage(decision SwitchDecision) string {
	return fmt.Sprintf(
		"当前 DNS 已经指向目标节点。\n\n域名：%s\n节点：%s\nIP：%s",
		valueOrDash(decision.Config.RecordName),
		valueOrDash(decision.Target.Name),
		valueOrDash(decision.Target.PublicIP),
	)
}

func agentUninstallCommand() string {
	return "bash <(curl -fsSL https://raw.githubusercontent.com/ike-sh/quota-dns-router/main/scripts/uninstall-agent.sh) --yes"
}

func (c *TelegramController) sendNodeCreatedSummary(ctx context.Context, chatID int64, node db.Node) error {
	hasDNS, err := c.groupHasDNSConfig(ctx, node.GroupID)
	if err != nil {
		return err
	}
	if !hasDNS {
		pending, pendingErr := c.groupHasPendingDNSConfig(ctx, node.GroupID)
		if pendingErr != nil {
			return pendingErr
		}
		if pending {
			return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：当前分组已有待绑定的 DNS A 记录，请先到 DNS 面板绑定到这个节点。", nodeCreatedMenu(node.ID, false))
		}
		return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：当前分组还没有绑定 DNS A 记录，请先配置 DNS。", nodeCreatedMenu(node.ID, false))
	}
	return c.sendMessageOrEdit(ctx, chatID, "✅ 节点已创建："+node.Name+"\n\n下一步：生成 Agent 安装命令。", nodeCreatedMenu(node.ID, true))
}

func (c *TelegramController) groupHasDNSConfig(ctx context.Context, groupID string) (bool, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) != "", nil
}

func (c *TelegramController) groupHasPendingDNSConfig(ctx context.Context, groupID string) (bool, error) {
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(cfg.RecordName) != "" && strings.TrimSpace(cfg.RecordID) == "", nil
}
