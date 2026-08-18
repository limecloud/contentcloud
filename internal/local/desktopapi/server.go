package desktopapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	localconfig "github.com/limecloud/contentcloud/internal/local/config"
	localsync "github.com/limecloud/contentcloud/internal/local/sync"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
)

const (
	SchemaVersion          = "contentcloud.desktop-snapshot/1.0"
	discoverySchemaVersion = "contentcloud.desktop-api-discovery/1.0"
	APIVersion             = "1.0"
	apiVersionHeader       = "X-ContentCloud-Desktop-API-Version"
	maxRequestBytes        = 64 << 10
	maxResponseBytes       = 4 << 20
)

var contentSections = []struct {
	ref   string
	label string
}{
	{ref: "10-context", label: "项目上下文"},
	{ref: "20-sources", label: "来源"},
	{ref: "30-knowledge", label: "知识"},
	{ref: "40-work", label: "工作内容"},
	{ref: "50-production", label: "生产"},
	{ref: "60-delivery", label: "交付"},
	{ref: "70-results", label: "结果"},
	{ref: "90-archive", label: "归档"},
}

type Options struct {
	Bindings         []localconfig.DaemonBinding
	Version          string
	DiscoveryPath    string
	StatePath        string
	SyncStore        *localsync.Store
	ReviewDispatcher ReviewDispatcher
	Now              func() time.Time
}

type Server struct {
	listener         net.Listener
	httpServer       *http.Server
	discoveryPath    string
	capability       string
	instanceID       string
	bindings         []localconfig.DaemonBinding
	version          string
	now              func() time.Time
	syncStore        *localsync.Store
	reviewDispatcher ReviewDispatcher
	ownsSyncStore    bool
	closeOnce        sync.Once
}

type Discovery struct {
	SchemaVersion string    `json:"schema_version"`
	InstanceID    string    `json:"instance_id"`
	Endpoint      string    `json:"endpoint"`
	Capability    string    `json:"capability"`
	APIVersions   []string  `json:"api_versions"`
	PID           int       `json:"pid"`
	CreatedAt     time.Time `json:"created_at"`
}

type Snapshot struct {
	SchemaVersion string            `json:"schema_version"`
	Daemon        DaemonStatus      `json:"daemon"`
	Projects      []ProjectSnapshot `json:"projects"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

type DaemonStatus struct {
	Connected bool   `json:"connected"`
	Version   string `json:"version"`
}

type ProjectSnapshot struct {
	ProjectID        string           `json:"project_id"`
	WorkspaceID      string           `json:"workspace_id"`
	Name             string           `json:"name"`
	LocalState       string           `json:"local_state"`
	TransferState    string           `json:"transfer_state"`
	ReviewState      string           `json:"review_state"`
	LifecycleState   string           `json:"lifecycle_state"`
	RuntimeState     string           `json:"runtime_state"`
	Content          []ContentSection `json:"content"`
	PendingFeedback  int              `json:"pending_feedback"`
	PendingDecision  int              `json:"pending_decision"`
	SourceCount      int              `json:"source_count"`
	LastSyncedAt     *time.Time       `json:"last_synced_at,omitempty"`
	LocalRevision    uint64           `json:"local_revision"`
	ObservedDigest   string           `json:"observed_digest,omitempty"`
	CloudRevision    string           `json:"cloud_revision"`
	CloudEventCursor int64            `json:"cloud_event_cursor"`
	SyncedDigest     string           `json:"synced_digest,omitempty"`
	EventCursor      uint64           `json:"event_cursor"`
	AllowedActions   []string         `json:"allowed_actions"`
	ErrorCode        string           `json:"error_code,omitempty"`
}

type ContentSection struct {
	Ref   string                  `json:"ref"`
	Label string                  `json:"label"`
	Items []ContentDirectoryEntry `json:"items"`
}

type ContentDirectoryEntry struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind"`
	ByteSize int64  `json:"byte_size,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

func DefaultDiscoveryPath() (string, error) {
	path, err := localconfig.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "desktop-api.json"), nil
}

func DefaultStatePath() (string, error) {
	path, err := localconfig.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "desktop.sqlite3"), nil
}

func Start(options Options) (*Server, error) {
	discoveryPath := strings.TrimSpace(options.DiscoveryPath)
	if discoveryPath == "" {
		var err error
		discoveryPath, err = DefaultDiscoveryPath()
		if err != nil {
			return nil, err
		}
	}
	capability, err := randomToken("dsk_", 32)
	if err != nil {
		return nil, err
	}
	instanceID, err := randomToken("dsi_", 18)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	store := options.SyncStore
	ownsStore := false
	if store == nil {
		statePath := strings.TrimSpace(options.StatePath)
		if statePath == "" {
			statePath, err = DefaultStatePath()
			if err != nil {
				_ = listener.Close()
				return nil, err
			}
		}
		store, err = localsync.Open(statePath)
		if err != nil {
			_ = listener.Close()
			return nil, err
		}
		ownsStore = true
	}
	server := &Server{
		listener: listener, discoveryPath: discoveryPath, capability: capability,
		instanceID: instanceID, bindings: cloneBindings(options.Bindings), version: strings.TrimSpace(options.Version), now: now,
		syncStore: store, reviewDispatcher: options.ReviewDispatcher, ownsSyncStore: ownsStore,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", server.authorize(server.health))
	mux.HandleFunc("GET /v1/capabilities", server.authorize(server.capabilities))
	mux.HandleFunc("GET /v1/snapshot", server.authorize(server.negotiate(server.snapshot)))
	mux.HandleFunc("GET /v1/projects/{project_id}/events", server.authorize(server.negotiate(server.projectEvents)))
	mux.HandleFunc("POST /v1/commands/workspace-publish", server.authorize(server.negotiate(server.publishWorkspace)))
	mux.HandleFunc("GET /v1/projects/{project_id}/review/inbox", server.authorize(server.negotiate(server.reviewInbox)))
	mux.HandleFunc("GET /v1/projects/{project_id}/review/revisions/{revision_id}", server.authorize(server.negotiate(server.reviewRevision)))
	mux.HandleFunc("POST /v1/projects/{project_id}/review/comments", server.authorize(server.negotiate(server.reviewComment)))
	mux.HandleFunc("POST /v1/projects/{project_id}/review/revisions/{revision_id}/approve", server.authorize(server.negotiate(server.reviewDecision("desktop.review.approve"))))
	mux.HandleFunc("POST /v1/projects/{project_id}/review/revisions/{revision_id}/reject", server.authorize(server.negotiate(server.reviewDecision("desktop.review.reject"))))
	mux.HandleFunc("POST /v1/projects/{project_id}/review/revisions/{revision_id}/request-changes", server.authorize(server.negotiate(server.reviewDecision("desktop.review.request_changes"))))
	server.httpServer = &http.Server{
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	if err := server.writeDiscovery(); err != nil {
		_ = listener.Close()
		if ownsStore {
			_ = store.Close()
		}
		return nil, err
	}
	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	return server, nil
}

func (s *Server) Close(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		closeErr = s.httpServer.Shutdown(ctx)
		var discovery Discovery
		body, err := os.ReadFile(s.discoveryPath)
		if err == nil && json.Unmarshal(body, &discovery) == nil && discovery.InstanceID == s.instanceID {
			if removeErr := os.Remove(s.discoveryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && closeErr == nil {
				closeErr = removeErr
			}
		}
		if s.ownsSyncStore {
			if storeErr := s.syncStore.Close(); storeErr != nil && closeErr == nil {
				closeErr = storeErr
			}
		}
	})
	return closeErr
}

func (s *Server) writeDiscovery() error {
	discovery := Discovery{
		SchemaVersion: discoverySchemaVersion,
		InstanceID:    s.instanceID,
		Endpoint:      "http://" + s.listener.Addr().String(),
		Capability:    s.capability,
		APIVersions:   []string{APIVersion},
		PID:           os.Getpid(),
		CreatedAt:     s.now().UTC(),
	}
	body, err := json.MarshalIndent(discovery, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.discoveryPath), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.discoveryPath), ".desktop-api-*.tmp")
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
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.discoveryPath)
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Origin") != "" {
			writeError(writer, http.StatusForbidden, "DESKTOP_API_ORIGIN_REJECTED")
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(s.capability) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.capability)) != 1 {
			writeError(writer, http.StatusUnauthorized, "DESKTOP_API_UNAUTHORIZED")
			return
		}
		next(writer, request)
	}
}

func (s *Server) negotiate(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if strings.TrimSpace(request.Header.Get(apiVersionHeader)) != APIVersion {
			writeError(writer, http.StatusUpgradeRequired, "DESKTOP_API_VERSION_UNSUPPORTED")
			return
		}
		next(writer, request)
	}
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "version": s.version})
}

func (s *Server) capabilities(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, APICapabilities{
		SchemaVersion: "contentcloud.desktop-api-capabilities/1.0",
		APIVersions:   []string{APIVersion}, SnapshotSchema: SchemaVersion,
		CommandSchema: CommandSchemaVersion, EventSchema: EventStreamSchemaVersion,
		Commands: []string{"workspace.publish", "project.projection", "review.inbox", "review.show", "review.comment", "review.approve", "review.reject", "review.request_changes"},
	})
}

func (s *Server) snapshot(writer http.ResponseWriter, request *http.Request) {
	now := s.now().UTC()
	projects := make([]ProjectSnapshot, 0)
	for _, binding := range s.bindings {
		for _, candidate := range binding.Workspaces {
			if strings.TrimSpace(candidate.Root) == "" || strings.TrimSpace(candidate.ProjectID) == "" {
				continue
			}
			projects = append(projects, s.projectSnapshot(request.Context(), candidate, now))
		}
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name != projects[j].Name {
			return projects[i].Name < projects[j].Name
		}
		return projects[i].ProjectID < projects[j].ProjectID
	})
	writeJSON(writer, http.StatusOK, Snapshot{
		SchemaVersion: SchemaVersion,
		Daemon:        DaemonStatus{Connected: true, Version: s.version},
		Projects:      projects,
		GeneratedAt:   now,
	})
}

func (s *Server) projectSnapshot(ctx context.Context, candidate localconfig.DaemonWorkspace, now time.Time) ProjectSnapshot {
	project := ProjectSnapshot{
		ProjectID: candidate.ProjectID, WorkspaceID: candidate.WorkspaceID,
		Name: workspaceName(candidate.Root, candidate.ProjectID), LocalState: "clean", TransferState: "idle",
		ReviewState: "unsubmitted", LifecycleState: "draft", RuntimeState: "succeeded", Content: []ContentSection{}, CloudRevision: "0", AllowedActions: []string{},
	}
	status, err := localworkspace.LoadStatus(candidate.Root)
	if err != nil {
		project.LocalState, project.RuntimeState, project.ErrorCode = "conflict", "failed", "WORKSPACE_STATUS_UNAVAILABLE"
		return project
	}
	project.SourceCount = status.SourceCount
	project.PendingFeedback = status.PendingFeedbackCount
	project.PendingDecision = status.PendingDecisionCount
	project.LastSyncedAt = status.Sync.LastPulledAt
	if status.PendingFeedbackCount+status.PendingDecisionCount > 0 {
		project.ReviewState = "changes_requested"
	}
	if len(status.ModifiedManagedFiles) > 0 || len(status.MissingManagedFiles) > 0 {
		project.LocalState = "modified"
	}
	observation, observationErr := localsync.ObserveWorkspace(candidate.Root)
	if observationErr != nil {
		project.LocalState, project.ErrorCode = "conflict", syncErrorCode(observationErr, "WORKSPACE_OBSERVATION_FAILED")
	} else if observation.ProjectID != candidate.ProjectID || observation.WorkspaceID != candidate.WorkspaceID {
		project.LocalState, project.ErrorCode = "conflict", "WORKSPACE_BINDING_MISMATCH"
	} else {
		state, stateErr := s.syncStore.ObserveProject(ctx, candidate.ProjectID, candidate.WorkspaceID, observation.Digest, now)
		if stateErr != nil {
			project.LocalState, project.ErrorCode = "conflict", syncErrorCode(stateErr, "LOCAL_SYNC_STATE_FAILED")
		} else {
			project.LocalRevision = state.LocalRevision
			project.ObservedDigest = state.ObservedDigest
			project.CloudRevision = state.CloudRevision
			project.CloudEventCursor = state.CloudCursor
			project.SyncedDigest = state.SyncedDigest
			project.EventCursor = state.EventCursor
			project.LastSyncedAt = state.SyncedAt
			project.TransferState = state.TransferState
			if state.ConflictCode != "" {
				project.LocalState, project.ErrorCode = "conflict", state.ConflictCode
			}
			if state.TransferState == "idle" && state.ConflictCode == "" {
				project.AllowedActions = append(project.AllowedActions, "workspace.publish")
			}
		}
	}
	for _, section := range contentSections {
		view, viewErr := localworkspace.BuildWorkspaceView(localworkspace.WorkspaceViewOptions{Root: candidate.Root, View: "file", Ref: section.ref, Now: now})
		if viewErr != nil {
			continue
		}
		entries, ok := view.View.Data.([]localworkspace.WorkspaceDirectoryEntry)
		if !ok {
			continue
		}
		items := make([]ContentDirectoryEntry, 0, len(entries))
		for _, entry := range entries {
			items = append(items, ContentDirectoryEntry{Ref: entry.Ref, Kind: entry.Kind, ByteSize: entry.ByteSize, MIMEType: entry.MIMEType})
		}
		project.Content = append(project.Content, ContentSection{Ref: section.ref, Label: section.label, Items: items})
	}
	if s.reviewDispatcher != nil {
		if raw, projectionErr := s.reviewDispatcher(ctx, candidate.ProjectID, "desktop.project.projection", map[string]any{"project_id": candidate.ProjectID}); projectionErr == nil && len(raw) > 0 {
			var projection struct {
				ProjectID       string `json:"project_id"`
				ReviewState     string `json:"review_state"`
				RuntimeState    string `json:"runtime_state"`
				LifecycleState  string `json:"lifecycle_state"`
				PendingFeedback int    `json:"pending_feedback"`
				PendingDecision int    `json:"pending_decision"`
			}
			if json.Unmarshal(raw, &projection) == nil && projection.ProjectID == candidate.ProjectID {
				project.ReviewState, project.RuntimeState, project.LifecycleState = projection.ReviewState, projection.RuntimeState, projection.LifecycleState
				project.PendingFeedback, project.PendingDecision = projection.PendingFeedback, projection.PendingDecision
			}
		}
	}
	return project
}

func (s *Server) publishWorkspace(writer http.ResponseWriter, request *http.Request) {
	var command PublishWorkspaceCommand
	if err := decodeRequest(request, &command); err != nil {
		writeError(writer, http.StatusBadRequest, "DESKTOP_COMMAND_INVALID")
		return
	}
	if code := validatePublishCommand(command); code != "" {
		writeError(writer, http.StatusBadRequest, code)
		return
	}
	candidate, ok := s.workspaceBinding(command.ProjectID, command.WorkspaceID)
	if !ok {
		writeError(writer, http.StatusNotFound, "DESKTOP_PROJECT_NOT_BOUND")
		return
	}
	observation, err := localsync.ObserveWorkspace(candidate.Root)
	if err != nil {
		writeError(writer, http.StatusConflict, syncErrorCode(err, "WORKSPACE_OBSERVATION_FAILED"))
		return
	}
	if observation.ProjectID != command.ProjectID || observation.WorkspaceID != command.WorkspaceID {
		writeError(writer, http.StatusConflict, "WORKSPACE_BINDING_MISMATCH")
		return
	}
	if observation.Digest != command.ObservedDigest {
		writeError(writer, http.StatusConflict, "WORKSPACE_DIGEST_STALE")
		return
	}
	if _, err := s.syncStore.ObserveProject(request.Context(), command.ProjectID, command.WorkspaceID, observation.Digest, s.now().UTC()); err != nil {
		writeError(writer, http.StatusConflict, syncErrorCode(err, "LOCAL_SYNC_STATE_FAILED"))
		return
	}
	result, err := s.syncStore.QueuePublish(request.Context(), localsync.PublishCommand{
		RequestID: command.RequestID, DeviceID: s.deviceIDForWorkspace(command.ProjectID, command.WorkspaceID), WorkspaceID: command.WorkspaceID, ProjectID: command.ProjectID,
		SubjectRef: command.SubjectRef, BaseRevision: command.BaseRevision, ObservedDigest: command.ObservedDigest,
		Files:          observation.Files,
		IdempotencyKey: command.IdempotencyKey, CreatedAt: s.now().UTC(),
	})
	if err != nil {
		writeError(writer, http.StatusConflict, syncErrorCode(err, "DESKTOP_COMMAND_REJECTED"))
		return
	}
	writeJSON(writer, http.StatusAccepted, CommandResponse{
		SchemaVersion: CommandResultSchemaVersion, RequestID: command.RequestID, CommandID: result.CommandID,
		ProjectID: result.ProjectID, State: result.State, EventCursor: result.EventCursor, AcceptedAt: result.CreatedAt,
	})
}

func (s *Server) projectEvents(writer http.ResponseWriter, request *http.Request) {
	projectID := strings.TrimSpace(request.PathValue("project_id"))
	candidate, ok := s.workspaceBinding(projectID, "")
	if !ok {
		writeError(writer, http.StatusNotFound, "DESKTOP_PROJECT_NOT_BOUND")
		return
	}
	after, err := parseUintQuery(request, "after", 0)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "DESKTOP_EVENT_CURSOR_INVALID")
		return
	}
	limitValue, err := parseUintQuery(request, "limit", 100)
	if err != nil || limitValue == 0 || limitValue > 200 {
		writeError(writer, http.StatusBadRequest, "DESKTOP_EVENT_LIMIT_INVALID")
		return
	}
	observation, err := localsync.ObserveWorkspace(candidate.Root)
	if err != nil {
		writeError(writer, http.StatusConflict, syncErrorCode(err, "WORKSPACE_OBSERVATION_FAILED"))
		return
	}
	if _, err := s.syncStore.ObserveProject(request.Context(), projectID, candidate.WorkspaceID, observation.Digest, s.now().UTC()); err != nil {
		writeError(writer, http.StatusConflict, syncErrorCode(err, "LOCAL_SYNC_STATE_FAILED"))
		return
	}
	events, next, gap, err := s.syncStore.ListEvents(request.Context(), projectID, after, int(limitValue))
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "DESKTOP_EVENTS_UNAVAILABLE")
		return
	}
	items := make([]DesktopEvent, 0, len(events))
	for _, event := range events {
		items = append(items, DesktopEvent{ID: event.ID, ProjectID: event.ProjectID, Cursor: event.Cursor, Type: event.Type, Payload: event.Payload, CreatedAt: event.CreatedAt})
	}
	writeJSON(writer, http.StatusOK, EventStream{
		SchemaVersion: EventStreamSchemaVersion, ProjectID: projectID, Events: items,
		NextCursor: next, Gap: gap, ResyncRequired: gap,
	})
}

type reviewCommandBody struct {
	RevisionID  string `json:"revision_id,omitempty"`
	Body        string `json:"body,omitempty"`
	Reason      string `json:"reason,omitempty"`
	JSONPointer string `json:"json_pointer,omitempty"`
}

func (s *Server) dispatchReview(writer http.ResponseWriter, request *http.Request, command string, params map[string]any) {
	projectID := strings.TrimSpace(request.PathValue("project_id"))
	if projectID == "" {
		writeError(writer, http.StatusBadRequest, "DESKTOP_PROJECT_INVALID")
		return
	}
	if _, ok := s.workspaceBinding(projectID, ""); !ok {
		writeError(writer, http.StatusNotFound, "DESKTOP_PROJECT_NOT_BOUND")
		return
	}
	if s.reviewDispatcher == nil {
		writeError(writer, http.StatusServiceUnavailable, "DESKTOP_REVIEW_UNAVAILABLE")
		return
	}
	params["project_id"] = projectID
	value, err := s.reviewDispatcher(request.Context(), projectID, command, params)
	if err != nil {
		writeError(writer, http.StatusBadGateway, syncErrorCode(err, "DESKTOP_REVIEW_UPSTREAM_FAILED"))
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (s *Server) reviewInbox(writer http.ResponseWriter, request *http.Request) {
	s.dispatchReview(writer, request, "desktop.review.inbox", map[string]any{})
}

func (s *Server) reviewRevision(writer http.ResponseWriter, request *http.Request) {
	s.dispatchReview(writer, request, "desktop.review.show", map[string]any{"revision_id": strings.TrimSpace(request.PathValue("revision_id"))})
}

func (s *Server) reviewComment(writer http.ResponseWriter, request *http.Request) {
	var body reviewCommandBody
	if err := decodeRequest(request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, "DESKTOP_REVIEW_COMMAND_INVALID")
		return
	}
	s.dispatchReview(writer, request, "desktop.review.comment", map[string]any{
		"revision_id": strings.TrimSpace(body.RevisionID), "body": body.Body, "json_pointer": body.JSONPointer,
	})
}

func (s *Server) reviewDecision(command string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var body reviewCommandBody
		if err := decodeRequest(request, &body); err != nil {
			writeError(writer, http.StatusBadRequest, "DESKTOP_REVIEW_COMMAND_INVALID")
			return
		}
		s.dispatchReview(writer, request, command, map[string]any{
			"revision_id": strings.TrimSpace(request.PathValue("revision_id")), "reason": body.Reason, "json_pointer": body.JSONPointer,
		})
	}
}

func (s *Server) workspaceBinding(projectID, workspaceID string) (localconfig.DaemonWorkspace, bool) {
	projectID, workspaceID = strings.TrimSpace(projectID), strings.TrimSpace(workspaceID)
	for _, binding := range s.bindings {
		for _, candidate := range binding.Workspaces {
			if strings.TrimSpace(candidate.ProjectID) == projectID && (workspaceID == "" || strings.TrimSpace(candidate.WorkspaceID) == workspaceID) {
				return candidate, true
			}
		}
	}
	return localconfig.DaemonWorkspace{}, false
}

func (s *Server) deviceIDForWorkspace(projectID, workspaceID string) string {
	for _, binding := range s.bindings {
		for _, candidate := range binding.Workspaces {
			if strings.TrimSpace(candidate.ProjectID) == strings.TrimSpace(projectID) && strings.TrimSpace(candidate.WorkspaceID) == strings.TrimSpace(workspaceID) {
				return strings.TrimSpace(binding.DeviceID)
			}
		}
	}
	return ""
}

func parseUintQuery(request *http.Request, name string, fallback uint64) (uint64, error) {
	value := strings.TrimSpace(request.URL.Query().Get(name))
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func workspaceName(root, fallback string) string {
	name := strings.TrimSpace(filepath.Base(filepath.Clean(root)))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return fallback
	}
	return name
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > maxResponseBytes {
		writeError(writer, http.StatusInternalServerError, "DESKTOP_API_RESPONSE_INVALID")
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(fmt.Sprintf(`{"error":{"code":%q}}`, code)))
}

func randomToken(prefix string, size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func cloneBindings(bindings []localconfig.DaemonBinding) []localconfig.DaemonBinding {
	result := make([]localconfig.DaemonBinding, len(bindings))
	for index, binding := range bindings {
		result[index] = binding
		result[index].Workspaces = append([]localconfig.DaemonWorkspace(nil), binding.Workspaces...)
	}
	return result
}
