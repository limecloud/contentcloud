package application

import (
	"testing"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestStudioArtifactAssetStatusRequiresReviewBeforeReuse(t *testing.T) {
	artifact := deliverydomain.Artifact{ID: "artifact-1"}
	tests := []struct {
		name   string
		view   WorkTaskView
		final  bool
		status string
	}{
		{name: "unreviewed result", view: WorkTaskView{}, status: "pending_confirmation"},
		{name: "approved result", view: WorkTaskView{MediaReviews: []deliverydomain.MediaReview{{SubjectArtifactID: "artifact-1", Status: deliverydomain.MediaReviewApproved}}}, status: "confirmed"},
		{name: "changes requested takes precedence", view: WorkTaskView{MediaReviews: []deliverydomain.MediaReview{{SubjectArtifactID: "artifact-1", Status: deliverydomain.MediaReviewApproved}, {SubjectArtifactID: "artifact-1", Status: deliverydomain.MediaReviewChanges}}}, status: "changes_requested"},
		{name: "delivered final result", view: WorkTaskView{Task: work.WorkTask{Status: work.TaskStatusDelivered}}, final: true, status: "delivered"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := studioArtifactAssetStatus(test.view, artifact, test.final); got != test.status {
				t.Fatalf("studioArtifactAssetStatus() = %q, want %q", got, test.status)
			}
		})
	}
}
