package db

import (
	"context"
	"testing"
	"time"
)

func TestValidateAgentToken(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	group, err := store.CreateGroup(ctx, "sg", 600)
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.CreateNode(ctx, Node{
		GroupID:               group.ID,
		Name:                  "sg-01",
		PublicIP:              "2.2.2.2",
		MonthlyQuotaBytes:     100,
		ThresholdPercent:      80,
		ResetDay:              1,
		TrafficMode:           TrafficModeBoth,
		Enabled:               true,
		AutoSwitch:            true,
		Priority:              1,
		PreferredIface:        "eth0",
		ReportIntervalSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	code, err := store.GenerateJoinCode(ctx, node.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	res, err := store.RedeemJoinCode(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := store.ValidateAgentToken(ctx, res.AgentID, res.AgentToken)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected token to validate")
	}
	if err := store.RevokeAgentTokens(ctx, res.AgentID); err != nil {
		t.Fatal(err)
	}
	ok, err = store.ValidateAgentToken(ctx, res.AgentID, res.AgentToken)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected token to be revoked")
	}
}
