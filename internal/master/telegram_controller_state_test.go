package master

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/telegram"
)

func TestConfigMasterURLNoArgEntersPending(t *testing.T) {
	controller, rec := newTestTelegramController(t)
	if err := controller.handleText(context.Background(), 1, "/config_master_url"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != "master_url" {
		t.Fatalf("expected pending master_url, got %q", controller.sessions[1])
	}
	if !rec.contains("使用当前公网地址") {
		t.Fatalf("expected suggested URL button in payloads: %v", rec.payloads)
	}
}

func TestConfigMasterURLInvalidThenValidKeepsPending(t *testing.T) {
	controller, rec := newTestTelegramController(t)
	ctx := context.Background()
	controller.setSession(1, "master_url")
	if err := controller.handleText(ctx, 1, "http:203.0.113.10:8080"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != "master_url" {
		t.Fatalf("expected pending state after invalid input")
	}
	if !rec.contains("请使用 http:// 或 https:// 开头") {
		t.Fatalf("expected precise error, got %v", rec.messages)
	}
	if err := controller.handleText(ctx, 1, "http://203.0.113.10:8080"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending state cleared")
	}
	value, err := controller.Store.GetMasterPublicURL(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if value != "http://203.0.113.10:8080" {
		t.Fatalf("unexpected saved URL: %s", value)
	}
}

func TestConfigMasterURLCommandArgSaves(t *testing.T) {
	controller, _ := newTestTelegramController(t)
	ctx := context.Background()
	if err := controller.handleText(ctx, 1, "/config_master_url http://203.0.113.10:8080"); err != nil {
		t.Fatal(err)
	}
	value, _ := controller.Store.GetMasterPublicURL(ctx, "")
	if value != "http://203.0.113.10:8080" {
		t.Fatalf("unexpected saved URL: %s", value)
	}
}

func TestConfigMasterURLInvalidCommandArgKeepsPending(t *testing.T) {
	controller, rec := newTestTelegramController(t)
	if err := controller.handleText(context.Background(), 1, "/config_master_url http:203.0.113.10:8080"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != "master_url" {
		t.Fatalf("expected pending state after invalid command arg")
	}
	if !rec.contains("请使用 http:// 或 https:// 开头") {
		t.Fatalf("expected precise error, got %v", rec.messages)
	}
}

func TestConfigMasterURLAutoCompletesHostPort(t *testing.T) {
	controller, _ := newTestTelegramController(t)
	ctx := context.Background()
	controller.setSession(1, "master_url")
	if err := controller.handleText(ctx, 1, "203.0.113.10:8080"); err != nil {
		t.Fatal(err)
	}
	value, _ := controller.Store.GetMasterPublicURL(ctx, "")
	if value != "http://203.0.113.10:8080" {
		t.Fatalf("unexpected saved URL: %s", value)
	}
}

func TestConfigMasterURLAutoCompletesIPWithDefaultPort(t *testing.T) {
	controller, _ := newTestTelegramController(t)
	ctx := context.Background()
	controller.setSession(1, "master_url")
	if err := controller.handleText(ctx, 1, "203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	value, _ := controller.Store.GetMasterPublicURL(ctx, "")
	if value != "http://203.0.113.10:8080" {
		t.Fatalf("unexpected saved URL: %s", value)
	}
}

func TestConfigMasterURLRejectsLocalAddresses(t *testing.T) {
	for _, input := range []string{"127.0.0.1", "localhost", "0.0.0.0"} {
		controller, _ := newTestTelegramController(t)
		controller.setSession(1, "master_url")
		if err := controller.handleText(context.Background(), 1, input); err != nil {
			t.Fatal(err)
		}
		if controller.sessions[1] != "master_url" {
			t.Fatalf("expected pending state for rejected %s", input)
		}
	}
}

func TestUseSuggestedMasterURLCallbackSaves(t *testing.T) {
	controller, _ := newTestTelegramController(t)
	ctx := context.Background()
	if err := controller.handleCallback(ctx, 1, "use_suggested_master_url"); err != nil {
		t.Fatal(err)
	}
	value, _ := controller.Store.GetMasterPublicURL(ctx, "")
	if value != "http://198.51.100.10:8080" {
		t.Fatalf("unexpected saved URL: %s", value)
	}
}

func TestCancelClearsPendingAndUnknownTextOnlyWithoutPending(t *testing.T) {
	controller, rec := newTestTelegramController(t)
	controller.setSession(1, "master_url")
	if err := controller.handleText(context.Background(), 1, "/cancel"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending state cleared")
	}
	if !rec.contains("已取消当前配置") {
		t.Fatalf("expected cancel response, got %v", rec.messages)
	}
	if err := controller.handleText(context.Background(), 1, "hello"); err != nil {
		t.Fatal(err)
	}
	if !rec.contains("未知命令") {
		t.Fatalf("expected unknown command only after no pending state, got %v", rec.messages)
	}
}

func newTestTelegramController(t *testing.T) (*TelegramController, *recordingTelegramClient) {
	t.Helper()
	store := testMasterStore(t)
	ctx := context.Background()
	if err := store.SetSetting(ctx, settingSuggestedPublicAPIURL, "http://198.51.100.10:8080"); err != nil {
		t.Fatal(err)
	}
	rec := &recordingTelegramClient{}
	bot := telegram.NewBot("token", 123, rec)
	return NewTelegramController(bot, store, "http://127.0.0.1:8080", time.Second, nil), rec
}

type recordingTelegramClient struct {
	messages []string
	payloads []string
	paths    []string
}

func (c *recordingTelegramClient) Do(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	c.paths = append(c.paths, req.URL.Path)
	if len(body) > 0 {
		c.payloads = append(c.payloads, string(body))
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			if text, ok := payload["text"].(string); ok {
				c.messages = append(c.messages, text)
			}
		}
	}
	response := `{"ok":true,"result":{}}`
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(response)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func (c *recordingTelegramClient) contains(value string) bool {
	for _, item := range append(append([]string{}, c.messages...), c.payloads...) {
		if strings.Contains(item, value) {
			return true
		}
	}
	return false
}

func (c *recordingTelegramClient) countPath(suffix string) int {
	count := 0
	for _, path := range c.paths {
		if strings.HasSuffix(path, suffix) {
			count++
		}
	}
	return count
}
