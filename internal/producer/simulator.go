package producer

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/models"
	"go.uber.org/zap"
)

type sensorSpec struct {
	sType    models.SensorType
	unit     string
	normal   [2]float64 // min, max for normal readings
	anomaly  float64    // anomaly value
	location string
}

var sensorSpecs = []sensorSpec{
	{models.SensorTemperature, "°C", [2]float64{18, 35}, 55.0, "floor-1"},
	{models.SensorTemperature, "°C", [2]float64{20, 32}, 52.0, "floor-2"},
	{models.SensorPressure, "hPa", [2]float64{950, 1050}, 1250.0, "lab-a"},
	{models.SensorPressure, "hPa", [2]float64{960, 1040}, 1200.0, "lab-b"},
	{models.SensorVibration, "mm/s", [2]float64{0, 8}, 25.0, "machine-1"},
	{models.SensorVibration, "mm/s", [2]float64{0, 10}, 30.0, "machine-2"},
	{models.SensorHumidity, "%", [2]float64{35, 65}, 95.0, "server-room"},
	{models.SensorHumidity, "%", [2]float64{40, 70}, 92.0, "storage"},
}

// SensorSimulator generates synthetic sensor data and publishes to Kafka.
type SensorSimulator struct {
	producer   *Producer
	numSensors int
	log        *zap.Logger
}

// NewSensorSimulator creates a simulator with the given number of sensors.
func NewSensorSimulator(p *Producer, numSensors int, log *zap.Logger) *SensorSimulator {
	return &SensorSimulator{producer: p, numSensors: numSensors, log: log}
}

// Run spawns one goroutine per sensor and blocks until ctx is cancelled.
func (s *SensorSimulator) Run(ctx context.Context) {
	s.log.Info("sensor simulator started", zap.Int("sensors", s.numSensors))
	done := make(chan struct{})
	for i := 0; i < s.numSensors; i++ {
		go func(id int) {
			s.runSensor(ctx, id)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < s.numSensors; i++ {
		<-done
	}
	s.log.Info("sensor simulator stopped")
}

func (s *SensorSimulator) runSensor(ctx context.Context, id int) {
	spec := sensorSpecs[id%len(sensorSpecs)]
	sensorID := fmt.Sprintf("sensor-%03d", id+1)
	rng := rand.New(rand.NewSource(int64(id) + time.Now().UnixNano()))

	// Random interval 1-5 seconds
	interval := time.Duration(1000+rng.Intn(4000)) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			value := s.generateValue(rng, spec)
			event := models.SensorEvent{
				SensorID:   sensorID,
				SensorType: spec.sType,
				Location:   spec.location,
				Value:      value,
				Unit:       spec.unit,
				Timestamp:  time.Now(),
				Tags:       map[string]string{"building": "main"},
			}
			if err := s.producer.Publish(ctx, event); err != nil {
				if ctx.Err() == nil {
					s.log.Warn("publish failed", zap.String("sensor", sensorID), zap.Error(err))
				}
				return
			}
		}
	}
}

// generateValue returns a normal reading 95% of the time, anomaly 5%.
func (s *SensorSimulator) generateValue(rng *rand.Rand, spec sensorSpec) float64 {
	if rng.Float64() < 0.05 {
		// Anomaly: value above normal range with some jitter
		return spec.anomaly + rng.Float64()*5
	}
	lo, hi := spec.normal[0], spec.normal[1]
	return lo + rng.Float64()*(hi-lo)
}
