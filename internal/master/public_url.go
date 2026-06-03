package master

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const localMasterURLWarning = "WARNING: 当前 Master 地址仍是本机地址，Agent join/install 将无法从其他服务器访问。请通过 Telegram /config_master_url 配置公网地址。"

var ErrLocalMasterPublicURL = errors.New("当前 Master 地址仍是本机地址，Agent 无法访问。请先配置 Master 公网地址。")

func ValidateMasterPublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("Master 公网地址不能为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("Master 公网地址格式无效")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("Master 公网地址只支持 http 或 https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return "", fmt.Errorf("Master 公网地址必须包含主机名或 IP")
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("Master 公网地址不能包含 path/query/fragment")
	}
	return parsed.String(), nil
}

func IsLocalMasterPublicURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "", "localhost", "0.0.0.0", "::1":
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	return ip.Equal(net.ParseIP("0.0.0.0"))
}

func MasterPublicURLWarning(raw string) string {
	if IsLocalMasterPublicURL(raw) {
		return localMasterURLWarning
	}
	return ""
}

func BuildAgentInstallCommand(publicAPIURL, scriptURL, code string) (string, error) {
	publicAPIURL, err := ValidateMasterPublicURL(publicAPIURL)
	if err != nil {
		return "", err
	}
	if IsLocalMasterPublicURL(publicAPIURL) {
		return "", ErrLocalMasterPublicURL
	}
	return fmt.Sprintf("QDR_MASTER_API_URL=%s bash <(curl -fsSL %s) --join %s", publicAPIURL, scriptURL, code), nil
}
