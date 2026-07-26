package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/httpapi"
	storepkg "github.com/limecloud/contentcloud/internal/store"
	"github.com/limecloud/contentcloud/internal/store/memory"
	"github.com/limecloud/contentcloud/internal/store/postgres"
	"github.com/limecloud/contentcloud/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	var st storepkg.Store = memory.New()
	var closeStore func()
	databaseURL := os.Getenv("CONTENTCLOUD_DATABASE_URL")
	if databaseURL != "" {
		postgresStore, err := postgres.New(context.Background(), databaseURL)
		if err != nil {
			logger.Error("connect PostgreSQL", "error", err)
			os.Exit(1)
		}
		if os.Getenv("CONTENTCLOUD_AUTO_MIGRATE") == "1" {
			if err := postgresStore.Migrate(context.Background()); err != nil {
				logger.Error("apply PostgreSQL migrations", "error", err)
				os.Exit(1)
			}
		}
		st = postgresStore
		closeStore = postgresStore.Close
		logger.Info("using PostgreSQL store", "auto_migrate", os.Getenv("CONTENTCLOUD_AUTO_MIGRATE") == "1")
	}
	if closeStore != nil {
		defer closeStore()
	}
	blobStore, err := blob.NewFromEnv(context.Background(), env("CONTENTCLOUD_DATA_DIR", "var/data"))
	if err != nil {
		logger.Error("initialize object storage", "error", err)
		os.Exit(1)
	}
	service := app.NewWithBlob(st, logger, blobStore)
	addr := env("CONTENTCLOUD_ADDR", ":8080")
	webDist := env("CONTENTCLOUD_WEB_DIST", "web/dist")
	devMode := os.Getenv("CONTENTCLOUD_DEV_MODE") == "1" || os.Getenv("CONTENTCLOUD_DEV_MODE") == "true"
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	if devMode && databaseURL == "" {
		go func() {
			ticker := time.NewTicker(750 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
					if _, err := worker.ProcessPendingSources(workerCtx, service, 5); err != nil {
						logger.Error("development source ingestion", "error", err)
					}
				}
			}
		}()
	}
	server := &http.Server{Addr: addr, Handler: httpapi.New(service, logger, devMode, webDist).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		logger.Info("contentcloud server listening", "addr", addr, "dev_mode", devMode, "zero_exec", true)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancelWorker()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
