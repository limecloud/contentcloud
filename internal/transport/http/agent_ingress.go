package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/go-chi/chi/v5"

	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

type agentCallbackRequest struct {
	MessageID  string          `json:"message_id"`
	AttemptID  string          `json:"attempt_id"`
	SessionID  string          `json:"session_id"`
	EventType  string          `json:"event_type"`
	Data       json.RawMessage `json:"data,omitempty"`
	ErrorCode  string          `json:"error_code,omitempty"`
	OccurredAt string          `json:"occurred_at,omitempty"`
}

func (s *Server) agentCallback(w http.ResponseWriter, r *http.Request) {
	harnessKind, tenantID, body, digest, ok := s.authenticateAgentIngress(w, r)
	if !ok {
		return
	}
	var input agentCallbackRequest
	if !decodeProviderIngressBody(body, &input) {
		s.fail(w, r, "agent.callback", fault.Invalid("AGENT_CALLBACK_INVALID", "Agent 回调 JSON 无效"))
		return
	}
	occurredAt := time.Time{}
	if strings.TrimSpace(input.OccurredAt) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.OccurredAt))
		if err != nil {
			s.fail(w, r, "agent.callback", fault.Invalid("AGENT_CALLBACK_TIME_INVALID", "Agent 回调 occurred_at 必须是 RFC3339 时间"))
			return
		}
		occurredAt = parsed.UTC()
	}
	if s.service.Runtime.Runtime() == nil {
		s.fail(w, r, "agent.callback", fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime"))
		return
	}
	result, err := s.service.Runtime.Runtime().ReceiveAgentCallback(r.Context(), contentruntime.AgentCallbackInput{
		TenantID: tenantID, HarnessKind: harnessKind, MessageID: strings.TrimSpace(input.MessageID),
		AttemptID: strings.TrimSpace(input.AttemptID), SessionID: strings.TrimSpace(input.SessionID),
		EventType: strings.TrimSpace(input.EventType), Data: input.Data, ErrorCode: strings.TrimSpace(input.ErrorCode),
		OccurredAt: occurredAt, ReceivedAt: time.Now().UTC(), ReceivedDigest: digest,
	})
	s.dispatchResult(w, r, "agent.callback", result, err)
}

func (s *Server) authenticateAgentIngress(w http.ResponseWriter, r *http.Request) (string, string, []byte, string, bool) {
	harnessKind := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "harnessKind")))
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenantID"))
	if harnessKind == "" || tenantID == "" {
		s.fail(w, r, "agent.ingress.auth", fault.Invalid("AGENT_INGRESS_SCOPE_INVALID", "Agent ingress 缺少 Harness 或租户作用域"))
		return "", "", nil, "", false
	}
	secret := s.agentCallbackSecrets[tenantID+":"+harnessKind]
	if len(secret) == 0 {
		s.fail(w, r, "agent.ingress.auth", fault.Policy("AGENT_INGRESS_SECRET_UNAVAILABLE", "Agent ingress 未配置回调密钥", "在服务端密钥配置中登记租户和 Harness 的 HMAC 密钥"))
		return "", "", nil, "", false
	}
	body, digest, err := verifySignedIngress(w, r, secret, "AGENT_INGRESS", "Agent")
	if err != nil {
		s.fail(w, r, "agent.ingress.auth", err)
		return "", "", nil, "", false
	}
	return harnessKind, tenantID, body, digest, true
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
