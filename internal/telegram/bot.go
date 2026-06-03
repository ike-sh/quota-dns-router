package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Bot struct {
	token      string
	adminID    int64
	httpClient HTTPClient
	baseURL    string
}

type UpdateResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

type APIError struct {
	Description string
}

func (e APIError) Error() string {
	if e.Description == "" {
		return "Telegram API 返回失败"
	}
	return e.Description
}

type BotUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type WebhookInfo struct {
	URL              string `json:"url"`
	PendingUpdates   int    `json:"pending_update_count"`
	LastErrorDate    int64  `json:"last_error_date"`
	LastErrorMessage string `json:"last_error_message"`
	MaxConnections   int    `json:"max_connections"`
	AllowedUpdates   any    `json:"allowed_updates"`
}

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
	Text      string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	ID int64 `json:"id"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	From    User    `json:"from"`
	Message Message `json:"message"`
	Data    string  `json:"data"`
}

type ReplyMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard,omitempty"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

func NewBot(token string, adminID int64, client HTTPClient) *Bot {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Bot{
		token:      token,
		adminID:    adminID,
		httpClient: client,
		baseURL:    fmt.Sprintf("https://api.telegram.org/bot%s", token),
	}
}

func (b *Bot) IsAdmin(id int64) bool {
	return id == b.adminID
}

func (b *Bot) GetUpdates(ctx context.Context, offset int, timeout time.Duration) ([]Update, error) {
	v := url.Values{}
	v.Set("offset", strconv.Itoa(offset))
	v.Set("timeout", strconv.Itoa(int(timeout.Seconds())))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/getUpdates?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out UpdateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("Telegram getUpdates 失败")
	}
	return out.Result, nil
}

func (b *Bot) SendMessage(ctx context.Context, chatID int64, text string, markup *ReplyMarkup) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return b.post(ctx, "/sendMessage", payload)
}

func (b *Bot) SendAdminMessage(ctx context.Context, text string) error {
	return b.SendMessage(ctx, b.adminID, text, nil)
}

func (b *Bot) AnswerCallback(ctx context.Context, callbackID, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
	}
	return b.post(ctx, "/answerCallbackQuery", payload)
}

func (b *Bot) GetMe(ctx context.Context) (BotUser, error) {
	var out BotUser
	if err := b.get(ctx, "/getMe", &out); err != nil {
		return BotUser{}, err
	}
	return out, nil
}

func (b *Bot) GetWebhookInfo(ctx context.Context) (WebhookInfo, error) {
	var out WebhookInfo
	if err := b.get(ctx, "/getWebhookInfo", &out); err != nil {
		return WebhookInfo{}, err
	}
	return out, nil
}

func (b *Bot) get(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var generic struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(respBody, &generic); err != nil {
		return err
	}
	if !generic.OK {
		return APIError{Description: generic.Description}
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(generic.Result, result)
}

func (b *Bot) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var generic struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &generic); err != nil {
		return err
	}
	if !generic.OK {
		return APIError{Description: generic.Description}
	}
	return nil
}
