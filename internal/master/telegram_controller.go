package master

import (
	"context"
	"strings"
	"time"

	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

const route53PlaceholderToken = "route53:aws-default"

type TelegramController struct {
	Bot               *telegram.Bot
	Store             *db.Store
	PublicAPIURL      string
	PollTimeout       time.Duration
	DNS               DNSProvider
	DNSProviderKind   string
	sessions          map[int64]string
	sessionMeta       map[int64]*telegramSessionMeta
	promptMeta        map[int64]*telegramPromptMeta
	agentInstallCache map[string]agentInstallPreview
	callbackChat      int64
	callbackMsg       int64
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

type telegramPromptMeta struct {
	State     string
	MessageID int64
}

func NewTelegramController(bot *telegram.Bot, store *db.Store, publicAPIURL string, pollTimeout time.Duration, dns DNSProvider, dnsProviderKind string) *TelegramController {
	return &TelegramController{
		Bot:               bot,
		Store:             store,
		PublicAPIURL:      publicAPIURL,
		PollTimeout:       pollTimeout,
		DNS:               dns,
		DNSProviderKind:   strings.ToLower(strings.TrimSpace(dnsProviderKind)),
		sessions:          make(map[int64]string),
		sessionMeta:       make(map[int64]*telegramSessionMeta),
		promptMeta:        make(map[int64]*telegramPromptMeta),
		agentInstallCache: make(map[string]agentInstallPreview),
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

func (c *TelegramController) setPrompt(chatID int64, state string, messageID int64) {
	if c.promptMeta == nil {
		c.promptMeta = make(map[int64]*telegramPromptMeta)
	}
	c.promptMeta[chatID] = &telegramPromptMeta{State: state, MessageID: messageID}
}

func (c *TelegramController) prompt(chatID int64) *telegramPromptMeta {
	if c.promptMeta == nil {
		return nil
	}
	return c.promptMeta[chatID]
}

func (c *TelegramController) clearPrompt(chatID int64) {
	if c.promptMeta == nil {
		return
	}
	delete(c.promptMeta, chatID)
}

func (c *TelegramController) cachedAgentInstallPreview(nodeID string) (agentInstallPreview, bool) {
	if c.agentInstallCache == nil {
		return agentInstallPreview{}, false
	}
	preview, ok := c.agentInstallCache[nodeID]
	if !ok {
		return agentInstallPreview{}, false
	}
	if preview.ExpiresAt.IsZero() || timeNow().After(preview.ExpiresAt) {
		delete(c.agentInstallCache, nodeID)
		return agentInstallPreview{}, false
	}
	return preview, true
}

func (c *TelegramController) storeAgentInstallPreview(nodeID string, preview agentInstallPreview) {
	if c.agentInstallCache == nil {
		c.agentInstallCache = make(map[string]agentInstallPreview)
	}
	c.agentInstallCache[nodeID] = preview
}

func (c *TelegramController) clearAgentInstallPreview(nodeID string) {
	if c.agentInstallCache == nil {
		return
	}
	delete(c.agentInstallCache, nodeID)
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
		userID := update.Message.From.ID
		if !c.Bot.CanAccess(userID) {
			return c.Bot.SendMessage(ctx, update.Message.Chat.ID, "无权限操作。", nil)
		}
		if c.Bot.IsObserver(userID) && isMutatingCommand(strings.TrimSpace(update.Message.Text)) {
			return c.Bot.SendMessage(ctx, update.Message.Chat.ID, "你是只读观察者，无法修改配置。", nil)
		}
		return c.handleMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		userID := update.CallbackQuery.From.ID
		if !c.Bot.CanAccess(userID) {
			_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "无权限")
			return nil
		}
		if c.Bot.IsObserver(userID) && isMutatingCallback(update.CallbackQuery.Data) {
			_ = c.Bot.AnswerCallback(ctx, update.CallbackQuery.ID, "只读观察者无权限")
			return c.Bot.SendMessage(ctx, update.CallbackQuery.Message.Chat.ID, "你是只读观察者，无法修改配置。", nil)
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
		return c.sendSwitchPanel(ctx, chatID, c.replaceSession(chatID))
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
		c.cancelPrompt(ctx, chatID)
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

func (c *TelegramController) sendMenu(ctx context.Context, chatID int64) error {
	return c.sendMessageOrEdit(ctx, chatID, "quota-dns-router 已启动。请选择操作：", mainMenu())
}

func (c *TelegramController) sendStatus(ctx context.Context, chatID int64) error {
	overview, err := BuildStatusOverview(ctx, c.Store, c.PublicAPIURL, c.DNS, time.Now())
	if err != nil {
		return err
	}
	msg := FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	markup := (*telegram.ReplyMarkup)(nil)
	if c.callbackChat == chatID && c.callbackMsg > 0 {
		markup = statusMenu()
	}
	return c.sendMessageOrEdit(ctx, chatID, strings.TrimSpace(msg), markup)
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
