package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMasterSupportsMultipleAdminIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.env")
	content := `QDR_TELEGRAM_TOKEN=token
QDR_TELEGRAM_ADMIN_IDS=123,456
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadMaster(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TelegramAdminIDs) != 2 || cfg.TelegramAdminIDs[0] != 123 || cfg.TelegramAdminIDs[1] != 456 {
		t.Fatalf("unexpected admin ids %+v", cfg.TelegramAdminIDs)
	}
}
