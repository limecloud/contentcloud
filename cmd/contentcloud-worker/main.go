package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/postgres"
	"github.com/limecloud/contentcloud/internal/runtime/worker"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	if err := newCommand().Execute(); err != nil {
		slog.Error("contentcloud worker stopped", "error", err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	settings := viper.New()
	settings.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	settings.AutomaticEnv()
	command := &cobra.Command{Use: "contentcloud-worker", Short: "启动 ContentCloud Worker", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true}
	for _, flag := range []struct{ name, key, environment, usage string }{
		{"database-url", "database_url", "CONTENTCLOUD_DATABASE_URL", "PostgreSQL 连接串（仅通过 Secret 或受控环境变量提供）"},
		{"data-dir", "data_dir", "CONTENTCLOUD_DATA_DIR", "对象数据目录"},
	} {
		command.Flags().String(flag.name, "", flag.usage)
		_ = settings.BindPFlag(flag.key, command.Flags().Lookup(flag.name))
		_ = settings.BindEnv(flag.key, flag.environment)
	}
	command.RunE = func(_ *cobra.Command, _ []string) error {
		for _, binding := range []struct{ key, environment string }{{"database_url", "CONTENTCLOUD_DATABASE_URL"}, {"data_dir", "CONTENTCLOUD_DATA_DIR"}} {
			if value := strings.TrimSpace(settings.GetString(binding.key)); value != "" {
				if err := os.Setenv(binding.environment, value); err != nil {
					return err
				}
			}
		}
		run()
		return nil
	}
	return command
}

func run() {
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
	serviceOptions := []application.Option{}
	dependencies := application.DependenciesFrom(store)
	seedance25Provider, seedance25Err := application.Seedance25ProviderFromEnv(dependencies.Artifacts, dependencies.Review, blobs)
	if seedance25Err != nil {
		logger.Error("configure Seedance 2.5 Provider", "error", seedance25Err)
		os.Exit(1)
	}
	if seedance25Provider != nil {
		serviceOptions = append(serviceOptions, application.WithMediaProviderAdapter(application.Seedance25ProviderID, seedance25Provider))
	}
	service := application.NewWithBlob(dependencies, logger, blobs, serviceOptions...)
	runtimeWorkerID := worker.RuntimeEventWorkerID()
	capabilities := []string{"runtime_event_delivery", "business_result_materialization", "runtime_projection", "runtime_cleanup_reconciliation", "source_ingestion", "policy_validation", "context_compile", "export"}
	if seedance25Provider != nil {
		capabilities = append(capabilities, "seedance25_media")
	}
	logger.Info("contentcloud deterministic worker ready", "zero_exec", seedance25Provider == nil, "seedance25_enabled", seedance25Provider != nil, "capabilities", capabilities)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		case <-ticker.C:
			runtimeEvents, runtimeErr := worker.ProcessRuntimeEvents(ctx, dependencies.Identity, dependencies.Runtime, service, runtimeWorkerID, 50)
			if runtimeErr != nil {
				logger.Error("process runtime events", "error", runtimeErr)
			} else if runtimeEvents.BusinessClaimed > 0 || runtimeEvents.ProjectionClaims > 0 || runtimeEvents.CleanupClaimed > 0 || runtimeEvents.CleanupReclaimed > 0 {
				logger.Info("processed runtime events", "business_claimed", runtimeEvents.BusinessClaimed, "business_applied", runtimeEvents.BusinessApplied, "business_retried", runtimeEvents.BusinessRetried, "projection_claimed", runtimeEvents.ProjectionClaims, "projected", runtimeEvents.Projected, "cleanup_claimed", runtimeEvents.CleanupClaimed, "cleanup_reclaimed", runtimeEvents.CleanupReclaimed, "cleanup_cleaned", runtimeEvents.CleanupCleaned, "cleanup_not_found", runtimeEvents.CleanupNotFound, "cleanup_failed", runtimeEvents.CleanupFailed)
			}
			processed, err := worker.ProcessPendingSources(ctx, service, 10)
			if err != nil {
				logger.Error("process sources", "error", err)
			} else if processed > 0 {
				logger.Info("processed sources", "count", processed)
			}
			mediaProcessed, mediaErr := worker.ProcessPendingMedia(ctx, service.Delivery, 10)
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
