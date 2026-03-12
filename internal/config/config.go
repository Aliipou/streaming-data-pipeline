package config

import (
	"os"
	"strconv"
)

type Config struct {
	KafkaBrokers string
	KafkaTopic   string
	PostgresURL  string
	HTTPPort     int
	NumSensors   int
}

func Load() *Config {
	return &Config{
		KafkaBrokers: getEnv("KAFKA_BROKERS", "localhost:9092"),
		KafkaTopic:   getEnv("KAFKA_TOPIC", "sensor-events"),
		PostgresURL:  getEnv("POSTGRES_URL", "postgres://pipeline:password@localhost:5433/pipeline_db?sslmode=disable"),
		HTTPPort:     getEnvInt("HTTP_PORT", 8080),
		NumSensors:   getEnvInt("NUM_SENSORS", 20),
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
