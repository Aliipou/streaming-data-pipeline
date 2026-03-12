# Streaming Data Pipeline — Roadmap

A practical checklist for evolving this pipeline from a solid MVP into a
production-grade, horizontally-scalable streaming system.

---

## Phase 1 — Foundation (complete ✓)

- [x] 20-sensor Kafka producer with realistic value ranges and 5% anomaly injection
- [x] 1-minute tumbling window aggregation (count, sum, min, max, avg, p99)
- [x] Welford online Z-score anomaly detection (warmup=10, severity tiers)
- [x] EWMA layered detector (alpha=0.1, warmup=20) with severity escalation
- [x] PostgreSQL persistence — sensor_events + anomalies tables with indexes
- [x] REST API — events, anomalies, stats, windows, sensors endpoints
- [x] WebSocket push dashboard with 1-second refresh
- [x] Three independent binaries — producer, processor, API
- [x] Multi-stage Docker builds + docker-compose full stack
- [x] GitHub Actions CI — unit tests (no Kafka required), golangci-lint

---

## Phase 2 — Fault Tolerance & Reliability

### Dead-Letter Queue (DLQ)
- [ ] Add `failed-sensor-events` Kafka topic for unprocessable messages
- [ ] Route malformed JSON / schema-invalid events to DLQ instead of dropping
- [ ] DLQ consumer that logs + optionally replays after manual inspection
- [ ] Expose DLQ depth via `/api/v1/stats` and dashboard

### Processor Crash Recovery
- [ ] Commit Kafka offsets only after successful DB write (at-least-once delivery)
- [ ] Idempotent upsert on `sensor_events` using `(sensor_id, timestamp)` unique index
- [ ] Graceful shutdown drains in-flight messages before closing reader

### Schema Validation
- [ ] JSON Schema or struct-level validation before processing
- [ ] Structured error logging with original payload for debugging

---

## Phase 3 — Advanced Anomaly Detection

### ARIMA / Seasonal Decomposition
- [ ] Implement STL (Seasonal-Trend decomposition using Loess) for sensors with
  diurnal patterns (temperature, humidity)
- [ ] Residual-based anomaly scoring on top of trend-adjusted values
- [ ] Per-sensor configurable seasonality period (default: 24h)

### Isolation Forest (batch scoring)
- [ ] Periodic (5-min) batch scoring against a sliding 500-reading window
- [ ] Score merged into `LayeredDetector` as a third signal
- [ ] Only activate after 200+ readings to ensure tree quality

### Alert Deduplication
- [ ] Suppress repeat anomaly events for the same sensor within a 5-minute window
- [ ] Flap detection: only re-alert after 2 consecutive normal readings
- [ ] Configurable cooldown period per sensor type

---

## Phase 4 — Scalability

### Kafka Partitioning Strategy
- [ ] Partition `sensor-events` topic by `sensor_id` (keyed messages)
- [ ] Each processor instance owns a partition range via consumer-group rebalancing
- [ ] Verify per-partition ordering is preserved through window aggregation

### Horizontal Processor Scaling
- [ ] Stateless window aggregation via Redis sorted sets (replace in-memory map)
- [ ] Processor instances read from independent partition assignments
- [ ] KEDA `KafkaTopic` scaler to auto-scale processor replicas based on consumer lag

### TimescaleDB Migration
- [ ] Replace plain PostgreSQL with TimescaleDB hypertable on `sensor_events`
- [ ] Enable automatic time-based chunk compression (compress chunks older than 1h)
- [ ] Continuous aggregates for pre-computed 1-min, 5-min, 1-hour rollups
- [ ] Benchmark query improvement vs plain PostgreSQL on 10M+ row dataset

---

## Phase 5 — Alerting & Observability

### Webhook / Slack Alerts
- [ ] Alert manager goroutine subscribes to high/critical anomalies from a channel
- [ ] Configurable webhook URL + Slack `incoming-webhook` target per severity tier
- [ ] Exponential-backoff retry on failed deliveries (3 attempts, 1s/2s/4s)
- [ ] Alert payload includes: sensor ID, severity, value, expected, timestamp, dashboard link

### Self-Monitoring
- [ ] Expose `/metrics` endpoint in Prometheus text format
- [ ] Track: events/sec, anomalies/sec, Kafka consumer lag, window count, DB write latency
- [ ] Grafana dashboard JSON (checked into `deployments/grafana/`)

### Distributed Tracing
- [ ] Add OpenTelemetry spans for message fetch → process → DB write path
- [ ] Propagate trace context from producer message headers

---

## Phase 6 — Cloud & Operations

### Kubernetes Deployment (AKS)
- [ ] Helm chart with separate Deployments for producer, processor, API
- [ ] HorizontalPodAutoscaler for API tier (CPU-based)
- [ ] KEDA ScaledObject for processor tier (Kafka lag-based)
- [ ] PodDisruptionBudget for processor (min 1 available during rolling updates)
- [ ] Liveness + readiness probes on all services

### Event Sourcing
- [ ] Treat `sensor_events` as an append-only immutable event log
- [ ] Add `events_v2` table with event schema versioning
- [ ] Replay capability: reprocess any time range through the anomaly pipeline

### CI/CD Improvements
- [ ] Add integration test job with real Kafka (via docker-compose in CI)
- [ ] Trivy image vulnerability scan on each Docker build
- [ ] Automated semver tagging + GitHub Release on `main` merge
- [ ] Helm chart publish to GitHub Pages OCI registry
