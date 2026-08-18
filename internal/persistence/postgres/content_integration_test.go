package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	storepg "github.com/limecloud/contentcloud/internal/persistence/postgres"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/limecloud/contentcloud/internal/application"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestSourceLifecycleWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("CONTENTCLOUD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("CONTENTCLOUD_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	store, err := storepg.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service := application.New(application.DependenciesFrom(store), slog.Default())
	suffix := idgen.New()
	session, err := service.Identity.Register(ctx, fmt.Sprintf("source-%s@example.com", suffix), "long-enough-password", "Source Reviewer", "Source Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Source Brand", ProductName: "Source Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	quote := "PostgreSQL source evidence"
	revision, err := service.Source.UploadSource(ctx, actor, project.ID, "Product facts", "product_spec", "facts.txt", "text/plain", []byte(quote), "")
	if err != nil {
		t.Fatal(err)
	}
	worker := actor
	worker.Type = "worker"
	revision, err = service.Source.CompleteSource(ctx, worker, revision.ID, application.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test/v1", Evidence: []application.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: quote}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if revision.ProcessingStatus != "ready" {
		t.Fatalf("expected ready revision, got %s", revision.ProcessingStatus)
	}
	sources, err := service.Source.Sources(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Status != "ready" || sources[0].LatestRevision != revision.ID {
		t.Fatalf("unexpected source projection: %#v", sources)
	}
	spans, err := service.Source.Evidence(ctx, actor, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].ReviewStatus != "accepted" || spans[0].QuoteText != quote {
		t.Fatalf("unexpected evidence projection: %#v", spans)
	}
	extractionRun, err := service.Source.CreateKnowledgeExtractionRun(ctx, actor, application.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{revision.ID}, IdempotencyKey: "postgres-extract-" + suffix, OutputCount: 7}, "")
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := service.Runtime.Run(ctx, actor, extractionRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedRun.CapabilityID != sourcedomain.KnowledgeExtractCapability || persistedRun.OutputSchema != sourcedomain.KnowledgeCandidatesSchema || persistedRun.OutputCount != 7 {
		t.Fatalf("knowledge extraction run contract fields were not persisted: %#v", persistedRun)
	}
	object, err := service.Source.CreateKnowledgeObject(ctx, actor, application.CreateKnowledgeObjectInput{ProjectID: project.ID, ID: "fact:postgres", ObjectType: "FactAssertion", Layer: "product", Title: "PostgreSQL fact", Statement: quote, EvidenceRefs: []string{spans[0].ID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	object, decision, err := service.Source.ReviewKnowledgeObject(ctx, actor, object.ID, application.ReviewKnowledgeObjectInput{ExpectedVersion: object.Version, ExpectedDigest: object.Digest, Decision: "approve", Reason: "已复核 PostgreSQL Evidence"}, "")
	if err != nil || object.Status != "verified" || decision.ResultVersion != 2 {
		t.Fatalf("knowledge object decision was not persisted: object=%#v decision=%#v err=%v", object, decision, err)
	}
	pack, err := service.Source.CreateKnowledgePack(ctx, actor, application.CreateKnowledgePackInput{ProjectID: project.ID, ID: "pack:postgres", Name: "PostgreSQL knowledge", Purpose: "test", ObjectRefs: []sourcedomain.KnowledgePackObjectRef{{ObjectID: object.ID, Version: object.Version}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := service.Source.PublishKnowledgePack(ctx, actor, pack.ID, "")
	if err != nil || snapshot.ID == "" {
		t.Fatalf("knowledge snapshot was not persisted: snapshot=%#v err=%v", snapshot, err)
	}
	asset, err := service.Review.CreateAsset(ctx, actor, application.CreateAssetInput{ProjectID: project.ID, Name: "PostgreSQL product asset", AssetType: "product_image", SourceRevisionID: revision.ID, UsageMode: "generation_reference"}, "")
	if err != nil {
		t.Fatal(err)
	}
	rights, err := service.Review.CreateRightsRecord(ctx, actor, application.CreateRightsRecordInput{AssetID: asset.ID, RightsHolder: "Source Brand", RightsType: "owned", Territories: []string{"CN"}, Channels: []string{"douyin"}, ProofSourceRevisionID: revision.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	rights, err = service.Review.ReviewRightsRecord(ctx, actor, rights.ID, "approve", "")
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := service.Review.EligibleAssets(ctx, actor, project.ID, "douyin")
	if err != nil {
		t.Fatal(err)
	}
	if rights.Status != "approved" || len(eligible) != 1 || eligible[0].Asset.ID != asset.ID {
		t.Fatalf("unexpected persisted asset rights: rights=%#v eligible=%#v", rights, eligible)
	}
}
