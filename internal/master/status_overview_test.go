package master

import (
	"context"
	"strings"
	"testing"
	"time"

	"quota-dns-router-go/internal/db"
)

func TestStatusReportRecentSwitchSummary(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, oldNode, newNode := createSwitchFixture(t, ctx, store)
	if err := store.RecordSwitchHistory(ctx, group.ID, oldNode.ID, newNode.ID, "hk.example.com", "1.1.1.1", "2.2.2.2", "流量达到阈值", "success", ""); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildStatusOverview(ctx, store, "https://master.example.com", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	for _, want := range []string{"最近切换", "分组：hk", "域名：hk.example.com", "hk-01 / 1.1.1.1 -> hk-02 / 2.2.2.2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected status report to contain %q: %s", want, text)
		}
	}
}

func TestStatusReportRecentFailureSummary(t *testing.T) {
	store := testMasterStore(t)
	ctx := context.Background()
	group, oldNode, newNode := createSwitchFixture(t, ctx, store)
	if err := store.RecordSwitchHistory(ctx, group.ID, oldNode.ID, newNode.ID, "hk.example.com", "1.1.1.1", "2.2.2.2", "流量达到阈值", "failed", "Cloudflare 返回 403 token=cf_secret_abcd", "cf_secret_abcd"); err != nil {
		t.Fatal(err)
	}
	overview, err := BuildStatusOverview(ctx, store, "https://master.example.com", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras())
	if !strings.Contains(text, "最近失败") || !strings.Contains(text, "DNS 修改失败") {
		t.Fatalf("expected recent failure summary: %s", text)
	}
	if strings.Contains(text, "cf_secret_abcd") {
		t.Fatalf("expected failure reason to be redacted: %s", text)
	}
}

func TestStatusRiskPriorityOrder(t *testing.T) {
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup: SetupStatus{
			PublicAPIURL:              "http://127.0.0.1:8080",
			PublicURLWarning:          MasterPublicURLWarning("http://127.0.0.1:8080"),
			CloudflareTokenConfigured: false,
			DNSConfigCount:            1,
			AutoSwitchEnabled:         false,
		},
		DNS: []DNSSummary{{
			GroupName:     "hk",
			CurrentIP:     "9.9.9.9",
			IPMatchesNode: false,
		}},
		Groups: []GroupDiagnostic{{
			Name:                 "hk",
			NodeCount:            2,
			AvailableTargetCount: 0,
			CooldownRemaining:    "5m0s",
		}},
		Nodes: []NodeDiagnostic{
			{Name: "hk-01", GroupName: "hk", Online: true, HasReported: true, ReachedThreshold: true},
			{Name: "hk-03", GroupName: "hk", Online: false, HasReported: true},
		},
	})
	wantPending := []string{
		"配置 Master 公网地址",
		"配置 Cloudflare Token",
	}
	if len(risks.Pending) != len(wantPending) {
		t.Fatalf("expected %d pending items, got %v", len(wantPending), risks.Pending)
	}
	for i, want := range wantPending {
		if risks.Pending[i] != want {
			t.Fatalf("pending %d expected %q, got %q", i, want, risks.Pending[i])
		}
	}
	wantPrefixes := []string{
		"⚠️ hk DNS 当前 IP",
		"⚠️ hk 没有可用切换目标",
		"⚠️ 自动切换关闭",
		"⚠️ hk-01 已达到阈值",
		"⚠️ hk-03 Agent 离线",
		"⚠️ hk 当前处于 cooldown",
	}
	if len(risks.Items) != len(wantPrefixes) {
		t.Fatalf("expected %d risks, got %v", len(wantPrefixes), risks.Items)
	}
	for i, want := range wantPrefixes {
		if !strings.HasPrefix(risks.Items[i], want) {
			t.Fatalf("risk %d expected prefix %q, got %q", i, want, risks.Items[i])
		}
	}
}

func TestStatusRiskTruncation(t *testing.T) {
	var nodes []NodeDiagnostic
	for i := 0; i < 10; i++ {
		nodes = append(nodes, NodeDiagnostic{Name: "hk-offline-" + string(rune('a'+i)), Online: false, HasReported: true})
	}
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup: baseHealthyRiskSetup(),
		Nodes: nodes,
	})
	if len(risks.Items) != 8 || risks.Hidden != 2 {
		t.Fatalf("expected 8 visible and 2 hidden risks, got %+v", risks)
	}
	text := FormatStatusRiskSummary(risks)
	if !strings.Contains(text, "还有 2 条风险未显示") {
		t.Fatalf("expected truncation hint: %s", text)
	}
}

func TestStatusRiskDNSIPMismatch(t *testing.T) {
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup:      baseHealthyRiskSetup(),
		Cloudflare: CloudflareSummary{Verified: true},
		DNS: []DNSSummary{{
			GroupName:     "hk",
			CurrentIP:     "9.9.9.9",
			IPMatchesNode: false,
		}},
	})
	assertRiskContains(t, risks, "DNS 当前 IP 9.9.9.9 不匹配任何节点")
}

func TestStatusRiskNodeReachedThreshold(t *testing.T) {
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup:      baseHealthyRiskSetup(),
		Cloudflare: CloudflareSummary{Verified: true},
		Nodes:      []NodeDiagnostic{{Name: "hk-01", Online: true, ReachedThreshold: true}},
	})
	assertRiskContains(t, risks, "hk-01 已达到阈值")
}

func TestStatusRiskNodeOffline(t *testing.T) {
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup:      baseHealthyRiskSetup(),
		Cloudflare: CloudflareSummary{Verified: true},
		Nodes:      []NodeDiagnostic{{Name: "hk-03", Online: false, HasReported: true}},
	})
	assertRiskContains(t, risks, "hk-03 Agent 离线")
}

func TestStatusRiskPendingTasks(t *testing.T) {
	risks := BuildStatusRiskSummary(StatusRiskInput{
		Setup: SetupStatus{
			PublicAPIURL:              "https://master.example.com",
			PublicURLConfigured:       true,
			CloudflareTokenConfigured: true,
			ZoneName:                  "example.com",
			ZoneID:                    "zone-1",
			DNSConfigCount:            0,
			AutoSwitchEnabled:         true,
		},
		Nodes: []NodeDiagnostic{{Name: "hk-01", GroupName: "hk", Online: false, HasReported: false}},
	})
	text := FormatStatusRiskSummary(risks)
	for _, want := range []string{"待完成：", "- 配置 DNS A 记录", "- 安装 Agent 到节点 hk-01"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected pending item %q, got %s", want, text)
		}
	}
	if len(risks.Items) != 0 {
		t.Fatalf("expected no warning items for unfinished setup, got %v", risks.Items)
	}
}

func createSwitchFixture(t *testing.T, ctx context.Context, store *db.Store) (db.Group, db.Node, db.Node) {
	t.Helper()
	group, err := store.CreateGroup(ctx, "hk", 600)
	if err != nil {
		t.Fatal(err)
	}
	oldNode, err := store.CreateNode(ctx, db.Node{
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
	if err != nil {
		t.Fatal(err)
	}
	newNode, err := store.CreateNode(ctx, db.Node{
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
	if err != nil {
		t.Fatal(err)
	}
	return group, oldNode, newNode
}

func baseHealthyRiskSetup() SetupStatus {
	return SetupStatus{
		PublicAPIURL:              "https://master.example.com",
		PublicURLConfigured:       true,
		CloudflareTokenConfigured: true,
		ZoneName:                  "example.com",
		ZoneID:                    "zone-1",
		DNSConfigCount:            1,
		AutoSwitchEnabled:         true,
	}
}

func assertRiskContains(t *testing.T, risks StatusRiskSummary, want string) {
	t.Helper()
	for _, item := range risks.Items {
		if strings.Contains(item, want) {
			return
		}
	}
	t.Fatalf("expected risk containing %q, got %v", want, risks.Items)
}
