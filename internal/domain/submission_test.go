package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSubmissionBundleHashAndDisclosureValidation(t *testing.T) {
	object := submissionObject(t, "fact-1", "Fact", "30-knowledge/pages/facts/fact-1.json", map[string]any{"id": "fact-1", "kind": "fact", "status": "verified"})
	bundle := SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: "project-1", WorkspaceID: "workspace-1", BaseSnapshotIDs: []string{}, Objects: []SubmissionObjectRef{object}, EnvironmentDigest: "sha256:" + strings.Repeat("e", 64),
		SourceDisclosures: []SourceDisclosure{{SourceRef: "source-1", Level: "evidence_pack", SHA256: strings.Repeat("a", 64), EvidencePack: json.RawMessage(`{"quotes":["verified"]}`)}},
		Artifacts:         []SubmissionArtifact{}, LocalRunSummary: LocalRunSummary{Checks: []LocalRunCheck{}}, IdempotencyKey: "publish-1",
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	bundle.Objects[0].Content = json.RawMessage(`{"id":"fact-2"}`)
	if err := bundle.Validate(); err == nil {
		t.Fatal("tampered objects must fail content hash validation")
	}
}

func TestHighRiskMetadataOnlySubmissionIsEvidenceLimited(t *testing.T) {
	objects := []SubmissionObjectRef{submissionObject(t, "claim-1", "Claim", "30-knowledge/pages/claims/claim-1.json", map[string]any{"id": "claim-1", "risk_level": "high"})}
	if !EvidenceLimited(objects, []SourceDisclosure{{SourceRef: "source-1", Level: "metadata_only"}}) {
		t.Fatal("high-risk claim with metadata-only disclosure must be evidence limited")
	}
	if EvidenceLimited(objects, []SourceDisclosure{{SourceRef: "source-1", Level: "evidence_pack"}}) {
		t.Fatal("high-risk claim with evidence pack should pass the coarse disclosure gate")
	}
}

func TestMetadataAndFullSourceDisclosureRejectEvidencePackBody(t *testing.T) {
	for _, level := range []string{"metadata_only", "full_source"} {
		t.Run(level, func(t *testing.T) {
			bundle := SubmissionBundle{
				BundleVersion: "3.0", SubmissionType: "knowledge", ProjectID: "project-1", WorkspaceID: "workspace-1", BaseSnapshotIDs: []string{},
				Objects:           []SubmissionObjectRef{submissionObject(t, "fact-1", "Fact", "30-knowledge/pages/facts/fact-1.json", map[string]any{"id": "fact-1", "status": "verified"})},
				SourceDisclosures: []SourceDisclosure{{SourceRef: "source-1", Level: level, SHA256: strings.Repeat("a", 64), EvidencePack: json.RawMessage(`{"quote":"must-not-cross-boundary"}`)}},
				EnvironmentDigest: "sha256:" + strings.Repeat("e", 64), Artifacts: []SubmissionArtifact{}, LocalRunSummary: LocalRunSummary{Checks: []LocalRunCheck{}}, IdempotencyKey: "publish-1",
			}
			if err := bundle.SetComputedHash(); err != nil {
				t.Fatal(err)
			}
			err := bundle.Validate()
			if err == nil || !strings.Contains(err.Error(), "只有 evidence_pack") {
				t.Fatalf("%s disclosure accepted evidence pack body: %v", level, err)
			}
		})
	}
}

func TestApprovedRevisionMakesNonBlockedCandidatesEligible(t *testing.T) {
	revision := SubmissionRevision{Objects: []SubmissionObjectRef{
		submissionObject(t, "fact-candidate", "Fact", "facts/fact-candidate.json", map[string]any{"id": "fact-candidate", "status": "candidate"}),
		submissionObject(t, "claim-review-ready", "Claim", "claims/claim-review-ready.json", map[string]any{"id": "claim-review-ready", "status": "review_ready"}),
		submissionObject(t, "claim-blocked", "Claim", "claims/claim-blocked.json", map[string]any{"id": "claim-blocked", "status": "blocked"}),
		submissionObject(t, "manifest", "Manifest", "manifest.json", map[string]any{"id": "manifest", "status": "informational"}),
	}}
	ids := revision.EligibleObjectIDs()
	if len(ids) != 2 || ids[0] != "claim-review-ready" || ids[1] != "fact-candidate" {
		t.Fatalf("unexpected eligible IDs: %#v", ids)
	}
}

func submissionObject(t *testing.T, id, objectType, path string, content any) SubmissionObjectRef {
	t.Helper()
	value, err := NewSubmissionObjectRef(id, objectType, 1, path, content)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
