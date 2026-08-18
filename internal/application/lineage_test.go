package application_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/experience/projection"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"

	"github.com/limecloud/contentcloud/internal/application"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	performancedomain "github.com/limecloud/contentcloud/internal/performance"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

func TestProjectLineageAndImpactTraceSourceToRating(t *testing.T) {
	ctx := t.Context()
	store := memory.New()
	service := application.New(application.DependenciesFrom(store), slog.Default())
	now := time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC)
	tenantID, projectID := idgen.New(), idgen.New()
	actor := application.Actor{UserID: idgen.New(), TenantID: tenantID, Role: "project_manager", Type: "user"}
	mustStore(t, store.CreateProject(ctx, workspacedomain.Project{ID: projectID, TenantID: tenantID, BrandName: "金陵古香", ProductName: "香品", Status: "active", CreatedAt: now, UpdatedAt: now}))

	sourceID, revisionID := idgen.New(), idgen.New()
	mustStore(t, store.CreateSource(ctx,
		sourcedomain.Source{ID: sourceID, TenantID: tenantID, ProjectID: projectID, Name: "品牌事实手册", Status: "ready", RevisionCount: 1, LatestRevision: revisionID, CreatedAt: now},
		sourcedomain.SourceRevision{ID: revisionID, TenantID: tenantID, ProjectID: projectID, SourceID: sourceID, FileName: "facts.docx", SHA256: "source-hash", ProcessingStatus: "ready", CreatedAt: now.Add(time.Minute)},
	))
	snapshotID := idgen.New()
	mustStore(t, store.CreateSnapshot(ctx, sourcedomain.ContextSnapshot{ID: snapshotID, TenantID: tenantID, ProjectID: projectID, Sources: []sourcedomain.ContractSource{{SourceID: sourceID, RevisionID: revisionID}}, CreatedAt: now.Add(2 * time.Minute)}))
	started, err := service.Runtime.Runtime().Start(ctx, contentruntime.StartInput{
		TenantID: tenantID, ProjectID: projectID, WorkTaskID: "lineage:" + snapshotID, BusinessType: "knowledge_extract", InputSnapshotID: snapshotID,
		SOP:           catalogdomain.SOPVersion{ID: "lineage-sop-v1", TenantID: tenantID, SOPID: "lineage-sop", Version: 1, SchemaVersion: catalogdomain.SOPSchemaVersion, Name: "知识提取", Status: "published", DefaultExecutionMode: "agent", Stages: []catalogdomain.StageDefinition{{ID: "extract", Name: "提取", Order: 10, OutputSchema: sourcedomain.KnowledgeCandidatesSchema, ExecutionModes: []string{"agent"}}}},
		BindingDigest: "sha256:" + strings.Repeat("c", 64), InputDigest: "sha256:" + strings.Repeat("d", 64), RuntimePolicyID: "runtime-policy/lineage", ContractMajor: 1, CreatedBy: actor.UserID, IdempotencyKey: "lineage-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := started.Job.ID
	knowledgeID := idgen.New()
	evidenceID := idgen.New()
	mustStore(t, store.CreateEvidence(ctx, sourcedomain.EvidenceSpan{ID: evidenceID, TenantID: tenantID, ProjectID: projectID, RevisionID: revisionID, LocatorKind: "paragraph", Locator: map[string]any{"paragraph": 1}, QuoteText: "原料事实", QuoteHash: "sha256:" + strings.Repeat("b", 64), ReviewStatus: "accepted", CreatedAt: now.Add(2 * time.Minute)}))
	object := sourcedomain.KnowledgeObject{ID: knowledgeID, TenantID: tenantID, ProjectID: projectID, ObjectType: "FactAssertion", Layer: "product", Version: 1, Status: "verified", Title: "原料事实", Statement: "原料事实", Payload: map[string]any{"origin_run_id": runID}, EvidenceRefs: []string{evidenceID}, CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)}
	object.Digest, _ = object.ContentDigest()
	mustStore(t, store.CreateKnowledgeObject(ctx, object))
	submissionID, revisionIDV3, approvedSnapshotID := idgen.New(), idgen.New(), idgen.New()
	submission := reviewdomain.Submission{ID: submissionID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: idgen.New(), SubmissionType: "content_batch", Status: "submitted", CurrentRevisionID: revisionIDV3, CreatedAt: now.Add(6 * time.Minute), UpdatedAt: now.Add(6 * time.Minute)}
	revision := reviewdomain.SubmissionRevision{ID: revisionIDV3, TenantID: tenantID, ProjectID: projectID, WorkspaceID: submission.WorkspaceID, SubmissionID: submissionID, RevisionNo: 1, SchemaVersion: "3.0", ContentHash: "sha256:" + strings.Repeat("a", 64), CreatedAt: now.Add(6 * time.Minute)}
	cycle := reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revisionIDV3, Status: "approved", CreatedAt: now.Add(6 * time.Minute)}
	mustStore(t, store.CreateSubmissionRevision(ctx, submission, revision, nil, cycle))
	submission.Status = "approved"
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: tenantID, ProjectID: projectID, SubjectType: "submission_revision", SubjectID: revisionIDV3, Decision: "approved", PreviousState: "submitted", ResultingState: "approved", CreatedAt: now.Add(6 * time.Minute)}
	snapshot := reviewdomain.ApprovedSnapshot{ID: approvedSnapshotID, TenantID: tenantID, ProjectID: projectID, WorkspaceID: submission.WorkspaceID, SubmissionID: submissionID, SubmissionRevisionID: revisionIDV3, SubmissionType: "content_batch", SchemaVersion: "3.0", ContentHash: revision.ContentHash, DecisionID: decision.ID, CreatedAt: now.Add(6 * time.Minute)}
	mustStore(t, store.ApproveSubmissionRevision(ctx, submission, snapshot, decision))
	artifactID := idgen.New()
	mustStore(t, store.CreateArtifact(ctx, deliverydomain.Artifact{ID: artifactID, TenantID: tenantID, ProjectID: projectID, ApprovedSnapshotID: approvedSnapshotID, Kind: "delivery", FileName: "content.xlsx", CreatedAt: now.Add(6 * time.Minute)}))
	batchID, observationID := idgen.New(), idgen.New()
	mustStore(t, store.CreatePerformanceImportBatch(ctx,
		performancedomain.PerformanceImportBatch{ID: batchID, TenantID: tenantID, ProjectID: projectID, SourceName: "douyin.csv", Status: "imported", RowCount: 1, ImportedCount: 1, CreatedAt: now.Add(7 * time.Minute)},
		[]performancedomain.PerformanceObservation{{ID: observationID, TenantID: tenantID, ProjectID: projectID, ImportBatchID: batchID, ApprovedSnapshotID: approvedSnapshotID, Platform: "douyin", AccountAlias: "main", WindowHours: 24, SampleStatus: "insufficient_sample", DedupKey: "lineage-observation", CreatedAt: now.Add(8 * time.Minute)}},
	))
	ratingID := idgen.New()
	mustStore(t, store.CreateRatingDecision(ctx, performancedomain.RatingDecision{ID: ratingID, TenantID: tenantID, ProjectID: projectID, SubjectType: "approved_snapshot", SubjectID: approvedSnapshotID, ObservationIDs: []string{observationID}, Rating: "repairable", NextAction: "创建单变量变体", CreatedAt: now.Add(9 * time.Minute)}))

	graph, err := service.Operations.ProjectLineage(ctx, actor, projectID, application.LineageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source:" + sourceID, "source_revision:" + revisionID, "runtime_run:" + runID, "knowledge_object:" + knowledgeID, "approved_snapshot:" + approvedSnapshotID, "artifact:" + artifactID, "performance_observation:" + observationID, "rating_decision:" + ratingID} {
		if !hasLineageNode(graph, key) {
			t.Fatalf("downstream graph is missing %s: %#v", key, graph.Nodes)
		}
	}
	if graph.FocusKey != "" || len(graph.Edges) == 0 {
		t.Fatalf("unexpected focused graph: %#v", graph)
	}

	upstream, err := service.Operations.ProjectLineage(ctx, actor, projectID, application.LineageQuery{FocusType: "rating_decision", FocusID: ratingID, Direction: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasLineageNode(upstream, "performance_import_batch:"+batchID) || !hasLineageNode(upstream, "performance_observation:"+observationID) {
		t.Fatalf("rating upstream graph lost result import lineage: %#v", upstream.Nodes)
	}

	impact, err := service.Operations.ProjectImpact(ctx, actor, projectID, application.LineageQuery{FocusType: "performance_observation", FocusID: observationID})
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
	service := application.New(application.DependenciesFrom(store), slog.Default())
	project := workspacedomain.Project{ID: idgen.New(), TenantID: idgen.New(), Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	mustStore(t, store.CreateProject(ctx, project))
	actor := application.Actor{TenantID: project.TenantID, UserID: idgen.New(), Role: "viewer"}
	if _, err := service.Operations.ProjectLineage(ctx, actor, project.ID, application.LineageQuery{FocusType: "source"}); err == nil {
		t.Fatal("partial focus must be rejected")
	}
	if _, err := service.Operations.ProjectLineage(ctx, actor, project.ID, application.LineageQuery{FocusType: "source", FocusID: idgen.New()}); err == nil {
		t.Fatal("unknown focused object must be rejected")
	}
	actor.TenantID = idgen.New()
	if _, err := service.Operations.ProjectLineage(ctx, actor, project.ID, application.LineageQuery{}); err == nil {
		t.Fatal("cross-tenant project lineage must be rejected")
	}
}

func mustStore(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func hasLineageNode(graph projection.LineageGraph, key string) bool {
	for _, node := range graph.Nodes {
		if node.Key == key {
			return true
		}
	}
	return false
}

func hasImpactNode(analysis projection.ImpactAnalysis, id, severity string) bool {
	for _, item := range analysis.Items {
		if item.Node.ID == id && item.Severity == severity && item.Reason != "" && item.RecommendedAction != "" {
			return true
		}
	}
	return false
}
