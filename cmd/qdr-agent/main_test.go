package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestAgentVersionOutput(t *testing.T) {
	got := captureStdout(t, func() {
		if err := run([]string{"version"}); err != nil {
			t.Fatal(err)
		}
	})
	want := "quota-dns-router agent 0.1.0-alpha.5"
	if strings.TrimSpace(got) != want {
		t.Fatalf("got %q want %q", strings.TrimSpace(got), want)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
