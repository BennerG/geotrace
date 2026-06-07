# GeoTrace

## Local Development

### PostgreSQL commands

```bash
# set shell environment variable
DATABASE_URL=postgres://geotrace:password@localhost:5432/geotrace

# check events table
psql $DATABASE_URL -c "SELECT id, host(ip), country_code, path, created_at FROM events ORDER BY id DESC LIMIT 5;"
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

Start server, and in a separate shell window, run:

```bash
websocat ws://localhost:8090/ws
```

In another separate shell window, run:

```bash
curl -X POST localhost:8090/ingest \
  -H "Content-Type: application/json" \
  -H "X-Real-IP: 8.8.8.8" \
  -d '{"path":"/home","method":"GET","status_code":200}'
```

In the websocat terminal, you should see a new GeoJSON feature appear.
