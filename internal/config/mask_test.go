package config

import "testing"

func TestMaskSecret(t *testing.T) {
	got := MaskSecret("1234567890")
	if got != "123****890" {
		t.Fatalf("unexpected mask: %s", got)
	}
}
