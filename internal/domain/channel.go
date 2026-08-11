package domain

import (
	"strings"
	"time"
)

const (
	ChannelBindingActive   = "active"
	ChannelBindingDisabled = "disabled"

	ChannelPublicationPrepared             = "prepared"
	ChannelPublicationManualActionRequired = "manual_action_required"
	ChannelPublicationSubmitted            = "submitted"
	ChannelPublicationPublished            = "published"
	ChannelPublicationFailed               = "failed"
	ChannelPublicationUnknown              = "unknown"
	ChannelPublicationWithdrawn            = "withdrawn"
)

// ChannelBinding fixes the adapter, external account and credential reference
// used by a publication. Secret material remains in the configured secret store.
type ChannelBinding struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	ProjectID              string    `json:"project_id"`
	Channel                string    `json:"channel"`
	AdapterID              string    `json:"adapter_id"`
	AccountRef             string    `json:"account_ref"`
	AuthorizationSecretRef string    `json:"authorization_secret_ref"`
	Region                 string    `json:"region"`
	Status                 string    `json:"status"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

func (v ChannelBinding) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || strings.TrimSpace(v.Channel) == "" || strings.TrimSpace(v.AdapterID) == "" || strings.TrimSpace(v.AccountRef) == "" || strings.TrimSpace(v.AuthorizationSecretRef) == "" {
		return Invalid("CHANNEL_BINDING_INVALID", "渠道绑定缺少项目、渠道、适配器、账号或授权 SecretRef")
	}
	switch v.Status {
	case ChannelBindingActive, ChannelBindingDisabled:
	default:
		return Invalid("CHANNEL_BINDING_STATUS_INVALID", "渠道绑定状态无效")
	}
	return nil
}

// ChannelPublication is the durable publication intent and latest external
// receipt. It never stores credentials or an unredacted provider payload.
type ChannelPublication struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	ProjectID         string         `json:"project_id"`
	TaskID            string         `json:"task_id"`
	TaskDeliveryID    string         `json:"task_delivery_id"`
	DeliveryPackageID string         `json:"delivery_package_id"`
	ChannelBindingID  string         `json:"channel_binding_id"`
	Channel           string         `json:"channel"`
	AccountRef        string         `json:"account_ref"`
	State             string         `json:"state"`
	IdempotencyKey    string         `json:"idempotency_key"`
	DeliveryDigest    string         `json:"delivery_digest"`
	RequestDigest     string         `json:"request_digest"`
	ResponseDigest    string         `json:"response_digest,omitempty"`
	ExternalID        string         `json:"external_id,omitempty"`
	ExternalURL       string         `json:"external_url,omitempty"`
	Checklist         []string       `json:"checklist"`
	Preview           map[string]any `json:"preview"`
	Metadata          map[string]any `json:"metadata"`
	SafeSummary       map[string]any `json:"safe_summary"`
	CostMinor         int64          `json:"cost_minor"`
	Currency          string         `json:"currency,omitempty"`
	ErrorCode         string         `json:"error_code,omitempty"`
	ScheduledAt       *time.Time     `json:"scheduled_at,omitempty"`
	SubmittedAt       *time.Time     `json:"submitted_at,omitempty"`
	PublishedAt       *time.Time     `json:"published_at,omitempty"`
	ObservedAt        time.Time      `json:"observed_at"`
	CreatedBy         string         `json:"created_by"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// ChannelCallbackReceipt is an immutable, deduplicated ingress fact. The raw
// provider payload is represented only by a digest and redacted summary.
type ChannelCallbackReceipt struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	PublicationID string         `json:"publication_id"`
	AdapterID     string         `json:"adapter_id"`
	EventID       string         `json:"event_id"`
	PayloadDigest string         `json:"payload_digest"`
	State         string         `json:"state"`
	SafeSummary   map[string]any `json:"safe_summary"`
	ObservedAt    time.Time      `json:"observed_at"`
	ReceivedAt    time.Time      `json:"received_at"`
}

func (v ChannelCallbackReceipt) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.PublicationID == "" || strings.TrimSpace(v.AdapterID) == "" || strings.TrimSpace(v.EventID) == "" || !validSHA256Digest(v.PayloadDigest) {
		return Invalid("CHANNEL_CALLBACK_RECEIPT_INVALID", "渠道 Callback 回执缺少作用域、事件 ID 或 Payload 摘要")
	}
	switch v.State {
	case ChannelPublicationSubmitted, ChannelPublicationPublished, ChannelPublicationFailed, ChannelPublicationUnknown, ChannelPublicationWithdrawn:
	default:
		return Invalid("CHANNEL_CALLBACK_STATE_INVALID", "渠道 Callback 状态无效")
	}
	if v.ObservedAt.IsZero() || v.ReceivedAt.IsZero() {
		return Invalid("CHANNEL_CALLBACK_TIME_INVALID", "渠道 Callback 缺少观察或接收时间")
	}
	return nil
}

func (v *ChannelPublication) NormalizeCollections() {
	if v.Checklist == nil {
		v.Checklist = []string{}
	}
	if v.Preview == nil {
		v.Preview = map[string]any{}
	}
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
	if v.SafeSummary == nil {
		v.SafeSummary = map[string]any{}
	}
}

func (v ChannelPublication) Validate() error {
	v.NormalizeCollections()
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.TaskDeliveryID == "" || v.DeliveryPackageID == "" || v.ChannelBindingID == "" || v.IdempotencyKey == "" {
		return Invalid("CHANNEL_PUBLICATION_INVALID", "渠道发布缺少任务、交付、渠道绑定或幂等键")
	}
	if !validSHA256Digest(v.DeliveryDigest) || !validSHA256Digest(v.RequestDigest) {
		return Invalid("CHANNEL_PUBLICATION_DIGEST_INVALID", "渠道发布必须固定交付摘要和请求摘要")
	}
	if v.ResponseDigest != "" && !validSHA256Digest(v.ResponseDigest) {
		return Invalid("CHANNEL_PUBLICATION_RESPONSE_DIGEST_INVALID", "渠道发布响应摘要无效")
	}
	switch v.State {
	case ChannelPublicationPrepared, ChannelPublicationManualActionRequired, ChannelPublicationSubmitted, ChannelPublicationPublished, ChannelPublicationFailed, ChannelPublicationUnknown, ChannelPublicationWithdrawn:
	default:
		return Invalid("CHANNEL_PUBLICATION_STATE_INVALID", "渠道发布状态无效")
	}
	if v.State == ChannelPublicationPublished && (strings.TrimSpace(v.ExternalID) == "" || v.PublishedAt == nil) {
		return Invalid("CHANNEL_PUBLICATION_RECEIPT_INCOMPLETE", "已发布回执必须包含外部 ID 和发布时间")
	}
	return nil
}
