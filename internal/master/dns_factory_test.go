package master

import "testing"

func TestNewDNSProviderCloudflare(t *testing.T) {
	p, err := NewDNSProvider("cloudflare")
	if err != nil || p == nil {
		t.Fatalf("expected cloudflare provider, err=%v", err)
	}
}

func TestNewDNSProviderRejectsUnknown(t *testing.T) {
	if _, err := NewDNSProvider("route53"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
