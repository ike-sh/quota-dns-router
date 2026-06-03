package telegram

import "testing"

func TestIsAdmin(t *testing.T) {
	bot := NewBot("token", 123, nil)
	if !bot.IsAdmin(123) {
		t.Fatal("expected admin to pass")
	}
	if bot.IsAdmin(456) {
		t.Fatal("non-admin should fail")
	}
}
