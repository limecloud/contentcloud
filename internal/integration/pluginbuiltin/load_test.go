package pluginbuiltin_test

import (
	"path/filepath"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
)

func TestLoadBundledStandardPlugin(t *testing.T) {
	pkg, err := pluginbuiltin.Load(t.TempDir(), pluginbuiltin.VideoProduction, "0.20.0")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Name != pluginbuiltin.VideoProduction || pkg.SpecVersion != "1.0.0" || len(pkg.Skills) == 0 || len(pkg.MCPServers) != 1 {
		t.Fatalf("unexpected bundled Agent Plugin: %#v", pkg)
	}
	if filepath.Base(pkg.Root) != "0.20.0" {
		t.Fatalf("bundle was not materialized in the versioned store: %s", pkg.Root)
	}
}
