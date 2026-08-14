package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/localconfig"
)

type fakeUserDaemonService struct {
	state        userDaemonState
	startCalls   int
	stopCalls    int
	restartCalls int
}

func (s *fakeUserDaemonService) Start() (userDaemonState, error) {
	s.startCalls++
	s.state.Supported, s.state.Installed, s.state.Running = true, true, true
	s.state.Version = Version
	return s.state, nil
}

func (s *fakeUserDaemonService) Stop() (userDaemonState, error) {
	s.stopCalls++
	s.state.Running = false
	return s.state, nil
}

func (s *fakeUserDaemonService) Restart() (userDaemonState, error) {
	s.restartCalls++
	return s.Start()
}

func (s *fakeUserDaemonService) Status() (userDaemonState, error) { return s.state, nil }
func (s *fakeUserDaemonService) Uninstall() error                 { return nil }

func fakeDaemonFactory(service *fakeUserDaemonService) func() (userDaemonService, error) {
	return func() (userDaemonService, error) { return service, nil }
}

func TestLaunchdDaemonStartIsIdempotentAndVersionAware(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	loaded := false
	pid := 4242
	calls := [][]string{}
	runner := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		switch args[0] {
		case "print":
			if !loaded {
				return nil, errors.New("service not loaded")
			}
			return []byte("state = running\npid = 4242\n"), nil
		case "bootout":
			loaded = false
			return nil, nil
		case "bootstrap", "kickstart":
			loaded = true
			return nil, nil
		default:
			return nil, errors.New("unexpected launchctl command")
		}
	}
	launchEnvironment := map[string]string{"HOME": "/Users/test", "PATH": "/opt/homebrew/bin:/usr/bin", "CODEX_HOME": "/Users/test/.codex"}
	service := &launchdDaemonService{home: home, executable: "/opt/contentcloud-0.10.0", version: "0.10.0", uid: 501, now: func() time.Time { return now }, run: runner, environment: launchEnvironment}
	state, err := service.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Installed || !state.Running || state.PID != pid || state.Version != "0.10.0" || state.Executable != "/opt/contentcloud-0.10.0" {
		t.Fatalf("unexpected daemon state: %#v", state)
	}
	plist, err := os.ReadFile(service.plistPath())
	if err != nil || !strings.Contains(string(plist), "/opt/contentcloud-0.10.0") || !strings.Contains(string(plist), "KeepAlive") || !strings.Contains(string(plist), "/opt/homebrew/bin:/usr/bin") || !strings.Contains(string(plist), "CODEX_HOME") {
		t.Fatalf("invalid LaunchAgent: error=%v body=%s", err, plist)
	}
	firstCalls := len(calls)
	state, err = service.Start()
	if err != nil || !state.AlreadyRunning || len(calls) != firstCalls+1 || calls[len(calls)-1][1] != "print" {
		t.Fatalf("idempotent start restarted daemon: state=%#v error=%v calls=%#v", state, err, calls)
	}

	upgraded := &launchdDaemonService{home: home, executable: "/opt/contentcloud-0.19.0", version: "0.19.0", uid: 501, now: func() time.Time { return now.Add(time.Hour) }, run: runner, environment: launchEnvironment}
	state, err = upgraded.Start()
	if err != nil || state.Version != "0.19.0" || state.Executable != "/opt/contentcloud-0.19.0" || !state.Running {
		t.Fatalf("version-aware start did not reload daemon: state=%#v error=%v", state, err)
	}
	wantTail := [][]string{
		{"launchctl", "print", "gui/501/com.goodvision.contentcloud"},
		{"launchctl", "bootout", "gui/501/com.goodvision.contentcloud"},
		{"launchctl", "bootstrap", "gui/501", upgraded.plistPath()},
		{"launchctl", "kickstart", "-k", "gui/501/com.goodvision.contentcloud"},
		{"launchctl", "print", "gui/501/com.goodvision.contentcloud"},
	}
	if !reflect.DeepEqual(calls[len(calls)-len(wantTail):], wantTail) {
		t.Fatalf("upgrade restart sequence=%#v", calls)
	}
	metadata, err := os.ReadFile(filepath.Join(home, "Library", "Application Support", "ContentCloud", "daemon.json"))
	if err != nil || !strings.Contains(string(metadata), `"version": "0.19.0"`) {
		t.Fatalf("daemon metadata was not upgraded: error=%v body=%s", err, metadata)
	}
}

func TestParseLaunchdStatus(t *testing.T) {
	pid, running := parseLaunchdStatus([]byte("state = running\npid = 9087\n"))
	if pid != 9087 || !running {
		t.Fatalf("launchd status parse: pid=%d running=%t", pid, running)
	}
}

func TestDaemonLaunchEnvironmentKeepsRuntimePathsWithoutPersistingAPIKeys(t *testing.T) {
	t.Setenv("PATH", "/opt/homebrew/bin:/usr/bin")
	t.Setenv("CODEX_HOME", "/Users/test/.codex")
	t.Setenv("OPENAI_API_KEY", "must-not-be-persisted")
	t.Setenv("ANTHROPIC_API_KEY", "must-not-be-persisted")
	values := daemonLaunchEnvironment("/Users/test")
	if values["HOME"] != "/Users/test" || values["PATH"] != "/opt/homebrew/bin:/usr/bin" || values["CODEX_HOME"] != "/Users/test/.codex" {
		t.Fatalf("daemon runtime paths were not preserved: %#v", values)
	}
	if values["OPENAI_API_KEY"] != "" || values["ANTHROPIC_API_KEY"] != "" {
		t.Fatalf("daemon plist would persist API keys: %#v", values)
	}
}

func TestDaemonRestartIfInstalledSkipsWithoutRegistrationOrCredentials(t *testing.T) {
	t.Setenv("CONTENTCLOUD_CONFIG_PATH", filepath.Join(t.TempDir(), "config.json"))
	service := &fakeUserDaemonService{state: userDaemonState{SchemaVersion: userDaemonSchemaVersion, Supported: true}}
	var stdout, stderr bytes.Buffer
	root := &Root{stdout: &stdout, stderr: &stderr, daemonFactory: fakeDaemonFactory(service)}
	command := root.command()
	command.SetArgs([]string{"--json", "daemon", "restart", "--if-installed"})
	if err := command.Execute(); err != nil {
		t.Fatalf("restart --if-installed failed for an uninstalled daemon: %v; stderr=%s", err, stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Skipped bool `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || !envelope.Data.Skipped || service.restartCalls != 0 {
		t.Fatalf("unexpected skip result: error=%v output=%s service=%#v", err, stdout.String(), service)
	}
}

func TestDaemonRuntimeStatusIsAtomicMetadataOnlyAndFreshnessAware(t *testing.T) {
	home := t.TempDir()
	now := time.Now().UTC()
	service := &launchdDaemonService{home: home, executable: "/opt/contentcloud", version: Version, uid: 501, now: func() time.Time { return now }, run: func(name string, args ...string) ([]byte, error) {
		return []byte("state = running\npid = 4242\n"), nil
	}}
	recorder := newDaemonRuntimeStatusRecorder(service.runtimeStatusPath(), []localconfig.DaemonBinding{{DeviceID: "device-1"}}, 4242)
	recorder.observeControl("device-1", runtimeWakeObservation{State: "open"})
	recorder.observeWorker("device-1", runtimeWorkerObservation{State: "running", AttemptID: "attempt-1", At: now})
	recorder.observeWorker("device-1", runtimeWorkerObservation{State: "event", AttemptID: "attempt-1", At: now})
	recorder.mu.Lock()
	recorder.writeLocked(now)
	recorder.mu.Unlock()
	body, err := os.ReadFile(service.runtimeStatusPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "prompt", "workspace", "event_data", "server_url"} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Fatalf("runtime status exposed forbidden field %q: %s", forbidden, body)
		}
	}
	info, err := os.Stat(service.runtimeStatusPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("runtime status permissions = %v err=%v", info.Mode().Perm(), err)
	}
	state, err := service.Status()
	if err != nil || state.Runtime == nil || !state.Runtime.Fresh || len(state.Runtime.Bindings) != 1 || state.Runtime.Bindings[0].WorkerState != "running" || state.Runtime.Bindings[0].CurrentAttemptID != "attempt-1" || state.Runtime.Bindings[0].LastEventAt == nil {
		t.Fatalf("fresh runtime status = %#v err=%v", state.Runtime, err)
	}
	service.now = func() time.Time { return now.Add(daemonRuntimeStatusStaleAfter + time.Second) }
	state, err = service.Status()
	if err != nil || state.Runtime == nil || state.Runtime.Fresh {
		t.Fatalf("stale runtime status = %#v err=%v", state.Runtime, err)
	}
}

func TestRotatingDaemonLogRedactsRuntimeCredentialsAndPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	writer, err := newRotatingLogWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	input := "device=dt_device-secret bearer wt_workspace-secret token=rtg_gateway-secret path=/Users/coso/private"
	if _, err := writer.Write([]byte(input + "\n")); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, secret := range []string{"dt_device-secret", "wt_workspace-secret", "rtg_gateway-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("daemon log leaked %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED]") || strings.Contains(text, "/Users/coso/private") {
		t.Fatalf("daemon log redaction incomplete: %s", text)
	}
}

func TestDaemonDiagnosticBundleIsLocalRedactedAndHashable(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "daemon.log")
	if err := os.WriteFile(logPath, []byte("device=dt_secret bearer wt_secret path=/Users/coso/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := &daemonRuntimeStatusSnapshot{
		SchemaVersion: daemonRuntimeStatusSchemaVersion, ProcessID: 4242, WrittenAt: time.Now().UTC(), Fresh: true,
		Bindings: []daemonBindingStatusSnapshot{{DeviceID: "device-secret", CurrentAttemptID: "attempt-secret", LastAttemptID: "attempt-secret", WorkerState: "running"}},
	}
	now := time.Now().UTC()
	output := filepath.Join(root, "diagnostics.json")
	bundle, err := createDaemonDiagnosticBundle(output, userDaemonState{Supported: true, Running: true, PID: 4242, LogPath: logPath, ErrorLogPath: logPath, Runtime: status}, now)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("diagnostic permissions=%v err=%v", info.Mode().Perm(), err)
	}
	for _, forbidden := range []string{"dt_secret", "wt_secret", "device-secret", "attempt-secret", "/Users/coso/project"} {
		if strings.Contains(strings.ToLower(string(body)), strings.ToLower(forbidden)) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, body)
		}
	}
	if !bundle.Redacted || bundle.Uploaded || len(bundle.Logs) != 1 || !bundle.Logs[0].Available || bundle.Logs[0].Excerpt == "" {
		t.Fatalf("unexpected diagnostic bundle: %#v", bundle)
	}
}
