package application_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/application"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
)

func TestAssetRightsEligibility(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, err := service.Identity.Register(ctx, "rights@example.com", "long-enough-password", "Rights Reviewer", "Rights Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "")
	must(t, err)
	revision, err := service.Source.UploadSource(ctx, actor, project.ID, "Owned product image", "asset", "product.jpg", "image/jpeg", []byte("jpeg fixture"), "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	_, err = service.Source.CompleteSource(ctx, worker, revision.ID, application.CompleteSourceInput{DetectedMIME: "image/jpeg", Status: "ready", ParserVersion: "test/v1"}, "")
	must(t, err)

	analysisOnly, err := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "Competitor image", AssetType: "product_image", SourceRevisionID: revision.ID, UsageMode: "analysis_only"}, "")
	must(t, err)
	analysisRights, err := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: analysisOnly.ID, RightsHolder: "Brand", RightsType: "owned", Territories: []string{"CN"}, Channels: []string{"douyin"}, ProofSourceRevisionID: revision.ID}, "")
	must(t, err)
	_, err = service.Review.ReviewRightsRecord(ctx, actor, analysisRights.ID, "approve", "")
	must(t, err)

	generationAsset, err := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "Product hero", AssetType: "product_image", SourceRevisionID: revision.ID, UsageMode: "generation_reference"}, "")
	must(t, err)
	validUntil := time.Now().UTC().Add(24 * time.Hour)
	generationRights, err := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: generationAsset.ID, RightsHolder: "Brand", RightsType: "licensed_generation", Territories: []string{"CN"}, Channels: []string{"douyin"}, ValidUntil: &validUntil, ProofSourceRevisionID: revision.ID}, "")
	must(t, err)
	_, err = service.Review.ReviewRightsRecord(ctx, actor, generationRights.ID, "approve", "")
	must(t, err)

	wrongChannel, err := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "Print only", AssetType: "brand_mark", SourceRevisionID: revision.ID, UsageMode: "owned"}, "")
	must(t, err)
	wrongChannelRights, err := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: wrongChannel.ID, RightsHolder: "Brand", RightsType: "owned", Territories: []string{"CN"}, Channels: []string{"print"}, ProofSourceRevisionID: revision.ID}, "")
	must(t, err)
	_, err = service.Review.ReviewRightsRecord(ctx, actor, wrongChannelRights.ID, "approve", "")
	must(t, err)

	expiredAsset, err := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "Expired license", AssetType: "person", SourceRevisionID: revision.ID, UsageMode: "generation_reference"}, "")
	must(t, err)
	validFrom := time.Now().UTC().Add(-48 * time.Hour)
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	expiredRights, err := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: expiredAsset.ID, RightsHolder: "Talent", RightsType: "licensed_generation", Territories: []string{"CN"}, Channels: []string{"douyin"}, ValidFrom: &validFrom, ValidUntil: &expiredAt, ProofSourceRevisionID: revision.ID}, "")
	must(t, err)
	expiredRights, err = service.Review.ReviewRightsRecord(ctx, actor, expiredRights.ID, "approve", "")
	must(t, err)
	if expiredRights.Status != "expired" {
		t.Fatalf("expired rights must not be approved, got %s", expiredRights.Status)
	}

	eligible, err := service.Review.EligibleAssets(ctx, actor, project.ID, "douyin")
	must(t, err)
	if len(eligible) != 1 || eligible[0].Asset.ID != generationAsset.ID || eligible[0].Rights.ID != generationRights.ID {
		t.Fatalf("only rights-valid generation assets may enter a task contract: %#v", eligible)
	}
}

func TestRightsRejectDoesNotOverrideAnotherApprovedRecord(t *testing.T) {
	ctx := context.Background()
	service := application.New(application.DependenciesFrom(memory.New()), slog.Default())
	session, _ := service.Identity.Register(ctx, "multi-rights@example.com", "long-enough-password", "Reviewer", "Multi Rights")
	actor, _, _ := service.Identity.SessionActor(ctx, session.ID)
	project, _ := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "")
	revision, _ := service.Source.UploadSource(ctx, actor, project.ID, "Proof", "rights", "proof.txt", "text/plain", []byte("proof"), "")
	worker := actor
	worker.Type = "worker"
	_, _ = service.Source.CompleteSource(ctx, worker, revision.ID, application.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready"}, "")
	asset, _ := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "Hero", AssetType: "product_image", SourceRevisionID: revision.ID, UsageMode: "owned"}, "")
	first, _ := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: asset.ID, RightsHolder: "Brand", RightsType: "owned", Territories: []string{"CN"}, Channels: []string{"douyin"}, ProofSourceRevisionID: revision.ID}, "")
	second, _ := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: asset.ID, RightsHolder: "Agency", RightsType: "licensed_edit", Territories: []string{"CN"}, Channels: []string{"douyin"}, ProofSourceRevisionID: revision.ID}, "")
	_, _ = service.Review.ReviewRightsRecord(ctx, actor, first.ID, "approve", "")
	_, _ = service.Review.ReviewRightsRecord(ctx, actor, second.ID, "reject", "")
	asset, err := service.Review.Asset(ctx, actor, asset.ID)
	must(t, err)
	if asset.Status != "approved" {
		t.Fatalf("one rejected record must not override another approved grant, got %s", asset.Status)
	}
}
