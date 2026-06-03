package scripts

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestInstallMasterHelpDoesNotPrompt(t *testing.T) {
	out := runScript(t, "install-master.sh", "--help")
	assertNotContains(t, out, "Telegram Bot Token:")
	assertNotContains(t, out, "Telegram 管理员 ID:")
	assertContains(t, out, "用法：install-master.sh")
}

func TestInstallMasterVersionDoesNotPrompt(t *testing.T) {
	out := runScript(t, "install-master.sh", "--version")
	assertNotContains(t, out, "Telegram Bot Token:")
	assertContains(t, out, "quota-dns-router install-master 0.1.0-alpha.1")
}

func TestInstallAgentHelpDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--help")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "用法：install-agent.sh")
}

func TestInstallAgentVersionDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--version")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "quota-dns-router install-agent 0.1.0-alpha.1")
}

func runScript(t *testing.T, name string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("bash not found")
		}
		t.Fatal(err)
	}
	cmdArgs := append([]string{name}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	return string(out)
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got %s", want, got)
	}
}

func assertNotContains(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected output not to contain %q, got %s", unwanted, got)
	}
}
