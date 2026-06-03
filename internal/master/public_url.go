package master

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
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

func NormalizeMasterPublicURLInput(raw string) (string, bool, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return "", false, fmt.Errorf("Master 公网地址不能为空")
	}
	if strings.HasPrefix(strings.ToLower(original), "http:") && !strings.HasPrefix(strings.ToLower(original), "http://") {
		return "", false, fmt.Errorf("URL 格式不正确：请使用 http:// 或 https:// 开头。\n示例：http://192.236.242.173:8080")
	}
	if strings.HasPrefix(strings.ToLower(original), "https:") && !strings.HasPrefix(strings.ToLower(original), "https://") {
		return "", false, fmt.Errorf("URL 格式不正确：请使用 http:// 或 https:// 开头。\n示例：https://master.example.com")
	}

	candidate := original
	normalized := false
	if !strings.Contains(candidate, "://") {
		if strings.ContainsAny(candidate, "/?#") {
			return "", false, fmt.Errorf("请带上协议，例如：http://192.236.242.173:8080")
		}
		if !hasExplicitPort(candidate) {
			candidate += ":8080"
		}
		candidate = "http://" + candidate
		normalized = true
	}
	value, err := ValidateMasterPublicURL(candidate)
	if err != nil {
		return "", normalized, err
	}
	if IsLocalMasterPublicURL(value) {
		return "", normalized, ErrLocalMasterPublicURL
	}
	return value, normalized || value != original, nil
}

func DetectPublicIPv4(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
	var lastErr error
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body := make([]byte, 64)
		n, readErr := resp.Body.Read(body)
		_ = resp.Body.Close()
		if readErr != nil && n == 0 {
			lastErr = readErr
			continue
		}
		ip := strings.TrimSpace(string(body[:n]))
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil && !parsed.IsLoopback() && !parsed.IsPrivate() {
			return parsed.String(), nil
		}
		lastErr = fmt.Errorf("公网 IP 检测结果无效：%s", ip)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("公网 IP 检测失败")
	}
	return "", lastErr
}

func SuggestedPublicAPIURLFromIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return ""
	}
	return "http://" + parsed.String() + ":8080"
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

func hasExplicitPort(raw string) bool {
	if _, _, err := net.SplitHostPort(raw); err == nil {
		return true
	}
	if strings.Count(raw, ":") == 1 {
		_, port, _ := strings.Cut(raw, ":")
		return port != ""
	}
	return false
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
	return fmt.Sprintf("bash <(curl -fsSL %s) --join %s --master %s", scriptURL, code, publicAPIURL), nil
}
