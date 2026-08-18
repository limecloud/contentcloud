package plugin_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
)

func TestLoadDiscoversValidSkillsAndStdioMCP(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plugin.json", `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name":"example.plugin",
  "version":"1.2.3",
  "extensions":{"com.example":{"enabled":true}}
}`)
	writeFile(t, root, "skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy an approved release.\n---\n\n# Deploy\n")
	writeFile(t, root, "mcp.json", `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{"local":{"type":"stdio","command":"npx","args":["--root","${PLUGIN_ROOT}"],"env":{"CACHE":"${PLUGIN_DATA}/cache"},"cwd":"${PLUGIN_ROOT}"}}
}`)

	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.Name != "example.plugin" || loaded.SpecVersion != "1.0.0" || len(loaded.Skills) != 1 || len(loaded.MCPServers) != 1 {
		t.Fatalf("unexpected package: %+v", loaded)
	}
	if loaded.Skills[0].Name != "deploy" || loaded.MCPServers[0].Name != "local" || !loaded.MCPServers[0].Supported {
		t.Fatalf("unexpected components: skills=%+v mcp=%+v", loaded.Skills, loaded.MCPServers)
	}
	if !strings.HasPrefix(loaded.Digest, "sha256:") || len(loaded.Digest) != 71 || loaded.Files != 3 || loaded.Bytes == 0 {
		t.Fatalf("unexpected package summary: %+v", loaded)
	}
	second, err := plugin.Load(root)
	if err != nil || second.Digest != loaded.Digest {
		t.Fatalf("digest is not reproducible: first=%s second=%s err=%v", loaded.Digest, second.Digest, err)
	}
}

func TestLoadAppliesManifestExceptionsWithoutAcceptingInvalidKnownFields(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"valid","unknown":true,"extensions":"ignored"}`)
	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MANIFEST_UNKNOWN_FIELD", "")
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MANIFEST_EXTENSIONS_IGNORED", "")

	writeFile(t, root, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"valid","keywords":"wrong"}`)
	_, err = plugin.Load(root)
	assertDomainCode(t, err, "AGENT_PLUGIN_MANIFEST_INVALID")
}

func TestLoadRejectsUnsupportedManifestVersionAndDuplicateKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/9.0.0/plugin.schema.json","name":"valid"}`)
	_, err := plugin.Load(root)
	assertDomainCode(t, err, "AGENT_PLUGIN_MANIFEST_INVALID")

	writeFile(t, root, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"first","name":"second"}`)
	_, err = plugin.Load(root)
	assertDomainCode(t, err, "AGENT_PLUGIN_MANIFEST_INVALID")
}

func TestLoadIsolatesInvalidSkillAndMCPServerEntries(t *testing.T) {
	root := t.TempDir()
	writeMinimalManifest(t, root)
	writeFile(t, root, "skills/good/SKILL.md", "---\nname: good\ndescription: A valid skill.\n---\n# Good\n")
	writeFile(t, root, "skills/bad/SKILL.md", "---\nname: wrong\ndescription: Invalid directory identity.\n---\n# Bad\n")
	writeFile(t, root, "mcp.json", `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{
    "good":{"type":"stdio","command":"contentcloud"},
    "bad":{"type":"stdio","command":"sh -c unsafe"},
    "remote":{"type":"streamable-http","url":"https://example.com/mcp"}
  }
}`)

	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Skills) != 1 || loaded.Skills[0].Name != "good" {
		t.Fatalf("skill isolation failed: %+v", loaded.Skills)
	}
	if len(loaded.MCPServers) != 2 || loaded.MCPServers[0].Name != "good" || loaded.MCPServers[1].Name != "remote" || loaded.MCPServers[1].Supported {
		t.Fatalf("MCP isolation failed: %+v", loaded.MCPServers)
	}
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_SKILL_INVALID", "bad")
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MCP_SERVER_INVALID", "bad")
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MCP_TRANSPORT_UNSUPPORTED", "remote")
}

func TestInvalidMCPDocumentDoesNotDisableSkills(t *testing.T) {
	root := t.TempDir()
	writeMinimalManifest(t, root)
	writeFile(t, root, "skills/good/SKILL.md", "---\nname: good\ndescription: A valid skill.\n---\n# Good\n")
	writeFile(t, root, "mcp.json", `{"$schema":"wrong","mcpServers":{}}`)

	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Skills) != 1 || len(loaded.MCPServers) != 0 {
		t.Fatalf("component-type isolation failed: %+v", loaded)
	}
	assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MCP_SCHEMA_UNSUPPORTED", "")
}

func TestMCPValidatesPathURLHeadersAndReservedEnvironment(t *testing.T) {
	root := t.TempDir()
	writeMinimalManifest(t, root)
	writeFile(t, root, "mcp.json", `{
  "$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers":{
    "escape":{"type":"stdio","command":"tool","cwd":"${PLUGIN_DATA}/../escape"},
    "reserved":{"type":"stdio","command":"tool","env":{"plugin_root":"bad"}},
    "plain-http":{"type":"streamable-http","url":"http://example.com/mcp"},
    "duplicate-header":{"type":"streamable-http","url":"https://example.com/mcp","headers":{"X-Test":"a","x-test":"b"}},
    "loopback":{"type":"streamable-http","url":"http://127.0.0.1:8080/mcp"}
  }
}`)

	loaded, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MCPServers) != 1 || loaded.MCPServers[0].Name != "loopback" {
		t.Fatalf("unexpected valid MCP servers: %+v", loaded.MCPServers)
	}
	for _, name := range []string{"escape", "reserved", "plain-http", "duplicate-header"} {
		assertDiagnostic(t, loaded.Diagnostics, "PLUGIN_MCP_SERVER_INVALID", name)
	}
}

func TestPackageSafetyLimitsAndSymlinkEscapeFailClosed(t *testing.T) {
	root := t.TempDir()
	writeMinimalManifest(t, root)
	writeFile(t, root, "large.txt", strings.Repeat("x", 20))
	limits := plugin.DefaultLimits()
	limits.MaxFileBytes = 10
	limits.MaxPackBytes = 100
	_, err := plugin.LoadWithLimits(root, limits)
	assertDomainCode(t, err, "AGENT_PLUGIN_PACKAGE_INVALID")

	if runtime.GOOS == "windows" {
		return
	}
	os.Remove(filepath.Join(root, "large.txt"))
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	_, err = plugin.Load(root)
	assertDomainCode(t, err, "AGENT_PLUGIN_PACKAGE_INVALID")
}

func TestExpandPluginVariablesIsSinglePassAndLeavesUnknownText(t *testing.T) {
	value := plugin.ExpandPluginVariables("${PLUGIN_ROOT}/${PLUGIN_DATA}/${OTHER}", "${PLUGIN_DATA}", "/data")
	if value != "${PLUGIN_DATA}//data/${OTHER}" {
		t.Fatalf("unexpected single-pass expansion: %q", value)
	}
}

func writeMinimalManifest(t *testing.T, root string) {
	t.Helper()
	writeFile(t, root, "plugin.json", `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test-plugin"}`)
}

func writeFile(t *testing.T, root, relative, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertDiagnostic(t *testing.T, diagnostics []plugin.Diagnostic, code, component string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && (component == "" || diagnostic.Component == component) {
			return
		}
	}
	t.Fatalf("missing diagnostic %s/%s in %+v", code, component, diagnostics)
}

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *fault.Error
	if !errors.As(err, &domainErr) || domainErr.Code != code {
		t.Fatalf("error = %v, want domain code %s", err, code)
	}
}
