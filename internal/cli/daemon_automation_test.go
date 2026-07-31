package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/automationworkspace"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

func TestDaemonFixtureUsesAttemptScopedWorkspaceWithoutPersistingRunCredential(t *testing.T) {
	now := time.Date(2026, 7, 27, 16, 30, 0, 0, time.UTC)
	automationRoot := filepath.Join(t.TempDir(), "automation")
	runToken := "rt_super_secret"
	contract := domain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "knowledge_extract",
		Project: domain.Project{ID: "project-1"}, Sources: []domain.ContractSource{}, InputSnapshotID: "snapshot-1", OutputSchema: domain.KnowledgeCandidatesSchema,
		Capability:   domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
		ManifestHash: "sha256:" + strings.Repeat("c", 64),
	}
	bundle := environment.CreativeExecutionBundle{BundleID: "ceb_test", ProjectID: "project-1", Digest: "sha256:" + strings.Repeat("b", 64)}
	lease := app.Lease{
		Run:      domain.TaskRun{ID: "run-1", ProjectID: "project-1", TaskType: "knowledge_extract", OutputSchema: domain.KnowledgeCandidatesSchema, OutputCount: 1},
		Attempt:  domain.RunAttempt{ID: "attempt-1", ProjectID: "project-1", RunID: "run-1", State: "leased"},
		Contract: contract, ExecutionBundle: &bundle, LeaseExpiresAt: now.Add(5 * time.Minute), RunToken: runToken,
	}
	commands := []string{}
	workspaceInspected := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer dt_test" {
			http.Error(writer, "missing device credential", http.StatusUnauthorized)
			return
		}
		var payload struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		commands = append(commands, payload.Command)
		switch payload.Command {
		case "daemon.poll":
			writeCLIEnvelope(t, writer, payload.Command, lease)
			return
		case "run.report":
			var report struct {
				RunID     string          `json:"run_id"`
				AttemptID string          `json:"attempt_id"`
				RunToken  string          `json:"run_token"`
				Package   json.RawMessage `json:"package"`
			}
			if err := json.Unmarshal(payload.Params, &report); err != nil || report.RunID != "run-1" || report.AttemptID != "attempt-1" || report.RunToken != runToken || len(report.Package) == 0 {
				t.Errorf("report payload = %#v, err=%v", report, err)
				http.Error(writer, "invalid report", http.StatusBadRequest)
				return
			}
			entries, err := os.ReadDir(automationRoot)
			if err != nil || len(entries) != 1 || !entries[0].IsDir() {
				t.Errorf("automation root entries=%#v err=%v", entries, err)
				http.Error(writer, "workspace missing", http.StatusInternalServerError)
				return
			}
			workspace := filepath.Join(automationRoot, entries[0].Name())
			for _, name := range []string{"lease.json", "contract.json", "output.schema.json", "SKILL.md", "execution-bundle.json"} {
				body, readErr := os.ReadFile(filepath.Join(workspace, name))
				if readErr != nil || bytes.Contains(body, []byte(runToken)) || bytes.Contains(body, []byte("run_token")) {
					t.Errorf("workspace file %s leaked credential or failed: %v", name, readErr)
					http.Error(writer, "credential leak", http.StatusInternalServerError)
					return
				}
			}
			workspaceInspected = true
			writeCLIEnvelope(t, writer, payload.Command, map[string]any{"submission_revision_id": "revision-1"})
			return
		default:
			http.Error(writer, "unexpected command", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", configPath)
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_test")
	t.Setenv("CONTENTCLOUD_AUTOMATION_ROOT", automationRoot)
	if err := localconfig.Save(localconfig.Config{ServerURL: server.URL, DeviceID: "device-1"}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runtime := &Root{stdout: &stdout, stderr: &stderr, now: func() time.Time { return now }}
	command := runtime.command()
	command.SetArgs([]string{"--json", "daemon", "run", "--once", "--fixture"})
	if err := command.Execute(); err != nil {
		t.Fatalf("daemon fixture failed: %v; stderr=%s", err, stderr.String())
	}
	if !workspaceInspected || strings.Join(commands, ",") != "daemon.poll,run.report" {
		t.Fatalf("daemon flow commands=%#v inspected=%v", commands, workspaceInspected)
	}
	entries, err := os.ReadDir(automationRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("attempt workspace was not cleaned: entries=%#v err=%v", entries, err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			AttemptID         string `json:"attempt_id"`
			IsolatedWorkspace bool   `json:"isolated_workspace"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.AttemptID != "attempt-1" || !envelope.Data.IsolatedWorkspace {
		t.Fatalf("daemon output: err=%v output=%s", err, stdout.String())
	}
}

func TestDaemonMaxConcurrentTasksUsesBoundedOperationalDefault(t *testing.T) {
	t.Setenv("CONTENTCLOUD_DAEMON_MAX_CONCURRENT_TASKS", "")
	if got := daemonMaxConcurrentTasks(); got != 2 {
		t.Fatalf("default concurrency=%d", got)
	}
	t.Setenv("CONTENTCLOUD_DAEMON_MAX_CONCURRENT_TASKS", "20")
	if got := daemonMaxConcurrentTasks(); got != 8 {
		t.Fatalf("concurrency cap=%d", got)
	}
	t.Setenv("CONTENTCLOUD_DAEMON_MAX_CONCURRENT_TASKS", "4")
	if got := daemonMaxConcurrentTasks(); got != 4 {
		t.Fatalf("configured concurrency=%d", got)
	}
}

func TestDaemonFinishesAttemptWhenWorkspaceIsolationFails(t *testing.T) {
	now := time.Date(2026, 7, 27, 17, 0, 0, 0, time.UTC)
	automationRoot := filepath.Join(t.TempDir(), "automation")
	contract := domain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "knowledge_extract",
		Project: domain.Project{ID: "project-1"}, Sources: []domain.ContractSource{}, InputSnapshotID: "snapshot-1", OutputSchema: domain.KnowledgeCandidatesSchema,
		Capability:   domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
		ManifestHash: "sha256:" + strings.Repeat("c", 64),
	}
	lease := app.Lease{
		Run:     domain.TaskRun{ID: "run-1", ProjectID: "project-1", TaskType: "knowledge_extract", OutputSchema: domain.KnowledgeCandidatesSchema, OutputCount: 1},
		Attempt: domain.RunAttempt{ID: "attempt-1", ProjectID: "project-1", RunID: "run-1", State: "leased"}, Contract: contract,
		LeaseExpiresAt: now.Add(5 * time.Minute), RunToken: "rt_super_secret",
	}
	active, err := automationworkspace.Begin(automationworkspace.Options{
		BaseDir: automationRoot, AttemptID: lease.Attempt.ID, RunID: lease.Run.ID, ProjectID: lease.Run.ProjectID,
		Contract: contract, OutputSchema: []byte(`{"type":"object"}`), Skill: []byte("# Active\n"), Now: now, ExpiresAt: lease.LeaseExpiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer active.Cleanup()

	commands := []string{}
	finished := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Command string          `json:"command"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		commands = append(commands, payload.Command)
		switch payload.Command {
		case "daemon.poll":
			writeCLIEnvelope(t, writer, payload.Command, lease)
		case "run.finish":
			var finish struct {
				AttemptID    string `json:"attempt_id"`
				RunToken     string `json:"run_token"`
				Outcome      string `json:"outcome"`
				FailureClass string `json:"failure_class"`
			}
			if err := json.Unmarshal(payload.Params, &finish); err != nil || finish.AttemptID != lease.Attempt.ID || finish.RunToken != lease.RunToken || finish.Outcome != "failed" || finish.FailureClass != "workspace_isolation" {
				t.Errorf("finish payload = %#v, err=%v", finish, err)
				http.Error(writer, "invalid finish", http.StatusBadRequest)
				return
			}
			finished = true
			writeCLIEnvelope(t, writer, payload.Command, map[string]any{"finished": true})
		default:
			http.Error(writer, "unexpected command", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_test")
	t.Setenv("CONTENTCLOUD_AUTOMATION_ROOT", automationRoot)
	if err := localconfig.Save(localconfig.Config{ServerURL: server.URL, DeviceID: "device-1"}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runtime := &Root{stdout: &stdout, stderr: &stderr, now: func() time.Time { return now }}
	command := runtime.command()
	command.SetArgs([]string{"--json", "daemon", "run", "--once", "--fixture"})
	if err := command.Execute(); err == nil {
		t.Fatal("daemon unexpectedly ignored active workspace lease")
	}
	if !finished || strings.Join(commands, ",") != "daemon.poll,run.finish" {
		t.Fatalf("daemon failure flow commands=%#v finished=%v", commands, finished)
	}
}

func writeCLIEnvelope(t *testing.T, writer http.ResponseWriter, command string, data any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{"ok": true, "command": command, "request_id": "request-test", "meta": map[string]any{}, "data": data}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func TestConfiguredWorkspaceRootsIncludesEveryWorkspaceInBinding(t *testing.T) {
	t.Setenv("CONTENTCLOUD_WORKSPACE_ROOT", "")
	config := localconfig.Config{DaemonBindings: []localconfig.DaemonBinding{{
		ServerURL: "https://content.example.com", DeviceID: "device-1",
		Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", Root: "/work/one"}, {WorkspaceID: "workspace-2", Root: "/work/two"}, {WorkspaceID: "workspace-3", Root: "/work/one"}},
	}}}
	roots := configuredWorkspaceRoots(config)
	if len(roots) != 2 || roots[0] != "/work/one" || roots[1] != "/work/two" {
		t.Fatalf("interactive roots = %#v", roots)
	}
}

func TestDaemonRunsMultipleBindingsConcurrentlyThroughJournalAndReport(t *testing.T) {
	now := time.Date(2026, 7, 31, 15, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pollBarrier := make(chan struct{})
	reportsDone := make(chan struct{})
	var pollCount, reportCount atomic.Int32
	var barrierOnce, reportsOnce sync.Once

	newRuntimeServer := func(lease app.Lease) *httptest.Server {
		var leased atomic.Bool
		return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var payload struct {
				Command string          `json:"command"`
				Params  json.RawMessage `json:"params"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode request: %v", err)
				http.Error(writer, "invalid request", http.StatusBadRequest)
				return
			}
			switch payload.Command {
			case "daemon.poll":
				if !leased.CompareAndSwap(false, true) {
					writeCLIEnvelope(t, writer, payload.Command, app.DaemonPollResponse{Leased: false, Runtime: app.DaemonRuntimePolicy{CurrentVersion: Version}, PollAfterMS: 1000})
					return
				}
				if pollCount.Add(1) == 2 {
					barrierOnce.Do(func() { close(pollBarrier) })
				}
				select {
				case <-pollBarrier:
					writeCLIEnvelope(t, writer, payload.Command, app.DaemonPollResponse{Leased: true, Lease: &lease, Runtime: app.DaemonRuntimePolicy{CurrentVersion: Version}, PollAfterMS: 1000})
				case <-time.After(2 * time.Second):
					http.Error(writer, "bindings did not poll concurrently", http.StatusGatewayTimeout)
				}
			case "run.report":
				var report struct {
					RunID     string          `json:"run_id"`
					AttemptID string          `json:"attempt_id"`
					Package   json.RawMessage `json:"package"`
				}
				if err := json.Unmarshal(payload.Params, &report); err != nil || report.RunID != lease.Run.ID || report.AttemptID != lease.Attempt.ID || len(report.Package) == 0 {
					t.Errorf("invalid report for %s: %#v err=%v", lease.Run.ID, report, err)
					http.Error(writer, "invalid report", http.StatusBadRequest)
					return
				}
				writeCLIEnvelope(t, writer, payload.Command, map[string]any{"reported": true})
				if reportCount.Add(1) == 2 {
					go func() {
						timer := time.NewTimer(50 * time.Millisecond)
						defer timer.Stop()
						<-timer.C
						reportsOnce.Do(func() { close(reportsDone) })
					}()
				}
			default:
				http.Error(writer, "unexpected command", http.StatusBadRequest)
			}
		}))
	}

	leaseOne := daemonFixtureLease("one", "project-1", now)
	leaseTwo := daemonFixtureLease("two", "project-2", now)
	serverOne := newRuntimeServer(leaseOne)
	defer serverOne.Close()
	serverTwo := newRuntimeServer(leaseTwo)
	defer serverTwo.Close()
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", t.TempDir())
	t.Setenv("CONTENTCLOUD_DEVICE_TOKEN", "dt_shared_test")
	t.Setenv("CONTENTCLOUD_AUTOMATION_ROOT", filepath.Join(t.TempDir(), "automation"))
	if err := localconfig.Save(localconfig.Config{DaemonBindings: []localconfig.DaemonBinding{
		{ServerURL: serverOne.URL, DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1"}}},
		{ServerURL: serverTwo.URL, DeviceID: "device-2", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-2", ProjectID: "project-2"}}},
	}}); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "daemon.log")
	runtime := &Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, now: func() time.Time { return now }}
	command := runtime.command()
	command.SetArgs([]string{"--json", "daemon", "run", "--fixture", "--log-file", logPath})
	errCh := make(chan error, 1)
	go func() { errCh <- command.ExecuteContext(ctx) }()
	select {
	case <-reportsDone:
		cancel()
	case <-time.After(4 * time.Second):
		cancel()
		t.Fatal("multiple daemon bindings did not complete concurrently")
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after context cancellation")
	}
	if pollCount.Load() != 2 || reportCount.Load() != 2 {
		t.Fatalf("unexpected multi-binding flow: polls=%d reports=%d", pollCount.Load(), reportCount.Load())
	}
	pending, err := daemonJournalPendingCount()
	if err != nil || pending != 0 {
		t.Fatalf("multi-binding reports remain pending: count=%d err=%v", pending, err)
	}
}

func daemonFixtureLease(suffix, projectID string, now time.Time) app.Lease {
	runID := "run-" + suffix
	capability := domain.Capability{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true}
	return app.Lease{
		Run:            domain.TaskRun{ID: runID, ProjectID: projectID, TaskType: "knowledge_extract", OutputSchema: domain.KnowledgeCandidatesSchema, OutputCount: 1},
		Attempt:        domain.RunAttempt{ID: "attempt-" + suffix, ProjectID: projectID, RunID: runID, State: "leased"},
		Contract:       domain.TaskContract{ContractVersion: "1.0", ContractID: "snapshot-" + suffix, RunID: runID, TaskType: "knowledge_extract", Project: domain.Project{ID: projectID}, InputSnapshotID: "snapshot-" + suffix, OutputSchema: domain.KnowledgeCandidatesSchema, Capability: capability, ManifestHash: "sha256:" + strings.Repeat("c", 64)},
		LeaseExpiresAt: now.Add(5 * time.Minute), RunToken: "rt_" + suffix,
	}
}
