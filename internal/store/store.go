package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_init.sql
var migration001SQL string

//go:embed migrations/002_idempotency.sql
var migration002SQL string

const anomalyRingSize = 1000

// Store provides both PostgreSQL persistence and in-memory caching.
type Store struct {
	pool      *pgxpool.Pool
	mu        sync.RWMutex
	anomalies []models.Anomaly
	anomHead  int
	anomCount int
}

// New connects to PostgreSQL and runs migrations.
func New(ctx context.Context, pgURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &Store{
		pool:      pool,
		anomalies: make([]models.Anomaly, anomalyRingSize),
	}
	return s, nil
}

// Migrate runs all embedded SQL migrations in order.
func (s *Store) Migrate(ctx context.Context) error {
	for _, sql := range []string{migration001SQL, migration002SQL} {
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

// Ping checks connectivity to the database.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close shuts down the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// SaveEvent persists a sensor event to PostgreSQL.
// The Kafka topic, partition, and offset are stored to enforce idempotency:
// a duplicate delivery (same topic+partition+offset) is silently ignored via
// the unique index added in migration 002.
func (s *Store) SaveEvent(ctx context.Context, event models.SensorEvent, kafkaTopic string, kafkaPartition int, kafkaOffset int64) error {
	tagsJSON, _ := json.Marshal(event.Tags)
	_, err := s.pool.Exec(ctx,
		,
		event.SensorID, string(event.SensorType), event.Location,
		event.Value, event.Unit, tagsJSON, event.Timestamp,
		kafkaTopic, kafkaPartition, kafkaOffset,
	)
	return err
}

// GetRecentEvents returns the latest sensor events with optional filters.
func (s *Store) GetRecentEvents(ctx context.Context, limit int, sensorType, location string) ([]models.SensorEvent, error) {
	query := 
	args := []interface{}{}
	idx := 1

	if sensorType != "" {
		query += fmt.Sprintf(" AND sensor_type = $%d", idx)
		args = append(args, sensorType)
		idx++
	}
	if location != "" {
		query += fmt.Sprintf(" AND location = $%d", idx)
		args = append(args, location)
		idx++
	}
	query += fmt.Sprintf(" ORDER BY recorded_at DESC LIMIT $%d", idx)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.SensorEvent
	for rows.Next() {
		var e models.SensorEvent
		var sType string
		var tagsJSON []byte
		if err := rows.Scan(&e.SensorID, &sType, &e.Location, &e.Value, &e.Unit, &tagsJSON, &e.Timestamp); err != nil {
			return nil, err
		}
		e.SensorType = models.SensorType(sType)
		if len(tagsJSON) > 0 {
			_ = json.Unmarshal(tagsJSON, &e.Tags)
		}
		events = append(events, e)
	}
	return events, nil
}

// SaveAnomaly persists an anomaly to PostgreSQL and caches it in-memory.
func (s *Store) SaveAnomaly(ctx context.Context, a models.Anomaly) error {
	_, err := s.pool.Exec(ctx,
		,
		a.SensorID, string(a.SensorType), a.Location, a.Value, a.Expected, a.ZScore, a.Severity, a.DetectedAt,
	)
	if err != nil {
		return err
	}
	// Cache in ring buffer
	s.mu.Lock()
	s.anomalies[s.anomHead] = a
	s.anomHead = (s.anomHead + 1) % anomalyRingSize
	if s.anomCount < anomalyRingSize {
		s.anomCount++
	}
	s.mu.Unlock()
	return nil
}

// GetRecentAnomalies returns the most recent anomalies from the in-memory ring.
func (s *Store) GetRecentAnomalies(_ context.Context, limit int) ([]models.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > s.anomCount {
		limit = s.anomCount
	}
	result := make([]models.Anomaly, 0, limit)
	for i := 0; i < limit; i++ {
		idx := (s.anomHead - 1 - i + anomalyRingSize) % anomalyRingSize
		result = append(result, s.anomalies[idx])
	}
	return result, nil
}

// GetStats returns ingestion counters from PostgreSQL.
func (s *Store) GetStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	var totalEvents int64
	err := s.pool.QueryRow(ctx, ).Scan(&totalEvents)
	if err != nil {
		return nil, err
	}
	stats["total_events"] = totalEvents

	var totalAnomalies int64
	err = s.pool.QueryRow(ctx, ).Scan(&totalAnomalies)
	if err != nil {
		return nil, err
	}
	stats["total_anomalies"] = totalAnomalies

	var activeSensors int64
	err = s.pool.QueryRow(ctx,
		,
		time.Now().Add(-5*time.Minute),
	).Scan(&activeSensors)
	if err != nil {
		return nil, err
	}
	stats["active_sensors"] = activeSensors

	return stats, nil
}
