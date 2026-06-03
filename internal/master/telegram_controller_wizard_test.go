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

func TestCallbackUsesEditMessageAndAnswersQuery(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	err := controller.handleUpdate(context.Background(), telegram.Update{
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb-1",
			From: telegram.User{ID: 123},
			Message: telegram.Message{
				MessageID: 88,
				Chat:      telegram.Chat{ID: 1},
			},
			Data: "help",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.countPath("/answerCallbackQuery") != 1 {
		t.Fatalf("expected answerCallbackQuery once, got paths=%v", rec.paths)
	}
	if rec.countPath("/editMessageText") != 1 {
		t.Fatalf("expected editMessageText once, got paths=%v", rec.paths)
	}
	if rec.countPath("/sendMessage") != 0 {
		t.Fatalf("did not expect fallback sendMessage, got paths=%v", rec.paths)
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
	for _, want := range []string{"将使用默认流量策略", "修改流量策略"} {
		if !rec.contains(want) {
			t.Fatalf("expected default policy confirmation %q, got %v", want, rec.payloads)
		}
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

func TestNodesWizardCustomizesPolicyBeforeCreate(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []func() error{
		func() error { return controller.handleCallback(ctx, 1, "nodes_add") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_group:"+group.ID) },
		func() error { return controller.handleText(ctx, 1, "hk-02") },
		func() error { return controller.handleText(ctx, 1, "2.2.2.2") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_customize_policy") },
		func() error { return controller.handleText(ctx, 1, "2000GB") },
		func() error { return controller.handleText(ctx, 1, "85") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_mode:rx") },
		func() error { return controller.handleText(ctx, 1, "2") },
		func() error { return controller.handleText(ctx, 1, "15") },
		func() error { return controller.handleCallback(ctx, 1, "nodes_confirm") },
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	node, err := controller.Store.GetNodeByName(ctx, "hk-02")
	if err != nil {
		t.Fatal(err)
	}
	if node.MonthlyQuotaBytes != 2000*1024*1024*1024 || node.ThresholdPercent != 85 || node.ResetDay != 2 || node.Priority != 15 || node.TrafficMode != db.TrafficModeRX {
		t.Fatalf("unexpected custom node policy: %+v", node)
	}
	if !rec.contains("请确认节点配置") {
		t.Fatalf("expected confirmation message, got %v", rec.messages)
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

func TestDNSWizardAutoCreatesDefaultGroupWhenMissing(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	ctx := context.Background()
	if err := controller.Store.SaveCloudflareDefaults(ctx, "cf_secret_token_123456", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "dns_add"); err != nil {
		t.Fatal(err)
	}
	group, err := controller.Store.GetGroupByName(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if group.Name != "default" {
		t.Fatalf("unexpected default group: %+v", group)
	}
	if controller.sessions[1] != pendingDNSRecordName {
		t.Fatalf("expected pending dns record prompt, got %q", controller.sessions[1])
	}
	for _, want := range []string{"已自动创建默认分组 default", "当前分组：default"} {
		if !rec.contains(want) {
			t.Fatalf("expected auto default group hint %q, got %v", want, rec.payloads)
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

func TestDNSWizardSavesPendingRecordWhenNoNodes(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		recordErr:    errors.New("not found"),
		anyRecordErr: errors.New("not found"),
	})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
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
	cfg, err := controller.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RecordName != "hk.example.com" || cfg.RecordID != "" {
		t.Fatalf("expected pending dns config, got %+v", cfg)
	}
	if controller.sessions[1] != "" {
		t.Fatalf("expected pending flow cleared after saving pending dns")
	}
	for _, want := range []string{"待绑定", "添加节点", "查看 DNS 状态"} {
		if !rec.contains(want) {
			t.Fatalf("expected pending dns payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestDNSWizardOffersRepointWhenIPDoesNotMatchNode(t *testing.T) {
	updates := []dnsUpdateCall{}
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		record: cloudflare.DNSRecord{
			ID:      "rec-1",
			Type:    "A",
			Name:    "hk.example.com",
			Content: "9.9.9.9",
			TTL:     120,
		},
		updateCalls: &updates,
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
	if controller.sessions[1] != pendingDNSFixSelect {
		t.Fatalf("expected pending dns fix flow, got %q", controller.sessions[1])
	}
	for _, want := range []string{"没有匹配任何已配置节点", "改为指向节点 hk-01 / 1.1.1.1"} {
		if !rec.contains(want) {
			t.Fatalf("expected mismatch guidance %q, got %v", want, rec.payloads)
		}
	}
	if err := controller.handleCallback(ctx, 1, "dns_repoint:"+node.ID); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected one DNS update call, got %+v", updates)
	}
	if updates[0].IP != "1.1.1.1" || updates[0].RecordID != "rec-1" {
		t.Fatalf("unexpected DNS update call: %+v", updates[0])
	}
	for _, want := range []string{"✅ DNS A 记录已更新", "旧 IP：9.9.9.9", "新 IP：1.1.1.1", "Agent 安装"} {
		if !rec.contains(want) {
			t.Fatalf("expected DNS repoint payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestPolicyPanelShowsDefaultStrategyCenter(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
	if err := controller.handleCallback(context.Background(), 1, "policy"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"⚙️ 默认流量策略", "默认重置日：1", "默认优先级：10", "这些默认值会用于新建节点", "修改默认重置日"} {
		if !rec.contains(want) {
			t.Fatalf("expected policy panel to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestNodeDetailShowsPolicyActionsAndTroubleshooting(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
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
	if err := controller.handleCallback(ctx, 1, "nodes_view:"+node.ID); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"🖥 节点详情", "修改节点策略", "查看安装排查", "DNS 匹配：否"} {
		if !rec.contains(want) {
			t.Fatalf("expected node detail payload to contain %q, got %v", want, rec.payloads)
		}
	}
}

func TestNodePolicyEditUpdatesSingleField(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{})
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
	if err := controller.handleCallback(ctx, 1, "nodes_edit_policy:"+node.ID); err != nil {
		t.Fatal(err)
	}
	if !rec.contains("修改节点策略：" + node.Name) {
		t.Fatalf("expected node policy edit panel, got %v", rec.payloads)
	}
	if err := controller.handleCallback(ctx, 1, "nodes_edit_quota:"+node.ID); err != nil {
		t.Fatal(err)
	}
	if controller.sessions[1] != pendingNodeQuota {
		t.Fatalf("expected pending quota edit, got %q", controller.sessions[1])
	}
	if err := controller.handleText(ctx, 1, "2TB"); err != nil {
		t.Fatal(err)
	}
	updated, err := controller.Store.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MonthlyQuotaBytes != 2*1024*1024*1024*1024 {
		t.Fatalf("expected quota updated to 2TB, got %+v", updated)
	}
	if updated.ThresholdPercent != 80 || updated.Priority != 10 {
		t.Fatalf("unexpected collateral policy changes: %+v", updated)
	}
	if !rec.contains("月流量已更新") {
		t.Fatalf("expected update confirmation, got %v", rec.messages)
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

func TestAgentWizardCanSendPureCommand(t *testing.T) {
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
	if err := controller.handleCallback(ctx, 1, "agent_node:"+node.ID); err != nil {
		t.Fatal(err)
	}
	if !rec.contains("显示纯命令") {
		t.Fatalf("expected copy button in agent command menu, got %v", rec.payloads)
	}
	if err := controller.handleCallback(ctx, 1, "agent_copy:"+node.ID); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(rec.messages[len(rec.messages)-1])
	for _, want := range []string{"install-agent.sh", "--join", "--master https://master.example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected pure command to contain %q, got %q", want, got)
		}
	}
}

func TestManualSwitchWritesManualTriggerHistory(t *testing.T) {
	updates := []dnsUpdateCall{}
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		record: cloudflare.DNSRecord{
			ID:      "rec-1",
			Type:    "A",
			Name:    "hk.example.com",
			Content: "1.1.1.1",
			TTL:     120,
		},
		updateCalls: &updates,
	})
	ctx := context.Background()
	group, err := controller.Store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	oldNode, err := controller.Store.CreateNode(ctx, db.Node{
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
	newNode, err := controller.Store.CreateNode(ctx, db.Node{
		GroupID:               group.ID,
		Name:                  "hk-02",
		PublicIP:              "2.2.2.2",
		MonthlyQuotaBytes:     1000 * 1024 * 1024 * 1024,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              20,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Store.SaveCloudflareDefaults(ctx, "cf_secret_token_123456", "example.com", "zone-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, "hk.example.com", "rec-1", 120, false, true); err != nil {
		t.Fatal(err)
	}
	if err := controller.Store.UpdateGroupCurrentNode(ctx, group.ID, oldNode.ID); err != nil {
		t.Fatal(err)
	}
	if err := controller.handleCallback(ctx, 1, "switch_to_node:"+newNode.ID); err != nil {
		t.Fatal(err)
	}
	if !rec.contains("请确认手动切换") {
		t.Fatalf("expected manual switch confirm page, got %v", rec.payloads)
	}
	if err := controller.handleCallback(ctx, 1, "switch_do:"+group.ID+":"+newNode.ID); err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected one dns update call, got %+v", updates)
	}
	if updates[0].IP != "2.2.2.2" || updates[0].RecordID != "rec-1" {
		t.Fatalf("unexpected dns update call: %+v", updates[0])
	}
	history, err := controller.Store.GetLatestSwitchHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if history.TriggerType != db.SwitchTriggerManual {
		t.Fatalf("expected manual trigger type, got %+v", history)
	}
	if !rec.contains("手动切换完成") {
		t.Fatalf("expected manual switch success message, got %v", rec.messages)
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
