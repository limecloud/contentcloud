package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

const (
	providerIngressBodyLimit = 256 << 10
	providerIngressClockSkew = 5 * time.Minute
)

type providerCallbackRequest struct {
	JobRunID       string         `json:"job_run_id,omitempty"`
	EffectID       string         `json:"effect_id,omitempty"`
	MessageID      string         `json:"message_id"`
	ExternalID     string         `json:"external_id"`
	ProviderState  string         `json:"provider_state"`
	ResponseDigest string         `json:"response_digest,omitempty"`
	CostMinor      int64          `json:"cost_minor,omitempty"`
	Currency       string         `json:"currency,omitempty"`
	SafePayload    map[string]any `json:"safe_payload,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
}

type providerBillRequest struct {
	JobRunID    string `json:"job_run_id,omitempty"`
	EffectID    string `json:"effect_id,omitempty"`
	BillID      string `json:"bill_id"`
	ExternalID  string `json:"external_id"`
	BillDigest  string `json:"bill_digest,omitempty"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency,omitempty"`
	ObservedAt  string `json:"observed_at,omitempty"`
}

func (s *Server) providerCallback(w http.ResponseWriter, r *http.Request) {
	providerID, tenantID, body, ok := s.authenticateProviderIngress(w, r)
	if !ok {
		return
	}
	var input providerCallbackRequest
	if !decodeProviderIngressBody(body, &input) {
		s.fail(w, r, "provider.callback", domain.Invalid("PROVIDER_CALLBACK_INVALID", "Provider 回调 JSON 无效"))
		return
	}
	if s.service.Runtime() == nil {
		s.fail(w, r, "provider.callback", domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime"))
		return
	}
	message, effect, err := s.service.Runtime().ReceiveProviderCallback(r.Context(), contentruntime.ProviderCallbackInput{
		TenantID: tenantID, JobRunID: strings.TrimSpace(input.JobRunID), EffectID: strings.TrimSpace(input.EffectID), ProviderID: providerID,
		MessageID: strings.TrimSpace(input.MessageID), ExternalID: strings.TrimSpace(input.ExternalID), ProviderState: strings.TrimSpace(input.ProviderState),
		ResponseDigest: strings.TrimSpace(input.ResponseDigest), CostMinor: input.CostMinor, Currency: strings.TrimSpace(input.Currency),
		SafePayload: input.SafePayload, ErrorCode: strings.TrimSpace(input.ErrorCode), ReceivedAt: time.Now().UTC(),
	})
	s.dispatchResult(w, r, "provider.callback", map[string]any{"message": message, "effect": effect}, err)
}

func (s *Server) providerBill(w http.ResponseWriter, r *http.Request) {
	providerID, tenantID, body, ok := s.authenticateProviderIngress(w, r)
	if !ok {
		return
	}
	var input providerBillRequest
	if !decodeProviderIngressBody(body, &input) {
		s.fail(w, r, "provider.bill", domain.Invalid("PROVIDER_BILL_INVALID", "Provider 账单 JSON 无效"))
		return
	}
	if s.service.Runtime() == nil {
		s.fail(w, r, "provider.bill", domain.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime"))
		return
	}
	observedAt := time.Time{}
	if strings.TrimSpace(input.ObservedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.ObservedAt))
		if err != nil {
			s.fail(w, r, "provider.bill", domain.Invalid("PROVIDER_BILL_INVALID", "Provider 账单 observed_at 必须是 RFC3339 时间"))
			return
		}
		observedAt = parsed.UTC()
	}
	bill, err := s.service.Runtime().RecordProviderBill(r.Context(), contentruntime.ProviderBillInput{
		TenantID: tenantID, JobRunID: strings.TrimSpace(input.JobRunID), EffectID: strings.TrimSpace(input.EffectID), ProviderID: providerID,
		BillID: strings.TrimSpace(input.BillID), ExternalID: strings.TrimSpace(input.ExternalID), BillDigest: strings.TrimSpace(input.BillDigest),
		AmountMinor: input.AmountMinor, Currency: strings.TrimSpace(input.Currency), ObservedAt: observedAt,
	})
	s.dispatchResult(w, r, "provider.bill", bill, err)
}

func (s *Server) authenticateProviderIngress(w http.ResponseWriter, r *http.Request) (string, string, []byte, bool) {
	providerID := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "providerID")))
	tenantID := strings.TrimSpace(chi.URLParam(r, "tenantID"))
	if providerID == "" || tenantID == "" {
		s.fail(w, r, "provider.ingress.auth", domain.Invalid("PROVIDER_INGRESS_SCOPE_INVALID", "Provider ingress 缺少服务商或租户作用域"))
		return "", "", nil, false
	}
	binding, err := s.service.ProviderBinding(r.Context(), tenantID, providerID)
	if err != nil {
		s.fail(w, r, "provider.ingress.auth", domain.Policy("PROVIDER_INGRESS_UNBOUND", "Provider ingress 没有有效的租户绑定", "先配置当前租户的 Provider Binding"))
		return "", "", nil, false
	}
	if binding.State != "active" {
		s.fail(w, r, "provider.ingress.auth", domain.Policy("PROVIDER_INGRESS_DISABLED", "Provider ingress 绑定未启用", "启用当前租户的 Provider Binding"))
		return "", "", nil, false
	}
	secret := s.providerCallbackSecrets[tenantID+":"+providerID]
	if len(secret) == 0 {
		s.fail(w, r, "provider.ingress.auth", domain.Policy("PROVIDER_INGRESS_SECRET_UNAVAILABLE", "Provider ingress 未配置回调密钥", "在服务端密钥配置中登记当前租户和 Provider 的 HMAC 密钥"))
		return "", "", nil, false
	}
	timestamp := strings.TrimSpace(r.Header.Get("X-ContentCloud-Timestamp"))
	provided := strings.TrimSpace(r.Header.Get("X-ContentCloud-Signature"))
	seconds, parseErr := strconv.ParseInt(timestamp, 10, 64)
	if parseErr != nil || timestamp == "" || provided == "" || absDuration(time.Since(time.Unix(seconds, 0))) > providerIngressClockSkew {
		s.fail(w, r, "provider.ingress.auth", domain.Invalid("PROVIDER_INGRESS_REPLAY", "Provider ingress 时间戳无效或已超出重放保护窗口"))
		return "", "", nil, false
	}
	body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, providerIngressBodyLimit+1))
	if readErr != nil || int64(len(body)) > providerIngressBodyLimit {
		s.fail(w, r, "provider.ingress.auth", domain.Invalid("PROVIDER_INGRESS_BODY_TOO_LARGE", "Provider ingress 请求体超过大小限制"))
		return "", "", nil, false
	}
	digest := sha256.Sum256(body)
	digestHex := hex.EncodeToString(digest[:])
	message := timestamp + "\n" + digestHex + "\n" + r.URL.Path
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(message))
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		s.fail(w, r, "provider.ingress.auth", domain.Invalid("PROVIDER_INGRESS_SIGNATURE_INVALID", "Provider ingress 签名校验失败"))
		return "", "", nil, false
	}
	return providerID, tenantID, body, true
}

func decodeProviderIngressBody(body []byte, out any) bool {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
