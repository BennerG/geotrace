package geotrace_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	geotrace "github.com/BennerG/geotrace/middleware"
)

func TestMiddleware_PassesThrough(t *testing.T) {
	var called bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()

	wrapped := geotrace.Middleware(geotrace.Config{
		Endpoint: ingest.URL,
		Timeout:  500 * time.Millisecond,
	})(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if !called {
		t.Error("upstream handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestMiddleware_CapturesStatusCode(t *testing.T) {
	done := make(chan int, 1)

	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			StatusCode int `json:"status_code"`
		}
		json.NewDecoder(r.Body).Decode(&p)
		done <- p.StatusCode
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	wrapped := geotrace.Middleware(geotrace.Config{
		Endpoint: ingest.URL,
		Timeout:  500 * time.Millisecond,
	})(handler)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	capturedStatus := <-done

	if capturedStatus != http.StatusNotFound {
		t.Errorf("want status 404 in payload, got %d", capturedStatus)
	}
}

func TestMiddleware_SendsAPIKey(t *testing.T) {
	done := make(chan string, 1)

	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- r.Header.Get("X-GeoTrace-Key")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()

	wrapped := geotrace.Middleware(geotrace.Config{
		Endpoint: ingest.URL,
		APIKey:   "test-secret",
		Timeout:  500 * time.Millisecond,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	receivedKey := <-done

	if receivedKey != "test-secret" {
		t.Errorf("want X-GeoTrace-Key=test-secret, got %q", receivedKey)
	}
}

func TestMiddleware_ForwardsRealIP(t *testing.T) {
	done := make(chan string, 1)

	ingest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- r.Header.Get("X-Real-IP")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingest.Close()

	wrapped := geotrace.Middleware(geotrace.Config{
		Endpoint: ingest.URL,
		Timeout:  500 * time.Millisecond,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	receivedIP := <-done

	if receivedIP != "203.0.113.5" {
		t.Errorf("want X-Real-IP=203.0.113.5, got %q", receivedIP)
	}
}
