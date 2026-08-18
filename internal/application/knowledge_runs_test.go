package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/application"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestKnowledgeExtractionRuntimeWorkerImportsGroundedCandidates(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	session, err := service.Identity.Register(ctx, "extract@example.com", "long-enough-password", "Owner", "Extract Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Incense", Channel: "douyin"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "每盒净含量为 10 克。", nil)

	run, err := service.Source.CreateKnowledgeExtractionRun(ctx, actor, application.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-1", OutputCount: 5}, "")
	must(t, err)
	if _, err := uuid.Parse(run.InputSnapshotID); err != nil {
		t.Fatalf("context snapshot id must satisfy the PostgreSQL UUID schema: %q", run.InputSnapshotID)
	}
	repeatedRun, err := service.Source.CreateKnowledgeExtractionRun(ctx, actor, application.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-1", OutputCount: 5}, "")
	must(t, err)
	if repeatedRun.ID != run.ID || repeatedRun.InputSnapshotID != run.InputSnapshotID {
		t.Fatalf("knowledge extraction admission must be idempotent: first=%#v repeated=%#v", run, repeatedRun)
	}
	worker := actor
	worker.Type = "worker"
	handle, err := service.Runtime.PrepareRuntimeWorker(ctx, worker, application.RuntimeWorkerPrepareInput{JobRunID: run.ID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "knowledge-extractor", ExecutionProfileID: "knowledge-extract-v1", MaxTokens: 4096, BudgetMinor: 100})
	must(t, err)
	handle, err = service.Runtime.ActivateRuntimeWorker(ctx, worker, application.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "knowledge-session-1"}})
	must(t, err)

	value := 10.0
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{
		Kind: "fact", Title: "净含量", Statement: "每盒净含量为 10 克。", Subject: "Incense", Predicate: "净含量",
		Value: sourcedomain.TypedValue{Type: "number", Number: &value, Unit: "g"}, Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}},
		RiskLevel: "low", AllowedChannels: []string{}, Evidence: []sourcedomain.EvidenceRef{ref}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, err := json.Marshal(pkg)
	must(t, err)
	input := application.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: contentruntime.RuntimeAttemptSucceeded, BusinessPayload: body, SafeSummary: map[string]any{"candidate_count": 1}}
	finalized, err := service.Runtime.FinalizeRuntimeWorker(ctx, worker, input, "extract-finalize")
	must(t, err)
	if finalized.Job.State != contentruntime.JobRunCompleted || finalized.BusinessResultRef == "" {
		t.Fatalf("knowledge extraction did not complete through Runtime: %#v", finalized)
	}
	objects, err := service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("business objects must wait for the durable result consumer: %#v", objects)
	}
	businessPending, err := store.RuntimeOutboxMessages(ctx, actor.TenantID, contentruntime.RuntimeOutboxSubscriberBusinessResult, time.Now(), 20)
	must(t, err)
	if len(businessPending) != 1 {
		t.Fatalf("successful Runtime result must publish one business receipt: %#v", businessPending)
	}
	projected, err := contentruntime.NewProjector(store, time.Now).RunOnce(ctx, actor.TenantID, "projector-independent", time.Minute, 100)
	must(t, err)
	if projected.Projected == 0 {
		t.Fatalf("expected Runtime projection messages, got %#v", projected)
	}
	businessPending, err = store.RuntimeOutboxMessages(ctx, actor.TenantID, contentruntime.RuntimeOutboxSubscriberBusinessResult, time.Now(), 20)
	must(t, err)
	if len(businessPending) != 1 {
		t.Fatalf("projection acknowledgement must not consume the business receipt: %#v", businessPending)
	}
	consumed, err := service.Runtime.ConsumeRuntimeBusinessResults(ctx, actor.TenantID, "business-consumer-1", time.Minute, 20)
	must(t, err)
	if consumed.Applied != 1 || consumed.Retried != 0 {
		t.Fatalf("unexpected durable business result run: %#v", consumed)
	}
	objects, err = service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 || objects[0].Payload["origin_run_id"] != run.ID || objects[0].Status != "needs_review" {
		t.Fatalf("unexpected extraction objects: %#v", objects)
	}
	if _, err := service.Runtime.FinalizeRuntimeWorker(ctx, worker, input, "extract-finalize-retry"); err != nil {
		t.Fatalf("identical terminal retry must be idempotent: %v", err)
	}
	objects, err = service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 {
		t.Fatalf("terminal retry duplicated candidates: %#v", objects)
	}
	replayed, err := service.Runtime.ConsumeRuntimeBusinessResults(ctx, actor.TenantID, "business-consumer-2", time.Minute, 20)
	must(t, err)
	if replayed.Claimed != 0 {
		t.Fatalf("acknowledged business result must not be republished by terminal retry: %#v", replayed)
	}
}

type failBusinessAckStore struct {
	*memory.Store
	failAck bool
}

func (s *failBusinessAckStore) AckRuntimeOutbox(ctx context.Context, tenantID, messageID, subscriber, worker string, deliveredAt time.Time) error {
	if subscriber == contentruntime.RuntimeOutboxSubscriberBusinessResult && s.failAck {
		s.failAck = false
		return errors.New("simulated business receipt ack failure")
	}
	return s.Store.AckRuntimeOutbox(ctx, tenantID, messageID, subscriber, worker, deliveredAt)
}

func TestKnowledgeResultRecoversAfterBusinessWriteBeforeAck(t *testing.T) {
	ctx := context.Background()
	store := &failBusinessAckStore{Store: memory.New(), failAck: true}
	blobs := blob.NewMemory()
	service := application.NewWithBlob(application.DependenciesFrom(store), slog.Default(), blobs)
	session, err := service.Identity.Register(ctx, "extract-recovery@example.com", "long-enough-password", "Owner", "Extract Recovery Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product", Channel: "douyin"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "产品净含量为 12 克。", nil)
	run, err := service.Source.CreateKnowledgeExtractionRun(ctx, actor, application.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-recovery", OutputCount: 1}, "")
	must(t, err)
	workerActor := actor
	workerActor.Type = "worker"
	handle, err := service.Runtime.PrepareRuntimeWorker(ctx, workerActor, application.RuntimeWorkerPrepareInput{JobRunID: run.ID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "knowledge-extractor", ExecutionProfileID: "knowledge-extract-v1", MaxTokens: 4096, BudgetMinor: 100})
	must(t, err)
	handle, err = service.Runtime.ActivateRuntimeWorker(ctx, workerActor, application.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "knowledge-session-recovery"}})
	must(t, err)
	value := 12.0
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{Kind: "fact", Title: "净含量", Statement: "产品净含量为 12 克。", Subject: "Product", Predicate: "净含量", Value: sourcedomain.TypedValue{Type: "number", Number: &value, Unit: "g"}, Scope: sourcedomain.KnowledgeScope{}, RiskLevel: "low", Evidence: []sourcedomain.EvidenceRef{ref}}}}
	body, err := json.Marshal(pkg)
	must(t, err)
	finalized, err := service.Runtime.FinalizeRuntimeWorker(ctx, workerActor, application.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: contentruntime.RuntimeAttemptSucceeded, BusinessPayload: body}, "extract-recovery-finalize")
	must(t, err)
	resultKey := strings.TrimPrefix(finalized.BusinessResultRef, "runtime-result:")
	must(t, blobs.Put(ctx, resultKey, []byte(`{"schema_version":"1.0","candidates":[]}`)))
	corrupt, err := service.Runtime.ConsumeRuntimeBusinessResults(ctx, actor.TenantID, "business-corrupt-result", time.Minute, 20)
	must(t, err)
	if corrupt.Retried != 1 || corrupt.Applied != 0 {
		t.Fatalf("digest mismatch must remain pending for retry: %#v", corrupt)
	}
	objects, err := service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("digest mismatch must not materialize business objects: %#v", objects)
	}
	must(t, blobs.Put(ctx, resultKey, body))
	time.Sleep(1100 * time.Millisecond)
	if _, err := service.Runtime.ConsumeRuntimeBusinessResults(ctx, actor.TenantID, "business-before-crash", 5*time.Millisecond, 20); err == nil {
		t.Fatal("simulated acknowledgement failure must escape the consumer run")
	}
	objects, err = service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 {
		t.Fatalf("business write must complete before the failed acknowledgement: %#v", objects)
	}
	time.Sleep(10 * time.Millisecond)
	restarted := application.NewWithBlob(application.DependenciesFrom(store), slog.Default(), blobs)
	recovered, err := restarted.Runtime.ConsumeRuntimeBusinessResults(ctx, actor.TenantID, "business-after-restart", time.Minute, 20)
	must(t, err)
	if recovered.Applied != 1 || recovered.Retried != 0 {
		t.Fatalf("restarted consumer did not idempotently recover the receipt: %#v", recovered)
	}
	objects, err = restarted.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 1 {
		t.Fatalf("recovery duplicated the knowledge candidate: %#v", objects)
	}
}

func TestKnowledgeExtractionRuntimeWorkerRejectsEvidenceOutsideFrozenContract(t *testing.T) {
	ctx := context.Background()
	blobs := blob.NewMemory()
	service := application.NewWithBlob(application.DependenciesFrom(memory.New()), slog.Default(), blobs)
	session, err := service.Identity.Register(ctx, "extract-invalid@example.com", "long-enough-password", "Owner", "Extract Invalid Tenant")
	must(t, err)
	actor, _, err := service.Identity.SessionActor(ctx, session.ID)
	must(t, err)
	project, err := service.Workspace.CreateProject(ctx, actor, application.CreateProjectInput{BrandName: "Brand", ProductName: "Product"}, "")
	must(t, err)
	ref := createAcceptedEvidence(t, ctx, service, actor, project.ID, "可信原文", nil)
	validRef := ref
	run, err := service.Source.CreateKnowledgeExtractionRun(ctx, actor, application.CreateKnowledgeExtractionRunInput{ProjectID: project.ID, SourceRevisionIDs: []string{ref.SourceRevisionID}, IdempotencyKey: "extract-invalid", OutputCount: 1}, "")
	must(t, err)
	worker := actor
	worker.Type = "worker"
	handle, err := service.Runtime.PrepareRuntimeWorker(ctx, worker, application.RuntimeWorkerPrepareInput{JobRunID: run.ID, HarnessKind: "fake", Capabilities: agentadapter.HarnessCapabilities{Kind: "fake", Events: true, StructuredOutput: true, Resume: true, MaxParallelSessions: 128}, Role: "knowledge-extractor", ExecutionProfileID: "knowledge-extract-v1", MaxTokens: 4096, BudgetMinor: 100})
	must(t, err)
	handle, err = service.Runtime.ActivateRuntimeWorker(ctx, worker, application.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: agentadapter.AgentSessionRef{TenantID: actor.TenantID, HarnessKind: "fake", SessionID: "knowledge-session-invalid"}})
	must(t, err)

	ref.Locator = `{"paragraph":999}`
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{Kind: "fact", Title: "Fact", Statement: "可信原文", Subject: "Product", Predicate: "fact", Value: sourcedomain.TypedValue{Type: "text", Text: "可信原文"}, Scope: sourcedomain.KnowledgeScope{}, RiskLevel: "low", Evidence: []sourcedomain.EvidenceRef{ref}}}, Warnings: []string{}}
	body, err := json.Marshal(pkg)
	must(t, err)
	finalized, err := service.Runtime.FinalizeRuntimeWorker(ctx, worker, application.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: contentruntime.RuntimeAttemptSucceeded, BusinessPayload: body}, "extract-invalid-finalize")
	assertDomainCode(t, err, "KNOWLEDGE_CANDIDATE_GROUNDING_INVALID")
	if finalized.Handle.Attempt.State != contentruntime.RuntimeAttemptFailed || finalized.Job.State != contentruntime.JobRunFailed {
		t.Fatalf("invalid business result must fail Runtime: %#v", finalized)
	}
	var resultValue any
	must(t, json.Unmarshal(body, &resultValue))
	resultDigest, err := stablehash.Sum(resultValue)
	must(t, err)
	resultKey := "runtime/results/" + actor.TenantID + "/" + handle.Attempt.ID + "/" + resultDigest + ".json"
	if _, err := blobs.Get(ctx, resultKey); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("rejected business result left an orphan Blob: %v", err)
	}
	objects, err := service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("rejected package must not partially import candidates: %#v", objects)
	}
	corrected := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{Kind: "fact", Title: "Fact", Statement: "可信原文", Subject: "Product", Predicate: "fact", Value: sourcedomain.TypedValue{Type: "text", Text: "可信原文"}, Scope: sourcedomain.KnowledgeScope{}, RiskLevel: "low", Evidence: []sourcedomain.EvidenceRef{validRef}}}, Warnings: []string{}}
	correctedBody, err := json.Marshal(corrected)
	must(t, err)
	if _, err := service.Runtime.FinalizeRuntimeWorker(ctx, worker, application.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: contentruntime.RuntimeAttemptSucceeded, BusinessPayload: correctedBody}, "extract-corrected-finalize"); err == nil {
		t.Fatal("failed terminal attempt must reject a corrected result instead of changing terminal history")
	}
	objects, err = service.Source.KnowledgeObjects(ctx, actor, project.ID)
	must(t, err)
	if len(objects) != 0 {
		t.Fatalf("terminal conflict must be rejected before business writes: %#v", objects)
	}
}
