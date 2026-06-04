package db

import (
	"context"
	"testing"
	"time"
)

func TestTrafficOffsetColumnAndUsage(t *testing.T) {
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
		MonthlyQuotaBytes:     1000,
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
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	cycle := BillingCycleStart(now, node.ResetDay).Format("2006-01-02")
	if err := store.SetNodeTrafficOffset(ctx, node.ID, 350, cycle); err != nil {
		t.Fatal(err)
	}
	if err := store.BindAgentToNode(ctx, node.ID, "agent-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAgentReport(ctx, AgentReport{
		AgentID:      "agent-1",
		Hostname:     "host",
		PublicIP:     node.PublicIP,
		Iface:        "eth0",
		RXDelta:      10,
		TXDelta:      5,
		ReportedAt:   now,
		AgentVersion: "test",
		Status:       "online",
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := store.GetNodeUsage(ctx, updated, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TrafficOffsetBytes != 350 || usage.AgentUsedBytes != 15 || usage.UsedBytes != 365 {
		t.Fatalf("expected offset + agent usage, got %+v", usage)
	}
}

func TestTrafficOffsetResetsInNewCycle(t *testing.T) {
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
		MonthlyQuotaBytes:     1000,
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
	june := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if err := store.SetNodeTrafficOffset(ctx, node.ID, 350, BillingCycleStart(june, node.ResetDay).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	july := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	updated, err := store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := store.GetNodeUsage(ctx, updated, july)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TrafficOffsetBytes != 0 || usage.UsedBytes != 0 {
		t.Fatalf("expected offset reset in new cycle, got %+v", usage)
	}
	reloaded, err := store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.TrafficOffsetBytes != 0 || reloaded.TrafficOffsetCycle != "" {
		t.Fatalf("expected persisted offset reset, got %+v", reloaded)
	}
}
