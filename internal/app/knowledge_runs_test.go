package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestKnowledgeExtractionRuntimeWorkerImportsGroundedCandidates(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "extract@example.com", "long-enough-password", "Owner", "Extract Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Incense", Channel: "douyin"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "每盒净含量为 10 克。", nil)

	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-1", OutputCount: 5}, "")
	must(t, err)
	if _, err := uuid.Parse(run.InputSnapshotID); err != nil {
		t.Fatalf("context snapshot id must satisfy the PostgreSQL UUID schema: %q", run.InputSnapshotID)
	}
	repeatedRun, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-1", OutputCount: 5}, "")
	must(t, err)
	if repeatedRun.ID != run.ID || repeatedRun.InputSnapshotID != run.InputSnapshotID {
		t.Fatalf("knowledge extraction admission must be idempotent: first=%#v repeated=%#v", run, repeatedRun)
	}
	worker := actor
	worker.Type = "worker"
	handle, err := service.PrepareRuntimeWorker(ctx, worker, app.RuntimeWorkerPrepareInput{JobRunID: run.ID, HarnessKind: "fake", Role: "knowledge-extractor", ExecutionProfileID: "knowledge-extract-v1", MaxTokens: 4096, BudgetMinor: 100})
	must(t, err)
	handle, err = service.ActivateRuntimeWorker(ctx, worker, app.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "knowledge-session-1"}})
	must(t, err)

	value := 10.0
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{
		Kind: "fact", Title: "净含量", Statement: "每盒净含量为 10 克。", Subject: "Incense", Predicate: "净含量",
		Value: domain.TypedValue{Type: "number", Number: &value, Unit: "g"}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}},
		RiskLevel: "low", AllowedChannels: []string{}, Evidence: []domain.EvidenceRef{ref}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, err := json.Marshal(pkg)
	must(t, err)
	input := app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, BusinessPayload: body, SafeSummary: map[string]any{"candidate_count": 1}}
	finalized, err := service.FinalizeRuntimeWorker(ctx, worker, input, "extract-finalize")
	must(t, err)
	if finalized.Job.State != domain.JobRunCompleted || finalized.BusinessResultRef == "" {
		t.Fatalf("knowledge extraction did not complete through Runtime: %#v", finalized)
	}
	objects, err := service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 || objects[0].Payload["origin_run_id"] != run.ID || objects[0].Status != "needs_review" {
		t.Fatalf("unexpected extraction objects: %#v", objects)
	}
	if _, err := service.FinalizeRuntimeWorker(ctx, worker, input, "extract-finalize-retry"); err != nil {
		t.Fatalf("identical terminal retry must be idempotent: %v", err)
	}
	objects, err = service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 {
		t.Fatalf("terminal retry duplicated candidates: %#v", objects)
	}
}

func TestKnowledgeExtractionRuntimeWorkerRejectsEvidenceOutsideFrozenContract(t *testing.T) {
	ctx := context.Background()
	service := app.New(memory.New(), slog.Default())
	session, err := service.Register(ctx, "extract-invalid@example.com", "long-enough-password", "Owner", "Extract Invalid Tenant")
	must(t, err)
	actor, _, err := service.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.CreateProject(ctx, actor, app.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "可信原文", nil)
	validRef := ref
	run, err := service.CreateKnowledgeExtractionRun(ctx, actor, app.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-invalid", OutputCount: 1}, "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	handle, err := service.PrepareRuntimeWorker(ctx, worker, app.RuntimeWorkerPrepareInput{JobRunID: run.ID, HarnessKind: "fake", Role: "knowledge-extractor", ExecutionProfileID: "knowledge-extract-v1", MaxTokens: 4096, BudgetMinor: 100})
	must(t, err)
	handle, err = service.ActivateRuntimeWorker(ctx, worker, app.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "knowledge-session-invalid"}})
	must(t, err)

	ref.Locator = `{"paragraph":999}`
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{Kind: "fact", Title: "Fact", Statement: "可信原文", Subject: "Product", Predicate: "fact", Value: domain.TypedValue{Type: "text", Text: "可信原文"}, Scope: domain.KnowledgeScope{}, RiskLevel: "low", Evidence: []domain.EvidenceRef{ref}}}, Warnings: []string{}}
	body, err := json.Marshal(pkg)
	must(t, err)
	finalized, err := service.FinalizeRuntimeWorker(ctx, worker, app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, BusinessPayload: body}, "extract-invalid-finalize")
	assertDomainCode(t, err, "KNOWLEDGE_CANDIDATE_GROUNDING_INVALID")
	if finalized.Handle.Attempt.State != domain.RuntimeAttemptFailed || finalized.Job.State != domain.JobRunFailed {
		t.Fatalf("invalid business result must fail Runtime: %#v", finalized)
	}
	objects, err := service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("rejected package must not partially import candidates: %#v", objects)
	}
	corrected := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{Kind: "fact", Title: "Fact", Statement: "可信原文", Subject: "Product", Predicate: "fact", Value: domain.TypedValue{Type: "text", Text: "可信原文"}, Scope: domain.KnowledgeScope{}, RiskLevel: "low", Evidence: []domain.EvidenceRef{validRef}}}, Warnings: []string{}}
	correctedBody, err := json.Marshal(corrected)
	must(t, err)
	if _, err := service.FinalizeRuntimeWorker(ctx, worker, app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, BusinessPayload: correctedBody}, "extract-corrected-finalize"); err == nil {
		t.Fatal("failed terminal attempt must reject a corrected result instead of changing terminal history")
	}
	objects, err = service.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("terminal conflict must be rejected before business writes: %#v", objects)
	}
}
