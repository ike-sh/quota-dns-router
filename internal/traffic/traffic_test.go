package traffic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSampleDelta(t *testing.T) {
	current := Snapshot{Iface: "eth0", RX: 200, TX: 300}
	prev := State{Last: Snapshot{Iface: "eth0", RX: 100, TX: 200}}
	sample := BuildSample(current, prev)
	if sample.RXDelta != 100 || sample.TXDelta != 100 {
		t.Fatalf("unexpected delta: %+v", sample)
	}
}

func TestBuildSampleCounterReset(t *testing.T) {
	current := Snapshot{Iface: "eth0", RX: 10, TX: 20}
	prev := State{Last: Snapshot{Iface: "eth0", RX: 100, TX: 200}}
	sample := BuildSample(current, prev)
	if sample.RXDelta != 10 || sample.TXDelta != 20 {
		t.Fatalf("unexpected reset delta: %+v", sample)
	}
}

func TestResolveInterfaceUsesExplicitIfaceWhenRouteMissing(t *testing.T) {
	selected, routeIface, err := ResolveInterface("eth9", filepath.Join(t.TempDir(), "missing-route"))
	if err != nil {
		t.Fatalf("expected explicit iface to ignore route detection error, got %v", err)
	}
	if selected != "eth9" || routeIface != "" {
		t.Fatalf("expected selected eth9 and empty route iface, got selected=%q route=%q", selected, routeIface)
	}
}

func TestResolveInterfaceAutoUsesDefaultRoute(t *testing.T) {
	routePath := filepath.Join(t.TempDir(), "route")
	body := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t01020304\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	if err := os.WriteFile(routePath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	selected, routeIface, err := ResolveInterface("auto", routePath)
	if err != nil {
		t.Fatalf("ResolveInterface(auto): %v", err)
	}
	if selected != "eth0" || routeIface != "eth0" {
		t.Fatalf("expected eth0, got selected=%q route=%q", selected, routeIface)
	}
}
