package master

import (
	"context"
	"strings"
	"testing"

	"quota-dns-router-go/internal/db"
)

func TestBuildSetupStatusMissingItems(t *testing.T) {
	store := testMasterStore(t)
	status, err := BuildSetupStatus(context.Background(), store, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if status.PublicURLConfigured {
		t.Fatal("expected local URL to be unconfigured")
	}
	want := []string{"Master 公网地址", "Cloudflare Token", "Zone Name", "Zone ID", "DNS 记录", "分组", "节点"}
	for _, item := range want {
		if !contains(status.Missing, item) {
			t.Fatalf("expected missing item %s, got %v", item, status.Missing)
		}
	}
	report := FormatStatusReport(status, db.StatusSummary{})
	if !strings.Contains(report, "缺少配置") {
		t.Fatalf("expected missing hint in report: %s", report)
	}
}

func TestBuildSetupStatusMasksCloudflareToken(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	if err := store.SetMasterPublicURL(ctx, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCloudflareDefaults(ctx, "1234567890", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	status, err := BuildSetupStatus(ctx, store, "")
	if err != nil {
		t.Fatal(err)
	}
	if !status.CloudflareTokenConfigured {
		t.Fatal("expected token configured")
	}
	if status.CloudflareTokenMasked != "123****890" {
		t.Fatalf("unexpected masked token: %s", status.CloudflareTokenMasked)
	}
}

func TestCreateAgentInstallCommandPrecheck(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, err := store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "203.0.113.10",
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	controller := &TelegramController{Store: store, PublicAPIURL: "http://127.0.0.1:8080"}
	command, expiresAt, missing, err := controller.createAgentInstallCommand(ctx, node.ID, db.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if command != "" || !expiresAt.IsZero() {
		t.Fatal("expected command to be blocked")
	}
	if !contains(missing, "Master 公网地址") {
		t.Fatalf("expected Master 公网地址 missing, got %v", missing)
	}
}

func testMasterStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open("file:" + t.TempDir() + "/master.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
