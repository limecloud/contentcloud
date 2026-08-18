package runtime_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/limecloud/contentcloud/internal/runtime"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/persistence/memory"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
)

func activeAgentSaaSDispatch(t *testing.T) (*Service, *memory.Store, StartResult, DispatchHandle, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	repo := memory.New()
	service := New(repo, func() time.Time { return now })
	started, err := service.Start(t.Context(), testStartInput("agent-callback-task", "agent-callback-job"))
	if err != nil {
		t.Fatal(err)
	}
	input := dispatchInput(started.Job.ID)
	input.HarnessKind = "agent-saas"
	handle, err := service.PrepareRemoteDispatch(t.Context(), input, agentadapter.HarnessCapabilities{Kind: "agent-saas", Events: true, Resume: true, MCPHTTP: true, StructuredOutput: true, SandboxProfile: "remote", MaxParallelSessions: 16})
	if err != nil {
		t.Fatal(err)
	}
	handle, err = service.ActivateDispatch(t.Context(), handle, agentadapter.AgentSessionRef{TenantID: "tenant-1", HarnessKind: "agent-saas", SessionID: "saas-session-1"})
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, started, handle, now
}

func TestAgentCallbackRecordsProgressAndDeduplicatesReplay(t *testing.T) {
	service, repo, started, handle, now := activeAgentSaaSDispatch(t)
	input := AgentCallbackInput{
		TenantID: "tenant-1", HarnessKind: "agent-saas", MessageID: "progress-1",
		AttemptID: handle.Attempt.ID, SessionID: "saas-session-1", EventType: "session.progress",
		Data: json.RawMessage(`{"percent":50,"api_key":"must-not-persist"}`), OccurredAt: now, ReceivedAt: now,
	}
	first, err := service.ReceiveAgentCallback(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Applied || first.Replayed || first.Message.State != ProviderInboxApplied || first.Attempt.State != RuntimeAttemptRunning {
		t.Fatalf("progress callback did not converge: %#v", first)
	}
	data, ok := first.Message.SafePayload["data"].(map[string]any)
	if !ok || data["api_key"] != "[redacted]" {
		t.Fatalf("agent callback persisted unsafe data: %#v", first.Message.SafePayload)
	}
	events, err := repo.JobEvents(t.Context(), "tenant-1", started.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeReplay := len(events)
	replayed, err := service.ReceiveAgentCallback(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Applied || !replayed.Replayed || replayed.Message.ID != first.Message.ID {
		t.Fatalf("agent callback replay was not idempotent: %#v", replayed)
	}
	events, _ = repo.JobEvents(t.Context(), "tenant-1", started.Job.ID, 0)
	if len(events) != beforeReplay {
		t.Fatalf("agent callback replay appended events: before=%d after=%d", beforeReplay, len(events))
	}
	conflict := input
	conflict.Data = json.RawMessage(`{"percent":75}`)
	if _, err := service.ReceiveAgentCallback(t.Context(), conflict); !hasDomainCode(err, "PROVIDER_INBOX_DIGEST_CONFLICT") {
		t.Fatalf("same message id with changed body must conflict, got %v", err)
	}
}

func TestAgentCallbackResumesAfterDurableInboxCrashWindow(t *testing.T) {
	service, repo, started, handle, now := activeAgentSaaSDispatch(t)
	input := AgentCallbackInput{
		TenantID: "tenant-1", HarnessKind: "agent-saas", MessageID: "result-1",
		AttemptID: handle.Attempt.ID, SessionID: "saas-session-1", EventType: "result.completed",
		Data:       json.RawMessage(`{"output_refs":["artifact:video-1"],"output_digest":"sha256:video","safe_summary":{"kind":"video"},"used_cost_minor":20}`),
		OccurredAt: now, ReceivedAt: now, ReceivedDigest: "sha256:" + strings.Repeat("a", 64),
	}
	message := ProviderInboxMessage{
		ID: idgen.New(), TenantID: input.TenantID, JobRunID: handle.Attempt.JobRunID,
		ProviderID: "agent-harness:" + input.HarnessKind, MessageID: input.MessageID,
		ReceivedDigest: input.ReceivedDigest, ExternalID: input.SessionID, ProviderState: input.EventType,
		ResponseDigest: "sha256:" + strings.Repeat("b", 64), Currency: "CNY",
		SafePayload: map[string]any{"attempt_id": input.AttemptID}, State: ProviderInboxReceived,
		ReceivedAt: input.ReceivedAt, Version: 1, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt,
	}
	stored, err := repo.ReceiveAgentInboxCommand(t.Context(), message, JobEvent{
		ID: idgen.New(), TenantID: "tenant-1", JobRunID: started.Job.ID, NodeKey: handle.Node.NodeKey,
		Type: "agent.inbox.received", ActorType: "harness", ActorID: "agent-saas",
		IdempotencyKey: "simulate-crash-after-inbox", Payload: map[string]any{"attempt_id": handle.Attempt.ID}, OccurredAt: now,
	})
	if err != nil || stored.State != ProviderInboxReceived {
		t.Fatalf("failed to create crash-window inbox fact: %#v err=%v", stored, err)
	}

	resumed, err := service.ReceiveAgentCallback(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Replayed || !resumed.Applied || resumed.Message.ID != stored.ID || resumed.Message.State != ProviderInboxApplied || resumed.Attempt.State != RuntimeAttemptSucceeded {
		t.Fatalf("durable inbox replay did not finish attempt: %#v", resumed)
	}
	if resumed.Attempt.ResultDigest == "" || resumed.Node.OutputDigest != "sha256:video" || resumed.Attempt.OutputRefs[0] != "artifact:video-1" {
		t.Fatalf("terminal callback output was not preserved: %#v", resumed)
	}
	second, err := service.ReceiveAgentCallback(t.Context(), input)
	if err != nil || !second.Replayed || second.Applied || second.Attempt.ResultDigest != resumed.Attempt.ResultDigest {
		t.Fatalf("terminal callback replay was not idempotent: %#v err=%v", second, err)
	}
	inbox, err := repo.ProviderInboxMessages(t.Context(), "tenant-1", started.Job.ID)
	if err != nil || len(inbox) != 1 || inbox[0].State != ProviderInboxApplied {
		t.Fatalf("unexpected durable agent inbox: %#v err=%v", inbox, err)
	}
}

func TestAgentCallbackRejectsHarnessAndSessionMismatch(t *testing.T) {
	service, _, _, handle, now := activeAgentSaaSDispatch(t)
	base := AgentCallbackInput{TenantID: "tenant-1", HarnessKind: "agent-saas", MessageID: "mismatch-1", AttemptID: handle.Attempt.ID, SessionID: "wrong", EventType: "session.progress", Data: json.RawMessage(`{}`), OccurredAt: now, ReceivedAt: now}
	if _, err := service.ReceiveAgentCallback(t.Context(), base); !hasDomainCode(err, "HARNESS_SESSION_MISMATCH") {
		t.Fatalf("wrong session was accepted: %v", err)
	}
	base.SessionID = "saas-session-1"
	base.HarnessKind = "codex"
	if _, err := service.ReceiveAgentCallback(t.Context(), base); !hasDomainCode(err, "AGENT_CALLBACK_HARNESS_MISMATCH") {
		t.Fatalf("wrong harness was accepted: %v", err)
	}
}
