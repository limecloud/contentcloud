package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func TestJinlingMaterialReachesKnowledgePublishPreflight(t *testing.T) {
	jinlingRoot := os.Getenv("CONTENTCLOUD_JINLING_ROOT")
	if jinlingRoot == "" {
		t.Skip("set CONTENTCLOUD_JINLING_ROOT to the jinling-gudu workspace for the real-material integration test")
	}
	material := filepath.Clean(filepath.Join(jinlingRoot, "..", "金陵古都香线香_1款", "研发", "产品立项文件", "古都香十五维度分析.docx"))
	if _, err := os.Stat(material); err != nil {
		t.Fatalf("jinling material is unavailable: %v", err)
	}
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, ProjectID: "jinling-project", WorkspaceID: "jinling-workspace", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: root, File: material, ID: "source:product-15-dimensions", Title: "古都香十五维度分析", SourceKind: "product_analysis", StorageMode: "copy"}); err != nil {
		t.Fatal(err)
	}
	bundle, err := localworkspace.IngestLocalSource(root, "source:product-15-dimensions", zeroTime())
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
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{
		Kind: "fact", Title: "金陵古都香十五维度资料事实", Statement: span.Quote, Subject: "金陵古都香", Predicate: "产品十五维度资料", Value: domain.TypedValue{Type: "text", Text: span.Quote},
		Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{},
		Evidence: []domain.EvidenceRef{{SourceRevisionID: "source:product-15-dimensions", LocatorKind: span.LocatorKind, Locator: string(locator), Quote: span.Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	packagePath := filepath.Join(root, "work", "jinling-candidates.json")
	if err := os.WriteFile(packagePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: root, PackageFile: "work/jinling-candidates.json", OriginRunID: "jinling-gate-1"}); err != nil {
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
	pack, err := localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: root, Name: "金陵古都香知识包"})
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
