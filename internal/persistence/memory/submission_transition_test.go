package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/limecloud/contentcloud/internal/persistence/memory"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func TestSubmissionDecisionRejectsStaleStateTransition(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	tenantID, projectID, workspaceID := idgen.New(), idgen.New(), idgen.New()
	submission := reviewdomain.Submission{
		ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID,
		SubmissionType: "knowledge", Status: "submitted", CurrentRevisionID: idgen.New(), CreatedAt: now, UpdatedAt: now,
	}
	revision := reviewdomain.SubmissionRevision{
		ID: submission.CurrentRevisionID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID,
		SubmissionID: submission.ID, RevisionNo: 1, IdempotencyKey: "stale-transition", CreatedAt: now,
	}
	cycle := reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", CreatedAt: now}
	if err := store.CreateSubmissionRevision(t.Context(), submission, revision, nil, cycle); err != nil {
		t.Fatal(err)
	}

	approved := submission
	approved.Status = "approved"
	approved.UpdatedAt = now.Add(time.Minute)
	approveDecision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, PreviousState: "submitted", ResultingState: "approved", Decision: "approve", CreatedAt: approved.UpdatedAt}
	snapshot := reviewdomain.ApprovedSnapshot{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, DecisionID: approveDecision.ID, CreatedAt: approved.UpdatedAt}
	if err := store.ApproveSubmissionRevision(t.Context(), approved, snapshot, approveDecision); err != nil {
		t.Fatal(err)
	}

	stale := submission
	stale.Status = "changes_requested"
	stale.UpdatedAt = now.Add(2 * time.Minute)
	staleDecision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, PreviousState: "submitted", ResultingState: "changes_requested", Decision: "request_changes", CreatedAt: stale.UpdatedAt}
	comment := reviewdomain.ReviewComment{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, Body: "stale", Visibility: "internal", CreatedAt: stale.UpdatedAt}
	err := store.RequestSubmissionChanges(t.Context(), stale, staleDecision, comment)
	var domainError *fault.Error
	if !errors.As(err, &domainError) || domainError.Code != "SUBMISSION_STATE_INVALID" {
		t.Fatalf("stale transition error = %v", err)
	}
	persisted, err := store.Submission(t.Context(), tenantID, submission.ID)
	if err != nil || persisted.Status != "approved" {
		t.Fatalf("stale transition changed submission: %#v err=%v", persisted, err)
	}
	comments, err := store.ReviewComments(t.Context(), tenantID, revision.ID)
	if err != nil || len(comments) != 0 {
		t.Fatalf("stale transition persisted comment: %#v err=%v", comments, err)
	}
}
