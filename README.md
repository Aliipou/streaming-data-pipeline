# Streaming Data Pipeline

A real-time sensor data streaming pipeline built in Go — inspired by Apache Kafka + Flink. Ingests IoT sensor events, applies windowed aggregation, detects anomalies with Z-score analysis, and streams results to a live dashboard.

## Architecture

```
┌─────────────────────────────────────┐
│   Sensor Simulator (20 sensors)     │
│  temperature · pressure · vibration │
│  humidity  ·  5% anomaly injection  │
└─────────────┬───────────────────────┘
              │  JSON events
              ▼
┌─────────────────────────────────────┐
│            Apache Kafka             │
│        topic: sensor-events         │
└─────────────┬───────────────────────┘
              │  consumer group
              ▼
┌─────────────────────────────────────┐
│         Stream Processor            │
│  ┌──────────────────────────────┐   │
│  │  Window Aggregator           │   │
│  │  1-min tumbling windows      │   │
│  │  count·sum·min·max·avg·p99   │   │
│  └──────────────────────────────┘   │
│  ┌──────────────────────────────┐   │
│  │  Anomaly Detector            │   │
│  │  Welford online Z-score      │   │
│  │  low·medium·high·critical    │   │
│  └──────────────────────────────┘   │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│            PostgreSQL               │
│   sensor_events · anomalies tables  │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│      REST API + WebSocket           │
│  /api/v1/events  /anomalies         │
│  /windows  /stats  /sensors         │
│  ws://host/ws  (1s push)            │
└─────────────┬───────────────────────┘
              │
              ▼
┌─────────────────────────────────────┐
│         Web Dashboard               │
│  EPS line chart · anomaly feed      │
│  window table · event stream        │
└─────────────────────────────────────┘
```

## Features

- **Kafka-backed ingestion** — `segmentio/kafka-go`, auto topic creation, consumer groups
- **20 simulated sensors** — realistic value ranges, 5% anomaly injection rate, 1-5s intervals
- **1-minute tumbling windows** — count, sum, min, max, avg, p99 per sensor type × location
- **Welford online anomaly detection** — Z-score per sensor with 10-reading warmup, severity tiers
- **PostgreSQL persistence** — indexed sensor events and anomalies tables
- **WebSocket push** — dashboard updates every second, auto-reconnect
- **Three independent binaries** — producer, processor, API (scale independently)

## Quick Start

```bash
# Start Kafka + Postgres
docker compose -f deployments/docker-compose.yml up -d zookeeper kafka postgres

# Terminal 1 — stream processor
go run ./cmd/processor

# Terminal 2 — sensor producer
go run ./cmd/producer

# Terminal 3 — API server
go run ./cmd/api
```

Dashboard: http://localhost:8080

## Full Docker Stack

```bash
docker compose -f deployments/docker-compose.yml up -d
```

Services:
- Zookeeper + Kafka (Confluent 7.6)
- PostgreSQL 16 on port 5433
- Processor, Producer, API on port 8082

## Anomaly Detection

Anomaly detector uses Welford's online algorithm to maintain rolling mean μ and variance σ² per sensor. After 10+ readings, Z-score is computed as:

```
Z = |value − μ| / σ
```

| Z-score | Severity |
|---------|----------|
| 2 – 3   | low      |
| 3 – 4   | medium   |
| 4 – 5   | high     |
| > 5     | critical |

## API Reference

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/events` | Recent sensor events |
| `GET` | `/api/v1/anomalies` | Recent anomalies |
| `GET` | `/api/v1/stats` | Pipeline statistics |
| `GET` | `/api/v1/windows` | Current window aggregations |
| `GET` | `/api/v1/sensors` | Active sensors |
| `WS` | `/ws` | Real-time event stream |

Query params for `/events`: `limit`, `sensor_type`, `location`

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses |
| `KAFKA_TOPIC` | `sensor-events` | Topic name |
| `POSTGRES_URL` | `postgres://pipeline:password@localhost:5433/pipeline_db` | PostgreSQL DSN |
| `HTTP_PORT` | `8080` | API server port |
| `NUM_SENSORS` | `20` | Number of simulated sensors |

## Running Tests

```bash
go test ./... -race -count=1
```

Tests cover: event models, window aggregation (p99, multi-bucket, pruning), anomaly detection (warmup, severity, sensor isolation).

## Tech Stack

- **Go 1.22** — goroutines, atomic counters, ring buffers
- **Apache Kafka** — event streaming via `segmentio/kafka-go`
- **PostgreSQL 16** — event and anomaly persistence
- **Gin** — REST API
- **gorilla/websocket** — real-time dashboard push
- **go.uber.org/zap** — structured logging
