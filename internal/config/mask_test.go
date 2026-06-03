package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMaskSecret(t *testing.T) {
	got := MaskSecret("1234567890")
	if got != "123****890" {
		t.Fatalf("unexpected mask: %s", got)
	}
}

func TestLoadMasterEmptyPathUsesEnvironmentOnly(t *testing.T) {
	t.Setenv("QDR_TELEGRAM_TOKEN", "telegram-token")
	t.Setenv("QDR_TELEGRAM_ADMIN_ID", "123")
	cfg, err := LoadMaster("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnvPath != "" {
		t.Fatalf("expected empty env path, got %q", cfg.EnvPath)
	}
	if cfg.TelegramToken != "telegram-token" || cfg.TelegramAdminID != 123 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadAgentEmptyPathUsesEnvironmentOnly(t *testing.T) {
	t.Setenv("QDR_MASTER_API_URL", "https://master.example.com")
	t.Setenv("QDR_AGENT_ID", "agent-1")
	t.Setenv("QDR_AGENT_TOKEN", "agent-token")
	cfg, err := LoadAgent("", "test-version")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EnvPath != "" {
		t.Fatalf("expected empty env path, got %q", cfg.EnvPath)
	}
	if cfg.MasterAPIURL != "https://master.example.com" || cfg.AgentID != "agent-1" || cfg.AgentToken != "agent-token" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestFormatEnvFileReadErrorPermissionDenied(t *testing.T) {
	err := formatEnvFileReadError("/etc/quota-dns-router/master.env", os.ErrPermission)
	if !strings.Contains(err.Error(), "无法读取配置文件 /etc/quota-dns-router/master.env：权限不足") {
		t.Fatalf("expected friendly permission error, got %s", err)
	}
	err = formatEnvFileReadError("/etc/quota-dns-router/master.env", errors.New("permission denied"))
	if strings.Contains(err.Error(), "权限不足") {
		t.Fatalf("plain error text should not be treated as os permission: %s", err)
	}
}
