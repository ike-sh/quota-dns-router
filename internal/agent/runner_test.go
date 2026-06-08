package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/api"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/traffic"
)

func TestRenderAgentEnvIncludesTrafficMode(t *testing.T) {
	env := RenderAgentEnv(api.JoinResponse{
		MasterAPIURL:    "http://203.0.113.10:8080",
		AgentID:         "agent-1",
		AgentToken:      "secret",
		NodeName:        "hk-01",
		Interface:       "eth0",
		IntervalSeconds: 60,
		TrafficMode:     "rx",
	}, "/tmp/state.json")
	if !strings.Contains(env, "QDR_AGENT_TRAFFIC_MODE=rx") {
		t.Fatalf("expected traffic mode in env, got %s", env)
	}
}

func TestRenderAgentEnvDefaultsTrafficMode(t *testing.T) {
	env := RenderAgentEnv(api.JoinResponse{
		MasterAPIURL:    "http://203.0.113.10:8080",
		AgentID:         "agent-1",
		AgentToken:      "secret",
		NodeName:        "hk-01",
		Interface:       "auto",
		IntervalSeconds: 60,
	}, "")
	if !strings.Contains(env, "QDR_AGENT_TRAFFIC_MODE=rx+tx") {
		t.Fatalf("expected default traffic mode, got %s", env)
	}
}

func TestPostReportSendsTrafficMode(t *testing.T) {
	var got api.AgentReportRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Fatalf("unexpected auth %q", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"accepted":true,"message":"ok"}`))
	}))
	defer srv.Close()

	runner := NewRunner(config.AgentConfig{
		MasterAPIURL: srv.URL,
		AgentID:      "agent-1",
		AgentToken:   "test-token",
		TrafficMode:  "tx",
	})
	report := api.AgentReportRequest{
		AgentID:      "agent-1",
		TrafficMode:  "tx",
		ReportedAt:   time.Now().UTC(),
		Status:       "online",
		AgentVersion: "test",
	}
	if err := runner.PostReport(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	if got.TrafficMode != "tx" {
		t.Fatalf("expected tx mode, got %q", got.TrafficMode)
	}
}

func TestBuildSampleComputesDelta(t *testing.T) {
	current := traffic.Snapshot{Iface: "eth0", RX: 200, TX: 300}
	previous := traffic.State{Last: traffic.Snapshot{Iface: "eth0", RX: 100, TX: 250}}
	sample := traffic.BuildSample(current, previous)
	if sample.RXDelta != 100 || sample.TXDelta != 50 {
		t.Fatalf("unexpected deltas rx=%d tx=%d", sample.RXDelta, sample.TXDelta)
	}
}

func TestJoinDecodesTrafficMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"master_api_url":"http://203.0.113.10:8080","agent_id":"a1","agent_token":"tok","node_name":"hk","traffic_mode":"rx"}`)
	}))
	defer srv.Close()
	resp, err := Join(context.Background(), srv.URL, "code", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if resp.TrafficMode != "rx" {
		t.Fatalf("expected rx, got %q", resp.TrafficMode)
	}
}
