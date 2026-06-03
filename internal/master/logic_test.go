package master

import (
	"database/sql"
	"testing"
	"time"

	"quota-dns-router-go/internal/db"
)

func TestSelectTarget(t *testing.T) {
	now := time.Now()
	policy := db.DefaultPolicy()
	nodes := []db.NodeUsage{
		{Node: db.Node{ID: "current", Name: "hk-01", Priority: 10, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 70}},
		{Node: db.Node{ID: "n2", Name: "hk-02", Priority: 1, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 40}},
		{Node: db.Node{ID: "n3", Name: "hk-03", Priority: 1, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 20}},
	}
	target, ok := SelectTarget("current", nodes, policy, now)
	if !ok {
		t.Fatal("expected target")
	}
	if target.ID != "n3" {
		t.Fatalf("expected n3, got %s", target.ID)
	}
}

func TestCooldown(t *testing.T) {
	group := db.Group{
		SwitchCooldownSeconds: 600,
		LastSwitchAt: sql.NullTime{
			Valid: true,
			Time:  time.Now().Add(-5 * time.Minute),
		},
	}
	if !inCooldown(group, time.Now()) {
		t.Fatal("expected cooldown")
	}
}

func TestReasonForSwitchThreshold(t *testing.T) {
	policy := db.DefaultPolicy()
	current := db.NodeUsage{
		Node:          db.Node{Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80},
		UsageSnapshot: db.UsageSnapshot{UsedBytes: 80},
	}
	if reason := reasonForSwitch(current, policy, time.Now()); reason == "" {
		t.Fatal("expected threshold reason")
	}
}
