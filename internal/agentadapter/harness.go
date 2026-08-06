package agentadapter

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// HarnessCapabilities is the capability handshake used by Runtime scheduling.
// A binary name alone is never sufficient to assume resume, MCP or event
// support.
type HarnessCapabilities struct {
	Kind                string `json:"kind"`
	Events              bool   `json:"events"`
	Resume              bool   `json:"resume"`
	Fork                bool   `json:"fork"`
	MCPStdio            bool   `json:"mcp_stdio"`
	MCPHTTP             bool   `json:"mcp_http"`
	StructuredOutput    bool   `json:"structured_output"`
	SandboxProfile      string `json:"sandbox_profile"`
	MaxParallelSessions int    `json:"max_parallel_sessions"`
	TranscriptExport    bool   `json:"transcript_export"`
}

type StartAgentRequest struct {
	JobRunID       string
	NodeRunID      string
	AttemptID      string
	Workspace      string
	Prompt         string
	OutputSchema   json.RawMessage
	ContextDigest  string
	SessionOptions map[string]string
}

type ResumeAgentRequest struct {
	Session       AgentSessionRef
	Workspace     string
	Prompt        string
	OutputSchema  json.RawMessage
	ContextDigest string
}

type AgentSessionRef struct {
	HarnessKind string `json:"harness_kind"`
	SessionID   string `json:"session_id"`
}

type AgentEvent struct {
	Type       string          `json:"type"`
	Session    AgentSessionRef `json:"session"`
	Data       json.RawMessage `json:"data,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// EventStream deliberately exposes only structured events. Raw host
// transcripts remain an adapter concern and are never part of Runtime state.
type EventStream interface {
	Events() <-chan AgentEvent
	Close() error
}

type AgentSessionStatus struct {
	Session     AgentSessionRef `json:"session"`
	State       string          `json:"state"`
	LastEventAt time.Time       `json:"last_event_at,omitempty"`
	ErrorCode   string          `json:"error_code,omitempty"`
}

type AgentHarnessAdapter interface {
	Detect(context.Context) (HarnessCapabilities, error)
	Start(context.Context, StartAgentRequest) (AgentSessionRef, EventStream, error)
	Resume(context.Context, ResumeAgentRequest) (EventStream, error)
	Interrupt(context.Context, AgentSessionRef) error
	Inspect(context.Context, AgentSessionRef) (AgentSessionStatus, error)
}

type harnessEventStream struct {
	mu     sync.Mutex
	events chan AgentEvent
	closed bool
}

func newHarnessEventStream() *harnessEventStream {
	return &harnessEventStream{events: make(chan AgentEvent, 16)}
}

func (s *harnessEventStream) Events() <-chan AgentEvent { return s.events }

func (s *harnessEventStream) emit(event AgentEvent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.events <- event
	return true
}

func (s *harnessEventStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}

type fakeSession struct {
	status AgentSessionStatus
	stream *harnessEventStream
}

// FakeHarness is deterministic by design. It is the default CI harness and
// can be driven by tests without invoking a paid model or a local process.
type FakeHarness struct {
	mu       sync.Mutex
	sessions map[string]*fakeSession
	scripts  []FakeHarnessScript
}

func NewFakeHarness() *FakeHarness {
	return &FakeHarness{sessions: map[string]*fakeSession{}, scripts: []FakeHarnessScript{}}
}

// FakeHarnessScript lets tests deterministically drive event ordering and
// failure modes without invoking a local process or paid model.
type FakeHarnessScript struct {
	StartError    error
	MissingStream bool
	Events        []FakeHarnessScriptEvent
}

type FakeHarnessScriptEvent struct {
	Type      string
	Data      json.RawMessage
	ErrorCode string
	Delay     time.Duration
}

func (h *FakeHarness) QueueScript(script FakeHarnessScript) {
	h.mu.Lock()
	defer h.mu.Unlock()
	copyScript := FakeHarnessScript{StartError: script.StartError, MissingStream: script.MissingStream, Events: append([]FakeHarnessScriptEvent(nil), script.Events...)}
	h.scripts = append(h.scripts, copyScript)
}

func (h *FakeHarness) Detect(context.Context) (HarnessCapabilities, error) {
	return HarnessCapabilities{Kind: "fake", Events: true, Resume: true, Fork: true, MCPStdio: true, StructuredOutput: true, SandboxProfile: "fake", MaxParallelSessions: 128, TranscriptExport: false}, nil
}

func (h *FakeHarness) Start(_ context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.NodeRunID) == "" || strings.TrimSpace(request.AttemptID) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "启动智能体需要 NodeRun 和 Attempt 引用")
	}
	script, scripted := h.nextScript()
	if scripted && script.StartError != nil {
		return AgentSessionRef{}, nil, script.StartError
	}
	if scripted && script.MissingStream {
		return AgentSessionRef{HarnessKind: "fake", SessionID: domain.NewID()}, nil, nil
	}
	ref, stream, err := h.newSession("started")
	if err != nil || !scripted {
		return ref, stream, err
	}
	go h.runScript(ref, script)
	return ref, stream, nil
}

func (h *FakeHarness) Resume(_ context.Context, request ResumeAgentRequest) (EventStream, error) {
	h.mu.Lock()
	session, ok := h.sessions[request.Session.SessionID]
	h.mu.Unlock()
	if !ok {
		return nil, domain.NotFound("智能体会话")
	}
	session.stream.emit(AgentEvent{Type: "session.resumed", Session: request.Session, OccurredAt: time.Now().UTC()})
	return session.stream, nil
}

func (h *FakeHarness) Interrupt(_ context.Context, ref AgentSessionRef) error {
	h.mu.Lock()
	session, ok := h.sessions[ref.SessionID]
	if ok {
		session.status.State = "interrupted"
		session.status.LastEventAt = time.Now().UTC()
	}
	h.mu.Unlock()
	if !ok {
		return domain.NotFound("智能体会话")
	}
	session.stream.emit(AgentEvent{Type: "session.interrupted", Session: ref, OccurredAt: time.Now().UTC()})
	return nil
}

func (h *FakeHarness) Inspect(_ context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[ref.SessionID]
	if !ok {
		return AgentSessionStatus{}, domain.NotFound("智能体会话")
	}
	return session.status, nil
}

// Complete closes a fake session with a structured result, allowing black-box
// tests to model a normal worker report.
func (h *FakeHarness) Complete(ref AgentSessionRef, result any) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	h.mu.Lock()
	session, ok := h.sessions[ref.SessionID]
	if ok {
		session.status.State = "completed"
		session.status.LastEventAt = time.Now().UTC()
	}
	h.mu.Unlock()
	if !ok {
		return domain.NotFound("智能体会话")
	}
	session.stream.emit(AgentEvent{Type: "result.completed", Session: ref, Data: body, OccurredAt: time.Now().UTC()})
	return session.stream.Close()
}

func (h *FakeHarness) newSession(eventType string) (AgentSessionRef, EventStream, error) {
	ref := AgentSessionRef{HarnessKind: "fake", SessionID: domain.NewID()}
	stream := newHarnessEventStream()
	h.mu.Lock()
	h.sessions[ref.SessionID] = &fakeSession{status: AgentSessionStatus{Session: ref, State: "active", LastEventAt: time.Now().UTC()}, stream: stream}
	h.mu.Unlock()
	stream.emit(AgentEvent{Type: "session." + eventType, Session: ref, OccurredAt: time.Now().UTC()})
	return ref, stream, nil
}

func (h *FakeHarness) nextScript() (FakeHarnessScript, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.scripts) == 0 {
		return FakeHarnessScript{}, false
	}
	script := h.scripts[0]
	h.scripts = h.scripts[1:]
	return script, true
}

func (h *FakeHarness) runScript(ref AgentSessionRef, script FakeHarnessScript) {
	for _, scripted := range script.Events {
		if scripted.Delay > 0 {
			time.Sleep(scripted.Delay)
		}
		h.mu.Lock()
		session, ok := h.sessions[ref.SessionID]
		if ok {
			if session.status.State == "interrupted" {
				h.mu.Unlock()
				_ = session.stream.Close()
				return
			}
			session.status.LastEventAt = time.Now().UTC()
			switch scripted.Type {
			case "result.completed":
				session.status.State = "completed"
			case "session.failed":
				session.status.State = "failed"
				session.status.ErrorCode = scripted.ErrorCode
			}
		}
		h.mu.Unlock()
		if !ok || !session.stream.emit(AgentEvent{Type: scripted.Type, Session: ref, Data: append(json.RawMessage(nil), scripted.Data...), ErrorCode: scripted.ErrorCode, OccurredAt: time.Now().UTC()}) {
			return
		}
	}
	h.mu.Lock()
	session := h.sessions[ref.SessionID]
	h.mu.Unlock()
	if session != nil {
		_ = session.stream.Close()
	}
}

type cliHarness struct {
	adapter  Adapter
	kind     string
	mu       sync.Mutex
	sessions map[string]*cliSession
}

type cliSession struct {
	status AgentSessionStatus
	stream *harnessEventStream
	cancel context.CancelFunc
}

func newCLIHarness(adapter Adapter) AgentHarnessAdapter {
	return &cliHarness{adapter: adapter, kind: adapter.Kind(), sessions: map[string]*cliSession{}}
}

func (h *cliHarness) Detect(ctx context.Context) (HarnessCapabilities, error) {
	if err := h.adapter.Detect(); err != nil {
		return HarnessCapabilities{}, err
	}
	return HarnessCapabilities{Kind: h.kind, Events: true, Resume: false, Fork: false, MCPStdio: false, StructuredOutput: true, SandboxProfile: "legacy_cli", MaxParallelSessions: 1, TranscriptExport: false}, nil
}

func (h *cliHarness) Start(ctx context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.Workspace) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "CLI 智能体需要隔离工作区")
	}
	runCtx, cancel := context.WithCancel(ctx)
	ref := AgentSessionRef{HarnessKind: h.kind, SessionID: domain.NewID()}
	stream := newHarnessEventStream()
	session := &cliSession{status: AgentSessionStatus{Session: ref, State: "active", LastEventAt: time.Now().UTC()}, stream: stream, cancel: cancel}
	h.mu.Lock()
	h.sessions[ref.SessionID] = session
	h.mu.Unlock()
	stream.emit(AgentEvent{Type: "session.started", Session: ref, OccurredAt: time.Now().UTC()})
	go func() {
		result, err := h.adapter.Run(runCtx, request.Workspace)
		h.mu.Lock()
		if err != nil {
			session.status.State = "failed"
			session.status.ErrorCode = "AGENT_PROCESS_FAILED"
		} else {
			session.status.State = "completed"
		}
		session.status.LastEventAt = time.Now().UTC()
		h.mu.Unlock()
		if err != nil {
			stream.emit(AgentEvent{Type: "session.failed", Session: ref, ErrorCode: "AGENT_PROCESS_FAILED", OccurredAt: time.Now().UTC()})
		} else {
			stream.emit(AgentEvent{Type: "result.completed", Session: ref, Data: result, OccurredAt: time.Now().UTC()})
		}
		_ = stream.Close()
	}()
	return ref, stream, nil
}

func (h *cliHarness) Resume(_ context.Context, _ ResumeAgentRequest) (EventStream, error) {
	return nil, domain.Policy("AGENT_HARNESS_RESUME_UNSUPPORTED", h.kind+" 当前 CLI 模式不支持会话恢复", "等待 App Server/SDK 适配器，或按 ContextView 创建新会话")
}

func (h *cliHarness) Interrupt(_ context.Context, ref AgentSessionRef) error {
	h.mu.Lock()
	session, ok := h.sessions[ref.SessionID]
	h.mu.Unlock()
	if !ok {
		return domain.NotFound("智能体会话")
	}
	session.cancel()
	return nil
}

func (h *cliHarness) Inspect(_ context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[ref.SessionID]
	if !ok {
		return AgentSessionStatus{}, domain.NotFound("智能体会话")
	}
	return session.status, nil
}

// SelectHarness is a compatibility factory for callers outside Runtime. New
// Runtime scheduling must use HarnessRegistry so session state survives across
// Resolve and Resume calls.
//
// SelectHarness is separate from Select: legacy one-shot adapters can execute
// a node, while this selector exposes their actual resume/event capabilities to
// Runtime. Fake is explicit and never selected by auto mode.
func SelectHarness(kind string) (AgentHarnessAdapter, error) {
	normalized := normalizeHarnessKind(kind)
	switch normalized {
	case "fake":
		return NewFakeHarness(), nil
	case "codex":
		return newCLIHarness(Codex{}), nil
	case "claude":
		return newCLIHarness(Claude{}), nil
	default:
		return nil, domain.Invalid("AGENT_HARNESS_INVALID", "未知的智能体执行适配器")
	}
}
