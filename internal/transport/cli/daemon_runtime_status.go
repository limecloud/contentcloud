package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	localconfig "github.com/limecloud/contentcloud/internal/local/config"
)

const (
	daemonRuntimeStatusSchemaVersion = "1.0"
	daemonRuntimeStatusRefresh       = 5 * time.Second
	daemonRuntimeStatusStaleAfter    = 20 * time.Second
)

type daemonRuntimeStatusSnapshot struct {
	SchemaVersion string                        `json:"schema_version"`
	ProcessID     int                           `json:"process_id"`
	WrittenAt     time.Time                     `json:"written_at"`
	Fresh         bool                          `json:"fresh"`
	Bindings      []daemonBindingStatusSnapshot `json:"bindings"`
}

type daemonBindingStatusSnapshot struct {
	DeviceID         string     `json:"device_id"`
	ControlState     string     `json:"control_state"`
	WorkerState      string     `json:"worker_state"`
	CurrentAttemptID string     `json:"current_attempt_id,omitempty"`
	LastAttemptID    string     `json:"last_attempt_id,omitempty"`
	LastEventAt      *time.Time `json:"last_event_at,omitempty"`
	LastHeartbeatAt  *time.Time `json:"last_heartbeat_at,omitempty"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type runtimeWorkerObservation struct {
	State     string
	AttemptID string
	ErrorCode string
	At        time.Time
}

type daemonRuntimeStatusRecorder struct {
	mu        sync.Mutex
	path      string
	processID int
	bindings  map[string]daemonBindingStatusSnapshot
}

func newDaemonRuntimeStatusRecorder(path string, bindings []localconfig.DaemonBinding, processIDs ...int) *daemonRuntimeStatusRecorder {
	processID := os.Getpid()
	if len(processIDs) > 0 && processIDs[0] > 0 {
		processID = processIDs[0]
	}
	recorder := &daemonRuntimeStatusRecorder{path: path, processID: processID, bindings: make(map[string]daemonBindingStatusSnapshot, len(bindings))}
	now := time.Now().UTC()
	for _, binding := range bindings {
		deviceID := strings.TrimSpace(binding.DeviceID)
		if deviceID != "" {
			recorder.bindings[deviceID] = daemonBindingStatusSnapshot{DeviceID: deviceID, ControlState: "connecting", WorkerState: "starting", UpdatedAt: now}
		}
	}
	recorder.writeLocked(now)
	return recorder
}

func (r *daemonRuntimeStatusRecorder) run(ctx context.Context) {
	ticker := time.NewTicker(daemonRuntimeStatusRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			now := time.Now().UTC()
			for deviceID, binding := range r.bindings {
				binding.ControlState = "stopped"
				binding.WorkerState = "stopped"
				binding.CurrentAttemptID = ""
				binding.UpdatedAt = now
				r.bindings[deviceID] = binding
			}
			r.writeLocked(now)
			r.mu.Unlock()
			return
		case now := <-ticker.C:
			r.mu.Lock()
			r.writeLocked(now.UTC())
			r.mu.Unlock()
		}
	}
}

func (r *daemonRuntimeStatusRecorder) observeControl(deviceID string, observation runtimeWakeObservation) {
	r.update(deviceID, true, func(binding *daemonBindingStatusSnapshot, now time.Time) {
		binding.ControlState = observation.State
		if observation.ErrorCode != "" {
			binding.LastErrorCode = observation.ErrorCode
		}
		binding.UpdatedAt = now
	})
}

func (r *daemonRuntimeStatusRecorder) observeWorker(deviceID string, observation runtimeWorkerObservation) {
	persist := observation.State != "event" && observation.State != "heartbeat"
	r.update(deviceID, persist, func(binding *daemonBindingStatusSnapshot, now time.Time) {
		at := observation.At.UTC()
		if at.IsZero() {
			at = now
		}
		if observation.State != "event" && observation.State != "heartbeat" {
			binding.WorkerState = observation.State
		}
		if observation.AttemptID != "" {
			binding.LastAttemptID = observation.AttemptID
		}
		switch observation.State {
		case "prepared", "running", "event", "heartbeat", "finalizing":
			binding.CurrentAttemptID = observation.AttemptID
		case "idle", "succeeded", "failed", "stopped":
			binding.CurrentAttemptID = ""
		}
		if observation.State == "event" {
			binding.LastEventAt = &at
		}
		if observation.State == "heartbeat" {
			binding.LastHeartbeatAt = &at
		}
		if observation.ErrorCode != "" {
			binding.LastErrorCode = observation.ErrorCode
		}
		binding.UpdatedAt = at
	})
}

func (r *daemonRuntimeStatusRecorder) update(deviceID string, persist bool, mutate func(*daemonBindingStatusSnapshot, time.Time)) {
	if r == nil || strings.TrimSpace(deviceID) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	binding := r.bindings[deviceID]
	binding.DeviceID = deviceID
	mutate(&binding, now)
	r.bindings[deviceID] = binding
	if persist {
		r.writeLocked(now)
	}
}

func (r *daemonRuntimeStatusRecorder) writeLocked(now time.Time) {
	if r == nil || strings.TrimSpace(r.path) == "" {
		return
	}
	bindings := make([]daemonBindingStatusSnapshot, 0, len(r.bindings))
	for _, binding := range r.bindings {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].DeviceID < bindings[j].DeviceID })
	body, err := json.Marshal(daemonRuntimeStatusSnapshot{SchemaVersion: daemonRuntimeStatusSchemaVersion, ProcessID: r.processID, WrittenAt: now, Fresh: true, Bindings: bindings})
	if err != nil || os.MkdirAll(filepath.Dir(r.path), 0o700) != nil {
		return
	}
	_ = writeDaemonFile(r.path, body, 0o600)
}

func readDaemonRuntimeStatus(path string, now time.Time, running bool, processIDs ...int) (*daemonRuntimeStatusSnapshot, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot daemonRuntimeStatusSnapshot
	if json.Unmarshal(body, &snapshot) != nil || snapshot.SchemaVersion != daemonRuntimeStatusSchemaVersion || snapshot.WrittenAt.IsZero() {
		return nil, nil
	}
	processMatches := snapshot.ProcessID > 0
	if len(processIDs) == 0 || processIDs[0] <= 0 {
		processMatches = true
	} else {
		processMatches = snapshot.ProcessID == processIDs[0]
	}
	snapshot.Fresh = running && processMatches && now.Sub(snapshot.WrittenAt) <= daemonRuntimeStatusStaleAfter
	return &snapshot, nil
}

func daemonRuntimeStatusPath(logFile string) (string, error) {
	if strings.TrimSpace(logFile) != "" {
		return filepath.Join(filepath.Dir(logFile), "runtime-status.json"), nil
	}
	configPath, err := localconfig.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "runtime-status.json"), nil
}

func runtimeObservationError(err error) string {
	var domainErr *fault.Error
	if errors.As(err, &domainErr) && strings.TrimSpace(domainErr.Code) != "" {
		return domainErr.Code
	}
	if err != nil {
		return "RUNTIME_WORKER_EXECUTION_FAILED"
	}
	return ""
}
