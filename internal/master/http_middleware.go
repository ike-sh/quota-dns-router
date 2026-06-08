package master

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxRequestBodyBytes = 64 << 10

type joinRateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	limit   int
	window  time.Duration
}

func newJoinRateLimiter(limit int, window time.Duration) *joinRateLimiter {
	return &joinRateLimiter{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
}

func (l *joinRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.purgeLocked(now)
	return l.allowLocked(key, now)
}

func (l *joinRateLimiter) allowLocked(key string, now time.Time) bool {
	cutoff := now.Add(-l.window)
	times := l.entries[key]
	filtered := times[:0]
	for _, ts := range times {
		if ts.After(cutoff) {
			filtered = append(filtered, ts)
		}
	}
	if len(filtered) == 0 {
		delete(l.entries, key)
	} else {
		l.entries[key] = filtered
	}
	if len(filtered) >= l.limit {
		return false
	}
	filtered = append(filtered, now)
	l.entries[key] = filtered
	return true
}

func (l *joinRateLimiter) purgeLocked(now time.Time) {
	if len(l.entries) < 256 {
		return
	}
	cutoff := now.Add(-l.window)
	for key, times := range l.entries {
		filtered := times[:0]
		for _, ts := range times {
			if ts.After(cutoff) {
				filtered = append(filtered, ts)
			}
		}
		if len(filtered) == 0 {
			delete(l.entries, key)
			continue
		}
		l.entries[key] = filtered
	}
}

func isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if isTrustedProxy(host) {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); ip != "" {
					return ip
				}
			}
		}
		if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
			return xri
		}
	}
	return host
}

func withMaxBodyBytes(next http.HandlerFunc, limit int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			http.Error(w, "请求体过大", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next(w, r)
	}
}

func withAccessLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", clientIP(r),
		)
	}
}

func withJoinRateLimit(limiter *joinRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return withRateLimit(limiter, "join", clientIP, next)
}

func withReportRateLimit(limiter *joinRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return withRateLimit(limiter, "report", reportRateLimitKey, next)
}

func withRateLimit(limiter *joinRateLimiter, kind string, keyFn func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := keyFn(r)
		if key == "" {
			http.Error(w, "无法识别客户端", http.StatusBadRequest)
			return
		}
		now := time.Now()
		if !limiter.allow(key, now) {
			slog.Warn(kind+" rate limit exceeded", "key", key, "remote_ip", clientIP(r))
			http.Error(w, "请求过于频繁，请稍后再试", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func reportRateLimitKey(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return "token:" + token
	}
	return "ip:" + clientIP(r)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
