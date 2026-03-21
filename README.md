<div align="center">

<img src="https://capsule-render.vercel.app/api?type=waving&amp;color=gradient&amp;customColorList=8,16,22&amp;height=180&amp;section=header&amp;text=Streaming%20Data%20Pipeline&amp;fontSize=38&amp;fontColor=fff&amp;animation=twinkling&amp;fontAlignY=38" />

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&amp;logo=go)](https://golang.org)
[![Kafka](https://img.shields.io/badge/Kafka-3.x-231F20?style=flat&amp;logo=apachekafka)](https://kafka.apache.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

**Real-time streaming data pipeline with anomaly detection and WebSocket broadcast.**

</div>

## Overview

This pipeline ingests high-frequency event streams, applies statistical anomaly detection in real time, and broadcasts flagged events to WebSocket subscribers with sub-second latency.

## Architecture

```
Event Producers (IoT sensors, APIs, services)
         |
         v
    [Kafka Topic]          Durable, partitioned event log
         |
         v
  [Stream Processor]       Sliding window statistics
         |                 Z-score anomaly detection
         v
  [Anomaly Detector]       Flags events exceeding threshold
         |         |
         v         v
  [Result Store]  [WebSocket Hub]    Push to subscribers in real time
   (Redis/DB)     (gorilla/websocket)
```

## Features

**Anomaly Detection**
Sliding window Z-score detection. Configurable window size and threshold. No ML model required — statistically principled, zero cold-start latency.

**WebSocket Broadcast**
Detected anomalies are pushed to all connected WebSocket clients within milliseconds of detection. Fan-out handled concurrently with goroutines.

**Kafka Integration**
Reads from configurable Kafka topics. Supports consumer groups for horizontal scaling. Offset management is automatic — restart the processor without losing events.

**Backpressure Handling**
Internal channels are bounded. If downstream consumers fall behind, the processor applies backpressure to the Kafka consumer rather than dropping events or crashing.

## Quick Start

```bash
git clone https://github.com/Aliipou/streaming-data-pipeline.git
cd streaming-data-pipeline
docker compose up -d   # Starts Kafka + Zookeeper + Redis
go run cmd/processor/main.go
```

Connect a WebSocket client to `ws://localhost:8080/ws` to receive live anomaly alerts.

## Configuration

```yaml
kafka:
  brokers: [localhost:9092]
  topic: events
  group_id: pipeline-processor
  offset: earliest

detector:
  window_size: 100        # Events in sliding window
  z_score_threshold: 3.0  # Standard deviations for anomaly
  min_samples: 30         # Minimum samples before detection starts

websocket:
  port: 8080
  ping_interval: 30s
```

## License

MIT
