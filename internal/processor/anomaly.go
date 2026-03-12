package processor

import (
	"math"
	"sync"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
)

// sensorStats tracks rolling mean and variance for a single sensor.
type sensorStats struct {
	count int64
	mean  float64
	m2    float64 // Welford's online variance accumulator
}

func (s *sensorStats) update(x float64) {
	s.count++
	delta := x - s.mean
	s.mean += delta / float64(s.count)
	delta2 := x - s.mean
	s.m2 += delta * delta2
}

func (s *sensorStats) stddev() float64 {
	if s.count < 2 {
		return 1.0
	}
	return math.Sqrt(s.m2 / float64(s.count-1))
}

// AnomalyDetector uses Welford's online Z-score algorithm per sensor.
type AnomalyDetector struct {
	mu    sync.Mutex
	stats map[string]*sensorStats
}

// NewAnomalyDetector creates a new detector.
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{stats: make(map[string]*sensorStats)}
}

// Check updates rolling stats and returns an Anomaly if Z-score exceeds 2.
func (a *AnomalyDetector) Check(event models.SensorEvent) *models.Anomaly {
	a.mu.Lock()
	defer a.mu.Unlock()

	s, ok := a.stats[event.SensorID]
	if !ok {
		s = &sensorStats{}
		a.stats[event.SensorID] = s
	}

	expected := s.mean
	s.update(event.Value)
	stddev := s.stddev()

	zScore := math.Abs(event.Value-expected) / stddev
	if s.count < 10 || zScore < 2.0 {
		return nil
	}

	severity := zScoreSeverity(zScore)
	return &models.Anomaly{
		SensorID:   event.SensorID,
		SensorType: event.SensorType,
		Location:   event.Location,
		Value:      event.Value,
		Expected:   expected,
		ZScore:     zScore,
		Severity:   severity,
		DetectedAt: event.Timestamp,
	}
}

func zScoreSeverity(z float64) string {
	switch {
	case z >= 5.0:
		return "critical"
	case z >= 4.0:
		return "high"
	case z >= 3.0:
		return "medium"
	default:
		return "low"
	}
}
