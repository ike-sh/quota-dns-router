package master

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"quota-dns-router-go/internal/version"
)

//go:embed webui/*
var webUI embed.FS

type statusAPIResponse struct {
	Version       string                 `json:"version"`
	GeneratedAt   time.Time              `json:"generatedAt"`
	Summary       statusAPISummary       `json:"summary"`
	Setup         SetupStatus            `json:"setup"`
	Cloudflare    CloudflareSummary      `json:"cloudflare"`
	DNS           []DNSSummary           `json:"dns"`
	Groups        []GroupDiagnostic      `json:"groups"`
	Nodes         []NodeDiagnostic       `json:"nodes"`
	RecentSwitch  StatusSwitchSummary    `json:"recentSwitch"`
	RecentFailure StatusFailureSummary   `json:"recentFailure"`
	Risks         StatusRiskSummary      `json:"risks"`
}

type statusAPISummary struct {
	GroupCount        int `json:"groupCount"`
	NodeCount         int `json:"nodeCount"`
	OnlineAgentCount  int `json:"onlineAgentCount"`
	OfflineAgentCount int `json:"offlineAgentCount"`
}

func statusSummaryFromOverview(overview StatusOverview) statusAPISummary {
	online, offline := 0, 0
	for _, node := range overview.Nodes {
		if node.Online {
			online++
		} else if node.HasReported {
			offline++
		}
	}
	return statusAPISummary{
		GroupCount:        len(overview.Groups),
		NodeCount:         len(overview.Nodes),
		OnlineAgentCount:  online,
		OfflineAgentCount: offline,
	}
}

func (s HTTPServer) statusAuthOK(r *http.Request) bool {
	token := strings.TrimSpace(s.StatusReadonlyToken)
	if token == "" {
		return true
	}
	return bearerToken(r.Header.Get("Authorization")) == token
}

func (s HTTPServer) requireStatusAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.statusAuthOK(r) {
		return true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="quota-dns-router status"`)
	http.Error(w, "未授权", http.StatusUnauthorized)
	return false
}

func (s HTTPServer) serveStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireStatusAuth(w, r) {
		return
	}
	now := time.Now().UTC()
	overview, err := BuildStatusOverview(r.Context(), s.Store, s.PublicAPIURL, s.DNS, now)
	if err != nil {
		http.Error(w, "读取状态失败", http.StatusInternalServerError)
		return
	}
	resp := statusAPIResponse{
		Version:       version.Version,
		GeneratedAt:   now,
		Setup:         overview.Setup,
		Cloudflare:    overview.Cloudflare,
		DNS:           overview.DNS,
		Groups:        overview.Groups,
		Nodes:         overview.Nodes,
		RecentSwitch:  overview.RecentSwitch,
		RecentFailure: overview.RecentFailure,
		Risks:         overview.Risks,
		Summary:       statusSummaryFromOverview(overview),
	}
	writeJSON(w, resp)
}

func (s HTTPServer) serveStatusUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/status" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireStatusAuth(w, r) {
		return
	}
	content, err := fs.ReadFile(webUI, "webui/index.html")
	if err != nil {
		http.Error(w, "页面不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(content)
}
