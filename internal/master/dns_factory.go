package master

import (
	"fmt"
	"strings"

	"quota-dns-router-go/internal/cloudflare"
)

func NewDNSProvider(kind string) (DNSProvider, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "cloudflare":
		return cloudflare.NewClient(nil), nil
	default:
		return nil, fmt.Errorf("不支持的 DNS 服务商：%s（当前仅支持 cloudflare）", kind)
	}
}
