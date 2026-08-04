package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

func TestDaemonRuntimeTrackerPersistsNonSecretHealthAndUpdatePolicy(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("CONTENTCLOUD_DAEMON_STATE_DIR", stateDir)
	now := time.Date(2026, 7, 31, 13, 0, 0, 0, time.UTC)
	tracker, err := newDaemonRuntimeTracker([]localconfig.DaemonBinding{{DeviceID: "device-1", Workspaces: []localconfig.DaemonWorkspace{{ProjectID: "project-1"}}}}, "fixture", 2, now)
	if err != nil {
		t.Fatal(err)
	}
	tracker.recordPoll(app.DaemonRuntimePolicy{CurrentVersion: "0.10.0", LatestVersion: "0.17.0", UpdateAvailable: true}, true, nil)
	tracker.taskStarted()
	tracker.taskFinished(nil)
	state, err := loadDaemonRuntimeState()
	if err != nil || state.PendingReports < 0 || !state.RuntimePolicy.UpdateAvailable || state.ActiveTasks != 0 {
		t.Fatalf("unexpected runtime state: %#v err=%v", state, err)
	}
	body, err := os.ReadFile(filepath.Join(stateDir, "runtime.json"))
	if err != nil || string(body) == "" || containsSecret(string(body)) {
		t.Fatalf("runtime state unsafe: %v %s", err, body)
	}
}

func containsSecret(body string) bool {
	for _, value := range []string{"dt_", "wt_", "rt_", "api_key", "token"} {
		if strings.Contains(strings.ToLower(body), value) {
			return true
		}
	}
	return false
}
