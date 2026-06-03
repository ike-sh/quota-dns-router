package db

import (
	"context"
	"strings"
	"testing"
)

func TestSaveLastErrorMasksSecrets(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	err := store.SaveLastError(ctx, "cloudflare_zone_lookup", "token cf_secret_abcd returned 403", "cf_secret_abcd")
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.GetLastError(ctx, "cloudflare_zone_lookup")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(item.Message, "cf_secret_abcd") {
		t.Fatalf("expected masked message, got %s", item.Message)
	}
}
