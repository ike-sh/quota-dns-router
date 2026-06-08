package master

import (
	"context"
	"testing"

	"quota-dns-router-go/internal/cloudflare"
)

func TestRoute53PanelShowsHostedZoneButtonWithoutToken(t *testing.T) {
	controller, rec := newTestTelegramControllerWithDNS(t, fakeDNS{
		zones: []cloudflare.Zone{{Name: "example.com", ID: "Z1"}},
	})
	controller.DNSProviderKind = "route53"
	ctx := context.Background()
	_ = controller.Store.SaveCloudflareDefaults(ctx, route53PlaceholderToken, "", "")

	if err := controller.handleCallback(ctx, 1, "cf"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Route53 配置", "AWS 默认凭证链", "选择 Hosted Zone"} {
		if !rec.contains(want) {
			t.Fatalf("expected %q in %v", want, rec.payloads)
		}
	}
}
