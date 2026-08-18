package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

func TestRealMaterialReachesKnowledgePublishPreflight(t *testing.T) {
	material := filepath.Clean(os.Getenv("CONTENTCLOUD_REAL_MATERIAL_FILE"))
	if material == "." {
		t.Skip("set CONTENTCLOUD_REAL_MATERIAL_FILE to an exact local source file for the opt-in integration test")
	}
	if _, err := os.Stat(material); err != nil {
		t.Fatalf("real material is unavailable: %v", err)
	}
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "real-material-project", WorkspaceID: "real-material-workspace", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: root, File: material, ID: "source:real-material", Title: filepath.Base(material), SourceKind: "customer_material", StorageMode: "copy"}); err != nil {
		t.Fatal(err)
	}
	bundle, err := localworkspace.IngestLocalSource(root, "source:real-material", zeroTime())
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Status != "ready" || len(bundle.Evidence) == 0 {
		t.Fatalf("real DOCX did not produce accepted evidence: %+v", bundle)
	}
	span := bundle.Evidence[0]
	for _, candidate := range bundle.Evidence {
		if len(candidate.Quote) < 4000 {
			span = candidate
			break
		}
	}
	locator, _ := json.Marshal(span.Locator)
	pkg := sourcedomain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []sourcedomain.KnowledgeCandidate{{
		Kind: "fact", Title: "真实材料事实", Statement: span.Quote, Subject: "客户材料", Predicate: "包含内容", Value: sourcedomain.TypedValue{Type: "text", Text: span.Quote},
		Scope: sourcedomain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{},
		Evidence: []sourcedomain.EvidenceRef{{SourceRevisionID: "source:real-material", LocatorKind: span.LocatorKind, Locator: string(locator), Quote: span.Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	packagePath := filepath.Join(root, "40-work", "real-material-candidates.json")
	if err := os.WriteFile(packagePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: root, PackageFile: "40-work/real-material-candidates.json", OriginRunID: "real-material-run"}); err != nil {
		t.Fatal(err)
	}
	lint, err := localworkspace.LintKnowledge(root)
	if err != nil || !lint.Valid {
		t.Fatalf("real-material knowledge lint failed: %+v %v", lint, err)
	}
	diagnosis, err := localworkspace.DiagnoseKnowledge(root, "douyin", zeroTime())
	if err != nil || diagnosis.NeedsReview+diagnosis.Covered == 0 {
		t.Fatalf("real-material diagnosis failed: %+v %v", diagnosis, err)
	}
	pack, err := localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: root, Name: "真实材料知识包"})
	if err != nil {
		t.Fatal(err)
	}
	_, preflight, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: "knowledge", Files: []string{pack.PackPath}, DisclosuresFile: pack.DisclosuresPath})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.ObjectCount != 2 || preflight.DisclosureCount["evidence_pack"] != 1 || preflight.RawFilesUpload {
		t.Fatalf("unexpected real-material preflight: %+v", preflight)
	}
}
