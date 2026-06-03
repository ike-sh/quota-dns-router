package master

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

func TestCloudflarePanelShowsButtons(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	if err := controller.handleText(context.Background(), 1, "/cf"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"☁️ Cloudflare 配置", "配置/更新 Token", "选择 Zone", "查看当前配置"} {
		if !rec.contains(want) {
			t.Fatalf("expected payload to contain %q, got %v", want, rec.payloads)
		}
	}
	if len(rec.messages) != 1 {
		t.Fatalf("expected exactly one Cloudflare panel message, got %d: %v", len(rec.messages), rec.messages)
	}
}

func TestCloudflareTokenCallbackEntersPending(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	if err := controller.handleCallback(context.Background(), 1, "cf_token"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingCloudflareToken {
		t.Fatalf("expected pending %q, got %q", pendingCloudflareToken, controller.sessions[1])
	}
	if !rec.contains("请发送 Cloudflare API Token") {
		t.Fatalf("expected token prompt, got %v", rec.messages)
	}
}

func TestSwitchingWizardShowsNotice(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	controller.setSession(1, pendingMasterURL)
	if err := controller.handleCallback(context.Background(), 1, "cf_token"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingCloudflareToken {
		t.Fatalf("expected pending token after switch, got %q", controller.sessions[1])
	}
	if !rec.contains("已切换到新的配置流程") {
		t.Fatalf("expected switch notice, got %v", rec.messages)
	}
}

func TestCloudflareTokenErrorKeepsPendingAndMasksToken(t *testing.T) {
	rawToken := "cf_secret_token_123456"
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{zoneErr: cloudflare403Err()})
	if err := controller.handleCallback(context.Background(), 1, "cf_token"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleText(context.Background(), 1, rawToken); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingCloudflareToken {
		t.Fatalf("expected pending token after failure, got %q", controller.sessions[1])
	}
	if strings.Contains(strings.Join(rec.payloads, "\n"), rawToken) {
		t.Fatalf("expected raw token to stay hidden, payloads=%v", rec.payloads)
	}
	if !rec.contains("查询 Zone 失败") {
		t.Fatalf("expected zone lookup failure message, got %v", rec.messages)
	}
}

func TestCloudflareTokenSuccessShowsZoneButtons(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		zones: []cloudflare.Zone{
			{ID: "zone-1", Name: "example.com"},
			{ID: "zone-2", Name: "example.net"},
		},
	})
	if err := controller.handleCallback(context.Background(), 1, "cf_token"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleText(context.Background(), 1, "cf_secret_token_123456"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingCloudflareZoneSelect {
		t.Fatalf("expected pending zone selection, got %q", controller.sessions[1])
	}
	for _, want := range []string{"example.com", "example.net", "手动输入 Zone Name"} {
		if !rec.contains(want) {
			t.Fatalf("expected payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestCloudflareZonePickSavesZone(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		zones: []cloudflare.Zone{
			{ID: "zone-1", Name: "example.com"},
			{ID: "zone-2", Name: "example.net"},
		},
	})
	ctx := context.Background()
	if err := controller.handleCallback(ctx, 1, "cf_token"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleText(ctx, 1, "cf_secret_token_123456"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "cf_zone_pick:1"); err != nil {
		t.Fatal(err)
	}
	token, zoneName, zoneID, err := controller.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || zoneName != "example.net" || zoneID != "zone-2" {
		t.Fatalf("unexpected cloudflare defaults: token=%q zone=%q zoneID=%q", token, zoneName, zoneID)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending cleared after zone save")
	}
	if !rec.contains("配置 DNS") {
		t.Fatalf("expected next-step DNS button, got %v", rec.payloads)
	}
}

func TestCloudflareCommandCompatStillWorks(t *testing.T) {
	controller, _ := newTestTelegramControllerWithDNS(t, fakeDNS{zoneID: "zone-1"})
	ctx := context.Background()
	if err := controller.handleText(ctx, 1, "/cf cf_secret_token_123456 example.com"); err != nil {
		t.Fatal(err)
	}
	_, zoneName, zoneID, err := controller.Store.GetCloudflareDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if zoneName != "example.com" || zoneID != "zone-1" {
		t.Fatalf("unexpected cloudflare config: zone=%q zoneID=%q", zoneName, zoneID)
	}
}

func TestGroupsWizardCreatesGroup(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	if err := controller.handleCallback(ctx, 1, "groups_new"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingGroupName {
		t.Fatalf("expected pending group name, got %q", controller.sessions[1])
	}
	if err := controller.handleText(ctx, 1, "hk"); err != nil {
		t.Fatal(err)
	}
	group, err := controller.Store.GetGroupByName(ctx, "hk")
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "hk" {
		t.Fatalf("unexpected group: %+v", group)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending cleared after group create")
	}
	if !rec.contains("添加节点") {
		t.Fatalf("expected next-step node button, got %v", rec.payloads)
	}
}

func TestNodesWizardCreatesNode(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { return controller.handleCallback(ctx, 1, "nodes_add") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_group:"+group.ID) },
		func() error { return controller.handleText(ctx, 1, "hk-01") },
		func() error { return controller.handleText(ctx, 1, "1.1.1.1") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_quota_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_threshold_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_mode:both") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_reset_day_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_priority_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_confirm") },
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	node, err := controller.Store.GetNodeByName(ctx, "hk-01")
	if err != nil {
		t.Fatal(err)
	}
	if node.GroupID != group.ID || node.PublicIP != "1.1.1.1" || node.Priority != defaultNodePriority {
		t.Fatalf("unexpected node: %+v", node)
	}
	if node.ThresholdPercent != 80 || node.ResetDay != 1 || node.TrafficMode != db.TrafficModeBoth {
		t.Fatalf("unexpected node defaults: %+v", node)
	}
	for _, want := range []string{"配置 DNS", "继续生成 Agent 命令"} {
		if !rec.contains(want) {
			t.Fatalf("expected next-step button %q, got %v", want, rec.payloads)
		}
	}
}

func TestNodesWizardCreatesNodeWithDNSPrefersAgent(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Store.SaveCloudflareDefaults(ctx, "cf_secret_token_123456", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", 120, false, true); err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { return controller.handleCallback(ctx, 1, "nodes_add") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_group:"+group.ID) },
		func() error { return controller.handleText(ctx, 1, "hk-01") },
		func() error { return controller.handleText(ctx, 1, "1.1.1.1") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_quota_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_threshold_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_mode:both") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_reset_day_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_priority_default") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_confirm") },
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	if !rec.contains("生成 Agent 安装命令") {
		t.Fatalf("expected next-step agent button, got %v", rec.payloads)
	}
	if rec.contains("继续生成 Agent 命令") {
		t.Fatalf("did not expect DNS-first agent fallback buttons when DNS already exists: %v", rec.payloads)
	}
}

func TestDNSWizardSavesExistingRecord(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		record: cloudflare.DNSRecord{
			ID:      "rec-1",
			Type:    "A",
			Name:    "hk.example.com",
			Content: "1.1.1.1",
			TTL:     120,
		},
	})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := controller.Store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "1.1.1.1",
		MonthlyQuotaBytes:     1000 * 1024 * 1024 * 1024,
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
	if err := controller.Store.SaveCloudflareDefaults(ctx, "cf_secret_token_123456", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_add"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_group:"+group.ID); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingDNSRecordName {
		t.Fatalf("expected pending dns record name, got %q", controller.sessions[1])
	}
	if err := controller.handleText(ctx, 1, "hk"); err != nil {
		t.Fatal(err)
	}
	cfg, err := controller.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RecordName != "hk.example.com" || cfg.RecordID != "rec-1" {
		t.Fatalf("unexpected dns config: %+v", cfg)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending cleared after dns save")
	}
	for _, want := range []string{"匹配节点：hk-01", "Agent 安装", "agent_node:" + node.ID} {
		if !rec.contains(want) {
			t.Fatalf("expected DNS save payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestDNSWizardCreatesRecordShowsAgentNextStep(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		recordErr:    errors.New("not found"),
		anyRecordErr: errors.New("not found"),
	})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := controller.Store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "1.1.1.1",
		MonthlyQuotaBytes:     1000 * 1024 * 1024 * 1024,
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
	if err := controller.Store.SaveCloudflareDefaults(ctx, "cf_secret_token_123456", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_add"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_group:"+group.ID); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleText(ctx, 1, "hk"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_create:"+node.ID); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"✅ DNS A 记录已创建", "匹配节点：hk-01", "Agent 安装", "agent_node:" + node.ID} {
		if !rec.contains(want) {
			t.Fatalf("expected created DNS payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestPolicyThresholdWizardUpdatesPolicy(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	if err := controller.handleCallback(ctx, 1, "policy_threshold"); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingPolicyValue {
		t.Fatalf("expected pending policy value, got %q", controller.sessions[1])
	}
	if err := controller.handleText(ctx, 1, "85"); err != nil {
		t.Fatal(err)
	}
	policy, err := controller.Store.GetPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultThresholdPercent != 85 {
		t.Fatalf("unexpected threshold: %+v", policy)
	}
	if !rec.contains("✅ 策略已更新") {
		t.Fatalf("expected policy update message, got %v", rec.messages)
	}
}

func TestAgentWizardWarnsWhenDNSMissing(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	if err := controller.Store.SetMasterPublicURL(ctx, "https://master.example.com"); err != nil {
		t.Fatal(err)
	}
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := controller.Store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-01",
		PublicIP:              "1.1.1.1",
		MonthlyQuotaBytes:     1000 * 1024 * 1024 * 1024,
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
	if err := controller.handleCallback(ctx, 1, "agent"); err != nil {
		t.Fatal(err)
	}
	if !rec.contains("hk-01") {
		t.Fatalf("expected node button in payloads: %v", rec.payloads)
	}
	if err := controller.handleCallback(ctx, 1, "agent_node:"+node.ID); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"install-agent.sh", "--join", "30 分钟", "当前分组还没有 DNS A 记录", "节点：hk-01", "分组：hk", "Master：https://master.example.com"} {
		if !rec.contains(want) {
			t.Fatalf("expected install command payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func newTestTelegramControllerWithDNS(t *testing.T, dns DNSProvider) (*TelegramController, *recordingTelegramClient) {
	t.Helper()
	store := testMasterStore(t)
	ctx := context.Background()
	if err := store.SetSetting(ctx, settingSuggestedPublicAPIURL, "http://5.6.7.8:8080"); err != nil {
		t.Fatal(err)
	}
	rec := &recordingTelegramClient{}
	bot := telegram.NewBot("token", 123, rec)
	return NewTelegramController(bot, store, "http://127.0.0.1:8080", time.Second, dns), rec
}

func cloudflare403Err() error {
	return &httpErrString{text: "Cloudflare API 请求失败: 403 Forbidden"}
}

type httpErrString struct {
	text string
}

func (e *httpErrString) Error() string {
	return e.text
}
