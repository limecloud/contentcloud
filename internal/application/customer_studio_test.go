package application

import (
	"testing"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	performancedomain "github.com/limecloud/contentcloud/internal/performance"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
)

func TestNormalizeStudioBusinessBriefUsesStrictBusinessContracts(t *testing.T) {
	article, err := normalizeStudioBusinessBrief(StudioBusinessBrief{Audience: "新用户", Channel: "公众号", Tone: "可信", Keywords: []string{"成分", "成分"}}, "wechat_article")
	if err != nil {
		t.Fatalf("article brief should validate: %v", err)
	}
	if article.SchemaVersion == "" || len(article.Keywords) != 1 {
		t.Fatalf("article brief was not normalized: %#v", article)
	}
	if _, err := normalizeStudioBusinessBrief(StudioBusinessBrief{Audience: "新用户"}, "wechat_article"); err == nil {
		t.Fatal("incomplete article brief was accepted")
	}
	commerce, err := normalizeStudioBusinessBrief(StudioBusinessBrief{Channel: "短视频", TargetAudience: "上班族", ProductFacts: map[string]string{"净含量": "500g"}, OfferPoints: []string{"低糖"}}, "commerce")
	if err != nil {
		t.Fatalf("commerce brief should validate: %v", err)
	}
	if commerce.ProductFacts["净含量"] != "500g" || commerce.SchemaVersion == "" {
		t.Fatalf("commerce brief was not normalized: %#v", commerce)
	}
	video, err := normalizeStudioBusinessBrief(StudioBusinessBrief{Audience: "首次了解品牌的人", Channel: "抖音", Tone: "真实克制", Keywords: []string{"主理人", "产品"}}, "marketing_video")
	if err != nil {
		t.Fatalf("video brief should validate: %v", err)
	}
	if video.SchemaVersion == "" || len(video.Keywords) != 2 {
		t.Fatalf("video brief was not normalized: %#v", video)
	}
}

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

func TestCustomerStudioPipelineSummaryUsesSharedFactLinks(t *testing.T) {
	view := WorkTaskView{
		SOP:               catalogdomain.SOPVersion{Stages: []catalogdomain.StageDefinition{{ID: "brief"}, {ID: "delivery"}, {ID: "learning"}}},
		StageRuns:         []work.StageRun{{Status: work.StageRunStatusCompleted}, {Status: work.StageRunStatusRunning}},
		Runs:              []work.RuntimeRun{{ID: "run-1"}},
		Gates:             []reviewdomain.GateEvaluation{{ID: "gate-1", Status: reviewdomain.GateEvaluationPending}},
		ApprovedSnapshots: []reviewdomain.ApprovedSnapshot{{ID: "snapshot-1"}},
		Artifacts:         []deliverydomain.Artifact{{ID: "artifact-1"}, {ID: "artifact-2"}},
		DeliveryPackages:  []deliverydomain.DeliveryPackage{{ID: "delivery-1"}},
	}
	observations := []performancedomain.PerformanceObservation{
		{ID: "observation-1", ApprovedSnapshotID: "snapshot-1"},
		{ID: "observation-foreign", ApprovedSnapshotID: "snapshot-foreign"},
	}
	ratings := []performancedomain.RatingDecision{
		{ID: "rating-1", SubjectType: "approved_snapshot", SubjectID: "snapshot-1"},
		{ID: "rating-foreign", SubjectType: "approved_snapshot", SubjectID: "snapshot-foreign"},
		{ID: "rating-2", SubjectType: "approved_snapshot", SubjectID: "unrelated", ObservationIDs: []string{"observation-1"}},
	}

	steps := []StudioCustomerStep{{ID: "brief", Status: "completed"}, {ID: "delivery", Status: "working"}}
	got := customerStudioPipelineSummary(view, steps, observations, ratings)
	if got.StageCount != 2 || got.CompletedStageCount != 1 || got.ExecutionCount != 1 || got.PendingDecisionCount != 1 {
		t.Fatalf("unexpected orchestration projection: %#v", got)
	}
	if got.ApprovedVersionCount != 1 || got.ArtifactCount != 2 || got.DeliveryPackageCount != 1 {
		t.Fatalf("unexpected approved/delivery projection: %#v", got)
	}
	if got.PerformanceObservationCount != 1 || got.LearningDecisionCount != 2 {
		t.Fatalf("unexpected learning projection: %#v", got)
	}
}
