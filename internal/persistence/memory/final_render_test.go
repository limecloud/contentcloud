package memory_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func TestCreateFinalRenderIsAtomicInMemory(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	tenantID, projectID, workspaceID := idgen.New(), idgen.New(), idgen.New()
	submission := reviewdomain.Submission{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionType: "storyboard", Status: "submitted", CurrentRevisionID: idgen.New(), CreatedAt: now, UpdatedAt: now}
	revision := reviewdomain.SubmissionRevision{ID: submission.CurrentRevisionID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionID: submission.ID, RevisionNo: 1, IdempotencyKey: "final-render-atomic", CreatedAt: now}
	cycle := reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", CreatedAt: now}
	if err := store.CreateSubmissionRevision(t.Context(), submission, revision, nil, cycle); err != nil {
		t.Fatal(err)
	}
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, PreviousState: "submitted", ResultingState: "approved", Decision: "approve", CreatedAt: now}
	snapshot := reviewdomain.ApprovedSnapshot{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, DecisionID: decision.ID, CreatedAt: now}
	approved := submission
	approved.Status = "approved"
	approved.UpdatedAt = now
	if err := store.ApproveSubmissionRevision(t.Context(), approved, snapshot, decision); err != nil {
		t.Fatal(err)
	}

	artifact := deliverydomain.Artifact{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, ApprovedSnapshotID: snapshot.ID, Kind: "final_render", SHA256: "sha256:" + strings.Repeat("a", 64), ByteSize: 4, CreatedAt: now}
	invalidReview := deliverydomain.MediaReview{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, TaskID: idgen.New(), SubjectArtifactID: artifact.ID, SubjectDigest: "sha256:" + strings.Repeat("b", 64), ReviewKind: deliverydomain.MediaReviewFinal, Status: deliverydomain.MediaReviewPending, RowVersion: 1, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateFinalRender(t.Context(), artifact, invalidReview); err == nil {
		t.Fatal("expected mismatched final review to be rejected")
	}
	var domainError *fault.Error
	if _, err := store.Artifact(t.Context(), tenantID, artifact.ID); !errors.As(err, &domainError) {
		t.Fatalf("artifact was persisted after atomic failure: %v", err)
	}

	validReview := invalidReview
	validReview.ID = idgen.New()
	validReview.SubjectDigest = artifact.SHA256
	if err := store.CreateFinalRender(t.Context(), artifact, validReview); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Artifact(t.Context(), tenantID, artifact.ID); err != nil {
		t.Fatalf("artifact missing after atomic success: %v", err)
	}
	if _, err := store.MediaReview(t.Context(), tenantID, validReview.ID); err != nil {
		t.Fatalf("final review missing after atomic success: %v", err)
	}
}
