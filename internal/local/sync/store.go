package localsync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
	_ "modernc.org/sqlite"
)

const SchemaVersion = "contentcloud.local-sync/1.0"

var ErrProjectNotObserved = errors.New("local sync project has not been observed")

type ConflictError struct {
	Code string
}

func (e *ConflictError) Error() string { return e.Code }

type Store struct {
	db *sql.DB
}

type ProjectState struct {
	ProjectID      string
	WorkspaceID    string
	LocalRevision  uint64
	ObservedDigest string
	CloudRevision  string
	CloudCursor    int64
	SyncedDigest   string
	SyncedAt       *time.Time
	ConflictCode   string
	TransferState  string
	EventCursor    uint64
}

type PublishCommand struct {
	RequestID      string
	DeviceID       string
	WorkspaceID    string
	ProjectID      string
	SubjectRef     string
	BaseRevision   string
	ObservedDigest string
	Files          []workspacedomain.WorkspaceRevisionFile
	IdempotencyKey string
	CreatedAt      time.Time
}

type CommandResult struct {
	CommandID   string
	State       string
	ProjectID   string
	EventCursor uint64
	CreatedAt   time.Time
}

type PendingCommand struct {
	CommandID      string
	RequestID      string
	WorkspaceID    string
	ProjectID      string
	DeviceID       string
	BaseRevision   string
	ContentDigest  string
	Files          []workspacedomain.WorkspaceRevisionFile
	IdempotencyKey string
	Attempts       int
}

type CloudRevision struct {
	ID            string
	ContentDigest string
}

type CloudRevisionEvent struct {
	ID            string
	WorkspaceID   string
	ProjectID     string
	RevisionNo    int64
	ContentDigest string
}

type CloudEvents struct {
	Events         []CloudRevisionEvent
	NextCursor     int64
	ResyncRequired bool
}

type UploadTransfer struct {
	ProjectID      string
	Ref            string
	ContentDigest  string
	ByteSize       int64
	SessionID      string
	State          string
	ConfirmedParts []int
	UpdatedAt      time.Time
}

type ProjectEvent struct {
	ID        string
	ProjectID string
	Cursor    uint64
	Type      string
	Payload   map[string]any
	CreatedAt time.Time
}

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("local sync SQLite path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS project_sync_state (
  project_id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  local_revision INTEGER NOT NULL DEFAULT 0,
  observed_digest TEXT NOT NULL DEFAULT '',
  cloud_revision TEXT NOT NULL DEFAULT '0',
  cloud_cursor INTEGER NOT NULL DEFAULT 0,
  synced_digest TEXT NOT NULL DEFAULT '',
  synced_at TEXT,
  conflict_code TEXT NOT NULL DEFAULT '',
  event_cursor INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS outbound_commands (
  command_id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  device_id TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL,
  command_type TEXT NOT NULL,
  subject_ref TEXT NOT NULL,
  base_revision TEXT NOT NULL,
  content_digest TEXT NOT NULL,
  file_manifest TEXT NOT NULL DEFAULT '[]',
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  state TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL,
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_until TEXT,
  last_error_code TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(project_id, idempotency_key),
  FOREIGN KEY(project_id) REFERENCES project_sync_state(project_id)
);
CREATE INDEX IF NOT EXISTS outbound_commands_project_state_idx
  ON outbound_commands(project_id, state, created_at);
CREATE TABLE IF NOT EXISTS project_events (
  project_id TEXT NOT NULL,
  cursor INTEGER NOT NULL,
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(project_id, cursor),
  UNIQUE(event_id),
  FOREIGN KEY(project_id) REFERENCES project_sync_state(project_id)
);
CREATE TABLE IF NOT EXISTS workspace_upload_transfers (
  project_id TEXT NOT NULL,
  ref TEXT NOT NULL,
  content_digest TEXT NOT NULL,
  byte_size INTEGER NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL,
  confirmed_parts TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL,
  PRIMARY KEY(project_id,ref,content_digest),
  FOREIGN KEY(project_id) REFERENCES project_sync_state(project_id)
);`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) ObserveProject(ctx context.Context, projectID, workspaceID, digest string, now time.Time) (ProjectState, error) {
	projectID, workspaceID, digest = strings.TrimSpace(projectID), strings.TrimSpace(workspaceID), strings.TrimSpace(digest)
	if projectID == "" || workspaceID == "" || !validDigest(digest) {
		return ProjectState{}, errors.New("project observation requires project, workspace and SHA-256 digest")
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectState{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_sync_state(project_id,workspace_id,updated_at) VALUES(?,?,?) ON CONFLICT(project_id) DO NOTHING`, projectID, workspaceID, formatTime(now)); err != nil {
		return ProjectState{}, err
	}
	state, err := projectStateTx(ctx, tx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	if state.WorkspaceID != workspaceID {
		return ProjectState{}, &ConflictError{Code: "PROJECT_WORKSPACE_BINDING_CONFLICT"}
	}
	if state.ObservedDigest != digest {
		state.LocalRevision++
		state.ObservedDigest = digest
		state.EventCursor++
		if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET local_revision=?,observed_digest=?,event_cursor=?,updated_at=? WHERE project_id=?`, state.LocalRevision, digest, state.EventCursor, formatTime(now), projectID); err != nil {
			return ProjectState{}, err
		}
		if err := insertEvent(ctx, tx, ProjectEvent{
			ID: stableID("evt_", projectID, fmt.Sprint(state.EventCursor), digest), ProjectID: projectID,
			Cursor: state.EventCursor, Type: "project.observed", Payload: map[string]any{"local_revision": state.LocalRevision, "observed_digest": digest}, CreatedAt: now,
		}); err != nil {
			return ProjectState{}, err
		}
	}
	state.TransferState, err = transferStateTx(ctx, tx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProjectState{}, err
	}
	return state, nil
}

func (s *Store) ProjectState(ctx context.Context, projectID string) (ProjectState, error) {
	state, err := projectStateQuery(ctx, s.db, strings.TrimSpace(projectID))
	if err != nil {
		return ProjectState{}, err
	}
	state.TransferState, err = transferStateQuery(ctx, s.db, state.ProjectID)
	return state, err
}

func (s *Store) QueuePublish(ctx context.Context, command PublishCommand) (CommandResult, error) {
	command.RequestID = strings.TrimSpace(command.RequestID)
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.ProjectID = strings.TrimSpace(command.ProjectID)
	command.SubjectRef = strings.TrimSpace(command.SubjectRef)
	command.BaseRevision = strings.TrimSpace(command.BaseRevision)
	command.ObservedDigest = strings.TrimSpace(command.ObservedDigest)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CreatedAt = command.CreatedAt.UTC()
	requestHash := publishRequestHash(command)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CommandResult{}, err
	}
	defer tx.Rollback()
	state, err := projectStateTx(ctx, tx, command.ProjectID)
	if err != nil {
		return CommandResult{}, err
	}
	if state.WorkspaceID != command.WorkspaceID {
		return CommandResult{}, &ConflictError{Code: "PROJECT_WORKSPACE_BINDING_CONFLICT"}
	}
	var existing CommandResult
	var existingHash, createdAt string
	err = tx.QueryRowContext(ctx, `SELECT command_id,state,project_id,request_hash,created_at FROM outbound_commands WHERE project_id=? AND idempotency_key=?`, command.ProjectID, command.IdempotencyKey).
		Scan(&existing.CommandID, &existing.State, &existing.ProjectID, &existingHash, &createdAt)
	if err == nil {
		if existingHash != requestHash {
			return CommandResult{}, &ConflictError{Code: "IDEMPOTENCY_KEY_REUSED"}
		}
		existing.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return CommandResult{}, err
		}
		existing.EventCursor = state.EventCursor
		return existing, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CommandResult{}, err
	}
	if command.BaseRevision != state.CloudRevision {
		return CommandResult{}, &ConflictError{Code: "CLOUD_REVISION_STALE"}
	}
	if command.ObservedDigest != state.ObservedDigest {
		return CommandResult{}, &ConflictError{Code: "WORKSPACE_DIGEST_STALE"}
	}
	commandID := stableID("cmd_", command.ProjectID, command.IdempotencyKey, requestHash)
	manifest, err := json.Marshal(command.Files)
	if err != nil {
		return CommandResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO outbound_commands(command_id,project_id,workspace_id,device_id,request_id,command_type,subject_ref,base_revision,content_digest,file_manifest,idempotency_key,request_hash,state,next_attempt_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		commandID, command.ProjectID, command.WorkspaceID, command.DeviceID, command.RequestID, "workspace.publish", command.SubjectRef, command.BaseRevision, command.ObservedDigest, string(manifest), command.IdempotencyKey, requestHash, "queued", formatTime(command.CreatedAt), formatTime(command.CreatedAt)); err != nil {
		return CommandResult{}, err
	}
	state.EventCursor++
	if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET event_cursor=?,updated_at=? WHERE project_id=?`, state.EventCursor, formatTime(command.CreatedAt), command.ProjectID); err != nil {
		return CommandResult{}, err
	}
	if err := insertEvent(ctx, tx, ProjectEvent{
		ID: stableID("evt_", commandID, "queued"), ProjectID: command.ProjectID, Cursor: state.EventCursor,
		Type: "workspace.publish.queued", Payload: map[string]any{"command_id": commandID}, CreatedAt: command.CreatedAt,
	}); err != nil {
		return CommandResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CommandResult{}, err
	}
	return CommandResult{CommandID: commandID, State: "queued", ProjectID: command.ProjectID, EventCursor: state.EventCursor, CreatedAt: command.CreatedAt}, nil
}

func (s *Store) SaveUploadTransfer(ctx context.Context, transfer UploadTransfer) error {
	transfer.ProjectID, transfer.Ref, transfer.ContentDigest = strings.TrimSpace(transfer.ProjectID), strings.TrimSpace(transfer.Ref), strings.TrimSpace(transfer.ContentDigest)
	transfer.SessionID, transfer.State = strings.TrimSpace(transfer.SessionID), strings.TrimSpace(transfer.State)
	if transfer.ProjectID == "" || transfer.Ref == "" || !validDigest(transfer.ContentDigest) || transfer.ByteSize < 0 || transfer.State == "" {
		return errors.New("workspace upload transfer is invalid")
	}
	parts := append([]int(nil), transfer.ConfirmedParts...)
	sort.Ints(parts)
	for index, part := range parts {
		if part < 0 || (index > 0 && parts[index-1] == part) {
			return errors.New("workspace upload transfer contains duplicate part")
		}
	}
	body, err := json.Marshal(parts)
	if err != nil {
		return err
	}
	transfer.UpdatedAt = transfer.UpdatedAt.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := projectStateTx(ctx, tx, transfer.ProjectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_upload_transfers(project_id,ref,content_digest,byte_size,session_id,state,confirmed_parts,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(project_id,ref,content_digest) DO UPDATE SET byte_size=excluded.byte_size,session_id=excluded.session_id,state=excluded.state,confirmed_parts=excluded.confirmed_parts,updated_at=excluded.updated_at`, transfer.ProjectID, transfer.Ref, transfer.ContentDigest, transfer.ByteSize, transfer.SessionID, transfer.State, string(body), formatTime(transfer.UpdatedAt)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UploadTransfer(ctx context.Context, projectID, ref, digest string) (UploadTransfer, error) {
	var transfer UploadTransfer
	var partsJSON, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT project_id,ref,content_digest,byte_size,session_id,state,confirmed_parts,updated_at FROM workspace_upload_transfers WHERE project_id=? AND ref=? AND content_digest=?`, projectID, ref, digest).
		Scan(&transfer.ProjectID, &transfer.Ref, &transfer.ContentDigest, &transfer.ByteSize, &transfer.SessionID, &transfer.State, &partsJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadTransfer{}, errors.New("workspace upload transfer not found")
	}
	if err != nil {
		return UploadTransfer{}, err
	}
	if err := json.Unmarshal([]byte(partsJSON), &transfer.ConfirmedParts); err != nil {
		return UploadTransfer{}, err
	}
	transfer.UpdatedAt, err = parseTime(updatedAt)
	return transfer, err
}

func (s *Store) ClaimPublish(ctx context.Context, worker, deviceID string, projectIDs []string, now time.Time, lease time.Duration) (PendingCommand, bool, error) {
	worker = strings.TrimSpace(worker)
	if worker == "" || lease <= 0 {
		return PendingCommand{}, false, errors.New("publish claim requires worker and positive lease")
	}
	allowed := make(map[string]bool, len(projectIDs))
	for _, projectID := range projectIDs {
		if projectID = strings.TrimSpace(projectID); projectID != "" {
			allowed[projectID] = true
		}
	}
	if len(allowed) == 0 {
		return PendingCommand{}, false, nil
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PendingCommand{}, false, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT command_id,request_id,workspace_id,project_id,device_id,base_revision,content_digest,file_manifest,idempotency_key,attempts FROM outbound_commands WHERE state IN ('queued','retrying') AND next_attempt_at<=? AND (lease_until IS NULL OR lease_until<=?) ORDER BY created_at,command_id LIMIT 100`, formatTime(now), formatTime(now))
	if err != nil {
		return PendingCommand{}, false, err
	}
	var command PendingCommand
	for rows.Next() {
		var candidate PendingCommand
		var manifest string
		if err := rows.Scan(&candidate.CommandID, &candidate.RequestID, &candidate.WorkspaceID, &candidate.ProjectID, &candidate.DeviceID, &candidate.BaseRevision, &candidate.ContentDigest, &manifest, &candidate.IdempotencyKey, &candidate.Attempts); err != nil {
			rows.Close()
			return PendingCommand{}, false, err
		}
		if err := json.Unmarshal([]byte(manifest), &candidate.Files); err != nil {
			rows.Close()
			return PendingCommand{}, false, err
		}
		if allowed[candidate.ProjectID] && (strings.TrimSpace(deviceID) == "" || candidate.DeviceID == strings.TrimSpace(deviceID)) {
			command = candidate
			break
		}
	}
	if err := rows.Close(); err != nil {
		return PendingCommand{}, false, err
	}
	if command.CommandID == "" {
		return PendingCommand{}, false, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `UPDATE outbound_commands SET state='uploading',attempts=attempts+1,lease_owner=?,lease_until=? WHERE command_id=? AND state IN ('queued','retrying') AND (lease_until IS NULL OR lease_until<=?)`, worker, formatTime(now.Add(lease)), command.CommandID, formatTime(now))
	if err != nil {
		return PendingCommand{}, false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return PendingCommand{}, false, err
	}
	command.Attempts++
	if err := tx.Commit(); err != nil {
		return PendingCommand{}, false, err
	}
	return command, true, nil
}

func (s *Store) CompletePublish(ctx context.Context, commandID, worker string, revision CloudRevision, now time.Time) error {
	if strings.TrimSpace(revision.ID) == "" || !validDigest(revision.ContentDigest) {
		return errors.New("cloud revision requires ID and SHA-256 digest")
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var projectID, contentDigest string
	if err := tx.QueryRowContext(ctx, `SELECT project_id,content_digest FROM outbound_commands WHERE command_id=? AND state='uploading' AND lease_owner=? AND lease_until>?`, commandID, worker, formatTime(now)).Scan(&projectID, &contentDigest); err != nil {
		return &ConflictError{Code: "OUTBOX_LEASE_STALE"}
	}
	if contentDigest != revision.ContentDigest {
		return &ConflictError{Code: "CLOUD_REVISION_DIGEST_MISMATCH"}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbound_commands SET state='synced',lease_owner='',lease_until=NULL,last_error_code='' WHERE command_id=?`, commandID); err != nil {
		return err
	}
	state, err := projectStateTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	state.EventCursor++
	if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET cloud_revision=?,synced_digest=?,synced_at=?,event_cursor=?,updated_at=? WHERE project_id=?`, revision.ID, revision.ContentDigest, formatTime(now), state.EventCursor, formatTime(now), projectID); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, ProjectEvent{ID: stableID("evt_", commandID, "synced", revision.ID), ProjectID: projectID, Cursor: state.EventCursor, Type: "workspace.publish.synced", Payload: map[string]any{"command_id": commandID, "cloud_revision": revision.ID, "content_digest": revision.ContentDigest}, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailPublish(ctx context.Context, commandID, worker, code string, retryable, conflict bool, retryAt, now time.Time) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errors.New("publish failure code is required")
	}
	now, retryAt = now.UTC(), retryAt.UTC()
	stateName, eventType := "failed", "workspace.publish.failed"
	if retryable {
		stateName, eventType = "retrying", "workspace.publish.retrying"
	} else if conflict {
		stateName, eventType = "conflict", "workspace.publish.conflict"
	} else if code == "DEVICE_TOKEN_INVALID" || code == "DEVICE_PROJECT_ACCESS_DENIED" {
		stateName, eventType = "auth_required", "workspace.publish.auth_required"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM outbound_commands WHERE command_id=? AND state='uploading' AND lease_owner=? AND lease_until>?`, commandID, worker, formatTime(now)).Scan(&projectID); err != nil {
		return &ConflictError{Code: "OUTBOX_LEASE_STALE"}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbound_commands SET state=?,next_attempt_at=?,lease_owner='',lease_until=NULL,last_error_code=? WHERE command_id=?`, stateName, formatTime(retryAt), code, commandID); err != nil {
		return err
	}
	state, err := projectStateTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	state.EventCursor++
	if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET event_cursor=?,updated_at=? WHERE project_id=?`, state.EventCursor, formatTime(now), projectID); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, ProjectEvent{ID: stableID("evt_", commandID, stateName, fmt.Sprint(state.EventCursor)), ProjectID: projectID, Cursor: state.EventCursor, Type: eventType, Payload: map[string]any{"command_id": commandID, "error_code": code}, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ApplyCloudEvents(ctx context.Context, projectID, workspaceID string, events []CloudRevisionEvent, now time.Time) error {
	if len(events) == 0 {
		return nil
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := projectStateTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if state.WorkspaceID != workspaceID {
		return &ConflictError{Code: "PROJECT_WORKSPACE_BINDING_CONFLICT"}
	}
	for _, event := range events {
		if event.ProjectID != projectID || event.WorkspaceID != workspaceID || event.RevisionNo != state.CloudCursor+1 || !validDigest(event.ContentDigest) || strings.TrimSpace(event.ID) == "" {
			return &ConflictError{Code: "CLOUD_EVENT_CURSOR_GAP"}
		}
		state.CloudCursor = event.RevisionNo
		state.CloudRevision = event.ID
		if event.ContentDigest == state.ObservedDigest {
			state.SyncedDigest = event.ContentDigest
			state.SyncedAt = &now
			state.ConflictCode = ""
		} else if state.SyncedDigest != "" && state.ObservedDigest != state.SyncedDigest {
			state.ConflictCode = "WORKSPACE_REMOTE_CHANGED"
		} else {
			state.ConflictCode = "WORKSPACE_REMOTE_CONTENT_PENDING"
		}
		state.EventCursor++
		if err := insertEvent(ctx, tx, ProjectEvent{
			ID: stableID("evt_", projectID, "cloud", fmt.Sprint(event.RevisionNo), event.ID), ProjectID: projectID,
			Cursor: state.EventCursor, Type: "cloud.workspace-revision.observed",
			Payload: map[string]any{"cloud_revision": event.ID, "cloud_cursor": event.RevisionNo, "content_digest": event.ContentDigest, "conflict_code": state.ConflictCode}, CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	var syncedAt any
	if state.SyncedAt != nil {
		syncedAt = formatTime(*state.SyncedAt)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET cloud_revision=?,cloud_cursor=?,synced_digest=?,synced_at=?,conflict_code=?,event_cursor=?,updated_at=? WHERE project_id=?`, state.CloudRevision, state.CloudCursor, state.SyncedDigest, syncedAt, state.ConflictCode, state.EventCursor, formatTime(now), projectID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RequireCloudResync(ctx context.Context, projectID, code string, now time.Time) error {
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := projectStateTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	state.EventCursor++
	if _, err := tx.ExecContext(ctx, `UPDATE project_sync_state SET cloud_cursor=0,conflict_code=?,event_cursor=?,updated_at=? WHERE project_id=?`, code, state.EventCursor, formatTime(now), projectID); err != nil {
		return err
	}
	if err := insertEvent(ctx, tx, ProjectEvent{ID: stableID("evt_", projectID, "cloud-resync", fmt.Sprint(state.EventCursor)), ProjectID: projectID, Cursor: state.EventCursor, Type: "cloud.resync.required", Payload: map[string]any{"error_code": code}, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListEvents(ctx context.Context, projectID string, after uint64, limit int) ([]ProjectEvent, uint64, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	state, err := s.ProjectState(ctx, projectID)
	if err != nil {
		return nil, 0, false, err
	}
	if after > state.EventCursor {
		return []ProjectEvent{}, state.EventCursor, true, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,project_id,cursor,event_type,payload,created_at FROM project_events WHERE project_id=? AND cursor>? ORDER BY cursor LIMIT ?`, projectID, after, limit)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	events := make([]ProjectEvent, 0)
	next := after
	for rows.Next() {
		var event ProjectEvent
		var payload, createdAt string
		if err := rows.Scan(&event.ID, &event.ProjectID, &event.Cursor, &event.Type, &payload, &createdAt); err != nil {
			return nil, 0, false, err
		}
		if err := json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return nil, 0, false, err
		}
		event.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, 0, false, err
		}
		events = append(events, event)
		next = event.Cursor
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	gap := len(events) > 0 && events[0].Cursor != after+1
	return events, next, gap, nil
}

func (s *Store) PendingCommands(ctx context.Context, projectID string) ([]CommandResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT command_id,state,project_id,created_at FROM outbound_commands WHERE project_id=? AND state='queued' ORDER BY created_at,command_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]CommandResult, 0)
	for rows.Next() {
		var result CommandResult
		var createdAt string
		if err := rows.Scan(&result.CommandID, &result.State, &result.ProjectID, &createdAt); err != nil {
			return nil, err
		}
		result.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func projectStateQuery(ctx context.Context, query rowQuerier, projectID string) (ProjectState, error) {
	var state ProjectState
	var syncedAt sql.NullString
	err := query.QueryRowContext(ctx, `SELECT project_id,workspace_id,local_revision,observed_digest,cloud_revision,cloud_cursor,synced_digest,synced_at,conflict_code,event_cursor FROM project_sync_state WHERE project_id=?`, projectID).
		Scan(&state.ProjectID, &state.WorkspaceID, &state.LocalRevision, &state.ObservedDigest, &state.CloudRevision, &state.CloudCursor, &state.SyncedDigest, &syncedAt, &state.ConflictCode, &state.EventCursor)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectState{}, ErrProjectNotObserved
	}
	if err == nil && syncedAt.Valid {
		var parsed time.Time
		parsed, err = parseTime(syncedAt.String)
		state.SyncedAt = &parsed
	}
	return state, err
}

func projectStateTx(ctx context.Context, tx *sql.Tx, projectID string) (ProjectState, error) {
	return projectStateQuery(ctx, tx, projectID)
}

func transferStateQuery(ctx context.Context, query rowQuerier, projectID string) (string, error) {
	var queued, uploading, failed int
	if err := query.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE state IN ('queued','retrying')),count(*) FILTER (WHERE state='uploading'),count(*) FILTER (WHERE state IN ('failed','conflict','auth_required')) FROM outbound_commands WHERE project_id=?`, projectID).Scan(&queued, &uploading, &failed); err != nil {
		return "", err
	}
	if uploading > 0 {
		return "uploading", nil
	}
	if queued > 0 {
		return "queued", nil
	}
	if failed > 0 {
		return "failed", nil
	}
	var synced int
	if err := query.QueryRowContext(ctx, `SELECT count(*) FROM outbound_commands WHERE project_id=? AND state='synced'`, projectID).Scan(&synced); err != nil {
		return "", err
	}
	if synced > 0 {
		return "synced", nil
	}
	return "idle", nil
}

func transferStateTx(ctx context.Context, tx *sql.Tx, projectID string) (string, error) {
	return transferStateQuery(ctx, tx, projectID)
}

func insertEvent(ctx context.Context, tx *sql.Tx, event ProjectEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_events(project_id,cursor,event_id,event_type,payload,created_at) VALUES(?,?,?,?,?,?)`, event.ProjectID, event.Cursor, event.ID, event.Type, string(payload), formatTime(event.CreatedAt))
	return err
}

func publishRequestHash(command PublishCommand) string {
	manifest, _ := json.Marshal(command.Files)
	value := strings.Join([]string{command.WorkspaceID, command.ProjectID, command.SubjectRef, command.BaseRevision, command.ObservedDigest, string(manifest)}, "\n")
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func stableID(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return prefix + hex.EncodeToString(sum[:16])
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
