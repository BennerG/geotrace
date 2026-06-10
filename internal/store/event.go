package store

import (
	"net"
	"time"
)

// Event represents a single tracked HTTP request, fully geo-enriched
type Event struct {
	ID          int64     `json:"id"           db:"id"`
	IP          net.IP    `json:"ip"           db:"ip"`  // INET in Postgres
	Lat         *float64  `json:"lat"          db:"lat"` // nil if geo lookup failed
	Lon         *float64  `json:"lon"          db:"lon"` // nil if geo lookup failed
	City        *string   `json:"city"         db:"city"`
	Region      *string   `json:"region"       db:"region"`
	Country     *string   `json:"country"      db:"country"`
	CountryCode *string   `json:"country_code" db:"country_code"` // ISO 3166-1 alpha-2
	Path        string    `json:"path"         db:"path"`
	Method      string    `json:"method"       db:"method"`
	UserAgent   *string   `json:"user_agent"   db:"user_agent"`
	StatusCode  *int      `json:"status_code"  db:"status_code"`
	CreatedAt   time.Time `json:"created_at"   db:"created_at"`
}

// IsPrivate reports whether the event's IP is a private/loopback/link-local
// address. Private IPs are captured and stored but never geo-enriched
func (e *Event) IsPrivate() bool {
	return e.IP.IsPrivate() || e.IP.IsLoopback() || e.IP.IsLinkLocalUnicast()
}

// GeoKnown reports whether this event has valid geo coordinates.
func (e *Event) GeoKnown() bool {
	return e.Lat != nil && e.Lon != nil
}

// QueryFilter defines the parameters for a time-windowed event query.
// The From/To pair maps directly to the WHERE created_at BETWEEN $1 AND $2 clause.
type QueryFilter struct {
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	CountryCode *string   `json:"country_code,omitempty"` // optional country filter
	// SubnetFilter, if non-nil, adds: AND ip << $N::inet
	// Enables things like "show only requests from Comcast's IPv6 block"
	SubnetFilter *net.IPNet `json:"subnet_filter,omitempty"`
	Limit        int        `json:"limit"` // 0 = no limit
}

// GeoJSONFeature is the wire format for a single event sent to the React map.
// Mapbox GL JS expects this exact structure for point layers.
type GeoJSONFeature struct {
	Type     string          `json:"type"` // always "Feature"
	Geometry GeoJSONGeometry `json:"geometry"`
	Props    EventProps      `json:"properties"`
}

// GeoJSONGeometry is a GeoJSON Point geometry.
type GeoJSONGeometry struct {
	Type        string     `json:"type"`        // always "Point"
	Coordinates [2]float64 `json:"coordinates"` // [lon, lat] — GeoJSON is lon-first
}

// EventProps is the properties object on each GeoJSON feature.
// These become available in Mapbox GL JS expressions and popups.
type EventProps struct {
	ID          int64  `json:"id"`
	City        string `json:"city"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Path        string `json:"path"`
	Method      string `json:"method"`
	StatusCode  int    `json:"status_code"`
	CreatedAt   string `json:"created_at"` // RFC3339 string for JS Date parsing
	IP          string `json:"ip"`         // host(ip)::text — never expose raw INET to frontend
}

// ToGeoJSON converts a fully-enriched Event to a GeoJSON Feature.
// Returns nil if the event has no coordinates (private IP, failed lookup).
func (e *Event) ToGeoJSON() *GeoJSONFeature {
	if !e.GeoKnown() {
		return nil
	}

	props := EventProps{
		ID:        e.ID,
		Path:      e.Path,
		Method:    e.Method,
		CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		IP:        normalizeIP(e.IP),
	}
	if e.City != nil {
		props.City = *e.City
	}
	if e.Country != nil {
		props.Country = *e.Country
	}
	if e.CountryCode != nil {
		props.CountryCode = *e.CountryCode
	}
	if e.StatusCode != nil {
		props.StatusCode = *e.StatusCode
	}

	return &GeoJSONFeature{
		Type: "Feature",
		Geometry: GeoJSONGeometry{
			Type:        "Point",
			Coordinates: [2]float64{*e.Lon, *e.Lat}, // GeoJSON is [lon, lat]
		},
		Props: props,
	}
}

func normalizeIP(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}
