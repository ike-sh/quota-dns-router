package master

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/db"
)

func TestSelectTarget(t *testing.T) {
	now := time.Now()
	policy := db.DefaultPolicy()
	nodes := []db.NodeUsage{
		{Node: db.Node{ID: "current", Name: "hk-01", Priority: 10, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 70}},
		{Node: db.Node{ID: "n2", Name: "hk-02", Priority: 1, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 40}},
		{Node: db.Node{ID: "n3", Name: "hk-03", Priority: 1, Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80}, UsageSnapshot: db.UsageSnapshot{UsedBytes: 20}},
	}
	target, ok := SelectTarget("current", nodes, policy, now)
	if !ok {
		t.Fatal("expected target")
	}
	if target.ID != "n3" {
		t.Fatalf("expected n3, got %s", target.ID)
	}
}

func TestCooldown(t *testing.T) {
	group := db.Group{
		SwitchCooldownSeconds: 600,
		LastSwitchAt: sql.NullTime{
			Valid: true,
			Time:  time.Now().Add(-5 * time.Minute),
		},
	}
	if !inCooldown(group, time.Now()) {
		t.Fatal("expected cooldown")
	}
}

func TestReasonForSwitchThreshold(t *testing.T) {
	policy := db.DefaultPolicy()
	current := db.NodeUsage{
		Node:          db.Node{Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80},
		UsageSnapshot: db.UsageSnapshot{UsedBytes: 80},
	}
	if reason := reasonForSwitch(current, policy, time.Now()); reason == "" {
		t.Fatal("expected threshold reason")
	}
}

func TestReasonForSwitchThresholdUsesTrafficOffset(t *testing.T) {
	policy := db.DefaultPolicy()
	current := db.NodeUsage{
		Node:          db.Node{Enabled: true, AutoSwitch: true, Online: true, MonthlyQuotaBytes: 100, ThresholdPercent: 80, TrafficOffsetBytes: 75},
		UsageSnapshot: db.UsageSnapshot{AgentUsedBytes: 5, UsedBytes: 80},
	}
	if reason := reasonForSwitch(current, policy, time.Now()); reason != switchReasonThreshold {
		t.Fatalf("expected threshold reason from offset + agent usage, got %q", reason)
	}
}

func TestThresholdNotificationDedupesWithinCycleAndResetsNextCycle(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, oldNode, newNode := createSwitchFixture(t, ctx, store)
	if err := store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true); err != nil {
		t.Fatal(err)
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy.NotifyOnly = true
	if err := store.SavePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	_ = store.BindAgentToNode(ctx, oldNode.ID, "agent-old")
	_ = store.BindAgentToNode(ctx, newNode.ID, "agent-new")
	_ = store.SaveAgentReport(ctx, reportFor("agent-old", oldNode.PublicIP, 900, now))
	_ = store.SaveAgentReport(ctx, reportFor("agent-new", newNode.PublicIP, 100, now))
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-1", Type: "A", Name: "hk.example.com", Content: oldNode.PublicIP, TTL: 60},
	})
	svc.Now = func() time.Time { return now }
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "流量阈值触发"); got != 1 {
		t.Fatalf("expected one threshold notification in same cycle, got %d: %v", got, notifier.messages)
	}
	nextCycle := time.Date(2026, 7, 4, 1, 0, 0, 0, time.UTC)
	_ = store.SaveAgentReport(ctx, reportFor("agent-old", oldNode.PublicIP, 900, nextCycle))
	_ = store.SaveAgentReport(ctx, reportFor("agent-new", newNode.PublicIP, 100, nextCycle))
	svc.Now = func() time.Time { return nextCycle }
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "流量阈值触发"); got != 2 {
		t.Fatalf("expected threshold notification to trigger in next cycle, got %d: %v", got, notifier.messages)
	}
}

func TestNoTargetNotificationDedupes(t *testing.T) {
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
	now := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true)
	_ = store.BindAgentToNode(ctx, node.ID, "agent-1")
	_ = store.SaveAgentReport(ctx, reportFor("agent-1", node.PublicIP, 900, now))
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-1", Type: "A", Name: "hk.example.com", Content: node.PublicIP, TTL: 60},
	})
	svc.Now = func() time.Time { return now }
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "没有可用切换目标"); got != 1 {
		t.Fatalf("expected one no-target notification, got %d: %v", got, notifier.messages)
	}
}

func TestOfflineAndRecoveryNotifications(t *testing.T) {
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
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true)
	_ = store.BindAgentToNode(ctx, node.ID, "agent-1")
	firstReport := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	_ = store.SaveAgentReport(ctx, reportFor("agent-1", node.PublicIP, 100, firstReport))
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{})
	svc.Now = func() time.Time { return firstReport.Add(10 * time.Minute) }
	if err := svc.CheckOfflineNodes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckOfflineNodes(ctx); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "节点离线"); got != 1 {
		t.Fatalf("expected one offline notification, got %d: %v", got, notifier.messages)
	}
	previous, err := store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	recoveredAt := firstReport.Add(11 * time.Minute)
	_ = store.SaveAgentReport(ctx, reportFor("agent-1", node.PublicIP, 100, recoveredAt))
	current, err := store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return recoveredAt }
	if err := svc.HandleAgentRecovery(ctx, previous, current); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "节点恢复在线"); got != 1 {
		t.Fatalf("expected one recovery notification, got %d: %v", got, notifier.messages)
	}
}

func TestNeverReportedNodeDoesNotSendOfflineNotification(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, err := store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateNode(ctx, db.Node{
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
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{})
	if err := svc.CheckOfflineNodes(ctx); err != nil {
		t.Fatal(err)
	}
	if len(notifier.messages) != 0 {
		t.Fatalf("expected no offline notification for never-reported node, got %v", notifier.messages)
	}
}

func TestResolveCurrentNodeReturnsErrorWhenDNSDoesNotMatchNode(t *testing.T) {
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
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	cfg, err := store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true)
	if err != nil {
		t.Fatal(err)
	}
	usages, err := store.ListNodeUsagesByGroup(ctx, group.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, nil, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-1", Type: "A", Name: "hk.example.com", Content: "198.51.100.99", TTL: 60},
	})
	_, err = svc.ResolveCurrentNode(ctx, group, cfg, usages)
	var unresolved *CurrentNodeUnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("expected CurrentNodeUnresolvedError, got %v", err)
	}
	if unresolved.DNSIP != "198.51.100.99" {
		t.Fatalf("unexpected dns ip %q", unresolved.DNSIP)
	}
	_ = node
}

func TestBuildDecisionNotifiesOnDNSMismatch(t *testing.T) {
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
	_ = store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1")
	_, _ = store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true)
	_ = store.BindAgentToNode(ctx, node.ID, "agent-1")
	now := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	_ = store.SaveAgentReport(ctx, reportFor("agent-1", node.PublicIP, 100, now))
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-1", Type: "A", Name: "hk.example.com", Content: "198.51.100.99", TTL: 60},
	})
	svc.Now = func() time.Time { return now }
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "未匹配任何已配置节点"); got != 1 {
		t.Fatalf("expected one dns mismatch notification, got %d: %v", got, notifier.messages)
	}
}

func TestMaintenanceModeSkipsAutoSwitch(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, oldNode, newNode := createSwitchFixture(t, ctx, store)
	if err := store.SaveCloudflareDefaults(ctx, "token", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true); err != nil {
		t.Fatal(err)
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	policy.MaintenanceMode = true
	if err := store.SavePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	_ = store.BindAgentToNode(ctx, oldNode.ID, "agent-old")
	_ = store.BindAgentToNode(ctx, newNode.ID, "agent-new")
	_ = store.SaveAgentReport(ctx, reportFor("agent-old", oldNode.PublicIP, 900, now))
	_ = store.SaveAgentReport(ctx, reportFor("agent-new", newNode.PublicIP, 100, now))
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{
		record: cloudflare.DNSRecord{ID: "rec-1", Type: "A", Name: "hk.example.com", Content: oldNode.PublicIP, TTL: 60},
	})
	svc.Now = func() time.Time { return now }
	if err := svc.HandleGroup(ctx, group.ID); err != nil {
		t.Fatal(err)
	}
	if got := countMessagesContaining(notifier.messages, "DNS 自动切换成功"); got != 0 {
		t.Fatalf("expected no auto switch during maintenance, got %v", notifier.messages)
	}
}

func TestSwitchFailureNotificationIsSanitized(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, oldNode, newNode := createSwitchFixture(t, ctx, store)
	if err := store.SaveCloudflareDefaults(ctx, "cf_secret_abcd", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", "A", 60, false, true); err != nil {
		t.Fatal(err)
	}
	notifier := &fakeNotifier{}
	svc := NewService(store, notifier, fakeDNS{updateErr: errors.New("Cloudflare failed token=cf_secret_abcd")})
	decision := SwitchDecision{
		Group:       group,
		Config:      db.CloudflareConfig{GroupID: group.ID, APIToken: "cf_secret_abcd", ZoneName: "example.com", ZoneID: "zone-1", RecordName: "hk.example.com", RecordID: "rec-1", TTL: 60},
		Current:     db.NodeUsage{Node: oldNode},
		Target:      db.NodeUsage{Node: newNode},
		TriggerType: db.SwitchTriggerThreshold,
		Reason:      switchReasonThreshold,
		Triggered:   true,
	}
	if err := svc.ExecuteSwitch(ctx, decision); err == nil {
		t.Fatal("expected switch failure")
	}
	payload := strings.Join(notifier.messages, "\n")
	if !strings.Contains(payload, "DNS 自动切换失败") {
		t.Fatalf("expected failure notification, got %v", notifier.messages)
	}
	if strings.Contains(payload, "cf_secret_abcd") {
		t.Fatalf("expected token to be sanitized, got %s", payload)
	}
}

func TestNotificationFailureRecordsLastErrorWithoutBlocking(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	notifier := &fakeNotifier{err: errors.New("telegram token=bot_secret failed")}
	svc := NewService(store, notifier, fakeDNS{})
	if err := svc.notify(ctx, "test", "target", "hello", ""); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetLastError(ctx, errorKeyNotification)
	if err != nil {
		t.Fatal(err)
	}
	if item.Message == "" {
		t.Fatal("expected notification failure last_error")
	}
	if strings.Contains(item.Message, "bot_secret") {
		t.Fatalf("expected notification error to be sanitized, got %q", item.Message)
	}
}

func TestHandleTrafficModeMismatchNotifiesOnce(t *testing.T) {
	notifier := &fakeNotifier{}
	svc := NewService(testMasterStore(t), notifier, nil)
	ctx := context.Background()

	svc.HandleTrafficModeMismatch(ctx, "agent-1", "hk-01", db.TrafficModeTX, db.TrafficModeRX)
	svc.HandleTrafficModeMismatch(ctx, "agent-1", "hk-01", db.TrafficModeTX, db.TrafficModeRX)

	if count := countMessagesContaining(notifier.messages, "traffic_mode"); count != 1 {
		t.Fatalf("expected notifyOnce semantics, got %d messages=%v", count, notifier.messages)
	}
}

func reportFor(agentID, publicIP string, used int64, at time.Time) db.AgentReport {
	return db.AgentReport{
		AgentID:      agentID,
		Hostname:     "host",
		PublicIP:     publicIP,
		Iface:        "eth0",
		RXBytesTotal: used,
		TXBytesTotal: 0,
		RXDelta:      used,
		TXDelta:      0,
		ReportedAt:   at,
		AgentVersion: "test",
		Status:       "online",
	}
}

func countMessagesContaining(messages []string, needle string) int {
	count := 0
	for _, message := range messages {
		if strings.Contains(message, needle) {
			count++
		}
	}
	return count
}

type fakeNotifier struct {
	messages []string
	err      error
}

func (n *fakeNotifier) SendAdminMessage(ctx context.Context, text string) error {
	n.messages = append(n.messages, text)
	return n.err
}
