package postgres_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	storepg "github.com/limecloud/contentcloud/internal/store/postgres"
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
	if persistedRun.BriefVersionID != "" || persistedRun.CapabilityID != domain.KnowledgeExtractCapability || persistedRun.OutputSchema != domain.KnowledgeCandidatesSchema || persistedRun.OutputCount != 7 {
		t.Fatalf("knowledge extraction run contract fields were not persisted: %#v", persistedRun)
	}
	connect, err := service.CreateConnectSession(ctx, actor, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	connected, err := service.ConnectDevice(ctx, app.ConnectDeviceInput{ConnectKey: connect.PlaintextConnectKey, Hostname: "postgres-local", Platform: "darwin", Arch: "arm64", Version: "test"})
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
	now := lease.Attempt.CreatedAt
	logical := domain.Script{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, Title: "PostgreSQL script", CreatedAt: now}
	firstVersion := domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, RunID: extractionRun.ID, ChangeType: "initial", InvariantFields: []string{}, ChangedFields: []string{}, Status: "review_ready", InputSnapshotID: extractionRun.InputSnapshotID, ContentHash: "script-v1-" + suffix, Package: domain.ScriptPackage{SchemaVersion: "1.1", Title: "PostgreSQL script"}, Validation: domain.ValidationReport{Valid: true, Errors: []domain.ValidationIssue{}, Warnings: []domain.ValidationIssue{}}, CreatedAt: now}
	firstVersion, err = store.CreateScript(ctx, logical, firstVersion)
	if err != nil {
		t.Fatal(err)
	}
	secondRun := extractionRun
	secondRun.ID = domain.NewID()
	secondRun.IdempotencyKey = "postgres-script-v2-" + suffix
	secondRun.ScriptID = logical.ID
	secondRun.BaselineVersionID = firstVersion.ID
	secondRun.ChangeType = "revision"
	secondRun.RevisionReason = "PostgreSQL lineage test"
	secondRun.State = "queued"
	secondRun.ActiveAttemptID = ""
	secondRun.LeaseDeviceID = ""
	secondRun.LeaseExpiresAt = nil
	secondRun.RunTokenHash = ""
	secondRun.AttemptCount = 0
	if err := store.CreateRun(ctx, secondRun); err != nil {
		t.Fatal(err)
	}
	secondVersion := domain.ScriptVersion{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ScriptID: logical.ID, RunID: secondRun.ID, SupersedesID: firstVersion.ID, BaselineID: firstVersion.ID, ChangeType: "revision", InvariantFields: []string{"/channel"}, ChangedFields: []string{"/title"}, RevisionReason: "PostgreSQL lineage test", Status: "review_ready", InputSnapshotID: extractionRun.InputSnapshotID, ContentHash: "script-v2-" + suffix, Package: domain.ScriptPackage{SchemaVersion: "1.1", Title: "PostgreSQL script v2"}, Validation: domain.ValidationReport{Valid: true, Errors: []domain.ValidationIssue{}, Warnings: []domain.ValidationIssue{}}, CreatedAt: now.Add(1)}
	secondVersion, err = store.CreateScriptVersion(ctx, secondVersion)
	if err != nil {
		t.Fatal(err)
	}
	persistedVersion, err := service.Script(ctx, actor, secondVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstVersion.Version != 1 || persistedVersion.Version != 2 || persistedVersion.ScriptID != logical.ID || persistedVersion.BaselineID != firstVersion.ID || len(persistedVersion.ChangedFields) != 1 {
		t.Fatalf("script version lineage was not persisted: first=%#v second=%#v", firstVersion, persistedVersion)
	}
	persistedVersion.Status = "internal_review"
	persistedVersion.Package.Title = "tampered title"
	if err := store.SaveScript(ctx, persistedVersion); err != nil {
		t.Fatal(err)
	}
	immutableVersion, err := service.Script(ctx, actor, secondVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if immutableVersion.Status != "internal_review" || immutableVersion.Package.Title != "PostgreSQL script v2" {
		t.Fatalf("SaveScript must only change workflow status: %#v", immutableVersion)
	}
	reviewCycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "script_version", SubjectID: secondVersion.ID, Status: "open", OpenedBy: actor.UserID, OpenedAt: now, CreatedAt: now}
	reviewCycle, err = store.CreateReviewCycle(ctx, reviewCycle)
	if err != nil {
		t.Fatal(err)
	}
	if reviewCycle.CycleNumber != 1 {
		t.Fatalf("expected first review cycle number 1, got %d", reviewCycle.CycleNumber)
	}
	firstComment := domain.ReviewComment{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ReviewCycleID: reviewCycle.ID, SubjectType: "script_version", SubjectID: secondVersion.ID, ShotID: "shot-1", JSONPointer: "/shots/0", Body: "PostgreSQL review comment", Visibility: "internal", AuthorID: actor.UserID, CreatedAt: now}
	if err := store.CreateReviewComment(ctx, firstComment); err != nil {
		t.Fatal(err)
	}
	carriedComment := firstComment
	carriedComment.ID = domain.NewID()
	carriedComment.CarriedFromID = firstComment.ID
	carriedComment.Body = "Carried PostgreSQL review comment"
	carriedComment.CreatedAt = now.Add(1)
	if err := store.CreateReviewComment(ctx, carriedComment); err != nil {
		t.Fatal(err)
	}
	decidedAt := now.Add(2)
	reviewCycle.Status = "changes_requested"
	reviewCycle.Conclusion = "Revise the opening shot"
	reviewCycle.AssigneeUserID = actor.UserID
	reviewCycle.DecidedBy = actor.UserID
	reviewCycle.DecidedAt = &decidedAt
	if err := store.SaveReviewCycle(ctx, reviewCycle); err != nil {
		t.Fatal(err)
	}
	persistedCycles, err := store.ReviewCycles(ctx, actor.TenantID, secondVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	persistedComments, err := store.ReviewComments(ctx, actor.TenantID, secondVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persistedCycles) != 1 || persistedCycles[0].Status != "changes_requested" || persistedCycles[0].Conclusion != reviewCycle.Conclusion || persistedCycles[0].AssigneeUserID != actor.UserID {
		t.Fatalf("review cycle was not persisted: %#v", persistedCycles)
	}
	if len(persistedComments) != 2 || persistedComments[0].ReviewCycleID != reviewCycle.ID || persistedComments[1].CarriedFromID != firstComment.ID {
		t.Fatalf("review comment cycle or carry lineage was not persisted: %#v", persistedComments)
	}
	reviewGrant := domain.ReviewGrant{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, SubjectType: "script_version", SubjectID: secondVersion.ID, SubjectHash: secondVersion.ContentHash, ReviewerEmail: "postgres-review@example.com", TokenHash: "secret-token-hash-" + suffix, OTPHash: "secret-otp-hash-" + suffix, ExpiresAt: now.Add(72 * time.Hour), CreatedAt: now}
	if err := store.CreateReviewGrant(ctx, reviewGrant); err != nil {
		t.Fatal(err)
	}
	persistedGrants, err := store.ReviewGrants(ctx, actor.TenantID, secondVersion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persistedGrants) != 1 || persistedGrants[0].TokenHash != "" || persistedGrants[0].OTPHash != "" {
		t.Fatalf("review grant list must persist the grant without exposing secrets: %#v", persistedGrants)
	}
	revokedAt := now.Add(3 * time.Minute).Truncate(time.Microsecond)
	reviewGrant.RevokedAt = &revokedAt
	if err := store.SaveReviewGrant(ctx, reviewGrant); err != nil {
		t.Fatal(err)
	}
	persistedGrant, err := store.ReviewGrant(ctx, actor.TenantID, reviewGrant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedGrant.RevokedAt == nil || !persistedGrant.RevokedAt.Equal(revokedAt) {
		t.Fatalf("review grant revocation was not persisted: %#v", persistedGrant)
	}
	artifactEnvelope := domain.ExtensionArtifactEnvelopeV1{EnvelopeVersion: "1.0", ProjectID: project.ID, ScriptVersionID: secondVersion.ID, Capability: domain.ArtifactCapabilityRef{ID: domain.ArtifactExportCapability, Version: "1.0.0", Digest: "postgres-artifact-digest"}, SchemaID: "test.timeline/1.0", MediaType: "application/octet-stream", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 128, Renditions: []domain.ArtifactRenditionRef{}, Metadata: map[string]any{"variant": "A"}}
	artifact := domain.Artifact{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ScriptVersionID: secondVersion.ID, Kind: "extension", CapabilityID: artifactEnvelope.Capability.ID, CapabilityVersion: artifactEnvelope.Capability.Version, CapabilityDigest: artifactEnvelope.Capability.Digest, SchemaID: artifactEnvelope.SchemaID, MediaType: artifactEnvelope.MediaType, FileName: "timeline.bin", SHA256: artifactEnvelope.SHA256, ByteSize: artifactEnvelope.Size, Visibility: "internal", RetentionClass: "project", Purpose: "primary", SourceDeviceID: device.ID, ValidationStatus: "valid", Envelope: &artifactEnvelope, PresentationTier: "metadata_only", Metadata: artifactEnvelope.Metadata, CreatedAt: now}
	if err := store.CreateArtifact(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	persistedArtifact, err := store.Artifact(ctx, actor.TenantID, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedArtifact.Envelope == nil || persistedArtifact.Envelope.SchemaID != artifactEnvelope.SchemaID || persistedArtifact.SourceDeviceID != device.ID || persistedArtifact.CapabilityDigest != artifactEnvelope.Capability.Digest {
		t.Fatalf("artifact envelope fields were not persisted: %#v", persistedArtifact)
	}
	openRequest := domain.ArtifactOpenRequest{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, ArtifactID: artifact.ID, DeviceID: device.ID, RequestedBy: actor.UserID, State: "pending", ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err := store.CreateArtifactOpenRequest(ctx, openRequest); err != nil {
		t.Fatal(err)
	}
	pendingOpenRequests, err := store.PendingArtifactOpenRequests(ctx, actor.TenantID, device.ID, now, 1)
	if err != nil || len(pendingOpenRequests) != 1 || pendingOpenRequests[0].ID != openRequest.ID {
		t.Fatalf("artifact open request was not persisted: %v %#v", err, pendingOpenRequests)
	}
	completedAt := now.Add(10 * time.Second)
	openRequest.State = "opened"
	openRequest.AcceptedAt = &completedAt
	openRequest.CompletedAt = &completedAt
	if err := store.SaveArtifactOpenRequest(ctx, openRequest); err != nil {
		t.Fatal(err)
	}
	persistedOpenRequest, err := store.ArtifactOpenRequest(ctx, actor.TenantID, openRequest.ID)
	if err != nil || persistedOpenRequest.State != "opened" || persistedOpenRequest.CompletedAt == nil {
		t.Fatalf("artifact open terminal state was not persisted: %v %#v", err, persistedOpenRequest)
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
