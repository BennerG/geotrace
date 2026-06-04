// internal/store/event_test.go
package store_test

import (
	"net"
	"testing"
	"time"

	"github.com/BennerG/geotrace/internal/store"
)

// ptr helpers — avoid & on literals in test table rows
func ptrF(v float64) *float64 { return &v }
func ptrS(v string) *string   { return &v }
func ptrI(v int) *int         { return &v }

func TestEvent_IsPrivate(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v6", "::1", true},
		{"RFC1918 10/8", "10.4.20.1", true},
		{"RFC1918 172.16/12", "172.31.200.1", true},
		{"RFC1918 192.168/16", "192.168.1.100", true},
		{"link-local", "169.254.0.1", true},
		{"link-local v6", "fe80::1", true},
		{"public v4", "8.8.8.8", false},
		{"public v6", "2001:4860:4860::8888", false},
		{"Cloudflare", "1.1.1.1", false},
		{"Comcast v6", "2601:600:8880::1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &store.Event{IP: net.ParseIP(tc.ip)}
			if got := e.IsPrivate(); got != tc.want {
				t.Errorf("IsPrivate(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestEvent_GeoKnown(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		e := &store.Event{}
		if e.GeoKnown() {
			t.Error("expected GeoKnown=false when lat/lon are nil")
		}
	})

	t.Run("lat only", func(t *testing.T) {
		e := &store.Event{Lat: ptrF(47.6062)}
		if e.GeoKnown() {
			t.Error("expected GeoKnown=false when only lat is set")
		}
	})

	t.Run("both set", func(t *testing.T) {
		e := &store.Event{Lat: ptrF(47.6062), Lon: ptrF(-122.3321)}
		if !e.GeoKnown() {
			t.Error("expected GeoKnown=true when lat and lon are both set")
		}
	})
}

func TestEvent_ToGeoJSON(t *testing.T) {
	t.Run("no coordinates returns nil", func(t *testing.T) {
		e := &store.Event{IP: net.ParseIP("8.8.8.8")}
		if f := e.ToGeoJSON(); f != nil {
			t.Errorf("expected nil GeoJSON feature for event with no coords, got %+v", f)
		}
	})

	t.Run("full event converts correctly", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		e := &store.Event{
			ID:          42,
			IP:          net.ParseIP("8.8.8.8"),
			Lat:         ptrF(37.386),
			Lon:         ptrF(-122.0838),
			City:        ptrS("Mountain View"),
			Country:     ptrS("United States"),
			CountryCode: ptrS("US"),
			Path:        "/api/v1/health",
			Method:      "GET",
			StatusCode:  ptrI(200),
			CreatedAt:   now,
		}

		f := e.ToGeoJSON()
		if f == nil {
			t.Fatal("expected non-nil GeoJSON feature")
		}

		if f.Type != "Feature" {
			t.Errorf("Type = %q, want %q", f.Type, "Feature")
		}
		if f.Geometry.Type != "Point" {
			t.Errorf("Geometry.Type = %q, want %q", f.Geometry.Type, "Point")
		}

		// GeoJSON coordinates are [longitude, latitude] — not [lat, lon]
		// This is a common mistake. Mapbox will render in the wrong hemisphere
		// if these are swapped.
		if f.Geometry.Coordinates[0] != -122.0838 {
			t.Errorf("Coordinates[0] (lon) = %v, want %v", f.Geometry.Coordinates[0], -122.0838)
		}
		if f.Geometry.Coordinates[1] != 37.386 {
			t.Errorf("Coordinates[1] (lat) = %v, want %v", f.Geometry.Coordinates[1], 37.386)
		}

		if f.Props.City != "Mountain View" {
			t.Errorf("Props.City = %q, want %q", f.Props.City, "Mountain View")
		}
		if f.Props.CountryCode != "US" {
			t.Errorf("Props.CountryCode = %q, want %q", f.Props.CountryCode, "US")
		}
		if f.Props.IP != "8.8.8.8" {
			t.Errorf("Props.IP = %q, want %q", f.Props.IP, "8.8.8.8")
		}
	})

	t.Run("nil optional fields produce empty strings not panics", func(t *testing.T) {
		e := &store.Event{
			IP:  net.ParseIP("1.1.1.1"),
			Lat: ptrF(25.0),
			Lon: ptrF(55.0),
			// City, Country, CountryCode, StatusCode all nil
		}

		// Must not panic
		f := e.ToGeoJSON()
		if f == nil {
			t.Fatal("expected non-nil GeoJSON feature")
		}
		if f.Props.City != "" {
			t.Errorf("nil City should produce empty string, got %q", f.Props.City)
		}
		if f.Props.StatusCode != 0 {
			t.Errorf("nil StatusCode should produce 0, got %d", f.Props.StatusCode)
		}
	})
}

func TestIsInSubnet(t *testing.T) {
	cases := []struct {
		ip   string
		cidr string
		want bool
	}{
		// These mirror what Postgres << does for the same inputs
		{"10.4.20.1", "10.0.0.0/8", true},
		{"172.31.200.1", "172.16.0.0/12", true},
		{"192.168.1.100", "192.168.0.0/16", true},
		{"8.8.8.8", "10.0.0.0/8", false},
		{"8.8.8.8", "0.0.0.0/0", true},  // everything is in /0
		{"2601::1", "2601::/32", true},   // IPv6 subnet
		{"2602::1", "2601::/32", false},  // different block
	}

	for _, tc := range cases {
		t.Run(tc.ip+" in "+tc.cidr, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			got, err := store.IsInSubnet(ip, tc.cidr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("IsInSubnet(%s, %s) = %v, want %v", tc.ip, tc.cidr, got, tc.want)
			}
		})
	}
}

func TestQueryFilter_Defaults(t *testing.T) {
	// Ensure a zero QueryFilter with Limit=0 is legal (no limit applied)
	f := store.QueryFilter{
		From:  time.Now().Add(-1 * 24 * time.Hour),
		To:    time.Now(),
		Limit: 0,
	}

	if f.CountryCode != nil {
		t.Error("default CountryCode should be nil")
	}
	if f.SubnetFilter != nil {
		t.Error("default SubnetFilter should be nil")
	}
}
