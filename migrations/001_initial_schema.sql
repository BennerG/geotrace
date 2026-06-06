CREATE TABLE IF NOT EXISTS events (
    id           BIGSERIAL        PRIMARY KEY,
    ip           INET             NOT NULL,
    lat          DOUBLE PRECISION,
    lon          DOUBLE PRECISION,
    city         TEXT,
    region       TEXT,
    country      TEXT,
    country_code CHAR(2),
    path         TEXT             NOT NULL DEFAULT '/',
    method       TEXT             NOT NULL DEFAULT 'GET',
    user_agent   TEXT,
    status_code  INT,
    created_at   TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

-- primary query pattern: time-window range scans
CREATE INDEX IF NOT EXISTS idx_events_created_at
    ON events (created_at DESC);

-- subnet / CIDR comparisons require a GIST index on INET columns
CREATE INDEX IF NOT EXISTS idx_events_ip_gist
    ON events USING GIST (ip inet_ops);

-- country-level aggregation (StatsBar top-countries query)
CREATE INDEX IF NOT EXISTS idx_events_country_code
    ON events (country_code);

-- combined index for the most common dashboard query:
-- WHERE created_at BETWEEN $1 AND $2 (already covered by idx_events_created_at,
-- but this composite helps if we add country filtering later)
CREATE INDEX IF NOT EXISTS idx_events_created_at_country
    ON events (created_at DESC, country_code);

COMMENT ON TABLE events IS
    'One row per tracked HTTP request. ip stored as INET for subnet ops.';

COMMENT ON COLUMN events.ip IS
    'Client IP address. Use host(ip) or ip::TEXT to get the string form.
     Use ip << ''10.0.0.0/8''::inet to test private/internal ranges.';

COMMENT ON COLUMN events.lat IS 'Latitude from MaxMind GeoLite2 city DB. NULL if lookup failed or IP is private.';
COMMENT ON COLUMN events.lon IS 'Longitude from MaxMind GeoLite2 city DB. NULL if lookup failed or IP is private.';
