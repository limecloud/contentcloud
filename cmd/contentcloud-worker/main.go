package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/store/postgres"
	"github.com/limecloud/contentcloud/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	databaseURL := os.Getenv("CONTENTCLOUD_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("CONTENTCLOUD_DATABASE_URL is required for the standalone worker")
		os.Exit(2)
	}
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		logger.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if os.Getenv("CONTENTCLOUD_AUTO_MIGRATE") == "1" {
		if err := store.Migrate(ctx); err != nil {
			logger.Error("apply migrations", "error", err)
			os.Exit(1)
		}
	}
	blobs, err := blob.NewFromEnv(ctx, env("CONTENTCLOUD_DATA_DIR", "var/data"))
	if err != nil {
		logger.Error("initialize object storage", "error", err)
		os.Exit(1)
	}
	service := app.NewWithBlob(store, logger, blobs)
	logger.Info("contentcloud deterministic worker ready", "zero_exec", true, "capabilities", []string{"source_ingestion", "policy_validation", "context_compile", "export"})
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		case <-ticker.C:
			processed, err := worker.ProcessPendingSources(ctx, service, 10)
			if err != nil {
				logger.Error("process sources", "error", err)
			} else if processed > 0 {
				logger.Info("processed sources", "count", processed)
			}
			mediaProcessed, mediaErr := worker.ProcessPendingMedia(ctx, service, 10)
			if mediaErr != nil {
				logger.Error("process media jobs", "error", mediaErr)
			} else if mediaProcessed > 0 {
				logger.Info("processed media jobs", "count", mediaProcessed)
			}
		}
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
