package ingest

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/BennerG/geotrace/internal/store"
)

type Handler struct {
	apiKey string              // empty = unauthenticated (dev mode)
	events chan<- *store.Event // send-only; owned by the enricher
}

func New(apiKey string, events chan<- *store.Event) *Handler {
	return &Handler{
		apiKey: apiKey,
		events: events,
	}
}

// JSON body sent by the middleware package
type IngestPayload struct {
	Path       string `json:"path"`
	Method     string `json:"method"`
	StatusCode int    `json:"status_code"`
	UserAgent  string `json:"user_agent"`
}

// ServeHTTP handles POST /ingest
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// auth
	if h.apiKey != "" {
		key := r.Header.Get("X-GeoTrace-Key")
		if key != h.apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// parse body
	var payload IngestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// extract real client IP
	ip := realIP(r)
	if ip == nil {
		http.Error(w, "could not determine client IP", http.StatusBadRequest)
		return
	}

	// build raw event
	ua := payload.UserAgent
	if ua == "" {
		ua = r.UserAgent()
	}
	sc := payload.StatusCode

	ev := &store.Event{
		IP:        ip,
		Path:      normalizePath(payload.Path),
		Method:    normalizeMethod(payload.Method),
		UserAgent: strPtr(ua),
		StatusCode: func() *int {
			if sc == 0 {
				return nil
			}
			return &sc
		}(),
	}

	// drop onto enricher channel (non-blocking)
	select {
	case h.events <- ev:
		// accepted
	default:
		slog.Warn("ingest: channel full, dropping event",
			"ip", ip.String(),
			"path", ev.Path,
		)
	}

	// 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// extracts the original client IP from the request
func realIP(r *http.Request) net.IP {
	// X-Real-IP is set by proxies that forward exactly one IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
			return ip
		}
	}

	// X-Forwarded-For may contain a chain: "client, proxy1, proxy2"
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) entry
		parts := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip
		}
	}

	// fall back to RemoteAddr — strip the port first
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr with no port (unusual but possible in tests)
		if ip := net.ParseIP(r.RemoteAddr); ip != nil {
			return ip
		}
		return nil
	}
	return net.ParseIP(host)
}

// ensures the path is non-empty and starts with /
func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// normalizeMethod uppercases the HTTP method and falls back to GET
func normalizeMethod(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		return "GET"
	}
	return m
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
