# Mastery Engineering Audit — Streaming Data Pipeline

> "Simplicity under pressure = mastery"

## Project Summary
A real-time IoT sensor streaming pipeline built in Go. Kafka consumer groups ingest sensor events, a layered anomaly detector (Welford online z-score + EWMA) flags outliers, detected anomalies and raw events are persisted to PostgreSQL, and live updates are broadcast to WebSocket subscribers. Embedded SQL migrations run on startup.

## Failure Mode Analysis

### What breaks when the main service goes down?
Kafka retains unacknowledged messages. Because the processor uses `CommitMessages` *after* processing (not before), messages dequeued during the crash are uncommitted and will be redelivered to the next processor instance on restart. The consumer group offset is safe. In-flight anomaly state (Welford mean/variance, EWMA mean/variance per sensor) is in-memory and **lost on restart** — the detector will require a warmup period (10 readings for Welford, 20 for EWMA) before anomaly detection is fully active again. WebSocket clients disconnect and must reconnect.

### What breaks when the database/storage slows down?
`SaveEvent` and `SaveAnomaly` are called synchronously in the message processing path. Slow Postgres blocks Kafka message commits — the processor lag grows and Kafka offset commits fall behind. Eventually, the Kafka consumer group heartbeat may time out, causing a rebalance. There is no write buffer or async persistence path. The `lag` atomic metric will surface this, but there is no automatic back-pressure applied to the Kafka reader.

### What breaks when the network is partitioned?
If Kafka is unreachable, `FetchMessage` blocks (with the underlying kafka-go client applying its own retry). If Postgres is unreachable, `SaveEvent` fails with an error that is logged but the Kafka message is still committed (via `CommitMessages`) on the next loop iteration — **anomalies and events are silently lost** if Postgres is down at commit time, because the commit happens regardless of whether the DB write succeeded.

### What breaks under duplicate execution?
If a processor crashes after `SaveEvent` but before `CommitMessages`, the same Kafka message is redelivered on restart and saved again — **duplicate events and anomalies in PostgreSQL**. There is no idempotency key or deduplication on the store. Running two processor instances in the same consumer group is safe (Kafka partitions distribute messages), but the Welford/EWMA state per sensor is per-process and not shared — detectors on different instances will have inconsistent baselines for the same sensor.

## Checklist Status

| Check | Status | Notes |
|-------|--------|-------|
| Invariants defined | ✅ | Warmup thresholds (count < 10 for Welford, count < 20 for EWMA) prevent false positives during cold start |
| Idempotency | ❌ | No idempotency key on event/anomaly inserts; Kafka redelivery causes duplicate rows in PostgreSQL |
| Race conditions handled | ✅ | AnomalyDetector and EWMADetector use sync.Mutex per-detector; WindowAggregator uses its own mutex — all correct |
| State consistency | ⚠️ | Kafka commit happens regardless of DB write success; a DB failure silently loses the event but advances the offset |
| Structured logging | ✅ | zap used throughout with sensor_id, severity, z_score on anomaly events |
| Metrics (not just logs) | ⚠️ | ProcessorLag, EventsIngested, AnomaliesDetected, EventsPerSecond are tracked as atomics and exposed via API — but no Prometheus /metrics endpoint for scraping |
| Distributed tracing | ❌ | No OpenTelemetry tracing; cannot correlate a Kafka message through detection to WebSocket push as a single trace |
| Rollback strategy | ⚠️ | Embedded SQL migrations via go:embed run on startup; no down-migration path |
| Safe migrations | ⚠️ | Migrations are embedded and run on every startup; no version table visible — re-runs are only safe because of IF NOT EXISTS patterns |
| 10x traffic plan | ⚠️ | At 10x sensors (200 vs 20), the Welford/EWMA state maps grow linearly but are bounded per sensor; the single-partition Kafka topic becomes the bottleneck |
| Bottleneck identified | ✅ | Synchronous SaveEvent in the hot path is the primary bottleneck; explicitly tracked via ProcessorLag metric |
| Simplicity test passed | ✅ | Halving the system (remove WebSocket broadcast) still gives a working anomaly detector + Postgres store |

## Critical Gaps (Must Fix)

1. **Kafka commit regardless of DB write success**: `CommitMessages` is called even when `SaveEvent` or `SaveAnomaly` fails. This means events are silently dropped when Postgres is degraded. Fix: only commit the Kafka offset after successful DB persistence, or implement a write-ahead buffer with retry.
2. **No idempotency on event inserts**: Kafka guarantees at-least-once delivery. A `message_offset` or `(sensor_id, timestamp)` unique constraint on the events table would prevent duplicate rows on redelivery.
3. **Detector state lost on restart**: the Welford and EWMA states are in-memory with no persistence. A process restart means all sensors require a warmup period where real anomalies are silently missed. Consider checkpointing state to Redis or Postgres periodically.
4. **No Prometheus /metrics endpoint**: the pipeline tracks lag, events/s, and anomaly counts internally but there is no scraped metrics endpoint. Operators cannot alert on high lag or anomaly rate without building a custom integration.
5. **Single Kafka topic, single consumer group**: adding a second processor instance shares the consumer group (correct for scaling), but the split Welford/EWMA state per instance means sensors assigned to different instances have divergent anomaly baselines — a sensor migrated during rebalance loses its detection history.

## What is Already Mastery-Level

- **Layered anomaly detection with severity escalation**: Welford z-score and EWMA run independently and severity is escalated when both agree — this reduces both false positives and missed detections elegantly.
- **Welford online algorithm**: numerically stable online variance calculation (not naive sum-of-squares) is the correct choice for unbounded streaming data.
- **Warmup guard**: both detectors skip anomaly flagging for the first N readings, preventing cold-start false positives from hitting the database and alerting operators.
- **Commit-after-process pattern**: `FetchMessage` then process then `CommitMessages` is the correct at-least-once Kafka pattern — messages are not lost on crash (only potentially duplicated).
- **Window pruning goroutine**: a background ticker prunes stale sensor windows every 5 minutes — memory is bounded even as sensors go offline.
- **Health and readiness endpoints**: `/healthz` (liveness) and `/readyz` (DB connectivity check) are implemented — correct for Kubernetes deployments.

## Recommended Next Steps

1. **Conditional Kafka commit**: wrap `SaveEvent` + `CommitMessages` so the offset is only committed on successful DB write; on DB failure, log the error and let Kafka redeliver.
2. **Idempotency key on events table**: add a unique constraint on `(sensor_id, timestamp)` or store the Kafka `(topic, partition, offset)` as a dedup key to make redelivery safe.
3. **Prometheus /metrics endpoint**: expose `pipeline_events_ingested_total`, `pipeline_anomalies_detected_total{severity}`, `pipeline_processor_lag_ms` (gauge), and `pipeline_events_per_second` as scrapeable metrics.
4. **Detector state checkpointing**: serialize Welford/EWMA state maps to Redis or Postgres every minute; restore on startup to eliminate the cold-start detection gap.
5. **Increase Kafka partition count**: scale the topic to N partitions (one per processor instance) so multiple processors can handle the load with independent sensor assignments and no shared state conflicts.
