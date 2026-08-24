package application_test

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestCommerceWorkbenchFactsCloseThroughSharedReviewDeliveryAndLearning(t *testing.T) {
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default(), application.WithPlatformAdminEmails("commerce-admin@example.com"))
	session, err := service.Identity.Register(t.Context(), "commerce-admin@example.com", "long-enough-password", "Commerce Admin", "Commerce Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(t.Context(), session.ID)
	must(t, err)
	_, err = service.Identity.UpdatePlatformTenantContentCapability(t.Context(), actor, actor.TenantID, identitydomain.ContentTypeCommerce, true, "enable-commerce")
	must(t, err)
	project, err := service.Workspace.CreateProject(t.Context(), actor, application.CreateProjectInput{BrandName: "Commerce Brand", ProductName: "Low Sugar Snack", ContentType: identitydomain.ContentTypeCommerce, Channel: "douyin"}, "commerce-project")
	must(t, err)
	task, err := service.Work.CreateWorkTask(t.Context(), actor, application.CreateWorkTaskInput{ProjectID: project.ID, Title: "Commerce governed task", Intent: "Produce channel variants from verified product facts", ContentType: identitydomain.ContentTypeCommerce, InputRefs: []string{"brief:commerce"}}, "commerce-task")
	must(t, err)

	now := time.Now().UTC()
	binding := workspacedomain.WorkspaceBinding{
		ID: task.Task.ID, TenantID: actor.TenantID, ProjectID: project.ID, OwnerUserID: actor.UserID,
		CredentialHash: idgen.TokenHash("commerce-workspace-token"), Status: "active", InitializedAt: now, LastSeenAt: now,
	}
	must(t, store.CreateWorkspaceBinding(t.Context(), binding))
	workspaceActor := application.Actor{TenantID: actor.TenantID, WorkspaceID: binding.ID, Type: "workspace", Role: "workspace"}
	binding, err = service.Review.RegisterWorkspace(t.Context(), workspaceActor, binding, localworkspace.TemplateID, localworkspace.TemplateVersion, []string{"codex"}, "commerce-workspace")
	must(t, err)

	item := localworkspace.CommerceContentItem{
		ID: "commerce-item:1", Type: "commerce_content_item", Status: "candidate", SchemaVersion: localworkspace.CommerceContentSchema, Deliverability: "review_ready",
		ProjectID: project.ID, ContentID: "commerce-content:1", ContentBatchID: "commerce-batch:1", Title: "Low sugar snack channel package", Channel: "douyin", TargetAudience: "糖分敏感的上班族",
		ProductFacts: map[string]string{"net_weight": "500g", "origin": "China"}, OfferPoints: []string{"low sugar", "individual packaging"},
		Variants:       []localworkspace.CommerceVariant{{ID: "variant:short-video", Format: "short_video_caption", Copy: "A restrained introduction based on verified product facts.", CTA: "View product details"}},
		BlockedReasons: []string{}, MissingInputs: []string{},
	}
	content, err := json.Marshal(item)
	must(t, err)
	revision, err := service.Work.CreateTaskRevision(t.Context(), actor, task.Task.ID, application.CreateTaskRevisionInput{
		ContentType: identitydomain.ContentTypeCommerce, SchemaVersion: localworkspace.CommerceContentSchema,
		Content: content,
	}, "commerce-submission")
	must(t, err)
	if revision.ID == "" || revision.ContentType != identitydomain.ContentTypeCommerce {
		t.Fatalf("compatibility content entrypoint did not return the governed revision: %#v", revision)
	}
	internalApproval, err := service.Review.ApproveSubmission(t.Context(), actor, revision.ID, "Internal commerce review passed", "commerce-internal-approve")
	must(t, err)
	if internalApproval.ApprovedSnapshot != nil || internalApproval.Submission.Status != "internally_approved" {
		t.Fatalf("commerce internal approval bypassed client review: %#v", internalApproval)
	}
	grant, err := service.Review.CreateReviewGrant(t.Context(), actor, revision.ID, "commerce-client@example.com", "commerce-client-grant")
	must(t, err)
	_, err = service.Review.VerifyReviewGrant(t.Context(), grant.PlaintextToken, grant.PlaintextOTP)
	must(t, err)
	clientDecision, err := service.Review.DecideReviewGrant(t.Context(), grant.PlaintextToken, "approve", "Commerce client approved", "", "commerce-client-approve")
	must(t, err)
	if clientDecision.ApprovedSnapshot == nil {
		t.Fatal("commerce client approval did not create an ApprovedSnapshot")
	}
	snapshot := *clientDecision.ApprovedSnapshot
	delivery, err := service.Review.CreateDeliveryPackage(t.Context(), actor, snapshot.ID, item.ID, "commerce-delivery")
	must(t, err)
	if len(delivery.Manifest) != 3 || delivery.ContentItemID != item.ID {
		t.Fatalf("commerce delivery package is incomplete: %#v", delivery)
	}

	performance, err := service.Performance.ImportPerformanceObservations(t.Context(), actor, application.ImportPerformanceInput{
		ProjectID: project.ID, SourceName: "commerce-manual", SourceFormat: "manual",
		Observations: []application.CreateObservationInput{{RowNumber: 1, ApprovedSnapshotID: snapshot.ID, Platform: "douyin", AccountAlias: "brand-main", PublishedAt: now.Add(-24 * time.Hour), WindowHours: 24, SampleStatus: "seed_candidate", Metrics: map[string]float64{"impressions": 1000, "clicks": 40}, Currency: "CNY", Spend: 100, GMV: 260, IssueCategory: "creative"}},
	}, "commerce-performance")
	must(t, err)
	if len(performance.Observations) != 1 {
		t.Fatalf("commerce performance import failed: %#v", performance)
	}
	rating, err := service.Performance.CreateRatingDecision(t.Context(), actor, application.CreateRatingDecisionInput{ProjectID: project.ID, SubjectType: "approved_snapshot", SubjectID: snapshot.ID, ObservationIDs: []string{performance.Observations[0].ID}, Rating: "seed_candidate", Reason: "Click signal is sufficient for the next small test", NextAction: "Keep facts fixed and test another opening"}, "commerce-rating")
	must(t, err)
	if rating.Decision.SubjectID != snapshot.ID {
		t.Fatalf("commerce rating lost snapshot lineage: %#v", rating)
	}

	view, err := service.Work.WorkTask(t.Context(), actor, task.Task.ID)
	must(t, err)
	if len(view.Revisions) != 1 || view.Revisions[0].ID != revision.ID || len(view.ApprovedSnapshots) != 1 || len(view.Artifacts) != 3 || len(view.DeliveryPackages) != 1 {
		t.Fatalf("task projection did not rebuild the shared commerce fact chain: revisions=%d snapshots=%d artifacts=%d packages=%d", len(view.Revisions), len(view.ApprovedSnapshots), len(view.Artifacts), len(view.DeliveryPackages))
	}
	studio, err := service.Work.CustomerStudioTask(t.Context(), actor, task.Task.ID)
	must(t, err)
	if studio.Pipeline.ApprovedVersionCount != 1 || studio.Pipeline.ArtifactCount != 3 || studio.Pipeline.DeliveryPackageCount != 1 || studio.Pipeline.PerformanceObservationCount != 1 || studio.Pipeline.LearningDecisionCount != 1 {
		t.Fatalf("commerce workbench did not project the shared closure: %#v", studio.Pipeline)
	}
}
