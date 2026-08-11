package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/bootstrapcheck"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

type bootstrapRunnerResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type bootstrapRunner struct {
	responses      []bootstrapRunnerResponse
	calls          [][]string
	rejectCanceled bool
}

func (r *bootstrapRunner) Run(ctx context.Context, command pluginhost.Command) (pluginhost.CommandResult, error) {
	r.calls = append(r.calls, append([]string{command.Name}, command.Args...))
	if r.rejectCanceled && ctx.Err() != nil {
		return pluginhost.CommandResult{}, ctx.Err()
	}
	if len(r.responses) == 0 {
		return pluginhost.CommandResult{}, errors.New("unexpected plugin host command")
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return pluginhost.CommandResult{Stdout: []byte(response.stdout), Stderr: []byte(response.stderr), ExitCode: response.exitCode}, response.err
}

func TestBootstrapPlanIsReadOnlyAndUsesOnlyPublicSessionID(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: `{"marketplaces":[]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	var stdout, stderr bytes.Buffer
	root := &Root{stdout: &stdout, stderr: &stderr, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), bootstrapCheckHook: healthyBootstrapCheck}
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", "https://content.example.com", "bootstrap", "plan", directory, "--session", testBootstrapSessionID})
	if err := command.Execute(); err != nil {
		t.Fatalf("bootstrap plan failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool          `json:"ok"`
		Data bootstrapPlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v; output=%s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.State != "ready" || !strings.HasPrefix(envelope.Data.PlanID, "bp_") || envelope.Data.CLIPackage != "@limecloud/contentcloud@0.25.0" || len(envelope.Data.Plugin.Actions) != 7 || !envelope.Data.WouldEnableDaemon {
		t.Fatalf("unexpected plan: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "connect_key") || envelope.Data.AuthorizationMode != "browser_device" || !envelope.Data.WouldAuthorizeDevice {
		t.Fatalf("bootstrap plan did not use browser-only authorization: %s", stdout.String())
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("bootstrap plan created the target directory: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("bootstrap plan ran a mutation: %#v", runner.calls)
	}
}

func TestBootstrapPlanIDIsStableUntilInputsChange(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	first := bootstrapPlanIDForTest(t, directory, "https://content.example.com")
	second := bootstrapPlanIDForTest(t, directory, "https://content.example.com")
	if first != second {
		t.Fatalf("unchanged bootstrap plan IDs differ: %q != %q", first, second)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "existing.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := bootstrapPlanIDForTest(t, directory, "https://content.example.com")
	if changed == first {
		t.Fatalf("directory state change did not invalidate plan ID %q", first)
	}
}

func TestBootstrapDiagnosticsRequiresPreviewAndUploadConfirmation(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	attemptID := "22222222-2222-4222-8222-222222222222"
	var stdout, stderr bytes.Buffer
	root := &Root{stdout: &stdout, stderr: &stderr, bootstrapCheckHook: healthyBootstrapCheck}
	command := root.command()
	command.SetArgs([]string{"--json", "bootstrap", "diagnostics", directory, "--attempt", attemptID})
	if err := command.Execute(); err != nil {
		t.Fatalf("diagnostic preview failed: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Redacted                   bool `json:"redacted"`
			Uploaded                   bool `json:"uploaded"`
			RequiresUploadConfirmation bool `json:"requires_upload_confirmation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.Redacted || envelope.Data.Uploaded || !envelope.Data.RequiresUploadConfirmation {
		t.Fatalf("unexpected diagnostic preview: error=%v output=%s", err, stdout.String())
	}

	root = &Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, bootstrapCheckHook: healthyBootstrapCheck}
	command = root.command()
	command.SetArgs([]string{"--json", "bootstrap", "diagnostics", directory, "--attempt", attemptID, "--upload"})
	err := command.Execute()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "BOOTSTRAP_DIAGNOSTIC_CONFIRMATION_REQUIRED" {
		t.Fatalf("unexpected upload confirmation error: %#v", err)
	}
}

func TestBootstrapApplyInstallsInitializesDoctorsAndRegisters(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	manifest, verifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	server, registered := newBootstrapRegisterServer(t, manifest, registry)
	defer server.Close()

	runner := successfulBootstrapRunner()
	planID := bootstrapPlanIDForTest(t, directory, server.URL)
	daemon := &fakeUserDaemonService{}
	var stdout, stderr bytes.Buffer
	root := &Root{
		stdout:               &stdout,
		stderr:               &stderr,
		pluginRunner:         runner,
		pluginRuntimeHook:    testPluginRuntimeHook(t, pluginhost.StatusAbsent),
		now:                  func() time.Time { return now },
		manifestVerifierHook: fixedManifestVerifier(verifier),
		registryVerifierHook: fixedRegistryVerifier(registryVerifier),
		bootstrapCheckHook:   healthyBootstrapCheck,
		daemonFactory:        fakeDaemonFactory(daemon),
		bootstrapAuthorizeHook: func(_ context.Context, sessionID, _ string) (localconfig.Config, app.ConnectDeviceResult, *bootstrapProgressReporter, error) {
			if sessionID != testBootstrapSessionID {
				t.Fatalf("unexpected ConnectSession: %s", sessionID)
			}
			return localconfig.Config{ServerURL: server.URL, DaemonBindings: []localconfig.DaemonBinding{{ServerURL: server.URL, DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1"}}}}}, app.ConnectDeviceResult{
				Device: domain.Device{ID: "device-1"}, WorkspaceID: "workspace-1", WorkspaceToken: "wt_test", ProjectID: "project-1", EnvironmentManifest: &manifest,
			}, nil, nil
		},
	}
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", server.URL, "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--plan-id", planID, "--accept", "--open-host=false"})
	if err := command.Execute(); err != nil {
		t.Fatalf("bootstrap apply failed: %v; stderr=%s", err, stderr.String())
	}
	if !*registered {
		t.Fatal("bootstrap did not register the workspace after doctor")
	}
	if daemon.startCalls != 1 {
		t.Fatalf("bootstrap did not start daemon exactly once: %d", daemon.startCalls)
	}
	status, err := localworkspace.LoadStatus(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(status.Template.Targets, []string{"codex-plugin"}) {
		t.Fatalf("unexpected targets: %#v", status.Template.Targets)
	}
	if _, err := os.Stat(filepath.Join(directory, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("codex-plugin target wrote project config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatalf("codex-plugin target duplicated plugin skills: %v", err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Doctor               localworkspace.DoctorReport `json:"doctor"`
			Workspace            localworkspace.Status       `json:"workspace"`
			BootstrapHandoffPath string                      `json:"bootstrap_handoff_path"`
			DaemonEnabled        bool                        `json:"daemon_enabled"`
			Daemon               userDaemonState             `json:"daemon"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.Doctor.OK || envelope.Data.BootstrapHandoffPath == "" || !envelope.Data.DaemonEnabled || !envelope.Data.Daemon.Running || !envelope.Data.Workspace.AutomationEnabled {
		t.Fatalf("unexpected output: err=%v output=%s", err, stdout.String())
	}
	if _, err := os.Stat(envelope.Data.BootstrapHandoffPath); err != nil {
		t.Fatalf("bootstrap handoff was not persisted: %v", err)
	}
}

func TestBootstrapApplyUpgradesExistingPluginAndInitializesWorkspace(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "upgraded-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	manifest, verifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	server, registered := newBootstrapRegisterServer(t, manifest, registry)
	defer server.Close()

	runner := successfulBootstrapUpgradeRunner()
	daemon := &fakeUserDaemonService{}
	var stdout, stderr bytes.Buffer
	root := &Root{
		stdout:               &stdout,
		stderr:               &stderr,
		serverURL:            server.URL,
		pluginRunner:         runner,
		pluginRuntimeHook:    testPluginRuntimeHook(t, pluginhost.StatusInstalled),
		now:                  func() time.Time { return now },
		manifestVerifierHook: fixedManifestVerifier(verifier),
		registryVerifierHook: fixedRegistryVerifier(registryVerifier),
		bootstrapCheckHook:   healthyBootstrapCheck,
		daemonFactory:        fakeDaemonFactory(daemon),
		bootstrapAuthorizeHook: func(_ context.Context, sessionID, _ string) (localconfig.Config, app.ConnectDeviceResult, *bootstrapProgressReporter, error) {
			if sessionID != testBootstrapSessionID {
				t.Fatalf("unexpected ConnectSession: %s", sessionID)
			}
			return localconfig.Config{ServerURL: server.URL, DaemonBindings: []localconfig.DaemonBinding{{ServerURL: server.URL, DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1"}}}}}, app.ConnectDeviceResult{
				Device: domain.Device{ID: "device-1"}, WorkspaceID: "workspace-1", WorkspaceToken: "wt_test", ProjectID: "project-1", EnvironmentManifest: &manifest,
			}, nil, nil
		},
	}
	plan, _, err := root.buildBootstrapPlan(t.Context(), directory, "codex")
	if err != nil {
		t.Fatal(err)
	}
	plan, err = root.withBootstrapPrerequisites(t.Context(), plan, testBootstrapSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Plugin.State != pluginhost.StatusStaged || len(plan.Plugin.Actions) == 0 {
		t.Fatalf("unexpected upgrade plan: %#v", plan.Plugin)
	}

	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", server.URL, "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--plan-id", plan.PlanID, "--accept", "--open-host=false"})
	if err := command.Execute(); err != nil {
		t.Fatalf("bootstrap upgrade apply failed: %v; stderr=%s", err, stderr.String())
	}
	if !*registered {
		t.Fatal("bootstrap upgrade did not register the workspace")
	}
	if daemon.startCalls != 1 {
		t.Fatalf("bootstrap upgrade did not reload daemon: %d", daemon.startCalls)
	}
	status, err := localworkspace.LoadStatus(directory)
	if err != nil {
		t.Fatalf("upgraded workspace status is unreadable: %v", err)
	}
	doctor, err := localworkspace.Doctor(directory)
	if err != nil || !doctor.OK {
		t.Fatalf("upgraded workspace is unhealthy: status=%#v doctor=%#v error=%v", status, doctor, err)
	}
}

func TestBootstrapResumeInitializesEmptyDirectoryFromSavedBinding(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "recovered-workspace")
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", configPath)
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test")
	now := time.Date(2026, 7, 27, 10, 30, 0, 0, time.UTC)
	manifest, verifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	server, registered := newBootstrapRegisterServer(t, manifest, registry)
	defer server.Close()
	if err := localconfig.Save(localconfig.Config{ServerURL: server.URL, DaemonBindings: []localconfig.DaemonBinding{{ServerURL: server.URL, DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1"}}}}}); err != nil {
		t.Fatal(err)
	}
	runner := successfulBootstrapRunner()
	daemon := &fakeUserDaemonService{}
	var stdout, stderr bytes.Buffer
	root := &Root{stdout: &stdout, stderr: &stderr, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), now: func() time.Time { return now }, manifestVerifierHook: fixedManifestVerifier(verifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier), daemonFactory: fakeDaemonFactory(daemon)}
	command := root.command()
	command.SetArgs([]string{"--json", "bootstrap", "resume", directory, "--accept", "--open-host=false"})
	if err := command.Execute(); err != nil {
		t.Fatalf("bootstrap resume failed: %v; stderr=%s", err, stderr.String())
	}
	if !*registered {
		t.Fatal("bootstrap resume did not register the saved Workspace binding")
	}
	if daemon.startCalls != 1 {
		t.Fatalf("bootstrap resume did not start daemon: %d", daemon.startCalls)
	}
	status, err := localworkspace.LoadStatus(directory)
	if err != nil {
		t.Fatal(err)
	}
	if status.Binding.ProjectID != "project-1" || status.Binding.WorkspaceID != "workspace-1" || !hasBootstrapTarget(status.Template.Targets) {
		t.Fatalf("unexpected recovered workspace: %#v", status)
	}
}

func TestBootstrapResumeUpgradesExistingPluginWithoutReinitializingWorkspace(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", configPath)
	t.Setenv("CONTENTCLOUD_WORKSPACE_TOKEN", "wt_test")
	now := time.Date(2026, 7, 29, 11, 0, 0, 0, time.UTC)
	manifest, verifier, registry, registryVerifier := bootstrapEnvironmentFixture(t, now)
	server, registered := newBootstrapRegisterServer(t, manifest, registry)
	defer server.Close()
	if _, err := localworkspace.Initialize(localworkspace.InitOptions{
		Root: directory, ProjectID: "project-1", WorkspaceID: "workspace-1", DeviceID: "device-1",
		ServerURL: server.URL, CLIVersion: "0.7.0", Target: "codex-plugin", Now: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(directory, "50-production", "scripts", "existing.json")
	if err := os.WriteFile(sentinelPath, []byte("existing business content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := localconfig.Save(localconfig.Config{ServerURL: server.URL, DaemonBindings: []localconfig.DaemonBinding{{ServerURL: server.URL, DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: "workspace-1", ProjectID: "project-1", Root: directory}}}}}); err != nil {
		t.Fatal(err)
	}

	runner := successfulBootstrapUpgradeRunner()
	daemon := &fakeUserDaemonService{}
	var stdout, stderr bytes.Buffer
	root := &Root{
		stdout: &stdout, stderr: &stderr, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusInstalled), now: func() time.Time { return now },
		manifestVerifierHook: fixedManifestVerifier(verifier), registryVerifierHook: fixedRegistryVerifier(registryVerifier),
		daemonFactory: fakeDaemonFactory(daemon),
	}
	command := root.command()
	command.SetArgs([]string{"--json", "bootstrap", "resume", directory, "--accept", "--open-host=false"})
	if err := command.Execute(); err != nil {
		t.Fatalf("bootstrap resume upgrade failed: %v; stderr=%s", err, stderr.String())
	}
	if !*registered {
		t.Fatal("bootstrap resume upgrade did not register the existing Workspace")
	}
	if daemon.startCalls != 1 {
		t.Fatalf("bootstrap resume upgrade did not reload daemon: %d", daemon.startCalls)
	}
	body, err := os.ReadFile(sentinelPath)
	if err != nil || string(body) != "existing business content" {
		t.Fatalf("bootstrap resume changed existing business content: body=%q error=%v", body, err)
	}
	status, err := localworkspace.LoadStatus(directory)
	if err != nil {
		t.Fatal(err)
	}
	if status.Binding.ProjectID != "project-1" || status.Template.CLIVersion != "0.7.0" {
		t.Fatalf("bootstrap resume silently reinitialized the Workspace: %#v", status)
	}
}

func TestBootstrapApplyAuthorizationFailureDoesNotMutatePluginOrWorkspace(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{{stdout: `{"marketplaces":[]}`}, {stdout: `{"installed":[],"available":[]}`}}}
	root := &Root{
		stdout:               &bytes.Buffer{},
		stderr:               &bytes.Buffer{},
		pluginRunner:         runner,
		pluginRuntimeHook:    testPluginRuntimeHook(t, pluginhost.StatusAbsent),
		manifestVerifierHook: fixedManifestVerifier(testManifestVerifier(t)),
		registryVerifierHook: fixedRegistryVerifier(testRegistryVerifier(t)),
		bootstrapCheckHook:   healthyBootstrapCheck,
		bootstrapAuthorizeHook: func(context.Context, string, string) (localconfig.Config, app.ConnectDeviceResult, *bootstrapProgressReporter, error) {
			return localconfig.Config{}, app.ConnectDeviceResult{}, nil, domain.Conflict("BOOTSTRAP_AUTHORIZATION_DENIED", "用户拒绝授权")
		},
	}
	planID := bootstrapPlanIDForTest(t, directory, "https://content.example.com")
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", "https://content.example.com", "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--plan-id", planID, "--accept", "--open-codex=false"})
	if err := command.Execute(); err == nil {
		t.Fatal("authorization failure must fail bootstrap")
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("authorization failure wrote the workspace: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("authorization failure mutated Codex: %#v", runner.calls)
	}
}

func TestBootstrapApplyRejectsUnconfirmedPlanID(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: `{"marketplaces":[]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	root := &Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), bootstrapCheckHook: healthyBootstrapCheck}
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", "https://content.example.com", "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--plan-id", "bp_wrong", "--accept", "--open-host=false"})
	err := command.Execute()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "BOOTSTRAP_PLAN_STALE" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("stale plan ran a mutation: %#v", runner.calls)
	}
}

func TestBootstrapApplyRequiresPlanIDBeforeMutation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: `{"marketplaces":[]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	root := &Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), bootstrapCheckHook: healthyBootstrapCheck}
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", "https://content.example.com", "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--accept", "--open-host=false"})
	err := command.Execute()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "BOOTSTRAP_PLAN_ID_REQUIRED" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("missing plan ID ran a mutation: %#v", runner.calls)
	}
}

func TestBootstrapApplyRejectsPlanAfterCodexStateChanges(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "new-workspace")
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	approvedPlanID := bootstrapPlanIDForTest(t, directory, "https://content.example.com")
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.21.0"}}]}`},
		{stdout: `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.21.0","installed":true,"enabled":true}],"available":[]}`},
	}}
	root := &Root{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusReady), bootstrapCheckHook: healthyBootstrapCheck}
	command := root.command()
	command.SetArgs([]string{"--json", "--server-url", "https://content.example.com", "bootstrap", "apply", directory, "--session", testBootstrapSessionID, "--plan-id", approvedPlanID, "--accept", "--open-host=false"})
	err := command.Execute()
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "BOOTSTRAP_PLAN_STALE" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("changed Codex state ran a mutation: %#v", runner.calls)
	}
}

func TestValidateBootstrapServerRequiresOrigin(t *testing.T) {
	for _, value := range []string{"https://content.example.com", "https://content.example.com/", "http://localhost:8080"} {
		if err := validateBootstrapServer(value); err != nil {
			t.Fatalf("valid origin %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{
		"content.example.com",
		"ftp://content.example.com",
		"https://user:secret@content.example.com",
		"https://content.example.com/api",
		"https://content.example.com?target=other",
		"https://content.example.com#fragment",
	} {
		if err := validateBootstrapServer(value); err == nil {
			t.Fatalf("non-origin server URL %q accepted", value)
		}
	}
}

func TestBootstrapVerificationURLMustMatchServerOrigin(t *testing.T) {
	got, err := sameOriginBootstrapURL("https://content.example.com", "https://content.example.com/projects/project/overview?bootstrap_attempt=attempt")
	if err != nil || got == "" {
		t.Fatalf("same-origin verification URL rejected: url=%q error=%v", got, err)
	}
	for _, value := range []string{
		"https://evil.example.com/projects/project/overview",
		"http://content.example.com/projects/project/overview",
		"https://user:secret@content.example.com/projects/project/overview",
	} {
		if _, err := sameOriginBootstrapURL("https://content.example.com", value); err == nil {
			t.Fatalf("unsafe verification URL %q accepted", value)
		}
	}
}

func TestRequireHealthyWorkspaceBlocksRegistration(t *testing.T) {
	report := localworkspace.DoctorReport{OK: false, Root: "/tmp/workspace", Checks: map[string]localworkspace.Check{
		"managed_files": {OK: false, Required: true, Message: "drift"},
	}}
	err := requireHealthyWorkspace(report)
	var domainError *domain.Error
	if !errors.As(err, &domainError) || domainError.Code != "WORKSPACE_DOCTOR_FAILED" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func successfulBootstrapRunner() *bootstrapRunner {
	missingMarketplace := `{"marketplaces":[]}`
	missingPlugin := `{"installed":[],"available":[]}`
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.25.0"}}]}`
	currentPlugin := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.25.0","installed":true,"enabled":true}],"available":[]}`
	return &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: missingMarketplace}, {stdout: missingPlugin},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stdout: `{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.25.0","installedPath":"/tmp/plugin"}`},
		{stdout: currentMarketplace}, {stdout: currentPlugin},
	}}
}

func successfulBootstrapUpgradeRunner() *bootstrapRunner {
	oldMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache-old","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.7.0"}}]}`
	oldPlugin := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.7.0","installed":true,"enabled":true}],"available":[]}`
	currentMarketplace := `{"marketplaces":[{"name":"contentcloud","root":"/tmp/cache","marketplaceSource":{"sourceType":"git","source":"limecloud/contentcloud","ref":"v0.25.0"}}]}`
	currentPlugin := `{"installed":[{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.25.0","installed":true,"enabled":true}],"available":[]}`
	return &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: oldMarketplace}, {stdout: oldPlugin},
		{stdout: oldMarketplace}, {stdout: oldPlugin},
		{stdout: oldMarketplace}, {stdout: oldPlugin},
		{stdout: `{}`},
		{stdout: `{}`},
		{stdout: `{"marketplaceName":"contentcloud","installedRoot":"/tmp/cache","alreadyAdded":false}`},
		{stdout: `{"pluginId":"contentcloud-video-production@contentcloud","name":"contentcloud-video-production","marketplaceName":"contentcloud","version":"0.25.0","installedPath":"/tmp/plugin"}`},
		{stdout: currentMarketplace}, {stdout: currentPlugin},
	}}
}

func bootstrapPlanIDForTest(t *testing.T, directory, serverURL string) string {
	t.Helper()
	runner := &bootstrapRunner{responses: []bootstrapRunnerResponse{
		{stdout: `{"marketplaces":[]}`},
		{stdout: `{"installed":[],"available":[]}`},
	}}
	root := &Root{serverURL: serverURL, pluginRunner: runner, pluginRuntimeHook: testPluginRuntimeHook(t, pluginhost.StatusAbsent), bootstrapCheckHook: healthyBootstrapCheck}
	plan, _, err := root.buildBootstrapPlan(t.Context(), directory, "codex")
	if err != nil {
		t.Fatal(err)
	}
	plan, err = root.withBootstrapPrerequisites(t.Context(), plan, testBootstrapSessionID)
	if err != nil {
		t.Fatal(err)
	}
	return plan.PlanID
}

type testBootstrapHost struct {
	status pluginhost.Status
}

func (h *testBootstrapHost) ID() pluginhost.HostID { return pluginhost.HostCodex }

func (h *testBootstrapHost) Capabilities(context.Context) (pluginhost.Capabilities, error) {
	return pluginhost.Capabilities{Skills: true, MCPStdio: true, Rollback: true}, nil
}

func (h *testBootstrapHost) Detect(_ context.Context, _ pluginhost.HostTarget) (pluginhost.State, error) {
	return pluginhost.State{SchemaVersion: pluginhost.SchemaVersion, HostID: h.ID(), Status: h.status, Generation: "test-generation", Capabilities: pluginhost.Capabilities{Skills: true, MCPStdio: true, Rollback: true}}, nil
}

func (h *testBootstrapHost) Apply(context.Context, pluginhost.NativeApply) (pluginhost.NativeChange, []pluginhost.InstalledComponent, error) {
	h.status = pluginhost.StatusReady
	return pluginhost.NativeChange{Data: []byte(`{"test":true}`)}, nil, nil
}

func (h *testBootstrapHost) Remove(context.Context, pluginhost.NativeRemove) (pluginhost.NativeChange, error) {
	h.status = pluginhost.StatusAbsent
	return pluginhost.NativeChange{Data: []byte(`{"test":true}`)}, nil
}

func (h *testBootstrapHost) Rollback(context.Context, pluginhost.NativeChange) error { return nil }
func (h *testBootstrapHost) Commit(context.Context, pluginhost.NativeChange) error   { return nil }

func testPluginRuntimeHook(t *testing.T, initial pluginhost.Status) func(string) (*hostPluginRuntime, error) {
	t.Helper()
	pkg, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.VideoProduction, Version)
	if err != nil {
		t.Fatal(err)
	}
	store, err := pluginhost.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	native := &testBootstrapHost{status: initial}
	adapter, err := pluginhost.New(native, store)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &hostPluginRuntime{Adapter: adapter, Package: pkg, HostID: pluginhost.HostCodex}
	return func(string) (*hostPluginRuntime, error) { return runtime, nil }
}

const testBootstrapSessionID = "11111111-1111-4111-8111-111111111111"

func healthyBootstrapCheck(context.Context, bootstrapcheck.Options) bootstrapcheck.Report {
	return bootstrapcheck.Report{SchemaVersion: domain.BootstrapSchemaVersion, OK: true, Platform: "darwin", Arch: "arm64", Checks: []bootstrapcheck.Check{{Stage: "prerequisites", CheckID: "runtime.platform.supported", Status: "passed", Facts: map[string]any{"platform": "darwin", "arch": "arm64", "supported": true}}}}
}

func newBootstrapRegisterServer(t *testing.T, manifest environment.Manifest, registry environment.Registry) (*httptest.Server, *bool) {
	t.Helper()
	registered := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/cli/dispatch" || request.Header.Get("Authorization") != "Bearer wt_test" {
			http.Error(writer, "unexpected request", http.StatusBadRequest)
			return
		}
		var payload struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(writer, "unexpected payload", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if payload.Command == "environment.manifest.get" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"ok": true, "command": payload.Command, "request_id": "request-test", "meta": map[string]any{}, "data": manifest})
			return
		}
		if payload.Command == "environment.registry.get" {
			_ = json.NewEncoder(writer).Encode(map[string]any{"ok": true, "command": payload.Command, "request_id": "request-test", "meta": map[string]any{}, "data": registry})
			return
		}
		if payload.Command != "workspace.register" {
			http.Error(writer, "unexpected command", http.StatusBadRequest)
			return
		}
		registered = true
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ok": true, "command": "workspace.register", "request_id": "request-test", "meta": map[string]any{},
			"data": domain.WorkspaceBinding{ID: "workspace-1", ProjectID: "project-1", TemplateID: localworkspace.TemplateID, TemplateVersion: localworkspace.TemplateVersion, Targets: []string{"codex-plugin"}, Status: "active"},
		})
	}))
	return server, &registered
}

func fixedManifestVerifier(verifier *environment.Verifier) func() (*environment.Verifier, error) {
	return func() (*environment.Verifier, error) { return verifier, nil }
}

func fixedRegistryVerifier(verifier *environment.RegistryVerifier) func() (*environment.RegistryVerifier, error) {
	return func() (*environment.RegistryVerifier, error) { return verifier, nil }
}

func testManifestVerifier(t *testing.T) *environment.Verifier {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "bootstrap-test", Status: "active", PublicKey: publicKey}})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func testRegistryVerifier(t *testing.T) *environment.RegistryVerifier {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "registry-bootstrap-test", Status: "active", PublicKey: publicKey}})
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func bootstrapEnvironmentFixture(t *testing.T, now time.Time) (environment.Manifest, *environment.Verifier, environment.Registry, *environment.RegistryVerifier) {
	t.Helper()
	standardPackage, err := pluginbuiltin.Load(t.TempDir(), pluginidentity.VideoProduction, Version)
	if err != nil {
		t.Fatal(err)
	}
	registryPublicKey, registryPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sceneEntry := environment.RegistryEntry{
		ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version,
		Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: "v" + Version}, License: "Apache-2.0", Digest: standardPackage.Digest,
		Signature: environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-bootstrap-test"}, CompatibleProfiles: []string{"contentcloud.video-production"},
		Permissions: []string{"workspace:read"}, DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/content-item-3.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: "evaluation.json", Digest: "sha256:" + strings.Repeat("e", 64), Evidence: []string{"test"}}, Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}
	packEntry := environment.RegistryEntry{
		ID: "contentcloud-visual-storytelling", Kind: "skill_pack", Version: "1.2.0",
		Source: environment.RegistrySource{Repository: "https://github.com/limecloud/contentcloud", Ref: "v" + Version}, License: "Apache-2.0", Digest: "sha256:" + strings.Repeat("b", 64),
		Signature: environment.RegistrySignature{Status: "verified", Algorithm: "ed25519", KeyID: "plugin-release-bootstrap-test"}, CompatibleProfiles: []string{"contentcloud.video-production"},
		Permissions: []string{"workspace:read", "workspace:write-managed"}, DataFlow: environment.RegistryDataFlow{LocalByDefault: true, CloudActions: []string{}}, OutputSchemas: []string{"contracts/content-item-3.0.schema.json"},
		Cost:       environment.RegistryCost{Model: "included", Notice: "Included in tests."},
		Evaluation: environment.RegistryEvaluation{Status: "passed", Report: "evaluation-pack.json", Digest: "sha256:" + strings.Repeat("f", 64), Evidence: []string{"test"}}, Lifecycle: "published", Revocation: environment.RegistryRevocation{Status: "active"},
	}
	entries := []environment.RegistryEntry{sceneEntry, packEntry}
	for index := range entries {
		payload, payloadErr := environment.RegistryEntrySigningPayload(entries[index])
		if payloadErr != nil {
			t.Fatal(payloadErr)
		}
		entries[index].Signature.Value = base64.StdEncoding.EncodeToString(ed25519.Sign(registryPrivateKey, payload))
	}
	registryVerifier, err := environment.NewRegistryVerifier([]environment.RegistryTrustedKey{{KeyID: "plugin-release-bootstrap-test", Status: "active", PublicKey: registryPublicKey}})
	if err != nil {
		t.Fatal(err)
	}
	rawRegistry := environment.Registry{SchemaVersion: "1.0", Entries: entries}
	registry, err := registryVerifier.Verify(rawRegistry)
	if err != nil {
		t.Fatal(err)
	}
	profile := environment.Profile{
		ID: "contentcloud.video-production", Version: "1.0.0", EnvironmentVersion: "2026.7.1", Harness: "codex", Marketplace: "contentcloud",
		Plugins: []environment.ProfilePlugin{
			{ID: "contentcloud-video-production", Kind: "scene_plugin", Version: Version, Required: true, Scope: "environment", Capabilities: []string{domain.KnowledgeExtractCapability}},
			{ID: "contentcloud-visual-storytelling", Kind: "skill_pack", Version: "1.2.0", Required: false, Scope: "task", Capabilities: []string{"contentcloud.asset.generate"}},
		},
		WorkspaceTemplate: environment.WorkspaceTemplateRef{ID: localworkspace.TemplateID, Version: localworkspace.TemplateVersion, Digest: "sha256:" + strings.Repeat("c", 64)}, Capabilities: []string{domain.KnowledgeExtractCapability}, Policies: environment.Policies{PublishRequiresConfirmation: true, AutomationEnabled: true},
	}
	profile.Capabilities = append(profile.Capabilities, "contentcloud.asset.generate")
	unsigned, err := environment.BuildManifest("project-1", []string{domain.ContentTypeVideoScript}, profile, registry, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := environment.NewIssuer("environment-bootstrap-test", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := issuer.Sign(unsigned)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := environment.NewVerifier([]environment.TrustedKey{{KeyID: "environment-bootstrap-test", Status: "active", PublicKey: publicKey}})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, verifier, rawRegistry, registryVerifier
}
