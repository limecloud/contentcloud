package runtime

import (
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestProjectorPersistsExplorerAfterOutboxClaim(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("projector-task", domain.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	projector := NewProjector(repo, time.Now)
	result, err := projector.RunOnce(t.Context(), "tenant-1", "runtime", "projector-1", time.Minute, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Projected == 0 || result.Retried != 0 {
		t.Fatalf("unexpected projection result: %#v", result)
	}
	view, err := service.RuntimeExplorer(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Job.ID != started.Job.ID || view.LastEventSeq == 0 || len(view.Nodes) != len(started.Nodes) {
		t.Fatalf("unexpected explorer view: %#v", view)
	}
	stats, err := service.RuntimeProjectionStats(t.Context(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pending != 0 {
		t.Fatalf("projected outbox messages remain pending: %#v", stats)
	}
}

func TestProjectorAcksOutboxWhenProjectionIsAlreadyAhead(t *testing.T) {
	repo := memory.New()
	service := New(repo, time.Now)
	started, err := service.Start(t.Context(), testStartInput("projector-stale-task", domain.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	job, err := repo.JobRun(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := repo.NodeRuns(t.Context(), "tenant-1", started.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectedAt := time.Now().UTC()
	if err := repo.SaveRuntimeExplorer(t.Context(), domain.RuntimeExplorerView{TenantID: "tenant-1", JobRunID: started.Job.ID, Job: job, Nodes: nodes, LastEventSeq: 999, SourceEventID: "newer-event", ProjectedAt: projectedAt}); err != nil {
		t.Fatal(err)
	}
	result, err := NewProjector(repo, time.Now).RunOnce(t.Context(), "tenant-1", "runtime", "projector-stale", time.Minute, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Projected != result.Claimed || result.Claimed == 0 || result.Retried != 0 {
		t.Fatalf("stale projection should be idempotently acknowledged: %#v", result)
	}
	stats, err := service.RuntimeProjectionStats(t.Context(), "tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Pending != 0 {
		t.Fatalf("stale outbox message must not remain pending: %#v", stats)
	}
}
