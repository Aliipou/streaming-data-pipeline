package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aliipou/streaming-data-pipeline/internal/api"
	"github.com/aliipou/streaming-data-pipeline/internal/config"
	"github.com/aliipou/streaming-data-pipeline/internal/processor"
	"github.com/aliipou/streaming-data-pipeline/internal/store"
	"github.com/gin-gonic/gin"
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

	s, err := store.New(ctx, cfg.PostgresURL)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer s.Close()

	// The API server connects to Kafka read-only to get processor stats via a
	// shared processor that is driven by the processor binary.
	// Here we spin up a local read-only processor for stats only.
	p := processor.New(s, cfg.KafkaBrokers, cfg.KafkaTopic, log)
	go p.Run(ctx)

	hub := api.NewHub(s, p, log)
	go hub.Run(ctx)

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(api.Logger(log), api.CORS(), gin.Recovery())

	h := api.New(s, p, hub, log)
	h.RegisterRoutes(engine)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Info("API server listening", zap.Int("port", cfg.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("listen", zap.Error(err))
		}
	}()

	<-ctx.Done()
	log.Info("shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Info("server stopped")
}
