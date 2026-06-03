package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func LoadEnvFile(path string) (map[string]string, error) {
	out := make(map[string]string)
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, formatEnvFileReadError(path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效环境变量行: %q", line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, formatEnvFileReadError(path, err)
	}
	return out, nil
}

func formatEnvFileReadError(path string, err error) error {
	if os.IsPermission(err) {
		return fmt.Errorf(`无法读取配置文件 %s：权限不足。
请检查：
1. 文件属主是否为 root:quota-dns-router 或 quota-dns-router:quota-dns-router
2. 文件权限是否为 640 或 600
3. systemd 服务是否使用 User=quota-dns-router`, path)
	}
	return fmt.Errorf("无法读取配置文件 %s：%w", path, err)
}

func MergeEnv(fileValues map[string]string) map[string]string {
	out := make(map[string]string, len(fileValues)+len(os.Environ()))
	for k, v := range fileValues {
		out[k] = v
	}
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func getString(values map[string]string, key, fallback string) string {
	if v := strings.TrimSpace(values[key]); v != "" {
		return v
	}
	return fallback
}

func getInt(values map[string]string, key string, fallback int) (int, error) {
	v := strings.TrimSpace(values[key])
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s 不是有效整数", key)
	}
	return n, nil
}

func getInt64(values map[string]string, key string, fallback int64) (int64, error) {
	v := strings.TrimSpace(values[key])
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s 不是有效整数", key)
	}
	return n, nil
}

func getBool(values map[string]string, key string, fallback bool) (bool, error) {
	v := strings.TrimSpace(values[key])
	if v == "" {
		return fallback, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s 不是有效布尔值", key)
	}
}

func getDuration(values map[string]string, key string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(values[key])
	if v == "" {
		return fallback, nil
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s 不是有效时间", key)
	}
	return d, nil
}

func MaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return strings.Repeat("*", len(value))
	}
	return value[:3] + strings.Repeat("*", len(value)-6) + value[len(value)-3:]
}
