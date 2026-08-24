package application

import (
	"context"
	"errors"
	"testing"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

type artifactStorageTestRepository struct {
	err error
}

func (r artifactStorageTestRepository) CreateArtifact(context.Context, deliverydomain.Artifact) error {
	return r.err
}
func (artifactStorageTestRepository) ArtifactsByApprovedSnapshot(context.Context, string, string) ([]deliverydomain.Artifact, error) {
	return nil, nil
}
func (artifactStorageTestRepository) Artifact(context.Context, string, string) (deliverydomain.Artifact, error) {
	return deliverydomain.Artifact{}, fault.NotFound("Artifact")
}
func (artifactStorageTestRepository) CreateDeliveryPackage(context.Context, deliverydomain.DeliveryPackage, []deliverydomain.Artifact) error {
	return nil
}

func (artifactStorageTestRepository) DeliveryPackageBySnapshotAndContentItem(context.Context, string, string, string) (deliverydomain.DeliveryPackage, error) {
	return deliverydomain.DeliveryPackage{}, fault.NotFound("交付包")
}
func (artifactStorageTestRepository) DeliveryPackages(context.Context, string, string) ([]deliverydomain.DeliveryPackage, error) {
	return nil, nil
}
func (artifactStorageTestRepository) DeliveryPackage(context.Context, string, string) (deliverydomain.DeliveryPackage, error) {
	return deliverydomain.DeliveryPackage{}, fault.NotFound("交付包")
}

type artifactStorageTestBlob struct {
	*blob.MemoryStore
	deleteErr error
}

func (s *artifactStorageTestBlob) Delete(ctx context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.MemoryStore.Delete(ctx, key)
}

type artifactStoragePutAfterWriteBlob struct {
	*artifactStorageTestBlob
}

func (s *artifactStoragePutAfterWriteBlob) Put(ctx context.Context, key string, body []byte) error {
	if err := s.MemoryStore.Put(ctx, key, body); err != nil {
		return err
	}
	return errors.New("write acknowledged after persistence failure")
}

func TestPersistArtifactObjectCleansObjectWhenFactWriteFails(t *testing.T) {
	ctx := context.Background()
	store := &artifactStorageTestBlob{MemoryStore: blob.NewMemory()}
	scope := &serviceScope{serviceCore: &serviceCore{artifacts: artifactStorageTestRepository{err: errors.New("fact rejected")}, blobs: store}}
	artifact := deliverydomain.Artifact{ID: "artifact-1", ObjectKey: "objects/artifact-1"}

	err := scope.persistArtifactObject(ctx, artifact, []byte("payload"))
	if err == nil || err.Error() != "fact rejected" {
		t.Fatalf("expected original fact error, got %v", err)
	}
	if _, getErr := store.Get(ctx, artifact.ObjectKey); !errors.Is(getErr, blob.ErrNotFound) {
		t.Fatalf("failed fact left an unattached object: %v", getErr)
	}
}

func TestPersistArtifactObjectCleansObjectWhenBlobPutFailsAfterWrite(t *testing.T) {
	ctx := context.Background()
	store := &artifactStoragePutAfterWriteBlob{artifactStorageTestBlob: &artifactStorageTestBlob{MemoryStore: blob.NewMemory()}}
	scope := &serviceScope{serviceCore: &serviceCore{artifacts: artifactStorageTestRepository{err: errors.New("fact must not be reached")}, blobs: store}}
	artifact := deliverydomain.Artifact{ID: "artifact-after-write", ObjectKey: "objects/artifact-after-write"}

	err := scope.persistArtifactObject(ctx, artifact, []byte("payload"))
	if err == nil || err.Error() != "write acknowledged after persistence failure" {
		t.Fatalf("expected Blob write error, got %v", err)
	}
	if _, getErr := store.Get(ctx, artifact.ObjectKey); !errors.Is(getErr, blob.ErrNotFound) {
		t.Fatalf("write-after-error left an unattached object: %v", getErr)
	}
}

func TestCleanupSpeculativeObjectsCleansEveryWrittenObject(t *testing.T) {
	ctx := context.Background()
	store := &artifactStorageTestBlob{MemoryStore: blob.NewMemory()}
	for _, key := range []string{"objects/one", "objects/two"} {
		if err := store.Put(ctx, key, []byte(key)); err != nil {
			t.Fatal(err)
		}
	}
	cause := errors.New("delivery transaction rejected")
	if err := cleanupSpeculativeObjects(ctx, store, []string{"objects/one", "objects/two"}, cause); err != cause {
		t.Fatalf("cleanup should preserve original error, got %v", err)
	}
	for _, key := range []string{"objects/one", "objects/two"} {
		if _, err := store.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("object %s survived cleanup: %v", key, err)
		}
	}
}

func TestCleanupSpeculativeObjectsReturnsRecoveryDetailsOnCleanupFailure(t *testing.T) {
	ctx := context.Background()
	store := &artifactStorageTestBlob{MemoryStore: blob.NewMemory(), deleteErr: errors.New("delete unavailable")}
	if err := store.Put(ctx, "objects/orphan", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	err := cleanupSpeculativeObjects(ctx, store, []string{"objects/orphan"}, errors.New("fact rejected"))
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != "ARTIFACT_BLOB_CLEANUP_FAILED" {
		t.Fatalf("cleanup failure was not structured for recovery: %v", err)
	}
	if _, ok := domainErr.Details.(map[string]any)["cleanup_errors"]; !ok {
		t.Fatalf("cleanup details missing object errors: %#v", domainErr.Details)
	}
}
