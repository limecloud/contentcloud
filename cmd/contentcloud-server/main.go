package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/httpapi"
	"github.com/limecloud/contentcloud/internal/serverconfig"
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
	addr := env("CONTENTCLOUD_ADDR", ":8080")
	webDist := env("CONTENTCLOUD_WEB_DIST", "web/dist")
	devMode := os.Getenv("CONTENTCLOUD_DEV_MODE") == "1" || os.Getenv("CONTENTCLOUD_DEV_MODE") == "true"
	adminEmails := splitValues(os.Getenv("CONTENTCLOUD_PLATFORM_ADMIN_EMAILS"))
	if devMode {
		adminEmails = append(adminEmails, "demo@contentcloud.local")
	}
	environmentRuntime, err := serverconfig.EnvironmentFromEnv()
	if err != nil {
		logger.Error("initialize Environment Control Plane", "error", err)
		os.Exit(1)
	}
	serviceOptions := []app.Option{app.WithPlatformAdminEmails(adminEmails...)}
	if environmentRuntime.Enabled {
		serviceOptions = append(serviceOptions, app.WithEnvironmentControlPlane(environmentRuntime.ControlPlane))
		if len(environmentRuntime.AutomationRequirements) > 0 {
			serviceOptions = append(serviceOptions, app.WithAutomationExecutionPolicy(environmentRuntime.AutomationRequirements, environmentRuntime.AutomationPackIDs))
		}
	}
	service := app.NewWithBlob(st, logger, blobStore, serviceOptions...)
	logger.Info("Environment Control Plane configured", "enabled", environmentRuntime.Enabled, "automation_policy", len(environmentRuntime.AutomationRequirements) > 0)
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
					if _, err := worker.ProcessPendingMedia(workerCtx, service, 5); err != nil {
						logger.Error("development media processing", "error", err)
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

func splitValues(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := strings.TrimSpace(part); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
