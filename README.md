# GeoTrace

Real-time visitor geolocation map: [live demo](https://geotrace.bennerg.com)

A self-hosted request geo-analytics service. Embed one middleware line in any Go HTTP server and get a live map of where your traffic is coming from.

![GeoTrace Map](docs/images/geotrace-map.png)

## Features

- Captures every HTTP request passing through the middleware
- Enriches each request with city/country/region via MaxMind GeoLite2 (in-process, <1ms)
- Stores events in Postgres with full INET type support for subnet queries
- Streams enriched events to connected dashboards over WebSocket in real time
- Serves a React dashboard with a Mapbox GL globe, time-window scrubber, and live stats

## Architecture

![GeoTrace data flow diagram](docs/images/geotrace-dataflow.svg)

**Two data paths:**

![Two data paths](docs/images/geotrace-two-paths.svg)

## Embedding the middleware

```go
import geotrace "github.com/BennerG/geotrace/middleware"

r.Use(geotrace.Middleware(geotrace.Config{
    Endpoint: "https://geotrace.example.com/ingest",
    APIKey:   os.Getenv("GEOTRACE_KEY"),
}))
```

The middleware wraps the handler, captures the response status code, and fires a non-blocking goroutine POST to the ingest endpoint after the response is written. Zero latency impact on the upstream handler.

## Stack

| Layer          | Technology                                               |
| -------------- | -------------------------------------------------------- |
| Backend        | Go 1.26, Chi router                                      |
| Concurrency    | goroutines, channels, errgroup                           |
| Geo enrichment | MaxMind GeoLite2-City (in-process)                       |
| Database       | PostgreSQL — INET type, GIST index for subnet queries    |
| Real-time      | WebSocket (gorilla/websocket), hub pattern               |
| Frontend       | React 18, TypeScript, Mapbox GL JS, styled-components    |
| Build          | Vite — embedded in Go binary via go:embed for production |

## Quick start

**Prerequisites:**

- Go 1.26+
- Node 24+
- Docker
- PostgreSQL client 16+
- [MaxMind account](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) - download free GeoLite2-City database and copy it to the root directory
- [Mapbox account](https://account.mapbox.com/auth/signup/) - create a public token and add it to `web/.env.local` as `VITE_MAPBOX_TOKEN=pk.your_token`

```bash
# 1. Clone and enter the repo
git clone https://github.com/BennerG/geotrace
cd geotrace

# 2. Start Postgres
make docker-db

# 3. Configure environment
cp .env.example .env
# set DATABASE_DSN and MAXMIND_DB_PATH
cp web/.env.local.example web/.env.local
# set VITE_MAPBOX_TOKEN

# 4. Run the backend (migrations run automatically on startup)
go run ./cmd/server

# 5. In a separate terminal, run the frontend dev server
cd web && npm install && npm run dev
```

Open `http://localhost:5173` in a browser and fire a test event in a new terminal:

```bash
curl -X POST localhost:8090/ingest \
  -H "Content-Type: application/json" \
  -H "X-Real-IP: 8.8.8.8" \
  -d '{"path":"/","method":"GET","status_code":200}'
```

A green pin should appear near Mountain View, CA.

> I have been getting Wichita, KS using my MaxMind database. I've also seen `8.8.8.8` route near Ashburn, VA, so your results may vary with Google's public DNS server.

## API

| Method | Path                | Description                                     |
| ------ | ------------------- | ----------------------------------------------- |
| `POST` | `/ingest`           | Receive a request event from the middleware     |
| `GET`  | `/events?from=&to=` | GeoJSON FeatureCollection for a time window     |
| `GET`  | `/stats?from=&to=`  | Country breakdown and req/min for a time window |
| `GET`  | `/summary?ip=`      | Top paths grouped by method for a given IP      |
| `GET`  | `/ws`               | WebSocket endpoint for live event stream        |
| `GET`  | `/health`           | Liveness check with Postgres ping               |

Time parameters are RFC3339 strings: `2026-01-01T00:00:00Z`

## Postgres notes

The `ip` column uses Postgres `INET` type rather than `TEXT`, which enables subnet comparison operators:

```sql
-- all requests from a specific CIDR block
WHERE ip << '192.0.2.0/24'::inet

-- exclude private/internal traffic
WHERE NOT (
    ip << '10.0.0.0/8'::inet OR
    ip << '172.16.0.0/12'::inet OR
    ip << '192.168.0.0/16'::inet
)
```

Subnet queries use a GIST index on the `ip` column. Time-window queries use a B-tree index on `created_at DESC`.

## Running tests

```bash
# all packages
go test -race ./...

# store package only (no database required)
go test -race ./internal/store/...

# middleware package only
go test -race ./middleware/...
```

## Production build

```bash
# build the frontend and embed it in the Go binary
cd web && npm run build
cd .. && go build -o bin/geotrace ./cmd/server

./bin/geotrace
```

## Environment variables

| Variable           | Required | Default        | Description                      |
| ------------------ | -------- | -------------- | -------------------------------- |
| `DATABASE_DSN`     | ✅       | —              | Postgres connection string       |
| `MAXMIND_DB_PATH`  | ✅       | —              | Path to GeoLite2-City.mmdb       |
| `PORT`             |          | `8090`         | HTTP server port                 |
| `MIGRATIONS_DIR`   |          | `./migrations` | Path to SQL migration files      |
| `INGEST_API_KEY`   |          | _(none)_       | Shared secret for `/ingest` auth |
| `ENRICHER_WORKERS` |          | `4`            | Geo-enrichment goroutine count   |
| `CHANNEL_BUFFER`   |          | `512`          | Ingest channel buffer size       |
| `CORS_ORIGINS`     |          | `*`            | Allowed CORS origin              |

## Local Development

### PostgreSQL commands

```bash
# set shell environment variable
DATABASE_URL=postgres://geotrace:password@localhost:5432/geotrace

# check events table
psql $DATABASE_URL -c "SELECT id, host(ip), country_code, path, created_at FROM events ORDER BY id DESC LIMIT 5;"

# check summary for ip - replace generic ip with target ip
psql $DATABASE_URL -c "SELECT method, path, COUNT(*) AS count \
FROM events \
WHERE ip = '8.8.8.8'::inet \
GROUP BY method, path \
ORDER BY count DESC;"
```

### curl commands

```bash
# create test ingest
curl -X POST localhost:8090/ingest \
  -H "Content-Type: application/json" \
  -d '{"path":"/test","method":"GET","status_code":200}'

# install jq to pipe output into pretty json format
# get events
curl "localhost:8090/events?from=2024-01-01T00:00:00Z&to=2026-12-31T00:00:00Z" | jq .
# get stats
curl "localhost:8090/stats?from=2024-01-01T00:00:00Z&to=2026-12-31T00:00:00Z" | jq .
```

### test websocket

> Ensure you have `websocat` installed with the package manager of your choice.

```bash
# Start server, and in a separate shell window, run:
websocat ws://localhost:8090/ws

# In another terminal, run:
curl -X POST localhost:8090/ingest \
  -H "Content-Type: application/json" \
  -H "X-Real-IP: 8.8.8.8" \
  -d '{"path":"/home","method":"GET","status_code":200}'

# In the websocat terminal, you should see a new GeoJSON feature appear.
```

## Design Decisions

- INET over TEXT for IP storage — enables `<<` subnet operator and GIST index, not just a cosmetic choice
- In-process MaxMind lookup over API — no rate limits, no network latency, sub-millisecond, trades disk space for speed
- Buffered channel with drop-on-full over backpressure — ingest latency must never affect the upstream server, so events are dropped rather than blocking
- Single hub goroutine over mutex-protected map — eliminates lock contention entirely, slow clients are dropped rather than stalling the broadcast
- Embedded React build over separate static server — one binary, one deployment unit, no nginx config to keep in sync with API routes
- IPv4 normalization at ingest — Go's `net.IP` can represent IPv4 as IPv4-mapped IPv6 (`::ffff:x.x.x.x`), normalizing to 4-byte form at write time keeps queries consistent

## What's Next

- pprof endpoint on internal port for goroutine and heap profiling in production
- CORS origin restriction from \* to specific domain in production
- Rate limiting on `/ingest` to prevent event flooding
- Retention policy — Postgres `DELETE FROM events WHERE created_at < NOW() - INTERVAL '90 days'` as a cron job
- Light map style toggle (Mapbox `light-v11`)
- Export events to CSV from the dashboard time window
