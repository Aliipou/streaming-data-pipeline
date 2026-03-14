# Streaming Data Pipeline

> Real-time IoT event ingestion, windowed aggregation, and online anomaly detection in Go — Kafka → Stream Processor → PostgreSQL → WebSocket dashboard.

![Go](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go) ![Kafka](https://img.shields.io/badge/Kafka-7.6-231F20?logo=apachekafka) ![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql) ![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker) ![CI](https://github.com/aliipou/streaming-data-pipeline/actions/workflows/ci.yml/badge.svg)

---

## Problem & Requirements

### Functional Requirements

- Ingest real-time IoT sensor events (temperature, pressure, vibration, flow) via Apache Kafka
- Apply 1-minute tumbling window aggregation per sensor: count, sum, min, max, avg, p99
- Detect anomalies per sensor using online algorithms (Welford Z-score + EWMA) without storing history
- Expose a REST API and WebSocket endpoint for a live monitoring dashboard

### Non-Functional Requirements

- Sustain 1,000 events/sec peak throughput (20 sensors × 50 events/sec burst)
- End-to-end latency from event production to anomaly detection under 500 ms
- At-least-once delivery via Kafka consumer group offset management
- Survive processor restart without data loss (Kafka replay + idempotent DB writes)

### Capacity Estimates

| Metric | Value |
|--------|-------|
| Sensors | 20, 1 event/sec baseline each = 20 events/sec steady state |
| Anomaly rate | ~5% → ~1 anomaly/sec |
| Kafka retention (7 days) | 20 ev/sec × 86,400 s × 7 days × ~120 bytes ≈ **1.4 GB** |
| PostgreSQL events | ~1.7 M rows/day × 200 bytes ≈ **340 MB/day** |
| Window aggregates | 20 sensors × 1 row/min = 28,800 rows/day (negligible) |
| In-memory detector state | 20 sensors × ~200 bytes each = **<5 KB total** |

---

## Architecture

```
┌──────────────────────────────────────────┐
│    Sensor Simulator (20 sensors)         │
│    temp · pressure · vibration · flow    │
│    1s interval · 5% anomaly injection    │
└─────────────────┬────────────────────────┘
                  │ JSON over TCP
                  ▼
┌──────────────────────────────────────────┐
│              Apache Kafka                │
│          topic: sensor-events            │
│          partitions: 1 (MVP)             │
│          retention: 7 days               │
└─────────────────┬────────────────────────┘
                  │ consumer group: processor
                  ▼
┌──────────────────────────────────────────┐
│           Stream Processor               │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │  Window Aggregator                 │  │
│  │  map[sensorID]→TumblingWindow      │  │
│  │  flushes on minute boundary        │  │
│  │  count · sum · min · max · avg     │  │
│  │  p99 (sorted reservoir, 1000 pts)  │  │
│  └────────────────────────────────────┘  │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │  Anomaly Detector (per sensor)     │  │
│  │  Layer 1: Welford Z-score          │  │
│  │    online mean+variance, warmup=10 │  │
│  │    Z = |v−μ|/σ → severity tier    │  │
│  │  Layer 2: EWMA                     │  │
│  │    α=0.1, warmup=20                │  │
│  │    escalates if both layers agree  │  │
│  └────────────────────────────────────┘  │
└─────────────────┬────────────────────────┘
                  │ INSERT / upsert
                  ▼
┌──────────────────────────────────────────┐
│             PostgreSQL 16                │
│  sensor_events(sensor_id, timestamp)     │
│  anomalies(sensor_id, severity, value)   │
└─────────────────┬────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│         REST API + WebSocket             │
│  /events · /anomalies · /windows         │
│  /stats · /sensors · ws:// (1s push)     │
└──────────────────────────────────────────┘
```

Three independent binaries — **producer**, **processor**, **api** — each scales independently. The processor is the only stateful component; its state is re-derived from Kafka replay on restart.

---

## Key Design Decisions

### 1. Kafka as event bus, not direct HTTP

Decouples the producer from the processor. The processor can lag behind during a spike or restart, then catch up by replaying from its committed offset. Seven-day retention also supports ad-hoc replay for backfilling new anomaly models.

Trade-off: Kafka adds operational overhead (ZooKeeper/KRaft, broker management) compared to a simple HTTP queue or channel. Justified here because replay and consumer-group semantics are core requirements.

### 2. At-least-once delivery via offset commit after DB write

The processor commits its Kafka consumer-group offset only after a successful PostgreSQL write. If it crashes between consuming and committing, messages are reprocessed on restart.

Trade-off: duplicate events are possible on crash-restart. Mitigated by a `UNIQUE` constraint on `(sensor_id, timestamp)` — duplicate inserts become idempotent upserts with no observable side-effects.

### 3. Per-sensor stateful detectors (`map[sensorID]Detector`)

Each sensor carries its own independent Welford state (n, mean, M2) and EWMA value. Sensor A's high variance does not pollute Sensor B's baseline.

Trade-off: state lives in-process and is lost on restart. The detector re-warms (10 readings for Welford, 20 for EWMA) from the resumed Kafka stream, so detection is briefly unavailable after a restart. Acceptable given the volume (10–20 seconds to re-warm at 1 ev/sec).

### 4. Welford's online algorithm for streaming mean and variance

Computes running mean and variance in O(1) time and O(1) space per sensor without storing any historical values. Numerically stable against catastrophic cancellation that plagues the naive `E[x²] − E[x]²` approach.

Trade-off: requires a warmup of n ≥ 10 before the variance estimate is reliable. A Z-score anomaly cannot fire during warmup — this is intentional, not a gap.

### 5. Layered detection: Welford Z-score + EWMA

Z-score detects isolated point anomalies (a single outlier spike). EWMA detects sustained drift (a sensor slowly trending out of range). Severity escalates only when both signals agree, reducing false positives.

Trade-off: two thresholds to tune (Z-score cutoff and EWMA deviation %). A sensor with high natural variance may need per-sensor threshold configuration (roadmap Phase 3).

### 6. Tumbling windows, not sliding windows

One-minute fixed-boundary windows flush atomically at the minute mark. State per window is a small accumulator struct — constant space regardless of event rate.

Trade-off: an event at 00:00:59 and one at 00:01:01 land in different windows despite being 2 seconds apart. Sliding windows would smooth this but require O(window_size) memory per sensor and more complex eviction logic. At 1-minute granularity the edge effect is acceptable for operational monitoring.

### 7. p99 via sorted reservoir

Each window keeps the last 1,000 values in a ring buffer, sorts on flush, and reads the 990th element. Straightforward and correct at this scale.

Trade-off: O(n log n) sort on flush (1,000 elements, negligible). At higher scale, replace with a streaming quantile sketch (DDSketch, HdrHistogram) to avoid the sort and cap memory at O(log ε⁻¹) per window.

### 8. Separate binaries for producer, processor, and API

The processor is CPU-bound (floating-point math, Kafka polling). The API is I/O-bound (DB reads, WebSocket fan-out). Separating them allows independent horizontal scaling and avoids a noisy-neighbour problem between detection throughput and API latency.

---

## Anomaly Detection Deep Dive

### Welford's Online Algorithm

```
For each new value x:
  n++
  delta  = x - mean
  mean  += delta / n
  delta2 = x - mean          // recalculated after mean update
  M2    += delta * delta2
  variance = M2 / (n - 1)   // Bessel-corrected; numerically stable
  std_dev  = sqrt(variance)
  Z = |x - mean| / std_dev
```

**Why Welford over batch variance**: maintains a running sum of squared deviations (`M2`) rather than summing raw squares. This avoids catastrophic cancellation when variance is small relative to the mean — a property that matters for sensors reporting, e.g., temperature as 22.001, 22.003, 22.002.

### EWMA (Exponential Weighted Moving Average)

```
ewma       = α × value + (1 − α) × ewma   // α = 0.1
deviation  = |value − ewma| / ewma × 100  // percent deviation
```

`α = 0.1` gives a half-life of ~6.5 readings, meaning the EWMA forgets older values exponentially. A sustained drift of >10% from the smoothed baseline triggers a secondary signal.

**Why layered**: Z-score is reactive (single outlier fires immediately); EWMA is inertial (slow trends accumulate). Requiring both to agree before escalating severity materially reduces false positives on noisy sensors.

### Severity Tiers

| Z-score | EWMA deviation | Severity |
|---------|---------------|----------|
| 2 – 3 | any | `low` |
| 3 – 4 | > 10% | `medium` |
| 4 – 5 | > 20% | `high` |
| > 5 | > 30% | `critical` |

An event at Z = 3.5 with EWMA deviation of 8% is classified `low` (Z tier) because the EWMA threshold for `medium` is not met. Severity is the minimum of what both layers independently justify.

---

## Scalability

| Component | Current (MVP) | Scale path |
|-----------|--------------|------------|
| Kafka | 1 partition | Key messages by `sensor_id`; add partitions; each processor instance owns a partition range via consumer-group rebalancing |
| Processor | Single instance | Stateless within a partition assignment; add consumer-group members = horizontal scale. Window state moves to Redis sorted sets (Phase 4) |
| Window state | In-process map | Redis sorted sets with TTL; atomic `ZADD`/`ZRANGEBYSCORE` per window |
| API | Single instance | Stateless; add replicas behind a load balancer |
| PostgreSQL | Single writer | Read replica for API queries; migrate to TimescaleDB hypertable for time-range query acceleration and automatic chunk compression (Phase 4) |
| Anomaly detection | In-process | KEDA `KafkaTopic` scaler auto-scales processor replicas based on consumer lag (Phase 6) |

**Partition-to-processor affinity** is the critical invariant for horizontal scale: all events for `sensor_id=7` must land on the same partition (keyed producer), so the per-sensor Welford state remains consistent within a single processor instance. Cross-partition sensor state would require distributed state (Redis) and a coordination protocol.

---

## Failure Modes

| Failure | Observed behaviour | Recovery |
|---------|-------------------|----------|
| Processor crash mid-batch | Kafka offsets not committed → messages redelivered on restart | Idempotent upsert on `(sensor_id, timestamp)` unique index absorbs duplicates |
| Processor restart (clean) | Detector state lost; re-warms from resumed Kafka stream | 10–20 events per sensor (10–20 seconds at 1 ev/sec) before anomaly detection resumes |
| Kafka broker unavailable | Producer buffers in memory (bounded); processor stalls at `FetchMessage` | Auto-reconnect with exponential backoff; no data lost while Kafka is down if producer buffer is not exhausted |
| PostgreSQL unavailable | Processor logs error, skips DB write, continues consuming and detecting | Events processed in memory but not persisted; Kafka retains originals for replay once DB recovers |
| Sensor produces malformed JSON | Currently: logged and skipped | Roadmap Phase 2: route to `failed-sensor-events` dead-letter topic for inspection and replay |
| Sensor goes silent | Window flushes with whatever data arrived; next window starts fresh | No special handling needed; missing windows are detectable by monitoring window count per sensor |

---

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/events` | Recent sensor events. Params: `limit`, `sensor_type`, `location` |
| `GET` | `/api/v1/anomalies` | Recent anomaly records with severity, value, Z-score |
| `GET` | `/api/v1/windows` | Current 1-minute window aggregates per sensor |
| `GET` | `/api/v1/stats` | Pipeline throughput: events/sec, anomalies/sec, uptime |
| `GET` | `/api/v1/sensors` | List of active sensors with last-seen timestamp |
| `WS` | `/ws` | Push stream — emits a JSON envelope every second with latest events and anomalies |

---

## Running Locally

```bash
# Start infrastructure
docker compose -f deployments/docker-compose.yml up -d zookeeper kafka postgres

# Terminal 1 — stream processor (stateful)
go run ./cmd/processor

# Terminal 2 — sensor producer
go run ./cmd/producer

# Terminal 3 — REST API + WebSocket
go run ./cmd/api
```

Dashboard: http://localhost:8080

### Full Docker Stack

```bash
docker compose -f deployments/docker-compose.yml up -d
```

| Service | Port | Notes |
|---------|------|-------|
| Kafka | 9092 | Confluent 7.6 |
| Zookeeper | 2181 | |
| PostgreSQL | 5433 | `pipeline_db` / `pipeline` / `password` |
| API + Dashboard | 8080 | REST + WebSocket + static UI |

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated broker addresses |
| `KAFKA_TOPIC` | `sensor-events` | Topic name |
| `POSTGRES_URL` | `postgres://pipeline:password@localhost:5433/pipeline_db` | pgx DSN |
| `HTTP_PORT` | `8080` | API server listen port |
| `NUM_SENSORS` | `20` | Number of simulated sensors |

---

## Testing

```bash
go test ./... -race -count=1
```

| Package | What is tested |
|---------|---------------|
| `internal/processor` | Welford warmup (no anomaly before n=10); all four severity tiers; sensor isolation (Sensor A spike does not affect Sensor B state) |
| `internal/processor` | EWMA warmup; sustained drift detection |
| `internal/processor` | Tumbling window flush; p99 correctness; multi-bucket pruning |
| `internal/models` | Event serialization round-trips |
| `internal/store` | (unit) upsert idempotency on duplicate `(sensor_id, timestamp)` |

The test suite runs without a live Kafka or PostgreSQL — all external dependencies are stubbed. CI enforces `-race` to catch goroutine races in the window aggregator and WebSocket broadcaster.

---

## Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go 1.23 | goroutines for concurrent sensor handling; value-type structs keep detector state allocation-free |
| Event bus | Apache Kafka (`segmentio/kafka-go`) | Durable log, consumer-group offset management, 7-day replay |
| Persistence | PostgreSQL 16 (`jackc/pgx/v5`) | Indexed time-range queries; UNIQUE constraint for idempotent upserts |
| HTTP/WS | Gin + `gorilla/websocket` | Gin for route ergonomics; gorilla for WebSocket upgrade and read/write pump pattern |
| Logging | `go.uber.org/zap` | Structured JSON logs; zero-allocation fast path |
| Containers | Docker multi-stage builds + Compose | Separate Dockerfiles per binary; scratch-based final images |

---

## Roadmap

See [ROADMAP.md](./ROADMAP.md) for the full phased plan. Summary:

| Phase | Focus |
|-------|-------|
| 1 — Foundation | Complete. All core features shipped. |
| 2 — Fault Tolerance | Dead-letter queue, idempotent offset commits, graceful shutdown |
| 3 — Advanced Detection | STL seasonal decomposition, Isolation Forest, alert deduplication |
| 4 — Scalability | Kafka partitioning by sensor ID, Redis window state, TimescaleDB |
| 5 — Observability | Prometheus `/metrics`, Grafana dashboard, OpenTelemetry tracing |
| 6 — Cloud & Operations | AKS Helm chart, KEDA autoscaler, event-sourcing replay capability |
