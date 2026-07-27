package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func TestKnowledgeCandidateFlowToApprovedQueryAndPack(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(t.TempDir(), "product.txt")
	if err := os.WriteFile(material, []byte("金陵古都香建议零售价为168元。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:product", StorageMode: "copy"}); err != nil {
		t.Fatal(err)
	}
	bundle, err := IngestLocalSource(root, "source:product", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := json.Marshal(bundle.Evidence[0].Locator)
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{
		Kind: "fact", Title: "产品价格", Statement: bundle.Evidence[0].Quote, Subject: "金陵古都香", Predicate: "建议零售价", Value: domain.TypedValue{Type: "number", Number: floatPointer(168), Unit: "CNY"},
		Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{},
		Evidence: []domain.EvidenceRef{{SourceRevisionID: "source:product", LocatorKind: bundle.Evidence[0].LocatorKind, Locator: string(locator), Quote: bundle.Evidence[0].Quote}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	packagePath := filepath.Join(root, "work", "knowledge-candidates.json")
	packageBody, _ := json.Marshal(pkg)
	if err := os.WriteFile(packagePath, packageBody, 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportKnowledgeCandidates(ImportKnowledgeOptions{Root: root, PackageFile: "work/knowledge-candidates.json", OriginRunID: "local-run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Imported) != 1 || imported.Imported[0].Status != "candidate" || !containsString(imported.Imported[0].Dimensions, "spec-cost-price") {
		t.Fatalf("unexpected import: %+v", imported)
	}
	lint, err := LintKnowledge(root)
	if err != nil || !lint.Valid || lint.ErrorCount != 0 {
		t.Fatalf("unexpected lint: %+v %v", lint, err)
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root})
	if err != nil || len(query.Informational) != 1 || len(query.Eligible) != 0 {
		t.Fatalf("candidate must be informational before approval: %+v %v", query, err)
	}
	pack, err := PackKnowledge(PackKnowledgeOptions{Root: root, Name: "金陵古都香知识包"})
	if err != nil {
		t.Fatal(err)
	}
	if pack.ObjectCount != 2 || pack.SourceCount != 1 {
		t.Fatalf("unexpected pack: %+v", pack)
	}
	for _, relative := range []string{pack.PackPath, pack.DisclosuresPath} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("missing pack output %s: %v", relative, err)
		}
	}
	objects, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pack.PackPath)))
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(map[string]any{"schema_version": "knowledge-pack/2.0", "submission_type": "knowledge", "objects": json.RawMessage(objects)})
	now := time.Date(2026, 7, 26, 11, 0, 0, 0, time.UTC)
	snapshot := domain.ApprovedSnapshot{ID: "snapshot-1", SubmissionType: "knowledge", CanonicalContent: canonical, EligibleIDs: []string{imported.Imported[0].ID}, CreatedAt: now}
	if _, err := StoreApprovedSnapshot(root, snapshot, now); err != nil {
		t.Fatal(err)
	}
	approvedQuery, err := QueryKnowledge(QueryKnowledgeOptions{Root: root})
	if err != nil || approvedQuery.ApprovedSnapshotID != snapshot.ID || len(approvedQuery.Eligible) != 1 {
		t.Fatalf("approved candidate must become eligible: %+v %v", approvedQuery, err)
	}
	diagnosis, err := DiagnoseKnowledge(root, "", time.Time{})
	if err != nil || diagnosis.Covered == 0 {
		t.Fatalf("unexpected diagnosis: %+v %v", diagnosis, err)
	}
}

func TestKnowledgeImportRejectsInventedEvidence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(t.TempDir(), "product.txt")
	if err := os.WriteFile(material, []byte("真实原文\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:product"}); err != nil {
		t.Fatal(err)
	}
	if _, err := IngestLocalSource(root, "source:product", time.Time{}); err != nil {
		t.Fatal(err)
	}
	pkg := domain.KnowledgeExtractionPackage{SchemaVersion: "1.0", Candidates: []domain.KnowledgeCandidate{{
		Kind: "fact", Title: "伪造", Statement: "并不存在", Subject: "产品", Predicate: "属性", Value: domain.TypedValue{Type: "text", Text: "并不存在"},
		Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}}, RiskLevel: "low", AllowedChannels: []string{},
		Evidence: []domain.EvidenceRef{{SourceRevisionID: "source:product", LocatorKind: "paragraph", Locator: `{"paragraph":1}`, Quote: "伪造原文"}}, ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{},
	}}, Warnings: []string{}}
	body, _ := json.Marshal(pkg)
	path := filepath.Join(root, "work", "bad-candidates.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportKnowledgeCandidates(ImportKnowledgeOptions{Root: root, PackageFile: "work/bad-candidates.json"}); err == nil {
		t.Fatal("invented evidence must be rejected")
	}
}

func TestKnowledgeImportRejectsSymlinkOutsideWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(validKnowledgeCandidatePackage())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "knowledge-candidates.json")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "work", "linked-candidates.json")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("当前文件系统不支持符号链接：%v", err)
	}
	_, err = ImportKnowledgeCandidates(ImportKnowledgeOptions{Root: root, PackageFile: "work/linked-candidates.json"})
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "LOCAL_FILE_OUTSIDE_WORKSPACE" {
		t.Fatalf("expected LOCAL_FILE_OUTSIDE_WORKSPACE, got %v", err)
	}
}

func TestKnowledgeImportRejectsInvalidCandidatePackageShapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	base := validKnowledgeCandidatePackage()
	cases := []struct {
		name string
		edit func(map[string]any)
		code string
	}{
		{
			name: "unknown field",
			edit: func(value map[string]any) { value["unexpected"] = true },
			code: "KNOWLEDGE_CANDIDATES_JSON_INVALID",
		},
		{
			name: "missing required array",
			edit: func(value map[string]any) {
				delete(value["candidates"].([]any)[0].(map[string]any), "allowed_channels")
			},
			code: "KNOWLEDGE_ARRAY_REQUIRED",
		},
		{
			name: "typed value mismatch",
			edit: func(value map[string]any) {
				candidate := value["candidates"].([]any)[0].(map[string]any)
				candidate["value"] = map[string]any{"type": "number", "text": "not-a-number"}
			},
			code: "KNOWLEDGE_VALUE_REQUIRED",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := cloneJSONMap(t, base)
			tc.edit(body)
			path := filepath.Join(root, "work", "invalid-"+tc.name+".json")
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = ImportKnowledgeCandidates(ImportKnowledgeOptions{Root: root, PackageFile: relativeWorkspacePath(root, path)})
			var domainError *domain.Error
			if !errors.As(err, &domainError) || domainError.Code != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}

func validKnowledgeCandidatePackage() map[string]any {
	return map[string]any{
		"schema_version": "1.0",
		"warnings":       []any{},
		"candidates": []any{map[string]any{
			"kind": "fact", "title": "产品规格", "statement": "产品规格为20支", "subject": "产品", "predicate": "规格",
			"value":      map[string]any{"type": "text", "text": "20支"},
			"scope":      map[string]any{"regions": []any{}, "channels": []any{}, "audiences": []any{}, "product_variants": []any{}},
			"risk_level": "low", "allowed_channels": []any{},
			"evidence":             []any{map[string]any{"source_revision_id": "source:product", "locator_kind": "paragraph", "locator": `{"paragraph":1}`, "quote": "产品规格为20支"}},
			"forbidden_extensions": []any{}, "depends_on_fact_ids": []any{},
		}},
	}
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func floatPointer(value float64) *float64 { return &value }
