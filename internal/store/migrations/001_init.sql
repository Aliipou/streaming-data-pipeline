CREATE TABLE IF NOT EXISTS sensor_events (
    id BIGSERIAL PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    sensor_type TEXT NOT NULL,
    location TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    unit TEXT NOT NULL,
    tags JSONB,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sensor_events_type_time
    ON sensor_events(sensor_type, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_events_sensor_time
    ON sensor_events(sensor_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_sensor_events_location_time
    ON sensor_events(location, recorded_at DESC);

CREATE TABLE IF NOT EXISTS anomalies (
    id BIGSERIAL PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    sensor_type TEXT NOT NULL,
    location TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    expected DOUBLE PRECISION NOT NULL,
    z_score DOUBLE PRECISION NOT NULL,
    severity TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_anomalies_time ON anomalies(detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_anomalies_severity ON anomalies(severity, detected_at DESC);
