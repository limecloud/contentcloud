package application_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
)

// TestMarketingCampaignBenchmarkClosesSharedFactChain verifies the customer
// side of the campaign benchmark. Ten channel variants remain ten immutable
// SubmissionRevision/ApprovedSnapshot facts; delivery and learning consume
// those snapshots instead of inventing a campaign-specific data model.
func TestMarketingCampaignBenchmarkClosesSharedFactChain(t *testing.T) {
	ctx, service, store, actor, binding := v3ContentFixture(t)

	snapshots := make([]string, 0, 10)
	deliveries := make([]string, 0, 10)
	for index := 1; index <= 10; index++ {
		itemID := fmt.Sprintf("campaign-content-%02d", index)
		revision := publishV3ContentItem(t, ctx, service, binding, itemID, "campaign-publish-"+itemID)
		if _, err := service.Review.ApproveSubmission(ctx, actor, revision.ID, "活动版本内部审核通过", "campaign-internal-"+itemID); err != nil {
			t.Fatal(err)
		}
		grant, err := service.Review.CreateReviewGrant(ctx, actor, revision.ID, fmt.Sprintf("campaign-client-%02d@example.com", index), "campaign-grant-"+itemID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Review.VerifyReviewGrant(ctx, grant.PlaintextToken, grant.PlaintextOTP); err != nil {
			t.Fatal(err)
		}
		decision, err := service.Review.DecideReviewGrant(ctx, grant.PlaintextToken, "approve", "活动渠道版本确认通过", "", "campaign-client-"+itemID)
		if err != nil || decision.ApprovedSnapshot == nil {
			t.Fatalf("campaign variant %s was not client approved: %#v err=%v", itemID, decision, err)
		}
		snapshot := *decision.ApprovedSnapshot
		delivery, err := service.Review.CreateDeliveryPackage(ctx, actor, snapshot.ID, itemID, "campaign-delivery-"+itemID)
		if err != nil {
			t.Fatal(err)
		}
		if delivery.Status != "ready" || len(delivery.Manifest) != 3 || len(delivery.ApprovedSnapshotIDs) != 1 || delivery.ApprovedSnapshotIDs[0] != snapshot.ID {
			t.Fatalf("campaign delivery lost snapshot lineage: %#v", delivery)
		}
		for _, artifact := range delivery.Manifest {
			if artifact.ApprovedSnapshotID != snapshot.ID {
				t.Fatalf("campaign artifact %s is not derived from its approved snapshot: %#v", artifact.ID, artifact)
			}
		}
		snapshots = append(snapshots, snapshot.ID)
		deliveries = append(deliveries, delivery.ID)
	}

	now := time.Now().UTC().Add(-24 * time.Hour)
	observations := make([]application.CreateObservationInput, 0, len(snapshots))
	for index, snapshotID := range snapshots {
		observations = append(observations, application.CreateObservationInput{
			RowNumber: index + 1, ApprovedSnapshotID: snapshotID, Platform: "douyin", AccountAlias: fmt.Sprintf("campaign-channel-%02d", index+1),
			PublishedAt: now, WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": float64(1000 + index*100), "clicks": float64(40 + index)},
			Currency: "CNY", Spend: 100, GMV: float64(250 + index*10), IssueCategory: "creative",
		})
	}
	imported, err := service.Performance.ImportPerformanceObservations(ctx, actor, application.ImportPerformanceInput{ProjectID: binding.ProjectID, SourceName: "campaign-results.csv", SourceFormat: "csv", Observations: observations}, "campaign-results")
	if err != nil || len(imported.Observations) != 10 {
		t.Fatalf("campaign result batch was not imported atomically: count=%d err=%v", len(imported.Observations), err)
	}
	for index, observation := range imported.Observations {
		if observation.ApprovedSnapshotID != snapshots[index] {
			t.Fatalf("observation %s lost campaign snapshot lineage", observation.ID)
		}
		if _, err := service.Performance.CreateRatingDecision(ctx, actor, application.CreateRatingDecisionInput{
			ProjectID: binding.ProjectID, SubjectType: "approved_snapshot", SubjectID: snapshots[index], ObservationIDs: []string{observation.ID},
			Rating: "seed_candidate", Reason: "活动首轮结果已完成受控观察", NextAction: "只改变一个渠道变量并进入下一轮",
		}, fmt.Sprintf("campaign-rating-%02d", index+1)); err != nil {
			t.Fatal(err)
		}
	}

	storedSnapshots, err := store.ApprovedSnapshots(ctx, actor.TenantID, binding.ProjectID, "content_batch")
	if err != nil {
		t.Fatal(err)
	}
	storedDeliveries, err := store.DeliveryPackages(ctx, actor.TenantID, binding.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	storedObservations, err := store.PerformanceObservations(ctx, actor.TenantID, binding.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	storedRatings, err := store.RatingDecisions(ctx, actor.TenantID, binding.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedSnapshots) != 10 || len(storedDeliveries) != 10 || len(storedObservations) != 10 || len(storedRatings) != 10 {
		t.Fatalf("campaign benchmark did not close the shared fact chain: snapshots=%d deliveries=%d observations=%d ratings=%d", len(storedSnapshots), len(storedDeliveries), len(storedObservations), len(storedRatings))
	}

	// The application composition remains the only owner of business facts;
	// no campaign-specific repository or Runtime write API was introduced.
}
