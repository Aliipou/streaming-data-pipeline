package processor

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
)

// windowKey identifies a 1-minute tumbling window bucket.
type windowKey struct {
	sensorType models.SensorType
	location   string
	bucket     int64 // unix minute
}

type windowBucket struct {
	values    []float64
	windowKey windowKey
}

// WindowAggregator computes tumbling 1-minute windows per sensor type + location.
type WindowAggregator struct {
	mu      sync.RWMutex
	buckets map[windowKey]*windowBucket
}

// NewWindowAggregator creates a new aggregator.
func NewWindowAggregator() *WindowAggregator {
	return &WindowAggregator{buckets: make(map[windowKey]*windowBucket)}
}

// Add appends a sensor event to the appropriate window bucket.
func (w *WindowAggregator) Add(event models.SensorEvent) {
	minute := event.Timestamp.Truncate(time.Minute).Unix()
	key := windowKey{sensorType: event.SensorType, location: event.Location, bucket: minute}

	w.mu.Lock()
	defer w.mu.Unlock()

	b, ok := w.buckets[key]
	if !ok {
		b = &windowBucket{windowKey: key}
		w.buckets[key] = b
	}
	b.values = append(b.values, event.Value)
}

// GetAllWindows returns aggregated WindowedMetrics for all completed buckets
// (buckets older than the current minute).
func (w *WindowAggregator) GetAllWindows() []models.WindowedMetric {
	w.mu.RLock()
	defer w.mu.RUnlock()

	now := time.Now().Truncate(time.Minute).Unix()
	var result []models.WindowedMetric

	for key, b := range w.buckets {
		if key.bucket >= now {
			continue // current minute, not yet complete
		}
		if len(b.values) == 0 {
			continue
		}
		result = append(result, computeWindow(key, b.values))
	}
	return result
}

// GetCurrentWindows returns aggregated metrics for in-progress windows.
func (w *WindowAggregator) GetCurrentWindows() []models.WindowedMetric {
	w.mu.RLock()
	defer w.mu.RUnlock()

	now := time.Now().Truncate(time.Minute).Unix()
	var result []models.WindowedMetric

	for key, b := range w.buckets {
		if key.bucket < now {
			continue
		}
		if len(b.values) == 0 {
			continue
		}
		result = append(result, computeWindow(key, b.values))
	}
	return result
}

// Prune removes buckets older than retainMinutes.
func (w *WindowAggregator) Prune(retainMinutes int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cutoff := time.Now().Add(-time.Duration(retainMinutes) * time.Minute).Truncate(time.Minute).Unix()
	for key := range w.buckets {
		if key.bucket < cutoff {
			delete(w.buckets, key)
		}
	}
}

func computeWindow(key windowKey, values []float64) models.WindowedMetric {
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	avg := sum / float64(len(sorted))

	p99Idx := int(math.Ceil(float64(len(sorted))*0.99)) - 1
	if p99Idx < 0 {
		p99Idx = 0
	}
	if p99Idx >= len(sorted) {
		p99Idx = len(sorted) - 1
	}

	bucketTime := time.Unix(key.bucket, 0)
	return models.WindowedMetric{
		SensorType:  key.sensorType,
		Location:    key.location,
		WindowStart: bucketTime,
		WindowEnd:   bucketTime.Add(time.Minute),
		Count:       int64(len(sorted)),
		Sum:         sum,
		Min:         sorted[0],
		Max:         sorted[len(sorted)-1],
		Avg:         avg,
		P99:         sorted[p99Idx],
	}
}
