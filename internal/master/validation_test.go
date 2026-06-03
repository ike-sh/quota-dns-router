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
	if err := ValidateNodeConfig(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.PublicIP = "bad-ip"
	if err := ValidateNodeConfig(invalid); err == nil {
		t.Fatal("expected invalid IP error")
	}
	invalid = valid
	invalid.ThresholdPercent = 0
	if err := ValidateNodeConfig(invalid); err == nil {
		t.Fatal("expected threshold error")
	}
}
