package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/limecloud/contentcloud/internal/integration/plugin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type fakeCall struct {
	name string
	args []string
	env  map[string]string
}

type fakeRunner struct {
	responses []fakeResponse
	calls     []fakeCall
}

func (f *fakeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	f.calls = append(f.calls, fakeCall{name: command.Name, args: append([]string(nil), command.Args...), env: command.Env})
	if len(f.responses) == 0 {
		return CommandResult{}, errors.New("unexpected command: " + strings.Join(command.Args, " "))
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return CommandResult{Stdout: []byte(response.stdout), Stderr: []byte(response.stderr), ExitCode: response.exitCode}, response.err
}

func TestCapabilitiesFollowVerifiedClaudeVersion(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{stdout: "2.1.219 (Claude Code)\n"}}}
	host := newTestHost(t, runner)

	capabilities, err := host.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.Skills || capabilities.MCPStdio {
		t.Fatalf("old Claude version reported Agent Plugins support: %#v", capabilities)
	}
	if capabilities.PluginDirectoryInstall || capabilities.AtomicInstall || !capabilities.Rollback || !capabilities.NewSessionRequired {
		t.Fatalf("unexpected Claude capabilities: %#v", capabilities)
	}
}

func TestMaterializePackageGeneratesOnlyClaudePrivateProjection(t *testing.T) {
	host := newTestHost(t, &fakeRunner{})
	pkg := writeStandardPackage(t, "example-plugin", "1.2.3")

	projectedRoot, err := host.materializePackage(pkg, pkg.Root)
	if err != nil {
		t.Fatal(err)
	}
	if !regularFile(filepath.Join(projectedRoot, "plugin.json")) || !regularFile(filepath.Join(projectedRoot, "mcp.json")) {
		t.Fatal("Claude projection did not preserve standard package files")
	}
	var nativeManifest claudePluginManifest
	readJSONFile(t, filepath.Join(projectedRoot, ".claude-plugin", "plugin.json"), &nativeManifest)
	if nativeManifest.Name != pkg.Manifest.Name || nativeManifest.Version != pkg.Manifest.Version {
		t.Fatalf("unexpected Claude plugin manifest: %#v", nativeManifest)
	}
	var nativeMCP claudeMCPManifest
	readJSONFile(t, filepath.Join(projectedRoot, ".mcp.json"), &nativeMCP)
	server := nativeMCP.Servers["example-mcp"]
	if server.Command != "node" || !reflect.DeepEqual(server.Args, []string{"${CLAUDE_PLUGIN_ROOT}/bin/server"}) || server.CWD != "${CLAUDE_PLUGIN_ROOT}" || server.Env["DATA"] != "${CLAUDE_PLUGIN_DATA}/state" || server.Env[workspaceRootEnvironment] != "${CLAUDE_PROJECT_DIR}" {
		t.Fatalf("Agent Plugins variables were not translated for Claude: %#v", server)
	}
	marker, err := readProjectionMarker(projectedRoot)
	if err != nil || marker.Digest != pkg.Digest {
		t.Fatalf("Claude projection marker mismatch: marker=%#v error=%v", marker, err)
	}
	if _, err := os.Stat(filepath.Join(pkg.Root, ".claude-plugin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("materialization mutated the standard package")
	}
}

func TestMaterializePackageRejectsPublishedClaudePrivateFiles(t *testing.T) {
	host := newTestHost(t, &fakeRunner{})
	pkg := writeStandardPackage(t, "example-plugin", "1.2.3")
	if err := os.WriteFile(filepath.Join(pkg.Root, ".mcp.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := host.materializePackage(pkg, pkg.Root); err == nil || !strings.Contains(err.Error(), "Claude-private") {
		t.Fatalf("published Claude private file was accepted: %v", err)
	}
}

func TestDetectReadyUsesClaudeNativeListShapesAndCacheMarker(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	pkg := writeStandardPackage(t, "example-plugin", "1.2.3")
	projectedRoot, err := host.materializePackage(pkg, pkg.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.upsertMarketplaceProjection(pkg, projectedRoot); err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(host.config.ConfigDir, "plugins", "cache", host.config.MarketplaceName, pkg.Manifest.Name, pkg.Manifest.Version)
	if err := copyProjectionTree(projectedRoot, installRoot); err != nil {
		t.Fatal(err)
	}
	target := pluginhost.TargetFromPackage(pkg)
	runner.responses = []fakeResponse{
		{stdout: "2.1.220 (Claude Code)\n"},
		{stdout: jsonValue(t, []marketplaceListItem{{Name: host.config.MarketplaceName, Source: "directory", Path: host.config.ProjectionRoot, InstallLocation: host.config.ProjectionRoot}})},
		{stdout: jsonValue(t, []pluginListItem{{ID: host.pluginID(pkg.Manifest.Name), Version: pkg.Manifest.Version, Scope: "user", Enabled: true, InstallPath: installRoot}})},
		{stdout: "Validation passed\n"},
	}

	state, err := host.Detect(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusReady || state.Release == nil || state.Release.Digest != pkg.Digest {
		t.Fatalf("unexpected Claude ready state: %#v", state)
	}
	for _, item := range state.Components {
		if item.Status != pluginhost.StatusReady {
			t.Fatalf("component not ready: %#v", item)
		}
	}
	canonicalProjectedRoot, err := canonicalPath(projectedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := runner.calls[3].args; !reflect.DeepEqual(got, []string{"plugin", "validate", canonicalProjectedRoot, "--strict"}) {
		t.Fatalf("Claude projection was not strictly validated: %#v", runner.calls)
	}
	for _, call := range runner.calls {
		if call.env["CLAUDE_CONFIG_DIR"] != host.config.ConfigDir {
			t.Fatalf("Claude command escaped isolated config directory: %#v", call)
		}
	}
}

func TestDetectBlocksSameNamedUnmanagedMarketplace(t *testing.T) {
	runner := &fakeRunner{}
	host := newTestHost(t, runner)
	pkg := writeStandardPackage(t, "example-plugin", "1.2.3")
	runner.responses = []fakeResponse{
		{stdout: "2.1.220 (Claude Code)\n"},
		{stdout: jsonValue(t, []marketplaceListItem{{Name: host.config.MarketplaceName, Source: "directory", Path: t.TempDir(), InstallLocation: t.TempDir()}})},
		{stdout: "[]"},
	}

	state, err := host.Detect(t.Context(), pluginhost.TargetFromPackage(pkg))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusBlocked {
		t.Fatalf("unmanaged Claude marketplace should be blocked: %#v", state)
	}
}

func TestProjectionPreservesOtherManagedPlugins(t *testing.T) {
	host := newTestHost(t, &fakeRunner{})
	first := writeStandardPackage(t, "z-plugin", "1.0.0")
	second := writeStandardPackage(t, "a-plugin", "1.0.0")
	firstRoot, err := host.materializePackage(first, first.Root)
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := host.materializePackage(second, second.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.upsertMarketplaceProjection(first, firstRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := host.upsertMarketplaceProjection(second, secondRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := host.removeMarketplaceProjection(first.Manifest.Name); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := host.readMarketplaceProjection()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Plugins) != 1 || manifest.Plugins[0].Name != second.Manifest.Name {
		t.Fatalf("projection update lost unrelated plugin: %#v", manifest)
	}
}

func TestRealClaudeAgentPluginLifecycle(t *testing.T) {
	if os.Getenv("CONTENTCLOUD_CLAUDE_PLUGIN_SMOKE") != "1" {
		t.Skip("set CONTENTCLOUD_CLAUDE_PLUGIN_SMOKE=1 to run against the installed Claude CLI")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	pluginName := os.Getenv("CONTENTCLOUD_PLUGIN_SMOKE_PACKAGE")
	if pluginName == "" {
		pluginName = "contentcloud-video-production"
	}
	pkg, err := plugin.Load(filepath.Join(repositoryRoot, "plugins", pluginName))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := pluginhost.NewStore(filepath.Join(root, "contentcloud"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := New(Config{
		ConfigDir:      filepath.Join(root, "claude"),
		ProjectionRoot: filepath.Join(store.HostPath(pluginhost.HostClaude), "marketplace"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := pluginhost.New(host, store)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.Plan(t.Context(), pkg, "install")
	if err != nil {
		t.Fatalf("plan failed: %#v", err)
	}
	receipt, err := adapter.Apply(t.Context(), pkg, plan, true)
	if err != nil {
		t.Fatalf("apply failed: %#v", err)
	}
	if receipt.Status != pluginhost.StatusReady {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	state, err := adapter.Inspect(t.Context(), pkg)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != pluginhost.StatusReady {
		t.Fatalf("real Claude did not activate Agent Plugin: %#v", state)
	}
	removePlan, err := adapter.Plan(t.Context(), pkg, "remove")
	if err != nil {
		t.Fatal(err)
	}
	removed, err := adapter.Remove(t.Context(), pkg, removePlan, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Status != pluginhost.StatusRemoved {
		t.Fatalf("unexpected removal receipt: %#v", removed)
	}
}

func newTestHost(t *testing.T, runner CommandRunner) *Host {
	t.Helper()
	root := t.TempDir()
	host, err := New(Config{
		Binary:          "claude",
		ConfigDir:       filepath.Join(root, "claude-config"),
		MarketplaceName: "contentcloud-test",
		ProjectionRoot:  filepath.Join(root, "projection"),
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func writeStandardPackage(t *testing.T, name, version string) plugin.Package {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	writeFile(t, filepath.Join(root, "plugin.json"), `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "`+name+`",
  "version": "`+version+`",
  "description": "Agent Plugins test package",
  "author": {"name": "ContentCloud"},
  "license": "Apache-2.0"
}
`)
	writeFile(t, filepath.Join(root, "skills", "example-skill", "SKILL.md"), `---
name: example-skill
description: Test skill for the Claude adapter.
---

# Example
`)
	writeFile(t, filepath.Join(root, "mcp.json"), `{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "example-mcp": {
      "type": "stdio",
	  "command": "node",
	  "args": ["${PLUGIN_ROOT}/bin/server"],
      "env": {"DATA": "${PLUGIN_DATA}/state"},
      "cwd": "${PLUGIN_ROOT}"
    }
  }
}
`)
	pkg, err := plugin.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatal(err)
	}
}

func jsonValue(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
