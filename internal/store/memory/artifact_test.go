package memory_test

import (
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestCreateArtifactRequiresExistingApprovedSnapshot(t *testing.T) {
	store := memory.New()
	err := store.CreateArtifact(t.Context(), domain.Artifact{ID: domain.NewID(), TenantID: domain.NewID(), ProjectID: domain.NewID()})
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "ARTIFACT_SNAPSHOT_REQUIRED" {
		t.Fatalf("missing snapshot error = %v", err)
	}

	err = store.CreateArtifact(t.Context(), domain.Artifact{ID: domain.NewID(), TenantID: domain.NewID(), ProjectID: domain.NewID(), ApprovedSnapshotID: domain.NewID()})
	if !errors.As(err, &domainError) || domainError.Code != "RESOURCE_NOT_FOUND" {
		t.Fatalf("unknown snapshot error = %v", err)
	}
}
