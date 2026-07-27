package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestSubmissionDecisionRejectsStaleStateTransition(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	tenantID, projectID, workspaceID := domain.NewID(), domain.NewID(), domain.NewID()
	submission := domain.Submission{
		ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID,
		SubmissionType: "knowledge", Status: "submitted", CurrentRevisionID: domain.NewID(), CreatedAt: now, UpdatedAt: now,
	}
	revision := domain.SubmissionRevision{
		ID: submission.CurrentRevisionID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID,
		SubmissionID: submission.ID, RevisionNo: 1, IdempotencyKey: "stale-transition", CreatedAt: now,
	}
	cycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", CreatedAt: now}
	if err := store.CreateSubmissionRevision(t.Context(), submission, revision, nil, cycle); err != nil {
		t.Fatal(err)
	}

	approved := submission
	approved.Status = "approved"
	approved.UpdatedAt = now.Add(time.Minute)
	approveDecision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, PreviousState: "submitted", ResultingState: "approved", Decision: "approve", CreatedAt: approved.UpdatedAt}
	snapshot := domain.ApprovedSnapshot{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, DecisionID: approveDecision.ID, CreatedAt: approved.UpdatedAt}
	if err := store.ApproveSubmissionRevision(t.Context(), approved, snapshot, approveDecision); err != nil {
		t.Fatal(err)
	}

	stale := submission
	stale.Status = "changes_requested"
	stale.UpdatedAt = now.Add(2 * time.Minute)
	staleDecision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, PreviousState: "submitted", ResultingState: "changes_requested", Decision: "request_changes", CreatedAt: stale.UpdatedAt}
	comment := domain.ReviewComment{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revision.ID, Body: "stale", Visibility: "internal", CreatedAt: stale.UpdatedAt}
	err := store.RequestSubmissionChanges(t.Context(), stale, staleDecision, comment)
	var domainError *domain.Error
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
