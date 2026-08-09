package agentadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type durableSessionState struct {
	TenantID    string          `json:"tenant_id,omitempty"`
	Session     AgentSessionRef `json:"session"`
	State       string          `json:"state"`
	LastEventAt time.Time       `json:"last_event_at"`
	ErrorCode   string          `json:"error_code,omitempty"`
}

type durableEventStream struct {
	mu     sync.Mutex
	events chan AgentEvent
	closed bool
}

func newDurableEventStream() *durableEventStream {
	return &durableEventStream{events: make(chan AgentEvent, 32)}
}
func (s *durableEventStream) Events() <-chan AgentEvent { return s.events }
func (s *durableEventStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.events)
	}
	return nil
}
func (s *durableEventStream) emit(event AgentEvent) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.events <- event
	return true
}

// DurableHarness persists session state and structured events as an append-only
// local spool. A new process can construct another instance with the same root
// and Resume using the persisted opaque session reference.
type DurableHarness struct {
	root    string
	store   SessionStore
	mu      sync.Mutex
	streams map[string]*durableEventStream
}

func NewDurableHarness(root string) (*DurableHarness, error) {
	return newDurableHarness(root, nil)
}

func NewDurableHarnessWithSessionStore(root string, store SessionStore) (*DurableHarness, error) {
	return newDurableHarness(root, store)
}

func newDurableHarness(root string, store SessionStore) (*DurableHarness, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, domain.Invalid("DURABLE_HARNESS_ROOT_INVALID", "DurableHarness 缺少持久化目录")
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions"), 0o700); err != nil {
		return nil, err
	}
	return &DurableHarness{root: root, store: store, streams: map[string]*durableEventStream{}}, nil
}

func (h *DurableHarness) Detect(context.Context) (HarnessCapabilities, error) {
	return HarnessCapabilities{Kind: "durable", Events: true, Resume: true, Fork: false, StructuredOutput: true, SandboxProfile: "durable_spool", MaxParallelSessions: 64, TranscriptExport: false}, nil
}

func (h *DurableHarness) sessionPath(id string) string {
	return filepath.Join(h.root, "sessions", id+".json")
}
func (h *DurableHarness) eventPath(id string) string {
	return filepath.Join(h.root, "sessions", id+".events.jsonl")
}

func (h *DurableHarness) saveSession(state durableSessionState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := h.sessionPath(state.Session.SessionID) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, h.sessionPath(state.Session.SessionID)); err != nil {
		return err
	}
	if h.store != nil && state.TenantID != "" {
		now := state.LastEventAt
		_ = h.store.SaveAgentSession(context.Background(), AgentSessionRecord{TenantID: state.TenantID, Session: state.Session, State: state.State, LastEventAt: state.LastEventAt, ErrorCode: state.ErrorCode, Version: 1, CreatedAt: now, UpdatedAt: now})
	}
	return nil
}

func (h *DurableHarness) loadSession(ref AgentSessionRef) (durableSessionState, error) {
	body, err := os.ReadFile(h.sessionPath(ref.SessionID))
	if errors.Is(err, os.ErrNotExist) {
		if h.store != nil {
			record, storeErr := h.store.AgentSession(context.Background(), ref.TenantID, ref)
			if storeErr == nil {
				return durableSessionState{TenantID: record.TenantID, Session: record.Session, State: record.State, LastEventAt: record.LastEventAt, ErrorCode: record.ErrorCode}, nil
			}
		}
		return durableSessionState{}, domain.NotFound("DurableHarness 会话")
	}
	if err != nil {
		return durableSessionState{}, err
	}
	var state durableSessionState
	if err := json.Unmarshal(body, &state); err != nil {
		return durableSessionState{}, err
	}
	if state.Session.HarnessKind != ref.HarnessKind {
		return durableSessionState{}, domain.Conflict("DURABLE_SESSION_KIND_MISMATCH", "持久化会话适配器类型不匹配")
	}
	return state, nil
}

func (h *DurableHarness) appendEvent(event AgentEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(h.eventPath(event.Session.SessionID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(body, '\n'))
	if err != nil || h.store == nil {
		return err
	}
	sequence := int64(1)
	if events, readErr := h.readEvents(event.Session); readErr == nil {
		sequence = int64(len(events))
	}
	digest, digestErr := domain.CanonicalHash(event)
	if digestErr != nil {
		return digestErr
	}
	_ = h.store.AppendAgentEvent(context.Background(), AgentSessionEvent{TenantID: event.Session.TenantID, Session: event.Session, Sequence: sequence, Event: event, Digest: "sha256:" + digest, CreatedAt: event.OccurredAt})
	return nil
}

func (h *DurableHarness) readEvents(ref AgentSessionRef) ([]AgentEvent, error) {
	file, err := os.Open(h.eventPath(ref.SessionID))
	if errors.Is(err, os.ErrNotExist) {
		if h.store != nil {
			mirrored, storeErr := h.store.AgentEvents(context.Background(), ref.TenantID, ref, 0)
			if storeErr == nil {
				result := make([]AgentEvent, 0, len(mirrored))
				for _, value := range mirrored {
					result = append(result, value.Event)
				}
				return result, nil
			}
		}
		return []AgentEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := []AgentEvent{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event AgentEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, scanner.Err()
}

func (h *DurableHarness) stream(ref AgentSessionRef) (*durableEventStream, error) {
	stream := newDurableEventStream()
	h.mu.Lock()
	h.streams[ref.SessionID] = stream
	h.mu.Unlock()
	go func() {
		defer stream.Close()
		sent := 0
		for {
			events, err := h.readEvents(ref)
			if err != nil {
				stream.emit(AgentEvent{Type: "session.failed", Session: ref, ErrorCode: "DURABLE_EVENT_READ_FAILED", OccurredAt: time.Now().UTC()})
				return
			}
			for sent < len(events) {
				if !stream.emit(events[sent]) {
					return
				}
				sent++
			}
			state, err := h.loadSession(ref)
			if err != nil || state.State == "completed" || state.State == "interrupted" || state.State == "failed" {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	return stream, nil
}

func (h *DurableHarness) Start(ctx context.Context, request StartAgentRequest) (AgentSessionRef, EventStream, error) {
	if strings.TrimSpace(request.NodeRunID) == "" || strings.TrimSpace(request.AttemptID) == "" {
		return AgentSessionRef{}, nil, domain.Invalid("AGENT_START_INVALID", "启动智能体需要 NodeRun 和 Attempt 引用")
	}
	ref := AgentSessionRef{TenantID: request.TenantID, HarnessKind: "durable", SessionID: domain.NewID()}
	now := time.Now().UTC()
	if err := h.saveSession(durableSessionState{TenantID: request.TenantID, Session: ref, State: "active", LastEventAt: now}); err != nil {
		return AgentSessionRef{}, nil, err
	}
	if err := h.appendEvent(AgentEvent{Type: "session.started", Session: ref, OccurredAt: now}); err != nil {
		return AgentSessionRef{}, nil, err
	}
	stream, err := h.stream(ref)
	if err != nil {
		return AgentSessionRef{}, nil, err
	}
	return ref, stream, nil
}

func (h *DurableHarness) Resume(_ context.Context, request ResumeAgentRequest) (EventStream, error) {
	if request.Session.TenantID == "" {
		request.Session.TenantID = request.TenantID
	}
	if _, err := h.loadSession(request.Session); err != nil {
		return nil, err
	}
	return h.stream(request.Session)
}

func (h *DurableHarness) Interrupt(_ context.Context, ref AgentSessionRef) error {
	state, err := h.loadSession(ref)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	state.State, state.LastEventAt = "interrupted", now
	if err := h.saveSession(state); err != nil {
		return err
	}
	return h.appendEvent(AgentEvent{Type: "session.interrupted", Session: ref, OccurredAt: now})
}

func (h *DurableHarness) Inspect(_ context.Context, ref AgentSessionRef) (AgentSessionStatus, error) {
	state, err := h.loadSession(ref)
	if err != nil {
		return AgentSessionStatus{}, err
	}
	return AgentSessionStatus{Session: state.Session, State: state.State, LastEventAt: state.LastEventAt, ErrorCode: state.ErrorCode}, nil
}

func (h *DurableHarness) Complete(ref AgentSessionRef, result any) error {
	state, err := h.loadSession(ref)
	if err != nil {
		return err
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	state.State, state.LastEventAt = "completed", now
	if err := h.saveSession(state); err != nil {
		return err
	}
	return h.appendEvent(AgentEvent{Type: "result.completed", Session: ref, Data: body, OccurredAt: now})
}
