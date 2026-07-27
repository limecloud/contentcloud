package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "script_generate",
		Project: domain.Project{ID: "project-1"}, InputSnapshotID: "snapshot-1", OutputSchema: domain.ScriptPackageSchema,
		Capability:   domain.Capability{ID: domain.ScriptCapability, Version: "1.1.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
		ManifestHash: "sha256:" + strings.Repeat("c", 64),
	}
	bundle := environment.CreativeExecutionBundle{BundleID: "ceb_test", ProjectID: "project-1", Digest: "sha256:" + strings.Repeat("b", 64)}
	lease := app.Lease{
		Run:      domain.TaskRun{ID: "run-1", ProjectID: "project-1", TaskType: "script_generate", OutputSchema: domain.ScriptPackageSchema, OutputCount: 1},
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
		case "artifact.open.poll":
			writer.WriteHeader(http.StatusNoContent)
			return
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
	if !workspaceInspected || strings.Join(commands, ",") != "artifact.open.poll,daemon.poll,run.report" {
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

func TestDaemonFinishesAttemptWhenWorkspaceIsolationFails(t *testing.T) {
	now := time.Date(2026, 7, 27, 17, 0, 0, 0, time.UTC)
	automationRoot := filepath.Join(t.TempDir(), "automation")
	contract := domain.TaskContract{
		ContractVersion: "1.0", ContractID: "snapshot-1", RunID: "run-1", TaskType: "script_generate",
		Project: domain.Project{ID: "project-1"}, InputSnapshotID: "snapshot-1", OutputSchema: domain.ScriptPackageSchema,
		Capability:   domain.Capability{ID: domain.ScriptCapability, Version: "1.1.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, Digest: "sha256:" + strings.Repeat("a", 64), LocalOnly: true},
		ManifestHash: "sha256:" + strings.Repeat("c", 64),
	}
	lease := app.Lease{
		Run:     domain.TaskRun{ID: "run-1", ProjectID: "project-1", TaskType: "script_generate", OutputSchema: domain.ScriptPackageSchema, OutputCount: 1},
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
		case "artifact.open.poll":
			writer.WriteHeader(http.StatusNoContent)
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
	if !finished || strings.Join(commands, ",") != "artifact.open.poll,daemon.poll,run.finish" {
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
