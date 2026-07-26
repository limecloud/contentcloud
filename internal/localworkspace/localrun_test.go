package localworkspace

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLocalRunEnforcesKnowledgeAndContentGates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "local-run-1", Intent: "content", SourceRefs: []string{"source:product"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if run.Stage != "knowledge-lint" {
		t.Fatalf("unexpected initial stage: %+v", run)
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "query", RecordLocalRunOptions{}, now); err == nil {
		t.Fatal("knowledge-lint must require kb-lint")
	}
	if _, err := CheckLocalRun(CheckLocalRunOptions{Root: root, RunID: run.RunID, Name: "kb-lint", Status: "passed", Now: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "query", RecordLocalRunOptions{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "compile", RecordLocalRunOptions{}, now); err == nil {
		t.Fatal("query must record an eligible or blocked result")
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "compile", RecordLocalRunOptions{BlockedIDs: []string{"claim:high-risk"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "output-lint", RecordLocalRunOptions{}, now); err == nil {
		t.Fatal("compile must record an output path")
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "output-lint", RecordLocalRunOptions{OutputPaths: []string{"outputs/scripts/draft.json"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := AdvanceLocalRun(root, run.RunID, "done", RecordLocalRunOptions{}, now); err == nil {
		t.Fatal("output-lint must require content-lint")
	}
	if _, err := CheckLocalRun(CheckLocalRunOptions{Root: root, RunID: run.RunID, Name: "content-lint", Status: "passed", Now: now}); err != nil {
		t.Fatal(err)
	}
	completed, err := AdvanceLocalRun(root, run.RunID, "done", RecordLocalRunOptions{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.Stage != "done" {
		t.Fatalf("unexpected completed run: %+v", completed)
	}
	report, err := ValidateLocalRuns(root)
	if err != nil || !report.Valid || report.RunCount != 1 || report.CurrentRun != run.RunID {
		t.Fatalf("unexpected validation: %+v %v", report, err)
	}
}

func TestFailedLocalRunCanResume(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "local-run-failure", Intent: "query"})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := FailLocalRun(root, run.RunID, []string{"来源冲突"}, time.Time{})
	if err != nil || failed.Status != "failed" {
		t.Fatalf("fail run: %+v %v", failed, err)
	}
	resumed, err := ResumeLocalRun(root, run.RunID, time.Time{})
	if err != nil || resumed.Status != "in_progress" {
		t.Fatalf("resume run: %+v %v", resumed, err)
	}
}
