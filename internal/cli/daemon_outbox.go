package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

const daemonJournalSchemaVersion = "1.0"

type daemonJournalState string

const (
	daemonJournalExecuting daemonJournalState = "executing"
	daemonJournalReport    daemonJournalState = "report"
	daemonJournalFinish    daemonJournalState = "finish"
)

type daemonJournalEntry struct {
	SchemaVersion string             `json:"schema_version"`
	State         daemonJournalState `json:"state"`
	ServerURL     string             `json:"server_url"`
	DeviceID      string             `json:"device_id"`
	RunID         string             `json:"run_id"`
	AttemptID     string             `json:"attempt_id"`
	RunToken      string             `json:"run_token"`
	Package       json.RawMessage    `json:"package,omitempty"`
	Outcome       string             `json:"outcome,omitempty"`
	FailureClass  string             `json:"failure_class,omitempty"`
	ExitCode      *int               `json:"exit_code,omitempty"`
	Usage         map[string]any     `json:"usage,omitempty"`
	Summary       string             `json:"summary,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	LastError     string             `json:"last_error,omitempty"`
	Attempts      int                `json:"attempts"`
}

type daemonJournal struct {
	dir string
	now func() time.Time
	mu  sync.Mutex
}

func newDaemonJournal() (*daemonJournal, error) {
	dir := strings.TrimSpace(os.Getenv("CONTENTCLOUD_DAEMON_STATE_DIR"))
	if dir == "" {
		path, err := localConfigPath()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(filepath.Dir(path), "daemon")
	}
	if err := os.MkdirAll(filepath.Join(dir, "outbox"), 0o700); err != nil {
		return nil, err
	}
	return &daemonJournal{dir: filepath.Join(dir, "outbox"), now: time.Now}, nil
}

func localConfigPath() (string, error) {
	return localconfig.Path()
}

func (j *daemonJournal) currentTime() time.Time {
	if j == nil || j.now == nil {
		return time.Now().UTC()
	}
	return j.now().UTC()
}

func (j *daemonJournal) entryPath(attemptID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(attemptID)))
	return filepath.Join(j.dir, "attempt-"+hex.EncodeToString(sum[:16])+".json")
}

func (j *daemonJournal) begin(lease app.Lease, serverURL, deviceID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j == nil || strings.TrimSpace(lease.Attempt.ID) == "" || strings.TrimSpace(lease.Run.ID) == "" || strings.TrimSpace(lease.RunToken) == "" {
		return domain.Invalid("DAEMON_JOURNAL_INVALID", "自动化执行日志缺少租约身份")
	}
	path := j.entryPath(lease.Attempt.ID)
	if existing, err := j.read(path); err == nil {
		if existing.RunID == lease.Run.ID && existing.AttemptID == lease.Attempt.ID && existing.State != daemonJournalExecuting {
			return nil
		}
		if existing.State == daemonJournalExecuting {
			return nil
		}
	}
	now := j.currentTime()
	return j.write(path, daemonJournalEntry{SchemaVersion: daemonJournalSchemaVersion, State: daemonJournalExecuting, ServerURL: strings.TrimRight(serverURL, "/"), DeviceID: deviceID, RunID: lease.Run.ID, AttemptID: lease.Attempt.ID, RunToken: lease.RunToken, CreatedAt: now, UpdatedAt: now})
}

func (j *daemonJournal) queueReport(lease app.Lease, packageBody json.RawMessage) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	path := j.entryPath(lease.Attempt.ID)
	entry, err := j.read(path)
	if err != nil {
		return err
	}
	if entry.RunID != lease.Run.ID || entry.AttemptID != lease.Attempt.ID {
		return domain.Conflict("DAEMON_JOURNAL_IDENTITY_MISMATCH", "执行日志与当前租约不一致")
	}
	if len(packageBody) == 0 {
		return domain.Invalid("DAEMON_JOURNAL_PACKAGE_REQUIRED", "执行成功后必须先保存结构化结果")
	}
	entry.State, entry.Package, entry.UpdatedAt, entry.LastError = daemonJournalReport, append(json.RawMessage(nil), packageBody...), j.currentTime(), ""
	return j.write(path, entry)
}

func (j *daemonJournal) queueFinish(lease app.Lease, outcome, failureClass, summary string, exitCode *int) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	path := j.entryPath(lease.Attempt.ID)
	entry, err := j.read(path)
	if err != nil {
		return err
	}
	entry.State, entry.Outcome, entry.FailureClass, entry.Summary, entry.ExitCode, entry.UpdatedAt = daemonJournalFinish, outcome, failureClass, strings.TrimSpace(summary), exitCode, j.currentTime()
	entry.LastError = ""
	return j.write(path, entry)
}

func (j *daemonJournal) read(path string) (daemonJournalEntry, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return daemonJournalEntry{}, err
	}
	var entry daemonJournalEntry
	if err := json.Unmarshal(body, &entry); err != nil || entry.SchemaVersion != daemonJournalSchemaVersion || entry.RunID == "" || entry.AttemptID == "" {
		return daemonJournalEntry{}, domain.Invalid("DAEMON_JOURNAL_CORRUPT", "自动化执行日志文件损坏")
	}
	return entry, nil
}

func (j *daemonJournal) write(path string, entry daemonJournalEntry) error {
	body, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(j.dir, ".attempt-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(body, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func (j *daemonJournal) flush(ctx context.Context, client *apiclient.Client) error {
	return j.flushMatching(ctx, client, "", "")
}

func (j *daemonJournal) flushMatching(ctx context.Context, client *apiclient.Client, serverURL, deviceID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, file := range entries {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") || strings.HasPrefix(file.Name(), ".") {
			continue
		}
		path := filepath.Join(j.dir, file.Name())
		entry, readErr := j.read(path)
		if readErr != nil {
			_ = os.Rename(path, path+".dead")
			if firstErr == nil {
				firstErr = readErr
			}
			continue
		}
		if serverURL != "" && (strings.TrimRight(entry.ServerURL, "/") != strings.TrimRight(serverURL, "/") || entry.DeviceID != deviceID) {
			continue
		}
		if entry.State == daemonJournalExecuting {
			entry.State, entry.Outcome, entry.FailureClass, entry.Summary = daemonJournalFinish, "failed", "daemon_restarted", "后台服务在智能体完成前重启，本次执行已安全回收"
			entry.UpdatedAt = j.currentTime()
			if writeErr := j.write(path, entry); writeErr != nil {
				if firstErr == nil {
					firstErr = writeErr
				}
				continue
			}
		}
		entry.Attempts++
		var dispatchErr error
		switch entry.State {
		case daemonJournalReport:
			dispatchErr = client.Dispatch(ctx, "run.report", map[string]any{"run_id": entry.RunID, "attempt_id": entry.AttemptID, "run_token": entry.RunToken, "package": entry.Package}, nil)
		case daemonJournalFinish:
			params := map[string]any{"run_id": entry.RunID, "attempt_id": entry.AttemptID, "run_token": entry.RunToken, "outcome": entry.Outcome, "failure_class": entry.FailureClass, "transcript_summary": entry.Summary}
			if entry.ExitCode != nil {
				params["exit_code"] = *entry.ExitCode
			}
			dispatchErr = client.Dispatch(ctx, "run.finish", params, nil)
		default:
			dispatchErr = domain.Invalid("DAEMON_JOURNAL_STATE_INVALID", "自动化执行日志状态无效")
		}
		if dispatchErr == nil {
			_ = os.Remove(path)
			continue
		}
		entry.LastError = dispatchErr.Error()
		entry.UpdatedAt = j.currentTime()
		if writeErr := j.write(path, entry); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		if !journalRetryable(dispatchErr) {
			_ = os.Rename(path, path+".dead")
		}
		if firstErr == nil {
			firstErr = dispatchErr
		}
	}
	return firstErr
}

func (j *daemonJournal) deliverAttempt(ctx context.Context, client *apiclient.Client, attemptID string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	path := j.entryPath(attemptID)
	entry, err := j.read(path)
	if err != nil {
		return err
	}
	if entry.State == daemonJournalExecuting {
		return domain.Conflict("DAEMON_JOURNAL_RESULT_NOT_READY", "本次自动化执行尚未进入可上报状态")
	}
	entry.Attempts++
	var dispatchErr error
	if entry.State == daemonJournalReport {
		dispatchErr = client.Dispatch(ctx, "run.report", map[string]any{"run_id": entry.RunID, "attempt_id": entry.AttemptID, "run_token": entry.RunToken, "package": entry.Package}, nil)
	} else {
		params := map[string]any{"run_id": entry.RunID, "attempt_id": entry.AttemptID, "run_token": entry.RunToken, "outcome": entry.Outcome, "failure_class": entry.FailureClass, "transcript_summary": entry.Summary}
		if entry.ExitCode != nil {
			params["exit_code"] = *entry.ExitCode
		}
		dispatchErr = client.Dispatch(ctx, "run.finish", params, nil)
	}
	if dispatchErr == nil {
		return os.Remove(path)
	}
	entry.LastError, entry.UpdatedAt = dispatchErr.Error(), j.currentTime()
	if writeErr := j.write(path, entry); writeErr != nil {
		return errors.Join(dispatchErr, writeErr)
	}
	if !journalRetryable(dispatchErr) {
		_ = os.Rename(path, path+".dead")
	}
	return dispatchErr
}

func journalRetryable(err error) bool {
	var value *domain.Error
	if errors.As(err, &value) {
		return value.Retryable || value.Code == "NETWORK_ERROR"
	}
	return true
}

func daemonJournalPendingCount() (int, error) {
	pending, _, err := daemonJournalCounts()
	return pending, err
}

func daemonJournalCounts() (int, int, error) {
	j, err := newDaemonJournal()
	if err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return 0, 0, err
	}
	pending, dead := 0, 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".json") {
			pending++
		} else if strings.HasSuffix(entry.Name(), ".dead") {
			dead++
		}
	}
	return pending, dead, nil
}
