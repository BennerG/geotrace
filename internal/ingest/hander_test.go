package ingest_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BennerG/geotrace/internal/ingest"
	"github.com/BennerG/geotrace/internal/store"
)

func makeEvents() chan *store.Event {
	return make(chan *store.Event, 64)
}

func post(h http.Handler, body any, headers map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "8.8.8.8:12345"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// auth tests
func TestHandler_NoAuth_Accepts(t *testing.T) {
	h := ingest.New("", makeEvents())
	rr := post(h, ingest.IngestPayload{Path: "/", Method: "GET"}, nil)
	if rr.Code != http.StatusAccepted {
		t.Errorf("want 202, got %d", rr.Code)
	}
}

func TestHandler_Auth_WrongKey_Rejects(t *testing.T) {
	h := ingest.New("secret", makeEvents())
	rr := post(h, ingest.IngestPayload{Path: "/"}, map[string]string{
		"X-GeoTrace-Key": "wrong",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestHandler_Auth_CorrectKey_Accepts(t *testing.T) {
	h := ingest.New("secret", makeEvents())
	rr := post(h, ingest.IngestPayload{Path: "/"}, map[string]string{
		"X-GeoTrace-Key": "secret",
	})
	if rr.Code != http.StatusAccepted {
		t.Errorf("want 202, got %d", rr.Code)
	}
}

// IP extraction tests
func TestHandler_IPFromRemoteAddr(t *testing.T) {
	events := makeEvents()
	h := ingest.New("", events)

	b, _ := json.Marshal(ingest.IngestPayload{Path: "/test", Method: "GET"})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "1.2.3.4:9999"

	httptest.NewRecorder() // discard response
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rr.Code)
	}

	ev := <-events
	if ev.IP.String() != "1.2.3.4" {
		t.Errorf("IP = %q, want %q", ev.IP.String(), "1.2.3.4")
	}
}

func TestHandler_IPFromXRealIP(t *testing.T) {
	events := makeEvents()
	h := ingest.New("", events)

	b, _ := json.Marshal(ingest.IngestPayload{Path: "/"})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Real-IP", "203.0.113.5")
	req.RemoteAddr = "10.0.0.1:1234" // proxy IP — should be ignored

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	ev := <-events
	if ev.IP.String() != "203.0.113.5" {
		t.Errorf("IP = %q, want %q from X-Real-IP", ev.IP.String(), "203.0.113.5")
	}
}

func TestHandler_IPFromXForwardedFor_TakesLeftmost(t *testing.T) {
	events := makeEvents()
	h := ingest.New("", events)

	b, _ := json.Marshal(ingest.IngestPayload{Path: "/"})
	req := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	// leftmost = original client, rightmost = last proxy
	req.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1, 172.16.0.5")
	req.RemoteAddr = "10.0.0.1:1234"

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	ev := <-events
	if ev.IP.String() != "198.51.100.7" {
		t.Errorf("IP = %q, want leftmost %q", ev.IP.String(), "198.51.100.7")
	}
}

// payload field tests
func TestHandler_PathNormalization(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/api/v1", "/api/v1"},
		{"api/v1", "/api/v1"}, // missing leading slash
	}

	for _, tc := range cases {
		events := makeEvents()
		h := ingest.New("", events)
		post(h, ingest.IngestPayload{Path: tc.input}, nil)
		ev := <-events
		if ev.Path != tc.want {
			t.Errorf("path %q → %q, want %q", tc.input, ev.Path, tc.want)
		}
	}
}

func TestHandler_MethodNormalization(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"get", "GET"},
		{"POST", "POST"},
		{"", "GET"}, // fallback
	}

	for _, tc := range cases {
		events := makeEvents()
		h := ingest.New("", events)
		post(h, ingest.IngestPayload{Method: tc.input}, nil)
		ev := <-events
		if ev.Method != tc.want {
			t.Errorf("method %q → %q, want %q", tc.input, ev.Method, tc.want)
		}
	}
}

func TestHandler_ChannelFull_Returns202AnyWay(t *testing.T) {
	// channel with zero buffer is always "full"
	events := make(chan *store.Event, 0)
	h := ingest.New("", events)

	// should return 202 even though the event is dropped (non-blocking send)
	rr := post(h, ingest.IngestPayload{Path: "/"}, nil)
	if rr.Code != http.StatusAccepted {
		t.Errorf("want 202 even when channel full, got %d", rr.Code)
	}
}
