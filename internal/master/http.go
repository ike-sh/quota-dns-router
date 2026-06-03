package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"quota-dns-router-go/internal/api"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
)

type HTTPServer struct {
	Store        *db.Store
	PublicAPIURL string
}

func (s HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api/agent/join", s.join)
	mux.HandleFunc("/api/agent/report", s.report)
	return mux
}

func (s HTTPServer) join(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	var req api.JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}
	res, err := s.Store.RedeemJoinCode(r.Context(), strings.TrimSpace(req.Code))
	if err != nil {
		http.Error(w, "加入码无效或已过期", http.StatusUnauthorized)
		return
	}
	publicAPIURL, err := s.Store.GetMasterPublicURL(r.Context(), s.PublicAPIURL)
	if err != nil {
		http.Error(w, "读取 Master 公网地址失败", http.StatusInternalServerError)
		return
	}
	resp := api.JoinResponse{
		MasterAPIURL:     publicAPIURL,
		AgentID:          res.AgentID,
		AgentToken:       res.AgentToken,
		NodeName:         res.NodeName,
		GroupName:        res.GroupName,
		Interface:        res.Interface,
		IntervalSeconds:  res.ReportIntervalSeconds,
		PublicIPOverride: res.PublicIPOverride,
	}
	writeJSON(w, resp)
}

func (s HTTPServer) report(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	var req api.AgentReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}
	token := bearerToken(r.Header.Get("Authorization"))
	ok, err := s.Store.ValidateAgentToken(r.Context(), req.AgentID, token)
	if err != nil || !ok {
		_ = s.Store.SaveLastError(r.Context(), errorKeyAgentReportAuth, "Agent 上报鉴权失败：agent_id="+strings.TrimSpace(req.AgentID), token)
		http.Error(w, "Agent Token 校验失败", http.StatusUnauthorized)
		return
	}
	_ = s.Store.ClearLastError(r.Context(), errorKeyAgentReportAuth)
	if req.ReportedAt.IsZero() {
		req.ReportedAt = time.Now().UTC()
	}
	if req.PublicIP == "" {
		req.PublicIP = "unknown"
	}
	err = s.Store.SaveAgentReport(r.Context(), db.AgentReport{
		AgentID:      req.AgentID,
		Hostname:     req.Hostname,
		PublicIP:     req.PublicIP,
		Iface:        req.Iface,
		RXBytesTotal: req.RXBytesTotal,
		TXBytesTotal: req.TXBytesTotal,
		RXDelta:      req.RXDelta,
		TXDelta:      req.TXDelta,
		ReportedAt:   req.ReportedAt,
		AgentVersion: req.AgentVersion,
		Status:       req.Status,
	})
	if err != nil {
		http.Error(w, "保存上报失败", http.StatusInternalServerError)
		return
	}
	writeJSON(w, api.AgentReportResponse{Accepted: true, Message: "ok"})
}

func bearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func StartHTTP(ctx context.Context, cfg config.MasterConfig, store *db.Store) error {
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: HTTPServer{Store: store, PublicAPIURL: cfg.PublicAPIURL}.Handler(),
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return fmt.Errorf("HTTP 服务启动失败: %w", err)
	}
}
