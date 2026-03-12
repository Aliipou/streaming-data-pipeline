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
	store    *store.Store
	windows  *WindowAggregator
	anomaly  *AnomalyDetector
	broker   string
	topic    string
	log      *zap.Logger
	ingested atomic.Int64
	detected atomic.Int64
	lag      atomic.Int64
}

// New creates a Processor connected to the given store and Kafka config.
func New(s *store.Store, broker, topic string, log *zap.Logger) *Processor {
	return &Processor{
		store:   s,
		windows: NewWindowAggregator(),
		anomaly: NewAnomalyDetector(),
		broker:  broker,
		topic:   topic,
		log:     log,
	}
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

	// Prune old windows every 5 minutes
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
		if err := p.processMessage(ctx, msg.Value); err != nil {
			p.log.Error("process message", zap.Error(err))
		}
		p.lag.Store(time.Since(start).Milliseconds())

		if err := reader.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			p.log.Warn("commit message", zap.Error(err))
		}
	}
	p.log.Info("stream processor stopped")
}

func (p *Processor) processMessage(ctx context.Context, data []byte) error {
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

	return p.store.SaveEvent(ctx, event)
}

// GetStats returns current pipeline statistics.
func (p *Processor) GetStats() models.PipelineStats {
	ingested := p.ingested.Load()
	return models.PipelineStats{
		EventsIngested:    ingested,
		EventsPerSecond:   float64(ingested) / time.Since(time.Now()).Abs().Seconds(),
		AnomaliesDetected: p.detected.Load(),
		ProcessorLag:      p.lag.Load(),
	}
}

// GetWindows returns the current window aggregations.
func (p *Processor) GetWindows() []models.WindowedMetric {
	return p.windows.GetCurrentWindows()
}
