package master

import (
	"strings"
	"testing"
)

func TestFormatTelegramStatusMasksToken(t *testing.T) {
	text := FormatTelegramStatus(TelegramStatus{
		TokenConfigured: true,
		TokenMasked:     "886********XtE",
		AdminID:         7919687304,
		AdminConfigured: true,
		GetMe:           "✅ @dns_test_bot",
		Webhook:         "✅ 未设置",
		Polling:         "✅ 可用",
		LastError:       "无",
	})
	for _, want := range []string{"Token：已配置 886********XtE", "Admin ID：7919687304", "getMe：✅ @dns_test_bot", "Webhook：✅ 未设置"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in %s", want, text)
		}
	}
	if strings.Contains(text, "886abcdefXtE") {
		t.Fatalf("unexpected raw token in %s", text)
	}
}
