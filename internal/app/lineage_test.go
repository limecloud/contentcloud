package app_test

import (
	"log/slog"
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
	knowledgeID := domain.NewID()
	mustStore(t, store.CreateKnowledge(ctx, domain.KnowledgeItem{ID: knowledgeID, TenantID: tenantID, ProjectID: projectID, Title: "原料事实", Status: "approved", Evidence: []domain.EvidenceRef{{SourceRevisionID: revisionID}}, CreatedAt: now.Add(2 * time.Minute)}))
	briefID := domain.NewID()
	mustStore(t, store.CreateBrief(ctx, domain.BriefVersion{ID: briefID, TenantID: tenantID, ProjectID: projectID, Version: 1, Objective: "验证产品兴趣", Status: "approved", ApprovedKnowledgeIDs: []string{knowledgeID}, CreatedAt: now.Add(3 * time.Minute)}))
	runID := domain.NewID()
	mustStore(t, store.CreateRun(ctx, domain.TaskRun{ID: runID, TenantID: tenantID, ProjectID: projectID, BriefVersionID: briefID, IdempotencyKey: "lineage-run", TaskType: "script_generation", State: "succeeded", CreatedAt: now.Add(4 * time.Minute)}))
	scriptID := domain.NewID()
	script, err := store.CreateScript(ctx,
		domain.Script{ID: domain.NewID(), TenantID: tenantID, ProjectID: projectID, Title: "香文化短片", CreatedAt: now.Add(5 * time.Minute)},
		domain.ScriptVersion{ID: scriptID, TenantID: tenantID, ProjectID: projectID, RunID: runID, Status: "approved", Package: domain.ScriptPackage{Title: "香文化短片", Citations: []domain.Citation{{KnowledgeID: knowledgeID}}}, CreatedAt: now.Add(5 * time.Minute)},
	)
	if err != nil {
		t.Fatal(err)
	}
	artifactID := domain.NewID()
	mustStore(t, store.CreateArtifact(ctx, domain.Artifact{ID: artifactID, TenantID: tenantID, ProjectID: projectID, ScriptVersionID: script.ID, FileName: "script.xlsx", ValidationStatus: "valid", CreatedAt: now.Add(6 * time.Minute)}))
	batchID, observationID := domain.NewID(), domain.NewID()
	mustStore(t, store.CreatePerformanceImportBatch(ctx,
		domain.PerformanceImportBatch{ID: batchID, TenantID: tenantID, ProjectID: projectID, SourceName: "douyin.csv", Status: "imported", RowCount: 1, ImportedCount: 1, CreatedAt: now.Add(7 * time.Minute)},
		[]domain.PerformanceObservation{{ID: observationID, TenantID: tenantID, ProjectID: projectID, ImportBatchID: batchID, ScriptVersionID: script.ID, Platform: "douyin", AccountAlias: "main", WindowHours: 24, SampleStatus: "insufficient_sample", DedupKey: "lineage-observation", CreatedAt: now.Add(8 * time.Minute)}},
	))
	ratingID := domain.NewID()
	mustStore(t, store.CreateRatingDecision(ctx, domain.RatingDecision{ID: ratingID, TenantID: tenantID, ProjectID: projectID, SubjectType: "script_version", SubjectID: script.ID, ObservationIDs: []string{observationID}, Rating: "repairable", NextAction: "创建单变量变体", CreatedAt: now.Add(9 * time.Minute)}))

	graph, err := service.ProjectLineage(ctx, actor, projectID, app.LineageQuery{FocusType: "source", FocusID: sourceID, Direction: "downstream"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source:" + sourceID, "source_revision:" + revisionID, "knowledge_item:" + knowledgeID, "brief_version:" + briefID, "task_run:" + runID, "script_version:" + script.ID, "artifact:" + artifactID, "performance_observation:" + observationID, "rating_decision:" + ratingID} {
		if !hasLineageNode(graph, key) {
			t.Fatalf("downstream graph is missing %s: %#v", key, graph.Nodes)
		}
	}
	if graph.FocusKey != "source:"+sourceID || len(graph.Edges) == 0 {
		t.Fatalf("unexpected focused graph: %#v", graph)
	}

	upstream, err := service.ProjectLineage(ctx, actor, projectID, app.LineageQuery{FocusType: "rating_decision", FocusID: ratingID, Direction: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasLineageNode(upstream, "source:"+sourceID) || !hasLineageNode(upstream, "performance_import_batch:"+batchID) {
		t.Fatalf("rating upstream graph lost source or import lineage: %#v", upstream.Nodes)
	}

	impact, err := service.ProjectImpact(ctx, actor, projectID, app.LineageQuery{FocusType: "source_revision", FocusID: revisionID})
	if err != nil {
		t.Fatal(err)
	}
	if impact.Focus == nil || impact.Focus.ID != revisionID || !hasImpactNode(impact, observationID, "attention") || !hasImpactNode(impact, ratingID, "attention") {
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
