package master

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJoinRateLimiterBlocksBurst(t *testing.T) {
	limiter := newJoinRateLimiter(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !limiter.allow("203.0.113.10", now) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if limiter.allow("203.0.113.10", now) {
		t.Fatal("expected fourth request to be blocked")
	}
}

func TestJoinRateLimiterIsolatesIPs(t *testing.T) {
	limiter := newJoinRateLimiter(1, time.Minute)
	now := time.Now()
	if !limiter.allow("203.0.113.10", now) {
		t.Fatal("expected first ip allowed")
	}
	if !limiter.allow("198.51.100.20", now) {
		t.Fatal("expected second ip allowed")
	}
}

func TestMaxBodyBytesRejectsLargePayload(t *testing.T) {
	handler := withMaxBodyBytes(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}, 32)

	body := strings.Repeat("a", 64)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/join", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestMaxBodyBytesRejectsOversizedRead(t *testing.T) {
	handler := withMaxBodyBytes(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		_, _ = w.Write([]byte("ok"))
	}, 32)

	req := httptest.NewRequest(http.MethodPost, "/api/agent/join", strings.NewReader(strings.Repeat("a", 64)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 on read overflow, got %d", rec.Code)
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 198.51.100.20")
	if got := clientIP(req); got != "203.0.113.10" {
		t.Fatalf("got %q", got)
	}
}
