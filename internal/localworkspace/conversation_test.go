package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestResolveWorkspaceRootPrefersExplicitDirectory(t *testing.T) {
	base := t.TempDir()
	explicit := filepath.Join(base, "explicit")
	cwdWorkspace := filepath.Join(base, "cwd")
	for _, root := range []string{explicit, cwdWorkspace} {
		if _, err := Initialize(InitOptions{Root: root, ProjectID: filepath.Base(root), Target: "none", CLIVersion: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	resolution, err := ResolveWorkspaceRoot(filepath.Join(explicit, "work", "runs"), cwdWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Root != canonicalPath(t, explicit) || resolution.Source != "explicit_directory" {
		t.Fatalf("unexpected resolution: %+v", resolution)
	}
}

func TestResolveWorkspaceRootUsesCWDWithoutScanningChildren(t *testing.T) {
	base := t.TempDir()
	workspace := filepath.Join(base, "workspace")
	if _, err := Initialize(InitOptions{Root: workspace, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	resolution, err := ResolveWorkspaceRoot("", filepath.Join(workspace, "outputs", "scripts"))
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Root != canonicalPath(t, workspace) || resolution.Source != "process_cwd" {
		t.Fatalf("unexpected resolution: %+v", resolution)
	}
	second := filepath.Join(base, "second")
	if _, err := Initialize(InitOptions{Root: second, ProjectID: "project-2", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveWorkspaceRoot("", base); err == nil {
		t.Fatal("parent directory must not scan and guess among child workspaces")
	}
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestConversationContextReadsPersistedOfflineState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC)
	status, err := Initialize(InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-1", Intent: "content", Now: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(map[string]any{"schema_version": "contentcloud.knowledge/2.0", "submission_type": "knowledge", "objects": []map[string]any{{"id": "fact-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domain.ApprovedSnapshot{ID: "snapshot-1", SubmissionType: "knowledge", SchemaVersion: "contentcloud.knowledge/2.0", CanonicalContent: canonical, EligibleIDs: []string{"fact-1"}, CreatedAt: now.Add(2 * time.Minute)}
	if _, err := StoreApprovedSnapshot(root, snapshot, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := StorePulledBundle(root, "decisions", "decision-1", map[string]any{"id": "decision-1"}, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".contentcloud", "inbox", "review-feedback", "feedback-1.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	context, err := ConversationContext("", root, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if context.WorkspaceID != status.Binding.WorkspaceID || context.ProjectID != "project-1" || context.EnvironmentHealth != "ready" || !context.Offline {
		t.Fatalf("unexpected context: %+v", context)
	}
	if len(context.ActiveRuns) != 1 || context.ActiveRuns[0].RunID != "run-1" {
		t.Fatalf("unexpected active runs: %+v", context.ActiveRuns)
	}
	if len(context.ReadyHandoffs) != 0 || len(context.PendingLocalDecisions) != 1 || context.PendingLocalDecisions[0] != "decision-1" {
		t.Fatalf("unexpected handoffs or decisions: %+v", context)
	}
	if len(context.CachedApprovedInputs) != 1 || context.CachedApprovedInputs[0] != "snapshot-1" || context.ReviewInboxCount != 1 {
		t.Fatalf("unexpected cached state: %+v", context)
	}
	if context.LastCloudPullAt == nil || !context.LastCloudPullAt.Equal(now.Add(3*time.Minute)) {
		t.Fatalf("unexpected last pull time: %v", context.LastCloudPullAt)
	}
}

func TestConversationContextCarriesBootstrapHandoffUntilWorkStarts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC)
	if _, err := Initialize(InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "0.6.0", Now: now}); err != nil {
		t.Fatal(err)
	}
	handoff, path, err := StoreBootstrapHandoff(root, "contentcloud-video-production@contentcloud", "0.6.0", "v0.6.0", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if handoff.EnvironmentDigest == "" || filepath.Base(path) != "bootstrap-handoff.json" {
		t.Fatalf("unexpected bootstrap handoff: %#v path=%s", handoff, path)
	}
	context, err := ConversationContext(root, "", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if context.BootstrapHandoff == nil || context.BootstrapHandoff.PluginID != handoff.PluginID || len(context.SuggestedIntents) == 0 || context.SuggestedIntents[0] != "bootstrap_continue" {
		t.Fatalf("bootstrap handoff missing from initial context: %#v", context)
	}
	if _, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-after-bootstrap", Intent: "content", Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	context, err = ConversationContext(root, "", now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if context.BootstrapHandoff != nil {
		t.Fatalf("bootstrap handoff must not shadow active work: %#v", context.BootstrapHandoff)
	}
}
