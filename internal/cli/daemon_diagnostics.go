package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	daemonDiagnosticSchemaVersion = "1.0"
	daemonDiagnosticLogMaxBytes   = 128 << 10
)

type daemonDiagnosticBundle struct {
	SchemaVersion string                      `json:"schema_version"`
	GeneratedAt   time.Time                   `json:"generated_at"`
	Redacted      bool                        `json:"redacted"`
	Uploaded      bool                        `json:"uploaded"`
	Platform      string                      `json:"platform"`
	Arch          string                      `json:"arch"`
	CLIVersion    string                      `json:"cli_version"`
	Daemon        daemonDiagnosticDaemonState `json:"daemon"`
	Logs          []daemonDiagnosticLog       `json:"logs"`
}

type daemonDiagnosticDaemonState struct {
	Supported bool                             `json:"supported"`
	Installed bool                             `json:"installed"`
	Running   bool                             `json:"running"`
	PID       int                              `json:"pid,omitempty"`
	Version   string                           `json:"version,omitempty"`
	UpdatedAt *time.Time                       `json:"updated_at,omitempty"`
	Runtime   *daemonDiagnosticRuntimeSnapshot `json:"runtime,omitempty"`
}

type daemonDiagnosticRuntimeSnapshot struct {
	WrittenAt time.Time                               `json:"written_at"`
	Fresh     bool                                    `json:"fresh"`
	Bindings  []daemonDiagnosticBindingStatusSnapshot `json:"bindings"`
}

type daemonDiagnosticBindingStatusSnapshot struct {
	DeviceRef         string     `json:"device_ref"`
	ControlState      string     `json:"control_state"`
	WorkerState       string     `json:"worker_state"`
	CurrentAttemptRef string     `json:"current_attempt_ref,omitempty"`
	LastAttemptRef    string     `json:"last_attempt_ref,omitempty"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	LastHeartbeatAt   *time.Time `json:"last_heartbeat_at,omitempty"`
	LastErrorCode     string     `json:"last_error_code,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type daemonDiagnosticLog struct {
	Kind      string `json:"kind"`
	Available bool   `json:"available"`
	Truncated bool   `json:"truncated,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`
}

func createDaemonDiagnosticBundle(path string, state userDaemonState, now time.Time) (daemonDiagnosticBundle, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return daemonDiagnosticBundle{}, domain.Invalid("DAEMON_DIAGNOSTIC_OUTPUT_REQUIRED", "生成 Daemon 诊断包需要明确指定输出文件")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return daemonDiagnosticBundle{}, err
	}
	info, err := os.Lstat(absPath)
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return daemonDiagnosticBundle{}, domain.Policy("DAEMON_DIAGNOSTIC_OUTPUT_UNSAFE", "Daemon 诊断包输出路径必须是普通文件", "选择私有目录中的新 JSON 文件")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return daemonDiagnosticBundle{}, err
	}
	bundle := daemonDiagnosticBundle{
		SchemaVersion: daemonDiagnosticSchemaVersion,
		GeneratedAt:   now.UTC(),
		Redacted:      true,
		Uploaded:      false,
		Platform:      runtime.GOOS,
		Arch:          runtime.GOARCH,
		CLIVersion:    Version,
		Daemon: daemonDiagnosticDaemonState{
			Supported: state.Supported, Installed: state.Installed, Running: state.Running,
			PID: state.PID, Version: state.Version, UpdatedAt: state.UpdatedAt,
			Runtime: diagnosticRuntimeSnapshot(state.Runtime),
		},
		Logs: []daemonDiagnosticLog{},
	}
	identifiers := diagnosticIdentifiers(state.Runtime)
	for _, candidate := range []struct {
		kind string
		path string
	}{{kind: "daemon", path: state.LogPath}, {kind: "daemon_error", path: state.ErrorLogPath}} {
		if strings.TrimSpace(candidate.path) == "" || diagnosticLogKindExists(bundle.Logs, candidate.kind, candidate.path, state.LogPath) {
			continue
		}
		bundle.Logs = append(bundle.Logs, readDaemonDiagnosticLog(candidate.kind, candidate.path, identifiers))
	}
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return daemonDiagnosticBundle{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o700); err != nil {
		return daemonDiagnosticBundle{}, err
	}
	if err := writeDaemonFile(absPath, append(body, '\n'), 0o600); err != nil {
		return daemonDiagnosticBundle{}, err
	}
	return bundle, nil
}

func diagnosticRuntimeSnapshot(snapshot *daemonRuntimeStatusSnapshot) *daemonDiagnosticRuntimeSnapshot {
	if snapshot == nil {
		return nil
	}
	result := &daemonDiagnosticRuntimeSnapshot{WrittenAt: snapshot.WrittenAt, Fresh: snapshot.Fresh, Bindings: make([]daemonDiagnosticBindingStatusSnapshot, 0, len(snapshot.Bindings))}
	for _, binding := range snapshot.Bindings {
		result.Bindings = append(result.Bindings, daemonDiagnosticBindingStatusSnapshot{
			DeviceRef: diagnosticRef("device", binding.DeviceID), ControlState: binding.ControlState, WorkerState: binding.WorkerState,
			CurrentAttemptRef: diagnosticRef("attempt", binding.CurrentAttemptID), LastAttemptRef: diagnosticRef("attempt", binding.LastAttemptID),
			LastEventAt: binding.LastEventAt, LastHeartbeatAt: binding.LastHeartbeatAt, LastErrorCode: binding.LastErrorCode, UpdatedAt: binding.UpdatedAt,
		})
	}
	return result
}

func diagnosticIdentifiers(snapshot *daemonRuntimeStatusSnapshot) map[string]string {
	result := map[string]string{}
	if snapshot == nil {
		return result
	}
	for _, binding := range snapshot.Bindings {
		for kind, value := range map[string]string{"device": binding.DeviceID, "attempt": binding.CurrentAttemptID, "last-attempt": binding.LastAttemptID} {
			if value = strings.TrimSpace(value); value != "" {
				result[value] = diagnosticRef(kind, value)
			}
		}
	}
	return result
}

func diagnosticRef(kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(kind + ":" + value))
	return kind + ":sha256:" + hex.EncodeToString(sum[:6])
}

func readDaemonDiagnosticLog(kind, path string, identifiers map[string]string) daemonDiagnosticLog {
	result := daemonDiagnosticLog{Kind: kind}
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return result
	}
	offset := int64(0)
	if info.Size() > daemonDiagnosticLogMaxBytes {
		offset = info.Size() - daemonDiagnosticLogMaxBytes
		result.Truncated = true
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return result
	}
	body := make([]byte, info.Size()-offset)
	n, err := file.Read(body)
	if err != nil && n == 0 {
		return result
	}
	excerpt := sanitizeDaemonLog(string(body[:n]))
	for raw, replacement := range identifiers {
		excerpt = strings.ReplaceAll(excerpt, raw, replacement)
	}
	result.Available = true
	result.Excerpt = excerpt
	return result
}

func diagnosticLogKindExists(logs []daemonDiagnosticLog, kind, path, primaryPath string) bool {
	return kind == "daemon_error" && strings.TrimSpace(path) == strings.TrimSpace(primaryPath)
}
