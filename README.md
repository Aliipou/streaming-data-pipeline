<div align="center">

[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Kafka](https://img.shields.io/badge/Kafka-3.x-231F20?style=flat&logo=apachekafka)](https://kafka.apache.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

**Real-time streaming data pipeline with anomaly detection and WebSocket broadcast.**

</div>

## Overview

This pipeline ingests sensor event streams from Kafka, applies statistical anomaly detection in real time using Welford's algorithm and EWMA, persists events and anomalies to PostgreSQL, and broadcasts updates to WebSocket subscribers.

## Architecture

```
Event Producers (simulated IoT sensors)
         |
         v
    [Kafka Topic]          Durable, partitioned event log
         |
         v
  [Stream Processor]       Sliding window statistics
         |                 Layered anomaly detection (Welford + EWMA)
         v
  [Anomaly Detector]       Flags events exceeding z-score threshold
         |         |
         v         v
  [PostgreSQL]  [WebSocket Hub]    Push to subscribers
```

## Features

**Layered Anomaly Detection**
Two complementary detectors run on every event: a Welford online z-score detector and an EWMA (exponentially weighted moving average) detector. When both agree, severity is escalated by one tier. Thresholds are configurable via environment variables.

**WebSocket Broadcast**
Detected anomalies and pipeline stats are pushed to connected WebSocket clients once per second.

**Kafka Integration**
Reads from configurable Kafka topics using consumer groups. Offset management is automatic.

**PostgreSQL Persistence**
Events and anomalies are stored in PostgreSQL. Migrations are embedded in the binary via `go:embed`.

**Health Checks**
`/healthz` (liveness) and `/readyz` (readiness, verifies DB connectivity) endpoints are available for container orchestration.

## Quick Start

```bash
git clone https://github.com/Aliipou/streaming-data-pipeline.git
cd streaming-data-pipeline
docker compose -f deployments/docker-compose.yml up -d
go run cmd/processor/main.go
go run cmd/api/main.go
```

Connect a WebSocket client to `ws://localhost:8080/ws` to receive live pipeline updates.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `KAFKA_BROKERS` | `localhost:9092` | Comma-separated Kafka broker addresses |
| `KAFKA_TOPIC` | `sensor-events` | Kafka topic to consume |
| `POSTGRES_URL` | `postgres://pipeline:password@localhost:5433/pipeline_db?sslmode=disable` | PostgreSQL connection string |
| `HTTP_PORT` | `8080` | API server port |
| `NUM_SENSORS` | `20` | Number of simulated sensors (producer) |
| `ZSCORE_THRESHOLD` | `2.0` | Welford detector z-score threshold |
| `EWMA_THRESHOLD` | `3.0` | EWMA detector deviation threshold |
| `EWMA_ALPHA` | `0.1` | EWMA smoothing factor (0 < alpha < 1) |

## License

MIT
