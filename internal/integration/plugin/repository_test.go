package plugin_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

func TestRepositoryPluginsArePortableAgentPluginPackages(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		kind       string
		skills     int
		mcpServers int
	}{
		{name: "contentcloud-video-production", version: "0.27.0", kind: "scene_plugin", skills: 7, mcpServers: 1},
		{name: "contentcloud-wechat-article", version: "0.1.0", kind: "skill_pack", skills: 4},
		{name: "contentcloud-marketing", version: "0.1.0", kind: "skill_pack", skills: 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loaded, err := plugin.Load(filepath.Join(repositoryRoot(t), "plugins", test.name))
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Manifest.Name != test.name || loaded.Manifest.Version != test.version {
				t.Fatalf("unexpected manifest identity: %+v", loaded.Manifest)
			}
			if loaded.Claims == nil || loaded.Claims.Kind != test.kind || loaded.Claims.PluginID != test.name || loaded.Claims.PluginVersion != test.version {
				t.Fatalf("unexpected claims: %+v", loaded.Claims)
			}
			if len(loaded.Skills) != test.skills || len(loaded.MCPServers) != test.mcpServers {
				t.Fatalf("unexpected components: skills=%d mcp=%d diagnostics=%+v", len(loaded.Skills), len(loaded.MCPServers), loaded.Diagnostics)
			}
			for _, diagnostic := range loaded.Diagnostics {
				if diagnostic.Level == plugin.DiagnosticError || diagnostic.Level == plugin.DiagnosticUnsupported {
					t.Fatalf("package has blocking diagnostic: %+v", diagnostic)
				}
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve repository test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}
