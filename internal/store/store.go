// internal/store/store.go
// Package store manages the Postgres connection pool and all event queries.
// It is the only package that knows about the database schema.
package store

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store wraps a pgxpool.Pool and exposes typed methods for all database
// operations. Callers never touch SQL directly — they go through Store.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store backed by a connection pool pointed at the given DSN.
// The pool is configured for GeoTrace's workload: mostly writes (ingest) with
// periodic burst reads (dashboard time-window queries).
//
// DSN format: "postgres://user:pass@host:5432/dbname?sslmode=disable"
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}

	// Pool sizing rationale:
	// - MinConns=2: keep two connections warm so the first ingest after idle
	//   doesn't pay connection setup latency.
	// - MaxConns=10: enough for concurrent dashboard scrubber requests + ingest
	//   writes without exhausting a $6 Postgres instance.
	cfg.MinConns = 2
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("store: ping: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close releases all pool connections. Call on shutdown.
func (s *Store) Close() {
	s.pool.Close()
}

// Pool returns the underlying pgxpool.Pool for use by packages that need
// direct pool access (e.g. the migration runner).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Ping verifies the database connection is alive. Used by the health endpoint.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Insert writes a single Event to the events table.
// The event's IP is stored as INET — pgx handles net.IP ↔ INET transparently.
// ID and CreatedAt are set by Postgres defaults and scanned back into e.
//
// Insert is called from the enricher goroutine after geo lookup completes.
// It must be fast — the enricher channel backs up if inserts are slow.
func (s *Store) Insert(ctx context.Context, e *Event) error {
	const q = `
		INSERT INTO events (
			ip, lat, lon,
			city, region, country, country_code,
			path, method, user_agent, status_code
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, $9, $10, $11
		)
		RETURNING id, created_at`

	return s.pool.QueryRow(ctx, q,
		e.IP,          // $1  INET — pgx encodes net.IP correctly
		e.Lat,         // $2  DOUBLE PRECISION, nullable
		e.Lon,         // $3  DOUBLE PRECISION, nullable
		e.City,        // $4  TEXT, nullable
		e.Region,      // $5  TEXT, nullable
		e.Country,     // $6  TEXT, nullable
		e.CountryCode, // $7  CHAR(2), nullable
		e.Path,        // $8  TEXT
		e.Method,      // $9  TEXT
		e.UserAgent,   // $10 TEXT, nullable
		e.StatusCode,  // $11 INT, nullable
	).Scan(&e.ID, &e.CreatedAt)
}

// Query returns events within the time window defined by f.
// Results are ordered newest-first (matching the DESC index).
// The SubnetFilter, if set, adds: AND ip << $N — uses the GIST index.
//
// This is the hot path for the dashboard time scrubber.
func (s *Store) Query(ctx context.Context, f QueryFilter) ([]Event, error) {
	var (
		args   []any
		wheres []string
		idx    = 1 // Postgres $N placeholder counter
	)

	// Time window — always required
	wheres = append(wheres, fmt.Sprintf("created_at >= $%d", idx))
	args = append(args, f.From)
	idx++

	wheres = append(wheres, fmt.Sprintf("created_at <= $%d", idx))
	args = append(args, f.To)
	idx++

	// Optional country filter — uses idx_events_country_code
	if f.CountryCode != nil {
		wheres = append(wheres, fmt.Sprintf("country_code = $%d", idx))
		args = append(args, *f.CountryCode)
		idx++
	}

	// Optional subnet filter — uses idx_events_ip_gist
	// Postgres << operator: "ip is contained within subnet"
	// e.g. ip << '10.0.0.0/8'::inet returns true for any RFC1918 address
	if f.SubnetFilter != nil {
		wheres = append(wheres, fmt.Sprintf("ip << $%d", idx))
		args = append(args, f.SubnetFilter.String())
		idx++
	}

	q := fmt.Sprintf(`
		SELECT
			id, ip, lat, lon,
			city, region, country, country_code,
			path, method, user_agent, status_code,
			created_at
		FROM events
		WHERE %s
		ORDER BY created_at DESC`,
		strings.Join(wheres, " AND "),
	)

	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, scanEvent)
}

// CountByCountry returns a map of country_code → request count
// for the given time window. Used by the StatsBar component.
func (s *Store) CountByCountry(ctx context.Context, f QueryFilter) (map[string]int, error) {
	const q = `
		SELECT
			COALESCE(country_code, 'XX') AS cc,
			COUNT(*)                      AS n
		FROM events
		WHERE created_at >= $1
		  AND created_at <= $2
		GROUP BY cc
		ORDER BY n DESC
		LIMIT 20`

	rows, err := s.pool.Query(ctx, q, f.From, f.To)
	if err != nil {
		return nil, fmt.Errorf("store: count by country: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var cc string
		var n int
		if err := rows.Scan(&cc, &n); err != nil {
			return nil, err
		}
		out[cc] = n
	}
	return out, rows.Err()
}

// RecentCount returns the number of events in the last n seconds.
// Used for the req/min sparkline in the StatsBar.
func (s *Store) RecentCount(ctx context.Context, lastSeconds int) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM events
		WHERE created_at >= NOW() - ($1 || ' seconds')::INTERVAL`

	var count int
	err := s.pool.QueryRow(ctx, q, lastSeconds).Scan(&count)
	return count, err
}

// IsInSubnet is a helper that demonstrates the << INET operator via Go.
// For direct Postgres subnet queries, use QueryFilter.SubnetFilter instead.
// This is useful for application-level filtering after a Query call.
func IsInSubnet(ip net.IP, cidr string) (bool, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, fmt.Errorf("store: parse cidr %q: %w", cidr, err)
	}
	return subnet.Contains(ip), nil
}

// scanEvent is the pgx row scanner for the Event type.
// It handles the INET→net.IP conversion that pgx provides automatically
// when scanning into *net.IP — we just need to give it the right pointer type.
func scanEvent(row pgx.CollectableRow) (Event, error) {
	var e Event
	err := row.Scan(
		&e.ID,
		&e.IP,          // pgx decodes INET → net.IP
		&e.Lat,
		&e.Lon,
		&e.City,
		&e.Region,
		&e.Country,
		&e.CountryCode,
		&e.Path,
		&e.Method,
		&e.UserAgent,
		&e.StatusCode,
		&e.CreatedAt,
	)
	return e, err
}
