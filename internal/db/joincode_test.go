package db

import (
	"context"
	"testing"
	"time"
)

func TestRedeemJoinCodeExpired(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	group, err := store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.CreateNode(ctx, Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "203.0.113.10",
		MonthlyQuotaBytes:     100,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              1,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.GenerateJoinCode(ctx, node.ID, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RedeemJoinCode(ctx, code); err == nil {
		t.Fatal("expected expired join code error")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open("file:" + t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}
