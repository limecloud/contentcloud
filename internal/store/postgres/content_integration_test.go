package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
	"github.com/limecloud/contentcloud/internal/testsupport"
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
	service := app.New(store, slog.Default())
	suffix := domain.NewID()
	session, err := service.Register(ctx, fmt.Sprintf("source-%s@example.com", suffix), "long-enough-password", "Source Reviewer", "Source Tenant "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	actor, _, err := service.SessionActor(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Source Brand", ProductName: "Source Product"}, "")
	if err != nil {
		t.Fatal(err)
	}
	quote := "PostgreSQL source evidence"
	revision, err := service.UploadSource(ctx, actor, project.ID, "Product facts", "product_spec", "facts.txt", "text/plain", []byte(quote), "")
	if err != nil {
		t.Fatal(err)
	}
	worker := actor
	worker.Type = "worker"
	revision, err = service.CompleteSource(ctx, worker, revision.ID, app.CompleteSourceInput{DetectedMIME: "text/plain", Status: "ready", ParserVersion: "test/v1", Evidence: []app.CreateEvidenceInput{{LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: quote}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if revision.ProcessingStatus != "ready" {
		t.Fatalf("expected ready revision, got %s", revision.ProcessingStatus)
	}
	sources, err := service.Sources(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Status != "ready" || sources[0].LatestRevision != revision.ID {
		t.Fatalf("unexpected source projection: %#v", sources)
	}
	spans, err := service.Evidence(ctx, actor, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].ReviewStatus != "accepted" || spans[0].QuoteText != quote {
		t.Fatalf("unexpected evidence projection: %#v", spans)
	}
	extractionRun, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{revision.ID}, IdempotencyKey: "postgres-extract-" + suffix, OutputCount: 7}, "")
	if err != nil {
		t.Fatal(err)
	}
	persistedRun, err := service.Run(ctx, actor, extractionRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedRun.CapabilityID != domain.KnowledgeExtractCapability || persistedRun.OutputSchema != domain.KnowledgeCandidatesSchema || persistedRun.OutputCount != 7 {
		t.Fatalf("knowledge extraction run contract fields were not persisted: %#v", persistedRun)
	}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := testsupport.ConnectBootstrap(ctx, service, actor, connect, app.ConnectDeviceInput{Hostname: "postgres-local", Platform: "darwin", Arch: "arm64", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	deviceActor, device, err := service.DeviceActor(ctx, connected.DeviceToken)
	if err != nil {
		t.Fatal(err)
	}
	capability := domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:postgres-test", LocalOnly: true}
	lease, err := service.Poll(ctx, deviceActor, device, []domain.Capability{capability})
	if err != nil {
		t.Fatal(err)
	}
	if lease.Run.ID != extractionRun.ID || lease.Attempt.CapabilityDigest != capability.Digest {
		t.Fatalf("unexpected persisted attempt lease: %#v", lease)
	}
	if _, err := service.HeartbeatRun(ctx, deviceActor, device, lease.Run.ID, lease.Attempt.ID, lease.RunToken, domain.RunHeartbeat{Sequence: 1, Phase: "executing", Label: "postgres"}, ""); err != nil {
		t.Fatal(err)
	}
	attempts, err := service.RunAttempts(ctx, actor, extractionRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].State != "running" || attempts[0].TokenHash != "" || attempts[0].HeartbeatAt == nil {
		t.Fatalf("unexpected persisted attempt history: %#v", attempts)
	}
	knowledge, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: project.ID, Kind: "fact", Title: "PostgreSQL fact", Statement: quote, Evidence: []domain.EvidenceRef{{SourceRevisionID: revision.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: quote}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviewKnowledge(ctx, actor, knowledge.ID, "approve", ""); err != nil {
		t.Fatal(err)
	}
	conflictingKnowledge, err := service.CreateKnowledge(ctx, actor, app.CreateKnowledgeInput{ProjectID: project.ID, Kind: "fact", Title: "PostgreSQL fact v2", Statement: "different value", Subject: knowledge.Title, Predicate: knowledge.Kind, Value: domain.TypedValue{Type: "text", Text: "different value"}, Evidence: []domain.EvidenceRef{{SourceRevisionID: revision.ID, LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: quote}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	conflicts, err := service.KnowledgeConflicts(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := service.DecisionRequests(ctx, actor, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if conflictingKnowledge.Status != "conflicted" || len(conflicts) != 1 || len(requests) != 1 {
		t.Fatalf("typed knowledge conflict was not persisted: knowledge=%#v conflicts=%#v requests=%#v", conflictingKnowledge, conflicts, requests)
	}
	if _, err := service.ResolveDecisionRequest(ctx, actor, requests[0].ID, conflictingKnowledge.ID, "PostgreSQL resolution", ""); err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateAsset(ctx, actor, app.CreateAssetInput{ProjectID: project.ID, Name: "PostgreSQL product asset", AssetType: "product_image", SourceRevisionID: revision.ID, UsageMode: "generation_reference"}, "")
	if err != nil {
		t.Fatal(err)
	}
	rights, err := service.CreateRightsRecord(ctx, actor, app.CreateRightsRecordInput{AssetID: asset.ID, RightsHolder: "Source Brand", RightsType: "owned", Territories: []string{"CN"}, Channels: []string{"douyin"}, ProofSourceRevisionID: revision.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	rights, err = service.ReviewRightsRecord(ctx, actor, rights.ID, "approve", "")
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := service.EligibleAssets(ctx, actor, project.ID, "douyin")
	if err != nil {
		t.Fatal(err)
	}
	if rights.Status != "approved" || len(eligible) != 1 || eligible[0].Asset.ID != asset.ID {
		t.Fatalf("unexpected persisted asset rights: rights=%#v eligible=%#v", rights, eligible)
	}
}
