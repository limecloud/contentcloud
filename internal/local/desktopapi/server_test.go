package desktopapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	localconfig "github.com/limecloud/contentcloud/internal/local/config"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
)

func TestServerRequiresCapabilityAndReturnsWorkspaceRelativeSnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project-alpha")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	discoveryPath := filepath.Join(t.TempDir(), "desktop-api.json")
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	server, err := Start(Options{
		Bindings: []localconfig.DaemonBinding{{DeviceID: "device-secret", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}}}},
		Version:  "test", DiscoveryPath: discoveryPath, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/snapshot", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+server.capability)
	request.Header.Set(apiVersionHeader, APIVersion)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Projects) != 1 || snapshot.Projects[0].ProjectID != "project-1" || snapshot.Projects[0].Name != "project-alpha" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	body, _ := json.Marshal(snapshot)
	for _, secret := range []string{root, "device-secret", server.capability} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("snapshot leaked %q: %s", secret, body)
		}
	}
}

func TestServerNegotiatesVersionAndQueuesIdempotentPublishWithEvents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project-alpha")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	server, err := Start(Options{
		Bindings:      []localconfig.DaemonBinding{{DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}}}},
		DiscoveryPath: filepath.Join(t.TempDir(), "desktop-api.json"), StatePath: filepath.Join(t.TempDir(), "desktop.sqlite3"), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/snapshot", nil)
	request.Header.Set("Authorization", "Bearer "+server.capability)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("unversioned snapshot status = %d", response.StatusCode)
	}

	snapshot := requestSnapshotForTest(t, server)
	project := snapshot.Projects[0]
	command := PublishWorkspaceCommand{
		SchemaVersion: CommandSchemaVersion, RequestID: "request-1", WorkspaceID: project.WorkspaceID, ProjectID: project.ProjectID,
		SubjectRef: "workspace", BaseRevision: project.CloudRevision, ObservedDigest: project.ObservedDigest, IdempotencyKey: "publish-1",
	}
	first := publishForTest(t, server, command)
	second := publishForTest(t, server, command)
	if first.CommandID != second.CommandID || first.EventCursor != second.EventCursor || first.State != "queued" {
		t.Fatalf("publish was not idempotent: %#v %#v", first, second)
	}

	request, _ = http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/events?after=0", nil)
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var stream EventStream
	if err := json.NewDecoder(response.Body).Decode(&stream); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || stream.Gap || len(stream.Events) != 2 || stream.Events[1].Type != "workspace.publish.queued" {
		t.Fatalf("unexpected event stream: status=%d %#v", response.StatusCode, stream)
	}
}

func TestPublishRejectsStaleDigestAndUnknownFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project-alpha")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	server, err := Start(Options{
		Bindings:      []localconfig.DaemonBinding{{DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}}}},
		DiscoveryPath: filepath.Join(t.TempDir(), "desktop-api.json"), StatePath: filepath.Join(t.TempDir(), "desktop.sqlite3"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	project := requestSnapshotForTest(t, server).Projects[0]
	if err := os.WriteFile(filepath.Join(root, "40-work", "changed.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := PublishWorkspaceCommand{SchemaVersion: CommandSchemaVersion, RequestID: "request-1", WorkspaceID: project.WorkspaceID, ProjectID: project.ProjectID, SubjectRef: "workspace", BaseRevision: project.CloudRevision, ObservedDigest: project.ObservedDigest, IdempotencyKey: "publish-1"}
	body, _ := json.Marshal(command)
	request, _ := http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/commands/workspace-publish", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("stale publish status = %d", response.StatusCode)
	}

	unknown := strings.TrimSuffix(string(body), "}") + `,"extra":true}`
	request, _ = http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/commands/workspace-publish", strings.NewReader(unknown))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown command field status = %d", response.StatusCode)
	}
}

func requestSnapshotForTest(t *testing.T, server *Server) Snapshot {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/snapshot", nil)
	authorizeTestRequest(request, server)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || len(snapshot.Projects) != 1 {
		t.Fatalf("snapshot status=%d value=%#v", response.StatusCode, snapshot)
	}
	return snapshot
}

func publishForTest(t *testing.T, server *Server, command PublishWorkspaceCommand) CommandResponse {
	t.Helper()
	body, _ := json.Marshal(command)
	request, _ := http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/commands/workspace-publish", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result CommandResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("publish status=%d value=%#v", response.StatusCode, result)
	}
	return result
}

func authorizeTestRequest(request *http.Request, server *Server) {
	request.Header.Set("Authorization", "Bearer "+server.capability)
	request.Header.Set(apiVersionHeader, APIVersion)
}

func TestDiscoveryFileIsPrivateAndRemovedByOwningInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-api.json")
	server, err := Start(Options{DiscoveryPath: path, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("discovery mode = %o", info.Mode().Perm())
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discovery file still exists: %v", err)
	}
}

func TestServerRejectsBrowserOrigin(t *testing.T) {
	server, err := Start(Options{DiscoveryPath: filepath.Join(t.TempDir(), "desktop-api.json")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/health", nil)
	request.Header.Set("Authorization", "Bearer "+server.capability)
	request.Header.Set("Origin", "https://example.com")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("origin request status = %d", response.StatusCode)
	}
}

func TestReviewRoutesStrictlyForwardProjectAndRevisionScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project-alpha")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		project string
		command string
		params  map[string]any
	}
	server, err := Start(Options{
		Bindings:      []localconfig.DaemonBinding{{DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}}}},
		DiscoveryPath: filepath.Join(t.TempDir(), "desktop-api.json"),
		ReviewDispatcher: func(_ context.Context, projectID, command string, params any) (json.RawMessage, error) {
			value, ok := params.(map[string]any)
			if !ok {
				t.Fatalf("review params type = %T", params)
			}
			calls = append(calls, struct {
				project string
				command string
				params  map[string]any
			}{project: projectID, command: command, params: value})
			switch command {
			case "desktop.review.inbox":
				return json.RawMessage(`{"project_id":"project-1","items":[]}`), nil
			case "desktop.review.show":
				return json.RawMessage(`{"submission":{},"revision":{},"comments":[],"diffs":[],"allowed_actions":[]}`), nil
			case "desktop.review.comment":
				return json.RawMessage(`{"id":"comment-1"}`), nil
			default:
				return json.RawMessage(`{"status":"accepted"}`), nil
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/review/inbox", nil)
	authorizeTestRequest(request, server)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(calls) != 1 || calls[0].command != "desktop.review.inbox" || calls[0].params["project_id"] != "project-1" {
		t.Fatalf("inbox forwarding = status %d calls %#v", response.StatusCode, calls)
	}

	body := `{"revision_id":"revision-1","body":"请补来源","json_pointer":"/objects/0"}`
	request, _ = http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/review/comments", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(calls) != 2 || calls[1].params["revision_id"] != "revision-1" || calls[1].params["body"] != "请补来源" {
		t.Fatalf("comment forwarding = status %d calls %#v", response.StatusCode, calls)
	}

	body = `{"reason":"拒绝原因"}`
	request, _ = http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/review/revisions/revision-1/reject", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || len(calls) != 3 || calls[2].command != "desktop.review.reject" || calls[2].params["revision_id"] != "revision-1" || calls[2].params["reason"] != "拒绝原因" {
		t.Fatalf("reject forwarding = status %d calls %#v", response.StatusCode, calls)
	}

	unknown := `{"revision_id":"revision-1","body":"x","extra":true}`
	request, _ = http.NewRequest(http.MethodPost, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/review/comments", strings.NewReader(unknown))
	request.Header.Set("Content-Type", "application/json")
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown review field status = %d", response.StatusCode)
	}
}

func TestReviewRoutesMapDispatcherFailuresAndScopeErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project-alpha")
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{Root: root, WorkspaceID: "workspace-1", ProjectID: "project-1", Target: "codex-plugin", CLIVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	server, err := Start(Options{
		Bindings:      []localconfig.DaemonBinding{{DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: root}}}},
		DiscoveryPath: filepath.Join(t.TempDir(), "desktop-api.json"),
		ReviewDispatcher: func(context.Context, string, string, any) (json.RawMessage, error) {
			return nil, errors.New("CLOUD_REVIEW_TIMEOUT")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	request, _ := http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/projects/project-1/review/inbox", nil)
	authorizeTestRequest(request, server)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("dispatcher failure status = %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodGet, "http://"+server.listener.Addr().String()+"/v1/projects/not-bound/review/inbox", nil)
	authorizeTestRequest(request, server)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unbound review project status = %d", response.StatusCode)
	}
}
