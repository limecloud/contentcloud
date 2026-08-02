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

	"github.com/limecloud/contentcloud/internal/app"
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

	upgraded := &launchdDaemonService{home: home, executable: "/opt/contentcloud-0.15.0", version: "0.15.0", uid: 501, now: func() time.Time { return now.Add(time.Hour) }, run: runner, environment: launchEnvironment}
	state, err = upgraded.Start()
	if err != nil || state.Version != "0.15.0" || state.Executable != "/opt/contentcloud-0.15.0" || !state.Running {
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
	if err != nil || !strings.Contains(string(metadata), `"version": "0.15.0"`) {
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

func TestLaunchdDaemonStatusKeepsLastRuntimeStateWhenStopped(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	now := time.Date(2026, 7, 31, 14, 0, 0, 0, time.UTC)
	tracker, err := newDaemonRuntimeTracker(nil, "fixture", 2, now)
	if err != nil {
		t.Fatal(err)
	}
	tracker.recordPoll(app.DaemonRuntimePolicy{CurrentVersion: "0.10.0", LatestVersion: "0.15.0", UpdateAvailable: true}, false, nil)
	service := &launchdDaemonService{
		home: t.TempDir(), executable: "/opt/contentcloud", version: "0.10.0", uid: 501, now: func() time.Time { return now },
		run: func(string, ...string) ([]byte, error) { return nil, errors.New("service not loaded") },
	}
	state, err := service.Status()
	if err != nil {
		t.Fatal(err)
	}
	if state.Running || state.Runtime == nil || !state.Runtime.RuntimePolicy.UpdateAvailable || state.Runtime.RuntimePolicy.CurrentVersion != "0.10.0" {
		t.Fatalf("stopped daemon status lost runtime state: %#v", state)
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
