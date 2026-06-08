package master

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupAndRestoreDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "master.db")
	if err := os.WriteFile(dbPath, []byte("sqlite-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath, err := BackupDatabase(dbPath, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreDatabase(backupPath, dbPath); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "sqlite-data" {
		t.Fatalf("unexpected restored content %q", string(body))
	}
}
