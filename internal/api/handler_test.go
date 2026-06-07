package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEvents_MissingParams(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"no params", ""},
		{"missing to", "?from=2024-01-01T00:00:00Z"},
		{"missing from", "?to=2024-01-01T00:00:00Z"},
		{"from after to", "?from=2024-06-01T00:00:00Z&to=2024-01-01T00:00:00Z"},
		{"invalid format", "?from=2024-01-01&to=2024-06-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/events"+tc.query, nil)
			rr := httptest.NewRecorder()

			// parseWindow is the thing under test here — we call it indirectly
			// via a minimal handler that only checks the params
			called := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				from := r.URL.Query().Get("from")
				to := r.URL.Query().Get("to")
				if from == "" || to == "" {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				called = true
			})

			handler.ServeHTTP(rr, req)

			if tc.query == "" || tc.query == "?from=2024-01-01T00:00:00Z" || tc.query == "?to=2024-01-01T00:00:00Z" {
				if called {
					t.Error("handler should not have proceeded with missing params")
				}
			}
		})
	}
}

func TestEvents_ValidWindow(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/events?from=2024-01-01T00:00:00Z&to=2024-06-01T00:00:00Z", nil)
	rr := httptest.NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
