package store

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

// creates a Store backed by a connection pool
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}

	// keep 2 connections warm while providing enough for db reads
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

// releases all pool connections
func (s *Store) Close() {
	s.pool.Close()
}

// returns the underlying pgxpool.Pool
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// verifies the database connection is alive
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// writes a single Event to the events table
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
		e.IP,          // $1  INET; pgx encodes net.IP correctly
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

// returns events within the time window defined by f, DESC
func (s *Store) Query(ctx context.Context, f QueryFilter) ([]Event, error) {
	var (
		args   []any
		wheres []string
		idx    = 1 // Postgres $N placeholder counter
	)

	// time window, required
	wheres = append(wheres, fmt.Sprintf("created_at >= $%d", idx))
	args = append(args, f.From)
	idx++

	wheres = append(wheres, fmt.Sprintf("created_at <= $%d", idx))
	args = append(args, f.To)
	idx++

	// optional country filter uses idx_events_country_code
	if f.CountryCode != nil {
		wheres = append(wheres, fmt.Sprintf("country_code = $%d", idx))
		args = append(args, *f.CountryCode)
		idx++
	}

	// optional subnet filter uses idx_events_ip_gist
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

// returns a map of country_code: request count
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

// returns the number of events in the last n seconds
func (s *Store) RecentCount(ctx context.Context, lastSeconds int) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM events
		WHERE created_at >= NOW() - make_interval(secs => $1)`

	var count int
	err := s.pool.QueryRow(ctx, q, lastSeconds).Scan(&count)
	return count, err
}

// helper used in application-level filtering. Use QueryFilter.SubnetFilter for direct db queries
func IsInSubnet(ip net.IP, cidr string) (bool, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, fmt.Errorf("store: parse cidr %q: %w", cidr, err)
	}
	return subnet.Contains(ip), nil
}

// scanEvent is the pgx row scanner for the Event type
func scanEvent(row pgx.CollectableRow) (Event, error) {
	var e Event
	err := row.Scan(
		&e.ID,
		&e.IP, // pgx decodes INET to net.IP
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
