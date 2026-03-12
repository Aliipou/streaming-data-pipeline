package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
)

// ── SensorType constants ──────────────────────────────────────────────────────

func TestSensorTypeConstants(t *testing.T) {
	types := []models.SensorType{
		models.SensorTemperature,
		models.SensorPressure,
		models.SensorVibration,
		models.SensorHumidity,
	}
	seen := make(map[models.SensorType]bool)
	for _, st := range types {
		if st == "" {
			t.Error("sensor type should not be empty")
		}
		if seen[st] {
			t.Errorf("duplicate sensor type: %q", st)
		}
		seen[st] = true
	}
}

// ── SensorEvent serialization ─────────────────────────────────────────────────

func TestSensorEvent_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	event := models.SensorEvent{
		SensorID:   "sensor-001",
		SensorType: models.SensorTemperature,
		Location:   "floor-1",
		Value:      23.5,
		Unit:       "°C",
		Timestamp:  now,
		Tags:       map[string]string{"building": "A", "zone": "north"},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.SensorEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.SensorID != event.SensorID {
		t.Errorf("SensorID: got %q, want %q", decoded.SensorID, event.SensorID)
	}
	if decoded.SensorType != models.SensorTemperature {
		t.Errorf("SensorType: got %q, want temperature", decoded.SensorType)
	}
	if decoded.Value != event.Value {
		t.Errorf("Value: got %v, want %v", decoded.Value, event.Value)
	}
	if decoded.Location != event.Location {
		t.Errorf("Location: got %q, want %q", decoded.Location, event.Location)
	}
	if decoded.Tags["building"] != "A" {
		t.Errorf("Tags: got %v, want building=A", decoded.Tags)
	}
}

func TestSensorEvent_NoTags(t *testing.T) {
	event := models.SensorEvent{
		SensorID:   "s-1",
		SensorType: models.SensorPressure,
		Location:   "lab",
		Value:      1013.25,
		Unit:       "hPa",
		Timestamp:  time.Now(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded models.SensorEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Tags != nil && len(decoded.Tags) > 0 {
		t.Errorf("expected nil/empty tags, got %v", decoded.Tags)
	}
}

func TestSensorEvent_AllTypes(t *testing.T) {
	sensorTypes := []struct {
		sType models.SensorType
		unit  string
		value float64
	}{
		{models.SensorTemperature, "°C", 22.1},
		{models.SensorPressure, "hPa", 1013.0},
		{models.SensorVibration, "mm/s", 3.5},
		{models.SensorHumidity, "%", 55.0},
	}

	for _, tc := range sensorTypes {
		t.Run(string(tc.sType), func(t *testing.T) {
			event := models.SensorEvent{
				SensorID:   "s-1",
				SensorType: tc.sType,
				Location:   "test",
				Value:      tc.value,
				Unit:       tc.unit,
				Timestamp:  time.Now(),
			}
			data, _ := json.Marshal(event)
			var decoded models.SensorEvent
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Value != tc.value {
				t.Errorf("value mismatch: got %v, want %v", decoded.Value, tc.value)
			}
		})
	}
}

// ── WindowedMetric ────────────────────────────────────────────────────────────

func TestWindowedMetric_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	wm := models.WindowedMetric{
		SensorType:  models.SensorTemperature,
		Location:    "floor-2",
		WindowStart: now.Add(-time.Minute),
		WindowEnd:   now,
		Count:       60,
		Sum:         1380.0,
		Min:         21.5,
		Max:         24.5,
		Avg:         23.0,
		P99:         24.3,
	}

	data, err := json.Marshal(wm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.WindowedMetric
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Count != 60 {
		t.Errorf("Count: got %d, want 60", decoded.Count)
	}
	if decoded.Avg != 23.0 {
		t.Errorf("Avg: got %v, want 23.0", decoded.Avg)
	}
	if decoded.P99 != 24.3 {
		t.Errorf("P99: got %v, want 24.3", decoded.P99)
	}
	if decoded.Min > decoded.Max {
		t.Error("Min should not be greater than Max")
	}
}

// ── Anomaly ───────────────────────────────────────────────────────────────────

func TestAnomaly_Severity_JSONRoundTrip(t *testing.T) {
	severities := []string{"low", "medium", "high", "critical"}
	for _, sev := range severities {
		t.Run(sev, func(t *testing.T) {
			a := models.Anomaly{
				SensorID:   "s-1",
				SensorType: models.SensorVibration,
				Location:   "floor-3",
				Value:      25.0,
				Expected:   5.0,
				ZScore:     4.5,
				Severity:   sev,
				DetectedAt: time.Now(),
			}
			data, _ := json.Marshal(a)
			var decoded models.Anomaly
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Severity != sev {
				t.Errorf("Severity: got %q, want %q", decoded.Severity, sev)
			}
		})
	}
}

func TestAnomaly_ZScore(t *testing.T) {
	a := models.Anomaly{ZScore: 4.2, Severity: "high"}
	if a.ZScore < 0 {
		t.Error("ZScore should be non-negative for an anomaly")
	}
}

// ── PipelineStats ─────────────────────────────────────────────────────────────

func TestPipelineStats_JSONRoundTrip(t *testing.T) {
	stats := models.PipelineStats{
		EventsIngested:    100000,
		EventsPerSecond:   250.5,
		AnomaliesDetected: 42,
		ActiveSensors:     20,
		ProcessorLag:      15,
	}

	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.PipelineStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.EventsIngested != stats.EventsIngested {
		t.Errorf("EventsIngested: got %d, want %d", decoded.EventsIngested, stats.EventsIngested)
	}
	if decoded.EventsPerSecond != stats.EventsPerSecond {
		t.Errorf("EventsPerSecond: got %v, want %v", decoded.EventsPerSecond, stats.EventsPerSecond)
	}
	if decoded.ActiveSensors != stats.ActiveSensors {
		t.Errorf("ActiveSensors: got %d, want %d", decoded.ActiveSensors, stats.ActiveSensors)
	}
}

func TestPipelineStats_ZeroValue(t *testing.T) {
	var stats models.PipelineStats
	if stats.EventsIngested != 0 || stats.EventsPerSecond != 0 {
		t.Error("zero-value PipelineStats should have zero counters")
	}
}
