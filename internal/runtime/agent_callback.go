package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/domain"
)

const agentHarnessProviderPrefix = "agent-harness:"

type AgentCallbackInput struct {
	TenantID       string
	HarnessKind    string
	MessageID      string
	AttemptID      string
	SessionID      string
	EventType      string
	Data           json.RawMessage
	ErrorCode      string
	OccurredAt     time.Time
	ReceivedAt     time.Time
	ReceivedDigest string
}

type AgentCallbackResult struct {
	Message  domain.ProviderInboxMessage `json:"message"`
	Attempt  domain.RuntimeAttempt       `json:"attempt"`
	Node     domain.NodeRun              `json:"node"`
	Job      domain.JobRun               `json:"job,omitempty"`
	Applied  bool                        `json:"applied"`
	Replayed bool                        `json:"replayed"`
}

// ReceiveAgentCallback durably records a remote harness event before applying
// it to the authoritative RuntimeAttempt. Replaying the same message resumes
// processing when a process stopped between inbox receipt and completion.
func (s *Service) ReceiveAgentCallback(ctx context.Context, input AgentCallbackInput) (AgentCallbackResult, error) {
	if s == nil || s.repo == nil {
		return AgentCallbackResult{}, domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.HarnessKind = strings.ToLower(strings.TrimSpace(input.HarnessKind))
	input.MessageID = strings.TrimSpace(input.MessageID)
	input.AttemptID = strings.TrimSpace(input.AttemptID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.EventType = safeEventType(input.EventType)
	input.ErrorCode = safeErrorCode(input.ErrorCode, "")
	if input.TenantID == "" || input.HarnessKind == "" || input.MessageID == "" || input.AttemptID == "" || input.SessionID == "" || input.EventType == "unknown" {
		return AgentCallbackResult{}, domain.Invalid("AGENT_CALLBACK_INVALID", "Agent 回调缺少租户、Harness、消息、Attempt、Session 或事件类型")
	}
	if !supportedAgentCallbackEvent(input.EventType) {
		return AgentCallbackResult{}, domain.Invalid("AGENT_CALLBACK_EVENT_UNSUPPORTED", "Agent 回调事件类型不受支持")
	}
	if len(input.Data) > 64<<10 {
		return AgentCallbackResult{}, domain.Policy("HARNESS_EVENT_TOO_LARGE", "Harness 结构化事件超过持久化上限", "只上报事件类型、状态和受控摘要")
	}
	dataValue := any(nil)
	if len(input.Data) > 0 && json.Unmarshal(input.Data, &dataValue) != nil {
		return AgentCallbackResult{}, domain.Invalid("AGENT_CALLBACK_DATA_INVALID", "Agent 回调 data 必须是合法 JSON")
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = s.now().UTC()
	} else {
		input.OccurredAt = input.OccurredAt.UTC()
	}
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = s.now().UTC()
	} else {
		input.ReceivedAt = input.ReceivedAt.UTC()
	}

	handle, err := s.LoadDispatchHandle(ctx, input.TenantID, input.AttemptID)
	if err != nil {
		return AgentCallbackResult{}, err
	}
	boundSession, err := boundAgentCallbackSession(handle.Attempt, input)
	if err != nil {
		return AgentCallbackResult{Attempt: handle.Attempt, Node: handle.Node}, err
	}

	message, err := s.newAgentInboxMessage(input, handle, dataValue)
	if err != nil {
		return AgentCallbackResult{Attempt: handle.Attempt, Node: handle.Node}, err
	}
	commands, err := s.commands()
	if err != nil {
		return AgentCallbackResult{Message: message, Attempt: handle.Attempt, Node: handle.Node}, err
	}
	stored, err := commands.ReceiveAgentInboxCommand(ctx, message, domain.JobEvent{
		ID: domain.NewID(), TenantID: message.TenantID, JobRunID: message.JobRunID, NodeKey: handle.Node.NodeKey,
		Type: "agent.inbox.received", ActorType: "harness", ActorID: input.HarnessKind,
		IdempotencyKey: "agent-inbox:" + message.ProviderID + ":" + message.MessageID + ":received",
		Payload:        map[string]any{"attempt_id": input.AttemptID, "session_id": input.SessionID, "event_type": input.EventType, "received_digest": message.ReceivedDigest}, OccurredAt: input.ReceivedAt,
	})
	if err != nil {
		return AgentCallbackResult{Message: stored, Attempt: handle.Attempt, Node: handle.Node}, err
	}
	result := AgentCallbackResult{Message: stored, Attempt: handle.Attempt, Node: handle.Node, Replayed: stored.ID != message.ID}
	if stored.State == domain.ProviderInboxApplied {
		return result, nil
	}
	if stored.State == domain.ProviderInboxFailed {
		return result, domain.Conflict("AGENT_CALLBACK_PREVIOUSLY_FAILED", "相同 Agent 回调已经处理失败")
	}

	event := agentadapter.AgentEvent{Type: input.EventType, Session: boundSession, Data: input.Data, ErrorCode: input.ErrorCode, OccurredAt: input.OccurredAt}
	switch input.EventType {
	case "result.completed":
		outcome, parseErr := outcomeFromHarnessResult(input.Data)
		if parseErr != nil {
			return result, parseErr
		}
		finalized, finalizeErr := s.FinalizeDispatch(ctx, handle, outcome)
		if finalizeErr != nil {
			return result, finalizeErr
		}
		handle, result.Job = finalized.Handle, finalized.Job
	case "session.failed":
		finalized, finalizeErr := s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: safeErrorCode(input.ErrorCode, "HARNESS_SESSION_FAILED"), UsedCostMinor: handle.Agent.UsedCostMinor})
		if finalizeErr != nil {
			return result, finalizeErr
		}
		handle, result.Job = finalized.Handle, finalized.Job
	case "session.interrupted":
		finalized, finalizeErr := s.FinalizeDispatch(ctx, handle, DispatchOutcome{State: domain.RuntimeAttemptRetryableFailed, ErrorCode: "HARNESS_SESSION_INTERRUPTED", UsedCostMinor: handle.Agent.UsedCostMinor})
		if finalizeErr != nil {
			return result, finalizeErr
		}
		handle, result.Job = finalized.Handle, finalized.Job
	case "runtime.yield":
		var request struct {
			Reason        string         `json:"reason"`
			WaitRefs      []string       `json:"wait_refs"`
			SafeSummary   map[string]any `json:"safe_summary"`
			UsedCostMinor int64          `json:"used_cost_minor"`
		}
		if len(input.Data) == 0 || json.Unmarshal(input.Data, &request) != nil {
			return result, domain.Invalid("HARNESS_YIELD_INVALID", "Harness 让出执行权事件无效")
		}
		yielded, yieldErr := s.YieldDispatch(ctx, handle, YieldDispatchInput{Reason: request.Reason, WaitRefs: request.WaitRefs, SafeSummary: request.SafeSummary, UsedCostMinor: request.UsedCostMinor})
		if yieldErr != nil {
			return result, yieldErr
		}
		handle, result.Job = yielded.Handle, yielded.Job
	default:
		if handle.Attempt.Terminal() {
			break
		}
		if err := s.RecordHarnessEvent(ctx, handle, event); err != nil {
			return result, err
		}
	}

	processedAt := s.now().UTC()
	completed := stored
	completed.State = domain.ProviderInboxApplied
	completed.ErrorCode = ""
	completed.ProcessedAt = &processedAt
	completed.Version++
	completed.UpdatedAt = processedAt
	completed, err = commands.CompleteAgentInboxCommand(ctx, completed, stored.Version, domain.JobEvent{
		ID: domain.NewID(), TenantID: completed.TenantID, JobRunID: completed.JobRunID, NodeKey: handle.Node.NodeKey,
		Type: "agent.inbox.applied", ActorType: "runtime", ActorID: input.HarnessKind,
		IdempotencyKey: "agent-inbox:" + completed.ID + ":applied",
		Payload:        map[string]any{"attempt_id": input.AttemptID, "event_type": input.EventType, "received_digest": completed.ReceivedDigest}, OccurredAt: processedAt,
	})
	result.Message, result.Attempt, result.Node, result.Applied = completed, handle.Attempt, handle.Node, err == nil
	return result, err
}

func (s *Service) newAgentInboxMessage(input AgentCallbackInput, handle DispatchHandle, dataValue any) (domain.ProviderInboxMessage, error) {
	dataDigest := ""
	if dataValue != nil {
		digest, err := domain.CanonicalHash(dataValue)
		if err != nil {
			return domain.ProviderInboxMessage{}, err
		}
		dataDigest = "sha256:" + digest
	}
	receivedDigest := strings.TrimSpace(input.ReceivedDigest)
	if receivedDigest == "" {
		digest, err := domain.CanonicalHash(struct {
			HarnessKind string
			MessageID   string
			AttemptID   string
			SessionID   string
			EventType   string
			Data        any
			ErrorCode   string
			OccurredAt  time.Time
		}{input.HarnessKind, input.MessageID, input.AttemptID, input.SessionID, input.EventType, dataValue, input.ErrorCode, input.OccurredAt})
		if err != nil {
			return domain.ProviderInboxMessage{}, err
		}
		receivedDigest = "sha256:" + digest
	}
	responseDigest, err := domain.CanonicalHash(struct {
		EventType  string
		DataDigest string
		ErrorCode  string
	}{input.EventType, dataDigest, input.ErrorCode})
	if err != nil {
		return domain.ProviderInboxMessage{}, err
	}
	safePayload := map[string]any{"attempt_id": input.AttemptID, "session_id": input.SessionID, "event_type": input.EventType, "data_digest": dataDigest}
	if values, ok := dataValue.(map[string]any); ok {
		safePayload["data"] = sanitizeSafeSummary(values)
	}
	return domain.ProviderInboxMessage{
		ID: domain.NewID(), TenantID: input.TenantID, JobRunID: handle.Attempt.JobRunID,
		ProviderID: agentHarnessProviderPrefix + input.HarnessKind, MessageID: input.MessageID,
		ReceivedDigest: receivedDigest, ExternalID: input.SessionID, ProviderState: input.EventType,
		ResponseDigest: "sha256:" + responseDigest, CostMinor: 0, Currency: "CNY", SafePayload: safePayload,
		State: domain.ProviderInboxReceived, ErrorCode: input.ErrorCode, ReceivedAt: input.ReceivedAt,
		Version: 1, CreatedAt: input.ReceivedAt, UpdatedAt: input.ReceivedAt,
	}, nil
}

func boundAgentCallbackSession(attempt domain.RuntimeAttempt, input AgentCallbackInput) (agentadapter.AgentSessionRef, error) {
	if attempt.HarnessKind != input.HarnessKind {
		return agentadapter.AgentSessionRef{}, domain.Conflict("AGENT_CALLBACK_HARNESS_MISMATCH", "Agent 回调 Harness 与 RuntimeAttempt 不一致")
	}
	var session agentadapter.AgentSessionRef
	if attempt.SessionRef == "" || json.Unmarshal([]byte(attempt.SessionRef), &session) != nil || session.HarnessKind != input.HarnessKind || session.SessionID != input.SessionID || (session.TenantID != "" && session.TenantID != input.TenantID) {
		return agentadapter.AgentSessionRef{}, domain.Conflict("HARNESS_SESSION_MISMATCH", "Agent 回调 Session 与 RuntimeAttempt 不一致")
	}
	return session, nil
}

func supportedAgentCallbackEvent(eventType string) bool {
	switch eventType {
	case "session.started", "session.resumed", "session.progress", "usage.reported", "result.completed", "session.failed", "session.interrupted", "runtime.yield":
		return true
	default:
		return false
	}
}
