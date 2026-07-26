package localworkspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanRejectsNonEmptyUnknownDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "customer-material.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(root, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "non_empty" || len(plan.Conflicts) != 1 || plan.Conflicts[0] != "customer-material.txt" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.WouldUpload || plan.WouldDaemon {
		t.Fatalf("init must not upload or enable daemon: %+v", plan)
	}
}

func TestInitializeCreatesLocalFirstWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	status, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", DeviceID: "device-1", ServerURL: "https://content.example/", CLIVersion: "test", Target: "all", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || status.Binding.ProjectID != "project-1" || status.Binding.ServerURL != "https://content.example" {
		t.Fatalf("unexpected status: %+v", status)
	}
	for _, path := range []string{
		".contentcloud/project.yaml",
		".contentcloud/template.lock",
		".contentcloud/sync-state.json",
		".contentcloud/skills/contentcloud-knowledge-extraction/SKILL.md",
		".agents/skills/contentcloud-marketing-video-script/SKILL.md",
		".claude/skills/contentcloud-marketing-video-script/SKILL.md",
		".codex/config.toml",
		".mcp.json",
		"raw/.gitignore",
		"raw/source-registry.yaml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	report, err := Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || !report.Checks["automation"].OK || report.Checks["automation"].Required {
		t.Fatalf("unexpected doctor report: %+v", report)
	}
	second, err := Initialize(InitOptions{Root: root, ProjectID: "must-not-rebind", Target: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Binding.ProjectID != "project-1" {
		t.Fatalf("idempotent init changed binding: %+v", second.Binding)
	}
}

func TestStatusDetectsManagedFileChangesFromNestedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ontology", "classes.yaml")
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := LoadStatus(filepath.Join(root, "knowledge", "facts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(status.ModifiedManagedFiles) != 1 || status.ModifiedManagedFiles[0] != "ontology/classes.yaml" {
		t.Fatalf("unexpected modified files: %v", status.ModifiedManagedFiles)
	}
	report, err := Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Checks["managed_files"].OK {
		t.Fatalf("doctor should reject modified generated ontology: %+v", report)
	}
}
