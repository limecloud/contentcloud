package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

const daemonRuntimeStateSchemaVersion = "1.0"

type daemonRuntimeState struct {
	SchemaVersion   string                  `json:"schema_version"`
	DaemonVersion   string                  `json:"daemon_version"`
	Provider        string                  `json:"provider"`
	ProviderVersion string                  `json:"provider_version,omitempty"`
	BindingCount    int                     `json:"binding_count"`
	WorkspaceCount  int                     `json:"workspace_count"`
	MaxConcurrent   int                     `json:"max_concurrent_tasks"`
	ActiveTasks     int                     `json:"active_tasks"`
	PendingReports  int                     `json:"pending_reports"`
	DeadLetters     int                     `json:"dead_letters"`
	RuntimePolicy   app.DaemonRuntimePolicy `json:"runtime_policy"`
	StartedAt       time.Time               `json:"started_at"`
	LastPollAt      *time.Time              `json:"last_poll_at,omitempty"`
	LastLeaseAt     *time.Time              `json:"last_lease_at,omitempty"`
	LastError       string                  `json:"last_error,omitempty"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type daemonRuntimeTracker struct {
	mu    sync.Mutex
	path  string
	state daemonRuntimeState
}

func newDaemonRuntimeTracker(bindings []localconfig.DaemonBinding, provider string, maxConcurrent int, now time.Time) (*daemonRuntimeTracker, error) {
	path, err := daemonRuntimeStatePath()
	if err != nil {
		return nil, err
	}
	workspaceCount := 0
	for _, binding := range bindings {
		workspaceCount += len(binding.Workspaces)
	}
	pending, dead, _ := daemonJournalCounts()
	tracker := &daemonRuntimeTracker{path: path, state: daemonRuntimeState{SchemaVersion: daemonRuntimeStateSchemaVersion, DaemonVersion: Version, Provider: provider, ProviderVersion: detectAgentVersion(provider), BindingCount: len(bindings), WorkspaceCount: workspaceCount, MaxConcurrent: maxConcurrent, PendingReports: pending, DeadLetters: dead, StartedAt: now.UTC(), UpdatedAt: now.UTC()}}
	if err := tracker.persistLocked(); err != nil {
		return nil, err
	}
	return tracker, nil
}

func daemonRuntimeStatePath() (string, error) {
	dir := strings.TrimSpace(os.Getenv("CONTENTCLOUD_DAEMON_STATE_DIR"))
	if dir == "" {
		path, err := localconfig.Path()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(filepath.Dir(path), "daemon")
	}
	return filepath.Join(dir, "runtime.json"), nil
}

func loadDaemonRuntimeState() (*daemonRuntimeState, error) {
	path, err := daemonRuntimeStatePath()
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state daemonRuntimeState
	if err := json.Unmarshal(body, &state); err != nil {
		return nil, err
	}
	if state.SchemaVersion != daemonRuntimeStateSchemaVersion {
		return nil, os.ErrInvalid
	}
	return &state, nil
}

func (t *daemonRuntimeTracker) recordPoll(policy app.DaemonRuntimePolicy, leased bool, pollErr error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	t.state.LastPollAt, t.state.RuntimePolicy, t.state.UpdatedAt = &now, policy, now
	if leased {
		t.state.LastLeaseAt = &now
	}
	if pollErr != nil {
		t.state.LastError = pollErr.Error()
	} else {
		t.state.LastError = ""
	}
	t.state.PendingReports, t.state.DeadLetters, _ = daemonJournalCounts()
	_ = t.persistLocked()
}

func (t *daemonRuntimeTracker) taskStarted() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.ActiveTasks++
	t.state.UpdatedAt = time.Now().UTC()
	_ = t.persistLocked()
}

func (t *daemonRuntimeTracker) taskFinished(runErr error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.ActiveTasks > 0 {
		t.state.ActiveTasks--
	}
	if runErr != nil {
		t.state.LastError = runErr.Error()
	}
	t.state.PendingReports, t.state.DeadLetters, _ = daemonJournalCounts()
	t.state.UpdatedAt = time.Now().UTC()
	_ = t.persistLocked()
}

func (t *daemonRuntimeTracker) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(t.state, "", "  ")
	if err != nil {
		return err
	}
	return writeDaemonFile(t.path, append(body, '\n'), 0o600)
}

func detectAgentVersion(provider string) string {
	binary := ""
	switch provider {
	case "codex":
		binary = "codex"
	case "claude-code":
		binary = "claude"
	}
	if binary == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(output))
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}
