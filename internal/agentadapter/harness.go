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
	TenantID       string
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
	TenantID      string
	Session       AgentSessionRef
	Workspace     string
	Prompt        string
	OutputSchema  json.RawMessage
	ContextDigest string
}

type AgentSessionRef struct {
	TenantID    string `json:"tenant_id,omitempty"`
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
		return AgentSessionRef{TenantID: request.TenantID, HarnessKind: "fake", SessionID: domain.NewID()}, nil, nil
	}
	ref, stream, err := h.newSession(request.TenantID, "started")
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

func (h *FakeHarness) newSession(tenantID, eventType string) (AgentSessionRef, EventStream, error) {
	ref := AgentSessionRef{TenantID: tenantID, HarnessKind: "fake", SessionID: domain.NewID()}
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
