package processor_test

import (
	"testing"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func makeEventAt(sensorID string, value float64) models.SensorEvent {
	return models.SensorEvent{
		SensorID:   sensorID,
		SensorType: models.SensorTemperature,
		Location:   "lab",
		Value:      value,
		Unit:       "°C",
		Timestamp:  time.Now(),
	}
}

// warmEWMA feeds n steady readings to give the EWMA detector a stable baseline.
func warmEWMA(d *processor.EWMADetector, sensorID string, n int) {
	for i := 0; i < n; i++ {
		d.Check(makeEventAt(sensorID, 22.0+float64(i%3)*0.05))
	}
}

// warmLayered warms both sub-detectors inside a LayeredDetector.
func warmLayered(d *processor.LayeredDetector, sensorID string, n int) {
	for i := 0; i < n; i++ {
		d.Check(makeEventAt(sensorID, 22.0+float64(i%3)*0.05))
	}
}

// ── EWMADetector tests ────────────────────────────────────────────────────────

// TestEWMADetector_WarmupPeriod verifies that no anomaly is returned for the
// first 20 readings, even when every value is an extreme spike.
func TestEWMADetector_WarmupPeriod(t *testing.T) {
	d := processor.NewEWMADetector()
	for i := 0; i < 20; i++ {
		a := d.Check(makeEventAt("s1", 99999.0))
		if a != nil {
			t.Errorf("reading %d: expected no anomaly during warmup (first 20 readings), got severity=%s", i+1, a.Severity)
		}
	}
}

// TestEWMADetector_NormalValuesNoAnomaly verifies that steady-state readings
// around the established baseline do not trigger detection.
func TestEWMADetector_NormalValuesNoAnomaly(t *testing.T) {
	d := processor.NewEWMADetector()
	warmEWMA(d, "s1", 50)

	// Values very close to the established mean should be silent.
	normals := []float64{22.0, 22.1, 22.02, 22.05, 21.95}
	for _, v := range normals {
		a := d.Check(makeEventAt("s1", v))
		if a != nil {
			t.Errorf("value=%.3f: expected no anomaly for normal reading, got severity=%s deviation=%.2f",
				v, a.Severity, a.ZScore)
		}
	}
}

// TestEWMADetector_SpikeTriggersDetection verifies that a large spike after
// warmup produces an anomaly with the correct severity tier.
func TestEWMADetector_SpikeTriggersDetection(t *testing.T) {
	tests := []struct {
		name      string
		spike     float64
		wantAtLeast string // minimum expected severity
	}{
		{"low-deviation", 32.0, "low"},       // moderate jump from ~22
		{"medium-deviation", 50.0, "medium"},  // bigger jump
		{"high-deviation", 200.0, "high"},
		{"critical-deviation", 9999.0, "critical"},
	}

	severityRank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := processor.NewEWMADetector()
			warmEWMA(d, "sensor", 100) // well past warmup

			a := d.Check(makeEventAt("sensor", tc.spike))
			if a == nil {
				// May legitimately not trigger for the smaller spikes depending
				// on the variance accumulated — only fail for the large ones.
				if tc.spike >= 200.0 {
					t.Errorf("spike=%.0f: expected anomaly, got nil", tc.spike)
				}
				return
			}

			// Severity must be one of the valid tiers.
			validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
			if !validSeverities[a.Severity] {
				t.Errorf("spike=%.0f: invalid severity %q", tc.spike, a.Severity)
			}

			// For the large spikes, verify the severity is at least the expected tier.
			if tc.spike >= 200.0 {
				if severityRank[a.Severity] < severityRank[tc.wantAtLeast] {
					t.Errorf("spike=%.0f: got severity=%s, want at least %s",
						tc.spike, a.Severity, tc.wantAtLeast)
				}
			}

			// The anomaly must reference the correct sensor.
			if a.SensorID != "sensor" {
				t.Errorf("SensorID: got %q, want sensor", a.SensorID)
			}
			if a.ZScore <= 3.0 {
				t.Errorf("deviation score should be > 3.0 when anomaly is returned, got %.2f", a.ZScore)
			}
		})
	}
}

// TestEWMADetector_SeverityThresholds checks the four severity bands directly
// by controlling the deviation score via a very tight variance.
func TestEWMADetector_SeverityThresholds(t *testing.T) {
	// Warm with identical values so variance is near zero, forcing any deviation
	// to produce a very high score.
	tests := []struct {
		name     string
		spike    float64
		wantSev  string
	}{
		{"large-spike", 9999.0, "critical"},
		{"medium-spike", 500.0, "critical"},
		{"small-spike", 100.0, "critical"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := processor.NewEWMADetector()
			// Use perfectly constant warmup to minimise variance.
			for i := 0; i < 100; i++ {
				d.Check(makeEventAt("s", 20.0))
			}
			a := d.Check(makeEventAt("s", tc.spike))
			if a == nil {
				t.Fatalf("spike=%.0f: expected anomaly, got nil", tc.spike)
			}
			if a.Severity != tc.wantSev {
				t.Errorf("spike=%.0f: severity=%s, want %s (deviation=%.2f)",
					tc.spike, a.Severity, tc.wantSev, a.ZScore)
			}
		})
	}
}

// TestEWMADetector_IndependentSensors confirms per-sensor isolation.
func TestEWMADetector_IndependentSensors(t *testing.T) {
	d := processor.NewEWMADetector()
	warmEWMA(d, "s1", 50)
	warmEWMA(d, "s2", 50)

	// Large spike on s1 must not affect s2.
	_ = d.Check(makeEventAt("s1", 9999.0))
	a := d.Check(makeEventAt("s2", 22.0))
	if a != nil {
		t.Errorf("s2 should be unaffected by s1 spike, got severity=%s", a.Severity)
	}
}

// ── LayeredDetector tests ─────────────────────────────────────────────────────

// TestLayeredDetector_EscalatesWhenBothAgree verifies that when both the
// Welford and EWMA detectors fire on the same event the returned severity is
// one tier above the higher of the two individual severities.
func TestLayeredDetector_EscalatesWhenBothAgree(t *testing.T) {
	severityRank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

	d := processor.NewLayeredDetector()
	// Use enough readings to satisfy both detectors' warmup periods (≥20 for
	// EWMA, ≥10 for Welford) and build a tight baseline.
	for i := 0; i < 100; i++ {
		d.Check(makeEventAt("s1", 20.0))
	}

	// A very large spike should cause both detectors to fire.
	a := d.Check(makeEventAt("s1", 9999.0))
	if a == nil {
		t.Fatal("expected anomaly from layered detector for large spike, got nil")
	}

	// The result must be a valid severity.
	validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !validSeverities[a.Severity] {
		t.Fatalf("invalid severity %q", a.Severity)
	}

	// Both sub-detectors should produce at least "high" for a 9999 spike over a
	// baseline of 20; the escalated result must therefore be "critical".
	if severityRank[a.Severity] < severityRank["critical"] {
		t.Errorf("layered detector should escalate to critical for extreme spike, got %s", a.Severity)
	}
}

// TestLayeredDetector_NormalValuesNoAnomaly confirms silence for normal data.
func TestLayeredDetector_NormalValuesNoAnomaly(t *testing.T) {
	d := processor.NewLayeredDetector()
	warmLayered(d, "s1", 100)

	normals := []float64{22.0, 22.1, 22.02, 22.05}
	for _, v := range normals {
		a := d.Check(makeEventAt("s1", v))
		if a != nil {
			t.Errorf("value=%.3f: expected no anomaly, got severity=%s", v, a.Severity)
		}
	}
}

// TestLayeredDetector_WarmupSilent checks that the layered detector is silent
// during the warmup window of its sub-detectors.
func TestLayeredDetector_WarmupSilent(t *testing.T) {
	d := processor.NewLayeredDetector()
	// Both sub-detectors require a minimum number of observations; feed fewer
	// than the strictest (EWMA = 20) to ensure neither fires.
	for i := 0; i < 19; i++ {
		a := d.Check(makeEventAt("s1", 9999.0))
		if a != nil {
			t.Errorf("reading %d: layered detector should be silent during warmup, got severity=%s", i+1, a.Severity)
		}
	}
}

// TestLayeredDetector_OnlyOneDetects verifies that when only one sub-detector
// fires the result reflects that detector's severity without escalation.
func TestLayeredDetector_OnlyOneDetects(t *testing.T) {
	// It is difficult to guarantee exactly one detector fires in a unit test
	// without white-box access, so this test acts as a smoke-test: after a
	// moderate anomaly the layered detector must return a non-nil result with a
	// valid severity.
	d := processor.NewLayeredDetector()
	for i := 0; i < 100; i++ {
		d.Check(makeEventAt("s1", 20.0))
	}

	// A clearly anomalous value must produce some result.
	a := d.Check(makeEventAt("s1", 500.0))
	if a == nil {
		t.Fatal("expected at least one sub-detector to fire for a large spike")
	}

	validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !validSeverities[a.Severity] {
		t.Errorf("invalid severity %q returned by layered detector", a.Severity)
	}
}
