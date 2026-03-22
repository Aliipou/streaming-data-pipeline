package config

import (
	"os"
	"strconv"
)

type Config struct {
	KafkaBrokers       string
	KafkaTopic         string
	PostgresURL        string
	HTTPPort           int
	NumSensors         int
	ZScoreThreshold    float64
	EWMAThreshold      float64
	EWMAAlpha          float64
}

func Load() *Config {
	return &Config{
		KafkaBrokers:       getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:         getEnv("KAFKA_TOPIC", "sensor-events"),
		PostgresURL:        getEnv("POSTGRES_URL", "postgres://pipeline:password@localhost:5433/pipeline_db?sslmode=disable"),
		HTTPPort:           getEnvInt("HTTP_PORT", 8080),
		NumSensors:         getEnvInt("NUM_SENSORS", 20),
		ZScoreThreshold:    getEnvFloat("ZSCORE_THRESHOLD", 2.0),
		EWMAThreshold:      getEnvFloat("EWMA_THRESHOLD", 3.0),
		EWMAAlpha:          getEnvFloat("EWMA_ALPHA", 0.1),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getEnvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
