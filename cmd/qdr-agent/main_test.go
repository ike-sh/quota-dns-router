package main

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/traffic"
)

func TestAgentVersionOutput(t *testing.T) {
	got := captureStdout(t, func() {
		if err := run([]string{"version"}); err != nil {
			t.Fatal(err)
		}
	})
	want := "quota-dns-router agent 0.1.0-rc.1"
	if strings.TrimSpace(got) != want {
		t.Fatalf("got %q want %q", strings.TrimSpace(got), want)
	}
}

func TestFormatAgentStatusShowsInterfaceDiagnostics(t *testing.T) {
	cfg := config.AgentConfig{
		MasterAPIURL: "http://203.0.113.10:8080",
		NodeName:     "hk",
	}
	diag := traffic.Diagnostics{
		SelectedIface: "eth0",
		RouteIface:    "eth0",
		Snapshot:      traffic.Snapshot{Iface: "eth0", RX: 123, TX: 456},
		LastState:     traffic.State{At: time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)},
	}
	out := formatAgentStatus(cfg, diag)
	for _, want := range []string{"统计网卡：eth0", "默认路由网卡：eth0", "RX：123", "TX：456", "最近上报：2026-06-04T12:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected status to contain %q, got %s", want, out)
		}
	}
}

func TestFormatAgentConfigCheckShowsProcNetDevAndWarning(t *testing.T) {
	cfg := config.AgentConfig{Interface: "eth1"}
	diag := traffic.Diagnostics{
		ProcNetDevReadable: true,
		SelectedIface:      "eth1",
		RouteIface:         "eth0",
		Warning:            "⚠️ 当前统计网卡 eth1 与默认路由网卡 eth0 不一致，请检查 Agent 配置。",
	}
	out := formatAgentConfigCheck(cfg, diag)
	for _, want := range []string{"/proc/net/dev：可读取", "默认路由网卡：eth0", "统计网卡：eth1", "当前统计网卡 eth1 与默认路由网卡 eth0 不一致"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected config-check to contain %q, got %s", want, out)
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
