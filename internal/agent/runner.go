package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"quota-dns-router-go/internal/api"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/traffic"
)

type Runner struct {
	Config     config.AgentConfig
	HTTPClient *http.Client
}

func NewRunner(cfg config.AgentConfig) *Runner {
	return &Runner{
		Config:     cfg,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.Config.Interval)
	defer ticker.Stop()
	if err := r.Once(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.Once(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "上报失败：%v\n", err)
			}
		}
	}
}

func (r *Runner) Once(ctx context.Context) error {
	iface, routeIface, err := traffic.ResolveInterface(r.Config.Interface, "")
	if err != nil {
		return err
	}
	dev, err := traffic.ReadProcNetDev("")
	if err != nil {
		return err
	}
	current, ok := dev[iface]
	if !ok {
		return fmt.Errorf("未找到网卡 %s", iface)
	}
	state, err := traffic.LoadState(r.Config.StateFile)
	if err != nil {
		return err
	}
	sample := traffic.BuildSample(current, state)
	now := time.Now().UTC()
	if err := traffic.SaveState(r.Config.StateFile, traffic.State{Last: current, At: now}); err != nil {
		return err
	}
	report := api.AgentReportRequest{
		AgentID:      r.Config.AgentID,
		Hostname:     r.Config.Hostname,
		PublicIP:     traffic.DiscoverPublicIP(r.Config.PublicIPOverride, r.HTTPClient),
		Iface:        sample.Iface,
		RouteIface:   routeIface,
		RXBytesTotal: sample.RXBytesTotal,
		TXBytesTotal: sample.TXBytesTotal,
		RXDelta:      sample.RXDelta,
		TXDelta:      sample.TXDelta,
		ReportedAt:   now,
		ReportTime:   now,
		TrafficMode:  r.Config.TrafficMode,
		AgentVersion: r.Config.Version,
		Status:       "online",
	}
	return r.PostReport(ctx, report)
}

func (r *Runner) PostReport(ctx context.Context, report api.AgentReportRequest) error {
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.Config.MasterAPIURL, "/")+"/api/agent/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Config.AgentToken)
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Master 返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

func Join(ctx context.Context, masterAPIURL, code string, client *http.Client) (api.JoinResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	payload, err := json.Marshal(api.JoinRequest{Code: code})
	if err != nil {
		return api.JoinResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(masterAPIURL, "/")+"/api/agent/join", bytes.NewReader(payload))
	if err != nil {
		return api.JoinResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return api.JoinResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return api.JoinResponse{}, fmt.Errorf("加入失败，Master 返回 HTTP %d", resp.StatusCode)
	}
	var out api.JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return api.JoinResponse{}, err
	}
	return out, nil
}

func RenderAgentEnv(resp api.JoinResponse, stateFile string) string {
	if stateFile == "" {
		stateFile = "/var/lib/quota-dns-router/agent-state.json"
	}
	trafficMode := resp.TrafficMode
	if trafficMode == "" {
		trafficMode = "rx+tx"
	}
	return fmt.Sprintf(`QDR_MASTER_API_URL=%s
QDR_AGENT_ID=%s
QDR_AGENT_TOKEN=%s
QDR_AGENT_NODE_NAME=%s
QDR_AGENT_IFACE=%s
QDR_AGENT_TRAFFIC_MODE=%s
QDR_AGENT_INTERVAL=%ds
QDR_AGENT_PUBLIC_IP_OVERRIDE=%s
QDR_AGENT_STATE_FILE=%s
`, resp.MasterAPIURL, resp.AgentID, resp.AgentToken, resp.NodeName, resp.Interface, trafficMode, resp.IntervalSeconds, resp.PublicIPOverride, stateFile)
}
