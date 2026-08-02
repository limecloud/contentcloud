package app_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestProjectLineageAndImpactTraceSourceToRating(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := app.New(store, slog.Default())
	now := time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	tenantID, projectID := domain.NewID(), domain.NewID()
	actor := app.Actor{UserID: domain.NewID(), TenantID: tenantID, Role: "project_manager", Type: "user"}
	mustStore(t, store.CreateProject(ctx, domain.Project{ID: projectID, TenantID: tenantID, BrandName: "金陵古香", ProductName: "香品", Status: "active", CreatedAt: now, UpdatedAt: now}))

	sourceID, revisionID := domain.NewID(), domain.NewID()
	mustStore(t, store.CreateSource(ctx,
		domain.Source{ID: sourceID, TenantID: tenantID, ProjectID: projectID, Name: "品牌事实手册", Status: "ready", RevisionCount: 1, LatestRevision: revisionID, CreatedAt: now},
		domain.SourceRevision{ID: revisionID, TenantID: tenantID, ProjectID: projectID, SourceID: sourceID, FileName: "facts.docx", SHA256: "source-hash", ProcessingStatus: "ready", CreatedAt: now.Add(time.Minute)},
	))
	snapshotID, runID := domain.NewID(), domain.NewID()
	mustStore(t, store.CreateSnapshot(ctx, domain.ContextSnapshot{ID: snapshotID, TenantID: tenantID, ProjectID: projectID, Sources: []domain.ContractSource{{SourceID: sourceID, RevisionID: revisionID}}, CreatedAt: now.Add(2 * time.Minute)}))
	mustStore(t, store.CreateRun(ctx, domain.TaskRun{ID: runID, TenantID: tenantID, ProjectID: projectID, InputSnapshotID: snapshotID, IdempotencyKey: "lineage-run", TaskType: "knowledge_extract", State: "succeeded", CreatedAt: now.Add(2 * time.Minute)}))
	knowledgeID := domain.NewID()
	evidenceID := domain.NewID()
	mustStore(t, store.CreateEvidence(ctx, domain.EvidenceSpan{ID: evidenceID, TenantID: tenantID, ProjectID: projectID, RevisionID: revisionID, LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "原料事实", QuoteHash: "sha256:" + strings.Repeat("b", 64), ReviewStatus: "accepted", CreatedAt: now.Add(2 * time.Minute)}))
	object := domain.KnowledgeObject{ID: knowledgeID, TenantID: tenantID, ProjectID: projectID, ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "verified", Title: "原料事实", Statement: "原料事实", Payload: map[string]any{"origin_run_id": runID}, EvidenceRefs: []string{evidenceID}, CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)}
	object.Digest, _ = object.ContentDigest()
	mustStore(t, store.CreateKnowledgeObject(ctx, object))
	submissionID, revisionIDV3, approvedSnapshotID := domain.NewID(), domain.NewID(), domain.NewID()
	submission := domain.Submission{ID: submissionID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: domain.NewID(), SubmissionType: "content_batch", Status: "submitted", CurrentRevisionID: revisionIDV3, CreatedAt: now.Add(6 * time.Minute), UpdatedAt: now.Add(6 * time.Minute)}
	revision := domain.SubmissionRevision{ID: revisionIDV3, TenantID: tenantID, ProjectID: projectID, WorkspaceID: submission.WorkspaceID, SubmissionID: submissionID, RevisionNo: 1, SchemaVersion: "3.0", ContentHash: "sha256:" + strings.Repeat("a", 64), CreatedAt: now.Add(6 * time.Minute)}
	cycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revisionIDV3, Status: "approved", CreatedAt: now.Add(6 * time.Minute)}
	mustStore(t, store.CreateSubmissionRevision(ctx, submission, revision, nil, cycle))
	submission.Status = "approved"
	decision := domain.ApprovalDecision{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revisionIDV3, Decision: "approved", PreviousState: "submitted", ResultingState: "approved", CreatedAt: now.Add(6 * time.Minute)}
	snapshot := domain.ApprovedSnapshot{ID: approvedSnapshotID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: submission.WorkspaceID, SubmissionID: submissionID, SubmissionRevisionID: revisionIDV3, SubmissionType: "content_batch", SchemaVersion: "3.0", ContentHash: revision.ContentHash, DecisionID: decision.ID, CreatedAt: now.Add(6 * time.Minute)}
	mustStore(t, store.ApproveSubmissionRevision(ctx, submission, snapshot, decision))
	artifactID := domain.NewID()
	mustStore(t, store.CreateArtifact(ctx, domain.Artifact{ID: artifactID, TenantID: tenantID, ProjectID: projectID, ApprovedSnapshotID: approvedSnapshotID, Kind: "delivery", FileName: "content.xlsx", CreatedAt: now.Add(6 * time.Minute)}))
	batchID, observationID := domain.NewID(), domain.NewID()
	mustStore(t, store.CreatePerformanceImportBatch(ctx,
		domain.PerformanceImportBatch{ID: batchID, TenantID: tenantID, ProjectID: projectID, SourceName: "douyin.csv", Status: "imported", RowCount: 1, ImportedCount: 1, CreatedAt: now.Add(7 * time.Minute)},
		[]domain.PerformanceObservation{{ID: observationID, TenantID: tenantID, ProjectID: projectID, ImportBatchID: batchID, ApprovedSnapshotID: approvedSnapshotID, Platform: "douyin", AccountAlias: "main", WindowHours: 24, SampleStatus: "insufficient_sample", DedupKey: "lineage-observation", CreatedAt: now.Add(8 * time.Minute)}},
	))
	ratingID := domain.NewID()
	mustStore(t, store.CreateRatingDecision(ctx, domain.RatingDecision{ID: ratingID, TenantID: tenantID, ProjectID: projectID, SubjectType: "approved_snapshot", SubjectID: approvedSnapshotID, ObservationIDs: []string{observationID}, Rating: "repairable", NextAction: "创建单变量变体", CreatedAt: now.Add(9 * time.Minute)}))

	graph, err := service.ProjectLineage(ctx, actor, projectID, app.LineageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source:" + sourceID, "source_revision:" + revisionID, "task_run:" + runID, "knowledge_object:" + knowledgeID, "approved_snapshot:" + approvedSnapshotID, "artifact:" + artifactID, "performance_observation:" + observationID, "rating_decision:" + ratingID} {
		if !hasLineageNode(graph, key) {
			t.Fatalf("downstream graph is missing %s: %#v", key, graph.Nodes)
		}
	}
	if graph.FocusKey != "" || len(graph.Edges) == 0 {
		t.Fatalf("unexpected focused graph: %#v", graph)
	}

	upstream, err := service.ProjectLineage(ctx, actor, projectID, app.LineageQuery{FocusType: "rating_decision", FocusID: ratingID, Direction: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasLineageNode(upstream, "performance_import_batch:"+batchID) || !hasLineageNode(upstream, "performance_observation:"+observationID) {
		t.Fatalf("rating upstream graph lost result import lineage: %#v", upstream.Nodes)
	}

	impact, err := service.ProjectImpact(ctx, actor, projectID, app.LineageQuery{FocusType: "performance_observation", FocusID: observationID})
	if err != nil {
		t.Fatal(err)
	}
	if impact.Focus == nil || impact.Focus.ID != observationID || !hasImpactNode(impact, ratingID, "attention") {
		t.Fatalf("impact did not expose current status and action: %#v", impact)
	}
}

func TestProjectLineageValidatesFocusAndTenant(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := app.New(store, slog.Default())
	project := domain.Project{ID: domain.NewID(), TenantID: domain.NewID(), Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	mustStore(t, store.CreateProject(ctx, project))
	actor := app.Actor{TenantID: project.TenantID, UserID: domain.NewID(), Role: "viewer"}
	if _, err := service.ProjectLineage(ctx, actor, project.ID, app.LineageQuery{FocusType: "source"}); err == nil {
		t.Fatal("partial focus must be rejected")
	}
	if _, err := service.ProjectLineage(ctx, actor, project.ID, app.LineageQuery{FocusType: "source", FocusID: domain.NewID()}); err == nil {
		t.Fatal("unknown focused object must be rejected")
	}
	actor.TenantID = domain.NewID()
	if _, err := service.ProjectLineage(ctx, actor, project.ID, app.LineageQuery{}); err == nil {
		t.Fatal("cross-tenant project lineage must be rejected")
	}
}

func mustStore(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func hasLineageNode(graph domain.LineageGraph, key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func hasImpactNode(analysis domain.ImpactAnalysis, id, severity string) bool {
	for _, item := range analysis.Items {
		if item.Node.ID == id && item.Severity == severity && item.Reason != "" && item.RecommendedAction != "" {
			return true
		}
	}
	return false
}
