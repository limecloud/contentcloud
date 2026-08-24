package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	storepg "github.com/limecloud/contentcloud/internal/persistence/postgres"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

// TestRuntimeCleanupDiagnosticCASWithPostgres requires a dedicated integration
// database and never connects to a developer or production database by default.
func TestRuntimeCleanupDiagnosticCASWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := idgen.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(ctx, fmt.Sprintf("cleanup-pg-%s@example.com", suffix), "long-enough-password", "Cleanup Owner", "Cleanup Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-pg-" + suffix, TenantID: actor.TenantID, ProjectID: "project-1", TaskID: "task-1", RequestID: "request-1",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/" + suffix + "/final.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	retrying := diagnostic
	retrying.Status = contentruntime.RuntimeCleanupRetrying
	retrying.AttemptCount = 1
	retrying.Version = 2
	retrying.UpdatedAt = now.Add(time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(ctx, retrying, diagnostic.Version); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateRuntimeCleanupDiagnostic(ctx, retrying, diagnostic.Version); err == nil {
		t.Fatal("stale CAS update unexpectedly succeeded")
	}
	cleaned := retrying
	cleaned.Status = contentruntime.RuntimeCleanupCleaned
	cleaned.Version = 3
	cleaned.UpdatedAt = now.Add(2 * time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(ctx, cleaned, retrying.Version); err != nil {
		t.Fatal(err)
	}
	terminalRetry := cleaned
	terminalRetry.Status = contentruntime.RuntimeCleanupRetrying
	terminalRetry.Version = 4
	terminalRetry.UpdatedAt = now.Add(3 * time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(ctx, terminalRetry, cleaned.Version); err == nil {
		t.Fatal("terminal cleanup diagnostic unexpectedly became retryable")
	}
	if _, err := store.RuntimeCleanupDiagnostic(ctx, "tenant-other-"+suffix, diagnostic.ID); err == nil {
		t.Fatal("cross-tenant cleanup diagnostic unexpectedly became visible")
	}
}

func TestRuntimeCleanupDiagnosticCrossStoreWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	first, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	suffix := idgen.New()
	service := application.New(application.DependenciesFrom(first), slog.Default())
	session, err := service.Identity.Register(ctx, fmt.Sprintf("cleanup-cross-store-%s@example.com", suffix), "long-enough-password", "Cleanup Owner", "Cleanup Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	diagnostic := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-cross-store-" + suffix, TenantID: actor.TenantID, ProjectID: "project-cross", TaskID: "task-cross", RequestID: "request-cross",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/" + suffix + "/cross-store.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := first.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	read, err := second.RuntimeCleanupDiagnostic(ctx, actor.TenantID, diagnostic.ID)
	if err != nil || read.Status != contentruntime.RuntimeCleanupPending {
		t.Fatalf("second store could not read diagnostic: %#v err=%v", read, err)
	}
	read.Status = contentruntime.RuntimeCleanupRetrying
	read.Version = 2
	read.UpdatedAt = now.Add(time.Second)
	if err := second.UpdateRuntimeCleanupDiagnostic(ctx, read, 1); err != nil {
		t.Fatal(err)
	}
	persisted, err := first.RuntimeCleanupDiagnostic(ctx, actor.TenantID, diagnostic.ID)
	if err != nil || persisted.Status != contentruntime.RuntimeCleanupRetrying || persisted.Version != 2 {
		t.Fatalf("first store did not observe second store update: %#v err=%v", persisted, err)
	}
}
