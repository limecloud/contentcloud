package agentadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const maxRemoteAgentResponse = 2 << 20

type RemoteHTTPHarnessConfig struct {
	Kind         string
	Endpoint     string
	Token        string
	HTTPClient   *http.Client
	PollInterval time.Duration
	AllowHTTP    bool
}

// RemoteHTTPHarness implements the same Runtime contract for a Pi Agent
// service wrapper, an internal agent platform, or an external Agent SaaS.
// Remote transcripts and credentials remain outside business state.
type RemoteHTTPHarness struct {
	kind         string
	endpoint     *url.URL
	token        string
	client       *http.Client
	pollInterval time.Duration

	mu       sync.Mutex
	sessions map[string]*remoteHTTPSession
}

type remoteHTTPSession struct {
	status AgentSessionStatus
	stream *harnessEventStream
	cancel context.CancelFunc
	cursor string
}

type remoteSessionResponse struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	Cursor    string `json:"cursor,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type remoteEventsResponse struct {
	Events   []remoteEvent `json:"events"`
	Cursor   string        `json:"cursor"`
	Terminal bool          `json:"terminal"`
	State    string        `json:"state,omitempty"`
}

type remoteEvent struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

func NewRemoteHTTPHarness(config RemoteHTTPHarnessConfig) (*RemoteHTTPHarness, error) {
	kind := normalizeHarnessKind(config.Kind)
	if kind == "" {
		kind = "remote-http"
	}
	endpoint, err := url.Parse(strings.TrimSpace(config.Endpoint))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http")) {
		return nil, domain.Invalid("REMOTE_AGENT_ENDPOINT_INVALID", "远程 Agent Endpoint 必须是 HTTPS URL")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	return &RemoteHTTPHarness{kind: kind, endpoint: endpoint, token: strings.TrimSpace(config.Token), client: client, pollInterval: pollInterval, sessions: map[string]*remoteHTTPSession{}}, nil
}

func newRemoteHTTPHarnessFromEnv(kind, envPrefix string) AgentHarnessAdapter {
	endpoint := strings.TrimSpace(os.Getenv(envPrefix + "_ENDPOINT"))
	if endpoint == "" {
		return &unconfiguredRemoteHTTPHarness{kind: kind, envPrefix: envPrefix}
	}
	harness, err := NewRemoteHTTPHarness(RemoteHTTPHarnessConfig{Kind: kind, Endpoint: endpoint, Token: os.Getenv(envPrefix + "_TOKEN")})
	if err != nil {
		return &unconfiguredRemoteHTTPHarness{kind: kind, envPrefix: envPrefix, err: err}
	}
	return harness
}

type unconfiguredRemoteHTTPHarness struct {
	kind      string
	envPrefix string
	err       error
}

func (h *unconfiguredRemoteHTTPHarness) unavailable() error {
	if h.err != nil {
		return h.err
	}
	return domain.Policy("REMOTE_AGENT_UNAVAILABLE", "远程 Agent Harness 尚未配置", "配置 "+h.envPrefix+"_ENDPOINT 和凭据")
}
func (h *unconfiguredRemoteHTTPHarness) Detect(context.Context) (HarnessCapabilities, error) {
	return HarnessCapabilities{}, h.unavailable()
}
func (h *unconfiguredRemoteHTTPHarness) Start(context.Context, StartAgentRequest) (AgentSessionRef, EventStream, error) {
	return AgentSessionRef{}, nil, h.unavailable()
}
func (h *unconfiguredRemoteHTTPHarness) Resume(context.Context, ResumeAgentRequest) (EventStream, error) {
	return nil, h.unavailable()
}
func (h *unconfiguredRemoteHTTPHarness) Interrupt(context.Context, AgentSessionRef) error {
	return h.unavailable()
}
func (h *unconfiguredRemoteHTTPHarness) Inspect(context.Context, AgentSessionRef) (AgentSessionStatus, error) {
	return AgentSessionStatus{}, h.unavailable()
}

func (h *RemoteHTTPHarness) Detect(ctx context.Context) (HarnessCapabilities, error) {
	var caps HarnessCapabilities
	if err := h.request(ctx, http.MethodGet, "/v1/capabilities", nil, &caps); err != nil {
		return HarnessCapabilities{}, err
	}
	caps.Kind = normalizeHarnessKind(caps.Kind)
	if caps.Kind != h.kind || !caps.Events || !caps.StructuredOutput || caps.MaxParallelSessions < 1 {
		return HarnessCapabilities{}, domain.Policy("REMOTE_AGENT_CAPABILITY_INSUFFICIENT", "远程 Agent 缺少结构化事件、结构化输出或并发能力声明", "升级远程 Agent Adapter")
	}
	return caps, nil
}

func (h *RemoteHTTPHarness) Start(ctx context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.NodeRunID) == "" || strings.TrimSpace(request.AttemptID) == "" || strings.TrimSpace(request.ContextDigest) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "启动远程 Agent 需要 NodeRun、Attempt 和 context digest")
	}
	_, contract, schema, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return AgentSessionRef{}, nil, err
	}
	payload := map[string]any{
		"tenant_id": request.TenantID, "job_run_id": request.JobRunID, "node_run_id": request.NodeRunID, "attempt_id": request.AttemptID,
		"context_digest": request.ContextDigest, "prompt": agentPrompt(contract, skill), "continuation_prompt": request.Prompt,
		"output_schema": json.RawMessage(schema), "session_options": request.SessionOptions, "runtime_gateway": request.RuntimeGateway,
	}
	var response remoteSessionResponse
	if err := h.request(ctx, http.MethodPost, "/v1/sessions", payload, &response); err != nil {
		return AgentSessionRef{}, nil, err
	}
	return h.bindSession(ctx, request.TenantID, response)
}

func (h *RemoteHTTPHarness) Resume(ctx context.Context, request ResumeAgentRequest) (EventStream, error) {
	if normalizeHarnessKind(request.Session.HarnessKind) != h.kind || strings.TrimSpace(request.Session.SessionID) == "" {
		return nil, domain.Invalid("AGENT_SESSION_INVALID", "远程 Agent Resume 缺少有效会话引用")
	}
	if request.Session.TenantID != "" && request.TenantID != "" && request.Session.TenantID != request.TenantID {
		return nil, domain.Policy("AGENT_SESSION_TENANT_MISMATCH", "远程 Agent 会话不属于当前租户", "使用 RuntimeAttempt 固定的会话引用")
	}
	_, contract, schema, skill, err := loadWorkspace(request.Workspace)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"context_digest": request.ContextDigest, "prompt": agentPrompt(contract, skill), "continuation_prompt": request.Prompt, "output_schema": json.RawMessage(schema), "runtime_gateway": request.RuntimeGateway}
	var response remoteSessionResponse
	path := "/v1/sessions/" + url.PathEscape(request.Session.SessionID) + "/resume"
	if err := h.request(ctx, http.MethodPost, path, payload, &response); err != nil {
		return nil, err
	}
	if response.SessionID == "" {
		response.SessionID = request.Session.SessionID
	}
	if response.SessionID != request.Session.SessionID {
		return nil, domain.Conflict("REMOTE_AGENT_SESSION_MISMATCH", "远程 Agent Resume 返回了不同的会话 ID")
	}
	_, stream, err := h.bindSession(ctx, request.TenantID, response)
	return stream, err
}

func (h *RemoteHTTPHarness) Interrupt(ctx context.Context, ref AgentSessionRef) error {
	if err := h.validateSessionRef(ref); err != nil {
		return err
	}
	if err := h.request(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(ref.SessionID)+"/interrupt", map[string]any{}, nil); err != nil {
		return err
	}
	h.mu.Lock()
	if session := h.sessions[ref.SessionID]; session != nil {
		session.status.State = "interrupted"
		session.status.LastEventAt = time.Now().UTC()
		session.cancel()
	}
	h.mu.Unlock()
	return nil
}

func (h *RemoteHTTPHarness) Inspect(ctx context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	if err := h.validateSessionRef(ref); err != nil {
		return AgentSessionStatus{}, err
	}
	var response remoteSessionResponse
	if err := h.request(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(ref.SessionID), nil, &response); err != nil {
		return AgentSessionStatus{}, err
	}
	state := normalizeRemoteState(response.State)
	return AgentSessionStatus{Session: ref, State: state, ErrorCode: response.ErrorCode, LastEventAt: time.Now().UTC()}, nil
}

func (h *RemoteHTTPHarness) bindSession(parent context.Context, tenantID string, response remoteSessionResponse) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(response.SessionID) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("REMOTE_AGENT_SESSION_ID_MISSING", "远程 Agent 响应缺少 session_id")
	}
	ref := AgentSessionRef{TenantID: tenantID, HarnessKind: h.kind, SessionID: response.SessionID}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(parent))
	stream := newHarnessEventStream()
	session := &remoteHTTPSession{status: AgentSessionStatus{Session: ref, State: normalizeRemoteState(response.State), LastEventAt: time.Now().UTC()}, stream: stream, cancel: cancel, cursor: response.Cursor}
	h.mu.Lock()
	if existing := h.sessions[ref.SessionID]; existing != nil {
		existing.cancel()
	}
	h.sessions[ref.SessionID] = session
	h.mu.Unlock()
	stream.emit(AgentEvent{Type: "session.started", Session: ref, OccurredAt: session.status.LastEventAt})
	go h.poll(runCtx, session)
	return ref, stream, nil
}

func (h *RemoteHTTPHarness) poll(ctx context.Context, session *remoteHTTPSession) {
	defer session.stream.Close()
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			path := "/v1/sessions/" + url.PathEscape(session.status.Session.SessionID) + "/events?after=" + url.QueryEscape(session.cursor)
			var response remoteEventsResponse
			if err := h.request(ctx, http.MethodGet, path, nil, &response); err != nil {
				h.setUnknown(session, "REMOTE_AGENT_EVENT_POLL_FAILED")
				return
			}
			if response.Cursor != "" {
				session.cursor = response.Cursor
			}
			for _, value := range response.Events {
				occurredAt := value.OccurredAt
				if occurredAt.IsZero() {
					occurredAt = time.Now().UTC()
				}
				eventType := safeRemoteEventType(value.Type)
				session.stream.emit(AgentEvent{Type: eventType, Session: session.status.Session, Data: value.Data, ErrorCode: value.ErrorCode, OccurredAt: occurredAt})
				session.status.LastEventAt = occurredAt
			}
			if response.Terminal {
				session.status.State = normalizeRemoteState(response.State)
				return
			}
		}
	}
}

func (h *RemoteHTTPHarness) setUnknown(session *remoteHTTPSession, code string) {
	h.mu.Lock()
	session.status.State = "unknown"
	session.status.ErrorCode = code
	session.status.LastEventAt = time.Now().UTC()
	h.mu.Unlock()
	session.stream.emit(AgentEvent{Type: "session.unknown", Session: session.status.Session, ErrorCode: code, OccurredAt: session.status.LastEventAt})
}

func (h *RemoteHTTPHarness) validateSessionRef(ref AgentSessionRef) error {
	if normalizeHarnessKind(ref.HarnessKind) != h.kind || strings.TrimSpace(ref.SessionID) == "" {
		return domain.Invalid("AGENT_SESSION_INVALID", "远程 Agent 会话引用无效")
	}
	return nil
}

func (h *RemoteHTTPHarness) request(ctx context.Context, method, path string, input any, output any) error {
	endpoint := *h.endpoint
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + strings.SplitN(path, "?", 2)[0]
	if index := strings.IndexByte(path, '?'); index >= 0 {
		endpoint.RawQuery = path[index+1:]
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if h.token != "" {
		request.Header.Set("Authorization", "Bearer "+h.token)
	}
	response, err := h.client.Do(request)
	if err != nil {
		return fmt.Errorf("remote agent request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteAgentResponse+1))
	if err != nil {
		return err
	}
	if len(data) > maxRemoteAgentResponse {
		return errors.New("remote agent response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("remote agent returned HTTP %d", response.StatusCode)
	}
	if output != nil {
		if len(bytes.TrimSpace(data)) == 0 || json.Unmarshal(data, output) != nil {
			return errors.New("remote agent response is not valid JSON")
		}
	}
	return nil
}

func normalizeRemoteState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "queued", "active", "completed", "failed", "interrupted":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func safeRemoteEventType(value string) string {
	switch strings.TrimSpace(value) {
	case "session.started", "session.resumed", "session.progress", "usage.reported", "result.completed", "session.failed", "session.interrupted":
		return strings.TrimSpace(value)
	default:
		return "session.progress"
	}
}

var _ AgentHarnessAdapter = (*RemoteHTTPHarness)(nil)
