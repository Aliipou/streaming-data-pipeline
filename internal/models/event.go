package models

import "time"

type SensorType string

const (
	SensorTemperature SensorType = "temperature"
	SensorPressure    SensorType = "pressure"
	SensorVibration   SensorType = "vibration"
	SensorHumidity    SensorType = "humidity"
)

type SensorEvent struct {
	SensorID   string            `json:"sensor_id"`
	SensorType SensorType        `json:"sensor_type"`
	Location   string            `json:"location"`
	Value      float64           `json:"value"`
	Unit       string            `json:"unit"`
	Timestamp  time.Time         `json:"timestamp"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type WindowedMetric struct {
	SensorType  SensorType `json:"sensor_type"`
	Location    string     `json:"location"`
	WindowStart time.Time  `json:"window_start"`
	WindowEnd   time.Time  `json:"window_end"`
	Count       int64      `json:"count"`
	Sum         float64    `json:"sum"`
	Min         float64    `json:"min"`
	Max         float64    `json:"max"`
	Avg         float64    `json:"avg"`
	P99         float64    `json:"p99"`
}

type Anomaly struct {
	SensorID   string     `json:"sensor_id"`
	SensorType SensorType `json:"sensor_type"`
	Location   string     `json:"location"`
	Value      float64    `json:"value"`
	Expected   float64    `json:"expected"`
	ZScore     float64    `json:"z_score"`
	Severity   string     `json:"severity"` // low, medium, high, critical
	DetectedAt time.Time  `json:"detected_at"`
}

type PipelineStats struct {
	EventsIngested    int64   `json:"events_ingested"`
	EventsPerSecond   float64 `json:"events_per_second"`
	AnomaliesDetected int64   `json:"anomalies_detected"`
	ActiveSensors     int     `json:"active_sensors"`
	ProcessorLag      int64   `json:"processor_lag_ms"`
}
