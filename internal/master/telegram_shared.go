package master

import (
	"fmt"
	"strings"
)

const settingSuggestedPublicAPIURL = "suggested_public_api_url"

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func formatDNSRecordType(recordType string) string {
	if strings.TrimSpace(recordType) == "" {
		return "A"
	}
	return strings.TrimSpace(recordType)
}

func formatDNSCurrentRecordLine(recordType, value string) string {
	return fmt.Sprintf("当前 %s 记录：%s", formatDNSRecordType(recordType), valueOrDash(value))
}

func dnsRecordLabel(recordType string) string {
	return "DNS " + formatDNSRecordType(recordType) + " 记录"
}
