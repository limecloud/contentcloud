package localconfig

import (
	"sync"
	"testing"
)

func TestRuntimeBindingsMigratesLegacyAndDeduplicatesCurrentWorkspace(t *testing.T) {
	config := Config{
		ServerURL: "https://content.example.com/", DeviceID: "device-1", WorkspaceID: "workspace-2", ProjectID: "project-2", WorkspaceRoot: "/work/two",
		DaemonBindings: []DaemonBinding{{ServerURL: "https://content.example.com", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: "/work/one"}}}},
	}
	bindings := config.RuntimeBindings()
	if len(bindings) != 1 || len(bindings[0].Workspaces) != 2 || bindings[0].ServerURL != "https://content.example.com" {
		t.Fatalf("unexpected normalized bindings: %#v", bindings)
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

func TestRuntimeBindingsDoesNotMutateConfigAndSupportsConcurrentReads(t *testing.T) {
	config := Config{
		ServerURL: "https://content.example.com", DeviceID: "device-1", WorkspaceID: "workspace-1", ProjectID: "project-current", WorkspaceRoot: "/work/current",
		DaemonBindings: []DaemonBinding{{ServerURL: "https://content.example.com", DeviceID: "device-1", Workspaces: []DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-stored", Root: "/work/stored"}}}},
	}

	var wait sync.WaitGroup
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			bindings := config.RuntimeBindings()
			if len(bindings) != 1 || len(bindings[0].Workspaces) != 1 || bindings[0].Workspaces[0].ProjectID != "project-current" {
				t.Errorf("unexpected runtime bindings: %#v", bindings)
			}
		}()
	}
	wait.Wait()

	stored := config.DaemonBindings[0].Workspaces[0]
	if stored.ProjectID != "project-stored" || stored.Root != "/work/stored" {
		t.Fatalf("RuntimeBindings mutated the stored config: %#v", stored)
	}
}
