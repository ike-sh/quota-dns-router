package db

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	rootmigrations "quota-dns-router-go/migrations"
)

func TestMigrateEmptyDatabase(t *testing.T) {
	store := openTestStoreWithoutMigrate(t)
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		table  string
		column string
	}{
		{"nodes", "traffic_offset_bytes"},
		{"nodes", "traffic_offset_cycle"},
		{"dns_switch_history", "trigger_type"},
	} {
		if !testColumnExists(t, store, item.table, item.column) {
			t.Fatalf("expected %s.%s to exist", item.table, item.column)
		}
	}
}

func TestMigrateCanRunRepeatedly(t *testing.T) {
	store := openTestStoreWithoutMigrate(t)
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateOldSchemaWithExistingColumns(t *testing.T) {
	store := openTestStoreWithoutMigrate(t)
	ctx := context.Background()
	body, err := fs.ReadFile(rootmigrations.FS, filepath.Base("001_initial.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO schema_migrations(version) VALUES('001_initial.sql'), ('002_last_errors.sql');
		ALTER TABLE dns_switch_history ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'threshold';
		ALTER TABLE nodes ADD COLUMN traffic_offset_bytes INTEGER NOT NULL DEFAULT 0;
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if !testColumnExists(t, store, "nodes", "traffic_offset_cycle") {
		t.Fatal("expected missing traffic_offset_cycle to be added")
	}
	for _, version := range []string{"003_switch_trigger_type.sql", "004_traffic_offset.sql"} {
		var count int
		if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("expected migration %s to be recorded once, got %d", version, count)
		}
	}
}

func openTestStoreWithoutMigrate(t *testing.T) *Store {
	t.Helper()
	store, err := Open("file:" + t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testColumnExists(t *testing.T, store *Store, table, column string) bool {
	t.Helper()
	rows, err := store.DB().Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
