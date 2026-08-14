package localconfig

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLoadRejectsRetiredSingleWorkspaceFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", path)
	body := []byte(`{"server_url":"https://content.example.com/","device_id":"retired-device","workspace_id":"retired-workspace","project_id":"retired-project","workspace_root":"/retired","daemon_bindings":[{"server_url":"https://content.example.com","device_id":"device-1","workspaces":[{"workspace_id":"workspace-1","project_id":"project-1","root":"/work/one"}]}]}`)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("retired top-level workspace fields must be rejected")
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(body) {
		t.Fatalf("Load must not rewrite configuration: %s", stored)
	}
}

func TestLoadNormalizesCurrentDaemonBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", path)
	body := []byte(`{"server_url":"https://content.example.com/","daemon_bindings":[{"server_url":"https://content.example.com/","device_id":"device-1","workspaces":[{"workspace_id":"workspace-1","project_id":"project-1","root":"/work/one"}]}]}`)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	bindings := config.Bindings()
	if len(bindings) != 1 || len(bindings[0].Workspaces) != 1 || bindings[0].Workspaces[0].WorkspaceID != "workspace-1" || bindings[0].ServerURL != "https://content.example.com" {
		t.Fatalf("unexpected normalized bindings: %#v", bindings)
	}
}

func TestLoadRejectsTrailingJSONValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{"daemon_bindings":[]} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("trailing JSON value must be rejected")
	}
}

func TestUpsertDaemonBindingPreservesOtherDevicesAndUpdatesWorkspace(t *testing.T) {
	config := Config{DaemonBindings: []DaemonBinding{
		{ServerURL: "https://one.example", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-old", Root: "/old"}}},
		{ServerURL: "https://two.example", DeviceID: "device-2"},
	}}
	config.UpsertDaemonBinding(DaemonBinding{ServerURL: "https://one.example/", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: "/new"}}})
	if len(config.DaemonBindings) != 2 || config.DaemonBindings[0].Workspaces[0].Root != "/new" || config.DaemonBindings[1].DeviceID != "device-2" {
		t.Fatalf("unexpected bindings after upsert: %#v", config.DaemonBindings)
	}
}

func TestBindingsDoesNotMutateConfigAndSupportsConcurrentReads(t *testing.T) {
	config := Config{
		ServerURL:      "https://content.example.com",
		DaemonBindings: []DaemonBinding{{ServerURL: "https://content.example.com", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-current", Root: "/work/current"}}}},
	}

	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			bindings := config.Bindings()
			if len(bindings) != 1 || len(bindings[0].Workspaces) != 1 || bindings[0].Workspaces[0].ProjectID != "project-current" {
				t.Errorf("unexpected runtime bindings: %#v", bindings)
			}
		}()
	}
	wait.Wait()

	stored := config.DaemonBindings[0].Workspaces[0]
	if stored.ProjectID != "project-current" || stored.Root != "/work/current" {
		t.Fatalf("Bindings mutated the stored config: %#v", stored)
	}
}

func TestPrimaryBindingAndWorkspace(t *testing.T) {
	config := Config{DaemonBindings: []DaemonBinding{{ServerURL: "https://one.example", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: "/work/one"}}}}}
	binding, workspace, ok := config.PrimaryWorkspace()
	if !ok || binding.DeviceID != "device-1" || workspace.ProjectID != "project-1" {
		t.Fatalf("unexpected primary binding: %#v %#v", binding, workspace)
	}
}

func TestEnsureMachineIDIsStableAndValid(t *testing.T) {
	var config Config
	first, err := config.EnsureMachineID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := config.EnsureMachineID()
	if err != nil || second != first || !strings.HasPrefix(first, "mach_") || len(first) != 37 {
		t.Fatalf("machine id is not stable: first=%q second=%q err=%v", first, second, err)
	}
}
