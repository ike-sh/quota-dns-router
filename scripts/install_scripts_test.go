package scripts

import (
	"errors"
	"os"
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
	assertContains(t, out, "quota-dns-router install-master 0.1.0-alpha.4")
}

func TestInstallAgentHelpDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--help")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "用法：install-agent.sh")
}

func TestInstallAgentVersionDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--version")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "quota-dns-router install-agent 0.1.0-alpha.4")
}

func TestInstallMasterScriptSetsSecureEnvPermissions(t *testing.T) {
	body := readScript(t, "install-master.sh")
	assertContains(t, body, `install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"`)
	assertContains(t, body, `install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"`)
	assertContains(t, body, `chown root:quota-dns-router "${ETC_DIR}/master.env"`)
	assertContains(t, body, `chmod 0640 "${ETC_DIR}/master.env"`)
	assertContains(t, body, `chown quota-dns-router:quota-dns-router "${DATA_DIR}/master.db"`)
	assertContains(t, body, `chmod 600 "${DATA_DIR}/master.db"`)
}

func TestInstallMasterDryRunWritesSuggestedPublicURL(t *testing.T) {
	out := runBash(t, "QDR_TELEGRAM_BOT_TOKEN=xxx QDR_TELEGRAM_ADMIN_ID=123 bash install-master.sh --yes --dry-run")
	assertContains(t, out, "检测公网 IPv4")
	assertContains(t, out, "http://203.0.113.10:8080")
	assertContains(t, out, "QDR_SUGGESTED_PUBLIC_API_URL")
}

func TestInstallAgentScriptSetsSecureEnvPermissions(t *testing.T) {
	body := readScript(t, "install-agent.sh")
	assertContains(t, body, `install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"`)
	assertContains(t, body, `install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"`)
	assertContains(t, body, `chown root:quota-dns-router "${ETC_DIR}/agent.env"`)
	assertContains(t, body, `chmod 0640 "${ETC_DIR}/agent.env"`)
	assertContains(t, body, `User=quota-dns-router`)
	assertContains(t, body, `Group=quota-dns-router`)
}

func TestInstallUnitsDoNotExposeTokensInExecStart(t *testing.T) {
	for _, name := range []string{"install-master.sh", "install-agent.sh"} {
		body := readScript(t, name)
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, "ExecStart=") && strings.Contains(strings.ToLower(line), "token") {
				t.Fatalf("%s exposes token in ExecStart: %s", name, line)
			}
		}
	}
}

func TestInstallScriptsPrintServiceFailureDiagnostics(t *testing.T) {
	master := readScript(t, "install-master.sh")
	assertContains(t, master, "systemctl status quota-dns-router-master --no-pager -l")
	assertContains(t, master, "journalctl -u quota-dns-router-master -n 100 --no-pager")
	agent := readScript(t, "install-agent.sh")
	assertContains(t, agent, "systemctl status quota-dns-router-agent --no-pager -l")
	assertContains(t, agent, "journalctl -u quota-dns-router-agent -n 100 --no-pager")
}

func TestUninstallScriptsPurgeAndResetFailed(t *testing.T) {
	master := readScript(t, "uninstall-master.sh")
	for _, want := range []string{
		"rm -rf /etc/quota-dns-router",
		"rm -rf /var/lib/quota-dns-router",
		"rm -rf /var/log/quota-dns-router",
		"rm -f /usr/local/bin/qdr-master",
		"rm -f /etc/systemd/system/quota-dns-router-master.service",
		"systemctl reset-failed quota-dns-router-master.service",
	} {
		assertContains(t, master, want)
	}
	agent := readScript(t, "uninstall-agent.sh")
	for _, want := range []string{
		"rm -f /usr/local/bin/qdr-agent",
		"rm -f /etc/systemd/system/quota-dns-router-agent.service",
		"systemctl reset-failed quota-dns-router-agent.service",
	} {
		assertContains(t, agent, want)
	}
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

func runBash(t *testing.T, command string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("bash not found")
		}
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-lc", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	return string(out)
}

func readScript(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
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
