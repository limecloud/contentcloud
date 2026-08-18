package localworkspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisterIngestAndVerifyLocalSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", WorkspaceID: "workspace-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(t.TempDir(), "product.txt")
	if err := os.WriteFile(material, []byte("产品名称：金陵古都香\n建议零售价：168元\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	source, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:product", Title: "产品资料", StorageMode: "copy", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if source.ID != "source:product" || source.Location.Kind != "workspace_file" || filepath.IsAbs(source.Location.Path) {
		t.Fatalf("unexpected source: %+v", source)
	}
	second, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:product", StorageMode: "copy", Now: now})
	if err != nil || second.SHA256 != source.SHA256 {
		t.Fatalf("idempotent register failed: %+v %v", second, err)
	}
	bundle, err := IngestLocalSource(root, source.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Status != "ready" || len(bundle.Evidence) != 2 || bundle.Evidence[0].ReviewStatus != "accepted" {
		t.Fatalf("unexpected evidence bundle: %+v", bundle)
	}
	report, err := VerifyLocalSources(root)
	if err != nil || !report.Valid || report.Count != 1 {
		t.Fatalf("unexpected verification: %+v %v", report, err)
	}
	if _, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:duplicate", StorageMode: "copy"}); err == nil {
		t.Fatal("same content under a different source ID must be rejected")
	}
}

func TestReferenceSourceDetectsContentChange(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Initialize(InitOptions{Root: root, ProjectID: "project-1", Target: "none", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	material := filepath.Join(t.TempDir(), "manual.txt")
	if err := os.WriteFile(material, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterLocalSource(RegisterLocalSourceOptions{Root: root, File: material, ID: "source:manual", StorageMode: "reference"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(material, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyLocalSources(root)
	if err != nil || report.Valid || report.Results[0].HashMatches {
		t.Fatalf("changed reference must fail verification: %+v %v", report, err)
	}
	if _, err := IngestLocalSource(root, "source:manual", time.Time{}); err == nil {
		t.Fatal("changed immutable source must not ingest")
	}
}
