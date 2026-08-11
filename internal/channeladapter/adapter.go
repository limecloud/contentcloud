package channeladapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	StatePrepared             = "prepared"
	StateManualActionRequired = "manual_action_required"
	StateSubmitted            = "submitted"
	StatePublished            = "published"
	StateFailed               = "failed"
	StateUnknown              = "unknown"
	StateWithdrawn            = "withdrawn"
)

type Request struct {
	TenantID          string         `json:"tenant_id"`
	ProjectID         string         `json:"project_id"`
	DeliveryPackageID string         `json:"delivery_package_id"`
	DeliveryDigest    string         `json:"delivery_digest"`
	Channel           string         `json:"channel"`
	AccountRef        string         `json:"account_ref"`
	AuthorizationRef  string         `json:"authorization_ref"`
	IdempotencyKey    string         `json:"idempotency_key"`
	ScheduledAt       *time.Time     `json:"scheduled_at,omitempty"`
	Metadata          map[string]any `json:"metadata"`
}

type Prepared struct {
	Request       Request        `json:"request"`
	RequestDigest string         `json:"request_digest"`
	Checklist     []string       `json:"checklist"`
	Preview       map[string]any `json:"preview"`
	PreparedAt    time.Time      `json:"prepared_at"`
}

type Receipt struct {
	Channel        string         `json:"channel"`
	AccountRef     string         `json:"account_ref"`
	State          string         `json:"state"`
	ExternalID     string         `json:"external_id,omitempty"`
	ExternalURL    string         `json:"external_url,omitempty"`
	RequestDigest  string         `json:"request_digest"`
	ResponseDigest string         `json:"response_digest,omitempty"`
	ErrorCode      string         `json:"error_code,omitempty"`
	CostMinor      int64          `json:"cost_minor"`
	Currency       string         `json:"currency,omitempty"`
	PublishedAt    *time.Time     `json:"published_at,omitempty"`
	ObservedAt     time.Time      `json:"observed_at"`
	SafeSummary    map[string]any `json:"safe_summary"`
}

type Adapter interface {
	Validate(context.Context, Request) error
	Prepare(context.Context, Request) (Prepared, error)
	Submit(context.Context, Prepared) (Receipt, error)
	Inspect(context.Context, Receipt) (Receipt, error)
	Withdraw(context.Context, Receipt, string) (Receipt, error)
}

type ManualAdapter struct{}

func (ManualAdapter) Validate(_ context.Context, request Request) error {
	return validateRequest(request)
}

func (ManualAdapter) Prepare(ctx context.Context, request Request) (Prepared, error) {
	if err := (ManualAdapter{}).Validate(ctx, request); err != nil {
		return Prepared{}, err
	}
	return prepare(request, []string{"verify_account", "upload_assets", "paste_content", "send_preview", "confirm_publish", "record_external_binding"})
}

func (ManualAdapter) Submit(_ context.Context, prepared Prepared) (Receipt, error) {
	return Receipt{Channel: prepared.Request.Channel, AccountRef: prepared.Request.AccountRef, State: StateManualActionRequired, RequestDigest: prepared.RequestDigest, ObservedAt: time.Now().UTC(), SafeSummary: map[string]any{"next_action": "operator_publish_and_record_receipt"}}, nil
}

func (ManualAdapter) Inspect(_ context.Context, receipt Receipt) (Receipt, error) {
	if receipt.State != StateManualActionRequired && receipt.State != StatePublished {
		return Receipt{}, domain.Conflict("CHANNEL_RECEIPT_STATE_INVALID", "人工渠道回执状态无法检查")
	}
	receipt.ObservedAt = time.Now().UTC()
	return receipt, nil
}

func (ManualAdapter) Withdraw(_ context.Context, _ Receipt, _ string) (Receipt, error) {
	return Receipt{}, domain.Policy("CHANNEL_WITHDRAW_MANUAL_REQUIRED", "人工发布内容必须由操作员在渠道后台撤回", "完成撤回后记录新的外部回执")
}

type HTTPConfig struct {
	Endpoint  string
	Token     string
	Client    *http.Client
	AllowHTTP bool
}

type HTTPAdapter struct {
	endpoint *url.URL
	token    string
	client   *http.Client
}

func NewHTTP(config HTTPConfig) (*HTTPAdapter, error) {
	endpoint, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.Endpoint), "/"))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http")) {
		return nil, domain.Invalid("CHANNEL_ENDPOINT_INVALID", "Channel Adapter Endpoint 必须是 HTTPS URL")
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &HTTPAdapter{endpoint: endpoint, token: strings.TrimSpace(config.Token), client: client}, nil
}

func (a *HTTPAdapter) Validate(_ context.Context, request Request) error {
	return validateRequest(request)
}

func (a *HTTPAdapter) Prepare(ctx context.Context, request Request) (Prepared, error) {
	if err := a.Validate(ctx, request); err != nil {
		return Prepared{}, err
	}
	return prepare(request, []string{"validate_channel_spec", "validate_account_scope", "validate_rights", "preview_fixed_package"})
}

func (a *HTTPAdapter) Submit(ctx context.Context, prepared Prepared) (Receipt, error) {
	var response Receipt
	if err := a.request(ctx, http.MethodPost, "/v1/publications", prepared.Request.IdempotencyKey, prepared, &response); err != nil {
		return Receipt{Channel: prepared.Request.Channel, AccountRef: prepared.Request.AccountRef, State: StateUnknown, RequestDigest: prepared.RequestDigest, ErrorCode: "CHANNEL_SUBMIT_UNKNOWN", ObservedAt: time.Now().UTC(), SafeSummary: map[string]any{}}, err
	}
	if response.ExternalID == "" || !validRemoteState(response.State) {
		return Receipt{}, domain.Invalid("CHANNEL_RESPONSE_INVALID", "Channel Adapter 提交响应缺少外部 ID 或状态")
	}
	response.Channel = prepared.Request.Channel
	response.AccountRef = prepared.Request.AccountRef
	response.RequestDigest = prepared.RequestDigest
	response.ObservedAt = time.Now().UTC()
	response.SafeSummary = safeMap(response.SafeSummary)
	return response, nil
}

func (a *HTTPAdapter) Inspect(ctx context.Context, receipt Receipt) (Receipt, error) {
	if strings.TrimSpace(receipt.ExternalID) == "" || strings.ContainsAny(receipt.ExternalID, "/?#") {
		return Receipt{}, domain.Invalid("CHANNEL_EXTERNAL_ID_INVALID", "Channel Receipt 外部 ID 无效")
	}
	var observed Receipt
	if err := a.request(ctx, http.MethodGet, "/v1/publications/"+url.PathEscape(receipt.ExternalID), "", nil, &observed); err != nil {
		receipt.State = StateUnknown
		receipt.ErrorCode = "CHANNEL_INSPECT_UNKNOWN"
		receipt.ObservedAt = time.Now().UTC()
		return receipt, err
	}
	if !validRemoteState(observed.State) {
		return Receipt{}, domain.Invalid("CHANNEL_RESPONSE_INVALID", "Channel Adapter Inspect 状态无效")
	}
	observed.Channel = receipt.Channel
	observed.AccountRef = receipt.AccountRef
	observed.RequestDigest = receipt.RequestDigest
	observed.ExternalID = receipt.ExternalID
	observed.ObservedAt = time.Now().UTC()
	observed.SafeSummary = safeMap(observed.SafeSummary)
	return observed, nil
}

func (a *HTTPAdapter) Withdraw(ctx context.Context, receipt Receipt, reason string) (Receipt, error) {
	if strings.TrimSpace(reason) == "" {
		return Receipt{}, domain.Invalid("CHANNEL_WITHDRAW_REASON_REQUIRED", "撤回渠道内容必须说明原因")
	}
	var result Receipt
	path := "/v1/publications/" + url.PathEscape(receipt.ExternalID) + "/withdraw"
	if err := a.request(ctx, http.MethodPost, path, receipt.RequestDigest+":withdraw", map[string]any{"reason": reason}, &result); err != nil {
		return Receipt{}, err
	}
	if result.State != StateWithdrawn {
		return Receipt{}, domain.Invalid("CHANNEL_RESPONSE_INVALID", "Channel Adapter 未确认撤回状态")
	}
	result.Channel, result.AccountRef, result.RequestDigest = receipt.Channel, receipt.AccountRef, receipt.RequestDigest
	result.ExternalID, result.ObservedAt = receipt.ExternalID, time.Now().UTC()
	result.SafeSummary = safeMap(result.SafeSummary)
	return result, nil
}

func validateRequest(request Request) error {
	if request.TenantID == "" || request.ProjectID == "" || request.DeliveryPackageID == "" || request.Channel == "" || request.AccountRef == "" || request.AuthorizationRef == "" || request.IdempotencyKey == "" {
		return domain.Invalid("CHANNEL_REQUEST_INVALID", "渠道请求缺少租户、项目、交付包、账号、授权或幂等键")
	}
	if !strings.HasPrefix(request.DeliveryDigest, "sha256:") || len(request.DeliveryDigest) != 71 {
		return domain.Invalid("CHANNEL_DELIVERY_DIGEST_INVALID", "渠道请求必须固定有效的交付摘要")
	}
	return nil
}

func prepare(request Request, checklist []string) (Prepared, error) {
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	digest, err := domain.CanonicalHash(request)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Request: request, RequestDigest: "sha256:" + digest, Checklist: checklist, Preview: map[string]any{"channel": request.Channel, "account_ref": request.AccountRef, "delivery_package_id": request.DeliveryPackageID, "scheduled_at": request.ScheduledAt}, PreparedAt: time.Now().UTC()}, nil
}

func (a *HTTPAdapter) request(ctx context.Context, method, path, idempotency string, input, output any) error {
	target := a.endpoint.ResolveReference(&url.URL{Path: path})
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotency != "" {
		request.Header.Set("Idempotency-Key", idempotency)
	}
	if a.token != "" {
		request.Header.Set("Authorization", "Bearer "+a.token)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("channel adapter returned HTTP %d", response.StatusCode)
	}
	if output != nil && (len(bytes.TrimSpace(data)) == 0 || json.Unmarshal(data, output) != nil) {
		return errors.New("channel adapter response is not valid JSON")
	}
	return nil
}

func validRemoteState(value string) bool {
	switch value {
	case StateSubmitted, StatePublished, StateFailed, StateUnknown, StateWithdrawn:
		return true
	default:
		return false
	}
}

func safeMap(input map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range input {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") {
			result[key] = "[redacted]"
			continue
		}
		result[key] = value
	}
	return result
}

var _ Adapter = ManualAdapter{}
var _ Adapter = (*HTTPAdapter)(nil)
