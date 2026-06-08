package master

import "testing"

func TestNewDNSProviderCloudflare(t *testing.T) {
	p, err := NewDNSProvider("cloudflare", "")
	if err != nil || p == nil {
		t.Fatalf("expected cloudflare provider, err=%v", err)
	}
}

func TestNewDNSProviderRoute53(t *testing.T) {
	p, err := NewDNSProvider("route53", "us-east-1")
	if err != nil || p == nil {
		t.Fatalf("expected route53 provider, err=%v", err)
	}
}

func TestNewDNSProviderRejectsUnknown(t *testing.T) {
	if _, err := NewDNSProvider("dnspod", ""); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
