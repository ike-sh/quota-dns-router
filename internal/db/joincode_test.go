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

func TestRedeemJoinCodeSingleUse(t *testing.T) {
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
	code, err := store.GenerateJoinCode(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RedeemJoinCode(ctx, code); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := store.RedeemJoinCode(ctx, code); err == nil {
		t.Fatal("expected second redeem to fail")
	}
}

func TestRedeemJoinCodeRevokesPreviousAgentToken(t *testing.T) {
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
	firstCode, err := store.GenerateJoinCode(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.RedeemJoinCode(ctx, firstCode)
	if err != nil {
		t.Fatal(err)
	}
	secondCode, err := store.GenerateJoinCode(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RedeemJoinCode(ctx, secondCode)
	if err != nil {
		t.Fatal(err)
	}
	if first.AgentID != second.AgentID {
		t.Fatalf("expected same agent id, got %q vs %q", first.AgentID, second.AgentID)
	}
	ok, err := store.ValidateAgentToken(ctx, first.AgentID, first.AgentToken)
	if err != nil || ok {
		t.Fatalf("expected previous token revoked, ok=%t err=%v", ok, err)
	}
	ok, err = store.ValidateAgentToken(ctx, second.AgentID, second.AgentToken)
	if err != nil || !ok {
		t.Fatalf("expected new token valid, ok=%t err=%v", ok, err)
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
