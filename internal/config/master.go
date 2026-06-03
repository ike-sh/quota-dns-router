package config

import (
	"fmt"
	"strconv"
	"time"
)

const DefaultMasterEnvPath = "/etc/quota-dns-router/master.env"

type MasterConfig struct {
	EnvPath             string
	ListenAddr          string
	PublicAPIURL        string
	DBPath              string
	DataDir             string
	LogDir              string
	TelegramToken       string
	TelegramAdminID     int64
	TelegramPollTimeout time.Duration
	CheckInterval       time.Duration
	AgentOfflineAfter   time.Duration
	OfflineNotifyAfter  time.Duration
}

func LoadMaster(path string) (MasterConfig, error) {
	if path == "" {
		path = DefaultMasterEnvPath
	}
	fileValues, err := LoadEnvFile(path)
	if err != nil {
		return MasterConfig{}, err
	}
	values := MergeEnv(fileValues)

	adminIDRaw := getString(values, "QDR_TELEGRAM_ADMIN_ID", "")
	if adminIDRaw == "" {
		return MasterConfig{}, fmt.Errorf("缺少 QDR_TELEGRAM_ADMIN_ID")
	}
	adminID, err := strconv.ParseInt(adminIDRaw, 10, 64)
	if err != nil {
		return MasterConfig{}, fmt.Errorf("QDR_TELEGRAM_ADMIN_ID 不是有效数字")
	}
	pollTimeout, err := getDuration(values, "QDR_TELEGRAM_POLL_TIMEOUT", 20*time.Second)
	if err != nil {
		return MasterConfig{}, err
	}
	checkInterval, err := getDuration(values, "QDR_CHECK_INTERVAL", time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}
	offlineAfter, err := getDuration(values, "QDR_AGENT_OFFLINE_AFTER", 5*time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}
	offlineNotifyAfter, err := getDuration(values, "QDR_OFFLINE_NOTIFY_AFTER", 10*time.Minute)
	if err != nil {
		return MasterConfig{}, err
	}

	cfg := MasterConfig{
		EnvPath:             path,
		ListenAddr:          getString(values, "QDR_MASTER_LISTEN_ADDR", ":8080"),
		PublicAPIURL:        getString(values, "QDR_MASTER_PUBLIC_API_URL", "http://127.0.0.1:8080"),
		DBPath:              getString(values, "QDR_MASTER_DB_PATH", "/var/lib/quota-dns-router/master.db"),
		DataDir:             getString(values, "QDR_MASTER_DATA_DIR", "/var/lib/quota-dns-router"),
		LogDir:              getString(values, "QDR_MASTER_LOG_DIR", "/var/log/quota-dns-router"),
		TelegramToken:       getString(values, "QDR_TELEGRAM_TOKEN", ""),
		TelegramAdminID:     adminID,
		TelegramPollTimeout: pollTimeout,
		CheckInterval:       checkInterval,
		AgentOfflineAfter:   offlineAfter,
		OfflineNotifyAfter:  offlineNotifyAfter,
	}
	if cfg.TelegramToken == "" {
		return MasterConfig{}, fmt.Errorf("缺少 QDR_TELEGRAM_TOKEN")
	}
	return cfg, nil
}

func (c MasterConfig) String() string {
	return fmt.Sprintf(
		"listen=%s public_api=%s db=%s data_dir=%s log_dir=%s admin_id=%d telegram_token=%s",
		c.ListenAddr,
		c.PublicAPIURL,
		c.DBPath,
		c.DataDir,
		c.LogDir,
		c.TelegramAdminID,
		MaskSecret(c.TelegramToken),
	)
}
