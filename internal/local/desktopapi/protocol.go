package desktopapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	localsync "github.com/limecloud/contentcloud/internal/local/sync"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const (
	CommandSchemaVersion       = "contentcloud.desktop-command/1.0"
	CommandResultSchemaVersion = "contentcloud.desktop-command-result/1.0"
	EventStreamSchemaVersion   = "contentcloud.desktop-events/1.0"
)

// ReviewDispatcher is the Daemon's in-memory bridge to the Cloud client.
// Cloud credentials never cross the local Desktop API boundary.
type ReviewDispatcher func(context.Context, string, string, any) (json.RawMessage, error)

type APICapabilities struct {
	SchemaVersion  string   `json:"schema_version"`
	APIVersions    []string `json:"api_versions"`
	SnapshotSchema string   `json:"snapshot_schema"`
	CommandSchema  string   `json:"command_schema"`
	EventSchema    string   `json:"event_schema"`
	Commands       []string `json:"commands"`
}

type PublishWorkspaceCommand struct {
	SchemaVersion  string `json:"schema_version"`
	RequestID      string `json:"request_id"`
	WorkspaceID    string `json:"workspace_id"`
	ProjectID      string `json:"project_id"`
	SubjectRef     string `json:"subject_ref"`
	BaseRevision   string `json:"base_revision"`
	ObservedDigest string `json:"observed_digest"`
	IdempotencyKey string `json:"idempotency_key"`
}

type CommandResponse struct {
	SchemaVersion string    `json:"schema_version"`
	RequestID     string    `json:"request_id"`
	CommandID     string    `json:"command_id"`
	ProjectID     string    `json:"project_id"`
	State         string    `json:"state"`
	EventCursor   uint64    `json:"event_cursor"`
	AcceptedAt    time.Time `json:"accepted_at"`
}

type EventStream struct {
	SchemaVersion  string         `json:"schema_version"`
	ProjectID      string         `json:"project_id"`
	Events         []DesktopEvent `json:"events"`
	NextCursor     uint64         `json:"next_cursor"`
	Gap            bool           `json:"gap"`
	ResyncRequired bool           `json:"resync_required"`
}

type DesktopEvent struct {
	ID        string         `json:"id"`
	ProjectID string         `json:"project_id"`
	Cursor    uint64         `json:"cursor"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

func decodeRequest(request *http.Request, destination any) error {
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		return errors.New("request content type must be application/json")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxRequestBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxRequestBytes {
		return errors.New("request body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func validatePublishCommand(command PublishWorkspaceCommand) string {
	if command.SchemaVersion != CommandSchemaVersion {
		return "DESKTOP_COMMAND_SCHEMA_UNSUPPORTED"
	}
	for _, value := range []string{command.RequestID, command.WorkspaceID, command.ProjectID, command.IdempotencyKey} {
		if !validProtocolID(value) {
			return "DESKTOP_COMMAND_IDENTIFIER_INVALID"
		}
	}
	if command.SubjectRef != "workspace" {
		return "DESKTOP_COMMAND_SUBJECT_INVALID"
	}
	if strings.TrimSpace(command.BaseRevision) == "" || len(command.BaseRevision) > 256 {
		return "DESKTOP_COMMAND_BASE_REVISION_INVALID"
	}
	if !validSHA256(command.ObservedDigest) {
		return "DESKTOP_COMMAND_DIGEST_INVALID"
	}
	return ""
}

func validProtocolID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func syncErrorCode(err error, fallback string) string {
	var domainErr *fault.Error
	if errors.As(err, &domainErr) && strings.TrimSpace(domainErr.Code) != "" {
		return domainErr.Code
	}
	var conflict *localsync.ConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}
	var observation *localsync.ObservationError
	if errors.As(err, &observation) {
		return observation.Code
	}
	return fallback
}
