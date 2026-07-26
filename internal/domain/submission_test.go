package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSubmissionBundleHashAndDisclosureValidation(t *testing.T) {
	bundle := SubmissionBundle{
		BundleVersion: "1.0", SchemaVersion: "contentcloud.knowledge/2.0", SubmissionType: "knowledge",
		ProjectID: "project-1", WorkspaceID: "workspace-1", Objects: json.RawMessage(`[ {"id":"fact-1","kind":"fact","status":"verified"} ]`),
		SourceDisclosures: []SourceDisclosure{{SourceRef: "source-1", Level: "evidence_pack", SHA256: strings.Repeat("a", 64), EvidencePack: json.RawMessage(`{"quotes":["verified"]}`)}},
		Artifacts:         []SubmissionArtifact{}, LocalRunSummary: LocalRunSummary{Checks: []LocalRunCheck{}}, IdempotencyKey: "publish-1",
	}
	if err := bundle.SetComputedHash(); err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	bundle.Objects = json.RawMessage(`[{"id":"fact-2"}]`)
	if err := bundle.Validate(); err == nil {
		t.Fatal("tampered objects must fail content hash validation")
	}
}

func TestHighRiskMetadataOnlySubmissionIsEvidenceLimited(t *testing.T) {
	objects := json.RawMessage(`[{"id":"claim-1","risk_level":"high"}]`)
	if !EvidenceLimited(objects, []SourceDisclosure{{SourceRef: "source-1", Level: "metadata_only"}}) {
		t.Fatal("high-risk claim with metadata-only disclosure must be evidence limited")
	}
	if EvidenceLimited(objects, []SourceDisclosure{{SourceRef: "source-1", Level: "evidence_pack"}}) {
		t.Fatal("high-risk claim with evidence pack should pass the coarse disclosure gate")
	}
}
