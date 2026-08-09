package agentadapter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type testAgentSessionStore struct {
	mu       sync.Mutex
	sessions map[string]AgentSessionRecord
	events   map[string][]AgentSessionEvent
}

func newTestAgentSessionStore() *testAgentSessionStore {
	return &testAgentSessionStore{sessions: map[string]AgentSessionRecord{}, events: map[string][]AgentSessionEvent{}}
}

func agentSessionTestKey(tenantID string, ref AgentSessionRef) string {
	return tenantID + ":" + ref.HarnessKind + ":" + ref.SessionID
}

func (s *testAgentSessionStore) SaveAgentSession(_ context.Context, value AgentSessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := agentSessionTestKey(value.TenantID, value.Session)
	if current, ok := s.sessions[key]; ok {
		value.Version = current.Version + 1
	}
	s.sessions[key] = value
	return nil
}

func (s *testAgentSessionStore) AgentSession(_ context.Context, tenantID string, ref AgentSessionRef) (AgentSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.sessions[agentSessionTestKey(tenantID, ref)]
	if !ok {
		return value, domain.NotFound("test session")
	}
	return value, nil
}

func (s *testAgentSessionStore) AppendAgentEvent(_ context.Context, value AgentSessionEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := agentSessionTestKey(value.TenantID, value.Session)
	s.events[key] = append(s.events[key], value)
	return nil
}

func (s *testAgentSessionStore) AgentEvents(_ context.Context, tenantID string, ref AgentSessionRef, after int64) ([]AgentSessionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []AgentSessionEvent{}
	for _, value := range s.events[agentSessionTestKey(tenantID, ref)] {
		if value.Sequence > after {
			result = append(result, value)
		}
	}
	return result, nil
}

func TestDurableHarnessResumesFromPersistedSession(t *testing.T) {
	root := t.TempDir()
	harness, err := NewDurableHarness(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, stream, err := harness.Start(t.Context(), StartAgentRequest{NodeRunID: "node-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.Complete(ref, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	other, err := NewDurableHarness(root)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := other.Resume(t.Context(), ResumeAgentRequest{Session: ref})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	seenResult := false
	deadline := time.After(time.Second)
	for !seenResult {
		select {
		case event, ok := <-resumed.Events():
			if !ok {
				t.Fatal("durable stream closed before replaying result")
			}
			if event.Type == "result.completed" {
				seenResult = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for persisted result")
		}
	}
}

func TestDurableHarnessResumesAcrossHostsFromSessionStore(t *testing.T) {
	store := newTestAgentSessionStore()
	first, err := NewDurableHarnessWithSessionStore(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	ref, stream, err := first.Start(t.Context(), StartAgentRequest{TenantID: "tenant-1", NodeRunID: "node-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Complete(ref, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()

	second, err := NewDurableHarnessWithSessionStore(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := second.Resume(t.Context(), ResumeAgentRequest{TenantID: "tenant-1", Session: ref})
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	seenStarted, seenResult := false, false
	for event := range resumed.Events() {
		seenStarted = seenStarted || event.Type == "session.started"
		seenResult = seenResult || event.Type == "result.completed"
	}
	if !seenStarted || !seenResult {
		t.Fatalf("mirrored session events were not replayed across hosts: started=%v result=%v", seenStarted, seenResult)
	}
}
