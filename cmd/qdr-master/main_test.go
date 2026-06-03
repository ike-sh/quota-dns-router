package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/master"
)

func TestFormatMasterConfigCheckWarnsForLocalURL(t *testing.T) {
	cfg := config.MasterConfig{
		ListenAddr:      ":8080",
		PublicAPIURL:    "http://127.0.0.1:8080",
		DBPath:          "/tmp/master.db",
		DataDir:         "/tmp",
		LogDir:          "/tmp",
		TelegramToken:   "secret-token",
		TelegramAdminID: 123,
	}
	got := formatMasterConfigCheck(cfg, master.StatusOverview{
		Setup: master.SetupStatus{
			PublicAPIURL:              "http://127.0.0.1:8080",
			PublicURLWarning:          master.MasterPublicURLWarning("http://127.0.0.1:8080"),
			CloudflareTokenConfigured: false,
			Missing:                   []string{"Master 公网地址"},
		},
	})
	if !strings.Contains(got, "WARNING") {
		t.Fatalf("expected warning, got %s", got)
	}
	if !strings.Contains(got, "Agent join/install") {
		t.Fatalf("expected Agent join/install hint, got %s", got)
	}
}

func TestMasterVersionOutput(t *testing.T) {
	got := captureStdout(t, func() {
		if err := run([]string{"version"}); err != nil {
			t.Fatal(err)
		}
	})
	want := "quota-dns-router master 0.1.0-alpha.6"
	if strings.TrimSpace(got) != want {
		t.Fatalf("got %q want %q", strings.TrimSpace(got), want)
	}
}

func TestCLIStatusAndConfigCheckIncludeSwitchAndRisk(t *testing.T) {
	ctx := context.Background()
	store := testCLIStore(t)
	group, _ := store.CreateGroup(ctx, "hk", 600)
	oldNode, _ := store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "1.1.1.1",
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
	newNode, _ := store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-02",
		PublicIP:              "2.2.2.2",
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              20,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	_ = store.RecordSwitchHistory(ctx, group.ID, oldNode.ID, newNode.ID, "hk.example.com", "1.1.1.1", "2.2.2.2", "测试切换", "success", "")

	overview, err := master.BuildStatusOverview(ctx, store, "http://127.0.0.1:8080", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	statusOut := master.FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	for _, want := range []string{"最近切换", "hk-01 / 1.1.1.1 -> hk-02 / 2.2.2.2", "当前风险"} {
		if !strings.Contains(statusOut, want) {
			t.Fatalf("expected CLI status output to contain %q: %s", want, statusOut)
		}
	}

	cfg := config.MasterConfig{ListenAddr: ":8080", PublicAPIURL: "http://127.0.0.1:8080"}
	checkOut := formatMasterConfigCheck(cfg, overview)
	for _, want := range []string{"最近切换", "当前风险"} {
		if !strings.Contains(checkOut, want) {
			t.Fatalf("expected config-check output to contain %q: %s", want, checkOut)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func testCLIStore(t *testing.T) *db.Store {
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
