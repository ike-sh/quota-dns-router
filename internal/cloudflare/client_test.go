package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateDNSRecord(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/z1/dns_records/r1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"id":"r1","type":"A","name":"hk.example.com","content":"198.51.100.10","ttl":120,"proxied":false}}`))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.baseURL = server.URL
	err := client.UpdateDNSRecord(context.Background(), "token", "z1", "r1", "hk.example.com", "198.51.100.10", 120, false)
	if err != nil {
		t.Fatal(err)
	}
}
