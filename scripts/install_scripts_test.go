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
	assertContains(t, out, "quota-dns-router install-master 0.1.0-alpha.11")
}

func TestInstallAgentHelpDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--help")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "用法：install-agent.sh")
}

func TestInstallAgentVersionDoesNotRequireJoinCode(t *testing.T) {
	out := runScript(t, "install-agent.sh", "--version")
	assertNotContains(t, out, "缺少 --join")
	assertContains(t, out, "quota-dns-router install-agent 0.1.0-alpha.11")
}

func TestInstallMasterDryRunDefaultsToBinaryRelease(t *testing.T) {
	out := runBash(t, "QDR_TELEGRAM_BOT_TOKEN=xxx QDR_TELEGRAM_ADMIN_ID=123 bash install-master.sh --yes --dry-run")
	for _, want := range []string{
		"安装模式：binary",
		"来源：GitHub Releases",
		"qdr-master_linux_amd64.tar.gz",
		"SHA256SUMS",
		"/usr/local/bin/qdr-master version",
		"QDR_SUGGESTED_PUBLIC_API_URL=http://203.0.113.10:8080",
	} {
		assertContains(t, out, want)
	}
	for _, unwanted := range []string{"go build", "git clone", "golang-go", "build-essential"} {
		assertNotContains(t, out, unwanted)
	}
}

func TestInstallAgentDryRunDefaultsToBinaryRelease(t *testing.T) {
	out := runBash(t, "bash install-agent.sh --join abc --master http://203.0.113.10:8080 --dry-run")
	for _, want := range []string{
		"安装模式：binary",
		"来源：GitHub Releases",
		"qdr-agent_linux_amd64.tar.gz",
		"SHA256SUMS",
		"/usr/local/bin/qdr-agent version",
		"/usr/local/bin/qdr-agent join --code <已隐藏> --master http://203.0.113.10:8080 --env /etc/quota-dns-router/agent.env",
	} {
		assertContains(t, out, want)
	}
	for _, unwanted := range []string{"go build", "git clone", "golang-go", "build-essential"} {
		assertNotContains(t, out, unwanted)
	}
}

func TestInstallSourceModeDryRunShowsSourceBuildFlow(t *testing.T) {
	masterOut := runBash(t, "QDR_INSTALL_MODE=source QDR_TELEGRAM_BOT_TOKEN=xxx QDR_TELEGRAM_ADMIN_ID=123 bash install-master.sh --yes --dry-run")
	for _, want := range []string{
		"安装模式：source",
		"来源：GitHub source main",
		"安装源码构建依赖",
		"CGO_ENABLED=0 go build",
		"尝试通过系统包管理器安装 Go",
	} {
		assertContains(t, masterOut, want)
	}
	agentOut := runBash(t, "QDR_INSTALL_MODE=source bash install-agent.sh --join abc --master http://203.0.113.10:8080 --dry-run")
	for _, want := range []string{
		"安装模式：source",
		"来源：GitHub source main",
		"安装源码构建依赖",
		"CGO_ENABLED=0 go build",
		"尝试通过系统包管理器安装 Go",
	} {
		assertContains(t, agentOut, want)
	}
}

func TestInstallScriptsSetSecurePermissions(t *testing.T) {
	master := readScript(t, "install-master.sh")
	assertContains(t, master, `install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"`)
	assertContains(t, master, `install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"`)
	assertContains(t, master, `chown root:quota-dns-router "${ETC_DIR}/master.env"`)
	assertContains(t, master, `chmod 0640 "${ETC_DIR}/master.env"`)
	assertContains(t, master, `chown quota-dns-router:quota-dns-router "${DATA_DIR}/master.db"`)
	assertContains(t, master, `chmod 600 "${DATA_DIR}/master.db"`)

	agent := readScript(t, "install-agent.sh")
	assertContains(t, agent, `install -d -m 750 -o root -g quota-dns-router "$ETC_DIR"`)
	assertContains(t, agent, `install -d -m 750 -o quota-dns-router -g quota-dns-router "$DATA_DIR" "$LOG_DIR"`)
	assertContains(t, agent, `chown root:quota-dns-router "${ETC_DIR}/agent.env"`)
	assertContains(t, agent, `chmod 0640 "${ETC_DIR}/agent.env"`)
	assertContains(t, agent, `User=quota-dns-router`)
	assertContains(t, agent, `Group=quota-dns-router`)
}

func TestInstallScriptsCheckDiskSpaceAndSafeGoFallback(t *testing.T) {
	master := readScript(t, "install-master.sh")
	for _, want := range []string{
		`BINARY_ROOT_MIN_SPACE_MB=80`,
		`BINARY_TMP_MIN_SPACE_MB=80`,
		`SOURCE_ROOT_MIN_SPACE_MB=800`,
		`SOURCE_TMP_MIN_SPACE_MB=500`,
		`SOURCE_USR_LOCAL_MIN_SPACE_MB=800`,
		`GO_TMP_DIR="$(mktemp -d)"`,
		`mkdir -p "${GO_TMP_DIR}/extract"`,
		`tar -C "${GO_TMP_DIR}/extract" -xzf "${GO_TMP_DIR}/go.tgz"`,
		`mv "${GO_TMP_DIR}/extract/go" /usr/local/go`,
		`tail -n 50`,
	} {
		assertContains(t, master, want)
	}

	agent := readScript(t, "install-agent.sh")
	for _, want := range []string{
		`BINARY_ROOT_MIN_SPACE_MB=50`,
		`BINARY_TMP_MIN_SPACE_MB=50`,
		`SOURCE_ROOT_MIN_SPACE_MB=800`,
		`SOURCE_TMP_MIN_SPACE_MB=500`,
		`SOURCE_USR_LOCAL_MIN_SPACE_MB=800`,
		`GO_TMP_DIR="$(mktemp -d)"`,
		`mkdir -p "${GO_TMP_DIR}/extract"`,
		`tar -C "${GO_TMP_DIR}/extract" -xzf "${GO_TMP_DIR}/go.tgz"`,
		`mv "${GO_TMP_DIR}/extract/go" /usr/local/go`,
		`tail -n 50`,
	} {
		assertContains(t, agent, want)
	}
}

func TestInstallScriptsExposeModeAndFallbackControls(t *testing.T) {
	for _, name := range []string{"install-master.sh", "install-agent.sh"} {
		body := readScript(t, name)
		for _, want := range []string{
			`QDR_INSTALL_MODE`,
			`QDR_ALLOW_SOURCE_FALLBACK`,
			`安装模式：binary`,
			`来源：GitHub Releases`,
			`安装模式：source`,
			`来源：GitHub source ${BRANCH}`,
		} {
			assertContains(t, body, want)
		}
	}
}

func TestInstallAgentScriptSupportsMasterAndVersionCheck(t *testing.T) {
	body := readScript(t, "install-agent.sh")
	for _, want := range []string{
		`[--master <url>] [--yes] [--dry-run] [--help] [--version]`,
		`--yes                    兼容参数，Agent 安装默认无交互`,
		`缺少 Master 地址。请使用 --master <url>，或直接使用 Telegram 生成的完整命令。`,
		`${PREFIX}/${BIN_NAME} version`,
		`expected="quota-dns-router agent ${VERSION}"`,
	} {
		assertContains(t, body, want)
	}
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

func TestInstallScriptsPrintUninstallCommands(t *testing.T) {
	master := readScript(t, "install-master.sh")
	for _, want := range []string{
		"Cloudflare、DNS、节点和 Agent 安装",
		"DNS 向导会自动创建 default 分组",
		"uninstall-master.sh) --yes",
		"uninstall-master.sh) --yes --purge",
	} {
		assertContains(t, master, want)
	}
	agent := readScript(t, "install-agent.sh")
	for _, want := range []string{
		"uninstall-agent.sh) --yes",
		"uninstall-agent.sh) --yes --purge",
	} {
		assertContains(t, agent, want)
	}
}

func TestInstallScriptsUseVersionedReleaseDownloads(t *testing.T) {
	for _, name := range []string{"install-master.sh", "install-agent.sh"} {
		body := readScript(t, name)
		assertContains(t, body, `VERSION="0.1.0-alpha.11"`)
		assertContains(t, body, `release_base="${repo_no_git}/releases/download/v${VERSION}"`)
	}
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
