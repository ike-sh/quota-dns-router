package master

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"quota-dns-router-go/internal/api"
	"quota-dns-router-go/internal/db"
)

func TestReportNotifiesTrafficModeMismatch(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, _ := store.CreateGroup(ctx, "hk", 600)
	node, _ := store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "203.0.113.10",
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeRX,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	code, _ := store.GenerateJoinCode(ctx, node.ID, time.Hour)
	join, _ := store.RedeemJoinCode(ctx, code)

	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, nil)
	srv := HTTPServer{Store: store, Service: svc}

	body, _ := json.Marshal(api.AgentReportRequest{
		AgentID:      join.AgentID,
		TrafficMode:  "tx",
		RXBytesTotal: 100,
		TXBytesTotal: 50,
		ReportedAt:   time.Now().UTC(),
		AgentVersion: "test",
		Status:       "online",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agent/report", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+join.AgentToken)
	rec := httptest.NewRecorder()
	srv.report(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if count := countMessagesContaining(notifier.messages, "traffic_mode"); count != 1 {
		t.Fatalf("expected one traffic_mode notification, got %d messages=%v", count, notifier.messages)
	}
	if count := countMessagesContaining(notifier.messages, "单向 TX"); count == 0 {
		t.Fatalf("expected reported mode in notification, got %v", notifier.messages)
	}
}
