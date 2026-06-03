package master

import "testing"

func TestValidateMasterPublicURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "http ip", input: "http://203.0.113.10:8080", want: "http://203.0.113.10:8080"},
		{name: "https domain", input: "https://example.com", want: "https://example.com"},
		{name: "empty", input: "", wantErr: true},
		{name: "bad scheme", input: "ftp://example.com", wantErr: true},
		{name: "path rejected", input: "https://example.com/api", wantErr: true},
		{name: "query rejected", input: "https://example.com?x=1", wantErr: true},
		{name: "fragment rejected", input: "https://example.com#x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateMasterPublicURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestIsLocalMasterPublicURL(t *testing.T) {
	for _, input := range []string{"", "http://127.0.0.1:8080", "http://localhost:8080", "http://0.0.0.0:8080", "http://[::1]:8080"} {
		if !IsLocalMasterPublicURL(input) {
			t.Fatalf("expected local URL: %s", input)
		}
	}
	if IsLocalMasterPublicURL("https://example.com") {
		t.Fatal("expected public URL")
	}
}

func TestBuildAgentInstallCommandRejectsLocalMasterURL(t *testing.T) {
	_, err := BuildAgentInstallCommand("http://127.0.0.1:8080", "https://example.com/install-agent.sh", "abc")
	if err != ErrLocalMasterPublicURL {
		t.Fatalf("expected ErrLocalMasterPublicURL, got %v", err)
	}
}
