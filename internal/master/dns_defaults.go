package master

import (
	"strings"

	"quota-dns-router-go/internal/config"
)

const Route53PlaceholderToken = "route53:aws-default"

func IsRoute53Provider(kind string) bool {
	return strings.EqualFold(strings.TrimSpace(kind), "route53")
}

func IsRoute53PlaceholderToken(token string) bool {
	return strings.TrimSpace(token) == Route53PlaceholderToken
}

func DNSCredentialLabel(dnsProvider string) string {
	if IsRoute53Provider(dnsProvider) {
		return "Route53（AWS 凭证）"
	}
	return "Cloudflare Token"
}

func DNSCredentialConfigured(dnsProvider, token, zoneID string) bool {
	if IsRoute53Provider(dnsProvider) {
		return strings.TrimSpace(zoneID) != ""
	}
	token = strings.TrimSpace(token)
	return token != "" && !IsRoute53PlaceholderToken(token)
}

func DNSCredentialMasked(dnsProvider, token string) string {
	if IsRoute53Provider(dnsProvider) {
		return "AWS 默认凭证链"
	}
	if IsRoute53PlaceholderToken(token) {
		return "（Route53 占位）"
	}
	return config.MaskSecret(token)
}
