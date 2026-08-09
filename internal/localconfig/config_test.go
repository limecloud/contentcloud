package localconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadMigratesLegacyAndDeduplicatesCurrentWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", path)
	legacy := map[string]any{
		"server_url": "https://content.example.com/", "device_id": "device-1", "workspace_id": "workspace-2", "project_id": "project-2", "workspace_root": "/work/two",
		"daemon_bindings": []DaemonBinding{{ServerURL: "https://content.example.com", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: "/work/one"}}}},
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	bindings := config.Bindings()
	if len(bindings) != 1 || len(bindings[0].Workspaces) != 2 || bindings[0].ServerURL != "https://content.example.com" {
		t.Fatalf("unexpected normalized bindings: %#v", bindings)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) == string(body) || string(stored) == "" {
		t.Fatalf("legacy config was not rewritten: %s", stored)
	}
	for _, field := range []string{"device_id", "workspace_id", "project_id", "workspace_root"} {
		if string(stored) != "" && containsJSONKey(stored, field) {
			t.Fatalf("migrated config retained legacy field %q: %s", field, stored)
		}
	}
}

func TestLoadRemovesEmptyLegacyKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", path)
	if err := os.WriteFile(path, []byte(`{"server_url":"https://content.example.com","device_id":"","workspace_id":"","project_id":"","workspace_root":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"device_id", "workspace_id", "project_id", "workspace_root"} {
		if containsJSONKey(stored, field) {
			t.Fatalf("empty legacy field remained after migration: %q in %s", field, stored)
		}
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

func containsJSONKey(body []byte, key string) bool {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(body, &value); err != nil {
		return false
	}
	_, ok := value[key]
	return ok
}
