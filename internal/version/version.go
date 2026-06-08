package version

import "fmt"

const Version = "0.2.3.1"

const releaseRepo = "ike-sh/quota-dns-router"

func MasterString() string {
	return "quota-dns-router master " + Version
}

func AgentString() string {
	return "quota-dns-router agent " + Version
}

func ReleaseScriptURL(script string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/v%s/scripts/%s", releaseRepo, Version, script)
}

func DefaultInstallAgentURL() string {
	return ReleaseScriptURL("install-agent.sh")
}

func DefaultUninstallAgentURL() string {
	return ReleaseScriptURL("uninstall-agent.sh")
}
