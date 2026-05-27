package processor

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/aliipou/streaming-data-pipeline/internal/store"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Processor reads from Kafka, runs window aggregation and anomaly detection.
type Processor struct {
	store     *store.Store
	windows   *WindowAggregator
	anomaly   *LayeredDetector
	broker    string
	topic     string
	log       *zap.Logger
	ingested  atomic.Int64
	detected  atomic.Int64
	lag       atomic.Int64
	startTime time.Time
	statePath string
}

// New creates a Processor connected to the given store and Kafka config.
func New(s *store.Store, broker, topic string, log *zap.Logger, zScoreThreshold, ewmaAlpha, ewmaThreshold float64) *Processor {
	p := &Processor{
		store:     s,
		windows:   NewWindowAggregator(),
		anomaly:   NewLayeredDetector(zScoreThreshold, ewmaAlpha, ewmaThreshold),
		broker:    broker,
		topic:     topic,
		log:       log,
		startTime: time.Now(),
		statePath: "/var/lib/processor/detector_state.json",
	}

	// Attempt to restore persisted detector state from a previous run.
	if err := p.anomaly.RestoreState(p.statePath); err != nil {
		log.Info("no prior detector state to restore", zap.Error(err))
	}
	return p
}

// Run consumes from Kafka until ctx is cancelled.
func (p *Processor) Run(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     strings.Split(p.broker, ","),
		Topic:       p.topic,
		GroupID:     "stream-processors",
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafka.LastOffset,
	})
	defer reader.Close()

	p.log.Info("stream processor started", zap.String("topic", p.topic))

	// Prune old windows every 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.windows.Prune(60)
			}
		}
	}()

	// Persist detector state to disk every 60 seconds so that a restart
	// does not lose the Welford/EWMA baselines built up at runtime.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Final snapshot on clean shutdown.
				if err := p.anomaly.PersistState(p.statePath); err != nil {
					p.log.Warn("persist detector state on shutdown", zap.Error(err))
				}
				return
			case <-ticker.C:
				if err := p.anomaly.PersistState(p.statePath); err != nil {
					p.log.Warn("persist detector state", zap.Error(err))
				}
			}
		}
	}()

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			p.log.Error("fetch message", zap.Error(err))
			continue
		}

		start := time.Now()
		processErr := p.processMessage(ctx, msg.Topic, msg.Partition, msg.Offset, msg.Value)
		p.lag.Store(time.Since(start).Milliseconds())

		if processErr != nil {
			// Do NOT advance the Kafka offset when the DB write failed; the
			// message will be redelivered and retried after a restart.
			p.log.Error("process message", zap.Error(processErr))
			continue
		}

		// Only commit the offset once the event has been durably written to
		// PostgreSQL, preventing silent data loss when the DB is unavailable.
		if err := reader.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			p.log.Warn("commit message", zap.Error(err))
		}
	}
	p.log.Info("stream processor stopped")
}

func (p *Processor) processMessage(ctx context.Context, topic string, partition int, offset int64, data []byte) error {
	var event models.SensorEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}

	p.ingested.Add(1)
	p.windows.Add(event)

	if anomaly := p.anomaly.Check(event); anomaly != nil {
		p.detected.Add(1)
		if err := p.store.SaveAnomaly(ctx, *anomaly); err != nil {
			p.log.Warn("save anomaly", zap.Error(err))
		}
		p.log.Warn("anomaly detected",
			zap.String("sensor", anomaly.SensorID),
			zap.String("severity", anomaly.Severity),
			zap.Float64("z_score", anomaly.ZScore),
		)
	}

	return p.store.SaveEvent(ctx, event, topic, partition, offset)
}

// GetStats returns current pipeline statistics.
func (p *Processor) GetStats() models.PipelineStats {
	ingested := p.ingested.Load()
	elapsed := time.Since(p.startTime).Seconds()
	var eps float64
	if elapsed > 0 {
		eps = float64(ingested) / elapsed
	}
	return models.PipelineStats{
		EventsIngested:    ingested,
		EventsPerSecond:   eps,
		AnomaliesDetected: p.detected.Load(),
		ProcessorLag:      p.lag.Load(),
	}
}

// GetWindows returns the current window aggregations.
func (p *Processor) GetWindows() []models.WindowedMetric {
	return p.windows.GetCurrentWindows()
}
