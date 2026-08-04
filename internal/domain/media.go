package domain

import (
	"strings"
	"time"
)

const (
	StageOutputSourceRevision     = "source_revision"
	StageOutputEvidenceSet        = "evidence_set"
	StageOutputKnowledgeObject    = "knowledge_object"
	StageOutputKnowledgeSnapshot  = "knowledge_snapshot"
	StageOutputSubmissionRevision = "submission_revision"
	StageOutputApprovedSnapshot   = "approved_snapshot"
	StageOutputStoryboardPackage  = "storyboard_package"
	StageOutputArtifact           = "artifact"
	StageOutputGenerationJob      = "generation_job"
	StageOutputMediaReview        = "media_review"
	StageOutputDeliveryPackage    = "delivery_package"

	StageOutputRolePrimary      = "primary"
	StageOutputRoleSupporting   = "supporting"
	StageOutputRolePreview      = "preview"
	StageOutputRoleSelectedTake = "selected_take"
	StageOutputRoleFinal        = "final"

	StageOutputStatusCandidate = "candidate"
	StageOutputStatusValidated = "validated"
	StageOutputStatusApproved  = "approved"
	StageOutputStatusBlocked   = "blocked"
	StageOutputStatusFailed    = "failed"

	StageCompletionAllRequired = "all_required"
	StageCompletionAtLeastOne  = "at_least_one"
	StageCompletionControlOnly = "control_only"
)

var validStageOutputTypes = map[string]struct{}{
	StageOutputSourceRevision: {}, StageOutputEvidenceSet: {}, StageOutputKnowledgeObject: {},
	StageOutputKnowledgeSnapshot: {}, StageOutputSubmissionRevision: {}, StageOutputApprovedSnapshot: {},
	StageOutputStoryboardPackage: {}, StageOutputArtifact: {}, StageOutputGenerationJob: {},
	StageOutputMediaReview: {}, StageOutputDeliveryPackage: {},
}

type StageObjectRequirement struct {
	OutputType string `json:"output_type"`
	Role       string `json:"role,omitempty"`
	MinStatus  string `json:"min_status,omitempty"`
	MinCount   int    `json:"min_count,omitempty"`
}

type StageRetryPolicy struct {
	MaxAttempts        int      `json:"max_attempts,omitempty"`
	BackoffSeconds     int      `json:"backoff_seconds,omitempty"`
	AllowPartialRetry  bool     `json:"allow_partial_retry,omitempty"`
	RetryableErrorCode []string `json:"retryable_error_codes,omitempty"`
}

type StageCostPolicy struct {
	Currency                  string `json:"currency,omitempty"`
	MaxEstimatedCostMinor     int64  `json:"max_estimated_cost_minor,omitempty"`
	RequireApprovalAboveMinor int64  `json:"require_approval_above_minor,omitempty"`
	EstimateTTLSeconds        int    `json:"estimate_ttl_seconds,omitempty"`
}

type TaskStageOutput struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	ProjectID     string         `json:"project_id"`
	TaskID        string         `json:"task_id"`
	StageRunID    string         `json:"stage_run_id"`
	StageID       string         `json:"stage_id"`
	OutputType    string         `json:"output_type"`
	ObjectID      string         `json:"object_id"`
	ObjectVersion int            `json:"object_version,omitempty"`
	ObjectDigest  string         `json:"object_digest"`
	Role          string         `json:"role"`
	Status        string         `json:"status"`
	Metadata      map[string]any `json:"metadata"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (v *TaskStageOutput) NormalizeCollections() {
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
}

func (v TaskStageOutput) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.StageRunID == "" || v.StageID == "" || strings.TrimSpace(v.ObjectID) == "" {
		return Invalid("TASK_STAGE_OUTPUT_INVALID", "Stage 输出缺少任务、StageRun 或规范对象标识")
	}
	if _, ok := validStageOutputTypes[v.OutputType]; !ok {
		return Invalid("TASK_STAGE_OUTPUT_TYPE_INVALID", "Stage 输出类型不受支持")
	}
	switch v.Role {
	case StageOutputRolePrimary, StageOutputRoleSupporting, StageOutputRolePreview, StageOutputRoleSelectedTake, StageOutputRoleFinal:
	default:
		return Invalid("TASK_STAGE_OUTPUT_ROLE_INVALID", "Stage 输出角色无效")
	}
	switch v.Status {
	case StageOutputStatusCandidate, StageOutputStatusValidated, StageOutputStatusApproved, StageOutputStatusBlocked, StageOutputStatusFailed:
	default:
		return Invalid("TASK_STAGE_OUTPUT_STATUS_INVALID", "Stage 输出状态无效")
	}
	if !validSHA256Digest(v.ObjectDigest) {
		return Invalid("TASK_STAGE_OUTPUT_DIGEST_INVALID", "Stage 输出必须绑定 sha256 摘要")
	}
	return nil
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
		return Invalid("MEDIA_JOB_INVALID", "媒体 Job 缺少任务、分镜或 Provider 配置")
	}
	if !validSHA256Digest(v.ProfileDigest) {
		return Invalid("MEDIA_JOB_PROFILE_DIGEST_INVALID", "媒体 Job 必须固定 Provider Profile digest")
	}
	if v.DurationSeconds < 1 || v.MaxAttempts < 1 || v.AttemptCount < 0 || v.AttemptCount > v.MaxAttempts || v.EstimatedCostMinor < 0 || v.ActualCostMinor < 0 || strings.TrimSpace(v.Currency) == "" || v.RowVersion < 1 {
		return Invalid("MEDIA_JOB_LIMIT_INVALID", "媒体 Job 的时长、费用、版本或重试边界无效")
	}
	if !ValidMediaJobState(v.State) {
		return Invalid("MEDIA_JOB_STATE_INVALID", "媒体 Job 状态无效")
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
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.GenerationJobID == "" || v.AttemptNumber < 1 || v.ProviderID == "" || !validSHA256Digest(v.RequestDigest) || v.ProviderState == "" || v.EstimatedCostMinor < 0 || v.ActualCostMinor < 0 || v.Currency == "" {
		return Invalid("PROVIDER_ATTEMPT_INVALID", "Provider Attempt 缺少 Job、请求摘要、费用或状态")
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
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.SubjectArtifactID == "" || !validSHA256Digest(v.SubjectDigest) || v.RowVersion < 1 {
		return Invalid("MEDIA_REVIEW_INVALID", "媒体审核缺少任务、Artifact、摘要或版本")
	}
	switch v.ReviewKind {
	case MediaReviewTechnical, MediaReviewContent, MediaReviewFinal:
	default:
		return Invalid("MEDIA_REVIEW_KIND_INVALID", "媒体审核类型无效")
	}
	switch v.Status {
	case MediaReviewPending, MediaReviewApproved, MediaReviewChanges, MediaReviewRejected:
	default:
		return Invalid("MEDIA_REVIEW_STATUS_INVALID", "媒体审核状态无效")
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
	if v.ProviderID == "" || v.Version == "" || !validSHA256Digest(v.Digest) || v.AdapterVersion == "" || v.Model == "" || v.Region == "" || v.OutputMediaType == "" || v.DataRetention == "" {
		return Invalid("PROVIDER_PROFILE_INVALID", "Provider Profile 缺少版本、摘要、模型或数据策略")
	}
	if len(v.Modes) == 0 || !v.ExpiresAt.After(v.VerifiedAt) {
		return Invalid("PROVIDER_PROFILE_LIMIT_INVALID", "Provider Profile 必须声明模式和有效期")
	}
	switch v.Status {
	case "draft", "published", "withdrawn":
	default:
		return Invalid("PROVIDER_PROFILE_STATUS_INVALID", "Provider Profile 状态无效")
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
		return Invalid("PROVIDER_BINDING_INVALID", "Provider Binding 缺少配置或预算边界无效")
	}
	switch v.State {
	case "active", "disabled", "misconfigured", "budget_blocked":
	default:
		return Invalid("PROVIDER_BINDING_STATE_INVALID", "Provider Binding 状态无效")
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
		MediaJobSubmitted:            {MediaJobGenerating: true, MediaJobDownloading: true, MediaJobRetryWait: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobGenerating:           {MediaJobDownloading: true, MediaJobRetryWait: true, MediaJobOutputInvalid: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobDownloading:          {MediaJobValidating: true, MediaJobRetryWait: true, MediaJobOutputInvalid: true, MediaJobFailed: true},
		MediaJobValidating:           {MediaJobSucceeded: true, MediaJobOutputInvalid: true, MediaJobRetryWait: true, MediaJobFailed: true},
		MediaJobRetryWait:            {MediaJobQueued: true, MediaJobRetryableFailed: true, MediaJobCancelled: true},
		MediaJobRetryableFailed:      {MediaJobQueued: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobOutputInvalid:        {MediaJobQueued: true, MediaJobFailed: true, MediaJobCancelled: true},
		MediaJobAwaitingExternal:     {MediaJobDownloading: true, MediaJobCancelled: true, MediaJobFailed: true},
	}
	return allowed[from][to]
}
