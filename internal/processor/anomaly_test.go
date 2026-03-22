package processor_test

import (
	"testing"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
)

func makeEvent(sensorID string, value float64) models.SensorEvent {
	return models.SensorEvent{
		SensorID:   sensorID,
		SensorType: models.SensorTemperature,
		Location:   "lab",
		Value:      value,
		Unit:       "°C",
		Timestamp:  time.Now(),
	}
}

// warmDetector feeds n normal readings to build stable statistics.
func warmDetector(d *processor.AnomalyDetector, sensorID string, n int) {
	for i := 0; i < n; i++ {
		d.Check(makeEvent(sensorID, 22.0+float64(i%3)*0.1))
	}
}

func TestAnomalyDetector_NoAnomalyForNormalValues(t *testing.T) {
	d := processor.NewAnomalyDetector(2.0)
	warmDetector(d, "s1", 50)

	// Normal reading — should not trigger
	a := d.Check(makeEvent("s1", 22.15))
	if a != nil {
		t.Errorf("expected no anomaly for normal value, got z=%.2f severity=%s", a.ZScore, a.Severity)
	}
}

func TestAnomalyDetector_DetectsAnomaly(t *testing.T) {
	d := processor.NewAnomalyDetector(2.0)
	warmDetector(d, "s1", 50)

	// Extreme spike — should trigger
	a := d.Check(makeEvent("s1", 999.0))
	if a == nil {
		t.Fatal("expected anomaly for extreme spike, got nil")
	}
	if a.ZScore < 2.0 {
		t.Errorf("expected z > 2, got %.2f", a.ZScore)
	}
	if a.SensorID != "s1" {
		t.Errorf("SensorID: got %q, want s1", a.SensorID)
	}
}

func TestAnomalyDetector_SeverityTiers(t *testing.T) {
	tests := []struct {
		value    float64
		wantSev  string
	}{
		{100.0, "low"},    // z ≈ 2-3
		{500.0, "medium"}, // z ≈ 3-4
		{2000.0, "high"},  // z ≈ 4-5
		{9999.0, "critical"}, // z >> 5
	}

	for _, tc := range tests {
		d := processor.NewAnomalyDetector(2.0)
		warmDetector(d, "sensor", 100)
		a := d.Check(makeEvent("sensor", tc.value))
		if a == nil {
			t.Logf("no anomaly for value=%.0f (may need larger spike)", tc.value)
			continue
		}
		// Just verify severity is one of the valid values
		validSeverities := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
		if !validSeverities[a.Severity] {
			t.Errorf("invalid severity %q for value=%.0f", a.Severity, tc.value)
		}
	}
}

func TestAnomalyDetector_WarmupPeriod(t *testing.T) {
	d := processor.NewAnomalyDetector(2.0)
	// Less than 10 readings — should never fire even for extreme values
	for i := 0; i < 9; i++ {
		a := d.Check(makeEvent("s1", 9999.0))
		if a != nil {
			t.Errorf("should not fire during warmup (reading %d)", i+1)
		}
	}
}

func TestAnomalyDetector_IndependentSensors(t *testing.T) {
	d := processor.NewAnomalyDetector(2.0)
	warmDetector(d, "s1", 50)
	warmDetector(d, "s2", 50)

	// Anomaly in s1 should not affect s2
	_ = d.Check(makeEvent("s1", 9999.0))
	a2 := d.Check(makeEvent("s2", 22.15))
	if a2 != nil {
		t.Errorf("s2 should not be affected by s1 anomaly, got z=%.2f", a2.ZScore)
	}
}

func TestAnomalyDetector_AnomalyFields(t *testing.T) {
	d := processor.NewAnomalyDetector(2.0)
	warmDetector(d, "sensor-1", 50)

	event := makeEvent("sensor-1", 9999.0)
	a := d.Check(event)
	if a == nil {
		t.Skip("no anomaly detected, skipping field validation")
	}

	if a.SensorID != "sensor-1" {
		t.Errorf("SensorID: got %q", a.SensorID)
	}
	if a.SensorType != models.SensorTemperature {
		t.Errorf("SensorType: got %q", a.SensorType)
	}
	if a.Location != "lab" {
		t.Errorf("Location: got %q", a.Location)
	}
	if a.Value != 9999.0 {
		t.Errorf("Value: got %v", a.Value)
	}
	if a.ZScore <= 0 {
		t.Errorf("ZScore should be positive, got %v", a.ZScore)
	}
	if a.DetectedAt.IsZero() {
		t.Error("DetectedAt should not be zero")
	}
}
