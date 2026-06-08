package master

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/db"
)

type fakeDNS struct {
	zones        []cloudflare.Zone
	zoneID       string
	zoneErr      error
	record       cloudflare.DNSRecord
	recordErr    error
	anyRecord    cloudflare.DNSRecord
	anyRecordErr error
	createRecord cloudflare.DNSRecord
	createErr    error
	updateCalls  *[]dnsUpdateCall
	updateErr    error
}

type dnsUpdateCall struct {
	Token      string
	ZoneID     string
	RecordID   string
	RecordName string
	IP         string
	TTL        int
	Proxied    bool
}

func (f fakeDNS) ListZones(ctx context.Context, token string) ([]cloudflare.Zone, error) {
	return f.zones, f.zoneErr
}

func (f fakeDNS) LookupZoneID(ctx context.Context, token, zoneName string) (string, error) {
	return f.zoneID, f.zoneErr
}

func (f fakeDNS) LookupDNSRecord(ctx context.Context, token, zoneID, recordName string) (cloudflare.DNSRecord, error) {
	return f.record, f.recordErr
}

func (f fakeDNS) LookupDNSRecordWithType(ctx context.Context, token, zoneID, recordName, recordType string) (cloudflare.DNSRecord, error) {
	if f.recordErr != nil {
		return f.record, f.recordErr
	}
	if recordType != "" && f.record.Type != "" && f.record.Type != recordType {
		return cloudflare.DNSRecord{}, fmt.Errorf("record type mismatch: want %s got %s", recordType, f.record.Type)
	}
	return f.record, nil
}

func (f fakeDNS) LookupDNSRecordAnyType(ctx context.Context, token, zoneID, recordName string) (cloudflare.DNSRecord, error) {
	return f.anyRecord, f.anyRecordErr
}

func (f fakeDNS) CreateDNSRecord(ctx context.Context, token, zoneID, recordName, ip string, ttl int, proxied bool) (cloudflare.DNSRecord, error) {
	return f.CreateDNSRecordWithType(ctx, token, zoneID, recordName, ip, "A", ttl, proxied)
}

func (f fakeDNS) CreateDNSRecordWithType(ctx context.Context, token, zoneID, recordName, ip, recordType string, ttl int, proxied bool) (cloudflare.DNSRecord, error) {
	_ = recordType
	if f.createRecord.ID != "" || f.createErr != nil {
		return f.createRecord, f.createErr
	}
	return cloudflare.DNSRecord{ID: "created", Type: "A", Name: recordName, Content: ip, TTL: ttl, Proxied: proxied}, nil
}

func (f fakeDNS) UpdateDNSRecord(ctx context.Context, token, zoneID, recordID, recordName, ip string, ttl int, proxied bool) error {
	return f.UpdateDNSRecordWithType(ctx, token, zoneID, recordID, recordName, ip, "A", ttl, proxied)
}

func (f fakeDNS) UpdateDNSRecordWithType(ctx context.Context, token, zoneID, recordID, recordName, ip, recordType string, ttl int, proxied bool) error {
	_ = recordType
	if f.updateCalls != nil {
		*f.updateCalls = append(*f.updateCalls, dnsUpdateCall{
			Token:      token,
			ZoneID:     zoneID,
			RecordID:   recordID,
			RecordName: recordName,
			IP:         ip,
			TTL:        ttl,
			Proxied:    proxied,
		})
	}
	return f.updateErr
}

func TestCloudflareSummaryOutput(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	if err := store.SaveCloudflareDefaults(ctx, "cf_secret_abcd", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	summary, err := BuildCloudflareSummary(ctx, store, fakeDNS{zoneID: "zone-1"})
	if err != nil {
		t.Fatal(err)
	}
	text := FormatCloudflareSummary(summary)
	if summary.TokenMasked == "cf_secret_abcd" {
		t.Fatal("expected masked token")
	}
	if !containsString(text, "Zone 已验证") {
		t.Fatalf("expected zone verified text: %s", text)
	}
}

func TestDNSSummaryOutput(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, _ := store.CreateGroup(ctx, "hk", 600)
	_, _ = store.CreateNode(ctx, db.Node{
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
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "", "A", 60, false, true)
	items, err := BuildDNSSummaries(ctx, store, fakeDNS{
		record: cloudflare.DNSRecord{ID: "r1", Type: "A", Name: "hk.example.com", Content: "203.0.113.10", TTL: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := FormatDNSSummaries(items)
	if !containsString(text, "匹配节点：hk-01") {
		t.Fatalf("expected matched node: %s", text)
	}
	if !containsString(text, "记录类型：A") {
		t.Fatalf("expected record type in summary: %s", text)
	}
}

func TestDNSSummaryOutputAAAA(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, _ := store.CreateGroup(ctx, "ipv6", 600)
	_, _ = store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "ipv6-01",
		PublicIP:              "2001:db8::1",
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
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "ipv6.example.com", "rec-aaaa", "AAAA", 60, false, true)
	items, err := BuildDNSSummaries(ctx, store, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-aaaa", Type: "AAAA", Name: "ipv6.example.com", Content: "2001:db8::1", TTL: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := FormatDNSSummaries(items)
	for _, want := range []string{"记录类型：AAAA", "当前 AAAA 记录：2001:db8::1", "匹配节点：ipv6-01"} {
		if !containsString(text, want) {
			t.Fatalf("expected %q in summary, got %s", want, text)
		}
	}
}

func TestNodeDiagnosticsThreshold(t *testing.T) {
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
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	_ = store.BindAgentToNode(ctx, node.ID, "agent-1")
	_ = store.SaveAgentReport(ctx, db.AgentReport{
		AgentID:      "agent-1",
		Hostname:     "host",
		PublicIP:     "203.0.113.10",
		Iface:        "eth0",
		RXBytesTotal: 900,
		TXBytesTotal: 0,
		RXDelta:      900,
		TXDelta:      0,
		ReportedAt:   time.Now(),
		AgentVersion: "test",
		Status:       "online",
	})
	items, err := BuildNodeDiagnostics(ctx, store, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].ReachedThreshold {
		t.Fatalf("expected threshold reached: %+v", items)
	}
	text := FormatNodeDiagnostics(items)
	if !containsString(text, "已达到阈值") {
		t.Fatalf("expected threshold text: %s", text)
	}
}

func TestGroupDiagnosticsAvailableTargets(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, _ := store.CreateGroup(ctx, "hk", 600)
	node1, _ := store.CreateNode(ctx, db.Node{
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
	node2, _ := store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-02",
		PublicIP:              "198.51.100.10",
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              20,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
		Online:                true,
	})
	_ = store.UpdateGroupCurrentNode(ctx, group.ID, node1.ID)
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "r1", "A", 60, false, true)
	_ = store.BindAgentToNode(ctx, node2.ID, "agent-2")
	_ = store.SaveAgentReport(ctx, db.AgentReport{
		AgentID:      "agent-2",
		Hostname:     "host",
		PublicIP:     "198.51.100.10",
		Iface:        "eth0",
		RXBytesTotal: 100,
		TXBytesTotal: 0,
		RXDelta:      100,
		TXDelta:      0,
		ReportedAt:   time.Now(),
		AgentVersion: "test",
		Status:       "online",
	})
	items, err := BuildGroupDiagnostics(ctx, store, time.Now(), fakeDNS{
		record: cloudflare.DNSRecord{ID: "r1", Type: "A", Name: "hk.example.com", Content: "203.0.113.10", TTL: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AvailableTargetCount != 1 {
		t.Fatalf("expected one target: %+v", items)
	}
	text := FormatGroupDiagnostics(items)
	if !containsString(text, "可用切换目标：1") {
		t.Fatalf("expected available target count: %s", text)
	}
}

func containsString(s, sub string) bool {
	return strings.Contains(s, sub)
}
