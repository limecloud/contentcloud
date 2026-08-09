package domain

import "time"

const (
	ProviderInboxReceived = "received"
	ProviderInboxApplied  = "applied"
	ProviderInboxPending  = "pending_reconciliation"
	ProviderInboxFailed   = "failed"

	ProviderReconPending      = "pending"
	ProviderReconMatched      = "matched"
	ProviderReconCostMismatch = "cost_mismatch"
	ProviderReconManual       = "manual_action"

	ProviderBillMatched   = "matched"
	ProviderBillDisputed  = "disputed"
	ProviderBillUnmatched = "unmatched"
)

// ProviderInboxMessage is the durable ingress fact for a provider callback or
// poll result. The message identity is provider-owned; the digest is local
// identity and prevents the same message ID from changing meaning.
type ProviderInboxMessage struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	JobRunID       string         `json:"job_run_id"`
	ProviderID     string         `json:"provider_id"`
	MessageID      string         `json:"message_id"`
	ReceivedDigest string         `json:"received_digest"`
	ExternalID     string         `json:"external_id"`
	EffectID       string         `json:"effect_id,omitempty"`
	ProviderState  string         `json:"provider_state"`
	ResponseDigest string         `json:"response_digest"`
	CostMinor      int64          `json:"cost_minor"`
	Currency       string         `json:"currency"`
	SafePayload    map[string]any `json:"safe_payload"`
	State          string         `json:"state"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ReceivedAt     time.Time      `json:"received_at"`
	ProcessedAt    *time.Time     `json:"processed_at,omitempty"`
	Version        int            `json:"version"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (m *ProviderInboxMessage) NormalizeCollections() {
	if m.SafePayload == nil {
		m.SafePayload = map[string]any{}
	}
}

func (m ProviderInboxMessage) Validate() error {
	if m.ID == "" || m.TenantID == "" || m.JobRunID == "" || m.ProviderID == "" || m.MessageID == "" || m.ExternalID == "" || m.ProviderState == "" || m.Currency == "" || m.CostMinor < 0 || m.Version < 1 || m.ReceivedAt.IsZero() || m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() || !validSHA256Digest(m.ReceivedDigest) || !validSHA256Digest(m.ResponseDigest) {
		return Invalid("PROVIDER_INBOX_INVALID", "Provider 回调缺少消息身份、外部任务、摘要或费用")
	}
	switch m.State {
	case ProviderInboxReceived, ProviderInboxApplied, ProviderInboxPending, ProviderInboxFailed:
	default:
		return Invalid("PROVIDER_INBOX_STATE_INVALID", "Provider 回调入站状态无效")
	}
	return nil
}

type ProviderReconciliation struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	JobRunID       string         `json:"job_run_id"`
	EffectID       string         `json:"effect_id,omitempty"`
	ProviderID     string         `json:"provider_id"`
	ExternalID     string         `json:"external_id"`
	RequestKey     string         `json:"request_key"`
	ObservedState  string         `json:"observed_state"`
	ResponseDigest string         `json:"response_digest"`
	ExpectedMinor  int64          `json:"expected_minor"`
	ObservedMinor  int64          `json:"observed_minor"`
	Currency       string         `json:"currency"`
	Reason         string         `json:"reason,omitempty"`
	Status         string         `json:"status"`
	SafeSummary    map[string]any `json:"safe_summary"`
	ResolvedAt     *time.Time     `json:"resolved_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Version        int            `json:"version"`
}

func (r *ProviderReconciliation) NormalizeCollections() {
	if r.SafeSummary == nil {
		r.SafeSummary = map[string]any{}
	}
}

func (r ProviderReconciliation) Validate() error {
	if r.ID == "" || r.TenantID == "" || r.JobRunID == "" || r.ProviderID == "" || r.ExternalID == "" || r.RequestKey == "" || r.ObservedState == "" || !validSHA256Digest(r.ResponseDigest) || r.ExpectedMinor < 0 || r.ObservedMinor < 0 || r.Currency == "" || r.Status == "" || r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.Version < 1 {
		return Invalid("PROVIDER_RECONCILIATION_INVALID", "Provider 对账缺少外部身份、摘要、费用或状态")
	}
	switch r.Status {
	case ProviderReconPending, ProviderReconMatched, ProviderReconCostMismatch, ProviderReconManual:
	default:
		return Invalid("PROVIDER_RECONCILIATION_STATE_INVALID", "Provider 对账状态无效")
	}
	return nil
}

type ProviderBillRecord struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	JobRunID    string    `json:"job_run_id"`
	ProviderID  string    `json:"provider_id"`
	BillID      string    `json:"bill_id"`
	ExternalID  string    `json:"external_id"`
	EffectID    string    `json:"effect_id,omitempty"`
	BillDigest  string    `json:"bill_digest"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	ObservedAt  time.Time `json:"observed_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

func (b ProviderBillRecord) Validate() error {
	if b.ID == "" || b.TenantID == "" || b.JobRunID == "" || b.ProviderID == "" || b.BillID == "" || b.ExternalID == "" || !validSHA256Digest(b.BillDigest) || b.AmountMinor < 0 || b.Currency == "" || b.ObservedAt.IsZero() || b.CreatedAt.IsZero() || b.UpdatedAt.IsZero() || b.Version < 1 {
		return Invalid("PROVIDER_BILL_INVALID", "Provider 账单缺少账单身份、外部任务、摘要或金额")
	}
	switch b.Status {
	case ProviderBillMatched, ProviderBillDisputed, ProviderBillUnmatched:
	default:
		return Invalid("PROVIDER_BILL_STATE_INVALID", "Provider 账单对账状态无效")
	}
	return nil
}
