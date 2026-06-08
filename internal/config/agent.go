package config

import (
	"fmt"
	"os"
	"time"
)

const DefaultAgentEnvPath = "/etc/quota-dns-router/agent.env"

type AgentConfig struct {
	EnvPath          string
	MasterAPIURL     string
	AgentID          string
	AgentToken       string
	NodeName         string
	Hostname         string
	Interface        string
	TrafficMode      string
	Interval         time.Duration
	StateFile        string
	PublicIPOverride string
	Version          string
}

func LoadAgent(path, version string) (AgentConfig, error) {
	fileValues, err := LoadEnvFile(path)
	if err != nil {
		return AgentConfig{}, err
	}
	values := MergeEnv(fileValues)
	interval, err := getDuration(values, "QDR_AGENT_INTERVAL", time.Minute)
	if err != nil {
		return AgentConfig{}, err
	}
	hostname := getString(values, "QDR_AGENT_HOSTNAME", "")
	if hostname == "" {
		if hn, err := os.Hostname(); err == nil {
			hostname = hn
		}
	}
	trafficMode := getString(values, "QDR_AGENT_TRAFFIC_MODE", "rx+tx")
	cfg := AgentConfig{
		EnvPath:          path,
		MasterAPIURL:     getString(values, "QDR_MASTER_API_URL", ""),
		AgentID:          getString(values, "QDR_AGENT_ID", ""),
		AgentToken:       getString(values, "QDR_AGENT_TOKEN", ""),
		NodeName:         getString(values, "QDR_AGENT_NODE_NAME", ""),
		Hostname:         hostname,
		Interface:        getString(values, "QDR_AGENT_IFACE", "auto"),
		TrafficMode:      trafficMode,
		Interval:         interval,
		StateFile:        getString(values, "QDR_AGENT_STATE_FILE", "/var/lib/quota-dns-router/agent-state.json"),
		PublicIPOverride: getString(values, "QDR_AGENT_PUBLIC_IP_OVERRIDE", ""),
		Version:          version,
	}
	if cfg.MasterAPIURL == "" {
		return AgentConfig{}, fmt.Errorf("缺少 QDR_MASTER_API_URL")
	}
	if cfg.AgentID == "" {
		return AgentConfig{}, fmt.Errorf("缺少 QDR_AGENT_ID")
	}
	if cfg.AgentToken == "" {
		return AgentConfig{}, fmt.Errorf("缺少 QDR_AGENT_TOKEN")
	}
	return cfg, nil
}

func (c AgentConfig) String() string {
	return fmt.Sprintf(
		"master_api=%s agent_id=%s node_name=%s hostname=%s iface=%s interval=%s state_file=%s token=%s",
		c.MasterAPIURL,
		c.AgentID,
		c.NodeName,
		c.Hostname,
		c.Interface,
		c.Interval,
		c.StateFile,
		MaskSecret(c.AgentToken),
	)
}
