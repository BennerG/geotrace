package api

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/BennerG/geotrace/internal/store"
)

type Handler struct {
	st *store.Store
}

func New(st *store.Store) *Handler {
	return &Handler{st: st}
}

type featureCollection struct {
	Type     string                  `json:"type"`
	Features []*store.GeoJSONFeature `json:"features"`
}

type statsResponse struct {
	Countries     map[string]int `json:"countries"`
	ReqPerMin     int            `json:"req_per_min"`
	TotalInWindow int            `json:"total_in_window"`
}

func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseWindow(r)
	if !ok {
		http.Error(w, "invalid or missing from/to params (RFC3339)", http.StatusBadRequest)
		return
	}

	events, err := h.st.Query(r.Context(), store.QueryFilter{
		From:  from,
		To:    to,
		Limit: 5000,
	})
	if err != nil {
		slog.Error("api: query failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	fc := featureCollection{
		Type:     "FeatureCollection",
		Features: make([]*store.GeoJSONFeature, 0, len(events)),
	}

	for i := range events {
		f := events[i].ToGeoJSON()
		if f != nil {
			fc.Features = append(fc.Features, f)
		}
	}

	writeJSON(w, fc)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseWindow(r)
	if !ok {
		http.Error(w, "invalid or missing from/to params (RFC3339)", http.StatusBadRequest)
		return
	}

	f := store.QueryFilter{From: from, To: to}

	countries, err := h.st.CountByCountry(r.Context(), f)
	if err != nil {
		slog.Error("api: count by country failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	recentCount, err := h.st.RecentCount(r.Context(), 60)
	if err != nil {
		slog.Error("api: recent count failed", "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	total := 0
	for _, n := range countries {
		total += n
	}

	writeJSON(w, statsResponse{
		Countries:     countries,
		ReqPerMin:     recentCount,
		TotalInWindow: total,
	})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		http.Error(w, "ip param required", http.StatusBadRequest)
		return
	}

	// validate ip format
	if net.ParseIP(ip) == nil {
		http.Error(w, "invalid ip address", http.StatusBadRequest)
		return
	}

	paths, err := h.st.SummaryByIP(r.Context(), ip)
	if err != nil {
		slog.Error("api: summary by ip failed", "ip", ip, "err", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	if paths == nil {
		paths = []store.PathCount{}
	}

	writeJSON(w, paths)
}

func parseWindow(r *http.Request) (from, to time.Time, ok bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}

	to, err = time.Parse(time.RFC3339, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, false
	}

	return from, to, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: encode response", "err", err)
	}
}
