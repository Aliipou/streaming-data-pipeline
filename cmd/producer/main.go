package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aliipou/streaming-data-pipeline/internal/config"
	"github.com/aliipou/streaming-data-pipeline/internal/producer"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	log, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p := producer.New(cfg.KafkaBrokers, cfg.KafkaTopic)
	defer p.Close()

	sim := producer.NewSensorSimulator(p, cfg.NumSensors, log)
	log.Info("starting sensor simulator", zap.Int("sensors", cfg.NumSensors), zap.String("topic", cfg.KafkaTopic))
	sim.Run(ctx)
}
