package master

import (
	"testing"

	"quota-dns-router-go/internal/db"
)

func TestValidateGroupName(t *testing.T) {
	if err := ValidateGroupName(""); err == nil {
		t.Fatal("expected empty group name error")
	}
	if err := ValidateGroupName("hk"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNodeConfig(t *testing.T) {
	valid := db.Node{
		GroupID:               "grp-1",
		Name:                  "hk-01",
		PublicIP:              "203.0.113.10",
		MonthlyQuotaBytes:     1000,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           db.TrafficModeBoth,
		Priority:              10,
		PreferredIface:        "auto",
		ReportIntervalSeconds: 60,
	}
	if err := ValidateNodeConfig(valid, "A"); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.PublicIP = "bad-ip"
	if err := ValidateNodeConfig(invalid, "A"); err == nil {
		t.Fatal("expected invalid IP error")
	}
	invalid = valid
	invalid.ThresholdPercent = 0
	if err := ValidateNodeConfig(invalid, "A"); err == nil {
		t.Fatal("expected threshold error")
	}
}

func TestValidatePublicIPv6(t *testing.T) {
	if err := ValidatePublicIPv6("2001:db8::1"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicIPv6("203.0.113.10"); err == nil {
		t.Fatal("expected ipv4 rejected for AAAA")
	}
	if err := ValidatePublicIPv6("fe80::1"); err == nil {
		t.Fatal("expected link-local rejected")
	}
}

func TestValidatePublicIPByRecordType(t *testing.T) {
	if err := ValidatePublicIP("203.0.113.10", "A"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicIP("2001:db8::1", "AAAA"); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicIP("203.0.113.10", "AAAA"); err == nil {
		t.Fatal("expected ipv4 rejected for AAAA group")
	}
}
