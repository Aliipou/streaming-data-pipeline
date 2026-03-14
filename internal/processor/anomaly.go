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
	if s.count < 2 || s.m2 == 0 {
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

// ── EWMA detector ────────────────────────────────────────────────────────────

const ewmaAlpha = 0.1 // slow-moving baseline; 0 < alpha < 1
const ewmaWarmup = 20  // minimum readings before detection is active

// ewmaState holds the exponentially-weighted mean and variance for one sensor.
type ewmaState struct {
	count    int
	mean     float64
	variance float64 // EWMA variance (initialised to 1 to avoid /0 on first spike)
}

// update applies one new observation using Welford-style EWMA update.
func (e *ewmaState) update(x float64) {
	e.count++
	if e.count == 1 {
		e.mean = x
		e.variance = 1.0
		return
	}
	diff := x - e.mean
	e.mean += ewmaAlpha * diff
	e.variance = (1 - ewmaAlpha) * (e.variance + ewmaAlpha*diff*diff)
}

func (e *ewmaState) std() float64 {
	if e.variance <= 0 {
		return 1.0
	}
	return math.Sqrt(e.variance)
}

// EWMADetector detects anomalies using an exponentially-weighted moving average
// baseline per sensor (alpha=0.1 for a slow-adapting reference signal).
type EWMADetector struct {
	mu     sync.Mutex
	states map[string]*ewmaState
}

// NewEWMADetector creates a new EWMADetector.
func NewEWMADetector() *EWMADetector {
	return &EWMADetector{states: make(map[string]*ewmaState)}
}

// Check updates the EWMA state and returns an Anomaly when the deviation score
// exceeds 3.0 and at least ewmaWarmup readings have been seen for that sensor.
func (e *EWMADetector) Check(event models.SensorEvent) *models.Anomaly {
	e.mu.Lock()
	defer e.mu.Unlock()

	st, ok := e.states[event.SensorID]
	if !ok {
		st = &ewmaState{}
		e.states[event.SensorID] = st
	}

	prevMean := st.mean
	prevVariance := st.variance
	st.update(event.Value)

	if st.count < ewmaWarmup {
		return nil
	}

	var prevStd float64
	if prevVariance <= 0 {
		prevStd = 1.0
	} else {
		prevStd = math.Sqrt(prevVariance)
	}
	deviation := math.Abs(event.Value-prevMean) / prevStd
	if deviation <= 3.0 {
		return nil
	}

	return &models.Anomaly{
		SensorID:   event.SensorID,
		SensorType: event.SensorType,
		Location:   event.Location,
		Value:      event.Value,
		Expected:   prevMean,
		ZScore:     deviation, // reuse ZScore field to carry the deviation score
		Severity:   ewmaSeverity(deviation),
		DetectedAt: event.Timestamp,
	}
}

func ewmaSeverity(dev float64) string {
	switch {
	case dev > 7.0:
		return "critical"
	case dev > 5.0:
		return "high"
	case dev > 4.0:
		return "medium"
	default: // > 3.0
		return "low"
	}
}

// ── Severity helpers ──────────────────────────────────────────────────────────

var severityRank = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

var severityByRank = []string{"", "low", "medium", "high", "critical"}

// escalateSeverity returns the next tier above sev, capped at "critical".
func escalateSeverity(sev string) string {
	rank, ok := severityRank[sev]
	if !ok {
		return sev
	}
	if rank+1 >= len(severityByRank) {
		return "critical"
	}
	return severityByRank[rank+1]
}

// higherSeverity returns whichever of a or b is the more severe.
func higherSeverity(a, b string) string {
	if severityRank[a] >= severityRank[b] {
		return a
	}
	return b
}

// ── Layered detector ─────────────────────────────────────────────────────────

// LayeredDetector runs both a Welford Z-score AnomalyDetector and an
// EWMADetector on every event. When both agree on an anomaly the severity is
// escalated by one tier. When only one fires, that detector's severity is used.
type LayeredDetector struct {
	welford *AnomalyDetector
	ewma    *EWMADetector
}

// NewLayeredDetector creates a LayeredDetector with fresh sub-detectors.
func NewLayeredDetector() *LayeredDetector {
	return &LayeredDetector{
		welford: NewAnomalyDetector(),
		ewma:    NewEWMADetector(),
	}
}

// Check runs both detectors and merges their results according to the agreement
// rules described on LayeredDetector.
func (l *LayeredDetector) Check(event models.SensorEvent) *models.Anomaly {
	wa := l.welford.Check(event)
	ea := l.ewma.Check(event)

	switch {
	case wa == nil && ea == nil:
		return nil

	case wa != nil && ea == nil:
		return wa

	case wa == nil && ea != nil:
		return ea

	default: // both fired — agree → escalate the higher severity
		merged := wa // use Welford's anomaly as the base (has Welford z-score)
		agreed := higherSeverity(wa.Severity, ea.Severity)
		merged.Severity = escalateSeverity(agreed)
		return merged
	}
}
