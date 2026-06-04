package api

import "time"

type JoinRequest struct {
	Code string `json:"code"`
}

type JoinResponse struct {
	MasterAPIURL     string `json:"master_api_url"`
	AgentID          string `json:"agent_id"`
	AgentToken       string `json:"agent_token"`
	NodeName         string `json:"node_name"`
	GroupName        string `json:"group_name"`
	Interface        string `json:"interface"`
	IntervalSeconds  int    `json:"interval_seconds"`
	PublicIPOverride string `json:"public_ip_override"`
}

type AgentReportRequest struct {
	AgentID      string    `json:"agent_id"`
	Hostname     string    `json:"hostname"`
	PublicIP     string    `json:"public_ip"`
	Iface        string    `json:"iface"`
	RouteIface   string    `json:"route_iface"`
	RXBytesTotal int64     `json:"rx_bytes_total"`
	TXBytesTotal int64     `json:"tx_bytes_total"`
	RXDelta      int64     `json:"rx_delta"`
	TXDelta      int64     `json:"tx_delta"`
	ReportedAt   time.Time `json:"reported_at"`
	ReportTime   time.Time `json:"report_time"`
	TrafficMode  string    `json:"traffic_mode"`
	AgentVersion string    `json:"agent_version"`
	Status       string    `json:"status"`
}

type AgentReportResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}
