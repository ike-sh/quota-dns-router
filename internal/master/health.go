package master

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"quota-dns-router-go/internal/db"
)

const healthCheckTimeout = 3 * time.Second

type HealthChecker struct {
	Store *db.Store
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h HealthChecker) liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (h HealthChecker) readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	checks := map[string]string{}
	status := "ok"
	httpStatus := http.StatusOK

	if h.Store == nil {
		checks["database"] = "store not configured"
		status = "unavailable"
		httpStatus = http.StatusServiceUnavailable
	} else if err := h.Store.Ping(ctx); err != nil {
		checks["database"] = err.Error()
		status = "unavailable"
		httpStatus = http.StatusServiceUnavailable
	} else {
		checks["database"] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: status, Checks: checks})
}
