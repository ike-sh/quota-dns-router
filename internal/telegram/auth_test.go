package telegram

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsAdmin(t *testing.T) {
	bot := NewBot("token", 123, nil)
	if !bot.IsAdmin(123) {
		t.Fatal("expected admin to pass")
	}
	if bot.IsAdmin(456) {
		t.Fatal("non-admin should fail")
	}
}

func TestIsAdminSupportsMultipleIDs(t *testing.T) {
	bot := NewBotForAdmins("token", []int64{123, 456}, nil)
	if !bot.IsAdmin(456) {
		t.Fatal("expected second admin to pass")
	}
	if bot.IsAdmin(789) {
		t.Fatal("unexpected admin")
	}
}

func TestInlineKeyboardButtonSupportsCopyText(t *testing.T) {
	markup := ReplyMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "复制安装命令", CopyText: &CopyTextButton{Text: "bash install.sh"}},
	}}}
	body, err := json.Marshal(markup)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, `"copy_text":{"text":"bash install.sh"}`) {
		t.Fatalf("expected copy_text JSON, got %s", got)
	}
}
