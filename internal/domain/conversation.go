package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	ConversationImportAwaitingConfirmation = "awaiting_client_confirmation"
	ConversationImportUploaded             = "uploaded"
	ConversationImportCancelled            = "cancelled"
	ConversationImportExpired              = "expired"
	ConversationImportRejected             = "rejected"

	ConversationScopeSummary        = "summary"
	ConversationScopeSelectedTurns  = "selected_turns"
	ConversationScopeFullTranscript = "full_transcript"

	ConversationAttachTaskInput         = "task_input"
	ConversationAttachEvidenceCandidate = "evidence_candidate"

	ConversationBundleSchema = "contentcloud.conversation_bundle/1.0"
)

// ConversationImport is a server-side export request. It intentionally does
// not contain local paths, private client transcript formats, or content until
// a client submits a validated ConversationBundle.
type ConversationImport struct {
	ID             string              `json:"import_id"`
	TenantID       string              `json:"tenant_id"`
	ProjectID      string              `json:"project_id"`
	TaskID         string              `json:"task_id"`
	StageRunID     string              `json:"stage_run_id,omitempty"`
	NodeID         string              `json:"node_id,omitempty"`
	ClientID       string              `json:"client_id"`
	AdapterVersion string              `json:"adapter_version"`
	AdapterID      string              `json:"adapter_id"`
	Purpose        string              `json:"purpose"`
	RequestedScope string              `json:"requested_scope"`
	AttachAs       string              `json:"attach_as"`
	RetentionDays  int                 `json:"retention_days"`
	Status         string              `json:"status"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	Bundle         *ConversationBundle `json:"bundle,omitempty"`
	ExpiresAt      time.Time           `json:"expires_at"`
	CreatedBy      string              `json:"created_by"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	CancelledAt    *time.Time          `json:"cancelled_at,omitempty"`
	UploadedAt     *time.Time          `json:"uploaded_at,omitempty"`
}

type ConversationClient struct {
	ID             string `json:"id"`
	ClientVersion  string `json:"client_version"`
	AdapterVersion string `json:"adapter_version"`
	NodeID         string `json:"node_id,omitempty"`
}

type ConversationSource struct {
	Format     string `json:"format"`
	SessionRef string `json:"session_ref"`
}

type ConversationScope struct {
	Mode          string `json:"mode"`
	SelectedCount int    `json:"selected_count,omitempty"`
}

type ConversationTarget struct {
	TaskID     string `json:"task_id"`
	StageRunID string `json:"stage_run_id,omitempty"`
}

type ConversationContent struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type ConversationRedaction struct {
	Applied      bool     `json:"applied"`
	PolicyDigest string   `json:"policy_digest"`
	RemovedTypes []string `json:"removed_types"`
}

type ConversationConsent struct {
	FullTranscript bool      `json:"full_transcript"`
	ConfirmedAt    time.Time `json:"confirmed_at"`
}

type ConversationBundle struct {
	SchemaVersion string                `json:"schema_version"`
	BundleID      string                `json:"bundle_id"`
	ImportID      string                `json:"import_id"`
	Client        ConversationClient    `json:"client"`
	Source        ConversationSource    `json:"source"`
	Purpose       string                `json:"purpose"`
	Scope         ConversationScope     `json:"scope"`
	Target        ConversationTarget    `json:"target"`
	Content       []ConversationContent `json:"content"`
	Redaction     ConversationRedaction `json:"redaction"`
	Consent       ConversationConsent   `json:"consent"`
	ContentDigest string                `json:"content_digest"`
	ExportedAt    time.Time             `json:"exported_at"`
}

func (v *ConversationImport) NormalizeCollections() {
	if v.Bundle != nil {
		v.Bundle.NormalizeCollections()
	}
}

func (v *ConversationBundle) NormalizeCollections() {
	if v.Content == nil {
		v.Content = []ConversationContent{}
	}
	if v.Redaction.RemovedTypes == nil {
		v.Redaction.RemovedTypes = []string{}
	}
}

func (v ConversationImport) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.TaskID == "" || v.ClientID == "" || v.AdapterVersion == "" || v.AdapterID == "" {
		return Invalid("CONVERSATION_IMPORT_INVALID", "对话导出请求缺少任务、客户端或 Adapter 信息")
	}
	if v.Status != ConversationImportAwaitingConfirmation && v.Status != ConversationImportUploaded && v.Status != ConversationImportCancelled && v.Status != ConversationImportExpired && v.Status != ConversationImportRejected {
		return Invalid("CONVERSATION_IMPORT_STATUS_INVALID", "对话导入状态无效")
	}
	if v.ExpiresAt.IsZero() {
		return Invalid("CONVERSATION_IMPORT_EXPIRY_REQUIRED", "对话导入请求必须有过期时间")
	}
	if v.RetentionDays < 1 || v.RetentionDays > 90 {
		return Invalid("CONVERSATION_IMPORT_RETENTION_INVALID", "对话导入保留天数必须在 1 到 90 天之间")
	}
	switch v.Purpose {
	case "task_handoff", "failure_diagnosis", "evidence_candidate", "retrospective":
	default:
		return Invalid("CONVERSATION_IMPORT_PURPOSE_INVALID", "对话导入用途无效")
	}
	switch v.RequestedScope {
	case ConversationScopeSummary, ConversationScopeSelectedTurns, ConversationScopeFullTranscript:
	default:
		return Invalid("CONVERSATION_IMPORT_SCOPE_INVALID", "对话导出范围无效")
	}
	switch v.AttachAs {
	case ConversationAttachTaskInput, ConversationAttachEvidenceCandidate:
	default:
		return Invalid("CONVERSATION_IMPORT_ATTACH_INVALID", "对话导入只能绑定为任务输入或 Evidence 候选")
	}
	if len(v.IdempotencyKey) > 128 {
		return Invalid("IDEMPOTENCY_KEY_INVALID", "幂等键长度不能超过 128 个字符")
	}
	return nil
}

func (v ConversationBundle) ValidateAgainst(imported ConversationImport, now time.Time) error {
	v.NormalizeCollections()
	if v.SchemaVersion != ConversationBundleSchema || v.BundleID == "" || v.ImportID != imported.ID {
		return Invalid("CONVERSATION_BUNDLE_INVALID", "Bundle Schema 或导入请求标识无效")
	}
	if imported.Status != ConversationImportAwaitingConfirmation {
		return Conflict("CONVERSATION_IMPORT_NOT_AWAITING_BUNDLE", "对话导入请求不在等待客户端上传状态")
	}
	if !now.Before(imported.ExpiresAt) {
		return Conflict("CONVERSATION_IMPORT_EXPIRED", "对话导入请求已过期，请重新创建导出请求")
	}
	if v.Client.ID != imported.ClientID || v.Client.AdapterVersion != imported.AdapterVersion || v.Client.NodeID != imported.NodeID {
		return Invalid("CONVERSATION_BUNDLE_ADAPTER_MISMATCH", "Bundle 的客户端、Adapter 或节点与导入请求不一致")
	}
	if strings.TrimSpace(v.Client.ClientVersion) == "" || strings.TrimSpace(v.Source.Format) == "" || !validOpaqueSessionRef(v.Source.SessionRef) {
		return Invalid("CONVERSATION_BUNDLE_SOURCE_INVALID", "Bundle 来源必须包含客户端版本、格式和不可逆 session_ref")
	}
	if v.Purpose != imported.Purpose || v.Scope.Mode != imported.RequestedScope || v.Target.TaskID != imported.TaskID || v.Target.StageRunID != imported.StageRunID {
		return Invalid("CONVERSATION_BUNDLE_SCOPE_INVALID", "Bundle 用途、范围或任务作用域与导入请求不一致")
	}
	if len(v.Content) == 0 || len(v.Content) > 2000 {
		return Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", "Bundle 必须包含 1 到 2000 条经过客户端筛选的内容")
	}
	if v.Scope.Mode == ConversationScopeSelectedTurns && v.Scope.SelectedCount != len(v.Content) {
		return Invalid("CONVERSATION_BUNDLE_SCOPE_INVALID", "selected_turns 的数量必须与 Bundle 内容一致")
	}
	if v.Scope.Mode == ConversationScopeFullTranscript && !v.Consent.FullTranscript {
		return Policy("CONVERSATION_FULL_TRANSCRIPT_CONSENT_REQUIRED", "完整 Transcript 必须有明确授权", "在客户端预览并再次确认完整导出")
	}
	if !v.Redaction.Applied || !validSHA256Digest(v.Redaction.PolicyDigest) {
		return Invalid("CONVERSATION_BUNDLE_REDACTION_INVALID", "Bundle 必须声明已脱敏并携带有效脱敏策略摘要")
	}
	if v.Consent.ConfirmedAt.IsZero() || v.Consent.ConfirmedAt.After(v.ExportedAt) || v.ExportedAt.IsZero() {
		return Invalid("CONVERSATION_BUNDLE_CONSENT_INVALID", "Bundle 的确认和导出时间无效")
	}
	for _, item := range v.Content {
		if item.Kind != "summary" && item.Kind != "turn" && item.Kind != "decision" && item.Kind != "evidence_candidate" {
			return Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", fmt.Sprintf("不支持的 Bundle 内容类型: %s", item.Kind))
		}
		text := strings.TrimSpace(item.Text)
		if text == "" || len(text) > 100000 {
			return Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", "Bundle 内容不能为空或超过单项大小限制")
		}
		if containsPrivateRuntimeData(text) {
			return Invalid("CONVERSATION_BUNDLE_DISCLOSURE_INVALID", "Bundle 内容包含本机路径、凭据或可执行运行时信息")
		}
	}
	digest, err := CanonicalHash(v.Content)
	if err != nil || v.ContentDigest != "sha256:"+digest {
		return Invalid("CONVERSATION_BUNDLE_DIGEST_INVALID", "Bundle content_digest 与内容不一致")
	}
	return nil
}

func validOpaqueSessionRef(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "hmac:") && len(trimmed) >= len("hmac:")+16 && !strings.ContainsAny(trimmed, `/\\`) && !strings.Contains(trimmed, "://")
}

func containsPrivateRuntimeData(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"/users/", "/home/", `c:\\`, "bearer ", "sk-", "api_key", "access_token", "private_key", "codex_session", "claude_session"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
