package master

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"quota-dns-router-go/internal/db"
)

func TestServeStatusAPIWithoutToken(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, _ := store.CreateGroup(ctx, "hk", 600)
	_, _ = store.CreateNode(ctx, nodeFixture(group.ID, "hk-01", "203.0.113.10"))

	srv := HTTPServer{Store: store, PublicAPIURL: "http://127.0.0.1:8080"}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	srv.serveStatusAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp statusAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Summary.GroupCount != 1 || resp.Summary.NodeCount != 1 {
		t.Fatalf("unexpected summary %+v", resp.Summary)
	}
}

func TestServeStatusAPIRequiresTokenWhenConfigured(t *testing.T) {
	store := testMasterStore(t)
	srv := HTTPServer{Store: store, StatusReadonlyToken: "secret-token"}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	srv.serveStatusAPI(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	srv.serveStatusAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestServeStatusUI(t *testing.T) {
	store := testMasterStore(t)
	srv := HTTPServer{Store: store}
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.serveStatusUI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !containsString(rec.Body.String(), "quota-dns-router") {
		t.Fatal("expected html body")
	}
}

func nodeFixture(groupID, name, ip string) db.Node {
	return db.Node{
		GroupID:               groupID,
		Name:                  name,
		PublicIP:              ip,
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	}
}
