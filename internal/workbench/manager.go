package workbench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

const (
	DescriptorSchema          = "contentcloud.workbench-handoff/1.0"
	SnapshotSchema            = "contentcloud.workbench-snapshot/1.0"
	EventSchema               = "contentcloud.workbench-event/1.0"
	handoffTTL                = time.Minute
	capabilityTTL             = 30 * time.Minute
	absoluteTTL               = 4 * time.Hour
	workspaceRequestBodyLimit = 2*1024*1024 + 64*1024
	resourceCookieName        = "contentcloud_workbench_resource"
)

//go:embed ui/*
var embeddedUI embed.FS

type OpenOptions struct {
	Root                    string
	View                    string
	Ref                     string
	RunID                   string
	ExpectedContextRevision uint64
	ExpectedDigest          string
}

type BrowserHandoff struct {
	Required      bool   `json:"required"`
	PreferredMode string `json:"preferred_mode"`
	BrowserAction string `json:"browser_action"`
}

type Descriptor struct {
	SchemaVersion     string                       `json:"schema_version"`
	WorkbenchID       string                       `json:"workbench_id"`
	WorkspaceID       string                       `json:"workspace_id"`
	ProjectID         string                       `json:"project_id"`
	RunID             string                       `json:"run_id,omitempty"`
	SessionState      string                       `json:"session_state"`
	SessionGeneration string                       `json:"session_generation"`
	View              string                       `json:"view"`
	Ref               string                       `json:"ref,omitempty"`
	BrowserHandoff    BrowserHandoff               `json:"browser_handoff"`
	Fallback          localworkspace.WorkspaceView `json:"fallback"`
}

type PrivateHandoff struct {
	WorkbenchID string `json:"workbench_id"`
	URL         string `json:"url"`
	Origin      string `json:"origin"`
}

type OpenResult struct {
	Descriptor Descriptor
	Private    PrivateHandoff
}

type Status struct {
	WorkbenchID       string    `json:"workbench_id"`
	WorkspaceID       string    `json:"workspace_id"`
	ProjectID         string    `json:"project_id"`
	State             string    `json:"state"`
	SessionGeneration string    `json:"session_generation"`
	StartedAt         time.Time `json:"started_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type Manager struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	now       func() time.Time
	proposals *localworkspace.ProposalStore
}

func NewManager(now func() time.Time) *Manager {
	return NewManagerWithProposalStore(now, nil)
}

func NewManagerWithProposalStore(now func() time.Time, proposals *localworkspace.ProposalStore) *Manager {
	if now == nil {
		now = time.Now
	}
	if proposals == nil {
		proposals = localworkspace.NewProposalStore()
	}
	return &Manager{sessions: map[string]*Session{}, now: now, proposals: proposals}
}

func (m *Manager) Open(ctx context.Context, options OpenOptions) (OpenResult, error) {
	root, err := localworkspace.FindRoot(options.Root)
	if err != nil {
		return OpenResult{}, err
	}
	options.Root = root
	if strings.TrimSpace(options.View) == "" {
		options.View = "workspace_summary"
	}
	view, err := localworkspace.BuildWorkspaceView(workspaceViewOptions(options, m.now()))
	if err != nil {
		return OpenResult{}, err
	}
	binding, err := localworkspace.ObserveSessionBinding(root, m.now())
	if err != nil {
		return OpenResult{}, err
	}

	m.mu.Lock()
	session := m.sessions[root]
	if session != nil && (session.Generation() != binding.Generation || session.Expired(m.now()) || session.Closed()) {
		delete(m.sessions, root)
		if !session.Closed() {
			session.closeAsync()
		}
		session = nil
	}
	if session == nil {
		session, err = newSession(ctx, options, view, binding.Generation, m.now, m.proposals)
		if err == nil {
			m.sessions[root] = session
		}
	}
	m.mu.Unlock()
	if err != nil {
		return OpenResult{}, err
	}
	session.SetView(options)
	token, err := session.IssueHandoff(m.now())
	if err != nil {
		return OpenResult{}, err
	}
	descriptor := Descriptor{
		SchemaVersion: DescriptorSchema, WorkbenchID: session.ID(), WorkspaceID: view.WorkspaceID, ProjectID: view.ProjectID,
		RunID: view.RunID, SessionState: "ready", SessionGeneration: binding.Generation, View: options.View, Ref: options.Ref,
		BrowserHandoff: BrowserHandoff{Required: true, PreferredMode: "codex-internal-browser", BrowserAction: "navigate"},
		Fallback:       view,
	}
	return OpenResult{Descriptor: descriptor, Private: PrivateHandoff{WorkbenchID: session.ID(), URL: session.Origin() + "/#handoff=" + token, Origin: session.Origin()}}, nil
}

func (m *Manager) Status(root string) (Status, error) {
	resolved, err := localworkspace.FindRoot(root)
	if err != nil {
		return Status{}, err
	}
	m.mu.Lock()
	session := m.sessions[resolved]
	if session != nil && (session.Expired(m.now()) || session.Closed()) {
		delete(m.sessions, resolved)
		if !session.Closed() {
			session.closeAsync()
		}
		session = nil
	}
	m.mu.Unlock()
	if session == nil {
		return Status{}, domain.NotFound("本地 Workbench 会话")
	}
	return session.Status(), nil
}

func (m *Manager) CloseWorkspace(root string) error {
	resolved, err := localworkspace.FindRoot(root)
	if err != nil {
		return err
	}
	m.mu.Lock()
	session := m.sessions[resolved]
	delete(m.sessions, resolved)
	m.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Close()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.sessions = map[string]*Session{}
	m.mu.Unlock()
	var joined error
	for _, session := range sessions {
		joined = errors.Join(joined, session.Close())
	}
	m.proposals.Clear()
	return joined
}

type Session struct {
	mu                   sync.Mutex
	commandMu            sync.Mutex
	id                   string
	root                 string
	origin               string
	workspaceID          string
	projectID            string
	generation           string
	startedAt            time.Time
	expiresAt            time.Time
	now                  func() time.Time
	view                 OpenOptions
	handoffs             map[string]time.Time
	capabilities         map[string]clientCapability
	resourceCapabilities map[string]time.Time
	resources            map[string]string
	proposalStore        *localworkspace.ProposalStore
	idempotency          map[string]idempotencyRecord
	events               []Event
	nextEventID          uint64
	subscribers          map[uint64]chan Event
	nextSubscriberID     uint64
	server               *http.Server
	listener             net.Listener
	closed               chan struct{}
	closeOnce            sync.Once
}

type clientCapability struct {
	CSRF      string
	ExpiresAt time.Time
}

type idempotencyRecord struct {
	Operation   string
	Fingerprint string
	Value       any
}

type Event struct {
	SchemaVersion   string    `json:"schema_version"`
	EventID         uint64    `json:"event_id"`
	WorkbenchID     string    `json:"workbench_id"`
	WorkspaceID     string    `json:"workspace_id"`
	ProjectID       string    `json:"project_id"`
	Topic           string    `json:"topic"`
	ContextRevision uint64    `json:"context_revision,omitempty"`
	Refs            []string  `json:"refs"`
	OccurredAt      time.Time `json:"occurred_at"`
}

type Snapshot struct {
	SchemaVersion     string                          `json:"schema_version"`
	WorkbenchID       string                          `json:"workbench_id"`
	WorkspaceID       string                          `json:"workspace_id"`
	ProjectID         string                          `json:"project_id"`
	SessionGeneration string                          `json:"session_generation"`
	View              localworkspace.WorkspaceView    `json:"view"`
	Resources         []BrowserResource               `json:"resources"`
	Ownership         *localworkspace.RunClaimSummary `json:"ownership,omitempty"`
}

type BrowserResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MIMEType string `json:"mime_type"`
	Digest   string `json:"digest"`
	ByteSize int64  `json:"byte_size"`
	URL      string `json:"url"`
}

func newSession(ctx context.Context, options OpenOptions, view localworkspace.WorkspaceView, generation string, now func() time.Time, proposals *localworkspace.ProposalStore) (*Session, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("启动本地 Workbench listener: %w", err)
	}
	startedAt := now().UTC()
	session := &Session{
		id: "wbk_" + randomID(18), root: options.Root, origin: "http://" + listener.Addr().String(),
		workspaceID: view.WorkspaceID, projectID: view.ProjectID, generation: generation,
		startedAt: startedAt, expiresAt: startedAt.Add(absoluteTTL), now: now, view: options,
		handoffs: map[string]time.Time{}, capabilities: map[string]clientCapability{}, resourceCapabilities: map[string]time.Time{}, resources: map[string]string{},
		proposalStore: proposals, idempotency: map[string]idempotencyRecord{},
		subscribers: map[uint64]chan Event{}, listener: listener, closed: make(chan struct{}),
	}
	session.server = &http.Server{Handler: session.routes(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if serveErr := session.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			session.closeAsync()
		}
	}()
	go session.watch(ctx)
	return session, nil
}

func (s *Session) ID() string         { return s.id }
func (s *Session) Origin() string     { return s.origin }
func (s *Session) Generation() string { return s.generation }

func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{WorkbenchID: s.id, WorkspaceID: s.workspaceID, ProjectID: s.projectID, State: "ready", SessionGeneration: s.generation, StartedAt: s.startedAt, ExpiresAt: s.expiresAt}
}

func (s *Session) Expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !now.Before(s.expiresAt)
}

func (s *Session) Closed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *Session) SetView(options OpenOptions) {
	s.mu.Lock()
	s.view = options
	s.mu.Unlock()
}

func (s *Session) IssueHandoff(now time.Time) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !now.Before(s.expiresAt) {
		return "", domain.Conflict("WORKBENCH_SESSION_EXPIRED", "本地 Workbench 会话已到期")
	}
	s.handoffs[tokenHash(token)] = now.Add(handoffTTL)
	return token, nil
}

func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.publish("session.closed", 0, nil)
		close(s.closed)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closeErr = s.server.Shutdown(ctx)
		if closeErr != nil {
			closeErr = errors.Join(closeErr, s.server.Close())
		}
	})
	return closeErr
}

func (s *Session) closeAsync() { go func() { _ = s.Close() }() }

func (s *Session) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.serveIndex)
	mux.HandleFunc("GET /assets/{name}", s.serveAsset)
	mux.HandleFunc("POST /api/v1/session/exchange", s.exchange)
	mux.HandleFunc("DELETE /api/v1/session", s.withCapability(true, s.closeHTTP))
	mux.HandleFunc("GET /api/v1/bootstrap", s.withCapability(false, s.bootstrap))
	mux.HandleFunc("GET /api/v1/views/{kind}", s.withCapability(false, s.viewHTTP))
	mux.HandleFunc("GET /api/v1/resources/{id}", s.withResourceCapability(s.resourceHTTP))
	mux.HandleFunc("GET /api/v1/events", s.withCapability(false, s.eventsHTTP))
	mux.HandleFunc("POST /api/v1/ownership/claim", s.withCapability(true, s.claimOwnershipHTTP))
	mux.HandleFunc("POST /api/v1/ownership/takeover", s.withCapability(true, s.takeoverOwnershipHTTP))
	mux.HandleFunc("POST /api/v1/proposals", s.withCapability(true, s.prepareProposalHTTP))
	mux.HandleFunc("POST /api/v1/proposals/{id}/apply", s.withCapability(true, s.applyProposalHTTP))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		s.securityHeaders(response)
		if request.Host != strings.TrimPrefix(s.origin, "http://") {
			http.Error(response, "invalid host", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(response, request)
	})
}

func (s *Session) serveIndex(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(response, request)
		return
	}
	s.serveUIFile(response, "index.html", "text/html; charset=utf-8")
}

func (s *Session) serveAsset(response http.ResponseWriter, request *http.Request) {
	name := path.Base(request.PathValue("name"))
	if name != request.PathValue("name") || name == "." || name == "" {
		http.NotFound(response, request)
		return
	}
	if name == "sw.js" {
		response.Header().Set("Service-Worker-Allowed", "/")
	}
	mediaType := mime.TypeByExtension(path.Ext(name))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	s.serveUIFile(response, name, mediaType)
}

func (s *Session) serveUIFile(response http.ResponseWriter, name, mediaType string) {
	body, err := fs.ReadFile(embeddedUI, "ui/"+name)
	if err != nil {
		http.Error(response, "not found", http.StatusNotFound)
		return
	}
	response.Header().Set("Content-Type", mediaType)
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(body)
}

func (s *Session) exchange(response http.ResponseWriter, request *http.Request) {
	if !s.validWriteOrigin(request) {
		writeHTTPError(response, domain.Policy("WORKBENCH_ORIGIN_INVALID", "Workbench 请求来源无效", "重新从 MCP 打开本地 Workbench"), http.StatusForbidden)
		return
	}
	if !isJSONRequest(request) {
		writeHTTPError(response, domain.Invalid("WORKBENCH_CONTENT_TYPE_INVALID", "Workbench 写入请求必须使用 application/json"), http.StatusUnsupportedMediaType)
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSONBody(request, &input, 4096); err != nil || strings.TrimSpace(input.Token) == "" {
		writeHTTPError(response, domain.Invalid("WORKBENCH_HANDOFF_INVALID", "Workbench handoff token 无效"), http.StatusBadRequest)
		return
	}
	now := s.now()
	s.mu.Lock()
	expiresAt, ok := s.handoffs[tokenHash(input.Token)]
	if ok {
		delete(s.handoffs, tokenHash(input.Token))
	}
	s.mu.Unlock()
	if !ok || !now.Before(expiresAt) {
		writeHTTPError(response, domain.Conflict("WORKBENCH_HANDOFF_EXPIRED", "Workbench handoff 已失效或已使用"), http.StatusGone)
		return
	}
	capability, err := randomToken(32)
	if err != nil {
		writeHTTPError(response, err, http.StatusInternalServerError)
		return
	}
	csrf, err := randomToken(24)
	if err != nil {
		writeHTTPError(response, err, http.StatusInternalServerError)
		return
	}
	resourceCapability, err := randomToken(32)
	if err != nil {
		writeHTTPError(response, err, http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	clientExpiry := now.Add(capabilityTTL)
	if clientExpiry.After(s.expiresAt) {
		clientExpiry = s.expiresAt
	}
	s.capabilities[tokenHash(capability)] = clientCapability{CSRF: csrf, ExpiresAt: clientExpiry}
	s.resourceCapabilities[tokenHash(resourceCapability)] = clientExpiry
	s.mu.Unlock()
	http.SetCookie(response, &http.Cookie{
		Name: resourceCookieName, Value: resourceCapability, Path: "/api/v1/resources/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(response, http.StatusOK, map[string]any{"capability": capability, "csrf": csrf, "expires_at": clientExpiry, "workbench_id": s.id})
}

func (s *Session) bootstrap(response http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	options := s.view
	s.mu.Unlock()
	snapshot, err := s.buildSnapshot(options)
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	writeJSON(response, http.StatusOK, snapshot)
}

func (s *Session) viewHTTP(response http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	for key := range query {
		switch key {
		case "ref", "run_id", "expected_digest", "expected_context_revision":
		default:
			writeHTTPError(response, domain.Invalid("WORKBENCH_VIEW_PARAMS_INVALID", "Workbench view 参数包含未知字段"), http.StatusBadRequest)
			return
		}
	}
	revision, err := strconv.ParseUint(defaultString(query.Get("expected_context_revision"), "0"), 10, 64)
	if err != nil {
		writeHTTPError(response, domain.Invalid("WORKBENCH_VIEW_PARAMS_INVALID", "expected_context_revision 必须是整数"), http.StatusBadRequest)
		return
	}
	options := OpenOptions{Root: s.root, View: request.PathValue("kind"), Ref: query.Get("ref"), RunID: query.Get("run_id"), ExpectedDigest: query.Get("expected_digest"), ExpectedContextRevision: revision}
	snapshot, err := s.buildSnapshot(options)
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	s.SetView(options)
	writeJSON(response, http.StatusOK, snapshot)
}

func (s *Session) buildSnapshot(options OpenOptions) (Snapshot, error) {
	view, err := localworkspace.BuildWorkspaceView(workspaceViewOptions(options, s.now()))
	if err != nil {
		return Snapshot{}, err
	}
	resources := make([]BrowserResource, 0, len(view.Resources))
	s.mu.Lock()
	for _, resource := range view.Resources {
		id := "res_" + shortHash(resource.URI)
		s.resources[id] = resource.URI
		resources = append(resources, BrowserResource{ID: id, Name: resource.Name, MIMEType: resource.MIMEType, Digest: resource.Digest, ByteSize: resource.ByteSize, URL: "/api/v1/resources/" + id})
	}
	s.mu.Unlock()
	var ownership *localworkspace.RunClaimSummary
	if view.RunID != "" {
		claim, claimErr := localworkspace.RunClaimStatus(s.root, view.RunID, s.now())
		if claimErr != nil {
			return Snapshot{}, claimErr
		}
		ownership = &claim
	}
	return Snapshot{SchemaVersion: SnapshotSchema, WorkbenchID: s.id, WorkspaceID: s.workspaceID, ProjectID: s.projectID, SessionGeneration: s.generation, View: view, Resources: resources, Ownership: ownership}, nil
}

func (s *Session) resourceHTTP(response http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	uri := s.resources[request.PathValue("id")]
	s.mu.Unlock()
	if uri == "" {
		writeHTTPError(response, domain.NotFound("Workbench 资源"), http.StatusNotFound)
		return
	}
	resource, err := localworkspace.OpenWorkspaceResource(s.root, uri)
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	defer resource.Reader.Close()
	response.Header().Set("Content-Type", resource.MIMEType)
	response.Header().Set("ETag", `"`+strings.TrimPrefix(resource.Digest, "sha256:")+`"`)
	response.Header().Set("Accept-Ranges", "bytes")
	response.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(response, request, path.Base(resource.Ref), time.Time{}, resource.Reader)
}

func (s *Session) eventsHTTP(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		writeHTTPError(response, errors.New("streaming unavailable"), http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Connection", "keep-alive")
	lastID, _ := strconv.ParseUint(request.Header.Get("Last-Event-ID"), 10, 64)
	backlog, subscriberID, events := s.subscribe(lastID)
	defer s.unsubscribe(subscriberID)
	for _, event := range backlog {
		writeSSE(response, event)
	}
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				return
			}
			writeSSE(response, event)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = io.WriteString(response, ": keepalive\n\n")
			flusher.Flush()
		case <-request.Context().Done():
			return
		case <-s.closed:
			return
		}
	}
}

func (s *Session) closeHTTP(response http.ResponseWriter, request *http.Request) {
	if _, err := requestIdempotencyKey(request); err != nil {
		writeHTTPError(response, err, http.StatusBadRequest)
		return
	}
	var input struct{}
	if err := decodeJSONBody(request, &input, 1024); err != nil {
		writeHTTPError(response, domain.Invalid("WORKBENCH_REQUEST_INVALID", "关闭 Workbench 的请求体无效"), http.StatusBadRequest)
		return
	}
	http.SetCookie(response, &http.Cookie{
		Name: resourceCookieName, Path: "/api/v1/resources/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(response, http.StatusOK, map[string]any{"closed": true, "workbench_id": s.id})
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = s.Close()
	}()
}

func (s *Session) claimOwnershipHTTP(response http.ResponseWriter, request *http.Request) {
	var input struct {
		RunID            string `json:"run_id"`
		ExpectedRevision uint64 `json:"expected_context_revision"`
		TTLSeconds       int64  `json:"ttl_seconds,omitempty"`
		TakeoverExpired  bool   `json:"takeover_expired,omitempty"`
	}
	if err := decodeJSONBody(request, &input, 16*1024); err != nil {
		writeHTTPError(response, domain.Invalid("WORKBENCH_OWNERSHIP_PARAMS_INVALID", "Workbench ownership claim 参数无效或包含未知字段"), http.StatusBadRequest)
		return
	}
	key, fingerprint, replay, err := s.prepareIdempotentRequest(request, "ownership.claim", input)
	if err != nil {
		writeHTTPError(response, err, http.StatusConflict)
		return
	}
	if replay != nil {
		writeJSON(response, http.StatusOK, replay)
		return
	}
	claim, err := localworkspace.ClaimRun(localworkspace.ClaimRunOptions{
		Root: s.root, RunID: input.RunID, OwnerKind: "browser", OwnerID: s.id,
		ExpectedRevision: input.ExpectedRevision, TTL: secondsDuration(input.TTLSeconds), TakeoverExpired: input.TakeoverExpired, Now: s.now(),
	})
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	s.storeIdempotentResult(key, "ownership.claim", fingerprint, claim)
	s.publish("claim.changed", claim.ContextRevision, nil)
	writeJSON(response, http.StatusOK, claim)
}

func (s *Session) takeoverOwnershipHTTP(response http.ResponseWriter, request *http.Request) {
	var input struct {
		RunID             string `json:"run_id"`
		ExpectedOwnerKind string `json:"expected_owner_kind"`
		ExpectedOwnerID   string `json:"expected_owner_id"`
		ExpectedEpoch     uint64 `json:"expected_epoch"`
		ExpectedRevision  uint64 `json:"expected_context_revision"`
		TTLSeconds        int64  `json:"ttl_seconds,omitempty"`
	}
	if err := decodeJSONBody(request, &input, 16*1024); err != nil {
		writeHTTPError(response, domain.Invalid("WORKBENCH_OWNERSHIP_PARAMS_INVALID", "Workbench ownership takeover 参数无效或包含未知字段"), http.StatusBadRequest)
		return
	}
	key, fingerprint, replay, err := s.prepareIdempotentRequest(request, "ownership.takeover", input)
	if err != nil {
		writeHTTPError(response, err, http.StatusConflict)
		return
	}
	if replay != nil {
		writeJSON(response, http.StatusOK, replay)
		return
	}
	claim, err := localworkspace.TakeoverRunClaim(localworkspace.TakeoverRunClaimOptions{
		Root: s.root, RunID: input.RunID, OwnerKind: "browser", OwnerID: s.id,
		ExpectedOwnerKind: input.ExpectedOwnerKind, ExpectedOwnerID: input.ExpectedOwnerID,
		ExpectedEpoch: input.ExpectedEpoch, ExpectedRevision: input.ExpectedRevision,
		TTL: secondsDuration(input.TTLSeconds), Now: s.now(),
	})
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	s.storeIdempotentResult(key, "ownership.takeover", fingerprint, claim)
	s.publish("claim.changed", claim.ContextRevision, nil)
	writeJSON(response, http.StatusOK, claim)
}

func (s *Session) prepareProposalHTTP(response http.ResponseWriter, request *http.Request) {
	var input struct {
		RunID            string `json:"run_id"`
		ClaimToken       string `json:"claim_token"`
		OwnerEpoch       uint64 `json:"owner_epoch"`
		ExpectedRevision uint64 `json:"expected_context_revision"`
		TypedAction      string `json:"typed_action"`
		Ref              string `json:"ref"`
		ExpectedDigest   string `json:"expected_digest"`
		Content          string `json:"content"`
	}
	if err := decodeJSONBody(request, &input, workspaceRequestBodyLimit); err != nil {
		writeHTTPError(response, domain.Invalid("WORKBENCH_PROPOSAL_PARAMS_INVALID", "Workbench Proposal 参数无效、过大或包含未知字段"), http.StatusBadRequest)
		return
	}
	key, fingerprint, replay, err := s.prepareIdempotentRequest(request, "proposal.prepare", input)
	if err != nil {
		writeHTTPError(response, err, http.StatusConflict)
		return
	}
	if replay != nil {
		writeJSON(response, http.StatusOK, replay)
		return
	}
	proposal, err := s.proposalStore.Prepare(localworkspace.PrepareWorkspaceProposalOptions{
		Root: s.root, RunID: input.RunID, ClaimToken: input.ClaimToken, OwnerKind: "browser", OwnerID: s.id, OwnerEpoch: input.OwnerEpoch,
		ExpectedContextRevision: input.ExpectedRevision, TypedAction: input.TypedAction, Ref: input.Ref,
		ExpectedDigest: input.ExpectedDigest, Content: input.Content, Now: s.now(),
	})
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	s.storeIdempotentResult(key, "proposal.prepare", fingerprint, proposal)
	s.publish("proposal.changed", proposal.BaseContextRevision, proposal.AffectedPaths)
	writeJSON(response, http.StatusCreated, proposal)
}

func (s *Session) applyProposalHTTP(response http.ResponseWriter, request *http.Request) {
	proposalID := request.PathValue("id")
	var input struct {
		ClaimToken       string `json:"claim_token"`
		OwnerEpoch       uint64 `json:"owner_epoch"`
		ExpectedRevision uint64 `json:"expected_context_revision"`
		Confirm          bool   `json:"confirm"`
	}
	if err := decodeJSONBody(request, &input, 16*1024); err != nil || !input.Confirm {
		writeHTTPError(response, domain.Invalid("WORKBENCH_PROPOSAL_APPLY_INVALID", "Apply 必须准确确认 Proposal 且参数不能包含未知字段"), http.StatusBadRequest)
		return
	}
	key, fingerprint, replay, err := s.prepareIdempotentRequest(request, "proposal.apply:"+proposalID, input)
	if err != nil {
		writeHTTPError(response, err, http.StatusConflict)
		return
	}
	if replay != nil {
		writeJSON(response, http.StatusOK, replay)
		return
	}
	applied, err := s.proposalStore.Apply(proposalID, localworkspace.ApplyWorkspaceProposalOptions{
		Root: s.root, ClaimToken: input.ClaimToken, OwnerKind: "browser", OwnerID: s.id,
		OwnerEpoch: input.OwnerEpoch, ExpectedContextRevision: input.ExpectedRevision, Now: s.now(),
	})
	if err != nil {
		writeHTTPError(response, err, httpStatus(err))
		return
	}
	s.storeIdempotentResult(key, "proposal.apply:"+proposalID, fingerprint, applied)
	refs := make([]string, 0, len(applied.Outputs))
	for _, output := range applied.Outputs {
		refs = append(refs, output.Ref)
	}
	s.publish("proposal.applied", applied.ContextRevision, refs)
	writeJSON(response, http.StatusOK, applied)
}

func (s *Session) prepareIdempotentRequest(request *http.Request, operation string, input any) (string, string, any, error) {
	key, err := requestIdempotencyKey(request)
	if err != nil {
		return "", "", nil, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return "", "", nil, err
	}
	fingerprint := shortHash(string(body))
	s.mu.Lock()
	record, exists := s.idempotency[key]
	s.mu.Unlock()
	if !exists {
		return key, fingerprint, nil, nil
	}
	if record.Operation != operation || record.Fingerprint != fingerprint {
		return "", "", nil, domain.Conflict("WORKBENCH_IDEMPOTENCY_CONFLICT", "Idempotency-Key 已用于不同的 Workbench 操作或参数")
	}
	return key, fingerprint, record.Value, nil
}

func (s *Session) storeIdempotentResult(key, operation, fingerprint string, value any) {
	s.mu.Lock()
	s.idempotency[key] = idempotencyRecord{Operation: operation, Fingerprint: fingerprint, Value: value}
	s.mu.Unlock()
}

func (s *Session) withCapability(write bool, next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		originValid := s.validOrigin(request)
		if write {
			originValid = s.validWriteOrigin(request)
		}
		if !originValid {
			writeHTTPError(response, domain.Policy("WORKBENCH_ORIGIN_INVALID", "Workbench 请求来源无效", "重新打开本地 Workbench"), http.StatusForbidden)
			return
		}
		value := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		client, ok := s.lookupClientCapability(value)
		if !ok {
			writeHTTPError(response, domain.Policy("WORKBENCH_CAPABILITY_INVALID", "Workbench capability 无效或已过期", "从 MCP 重新打开本地 Workbench"), http.StatusUnauthorized)
			return
		}
		if write && subtle.ConstantTimeCompare([]byte(client.CSRF), []byte(request.Header.Get("X-Workbench-CSRF"))) != 1 {
			writeHTTPError(response, domain.Policy("WORKBENCH_CSRF_INVALID", "Workbench 写入请求缺少有效 CSRF nonce", "刷新工作台后重试"), http.StatusForbidden)
			return
		}
		if write && !isJSONRequest(request) {
			writeHTTPError(response, domain.Invalid("WORKBENCH_CONTENT_TYPE_INVALID", "Workbench 写入请求必须使用 application/json"), http.StatusUnsupportedMediaType)
			return
		}
		if write {
			s.commandMu.Lock()
			defer s.commandMu.Unlock()
		}
		next(response, request)
	}
}

func (s *Session) withResourceCapability(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if !s.validOrigin(request) {
			writeHTTPError(response, domain.Policy("WORKBENCH_ORIGIN_INVALID", "Workbench 请求来源无效", "重新打开本地 Workbench"), http.StatusForbidden)
			return
		}
		value := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		_, bearerOK := s.lookupClientCapability(value)
		resourceOK := false
		if !bearerOK {
			if cookie, err := request.Cookie(resourceCookieName); err == nil {
				value = cookie.Value
				s.mu.Lock()
				expiresAt, exists := s.resourceCapabilities[tokenHash(value)]
				if exists && !s.now().Before(expiresAt) {
					delete(s.resourceCapabilities, tokenHash(value))
					exists = false
				}
				s.mu.Unlock()
				resourceOK = exists && value != ""
			}
		}
		if !bearerOK && !resourceOK {
			writeHTTPError(response, domain.Policy("WORKBENCH_CAPABILITY_INVALID", "Workbench 资源 capability 无效或已过期", "从 MCP 重新打开本地 Workbench"), http.StatusUnauthorized)
			return
		}
		next(response, request)
	}
}

func (s *Session) lookupClientCapability(value string) (clientCapability, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	client, ok := s.capabilities[tokenHash(value)]
	if ok && !s.now().Before(client.ExpiresAt) {
		delete(s.capabilities, tokenHash(value))
		ok = false
	}
	return client, ok && value != ""
}

func (s *Session) validOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	site := request.Header.Get("Sec-Fetch-Site")
	return (origin == "" || origin == s.origin) && (site == "" || site == "same-origin" || site == "none")
}

func (s *Session) validWriteOrigin(request *http.Request) bool {
	return request.Header.Get("Origin") == s.origin && request.Header.Get("Sec-Fetch-Site") == "same-origin"
}

func (s *Session) securityHeaders(response http.ResponseWriter) {
	response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; style-src 'self'; script-src 'self'; worker-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("X-Frame-Options", "DENY")
	response.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	response.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
}

func (s *Session) watch(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			_ = s.Close()
			return
		case <-s.closed:
			return
		case <-ticker.C:
			if !s.now().Before(s.expiresAt) {
				_ = s.Close()
				return
			}
			s.mu.Lock()
			options := s.view
			s.mu.Unlock()
			view, err := localworkspace.BuildWorkspaceView(workspaceViewOptions(options, s.now()))
			if err != nil {
				continue
			}
			key := viewRevisionKey(view)
			if last != "" && key != last {
				s.publish("view.invalidated", view.ContextRevision, nonEmptyRefs(view.View.Ref))
			}
			last = key
		}
	}
}

func (s *Session) publish(topic string, revision uint64, refs []string) {
	s.mu.Lock()
	s.nextEventID++
	event := Event{SchemaVersion: EventSchema, EventID: s.nextEventID, WorkbenchID: s.id, WorkspaceID: s.workspaceID, ProjectID: s.projectID, Topic: topic, ContextRevision: revision, Refs: refs, OccurredAt: s.now().UTC()}
	s.events = append(s.events, event)
	if len(s.events) > 128 {
		s.events = append([]Event(nil), s.events[len(s.events)-128:]...)
	}
	for id, subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
			close(subscriber)
			delete(s.subscribers, id)
		}
	}
	s.mu.Unlock()
}

func (s *Session) subscribe(lastID uint64) ([]Event, uint64, <-chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	backlog := make([]Event, 0)
	if len(s.events) > 0 && lastID > 0 && lastID < s.events[0].EventID-1 {
		backlog = append(backlog, Event{SchemaVersion: EventSchema, EventID: s.nextEventID, WorkbenchID: s.id, WorkspaceID: s.workspaceID, ProjectID: s.projectID, Topic: "event.gap", Refs: []string{}, OccurredAt: s.now().UTC()})
	} else {
		for _, event := range s.events {
			if event.EventID > lastID {
				backlog = append(backlog, event)
			}
		}
	}
	s.nextSubscriberID++
	id := s.nextSubscriberID
	channel := make(chan Event, 16)
	s.subscribers[id] = channel
	return backlog, id, channel
}

func (s *Session) unsubscribe(id uint64) {
	s.mu.Lock()
	if subscriber, ok := s.subscribers[id]; ok {
		close(subscriber)
		delete(s.subscribers, id)
	}
	s.mu.Unlock()
}

func workspaceViewOptions(options OpenOptions, now time.Time) localworkspace.WorkspaceViewOptions {
	return localworkspace.WorkspaceViewOptions{Root: options.Root, View: options.View, Ref: options.Ref, RunID: options.RunID, ExpectedContextRevision: options.ExpectedContextRevision, ExpectedDigest: options.ExpectedDigest, Now: now}
}

func decodeJSONBody(request *http.Request, target any, limit int64) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, limit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func writeHTTPError(response http.ResponseWriter, err error, status int) {
	var domainError *domain.Error
	if !errors.As(err, &domainError) {
		domainError = domain.E("internal", "workbench", "WORKBENCH_INTERNAL", err.Error(), 1)
	}
	writeJSON(response, status, map[string]any{"error": domainError})
}

func httpStatus(err error) int {
	var domainError *domain.Error
	if !errors.As(err, &domainError) {
		return http.StatusInternalServerError
	}
	switch domainError.Type {
	case "validation":
		return http.StatusBadRequest
	case "not_found":
		return http.StatusNotFound
	case "conflict":
		return http.StatusConflict
	case "policy":
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func writeSSE(response io.Writer, event Event) {
	body, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(response, "id: %d\nevent: %s\ndata: %s\n\n", event.EventID, event.Topic, body)
}

func randomToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func randomID(size int) string {
	value, err := randomToken(size)
	if err != nil {
		panic(err)
	}
	return value
}

func tokenHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:12])
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func secondsDuration(value int64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * time.Second
}

func isJSONRequest(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func requestIdempotencyKey(request *http.Request) (string, error) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 128 {
		return "", domain.Invalid("WORKBENCH_IDEMPOTENCY_KEY_INVALID", "Workbench 写入请求需要 8 到 128 个字符的 Idempotency-Key")
	}
	return key, nil
}

func nonEmptyRefs(value string) []string {
	if value == "" {
		return []string{}
	}
	return []string{value}
}

func viewRevisionKey(view localworkspace.WorkspaceView) string {
	body, _ := json.Marshal(view)
	return shortHash(string(body))
}
