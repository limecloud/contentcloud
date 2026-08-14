package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestRunClaimIsSingleWriterAndExpiredTakeoverIsExplicit(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 4, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-claim", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-a", ExpectedRevision: run.ContextRevision, TTL: time.Minute, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == "" || first.ContextRevision != run.ContextRevision {
		t.Fatalf("unexpected first claim: %+v", first)
	}
	var persisted map[string]any
	if err := readJSON(runClaimPath(root, run.RunID), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["token"] != nil || persisted["token_hash"] != first.TokenHash || persisted["epoch"] != float64(first.Epoch) {
		body, _ := json.Marshal(persisted)
		t.Fatalf("claim persistence exposed plaintext token or lost fencing state: %s", body)
	}
	if _, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-b", ExpectedRevision: run.ContextRevision, TTL: time.Minute, Now: now}); domainCode(err) != "RUN_ALREADY_CLAIMED" {
		t.Fatalf("expected active claim conflict, got %v", err)
	}
	if _, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-b", ExpectedRevision: run.ContextRevision, TTL: time.Minute, Now: now.Add(2 * time.Minute)}); domainCode(err) != "RUN_CLAIM_TAKEOVER_CONFIRMATION_REQUIRED" {
		t.Fatalf("expected takeover confirmation, got %v", err)
	}
	second, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-b", ExpectedRevision: run.ContextRevision, TTL: time.Minute, TakeoverExpired: true, Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token || second.OwnerID != "conversation-b" || second.Epoch <= first.Epoch {
		t.Fatalf("unexpected takeover claim: %+v", second)
	}
}

func TestRunClaimActiveTakeoverFencesPreviousOwner(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 8, 14, 7, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-fence", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	browser, err := TakeoverRunClaim(TakeoverRunClaimOptions{
		Root: root, RunID: run.RunID, OwnerKind: "browser", OwnerID: "wbk-a",
		ExpectedOwnerKind: agent.OwnerKind, ExpectedOwnerID: agent.OwnerID, ExpectedEpoch: agent.Epoch,
		ExpectedRevision: run.ContextRevision, Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if browser.OwnerKind != "browser" || browser.OwnerID != "wbk-a" || browser.Epoch <= agent.Epoch || browser.Token == "" {
		t.Fatalf("unexpected browser takeover: %#v", browser)
	}
	if _, err := ValidateRunOwnership(root, run.RunID, agent.Token, agent.OwnerKind, agent.OwnerID, agent.Epoch, run.ContextRevision, now.Add(2*time.Minute)); domainCode(err) != "RUN_CLAIM_TOKEN_INVALID" {
		t.Fatalf("previous owner token was not fenced: %v", err)
	}
	if _, err := ValidateRunOwnership(root, run.RunID, browser.Token, browser.OwnerKind, browser.OwnerID, browser.Epoch, run.ContextRevision, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("new browser owner could not validate its lease: %v", err)
	}
	if _, err := TakeoverRunClaim(TakeoverRunClaimOptions{
		Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-b",
		ExpectedOwnerKind: agent.OwnerKind, ExpectedOwnerID: agent.OwnerID, ExpectedEpoch: agent.Epoch,
		ExpectedRevision: run.ContextRevision, Now: now.Add(2 * time.Minute),
	}); domainCode(err) != "RUN_CLAIM_FENCE_CONFLICT" {
		t.Fatalf("stale takeover was not fenced: %v", err)
	}
	if err := ReleaseRunClaim(root, run.RunID, browser.Token, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	next, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-c", ExpectedRevision: run.ContextRevision, Now: now.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if next.Epoch <= browser.Epoch {
		t.Fatalf("epoch did not remain monotonic after release: browser=%d next=%d", browser.Epoch, next.Epoch)
	}
}

func TestLocalRunSaveRejectsStaleRevision(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 4, 30, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-cas", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	first := run
	second := run
	first.Findings = append(first.Findings, "first")
	if _, err := saveLocalRun(root, first, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	second.Findings = append(second.Findings, "second")
	if _, err := saveLocalRun(root, second, now.Add(2*time.Second)); domainCode(err) != "LOCAL_RUN_REVISION_CONFLICT" {
		t.Fatalf("expected stale revision conflict, got %v", err)
	}
	persisted, err := ShowLocalRun(root, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ContextRevision != 2 || len(persisted.Findings) != 1 || persisted.Findings[0] != "first" {
		t.Fatalf("stale write changed persisted context: %+v", persisted)
	}
}

func TestClaimedLocalRunWriteRequiresTokenAndCurrentRevision(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 4, 45, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-guarded", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordClaimedLocalRun(RecordLocalRunOptions{Root: root, RunID: run.RunID, ExpectedRevision: run.ContextRevision, Findings: []string{"no token"}, Now: now}); domainCode(err) != "RUN_CLAIM_REQUIRED" {
		t.Fatalf("expected claim requirement, got %v", err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := RecordClaimedLocalRun(RecordLocalRunOptions{Root: root, RunID: run.RunID, ClaimToken: claim.Token, ExpectedRevision: run.ContextRevision, Findings: []string{"guarded"}, Now: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContextRevision != run.ContextRevision+1 {
		t.Fatalf("revision did not advance: %+v", updated)
	}
	if _, err := RecordClaimedLocalRun(RecordLocalRunOptions{Root: root, RunID: run.RunID, ClaimToken: claim.Token, ExpectedRevision: run.ContextRevision, Findings: []string{"stale"}, Now: now.Add(2 * time.Second)}); domainCode(err) != "LOCAL_RUN_REVISION_CONFLICT" {
		t.Fatalf("expected stale revision rejection, got %v", err)
	}
	renewed, err := RenewRunClaim(root, run.RunID, claim.Token, time.Minute, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if renewed.ContextRevision != updated.ContextRevision {
		t.Fatalf("claim revision not synchronized: %+v", renewed)
	}
}

func TestWorkspaceSessionBindingBlocksComponentDrift(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 4, 50, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-session-drift", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-generation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(root, ".contentcloud", "template.lock")
	var lock TemplateLock
	if err := readJSON(templatePath, &lock); err != nil {
		t.Fatal(err)
	}
	lock.MCPServers = append(lock.MCPServers, InstalledComponent{Name: "new-runtime-tool", Version: "2"})
	if err := replaceJSON(templatePath, lock, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = RecordClaimedLocalRun(RecordLocalRunOptions{Root: root, RunID: run.RunID, ClaimToken: claim.Token, ExpectedRevision: run.ContextRevision, Findings: []string{"must not persist"}, Now: now.Add(time.Second)})
	if domainCode(err) != "WORKSPACE_SESSION_COMPONENTS_CHANGED" {
		t.Fatalf("component drift was not blocked: %v", err)
	}
	persisted, showErr := ShowLocalRun(root, run.RunID)
	if showErr != nil || len(persisted.Findings) != 0 {
		t.Fatalf("drifted session changed local run: run=%+v err=%v", persisted, showErr)
	}
}

func TestWorkspaceSessionBindingBlocksPluginHostReceiptDrift(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 4, 55, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-plugin-receipt-drift", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-plugin-generation", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := os.Getenv("CONTENTCLOUD_PLUGIN_STORE")
	receiptDirectory := filepath.Join(storeRoot, "receipts", "codex")
	if err := os.MkdirAll(receiptDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receiptDirectory, "contentcloud-video-production.json"), []byte("{\"release\":{\"digest\":\"sha256:changed\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = RecordClaimedLocalRun(RecordLocalRunOptions{Root: root, RunID: run.RunID, ClaimToken: claim.Token, ExpectedRevision: run.ContextRevision, Findings: []string{"must not persist"}, Now: now.Add(time.Second)})
	if domainCode(err) != "WORKSPACE_SESSION_COMPONENTS_CHANGED" {
		t.Fatalf("Plugin Host receipt drift was not blocked: %v", err)
	}
}

func TestHandoffAcceptIsAtomicAcrossConversations(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 5, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-handoff", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(root, "50-production", "scripts", "checkpoint.json")
	if err := os.WriteFile(inputPath, []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := CreateReadyHandoff(CreateReadyHandoffOptions{
		Root:             root,
		HandoffID:        "handoff-1",
		RunID:            run.RunID,
		ClaimToken:       claim.Token,
		ExpectedRevision: run.ContextRevision,
		NextCapabilityID: "contentcloud.marketing-video-script",
		NextAction:       "继续生成逐镜头剧本",
		InputPaths:       []string{"50-production/scripts/checkpoint.json"},
		Now:              now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextBefore, err := ConversationContext(root, "", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(contextBefore.ReadyHandoffs) != 1 || contextBefore.ReadyHandoffs[0].HandoffID != handoff.HandoffID {
		t.Fatalf("ready handoff missing from context: %+v", contextBefore.ReadyHandoffs)
	}
	owners := []string{"conversation-b", "conversation-c"}
	start := make(chan struct{})
	results := make(chan error, len(owners))
	var wait sync.WaitGroup
	for _, owner := range owners {
		owner := owner
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, _, acceptErr := AcceptHandoff(AcceptHandoffOptions{Root: root, HandoffID: handoff.HandoffID, OwnerKind: "agent", OwnerID: owner, Now: now.Add(2 * time.Minute)})
			results <- acceptErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one handoff accept, got %d", successes)
	}
	contextAfter, err := ConversationContext(root, "", now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(contextAfter.ReadyHandoffs) != 0 || !contextAfter.ActiveRuns[0].Claim.Claimed {
		t.Fatalf("unexpected context after accept: %+v", contextAfter)
	}
}

func TestHandoffRejectsChangedInputDigest(t *testing.T) {
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 7, 27, 6, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-digest", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(root, "50-production", "scripts", "checkpoint.json")
	if err := os.WriteFile(inputPath, []byte("{\"version\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "agent", OwnerID: "conversation-a", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := CreateReadyHandoff(CreateReadyHandoffOptions{Root: root, HandoffID: "handoff-digest", RunID: run.RunID, ClaimToken: claim.Token, ExpectedRevision: run.ContextRevision, NextCapabilityID: "contentcloud.marketing-video-script", NextAction: "continue", InputPaths: []string{"50-production/scripts/checkpoint.json"}, Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, []byte("{\"version\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AcceptHandoff(AcceptHandoffOptions{Root: root, HandoffID: handoff.HandoffID, OwnerKind: "agent", OwnerID: "conversation-b", Now: now.Add(2 * time.Minute)}); domainCode(err) != "HANDOFF_INPUT_DIGEST_MISMATCH" {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
	status, err := RunClaimStatus(root, run.RunID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if status.Claimed {
		t.Fatalf("digest mismatch must not leave a claim: %+v", status)
	}
}

func newCoordinationWorkspace(t *testing.T) string {
	t.Helper()
	t.Setenv("CONTENTCLOUD_PLUGIN_STORE", t.TempDir())
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func domainCode(err error) string {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}
