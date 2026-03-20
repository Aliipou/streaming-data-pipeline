package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aliipou/streaming-data-pipeline/internal/config"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
	"github.com/aliipou/streaming-data-pipeline/internal/store"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	log, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s, err := store.New(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer s.Close()

	if err := s.Migrate(ctx); err != nil {
		log.Fatal("migrate", zap.Error(err))
	}

	p := processor.New(s, cfg.KafkaBrokers, cfg.KafkaTopic, log)
	log.Info("starting stream processor", zap.String("broker", cfg.KafkaBrokers), zap.String("topic", cfg.KafkaTopic))
	p.Run(ctx)
}
