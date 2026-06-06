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
```
