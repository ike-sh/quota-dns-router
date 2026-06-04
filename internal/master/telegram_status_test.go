package master

import (
	"strings"
	"testing"
)

func TestFormatTelegramStatusMasksToken(t *testing.T) {
	text := FormatTelegramStatus(TelegramStatus{
		TokenConfigured: true,
		TokenMasked:     "cf_********abcd",
		AdminID:         123456789,
		AdminConfigured: true,
		GetMe:           "✅ @dns_demo_bot",
		Webhook:         "✅ 未设置",
		Polling:         "✅ 可用",
		LastError:       "无",
	})
	for _, want := range []string{"Token：已配置 cf_********abcd", "Admin ID：123456789", "getMe：✅ @dns_demo_bot", "Webhook：✅ 未设置"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %s", want, text)
		}
	}
	if strings.Contains(text, "cf_raw_token_abcd") {
		t.Fatalf("unexpected raw token in %s", text)
	}
}
