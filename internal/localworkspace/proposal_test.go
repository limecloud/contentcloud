package localworkspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceProposalAppliesWithOwnershipRevisionAndDigestCAS(t *testing.T) {
	root, run, claim, now := newProposalFixture(t)
	ref := "50-production/draft.md"
	path := filepath.Join(root, filepath.FromSlash(ref))
	before := []byte("# Draft\n\nBefore.\n")
	after := "# Draft\n\nAfter.\n"
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref, RunID: run.RunID, ExpectedContextRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := PrepareWorkspaceProposal(PrepareWorkspaceProposalOptions{
		Root: root, RunID: run.RunID, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID, OwnerEpoch: claim.Epoch,
		ExpectedContextRevision: run.ContextRevision, TypedAction: "workspace_file.replace", Ref: ref,
		ExpectedDigest: view.ObservedDigest, Content: after, Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.OwnerEpoch != claim.Epoch || proposal.BaseFileDigests[0].Digest != view.ObservedDigest || proposal.ValidatedArguments.ContentDigest == view.ObservedDigest {
		t.Fatalf("proposal lost its CAS bindings: %#v", proposal)
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != string(before) {
		t.Fatalf("prepare changed the workspace: body=%q err=%v", current, err)
	}
	applied, err := ApplyWorkspaceProposal(ApplyWorkspaceProposalOptions{
		Root: root, Proposal: proposal, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID,
		OwnerEpoch: claim.Epoch, ExpectedContextRevision: run.ContextRevision, Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.ContextRevision != run.ContextRevision+1 || applied.Outputs[0].Digest != proposal.ValidatedArguments.ContentDigest {
		t.Fatalf("unexpected apply result: %#v", applied)
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != after {
		t.Fatalf("apply did not write the exact proposal body: body=%q err=%v", current, err)
	}
	updated, err := ShowLocalRun(root, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContextRevision != applied.ContextRevision || len(updated.OutputPaths) != 1 || updated.OutputPaths[0] != ref {
		t.Fatalf("apply did not advance the governed LocalRun: %#v", updated)
	}
	if _, err := ApplyWorkspaceProposal(ApplyWorkspaceProposalOptions{
		Root: root, Proposal: proposal, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID,
		OwnerEpoch: claim.Epoch, ExpectedContextRevision: run.ContextRevision, Now: now.Add(3 * time.Minute),
	}); domainCode(err) != "WORKSPACE_PROPOSAL_STALE" {
		t.Fatalf("replayed proposal was not stale: %v", err)
	}
}

func TestWorkspaceProposalRejectsStaleDigestFenceAndExpiry(t *testing.T) {
	root, run, claim, now := newProposalFixture(t)
	ref := "40-work/draft.json"
	path := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.WriteFile(path, []byte(`{"title":"before"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := PrepareWorkspaceProposal(PrepareWorkspaceProposalOptions{
		Root: root, RunID: run.RunID, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID, OwnerEpoch: claim.Epoch,
		ExpectedContextRevision: run.ContextRevision, TypedAction: "workspace_file.replace", Ref: ref,
		ExpectedDigest: view.ObservedDigest, Content: `{"title":"after"}`, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"title":"other"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyWorkspaceProposal(ApplyWorkspaceProposalOptions{
		Root: root, Proposal: proposal, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID,
		OwnerEpoch: claim.Epoch, ExpectedContextRevision: run.ContextRevision, Now: now.Add(time.Minute),
	}); domainCode(err) != "WORKSPACE_PROPOSAL_STALE" {
		t.Fatalf("digest drift was not rejected as stale: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"title":"before"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyWorkspaceProposal(ApplyWorkspaceProposalOptions{
		Root: root, Proposal: proposal, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID,
		OwnerEpoch: claim.Epoch, ExpectedContextRevision: run.ContextRevision, Now: proposal.ExpiresAt,
	}); domainCode(err) != "WORKSPACE_PROPOSAL_STALE" {
		t.Fatalf("expired proposal was accepted: %v", err)
	}
	if _, err := ApplyWorkspaceProposal(ApplyWorkspaceProposalOptions{
		Root: root, Proposal: proposal, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID,
		OwnerEpoch: claim.Epoch + 1, ExpectedContextRevision: run.ContextRevision, Now: now.Add(time.Minute),
	}); domainCode(err) != "WORKSPACE_PROPOSAL_STALE" {
		t.Fatalf("owner epoch drift was accepted: %v", err)
	}
}

func TestWorkspaceProposalRestrictsActionPathAndDocumentType(t *testing.T) {
	root, run, claim, now := newProposalFixture(t)
	write := func(ref, body string) string {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		view, err := BuildWorkspaceView(WorkspaceViewOptions{Root: root, View: "file", Ref: ref, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		return view.ObservedDigest
	}
	tests := []struct {
		name, action, ref, body, content, code string
	}{
		{name: "source is immutable", action: "workspace_file.replace", ref: "20-sources/source.md", body: "before", content: "after", code: "WORKSPACE_PROPOSAL_PATH_DENIED"},
		{name: "run context is governed", action: "workspace_file.replace", ref: "40-work/runs/other/context.json", body: `{}`, content: `{}`, code: "WORKSPACE_PROPOSAL_PATH_DENIED"},
		{name: "unknown action", action: "file.patch", ref: "50-production/draft.md", body: "before", content: "after", code: "WORKSPACE_PROPOSAL_ACTION_INVALID"},
		{name: "invalid json", action: "workspace_file.replace", ref: "50-production/draft.json", body: `{}`, content: `{`, code: "WORKSPACE_PROPOSAL_DOCUMENT_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digest := write(test.ref, test.body)
			_, err := PrepareWorkspaceProposal(PrepareWorkspaceProposalOptions{
				Root: root, RunID: run.RunID, ClaimToken: claim.Token, OwnerKind: claim.OwnerKind, OwnerID: claim.OwnerID, OwnerEpoch: claim.Epoch,
				ExpectedContextRevision: run.ContextRevision, TypedAction: test.action, Ref: test.ref,
				ExpectedDigest: digest, Content: test.content, Now: now,
			})
			if domainCode(err) != test.code {
				t.Fatalf("unexpected error: got=%s want=%s err=%v", domainCode(err), test.code, err)
			}
		})
	}
}

func newProposalFixture(t *testing.T) (string, LocalRunContext, RunClaim, time.Time) {
	t.Helper()
	root := newCoordinationWorkspace(t)
	now := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: "run-proposal", Intent: "intent:content", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := ClaimRun(ClaimRunOptions{Root: root, RunID: run.RunID, OwnerKind: "browser", OwnerID: "wbk-proposal", ExpectedRevision: run.ContextRevision, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return root, run, claim, now
}
