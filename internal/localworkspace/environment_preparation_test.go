package localworkspace

import (
	"testing"
	"time"
)

func TestEnvironmentPreparationAndRunClaimAreMutuallyExclusive(t *testing.T) {
	now := time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test", Now: now}); err != nil {
		t.Fatal(err)
	}
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-preparation", Intent: "content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, Owner: "conversation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BeginEnvironmentPreparation(root, "epp_test", now.Add(time.Minute)); domainCode(err) != "ENVIRONMENT_PREPARATION_RUN_ACTIVE" {
		t.Fatalf("active RunClaim preparation error = %#v", err)
	}
	if err := ReleaseRunClaim(root, run.RunID, claim.Token, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	lease, err := BeginEnvironmentPreparation(root, "epp_test", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, Owner: "conversation-b", ExpectedRevision: run.ContextRevision, Now: now.Add(3 * time.Minute)}); domainCode(err) != "ENVIRONMENT_PREPARATION_IN_PROGRESS" {
		t.Fatalf("claim during preparation error = %#v", err)
	}
	if _, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, Owner: "conversation-b", ExpectedRevision: run.ContextRevision, Now: now.Add(13 * time.Minute)}); domainCode(err) != "ENVIRONMENT_PREPARATION_IN_PROGRESS" {
		t.Fatalf("claim after preparation lease expiry error = %#v", err)
	}
	if err := FinishEnvironmentPreparation(root, "wrong"); domainCode(err) != "ENVIRONMENT_PREPARATION_TOKEN_INVALID" {
		t.Fatalf("wrong preparation token error = %#v", err)
	}
	if err := FinishEnvironmentPreparation(root, lease.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, Owner: "conversation-b", ExpectedRevision: run.ContextRevision, Now: now.Add(4 * time.Minute)}); err != nil {
		t.Fatalf("claim after preparation: %v", err)
	}
}
