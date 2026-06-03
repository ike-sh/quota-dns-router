package traffic

import "testing"

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
