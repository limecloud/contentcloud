package memory_test

import (
	"errors"
	"testing"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestCreateArtifactRequiresExistingApprovedSnapshot(t *testing.T) {
	store := memory.New()
	err := store.CreateArtifact(t.Context(), deliverydomain.Artifact{ID: idgen.New(), TenantID: idgen.New(), ProjectID: idgen.New()})
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != "ARTIFACT_SNAPSHOT_REQUIRED" {
		t.Fatalf("missing snapshot error = %v", err)
	}

	err = store.CreateArtifact(t.Context(), deliverydomain.Artifact{ID: idgen.New(), TenantID: idgen.New(), ProjectID: idgen.New(), ApprovedSnapshotID: idgen.New()})
	if !errors.As(err, &domainError) || domainError.Code != "RESOURCE_NOT_FOUND" {
		t.Fatalf("unknown snapshot error = %v", err)
	}
}
