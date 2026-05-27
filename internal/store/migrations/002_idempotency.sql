-- Migration 002: add idempotency columns and unique constraint on
-- (kafka_topic, kafka_partition, kafka_offset) to prevent duplicate rows
-- from Kafka message redelivery.

ALTER TABLE sensor_events
    ADD COLUMN IF NOT EXISTS kafka_topic     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS kafka_partition INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS kafka_offset    BIGINT  NOT NULL DEFAULT -1;

-- Unique constraint used by INSERT ... ON CONFLICT DO NOTHING.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sensor_events_kafka_dedup
    ON sensor_events(kafka_topic, kafka_partition, kafka_offset)
    WHERE kafka_offset >= 0;
