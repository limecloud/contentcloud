package localworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/capabilityrouting"
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

func TestPlanRecognizesReservedButUnavailableClient(t *testing.T) {
	_, err := Plan(t.TempDir(), "cursor")
	if err == nil || !strings.Contains(err.Error(), "尚未提供") {
		t.Fatalf("reserved client must fail with an explicit capability error: %v", err)
	}
}

func TestInitializeCreatesLocalFirstWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	status, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", DeviceID: "device-1", ServerURL: "https://content.example/", CLIVersion: "test", Target: "codex-plugin", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Initialized || status.Binding.ProjectID != "project-1" || status.Binding.ServerURL != "https://content.example" {
		t.Fatalf("unexpected status: %+v", status)
	}
	for _, path := range []string{
		".contentcloud/workspace.yaml",
		".contentcloud/template.lock",
		".contentcloud/sync-state.json",
		"10-context/methodology.yaml",
		"20-sources/registry.yaml",
		"30-knowledge/schema/knowledge-page-3.0.schema.json",
		"30-knowledge/schema/local-run-3.0.schema.json",
		"40-work/focus.md",
		"workflows/knowledge-to-content.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	if info, err := os.Stat(filepath.Join(root, "60-delivery", "packages")); err != nil || !info.IsDir() {
		t.Fatalf("expected 60-delivery/packages directory: %v", err)
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

func TestInitializeCodexPluginUsesPluginDeliveryWithoutProjectDuplicates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	status, err := Initialize(InitOptions{
		Root:       root,
		ProjectID:  "project-1",
		ServerURL:  "https://content.example",
		CLIVersion: "test",
		Target:     "codex-plugin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Template.Targets) != 1 || status.Template.Targets[0] != "codex-plugin" {
		t.Fatalf("unexpected targets: %v", status.Template.Targets)
	}
	for _, path := range []string{
		".contentcloud/mcp/contentcloud-local.json",
		".contentcloud/workspace.yaml",
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	for _, path := range []string{".agents", ".codex", ".contentcloud/skills", ".mcp.json", ".claude"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatalf("plugin delivery must not create %s: %v", path, err)
		}
	}
	report, err := Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || !report.Checks["skills"].OK || !report.Checks["mcp"].OK {
		t.Fatalf("unexpected plugin delivery doctor report: %+v", report)
	}
}

func TestInitializeCodexMCPUsesPinnedNPXLauncher(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", CLIVersion: "0.25.0", Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".contentcloud/mcp/contentcloud-local.json", ".codex/config.toml"} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"command": "npx"`) && !strings.Contains(string(body), `command = "npx"`) {
			t.Fatalf("%s does not use npx: %s", path, body)
		}
		if !strings.Contains(string(body), "@limecloud/contentcloud@0.25.0") || !strings.Contains(string(body), "mcp") || !strings.Contains(string(body), "serve") {
			t.Fatalf("%s does not pin the MCP launcher: %s", path, body)
		}
	}
}

func TestPlanRejectsUnknownTarget(t *testing.T) {
	_, err := Plan(filepath.Join(t.TempDir(), "project"), "unknown")
	if err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestCapabilityRoutingUpdatePreservesUserAgentsContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", CLIVersion: "test", Target: "codex-plugin"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "AGENTS.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("\n# 用户规则\n\n保留正文。\n")...)
	body = []byte(strings.Replace(string(body), "version="+capabilityrouting.Version, "version=0.0.0", 1))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || report.Checks["capability_routing"].OK {
		t.Fatalf("doctor must report outdated routing: %+v", report)
	}
	inspection, err := UpdateCapabilityRouting(root)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Status != "current" {
		t.Fatalf("unexpected routing status: %+v", inspection)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "# 用户规则\n\n保留正文。") {
		t.Fatalf("routing update removed user content:\n%s", updated)
	}
	report, err = Doctor(root)
	if err != nil || !report.OK {
		t.Fatalf("doctor must pass after routing repair: err=%v report=%+v", err, report)
	}
}

func TestStatusDetectsManagedFileChangesFromNestedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", ServerURL: "http://localhost:8080", CLIVersion: "test", Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "30-knowledge", "schema", "knowledge-page-3.0.schema.json")
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := LoadStatus(filepath.Join(root, "30-knowledge", "pages", "facts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(status.ModifiedManagedFiles) != 1 || status.ModifiedManagedFiles[0] != "30-knowledge/schema/knowledge-page-3.0.schema.json" {
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
