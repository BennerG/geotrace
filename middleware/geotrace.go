package geotrace

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Config struct {
	Endpoint string
	APIKey   string
	Timeout  time.Duration
}

type payload struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	StatusCode int    `json:"status_code"`
	UserAgent  string `json:"user_agent"`
}

// Middleware returns an http.Handler middleware that fires a non-blocking
// ingest request to the GeoTrace service for every request it handles.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}

	client := &http.Client{Timeout: cfg.Timeout}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			p := payload{
				Path:       r.URL.Path,
				Method:     r.Method,
				StatusCode: rw.status,
				UserAgent:  r.UserAgent(),
			}
			ip := realIP(r)

			go send(client, cfg, p, ip)
		})
	}
}

func send(client *http.Client, cfg Config, p payload, ip string) {
	b, err := json.Marshal(p)
	if err != nil {
		slog.Debug("geotrace: marshal failed", "err", err)
		return
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		cfg.Endpoint,
		bytes.NewReader(b),
	)
	if err != nil {
		slog.Debug("geotrace: build request failed", "err", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("X-GeoTrace-Key", cfg.APIKey)
	}
	if ip != "" {
		req.Header.Set("X-Real-IP", ip)
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("geotrace: ingest request failed", "err", err)
		return
	}
	resp.Body.Close()
}

// responseWriter wraps http.ResponseWriter to capture the written status code.
type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.status = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	host, _, _ := splitHostPort(r.RemoteAddr)
	return host
}

func splitHostPort(addr string) (string, string, error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return addr, "", nil
}
