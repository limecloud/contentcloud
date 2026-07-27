package localworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/fixturev3"
)

func TestMaterializeJinlingFixtureBuildsCompleteWorkspace(t *testing.T) {
	file, err := os.Open(filepath.Join("..", "..", "fixtures", "v3", "jinling-gudu.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	fixture, err := fixturev3.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "jinling-gudu")
	result, err := MaterializeFixture(fixture, MaterializeFixtureOptions{Root: root, ProjectID: "project-fixture", WorkspaceID: "workspace-fixture", Target: "none", CLIVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Workspace.SourceCount != 20 || !result.Sources.Valid || !result.Knowledge.Valid || result.Diagnosis.Covered != 15 || result.Pack.Manifest.ItemCount != 15 {
		t.Fatalf("fixture did not materialize source and knowledge acceptance state: %#v", result)
	}
	if !strings.HasPrefix(result.Pack.Manifest.ContentHash, "sha256:") {
		t.Fatalf("knowledge pack content hash must use the V3 digest contract: %q", result.Pack.Manifest.ContentHash)
	}
	if !result.Runs.Valid || result.Run.Status != "completed" || result.ContentBatch.Batch.Status != "blocked" || result.ContentBatch.Report.Blocked != 10 || len(result.ContentFiles) != 10 {
		t.Fatalf("fixture did not materialize run and blocked content acceptance state: %#v", result)
	}
	for _, path := range append(append([]string{}, result.ContentBatch.Batch.ContentItemRefs...), result.Run.OutputPaths...) {
		if strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			t.Fatalf("fixture emitted a non-workspace-relative reference %q", path)
		}
	}
	if _, err := MaterializeFixture(fixture, MaterializeFixtureOptions{Root: root, ProjectID: "project-fixture", WorkspaceID: "workspace-fixture", Target: "none", CLIVersion: "test"}); err == nil {
		t.Fatal("fixture materialization must refuse to mutate an existing workspace")
	}
}
