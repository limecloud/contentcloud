package memory

import (
	"testing"
	"time"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

func TestRuntimeCleanupDiagnosticsAreTenantScopedAndObjectIdempotent(t *testing.T) {
	store := New()
	now := time.Now().UTC()
	value := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-1", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", RequestID: "request-1",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/tenant-1/task-1/final/temp.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), value); err != nil {
		t.Fatalf("create cleanup diagnostic: %v", err)
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), value); err == nil {
		t.Fatal("duplicate object key must be rejected")
	}
	got, err := store.RuntimeCleanupDiagnostic(t.Context(), value.TenantID, value.ID)
	if err != nil || got.ObjectKey != value.ObjectKey {
		t.Fatalf("read cleanup diagnostic = %#v, %v", got, err)
	}
	if _, err := store.RuntimeCleanupDiagnostic(t.Context(), "tenant-2", value.ID); err == nil {
		t.Fatal("cross-tenant cleanup diagnostic must be hidden")
	}
	items, err := store.RuntimeCleanupDiagnostics(t.Context(), value.TenantID, contentruntime.RuntimeCleanupPending, 10)
	if err != nil || len(items) != 1 || items[0].ID != value.ID {
		t.Fatalf("list cleanup diagnostics = %#v, %v", items, err)
	}
}

func TestRuntimeCleanupDiagnosticCASRejectsStaleAndIllegalTransitions(t *testing.T) {
	store := New()
	now := time.Now().UTC()
	value := contentruntime.RuntimeCleanupDiagnostic{
		ID: "cleanup-cas", TenantID: "tenant-1", ProjectID: "project-1", TaskID: "task-1", RequestID: "request-1",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      "media/tenant-1/task-1/final/cas.mp4", CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	if err := store.CreateRuntimeCleanupDiagnostic(t.Context(), value); err != nil {
		t.Fatal(err)
	}

	retrying := value
	retrying.Status = contentruntime.RuntimeCleanupRetrying
	retrying.AttemptCount = 1
	retrying.Version = 2
	retrying.UpdatedAt = now.Add(time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(t.Context(), retrying, value.Version); err != nil {
		t.Fatalf("pending -> retrying: %v", err)
	}
	if err := store.UpdateRuntimeCleanupDiagnostic(t.Context(), retrying, value.Version); err == nil {
		t.Fatal("stale CAS update unexpectedly succeeded")
	}

	cleaned := retrying
	cleaned.Status = contentruntime.RuntimeCleanupCleaned
	cleaned.Version = 3
	cleaned.UpdatedAt = now.Add(2 * time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(t.Context(), cleaned, retrying.Version); err != nil {
		t.Fatalf("retrying -> cleaned: %v", err)
	}
	terminalRetry := cleaned
	terminalRetry.Status = contentruntime.RuntimeCleanupRetrying
	terminalRetry.Version = 4
	terminalRetry.UpdatedAt = now.Add(3 * time.Second)
	if err := store.UpdateRuntimeCleanupDiagnostic(t.Context(), terminalRetry, cleaned.Version); err == nil {
		t.Fatal("terminal cleanup diagnostic unexpectedly became retryable")
	}
}
