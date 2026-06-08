package master

import "testing"

func TestIsMutatingCallbackAllowsReadOnlyViews(t *testing.T) {
	for _, data := range []string{"status", "dns_view:g1", "groups_view:g1", "nodes_view:n1"} {
		if isMutatingCallback(data) {
			t.Fatalf("expected read-only callback %q", data)
		}
	}
}

func TestIsMutatingCallbackBlocksWrites(t *testing.T) {
	for _, data := range []string{"dns_add", "dns_ttl_set:g1:60", "switch_do:g1:n1", "groups_rename:g1"} {
		if !isMutatingCallback(data) {
			t.Fatalf("expected mutating callback %q", data)
		}
	}
}

func TestIsMutatingCommandAllowsStatus(t *testing.T) {
	if isMutatingCommand("/status") {
		t.Fatal("/status should be allowed for observers")
	}
	if !isMutatingCommand("/dns set hk example.com") {
		t.Fatal("admin commands should be mutating")
	}
}
