package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMasterSupportsObserverIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.env")
	content := `QDR_TELEGRAM_TOKEN=token
QDR_TELEGRAM_ADMIN_ID=123
QDR_TELEGRAM_OBSERVER_IDS=456,789
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadMaster(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TelegramObserverIDs) != 2 || cfg.TelegramObserverIDs[0] != 456 {
		t.Fatalf("unexpected observers %+v", cfg.TelegramObserverIDs)
	}
}
