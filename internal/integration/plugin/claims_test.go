package plugin_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

func TestLoadValidatesContentCloudClaims(t *testing.T) {
	root := t.TempDir()
	writeClaimsPackage(t, root, validClaims(t, nil))

	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Claims == nil || loaded.Claims.PluginID != "test-plugin" {
		t.Fatalf("claims were not loaded: %+v", loaded.Claims)
	}
	if !strings.HasPrefix(loaded.ClaimsDigest, "sha256:") || len(loaded.ClaimsDigest) != 71 {
		t.Fatalf("unexpected claims digest: %q", loaded.ClaimsDigest)
	}
}

func TestLoadRejectsInvalidContentCloudClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "identity mismatch", mutate: func(claims map[string]any) { claims["plugin_id"] = "another-plugin" }},
		{name: "unknown field", mutate: func(claims map[string]any) { claims["unexpected"] = true }},
		{name: "unknown permission", mutate: func(claims map[string]any) { claims["permissions_requested"] = []string{"workspace:admin"} }},
		{name: "duplicate capability", mutate: func(claims map[string]any) {
			capability := map[string]any{"id": "contentcloud.video", "version": "1.0.0", "input_schemas": []string{}, "output_schemas": []string{}}
			claims["requested_capabilities"] = []any{capability, capability}
		}},
		{name: "duplicate host", mutate: func(claims map[string]any) {
			host := map[string]any{"id": "codex", "required": []string{"skills"}}
			claims["hosts"] = []any{host, host}
		}},
		{name: "runbook escape", mutate: func(claims map[string]any) {
			claims["support"] = map[string]any{"owner": "ContentCloud", "runbook": "../RUNBOOK.md"}
		}},
		{name: "missing runbook"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeClaimsPackageWithoutRunbook(t, root, validClaims(t, test.mutate))
			if test.name != "missing runbook" {
				writeFile(t, root, "run.zhongcao.contentcloud/RUNBOOK.md", "# Support\n")
			}
			_, err := plugin.Load(root)
			assertDomainCode(t, err, "CONTENTCLOUD_PLUGIN_CLAIMS_INVALID")
		})
	}
}

func TestLoadRejectsRunbookDirectory(t *testing.T) {
	root := t.TempDir()
	writeClaimsPackageWithoutRunbook(t, root, validClaims(t, func(claims map[string]any) {
		claims["support"] = map[string]any{"owner": "ContentCloud", "runbook": "./run.zhongcao.contentcloud"}
	}))

	_, err := plugin.Load(root)
	assertDomainCode(t, err, "CONTENTCLOUD_PLUGIN_CLAIMS_INVALID")
}

func writeClaimsPackage(t *testing.T, root, claims string) {
	t.Helper()
	writeClaimsPackageWithoutRunbook(t, root, claims)
	writeFile(t, root, "run.zhongcao.contentcloud/RUNBOOK.md", "# Support\n")
}

func writeClaimsPackageWithoutRunbook(t *testing.T, root, claims string) {
	t.Helper()
	writeFile(t, root, "plugin.json", `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name":"test-plugin",
  "version":"1.2.3",
  "extensions":{"run.zhongcao.contentcloud":{"claims":"./run.zhongcao.contentcloud/claims.json"}}
}`)
	writeFile(t, root, "run.zhongcao.contentcloud/claims.json", claims)
}

func validClaims(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	claims := map[string]any{
		"schema_version":       "contentcloud.plugin-claims/1.0",
		"plugin_id":            "test-plugin",
		"plugin_version":       "1.2.3",
		"package_spec_version": "1.0.0",
		"kind":                 "scene_plugin",
		"requested_capabilities": []any{
			map[string]any{"id": "contentcloud.video", "version": "1.0.0", "input_schemas": []string{"contracts/brief.schema.json"}, "output_schemas": []string{"contracts/storyboard.schema.json"}},
		},
		"permissions_requested": []string{"workspace:read", "workspace:write-managed"},
		"data_flow":             map[string]any{"local_by_default": true, "declared_cloud_actions": []string{"contentcloud.publish"}},
		"cost":                  map[string]any{"model": "included", "notice": "Included with ContentCloud."},
		"hosts": []any{
			map[string]any{"id": "codex", "required": []string{"skills"}},
			map[string]any{"id": "claude", "required": []string{"skills"}},
		},
		"support": map[string]any{"owner": "ContentCloud", "runbook": "./run.zhongcao.contentcloud/RUNBOOK.md"},
	}
	if mutate != nil {
		mutate(claims)
	}
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
