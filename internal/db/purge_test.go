package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeAgentReportsBefore(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
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
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindAgentToNode(ctx, node.ID, "agent-1"); err != nil {
		t.Fatal(err)
	}
	oldAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := store.SaveAgentReport(ctx, AgentReport{
		AgentID: "agent-1", Hostname: "h", PublicIP: node.PublicIP, Iface: "eth0",
		RXBytesTotal: 1, TXBytesTotal: 1, RXDelta: 1, TXDelta: 0,
		ReportedAt: oldAt, AgentVersion: "test", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAgentReport(ctx, AgentReport{
		AgentID: "agent-1", Hostname: "h", PublicIP: node.PublicIP, Iface: "eth0",
		RXBytesTotal: 2, TXBytesTotal: 2, RXDelta: 1, TXDelta: 0,
		ReportedAt: newAt, AgentVersion: "test", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	count, err := store.PurgeAgentReportsBefore(ctx, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 purged row, got %d", count)
	}
}
