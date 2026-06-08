package master

import (
	"context"
	"fmt"
	"strings"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/route53"
)

func NewDNSProvider(kind, awsRegion string) (DNSProvider, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "cloudflare":
		return cloudflare.NewClient(nil), nil
	case "route53":
		return route53.NewClient(context.Background(), awsRegion)
	default:
		return nil, fmt.Errorf("不支持的 DNS 服务商：%s（支持 cloudflare、route53）", kind)
	}
}
