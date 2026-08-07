package app

import (
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestStudioArtifactAssetStatusRequiresReviewBeforeReuse(t *testing.T) {
	artifact := domain.Artifact{ID: "artifact-1"}
	tests := []struct {
		name   string
		view   WorkTaskView
		final  bool
		status string
	}{
		{name: "unreviewed result", view: WorkTaskView{}, status: "pending_confirmation"},
		{name: "approved result", view: WorkTaskView{MediaReviews: []domain.MediaReview{{SubjectArtifactID: "artifact-1", Status: domain.MediaReviewApproved}}}, status: "confirmed"},
		{name: "changes requested takes precedence", view: WorkTaskView{MediaReviews: []domain.MediaReview{{SubjectArtifactID: "artifact-1", Status: domain.MediaReviewApproved}, {SubjectArtifactID: "artifact-1", Status: domain.MediaReviewChanges}}}, status: "changes_requested"},
		{name: "delivered final result", view: WorkTaskView{Task: domain.WorkTask{Status: domain.TaskStatusDelivered}}, final: true, status: "delivered"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := studioArtifactAssetStatus(test.view, artifact, test.final); got != test.status {
				t.Fatalf("studioArtifactAssetStatus() = %q, want %q", got, test.status)
			}
		})
	}
}
