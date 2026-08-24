package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

func testRuntimeCleanupDiagnostic(now time.Time, id, tenant, objectKey string) contentruntime.RuntimeCleanupDiagnostic {
	return contentruntime.RuntimeCleanupDiagnostic{
		ID: id, TenantID: tenant, ProjectID: "project-1", TaskID: "task-1", RequestID: "request-1",
		ManifestDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ObjectKey:      objectKey, CauseCode: "FINAL_RENDER_STORE_FAILED", CauseSummary: "最终成片事实写入失败",
		Status: contentruntime.RuntimeCleanupPending, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

func TestRuntimeCleanupOperatorRetryConvergesAndProtectsRoles(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	blobs := blob.NewMemory()
	service := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	now := time.Now().UTC()
	actor := Actor{UserID: "user-1", TenantID: "tenant-1", Role: "tenant_admin", Type: "user"}
	diagnostic := testRuntimeCleanupDiagnostic(now, "cleanup-success", actor.TenantID, "media/tenant-1/final/success.mp4")
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(ctx, diagnostic.ObjectKey, []byte("final")); err != nil {
		t.Fatal(err)
	}

	for _, executionType := range []string{"worker", "device"} {
		executionActor := actor
		executionActor.Role = "tenant_admin"
		executionActor.Type = executionType
		if _, err := service.Operations.RuntimeCleanupDiagnostics(ctx, executionActor, "", 10); err == nil {
			t.Fatalf("%s session unexpectedly passed manual cleanup authorization", executionType)
		}
	}
	result, err := service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, diagnostic.ID, "request-retry")
	if err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	if !result.DeleteAttempted || result.Diagnostic.Status != contentruntime.RuntimeCleanupCleaned || result.Diagnostic.Version != 3 {
		t.Fatalf("cleanup retry did not converge: %#v", result)
	}
	if _, err := blobs.Get(ctx, diagnostic.ObjectKey); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("cleaned object remains: %v", err)
	}
	if _, err := service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, diagnostic.ID, "request-retry-again"); err == nil {
		t.Fatal("terminal diagnostic unexpectedly retried")
	}
}

func TestRuntimeCleanupOperatorRetryHandlesNotFoundAndFailure(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	now := time.Now().UTC()
	actor := Actor{UserID: "user-2", TenantID: "tenant-2", Role: "project_manager", Type: "user"}
	service := NewWithBlob(DependenciesFrom(repo), nil, &notFoundRuntimeCleanupBlobStore{})
	notFound := testRuntimeCleanupDiagnostic(now, "cleanup-not-found", actor.TenantID, "media/tenant-2/final/missing.mp4")
	failed := testRuntimeCleanupDiagnostic(now, "cleanup-failed", actor.TenantID, "media/tenant-2/final/failing.mp4")
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, notFound); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, failed); err != nil {
		t.Fatal(err)
	}
	result, err := service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, notFound.ID, "request-not-found")
	if err != nil || result.Diagnostic.Status != contentruntime.RuntimeCleanupNotFound {
		t.Fatalf("not found cleanup result = %#v, %v", result, err)
	}
	service = NewWithBlob(DependenciesFrom(repo), nil, &failingRuntimeCleanupBlobStore{})
	result, err = service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, failed.ID, "request-failed")
	if err == nil || result.Diagnostic.Status != contentruntime.RuntimeCleanupFailed || result.Diagnostic.NextRetryAt == nil {
		t.Fatalf("failed cleanup result = %#v, %v", result, err)
	}
	if result.Diagnostic.CleanupError != "BLOB_DELETE_FAILED" || strings.Contains(result.Diagnostic.CleanupError, "secret") {
		t.Fatalf("cleanup error was not sanitized: %#v", result.Diagnostic)
	}
}

func TestRuntimeCleanupOperatorCASPreventsDuplicateConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	blobs := &countingRuntimeCleanupBlobStore{}
	service := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	actor := Actor{UserID: "user-concurrent", TenantID: "tenant-concurrent", Role: "tenant_admin", Type: "user"}
	diagnostic := testRuntimeCleanupDiagnostic(time.Now().UTC(), "cleanup-concurrent", actor.TenantID, "media/tenant-concurrent/final/concurrent.mp4")
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, diagnostic.ID, "request-concurrent")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var success, failure int
	for err := range errs {
		if err == nil {
			success++
		} else {
			failure++
		}
	}
	if success != 1 || failure != 1 || blobs.deleteCount != 1 {
		t.Fatalf("concurrent cleanup was not CAS protected: success=%d failure=%d deletes=%d", success, failure, blobs.deleteCount)
	}
}

func TestRuntimeCleanupOperatorCASWorksAcrossServiceInstances(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	blobs := &countingRuntimeCleanupBlobStore{}
	serviceA := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	serviceB := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	actor := Actor{UserID: "user-cross-instance", TenantID: "tenant-cross-instance", Role: "tenant_admin", Type: "user"}
	diagnostic := testRuntimeCleanupDiagnostic(time.Now().UTC(), "cleanup-cross-instance", actor.TenantID, "media/tenant-cross-instance/final.mp4")
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, service := range []*Application{serviceA, serviceB} {
		wg.Add(1)
		go func(service *Application) {
			defer wg.Done()
			_, err := service.Operations.RetryRuntimeCleanupDiagnostic(ctx, actor, diagnostic.ID, "request-cross-instance")
			errs <- err
		}(service)
	}
	wg.Wait()
	close(errs)
	var success, failure int
	for err := range errs {
		if err == nil {
			success++
		} else {
			failure++
		}
	}
	if success != 1 || failure != 1 || blobs.deleteCount != 1 {
		t.Fatalf("cross-instance cleanup was not CAS protected: success=%d failure=%d deletes=%d", success, failure, blobs.deleteCount)
	}
	final, err := repo.RuntimeCleanupDiagnostic(ctx, actor.TenantID, diagnostic.ID)
	if err != nil || final.Status != contentruntime.RuntimeCleanupCleaned || final.Version != 3 {
		t.Fatalf("cross-instance cleanup did not converge: %#v err=%v", final, err)
	}
}

func TestRuntimeCleanupReconcilerRecoversPendingAndExpiredClaimsAcrossApplicationInstances(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	blobs := blob.NewMemory()
	serviceA := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	serviceB := NewWithBlob(DependenciesFrom(repo), nil, blobs)
	now := time.Now().UTC()
	serviceB.Operations.now = func() time.Time { return now }
	actor := Actor{UserID: "user-reconciler", TenantID: "tenant-reconciler", Role: "tenant_admin", Type: "user"}
	pending := testRuntimeCleanupDiagnostic(now, "cleanup-reconcile-pending", actor.TenantID, "media/tenant-reconciler/pending.mp4")
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(ctx, pending.ObjectKey, []byte("pending")); err != nil {
		t.Fatal(err)
	}
	expired := testRuntimeCleanupDiagnostic(now, "cleanup-reconcile-expired", actor.TenantID, "media/tenant-reconciler/expired.mp4")
	expired.Status = contentruntime.RuntimeCleanupRetrying
	expired.AttemptCount = 1
	expired.Version = 2
	expired.UpdatedAt = now.Add(-runtimeCleanupClaimLease - time.Second)
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if err := blobs.Put(ctx, expired.ObjectKey, []byte("expired")); err != nil {
		t.Fatal(err)
	}
	if _, err := serviceA.Operations.RuntimeCleanupDiagnostic(ctx, actor, pending.ID); err != nil {
		t.Fatalf("first application instance could not read durable diagnostic: %v", err)
	}

	result, err := serviceB.Operations.ReconcileRuntimeCleanupDiagnostics(ctx, actor.TenantID, "worker-b", 10)
	if err != nil {
		t.Fatalf("reconcile cleanup diagnostics: %v", err)
	}
	if result.Scanned != 2 || result.Reclaimed != 1 || result.Claimed != 2 || result.Cleaned != 2 || result.NotFound != 0 || result.Failed != 0 {
		t.Fatalf("unexpected cleanup reconciliation result: %#v", result)
	}
	for _, value := range []contentruntime.RuntimeCleanupDiagnostic{pending, expired} {
		final, readErr := repo.RuntimeCleanupDiagnostic(ctx, actor.TenantID, value.ID)
		if readErr != nil || final.Status != contentruntime.RuntimeCleanupCleaned {
			t.Fatalf("reconciler did not converge %s: %#v err=%v", value.ID, final, readErr)
		}
		if _, readErr := blobs.Get(ctx, value.ObjectKey); !errors.Is(readErr, blob.ErrNotFound) {
			t.Fatalf("reconciler left object %s: %v", value.ObjectKey, readErr)
		}
	}
}

func TestRuntimeCleanupReconcilerHonorsFailureBackoff(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	service := NewWithBlob(DependenciesFrom(repo), nil, blob.NewMemory())
	now := time.Now().UTC()
	service.Operations.now = func() time.Time { return now }
	actor := Actor{UserID: "user-backoff", TenantID: "tenant-backoff", Role: "tenant_admin", Type: "user"}
	nextRetry := now.Add(time.Hour)
	diagnostic := testRuntimeCleanupDiagnostic(now, "cleanup-reconcile-backoff", actor.TenantID, "media/tenant-backoff/backoff.mp4")
	diagnostic.Status = contentruntime.RuntimeCleanupFailed
	diagnostic.AttemptCount = 1
	diagnostic.Version = 2
	diagnostic.NextRetryAt = &nextRetry
	if err := repo.CreateRuntimeCleanupDiagnostic(ctx, diagnostic); err != nil {
		t.Fatal(err)
	}
	result, err := service.Operations.ReconcileRuntimeCleanupDiagnostics(ctx, actor.TenantID, "worker-backoff", 10)
	if err != nil {
		t.Fatalf("reconcile backoff diagnostics: %v", err)
	}
	if result.Scanned != 1 || result.Skipped != 1 || result.Claimed != 0 {
		t.Fatalf("reconciler ignored failure backoff: %#v", result)
	}
	persisted, err := repo.RuntimeCleanupDiagnostic(ctx, actor.TenantID, diagnostic.ID)
	if err != nil || persisted.Status != contentruntime.RuntimeCleanupFailed || persisted.Version != diagnostic.Version {
		t.Fatalf("backoff diagnostic changed unexpectedly: %#v err=%v", persisted, err)
	}
}

type failingRuntimeCleanupBlobStore struct{}

func (*failingRuntimeCleanupBlobStore) Put(context.Context, string, []byte) error { return nil }
func (*failingRuntimeCleanupBlobStore) Get(context.Context, string) ([]byte, error) {
	return nil, blob.ErrNotFound
}
func (*failingRuntimeCleanupBlobStore) Delete(context.Context, string) error {
	return errors.New("secret local path delete failure")
}

type notFoundRuntimeCleanupBlobStore struct{}

func (*notFoundRuntimeCleanupBlobStore) Put(context.Context, string, []byte) error { return nil }
func (*notFoundRuntimeCleanupBlobStore) Get(context.Context, string) ([]byte, error) {
	return nil, blob.ErrNotFound
}
func (*notFoundRuntimeCleanupBlobStore) Delete(context.Context, string) error {
	return blob.ErrNotFound
}

type countingRuntimeCleanupBlobStore struct {
	mu          sync.Mutex
	deleteCount int
}

func (*countingRuntimeCleanupBlobStore) Put(context.Context, string, []byte) error { return nil }
func (*countingRuntimeCleanupBlobStore) Get(context.Context, string) ([]byte, error) {
	return []byte("present"), nil
}
func (s *countingRuntimeCleanupBlobStore) Delete(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteCount++
	return nil
}
