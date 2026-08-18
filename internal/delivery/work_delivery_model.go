package delivery

import "github.com/limecloud/contentcloud/internal/platform/stablehash"
import "github.com/limecloud/contentcloud/internal/platform/fault"
import "time"
import "strings"
import "sort"

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
		return fault.Invalid("CHANNEL_BINDING_INVALID", "渠道绑定缺少项目、渠道、适配器、账号或授权 SecretRef")
	}
	switch v.Status {
	case ChannelBindingActive, ChannelBindingDisabled:
	default:
		return fault.Invalid("CHANNEL_BINDING_STATUS_INVALID", "渠道绑定状态无效")
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
	if v.ID == "" || v.TenantID == "" || v.PublicationID == "" || strings.TrimSpace(v.AdapterID) == "" || strings.TrimSpace(v.EventID) == "" || !stablehash.Valid(v.PayloadDigest) {
		return fault.Invalid("CHANNEL_CALLBACK_RECEIPT_INVALID", "渠道 Callback 回执缺少作用域、事件 ID 或 Payload 摘要")
	}
	switch v.State {
	case ChannelPublicationSubmitted, ChannelPublicationPublished, ChannelPublicationFailed, ChannelPublicationUnknown, ChannelPublicationWithdrawn:
	default:
		return fault.Invalid("CHANNEL_CALLBACK_STATE_INVALID", "渠道 Callback 状态无效")
	}
	if v.ObservedAt.IsZero() || v.ReceivedAt.IsZero() {
		return fault.Invalid("CHANNEL_CALLBACK_TIME_INVALID", "渠道 Callback 缺少观察或接收时间")
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
		return fault.Invalid("CHANNEL_PUBLICATION_INVALID", "渠道发布缺少任务、交付、渠道绑定或幂等键")
	}
	if !stablehash.Valid(v.DeliveryDigest) || !stablehash.Valid(v.RequestDigest) {
		return fault.Invalid("CHANNEL_PUBLICATION_DIGEST_INVALID", "渠道发布必须固定交付摘要和请求摘要")
	}
	if v.ResponseDigest != "" && !stablehash.Valid(v.ResponseDigest) {
		return fault.Invalid("CHANNEL_PUBLICATION_RESPONSE_DIGEST_INVALID", "渠道发布响应摘要无效")
	}
	switch v.State {
	case ChannelPublicationPrepared, ChannelPublicationManualActionRequired, ChannelPublicationSubmitted, ChannelPublicationPublished, ChannelPublicationFailed, ChannelPublicationUnknown, ChannelPublicationWithdrawn:
	default:
		return fault.Invalid("CHANNEL_PUBLICATION_STATE_INVALID", "渠道发布状态无效")
	}
	if v.State == ChannelPublicationPublished && (strings.TrimSpace(v.ExternalID) == "" || v.PublishedAt == nil) {
		return fault.Invalid("CHANNEL_PUBLICATION_RECEIPT_INCOMPLETE", "已发布回执必须包含外部 ID 和发布时间")
	}
	return nil
}

type Artifact struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	ProjectID          string         `json:"project_id"`
	ApprovedSnapshotID string         `json:"approved_snapshot_id"`
	Kind               string         `json:"kind"`
	CapabilityID       string         `json:"capability_id"`
	CapabilityVersion  string         `json:"capability_version"`
	CapabilityDigest   string         `json:"capability_digest"`
	SchemaID           string         `json:"schema_id"`
	MediaType          string         `json:"media_type"`
	FileName           string         `json:"file_name"`
	SHA256             string         `json:"sha256"`
	ByteSize           int64          `json:"byte_size"`
	ObjectKey          string         `json:"-"`
	Visibility         string         `json:"visibility"`
	RetentionClass     string         `json:"retention_class"`
	Purpose            string         `json:"purpose"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at"`
}

type DeliveryPackage struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	ProjectID           string     `json:"project_id"`
	ApprovedSnapshotIDs []string   `json:"approved_snapshot_ids"`
	ContentItemID       string     `json:"content_item_id"`
	Status              string     `json:"status"`
	Manifest            []Artifact `json:"manifest"`
	CreatedBy           string     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
}

const (
	MediaJobDraft                = "draft"
	MediaJobAwaitingCostApproval = "awaiting_cost_approval"
	MediaJobQueued               = "queued"
	MediaJobSubmitting           = "submitting"
	MediaJobSubmitted            = "submitted"
	MediaJobGenerating           = "generating"
	MediaJobDownloading          = "downloading"
	MediaJobValidating           = "validating"
	MediaJobSucceeded            = "succeeded"
	MediaJobRetryWait            = "retry_wait"
	MediaJobRetryableFailed      = "retryable_failed"
	MediaJobOutputInvalid        = "output_invalid"
	MediaJobFailed               = "failed"
	MediaJobCancelled            = "cancelled"
	MediaJobBudgetBlocked        = "budget_blocked"
	MediaJobAwaitingExternal     = "awaiting_external_result"
)

type MediaGenerationJob struct {
	ID                      string     `json:"id"`
	TenantID                string     `json:"tenant_id"`
	ProjectID               string     `json:"project_id"`
	TaskID                  string     `json:"task_id"`
	StageRunID              string     `json:"stage_run_id"`
	StoryboardSnapshotID    string     `json:"storyboard_snapshot_id"`
	PromptPackageArtifactID string     `json:"prompt_package_artifact_id"`
	ProviderID              string     `json:"provider_id"`
	ProfileVersion          string     `json:"profile_version"`
	ProfileDigest           string     `json:"profile_digest"`
	Model                   string     `json:"model"`
	Mode                    string     `json:"mode"`
	AspectRatio             string     `json:"aspect_ratio"`
	DurationSeconds         int        `json:"duration_seconds"`
	InputArtifactRefs       []string   `json:"input_artifact_refs"`
	RuntimeJobRunID         string     `json:"runtime_job_run_id,omitempty"`
	RuntimeNodeRunID        string     `json:"runtime_node_run_id,omitempty"`
	RuntimeAttemptID        string     `json:"runtime_attempt_id,omitempty"`
	RuntimeEffectID         string     `json:"runtime_effect_id,omitempty"`
	State                   string     `json:"state"`
	IdempotencyKey          string     `json:"-"`
	EstimatedCostMinor      int64      `json:"estimated_cost_minor"`
	ActualCostMinor         int64      `json:"actual_cost_minor"`
	Currency                string     `json:"currency"`
	AttemptCount            int        `json:"attempt_count"`
	MaxAttempts             int        `json:"max_attempts"`
	LeaseOwner              string     `json:"lease_owner,omitempty"`
	LeaseExpiresAt          *time.Time `json:"lease_expires_at,omitempty"`
	CancelRequestedAt       *time.Time `json:"cancel_requested_at,omitempty"`
	ErrorCode               string     `json:"error_code,omitempty"`
	ErrorDetailSafe         string     `json:"error_detail_safe,omitempty"`
	RowVersion              int        `json:"row_version"`
	CreatedBy               string     `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func (v *MediaGenerationJob) NormalizeCollections() {
	if v.InputArtifactRefs == nil {
		v.InputArtifactRefs = []string{}
	}
}

func (v MediaGenerationJob) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.StageRunID == "" || v.StoryboardSnapshotID == "" || v.ProviderID == "" || v.ProfileVersion == "" || v.Model == "" || v.Mode == "" {
		return fault.Invalid("MEDIA_JOB_INVALID", "媒体 Job 缺少任务、分镜或 Provider 配置")
	}
	if !stablehash.Valid(v.ProfileDigest) {
		return fault.Invalid("MEDIA_JOB_PROFILE_DIGEST_INVALID", "媒体 Job 必须固定 Provider Profile digest")
	}
	if (v.RuntimeJobRunID == "") != (v.RuntimeNodeRunID == "") {
		return fault.Invalid("MEDIA_JOB_RUNTIME_SCOPE_INVALID", "媒体 Job 的 Runtime 关联必须同时包含 JobRun 和 NodeRun")
	}
	if v.RuntimeEffectID != "" && v.RuntimeJobRunID == "" {
		return fault.Invalid("MEDIA_JOB_RUNTIME_EFFECT_INVALID", "媒体 Job 不能在缺少 Runtime 作用域时绑定 ExternalEffect")
	}
	if v.DurationSeconds < 1 || v.MaxAttempts < 1 || v.AttemptCount < 0 || v.AttemptCount > v.MaxAttempts || v.EstimatedCostMinor < 0 || v.ActualCostMinor < 0 || strings.TrimSpace(v.Currency) == "" || v.RowVersion < 1 {
		return fault.Invalid("MEDIA_JOB_LIMIT_INVALID", "媒体 Job 的时长、费用、版本或重试边界无效")
	}
	if !ValidMediaJobState(v.State) {
		return fault.Invalid("MEDIA_JOB_STATE_INVALID", "媒体 Job 状态无效")
	}
	return nil
}

func ValidMediaJobState(value string) bool {
	switch value {
	case MediaJobDraft, MediaJobAwaitingCostApproval, MediaJobQueued, MediaJobSubmitting, MediaJobSubmitted,
		MediaJobGenerating, MediaJobDownloading, MediaJobValidating, MediaJobSucceeded, MediaJobRetryWait,
		MediaJobRetryableFailed, MediaJobOutputInvalid, MediaJobFailed, MediaJobCancelled, MediaJobBudgetBlocked,
		MediaJobAwaitingExternal:
		return true
	default:
		return false
	}
}

type ProviderAttempt struct {
	ID                  string         `json:"id"`
	TenantID            string         `json:"tenant_id"`
	ProjectID           string         `json:"project_id"`
	GenerationJobID     string         `json:"generation_job_id"`
	AttemptNumber       int            `json:"attempt_number"`
	ProviderID          string         `json:"provider_id"`
	RequestDigest       string         `json:"request_digest"`
	RuntimeJobRunID     string         `json:"runtime_job_run_id,omitempty"`
	RuntimeNodeRunID    string         `json:"runtime_node_run_id,omitempty"`
	RuntimeAttemptID    string         `json:"runtime_attempt_id,omitempty"`
	RuntimeEffectID     string         `json:"runtime_effect_id,omitempty"`
	ExternalJobID       string         `json:"external_job_id,omitempty"`
	ProviderState       string         `json:"provider_state"`
	SafeRequestSummary  map[string]any `json:"safe_request_summary"`
	SafeResponseSummary map[string]any `json:"safe_response_summary"`
	DisclosureManifest  map[string]any `json:"disclosure_manifest"`
	HTTPStatus          int            `json:"http_status,omitempty"`
	ProviderRequestID   string         `json:"provider_request_id,omitempty"`
	EstimatedCostMinor  int64          `json:"estimated_cost_minor"`
	ActualCostMinor     int64          `json:"actual_cost_minor"`
	Currency            string         `json:"currency"`
	LastPolledAt        *time.Time     `json:"last_polled_at,omitempty"`
	NextPollAt          *time.Time     `json:"next_poll_at,omitempty"`
	SubmittedAt         *time.Time     `json:"submitted_at,omitempty"`
	DownloadedAt        *time.Time     `json:"downloaded_at,omitempty"`
	CompletedAt         *time.Time     `json:"completed_at,omitempty"`
	RetryAfterSeconds   int            `json:"retry_after_seconds,omitempty"`
	ErrorCode           string         `json:"error_code,omitempty"`
	ErrorDetailSafe     string         `json:"error_detail_safe,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

func (v *ProviderAttempt) NormalizeCollections() {
	if v.SafeRequestSummary == nil {
		v.SafeRequestSummary = map[string]any{}
	}
	if v.SafeResponseSummary == nil {
		v.SafeResponseSummary = map[string]any{}
	}
	if v.DisclosureManifest == nil {
		v.DisclosureManifest = map[string]any{}
	}
}

func (v ProviderAttempt) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.GenerationJobID == "" || v.AttemptNumber < 1 || v.ProviderID == "" || !stablehash.Valid(v.RequestDigest) || v.ProviderState == "" || v.EstimatedCostMinor < 0 || v.ActualCostMinor < 0 || v.Currency == "" {
		return fault.Invalid("PROVIDER_ATTEMPT_INVALID", "Provider Attempt 缺少 Job、请求摘要、费用或状态")
	}
	if (v.RuntimeJobRunID == "") != (v.RuntimeNodeRunID == "") {
		return fault.Invalid("PROVIDER_ATTEMPT_RUNTIME_SCOPE_INVALID", "Provider Attempt 的 Runtime 关联必须同时包含 JobRun 和 NodeRun")
	}
	if v.RuntimeEffectID != "" && v.RuntimeJobRunID == "" {
		return fault.Invalid("PROVIDER_ATTEMPT_RUNTIME_EFFECT_INVALID", "Provider Attempt 不能在缺少 Runtime 作用域时绑定 ExternalEffect")
	}
	return nil
}

const (
	MediaReviewTechnical = "technical"
	MediaReviewContent   = "content"
	MediaReviewFinal     = "final"
	MediaReviewPending   = "pending"
	MediaReviewApproved  = "approved"
	MediaReviewChanges   = "changes_requested"
	MediaReviewRejected  = "rejected"
)

type MediaReview struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenant_id"`
	ProjectID         string         `json:"project_id"`
	TaskID            string         `json:"task_id"`
	GenerationJobID   string         `json:"generation_job_id,omitempty"`
	SubjectArtifactID string         `json:"subject_artifact_id"`
	SubjectDigest     string         `json:"subject_digest"`
	ReviewKind        string         `json:"review_kind"`
	Status            string         `json:"status"`
	Checks            map[string]any `json:"checks"`
	Selected          bool           `json:"selected"`
	DecisionReason    string         `json:"decision_reason,omitempty"`
	DecidedBy         string         `json:"decided_by,omitempty"`
	DecidedAt         *time.Time     `json:"decided_at,omitempty"`
	RowVersion        int            `json:"row_version"`
	CreatedBy         string         `json:"created_by"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (v *MediaReview) NormalizeCollections() {
	if v.Checks == nil {
		v.Checks = map[string]any{}
	}
}

func (v MediaReview) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.SubjectArtifactID == "" || !stablehash.Valid(v.SubjectDigest) || v.RowVersion < 1 {
		return fault.Invalid("MEDIA_REVIEW_INVALID", "媒体审核缺少任务、Artifact、摘要或版本")
	}
	switch v.ReviewKind {
	case MediaReviewTechnical, MediaReviewContent, MediaReviewFinal:
	default:
		return fault.Invalid("MEDIA_REVIEW_KIND_INVALID", "媒体审核类型无效")
	}
	switch v.Status {
	case MediaReviewPending, MediaReviewApproved, MediaReviewChanges, MediaReviewRejected:
	default:
		return fault.Invalid("MEDIA_REVIEW_STATUS_INVALID", "媒体审核状态无效")
	}
	return nil
}

type ProviderProfile struct {
	ProviderID      string         `json:"provider_id"`
	Version         string         `json:"version"`
	Digest          string         `json:"digest"`
	AdapterVersion  string         `json:"adapter_version"`
	Model           string         `json:"model"`
	Region          string         `json:"region"`
	Modes           []string       `json:"modes"`
	InputMediaTypes []string       `json:"input_media_types"`
	OutputMediaType string         `json:"output_media_type"`
	Limits          map[string]any `json:"limits"`
	DataRetention   string         `json:"data_retention"`
	Pricing         map[string]any `json:"pricing"`
	Status          string         `json:"status"`
	VerifiedAt      time.Time      `json:"verified_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (v *ProviderProfile) NormalizeCollections() {
	v.Modes = normalizeStrings(v.Modes)
	v.InputMediaTypes = normalizeStrings(v.InputMediaTypes)
	if v.Limits == nil {
		v.Limits = map[string]any{}
	}
	if v.Pricing == nil {
		v.Pricing = map[string]any{}
	}
}

func (v ProviderProfile) Validate() error {
	if v.ProviderID == "" || v.Version == "" || !stablehash.Valid(v.Digest) || v.AdapterVersion == "" || v.Model == "" || v.Region == "" || v.OutputMediaType == "" || v.DataRetention == "" {
		return fault.Invalid("PROVIDER_PROFILE_INVALID", "Provider Profile 缺少版本、摘要、模型或数据策略")
	}
	if len(v.Modes) == 0 || !v.ExpiresAt.After(v.VerifiedAt) {
		return fault.Invalid("PROVIDER_PROFILE_LIMIT_INVALID", "Provider Profile 必须声明模式和有效期")
	}
	switch v.Status {
	case "draft", "published", "withdrawn":
	default:
		return fault.Invalid("PROVIDER_PROFILE_STATUS_INVALID", "Provider Profile 状态无效")
	}
	return nil
}

type ProviderBinding struct {
	TenantID           string    `json:"tenant_id"`
	ProviderID         string    `json:"provider_id"`
	ProfileVersion     string    `json:"profile_version"`
	State              string    `json:"state"`
	CredentialRef      string    `json:"-"`
	EgressPolicy       string    `json:"egress_policy"`
	MonthlyBudgetMinor int64     `json:"monthly_budget_minor"`
	MaxJobCostMinor    int64     `json:"max_job_cost_minor"`
	MaxConcurrency     int       `json:"max_concurrency"`
	MaxRetries         int       `json:"max_retries"`
	UpdatedBy          string    `json:"updated_by"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (v ProviderBinding) Validate() error {
	if v.TenantID == "" || v.ProviderID == "" || v.ProfileVersion == "" || v.EgressPolicy == "" || v.MonthlyBudgetMinor < 0 || v.MaxJobCostMinor < 0 || v.MaxConcurrency < 1 || v.MaxRetries < 0 {
		return fault.Invalid("PROVIDER_BINDING_INVALID", "Provider Binding 缺少配置或预算边界无效")
	}
	switch v.State {
	case "active", "disabled", "misconfigured", "budget_blocked":
	default:
		return fault.Invalid("PROVIDER_BINDING_STATE_INVALID", "Provider Binding 状态无效")
	}
	return nil
}

func CanTransitionMediaJob(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string]map[string]bool{
		MediaJobDraft:                {MediaJobAwaitingCostApproval: true, MediaJobQueued: true, MediaJobBudgetBlocked: true, MediaJobAwaitingExternal: true, MediaJobCancelled: true},
		MediaJobAwaitingCostApproval: {MediaJobQueued: true, MediaJobBudgetBlocked: true, MediaJobCancelled: true},
		MediaJobBudgetBlocked:        {MediaJobAwaitingCostApproval: true, MediaJobQueued: true, MediaJobCancelled: true},
		MediaJobQueued:               {MediaJobSubmitting: true, MediaJobCancelled: true},
		MediaJobSubmitting:           {MediaJobSubmitted: true, MediaJobRetryWait: true, MediaJobRetryableFailed: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobSubmitted:            {MediaJobGenerating: true, MediaJobDownloading: true, MediaJobAwaitingExternal: true, MediaJobRetryWait: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobGenerating:           {MediaJobDownloading: true, MediaJobAwaitingExternal: true, MediaJobRetryWait: true, MediaJobOutputInvalid: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobDownloading:          {MediaJobValidating: true, MediaJobRetryWait: true, MediaJobOutputInvalid: true, MediaJobFailed: true},
		MediaJobValidating:           {MediaJobSucceeded: true, MediaJobOutputInvalid: true, MediaJobRetryWait: true, MediaJobFailed: true},
		MediaJobRetryWait:            {MediaJobQueued: true, MediaJobRetryableFailed: true, MediaJobCancelled: true},
		MediaJobRetryableFailed:      {MediaJobQueued: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobOutputInvalid:        {MediaJobQueued: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobAwaitingExternal:     {MediaJobGenerating: true, MediaJobDownloading: true, MediaJobCancelled: true, MediaJobFailed: true},
	}
	return allowed[from][to]
}

type ModelGenerationReceipt struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectID      string    `json:"project_id"`
	TaskID         string    `json:"task_id"`
	TaskRevisionID string    `json:"task_revision_id"`
	ProviderID     string    `json:"provider_id"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	RequestID      string    `json:"request_id,omitempty"`
	RequestDigest  string    `json:"request_digest"`
	ResponseDigest string    `json:"response_digest"`
	InputTokens    int64     `json:"input_tokens"`
	OutputTokens   int64     `json:"output_tokens"`
	TotalTokens    int64     `json:"total_tokens"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

func (v ModelGenerationReceipt) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.TaskRevisionID == "" || v.ProviderID == "" || v.Provider == "" || v.Model == "" {
		return fault.Invalid("MODEL_GENERATION_RECEIPT_INVALID", "模型生成回执缺少任务、候选修订或 Provider")
	}
	if !stablehash.Valid(v.RequestDigest) || !stablehash.Valid(v.ResponseDigest) || v.InputTokens < 0 || v.OutputTokens < 0 || v.TotalTokens < 0 {
		return fault.Invalid("MODEL_GENERATION_RECEIPT_USAGE_INVALID", "模型生成回执摘要或用量无效")
	}
	return nil
}

const (
	TaskDeliveryReady     = "ready"
	TaskDeliveryDelivered = "delivered"
	TaskDeliveryFailed    = "failed"
	TaskDeliveryCancelled = "cancelled"
)

type TaskDelivery struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	ProjectID         string     `json:"project_id"`
	TaskID            string     `json:"task_id"`
	RevisionID        string     `json:"revision_id"`
	Destination       string     `json:"destination"`
	Status            string     `json:"status"`
	Manifest          []string   `json:"manifest"`
	DeliveryPackageID string     `json:"delivery_package_id,omitempty"`
	IntegrityStatus   string     `json:"integrity_status"`
	DeliveryDigest    string     `json:"delivery_digest"`
	DeliveredBy       string     `json:"delivered_by,omitempty"`
	DeliveredAt       *time.Time `json:"delivered_at,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (v *TaskDelivery) NormalizeCollections() {
	if v.Manifest == nil {
		v.Manifest = []string{}
	}
}

func (v TaskDelivery) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.RevisionID == "" || strings.TrimSpace(v.Destination) == "" {
		return fault.Invalid("TASK_DELIVERY_INVALID", "交付缺少任务、Revision 或目的地")
	}
	switch v.Status {
	case TaskDeliveryReady, TaskDeliveryDelivered, TaskDeliveryFailed, TaskDeliveryCancelled:
	default:
		return fault.Invalid("TASK_DELIVERY_STATUS_INVALID", "交付状态无效")
	}
	if v.DeliveryDigest != "" && !stablehash.Valid(v.DeliveryDigest) {
		return fault.Invalid("TASK_DELIVERY_HASH_INVALID", "交付摘要必须是 sha256 摘要")
	}
	if v.Status == TaskDeliveryDelivered && (v.DeliveryPackageID == "" || len(v.Manifest) == 0 || v.IntegrityStatus != "complete") {
		return fault.Invalid("TASK_DELIVERY_INCOMPLETE", "已交付记录必须引用完整 DeliveryPackage 和非空 manifest")
	}
	return nil
}
