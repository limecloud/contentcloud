package work

import "github.com/limecloud/contentcloud/internal/catalog"
import "regexp"
import "path"
import "sort"
import "github.com/limecloud/contentcloud/internal/platform/stablehash"
import "github.com/limecloud/contentcloud/internal/platform/fault"
import "time"
import "strings"
import "fmt"

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
		return fault.Invalid("CONVERSATION_IMPORT_INVALID", "对话导出请求缺少任务、客户端或 Adapter 信息")
	}
	if v.Status != ConversationImportAwaitingConfirmation && v.Status != ConversationImportUploaded && v.Status != ConversationImportCancelled && v.Status != ConversationImportExpired && v.Status != ConversationImportRejected {
		return fault.Invalid("CONVERSATION_IMPORT_STATUS_INVALID", "对话导入状态无效")
	}
	if v.ExpiresAt.IsZero() {
		return fault.Invalid("CONVERSATION_IMPORT_EXPIRY_REQUIRED", "对话导入请求必须有过期时间")
	}
	if v.RetentionDays < 1 || v.RetentionDays > 90 {
		return fault.Invalid("CONVERSATION_IMPORT_RETENTION_INVALID", "对话导入保留天数必须在 1 到 90 天之间")
	}
	switch v.Purpose {
	case "task_handoff", "failure_diagnosis", "evidence_candidate", "retrospective":
	default:
		return fault.Invalid("CONVERSATION_IMPORT_PURPOSE_INVALID", "对话导入用途无效")
	}
	switch v.RequestedScope {
	case ConversationScopeSummary, ConversationScopeSelectedTurns, ConversationScopeFullTranscript:
	default:
		return fault.Invalid("CONVERSATION_IMPORT_SCOPE_INVALID", "对话导出范围无效")
	}
	switch v.AttachAs {
	case ConversationAttachTaskInput, ConversationAttachEvidenceCandidate:
	default:
		return fault.Invalid("CONVERSATION_IMPORT_ATTACH_INVALID", "对话导入只能绑定为任务输入或 Evidence 候选")
	}
	if len(v.IdempotencyKey) > 128 {
		return fault.Invalid("IDEMPOTENCY_KEY_INVALID", "幂等键长度不能超过 128 个字符")
	}
	return nil
}

func (v ConversationBundle) ValidateAgainst(imported ConversationImport, now time.Time) error {
	v.NormalizeCollections()
	if v.SchemaVersion != ConversationBundleSchema || v.BundleID == "" || v.ImportID != imported.ID {
		return fault.Invalid("CONVERSATION_BUNDLE_INVALID", "Bundle Schema 或导入请求标识无效")
	}
	if imported.Status != ConversationImportAwaitingConfirmation {
		return fault.Conflict("CONVERSATION_IMPORT_NOT_AWAITING_BUNDLE", "对话导入请求不在等待客户端上传状态")
	}
	if !now.Before(imported.ExpiresAt) {
		return fault.Conflict("CONVERSATION_IMPORT_EXPIRED", "对话导入请求已过期，请重新创建导出请求")
	}
	if v.Client.ID != imported.ClientID || v.Client.AdapterVersion != imported.AdapterVersion || v.Client.NodeID != imported.NodeID {
		return fault.Invalid("CONVERSATION_BUNDLE_ADAPTER_MISMATCH", "Bundle 的客户端、Adapter 或节点与导入请求不一致")
	}
	if strings.TrimSpace(v.Client.ClientVersion) == "" || strings.TrimSpace(v.Source.Format) == "" || !validOpaqueSessionRef(v.Source.SessionRef) {
		return fault.Invalid("CONVERSATION_BUNDLE_SOURCE_INVALID", "Bundle 来源必须包含客户端版本、格式和不可逆 session_ref")
	}
	if v.Purpose != imported.Purpose || v.Scope.Mode != imported.RequestedScope || v.Target.TaskID != imported.TaskID || v.Target.StageRunID != imported.StageRunID {
		return fault.Invalid("CONVERSATION_BUNDLE_SCOPE_INVALID", "Bundle 用途、范围或任务作用域与导入请求不一致")
	}
	if len(v.Content) == 0 || len(v.Content) > 2000 {
		return fault.Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", "Bundle 必须包含 1 到 2000 条经过客户端筛选的内容")
	}
	if v.Scope.Mode == ConversationScopeSelectedTurns && v.Scope.SelectedCount != len(v.Content) {
		return fault.Invalid("CONVERSATION_BUNDLE_SCOPE_INVALID", "selected_turns 的数量必须与 Bundle 内容一致")
	}
	if v.Scope.Mode == ConversationScopeFullTranscript && !v.Consent.FullTranscript {
		return fault.Policy("CONVERSATION_FULL_TRANSCRIPT_CONSENT_REQUIRED", "完整 Transcript 必须有明确授权", "在客户端预览并再次确认完整导出")
	}
	if !v.Redaction.Applied || !stablehash.Valid(v.Redaction.PolicyDigest) {
		return fault.Invalid("CONVERSATION_BUNDLE_REDACTION_INVALID", "Bundle 必须声明已脱敏并携带有效脱敏策略摘要")
	}
	if v.Consent.ConfirmedAt.IsZero() || v.Consent.ConfirmedAt.After(v.ExportedAt) || v.ExportedAt.IsZero() {
		return fault.Invalid("CONVERSATION_BUNDLE_CONSENT_INVALID", "Bundle 的确认和导出时间无效")
	}
	for _, item := range v.Content {
		if item.Kind != "summary" && item.Kind != "turn" && item.Kind != "decision" && item.Kind != "evidence_candidate" {
			return fault.Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", fmt.Sprintf("不支持的 Bundle 内容类型: %s", item.Kind))
		}
		text := strings.TrimSpace(item.Text)
		if text == "" || len(text) > 100000 {
			return fault.Invalid("CONVERSATION_BUNDLE_CONTENT_INVALID", "Bundle 内容不能为空或超过单项大小限制")
		}
		if containsPrivateRuntimeData(text) {
			return fault.Invalid("CONVERSATION_BUNDLE_DISCLOSURE_INVALID", "Bundle 内容包含本机路径、凭据或可执行运行时信息")
		}
	}
	digest, err := stablehash.Sum(v.Content)
	if err != nil || v.ContentDigest != "sha256:"+digest {
		return fault.Invalid("CONVERSATION_BUNDLE_DIGEST_INVALID", "Bundle content_digest 与内容不一致")
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

const (
	DouyinCommerceProfileID        = "douyin-commerce-video"
	DouyinCommerceValidationSchema = "contentcloud.douyin-commerce-validation/1.0"
)

// DouyinCommerceOfferFacts is the normalized subset of an approved offer that
// is allowed to appear in the final creative and landing page.
type DouyinCommerceOfferFacts struct {
	SKUID            string   `json:"sku_id"`
	ProductVersionID string   `json:"product_version_id"`
	DisplayPrice     string   `json:"display_price"`
	Currency         string   `json:"currency"`
	Benefits         []string `json:"benefits"`
	Conditions       []string `json:"conditions"`
}

// DouyinCommerceValidationReceipt freezes the deterministic pre-publication
// checks. It is evidence for ChannelPublication, not a second publication or
// approval aggregate.
type DouyinCommerceValidationReceipt struct {
	SchemaVersion                      string                   `json:"schema_version"`
	ContentProfileID                   string                   `json:"content_profile_id"`
	ProjectID                          string                   `json:"project_id"`
	AudienceStrategyApprovedSnapshotID string                   `json:"audience_strategy_approved_snapshot_id"`
	AudienceStrategyVersionID          string                   `json:"audience_strategy_version_id"`
	OfferApprovedSnapshotID            string                   `json:"offer_approved_snapshot_id"`
	OfferSnapshotID                    string                   `json:"offer_snapshot_id"`
	ContentApprovedSnapshotID          string                   `json:"content_approved_snapshot_id"`
	ContentItemID                      string                   `json:"content_item_id"`
	ContentItemDigest                  string                   `json:"content_item_digest"`
	StoryboardApprovedSnapshotID       string                   `json:"storyboard_approved_snapshot_id"`
	StoryboardPackageID                string                   `json:"storyboard_package_id"`
	StoryboardLockedDigest             string                   `json:"storyboard_locked_digest"`
	RenderedCreativeArtifactID         string                   `json:"rendered_creative_artifact_id"`
	RenderedCreativeDigest             string                   `json:"rendered_creative_digest"`
	VoiceoverTextDigest                string                   `json:"voiceover_text_digest"`
	OnScreenTextDigest                 string                   `json:"on_screen_text_digest"`
	LandingPageTextDigest              string                   `json:"landing_page_text_digest"`
	Offer                              DouyinCommerceOfferFacts `json:"offer"`
	ObservedBenefits                   []string                 `json:"observed_benefits"`
	ObservedConditions                 []string                 `json:"observed_conditions"`
	AccountRef                         string                   `json:"account_ref"`
	ProductAnchorRef                   string                   `json:"product_anchor_ref"`
	LandingPageRef                     string                   `json:"landing_page_ref"`
	ScheduledAt                        time.Time                `json:"scheduled_at"`
	ValidatedAt                        time.Time                `json:"validated_at"`
	ReceiptDigest                      string                   `json:"receipt_digest"`
}

func (v DouyinCommerceValidationReceipt) Validate() error {
	if v.SchemaVersion != DouyinCommerceValidationSchema || v.ContentProfileID != DouyinCommerceProfileID || strings.TrimSpace(v.ProjectID) == "" {
		return fault.Invalid("DOUYIN_COMMERCE_RECEIPT_IDENTITY_INVALID", "抖音电商校验回执缺少正确 Schema、内容 Profile 或项目")
	}
	for _, value := range []string{
		v.AudienceStrategyApprovedSnapshotID, v.AudienceStrategyVersionID,
		v.OfferApprovedSnapshotID, v.OfferSnapshotID,
		v.ContentApprovedSnapshotID, v.ContentItemID,
		v.StoryboardApprovedSnapshotID, v.StoryboardPackageID,
		v.RenderedCreativeArtifactID, v.AccountRef, v.ProductAnchorRef, v.LandingPageRef,
		v.Offer.SKUID, v.Offer.ProductVersionID, v.Offer.DisplayPrice, v.Offer.Currency,
	} {
		if strings.TrimSpace(value) == "" {
			return fault.Invalid("DOUYIN_COMMERCE_RECEIPT_FIELD_REQUIRED", "抖音电商校验回执缺少固定的事实或发布引用")
		}
	}
	for _, digest := range []string{
		v.ContentItemDigest, v.StoryboardLockedDigest, v.RenderedCreativeDigest,
		v.VoiceoverTextDigest, v.OnScreenTextDigest, v.LandingPageTextDigest, v.ReceiptDigest,
	} {
		if !stablehash.Valid(digest) {
			return fault.Invalid("DOUYIN_COMMERCE_RECEIPT_DIGEST_INVALID", "抖音电商校验回执包含无效 SHA-256 摘要")
		}
	}
	if len(v.Offer.Currency) != 3 || !uniqueNonEmpty(v.Offer.Benefits) || !uniqueNonEmpty(v.Offer.Conditions) || !uniqueNonEmpty(v.ObservedBenefits) || !uniqueNonEmpty(v.ObservedConditions) {
		return fault.Invalid("DOUYIN_COMMERCE_RECEIPT_OFFER_INVALID", "抖音电商校验回执中的 Offer 事实为空、重复或币种无效")
	}
	if v.ScheduledAt.IsZero() || v.ValidatedAt.IsZero() || v.ValidatedAt.After(v.ScheduledAt) {
		return fault.Invalid("DOUYIN_COMMERCE_RECEIPT_TIME_INVALID", "抖音电商校验必须发生在计划发布时间之前")
	}
	digest, err := v.ComputedDigest()
	if err != nil {
		return err
	}
	if digest != v.ReceiptDigest {
		return fault.Conflict("DOUYIN_COMMERCE_RECEIPT_DIGEST_MISMATCH", "抖音电商校验回执摘要与内容不一致")
	}
	return nil
}

func (v DouyinCommerceValidationReceipt) ComputedDigest() (string, error) {
	v.ReceiptDigest = ""
	hash, err := stablehash.Sum(v)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func DouyinTextDigest(value string) (string, error) {
	hash, err := stablehash.Sum(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

// DouyinCommercePublicationRefs binds a validated creative to the existing
// ChannelPublication intent and its exact approved inputs.
type DouyinCommercePublicationRefs struct {
	AudienceStrategyApprovedSnapshotID string                          `json:"audience_strategy_approved_snapshot_id"`
	AudienceStrategyVersionID          string                          `json:"audience_strategy_version_id"`
	OfferApprovedSnapshotID            string                          `json:"offer_approved_snapshot_id"`
	OfferSnapshotID                    string                          `json:"offer_snapshot_id"`
	ContentApprovedSnapshotID          string                          `json:"content_approved_snapshot_id"`
	ContentItemID                      string                          `json:"content_item_id"`
	StoryboardApprovedSnapshotID       string                          `json:"storyboard_approved_snapshot_id"`
	StoryboardPackageID                string                          `json:"storyboard_package_id"`
	RenderedCreativeArtifactID         string                          `json:"rendered_creative_artifact_id"`
	RenderedCreativeDigest             string                          `json:"rendered_creative_digest"`
	ValidationReceiptDigest            string                          `json:"validation_receipt_digest"`
	AccountRef                         string                          `json:"account_ref"`
	ProductAnchorRef                   string                          `json:"product_anchor_ref"`
	LandingPageRef                     string                          `json:"landing_page_ref"`
	ValidationReceipt                  DouyinCommerceValidationReceipt `json:"validation_receipt"`
}

func (v DouyinCommercePublicationRefs) Validate() error {
	for _, value := range []string{
		v.AudienceStrategyApprovedSnapshotID, v.AudienceStrategyVersionID,
		v.OfferApprovedSnapshotID, v.OfferSnapshotID,
		v.ContentApprovedSnapshotID, v.ContentItemID,
		v.StoryboardApprovedSnapshotID, v.StoryboardPackageID,
		v.RenderedCreativeArtifactID, v.AccountRef, v.ProductAnchorRef, v.LandingPageRef,
	} {
		if strings.TrimSpace(value) == "" {
			return fault.Invalid("DOUYIN_COMMERCE_PUBLICATION_REF_REQUIRED", "抖音电商发布缺少批准快照、内容、资产、账号、商品锚点或落地页引用")
		}
	}
	if !stablehash.Valid(v.RenderedCreativeDigest) || !stablehash.Valid(v.ValidationReceiptDigest) {
		return fault.Invalid("DOUYIN_COMMERCE_PUBLICATION_DIGEST_INVALID", "抖音电商发布引用包含无效摘要")
	}
	if err := v.ValidationReceipt.Validate(); err != nil {
		return err
	}
	receipt := v.ValidationReceipt
	for _, pair := range [][2]string{
		{v.AudienceStrategyApprovedSnapshotID, receipt.AudienceStrategyApprovedSnapshotID},
		{v.AudienceStrategyVersionID, receipt.AudienceStrategyVersionID},
		{v.OfferApprovedSnapshotID, receipt.OfferApprovedSnapshotID},
		{v.OfferSnapshotID, receipt.OfferSnapshotID},
		{v.ContentApprovedSnapshotID, receipt.ContentApprovedSnapshotID},
		{v.ContentItemID, receipt.ContentItemID},
		{v.StoryboardApprovedSnapshotID, receipt.StoryboardApprovedSnapshotID},
		{v.StoryboardPackageID, receipt.StoryboardPackageID},
		{v.RenderedCreativeArtifactID, receipt.RenderedCreativeArtifactID},
		{v.AccountRef, receipt.AccountRef},
		{v.ProductAnchorRef, receipt.ProductAnchorRef},
		{v.LandingPageRef, receipt.LandingPageRef},
		{v.RenderedCreativeDigest, receipt.RenderedCreativeDigest},
	} {
		if strings.TrimSpace(pair[0]) != strings.TrimSpace(pair[1]) {
			return fault.Conflict("DOUYIN_COMMERCE_PUBLICATION_LINEAGE_MISMATCH", "发布引用与校验回执的对象、账号或成片摘要不一致")
		}
	}
	if v.ValidationReceiptDigest != v.ValidationReceipt.ReceiptDigest {
		return fault.Conflict("DOUYIN_COMMERCE_PUBLICATION_RECEIPT_MISMATCH", "发布引用的校验摘要与校验回执不一致")
	}
	return nil
}

const (
	InputItemUntriaged       = "untriaged"
	InputItemNeedsInfo       = "needs_info"
	InputItemRouted          = "routed"
	InputItemTaskCreated     = "task_created"
	InputItemTaskMerged      = "task_merged"
	InputItemProjectMaterial = "project_material"
	InputItemArchived        = "archived"
)

type InputItem struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ProjectID      string         `json:"project_id,omitempty"`
	SourceType     string         `json:"source_type"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	Body           string         `json:"body,omitempty"`
	SourceRef      string         `json:"source_ref,omitempty"`
	SourceDigest   string         `json:"source_digest,omitempty"`
	Disclosure     string         `json:"disclosure"`
	Status         string         `json:"status"`
	TargetTaskID   string         `json:"target_task_id,omitempty"`
	AssigneeUserID string         `json:"assignee_user_id,omitempty"`
	MissingFields  []string       `json:"missing_fields"`
	Metadata       map[string]any `json:"metadata"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	RowVersion     int            `json:"row_version"`
	CreatedBy      string         `json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (v *InputItem) NormalizeCollections() {
	if v.MissingFields == nil {
		v.MissingFields = []string{}
	}
	if v.Metadata == nil {
		v.Metadata = map[string]any{}
	}
}

func (v InputItem) Validate() error {
	if v.ID == "" || v.TenantID == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.SourceType) == "" {
		return fault.Invalid("INPUT_ITEM_INVALID", "输入收集记录缺少标题或来源类型")
	}
	if len(v.Title) > 300 || len(v.Summary) > 10000 || len(v.Body) > 500000 {
		return fault.Invalid("INPUT_ITEM_TOO_LARGE", "输入收集内容超过大小限制")
	}
	switch v.SourceType {
	case "brief", "manual_inspiration", "workspace_file", "comment", "external_request", "trigger", "conversation_bundle", "other":
	default:
		return fault.Invalid("INPUT_ITEM_SOURCE_INVALID", "输入收集来源类型无效")
	}
	switch v.Disclosure {
	case "project", "tenant", "restricted":
	default:
		return fault.Invalid("INPUT_ITEM_DISCLOSURE_INVALID", "输入收集披露范围无效")
	}
	switch v.Status {
	case InputItemUntriaged, InputItemNeedsInfo, InputItemRouted, InputItemTaskCreated, InputItemTaskMerged, InputItemProjectMaterial, InputItemArchived:
	default:
		return fault.Invalid("INPUT_ITEM_STATUS_INVALID", "输入收集状态无效")
	}
	if v.RowVersion < 1 {
		return fault.Invalid("INPUT_ITEM_VERSION_INVALID", "输入收集记录版本无效")
	}
	if len(v.IdempotencyKey) > 128 {
		return fault.Invalid("IDEMPOTENCY_KEY_INVALID", "幂等键长度不能超过 128 个字符")
	}
	return nil
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
		return fault.Invalid("TASK_STAGE_OUTPUT_INVALID", "Stage 输出缺少任务、StageRun 或规范对象标识")
	}
	if !catalog.ValidStageOutputType(v.OutputType) {
		return fault.Invalid("TASK_STAGE_OUTPUT_TYPE_INVALID", "Stage 输出类型不受支持")
	}
	switch v.Role {
	case catalog.StageOutputRolePrimary, catalog.StageOutputRoleSupporting, catalog.StageOutputRolePreview, catalog.StageOutputRoleSelectedTake, catalog.StageOutputRoleFinal:
	default:
		return fault.Invalid("TASK_STAGE_OUTPUT_ROLE_INVALID", "Stage 输出角色无效")
	}
	switch v.Status {
	case catalog.StageOutputStatusCandidate, catalog.StageOutputStatusValidated, catalog.StageOutputStatusApproved, catalog.StageOutputStatusBlocked, catalog.StageOutputStatusFailed:
	default:
		return fault.Invalid("TASK_STAGE_OUTPUT_STATUS_INVALID", "Stage 输出状态无效")
	}
	if !stablehash.Valid(v.ObjectDigest) {
		return fault.Invalid("TASK_STAGE_OUTPUT_DIGEST_INVALID", "Stage 输出必须绑定 sha256 摘要")
	}
	return nil
}

type RuntimeRun struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ProjectID         string    `json:"project_id"`
	WorkTaskID        string    `json:"work_task_id,omitempty"`
	SOPID             string    `json:"sop_id,omitempty"`
	SOPVersion        int       `json:"sop_version,omitempty"`
	SOPDigest         string    `json:"sop_digest,omitempty"`
	StageID           string    `json:"stage_id,omitempty"`
	ExecutionMode     string    `json:"execution_mode,omitempty"`
	ExecutorKind      string    `json:"executor_kind,omitempty"`
	OutputRefs        []string  `json:"output_refs,omitempty"`
	TaskRevisionID    string    `json:"task_revision_id,omitempty"`
	GateEvaluationID  string    `json:"gate_evaluation_id,omitempty"`
	InputSnapshotID   string    `json:"input_snapshot_id"`
	IdempotencyKey    string    `json:"idempotency_key"`
	TaskType          string    `json:"task_type"`
	CapabilityID      string    `json:"capability_id"`
	CapabilityVersion string    `json:"capability_version"`
	InputSchema       string    `json:"input_schema"`
	OutputSchema      string    `json:"output_schema"`
	OutputCount       int       `json:"output_count"`
	DeliveryProfiles  []string  `json:"delivery_profiles"`
	State             string    `json:"state"`
	Priority          int       `json:"priority"`
	AttemptCount      int       `json:"attempt_count"`
	ProgressLabel     string    `json:"progress_label,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type RuntimeRunEvent struct {
	Cursor     int64     `json:"cursor"`
	TenantID   string    `json:"-"`
	ProjectID  string    `json:"project_id"`
	RunID      string    `json:"run_id"`
	AttemptID  string    `json:"attempt_id"`
	DeviceID   string    `json:"device_id"`
	Sequence   int       `json:"sequence"`
	Phase      string    `json:"phase"`
	Step       int       `json:"step"`
	Label      string    `json:"label"`
	OccurredAt time.Time `json:"occurred_at"`
}

const (
	TaskStatusNeedsInput  = "needs_input"
	TaskStatusReady       = "ready"
	TaskStatusRunning     = "running"
	TaskStatusPaused      = "paused"
	TaskStatusWaitingGate = "waiting_gate"
	TaskStatusBlocked     = "blocked"
	TaskStatusAccepted    = "accepted"
	TaskStatusDelivered   = "delivered"
	TaskStatusCancelled   = "cancelled"

	StageRunStatusPending     = "pending"
	StageRunStatusRunning     = "running"
	StageRunStatusWaitingGate = "waiting_gate"
	StageRunStatusBlocked     = "blocked"
	StageRunStatusCompleted   = "completed"
	StageRunStatusCancelled   = "cancelled"
)

// WorkTask is the user-facing work object. Formal content facts remain in
// Revision, Evidence, Gate and Delivery records.
type WorkTask struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	ProjectID       string         `json:"project_id"`
	EnvironmentID   string         `json:"environment_id"`
	SOPID           string         `json:"sop_id"`
	SOPVersion      int            `json:"sop_version"`
	SOPDigest       string         `json:"sop_digest"`
	Title           string         `json:"title"`
	Intent          string         `json:"intent"`
	ContentType     string         `json:"content_type"`
	InputRefs       []string       `json:"input_refs"`
	RequestedOutput map[string]any `json:"requested_output"`
	AssigneeUserID  string         `json:"assignee_user_id,omitempty"`
	Priority        string         `json:"priority"`
	DueAt           *time.Time     `json:"due_at,omitempty"`
	RiskProfile     string         `json:"risk_profile"`
	IdempotencyKey  string         `json:"-"`
	Status          string         `json:"status"`
	CurrentStageID  string         `json:"current_stage_id"`
	NextAction      string         `json:"next_action"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type StageRun struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	TaskID        string            `json:"task_id"`
	StageID       string            `json:"stage_id"`
	Status        string            `json:"status"`
	ExecutionMode string            `json:"execution_mode"`
	InputRefs     []string          `json:"input_refs"`
	OutputRefs    []string          `json:"output_refs"`
	Outputs       []TaskStageOutput `json:"outputs"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

func (v WorkTask) Validate() error {
	if v.ID == "" || v.TenantID == "" || v.ProjectID == "" || v.SOPID == "" || v.SOPVersion < 1 || strings.TrimSpace(v.Title) == "" {
		return fault.Invalid("TASK_INVALID", "任务缺少项目、SOP 或标题")
	}
	if v.Priority == "" {
		return fault.Invalid("TASK_PRIORITY_REQUIRED", "任务优先级不能为空")
	}
	if v.SOPDigest != "" && !stablehash.Valid(v.SOPDigest) {
		return fault.Invalid("TASK_SOP_DIGEST_INVALID", "任务必须固定合法的 SOP digest")
	}
	if len(v.IdempotencyKey) > 128 {
		return fault.Invalid("IDEMPOTENCY_KEY_INVALID", "idempotency_key 不能超过 128 字符")
	}
	if v.Status != "" {
		switch v.Status {
		case TaskStatusNeedsInput, TaskStatusReady, TaskStatusRunning, TaskStatusPaused, TaskStatusWaitingGate, TaskStatusBlocked, TaskStatusAccepted, TaskStatusDelivered, TaskStatusCancelled:
		default:
			return fault.Invalid("TASK_STATUS_INVALID", "任务状态无效")
		}
	}
	return nil
}

var (
	seedanceReferencePattern = regexp.MustCompile(`@(图片|视频|音频)[1-9][0-9]*`)
	storyboardShotIDPattern  = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)
)

const (
	AudienceTaxonomySchema         = "contentcloud.audience-taxonomy/1.0"
	AudienceStrategySchema         = "contentcloud.audience-strategy/1.0"
	CommerceOfferSchema            = "contentcloud.commerce-offer/1.0"
	StoryboardPackageSchema        = "contentcloud.storyboard-package/1.0"
	SeedancePromptPackageSchema    = "contentcloud.seedance-prompt-package/1.0"
	PublishedCreativeBindingSchema = "contentcloud.published-creative-binding/1.0"
)

type AudienceSegment struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Definition string `json:"definition"`
}

type AudienceTaxonomySnapshot struct {
	ID                 string            `json:"id"`
	Type               string            `json:"type"`
	SchemaVersion      string            `json:"schema_version"`
	Provider           string            `json:"provider"`
	TaxonomyID         string            `json:"taxonomy_id"`
	TaxonomyVersion    string            `json:"taxonomy_version"`
	Segments           []AudienceSegment `json:"segments"`
	SourceURL          string            `json:"source_url"`
	CapturedAt         time.Time         `json:"captured_at"`
	EffectiveFrom      time.Time         `json:"effective_from"`
	ExpiresAt          time.Time         `json:"expires_at"`
	VerificationStatus string            `json:"verification_status"`
	SourceSHA256       string            `json:"source_sha256"`
	Status             string            `json:"status"`
}

func (v AudienceTaxonomySnapshot) Validate(now time.Time, requireReviewReady bool) error {
	if strings.TrimSpace(v.ID) == "" || v.Type != "audience_taxonomy_snapshot" || v.SchemaVersion != AudienceTaxonomySchema || strings.TrimSpace(v.Provider) == "" || strings.TrimSpace(v.TaxonomyID) == "" || strings.TrimSpace(v.TaxonomyVersion) == "" {
		return fault.Invalid("AUDIENCE_TAXONOMY_IDENTITY_INVALID", "人群目录缺少稳定 ID、类型、Schema、平台或版本")
	}
	if len(v.Segments) != 8 || !validAudienceSegments(v.Segments) {
		return fault.Invalid("AUDIENCE_TAXONOMY_SEGMENTS_INVALID", "八大人群目录必须包含恰好 8 个代码、名称和定义均有效且不重复的分群")
	}
	if strings.TrimSpace(v.SourceURL) == "" || v.CapturedAt.IsZero() || v.EffectiveFrom.IsZero() || v.ExpiresAt.IsZero() || !v.ExpiresAt.After(v.EffectiveFrom) || !stablehash.Matches(v.SourceSHA256) {
		return fault.Invalid("AUDIENCE_TAXONOMY_PROVENANCE_INVALID", "人群目录来源、采集时间、有效期或 SHA-256 无效")
	}
	if v.VerificationStatus != "unverified" && v.VerificationStatus != "human_verified" && v.VerificationStatus != "expired" {
		return fault.Invalid("AUDIENCE_TAXONOMY_VERIFICATION_INVALID", "人群目录 verification_status 无效")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "deprecated" {
		return fault.Invalid("AUDIENCE_TAXONOMY_STATUS_INVALID", "人群目录 status 无效")
	}
	if requireReviewReady && (v.Status != "review_ready" || v.VerificationStatus != "human_verified" || !now.Before(v.ExpiresAt)) {
		return fault.Policy("AUDIENCE_TAXONOMY_NOT_REVIEW_READY", "只有人工验证且未过期的人群目录可以发布审核", "更新来源和有效期，并将状态设为 review_ready")
	}
	return nil
}

func DefaultDouyinAudienceSegments() []AudienceSegment {
	return []AudienceSegment{
		{Code: "gen_z", Label: "Z世代", Definition: "用于探索年轻消费需求状态、内容表达和决策阻力，不代表个体属性推断"},
		{Code: "refined_mothers", Label: "精致妈妈", Definition: "用于探索家庭场景、效率、安全证据和自我需求，不假定固定家庭结构"},
		{Code: "emerging_white_collars", Label: "新锐白领", Definition: "用于探索通勤、工作节奏、品质升级和即时便利，不推断具体收入"},
		{Code: "senior_middle_class", Label: "资深中产", Definition: "用于探索品质、长期价值、可信证明和服务体验，不假定价格不敏感"},
		{Code: "urban_blue_collars", Label: "都市蓝领", Definition: "用于探索高频刚需、耐用、直观收益和购买门槛，禁止贬低性表达"},
		{Code: "small_town_youth", Label: "小镇青年", Definition: "用于探索本地生活、兴趣表达、实用性和可获得性，不以城市层级推断审美"},
		{Code: "urban_silver", Label: "都市银发", Definition: "用于探索易理解、易使用、信任和服务边界，不假定数字能力"},
		{Code: "small_town_middle_aged_elderly", Label: "小镇中老年", Definition: "用于探索熟悉场景、实用证明、售后与信任，禁止利用恐惧或信息差"},
	}
}

type AudienceStrategyVersion struct {
	ID                  string   `json:"id"`
	Type                string   `json:"type"`
	SchemaVersion       string   `json:"schema_version"`
	ProjectID           string   `json:"project_id"`
	TaxonomySnapshotID  string   `json:"taxonomy_snapshot_id"`
	AudienceCode        string   `json:"audience_code"`
	AudienceLabel       string   `json:"audience_label"`
	SegmentDefinition   string   `json:"segment_definition"`
	Objective           string   `json:"objective"`
	DemandMoment        string   `json:"demand_moment"`
	InsightStatement    string   `json:"insight_statement"`
	HookHypotheses      []string `json:"hook_hypotheses"`
	Scenario            string   `json:"scenario"`
	ProofOrder          []string `json:"proof_order"`
	Objections          []string `json:"objections"`
	CTAStrategy         string   `json:"cta_strategy"`
	EvidenceRefs        []string `json:"evidence_refs"`
	Confidence          string   `json:"confidence"`
	TestType            string   `json:"test_type"`
	PrimaryVariable     string   `json:"primary_variable"`
	ControlledVariables []string `json:"controlled_variables"`
	TargetMetrics       []string `json:"target_metrics"`
	Constraints         []string `json:"constraints"`
	Status              string   `json:"status"`
	BasedOnVersionID    string   `json:"based_on_version_id,omitempty"`
	ContentHash         string   `json:"content_hash,omitempty"`
}

func (v AudienceStrategyVersion) Validate(requireReviewReady bool) error {
	if strings.TrimSpace(v.ID) == "" || v.Type != "audience_strategy_version" || v.SchemaVersion != AudienceStrategySchema || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.TaxonomySnapshotID) == "" || strings.TrimSpace(v.AudienceCode) == "" || strings.TrimSpace(v.AudienceLabel) == "" {
		return fault.Invalid("AUDIENCE_STRATEGY_IDENTITY_INVALID", "人群策略缺少稳定 ID、Schema、项目或目录引用")
	}
	for _, value := range []string{v.SegmentDefinition, v.Objective, v.DemandMoment, v.InsightStatement, v.Scenario, v.CTAStrategy} {
		if strings.TrimSpace(value) == "" {
			return fault.Invalid("AUDIENCE_STRATEGY_FIELD_REQUIRED", "人群策略定义、目标、需求时刻、洞察、场景和 CTA 必填")
		}
	}
	if len(v.HookHypotheses) == 0 || len(v.ProofOrder) == 0 || len(v.TargetMetrics) == 0 || !uniqueNonEmpty(v.EvidenceRefs) || !uniqueNonEmpty(v.ControlledVariables) || !uniqueNonEmpty(v.TargetMetrics) {
		return fault.Invalid("AUDIENCE_STRATEGY_ARRAY_INVALID", "人群策略数组缺失、含空值或重复值")
	}
	if v.Confidence != "low" && v.Confidence != "medium" && v.Confidence != "high" {
		return fault.Invalid("AUDIENCE_STRATEGY_CONFIDENCE_INVALID", "confidence 只允许 low、medium 或 high")
	}
	if !validTestType(v.TestType) || !validExperimentVariable(v.PrimaryVariable) || containsValue(v.ControlledVariables, v.PrimaryVariable) {
		return fault.Invalid("AUDIENCE_STRATEGY_EXPERIMENT_INVALID", "测试类型、主变量或受控变量无效")
	}
	if v.TestType == "strict_ab" && len(v.ControlledVariables) == 0 {
		return fault.Invalid("AUDIENCE_STRATEGY_CONTROLS_REQUIRED", "strict_ab 必须明确受控变量")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "deprecated" {
		return fault.Invalid("AUDIENCE_STRATEGY_STATUS_INVALID", "人群策略 status 无效")
	}
	if requireReviewReady {
		if v.Status != "review_ready" {
			return fault.Policy("AUDIENCE_STRATEGY_NOT_REVIEW_READY", "只有 review_ready 人群策略可以发布审核", "补齐证据与策略字段后重试")
		}
		if len(v.EvidenceRefs) == 0 || v.Confidence == "low" {
			return fault.Policy("AUDIENCE_STRATEGY_EVIDENCE_INSUFFICIENT", "review_ready 人群策略必须有当前证据且置信度不能为 low", "补充项目或平台证据")
		}
	}
	return validateOptionalContentHash(v, v.ContentHash)
}

func (v AudienceStrategyVersion) ValidateAgainstTaxonomy(taxonomy AudienceTaxonomySnapshot, now time.Time) error {
	if v.TaxonomySnapshotID != taxonomy.ID {
		return fault.Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "AudienceStrategyVersion 未引用所提供的 taxonomy 基线")
	}
	if err := taxonomy.Validate(now, true); err != nil {
		return err
	}
	for _, segment := range taxonomy.Segments {
		if segment.Code != v.AudienceCode {
			continue
		}
		if segment.Label != v.AudienceLabel || segment.Definition != v.SegmentDefinition {
			break
		}
		return nil
	}
	return fault.Conflict("AUDIENCE_STRATEGY_TAXONOMY_MISMATCH", "AudienceStrategyVersion 的人群代码、名称或定义与批准 taxonomy 不一致")
}

type CommerceOfferSnapshot struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	SchemaVersion     string    `json:"schema_version"`
	ProjectID         string    `json:"project_id"`
	SKUID             string    `json:"sku_id"`
	ProductVersionID  string    `json:"product_version_id"`
	ApprovedClaimRefs []string  `json:"approved_claim_refs"`
	DisplayPrice      string    `json:"display_price"`
	Currency          string    `json:"currency"`
	Benefits          []string  `json:"benefits"`
	Conditions        []string  `json:"conditions"`
	EvidenceRefs      []string  `json:"evidence_refs"`
	CapturedAt        time.Time `json:"captured_at"`
	ValidFrom         time.Time `json:"valid_from"`
	ValidUntil        time.Time `json:"valid_until"`
	Status            string    `json:"status"`
}

func (v CommerceOfferSnapshot) Validate(at time.Time, requireVerified bool) error {
	if strings.TrimSpace(v.ID) == "" || v.Type != "commerce_offer_snapshot" || v.SchemaVersion != CommerceOfferSchema || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.SKUID) == "" || strings.TrimSpace(v.ProductVersionID) == "" || strings.TrimSpace(v.DisplayPrice) == "" || len(v.Currency) != 3 {
		return fault.Invalid("COMMERCE_OFFER_IDENTITY_INVALID", "Offer 缺少稳定 ID、Schema、商品版本、价格或币种")
	}
	if len(v.EvidenceRefs) == 0 || !uniqueNonEmpty(v.EvidenceRefs) || !uniqueNonEmpty(v.ApprovedClaimRefs) || v.CapturedAt.IsZero() || v.ValidFrom.IsZero() || v.ValidUntil.IsZero() || !v.ValidUntil.After(v.ValidFrom) {
		return fault.Invalid("COMMERCE_OFFER_PROVENANCE_INVALID", "Offer 必须包含有效证据和时间窗口")
	}
	if v.Status != "candidate" && v.Status != "verified" && v.Status != "expired" && v.Status != "revoked" {
		return fault.Invalid("COMMERCE_OFFER_STATUS_INVALID", "Offer status 无效")
	}
	if requireVerified && (v.Status != "verified" || at.Before(v.ValidFrom) || !at.Before(v.ValidUntil)) {
		return fault.Policy("COMMERCE_OFFER_NOT_VALID", "Offer 未验证、尚未生效或已过期", "更新并人工验证 OfferSnapshot")
	}
	return nil
}

type CapabilityRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (v CapabilityRef) Validate() error {
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.Version) == "" || !strings.HasPrefix(v.Digest, "sha256:") || !stablehash.Matches(v.Digest) {
		return fault.Invalid("CAPABILITY_REF_INVALID", "能力引用需要 ID、版本和带前缀的 SHA-256")
	}
	return nil
}

type StoryboardAsset struct {
	ID         string   `json:"id"`
	Role       string   `json:"role"`
	ShotID     string   `json:"shot_id,omitempty"`
	Path       string   `json:"path"`
	MediaType  string   `json:"media_type"`
	SHA256     string   `json:"sha256"`
	ByteSize   int64    `json:"byte_size"`
	RightsRefs []string `json:"rights_refs"`
}

type StoryboardShot struct {
	ShotID               string   `json:"shot_id"`
	StartMS              int      `json:"start_ms"`
	EndMS                int      `json:"end_ms"`
	Role                 string   `json:"role"`
	FirstFrameArtifactID string   `json:"first_frame_artifact_id"`
	EndFrameArtifactID   string   `json:"end_frame_artifact_id"`
	ImagePromptZH        string   `json:"image_prompt_zh"`
	Subject              string   `json:"subject"`
	Product              string   `json:"product"`
	Scene                string   `json:"scene"`
	Composition          string   `json:"composition"`
	Lighting             string   `json:"lighting"`
	Camera               string   `json:"camera"`
	Action               string   `json:"action"`
	IncomingState        string   `json:"incoming_state"`
	OutgoingState        string   `json:"outgoing_state"`
	MovementAxis         string   `json:"movement_axis"`
	LightingLock         string   `json:"lighting_lock"`
	ProductLock          string   `json:"product_lock"`
	Anchors              []string `json:"anchors"`
	AssetRefs            []string `json:"asset_refs"`
	RightsRefs           []string `json:"rights_refs"`
	KnowledgeRefs        []string `json:"knowledge_refs"`
	ClaimRefs            []string `json:"claim_refs"`
	NegativeConstraints  []string `json:"negative_constraints"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	PlanB                string   `json:"plan_b"`
}

type StoryboardPackage struct {
	ID                    string            `json:"id"`
	Type                  string            `json:"type"`
	SchemaVersion         string            `json:"schema_version"`
	ProjectID             string            `json:"project_id"`
	ApprovedSnapshotID    string            `json:"approved_snapshot_id"`
	ContentItemID         string            `json:"content_item_id"`
	GeneratorCapability   CapabilityRef     `json:"generator_capability"`
	Status                string            `json:"status"`
	Shots                 []StoryboardShot  `json:"shots"`
	Assets                []StoryboardAsset `json:"assets"`
	ReviewSheetArtifactID string            `json:"review_sheet_artifact_id,omitempty"`
	RightsRefs            []string          `json:"rights_refs"`
	SourceDigest          string            `json:"source_digest"`
	LockedDigest          string            `json:"locked_digest"`
}

func (v StoryboardPackage) Validate(requireReviewReady bool) error {
	if strings.TrimSpace(v.ID) == "" || v.Type != "storyboard_package" || v.SchemaVersion != StoryboardPackageSchema || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.ApprovedSnapshotID) == "" || strings.TrimSpace(v.ContentItemID) == "" {
		return fault.Invalid("STORYBOARD_IDENTITY_INVALID", "分镜包缺少稳定 ID、Schema、项目、批准快照或 ContentItem")
	}
	if err := v.GeneratorCapability.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(v.SourceDigest, "sha256:") || !stablehash.Matches(v.SourceDigest) || !strings.HasPrefix(v.LockedDigest, "sha256:") || !stablehash.Matches(v.LockedDigest) {
		return fault.Invalid("STORYBOARD_DIGEST_INVALID", "分镜包 source_digest 或 locked_digest 无效")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "superseded" {
		return fault.Invalid("STORYBOARD_STATUS_INVALID", "分镜包 status 无效")
	}
	if len(v.Shots) == 0 {
		return fault.Invalid("STORYBOARD_SHOTS_REQUIRED", "分镜包必须包含至少一个镜头")
	}
	assetIndex := map[string]StoryboardAsset{}
	for _, asset := range v.Assets {
		if err := asset.Validate(); err != nil {
			return err
		}
		if _, exists := assetIndex[asset.ID]; exists {
			return fault.Invalid("STORYBOARD_ASSET_DUPLICATE", "分镜素材 ID 不能重复")
		}
		assetIndex[asset.ID] = asset
	}
	shotIDs := map[string]bool{}
	for _, shot := range v.Shots {
		if err := shot.Validate(assetIndex, requireReviewReady); err != nil {
			return err
		}
		if shotIDs[shot.ShotID] {
			return fault.Invalid("STORYBOARD_SHOT_DUPLICATE", "分镜 shot_id 不能重复")
		}
		shotIDs[shot.ShotID] = true
	}
	if requireReviewReady && (v.Status != "review_ready" || strings.TrimSpace(v.ReviewSheetArtifactID) == "") {
		return fault.Policy("STORYBOARD_NOT_REVIEW_READY", "发布审核前必须生成 review sheet 并将分镜设为 review_ready", "补齐独立首尾帧和审核接触图")
	}
	if v.ReviewSheetArtifactID != "" {
		asset, ok := assetIndex[v.ReviewSheetArtifactID]
		if !ok || asset.Role != "review_sheet" {
			return fault.Invalid("STORYBOARD_REVIEW_SHEET_INVALID", "review_sheet_artifact_id 必须引用 review_sheet 素材")
		}
	}
	return nil
}

func (v StoryboardPackage) ComputedLockedDigest() (string, error) {
	v.LockedDigest = ""
	v.Assets = append([]StoryboardAsset(nil), v.Assets...)
	v.Shots = append([]StoryboardShot(nil), v.Shots...)
	v.RightsRefs = sortedUniqueV5Strings(v.RightsRefs)
	sort.Slice(v.Assets, func(i, j int) bool { return v.Assets[i].ID < v.Assets[j].ID })
	sort.Slice(v.Shots, func(i, j int) bool {
		if v.Shots[i].StartMS != v.Shots[j].StartMS {
			return v.Shots[i].StartMS < v.Shots[j].StartMS
		}
		return v.Shots[i].ShotID < v.Shots[j].ShotID
	})
	hash, err := stablehash.Sum(v)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (v StoryboardAsset) Validate() error {
	clean := path.Clean(strings.TrimSpace(v.Path))
	if strings.TrimSpace(v.ID) == "" || !validStoryboardAssetRole(v.Role) || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) || strings.TrimSpace(v.MediaType) == "" || !stablehash.Matches(v.SHA256) || strings.HasPrefix(v.SHA256, "sha256:") || v.ByteSize < 0 || !uniqueNonEmpty(v.RightsRefs) {
		return fault.Invalid("STORYBOARD_ASSET_INVALID", "分镜素材缺少安全相对路径、类型、摘要、大小或权利引用")
	}
	if (v.Role == "first_frame" || v.Role == "end_frame") && strings.TrimSpace(v.ShotID) == "" {
		return fault.Invalid("STORYBOARD_ASSET_SHOT_REQUIRED", "首尾帧素材必须引用 shot_id")
	}
	return nil
}

func (v StoryboardShot) Validate(assets map[string]StoryboardAsset, requireMedia bool) error {
	if !storyboardShotIDPattern.MatchString(v.ShotID) || v.StartMS < 0 || v.EndMS <= v.StartMS || strings.TrimSpace(v.Role) == "" || strings.TrimSpace(v.ImagePromptZH) == "" || strings.TrimSpace(v.PlanB) == "" || len(v.NegativeConstraints) == 0 || len(v.AcceptanceCriteria) == 0 {
		return fault.Invalid("STORYBOARD_SHOT_INVALID", "分镜镜头缺少 ID、时间、提示词、禁止项、验收或 Plan B")
	}
	if requireMedia {
		first, ok := assets[v.FirstFrameArtifactID]
		if !ok || first.Role != "first_frame" || first.ShotID != v.ShotID {
			return fault.Policy("STORYBOARD_FIRST_FRAME_REQUIRED", "review_ready 镜头必须引用自己的独立首帧素材", "生成并登记首帧后重试")
		}
		if v.EndFrameArtifactID != "" {
			end, ok := assets[v.EndFrameArtifactID]
			if !ok || end.Role != "end_frame" || end.ShotID != v.ShotID {
				return fault.Invalid("STORYBOARD_END_FRAME_INVALID", "尾帧必须引用同一镜头的 end_frame 素材")
			}
		}
	}
	return nil
}

type SeedanceSettings struct {
	AspectRatio     string `json:"aspect_ratio"`
	DurationSeconds int    `json:"duration_seconds"`
	Sound           string `json:"sound"`
}

type SeedanceUpload struct {
	Reference  string `json:"reference"`
	ArtifactID string `json:"artifact_id"`
	File       string `json:"file"`
	Purpose    string `json:"purpose"`
	SHA256     string `json:"sha256"`
}

type SeedanceSegment struct {
	ID                 string   `json:"id"`
	Order              int      `json:"order"`
	StartMS            int      `json:"start_ms"`
	EndMS              int      `json:"end_ms"`
	PromptZH           string   `json:"prompt_zh"`
	IncomingState      string   `json:"incoming_state"`
	OutgoingState      string   `json:"outgoing_state"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type SeedanceValidation struct {
	ReferencesChecked bool `json:"references_checked"`
	LimitsChecked     bool `json:"limits_checked"`
	RightsChecked     bool `json:"rights_checked"`
	OfferChecked      bool `json:"offer_checked"`
	DigestChecked     bool `json:"digest_checked"`
}

type SeedancePromptPackage struct {
	ID                     string             `json:"id"`
	Type                   string             `json:"type"`
	SchemaVersion          string             `json:"schema_version"`
	StoryboardSnapshotID   string             `json:"storyboard_snapshot_id"`
	StoryboardPackageID    string             `json:"storyboard_package_id"`
	StoryboardLockedDigest string             `json:"storyboard_locked_digest"`
	Provider               string             `json:"provider"`
	ProviderProfileVersion string             `json:"provider_profile_version"`
	AdapterCapability      CapabilityRef      `json:"adapter_capability"`
	Mode                   string             `json:"mode"`
	Settings               SeedanceSettings   `json:"settings"`
	UploadManifest         []SeedanceUpload   `json:"upload_manifest"`
	Segments               []SeedanceSegment  `json:"segments"`
	PostProductionPlan     []string           `json:"post_production_plan"`
	Validation             SeedanceValidation `json:"validation"`
	Status                 string             `json:"status"`
}

func (v SeedancePromptPackage) Validate() error {
	if strings.TrimSpace(v.ID) == "" || v.Type != "seedance_prompt_package" || v.SchemaVersion != SeedancePromptPackageSchema || strings.TrimSpace(v.StoryboardSnapshotID) == "" || strings.TrimSpace(v.StoryboardPackageID) == "" || !strings.HasPrefix(v.StoryboardLockedDigest, "sha256:") || !stablehash.Matches(v.StoryboardLockedDigest) || v.Provider != "seedance" || strings.TrimSpace(v.ProviderProfileVersion) == "" {
		return fault.Invalid("SEEDANCE_PACKAGE_IDENTITY_INVALID", "Seedance 包缺少 ID、锁定分镜、Provider Profile 或摘要")
	}
	if err := v.AdapterCapability.Validate(); err != nil {
		return err
	}
	if v.Mode != "text_to_video" && v.Mode != "image_to_video" && v.Mode != "first_last_frame" && v.Mode != "all_reference" && v.Mode != "extend" {
		return fault.Invalid("SEEDANCE_MODE_INVALID", "Seedance 模式无效")
	}
	if !validAspect(v.Settings.AspectRatio) || v.Settings.DurationSeconds < 1 || (v.Mode != "text_to_video" && len(v.UploadManifest) == 0) || len(v.Segments) == 0 {
		return fault.Invalid("SEEDANCE_PACKAGE_CONTENT_INVALID", "Seedance 设置、上传清单或分段缺失")
	}
	references := map[string]bool{}
	for _, upload := range v.UploadManifest {
		cleanFile := path.Clean(strings.TrimSpace(upload.File))
		matchedReference := seedanceReferencePattern.FindString(upload.Reference)
		if matchedReference != upload.Reference || strings.TrimSpace(upload.ArtifactID) == "" || cleanFile == "." || strings.HasPrefix(cleanFile, "../") || strings.HasPrefix(cleanFile, "/") || strings.Contains(cleanFile, `\`) || !stablehash.Matches(upload.SHA256) || strings.HasPrefix(upload.SHA256, "sha256:") || references[upload.Reference] {
			return fault.Invalid("SEEDANCE_UPLOAD_INVALID", "Seedance 上传项缺失、摘要无效或引用重复")
		}
		references[upload.Reference] = true
	}
	for index, segment := range v.Segments {
		if strings.TrimSpace(segment.ID) == "" || segment.Order != index+1 || segment.EndMS <= segment.StartMS || strings.TrimSpace(segment.PromptZH) == "" || len(segment.AcceptanceCriteria) == 0 {
			return fault.Invalid("SEEDANCE_SEGMENT_INVALID", "Seedance 分段顺序、时间或提示词无效")
		}
		used := seedanceReferencePattern.FindAllString(segment.PromptZH, -1)
		if v.Mode != "text_to_video" && len(used) == 0 {
			return fault.Invalid("SEEDANCE_SEGMENT_REFERENCE_REQUIRED", "每个 Seedance 分段必须引用至少一个已上传素材")
		}
		for _, reference := range used {
			if !references[reference] {
				return fault.Invalid("SEEDANCE_SEGMENT_REFERENCE_UNKNOWN", "Seedance 提示词包含未映射引用："+reference)
			}
		}
	}
	if v.Status != "draft" && v.Status != "validated" && v.Status != "exported" && v.Status != "stale" && v.Status != "superseded" {
		return fault.Invalid("SEEDANCE_STATUS_INVALID", "Seedance package status 无效")
	}
	if v.Status == "validated" || v.Status == "exported" {
		if !v.Validation.ReferencesChecked || !v.Validation.LimitsChecked || !v.Validation.RightsChecked || !v.Validation.OfferChecked || !v.Validation.DigestChecked {
			return fault.Policy("SEEDANCE_VALIDATION_INCOMPLETE", "validated/exported Seedance 包必须通过全部门禁", "重新运行 package validator")
		}
	}
	return nil
}

type PublishedCreativeBinding struct {
	ID                         string    `json:"id"`
	TenantID                   string    `json:"tenant_id,omitempty"`
	SchemaVersion              string    `json:"schema_version"`
	ProjectID                  string    `json:"project_id"`
	DeliveryPackageID          string    `json:"delivery_package_id"`
	RenderedCreativeArtifactID string    `json:"rendered_creative_artifact_id"`
	Platform                   string    `json:"platform"`
	AccountAlias               string    `json:"account_alias"`
	PlatformCreativeID         string    `json:"platform_creative_id"`
	PlatformPostID             string    `json:"platform_post_id"`
	AudienceStrategyVersionID  string    `json:"audience_strategy_version_id"`
	ExperimentID               string    `json:"experiment_id"`
	ExperimentArmID            string    `json:"experiment_arm_id"`
	TestType                   string    `json:"test_type"`
	OfferSnapshotID            string    `json:"offer_snapshot_id,omitempty"`
	PublishedAt                time.Time `json:"published_at"`
	BindingHash                string    `json:"binding_hash"`
	CreatedBy                  string    `json:"created_by,omitempty"`
	CreatedAt                  time.Time `json:"created_at,omitempty"`
}

func (v PublishedCreativeBinding) Validate() error {
	if strings.TrimSpace(v.ID) == "" || v.SchemaVersion != PublishedCreativeBindingSchema || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.DeliveryPackageID) == "" || strings.TrimSpace(v.RenderedCreativeArtifactID) == "" || v.Platform != "douyin" || strings.TrimSpace(v.AccountAlias) == "" || (strings.TrimSpace(v.PlatformCreativeID) == "" && strings.TrimSpace(v.PlatformPostID) == "") || strings.TrimSpace(v.AudienceStrategyVersionID) == "" || strings.TrimSpace(v.ExperimentID) == "" || strings.TrimSpace(v.ExperimentArmID) == "" || !validTestType(v.TestType) || v.PublishedAt.IsZero() || !strings.HasPrefix(v.BindingHash, "sha256:") || !stablehash.Matches(v.BindingHash) {
		return fault.Invalid("PUBLISHED_CREATIVE_BINDING_INVALID", "发布绑定缺少交付、成片、平台、人群、实验、时间或摘要")
	}
	computed, err := v.ComputedHash()
	if err != nil {
		return err
	}
	if stablehash.Normalize(v.BindingHash) != computed {
		return fault.Conflict("PUBLISHED_CREATIVE_BINDING_HASH_MISMATCH", "发布绑定摘要与服务端复算不一致")
	}
	return nil
}

func (v PublishedCreativeBinding) ComputedHash() (string, error) {
	value := struct {
		ProjectID                  string    `json:"project_id"`
		DeliveryPackageID          string    `json:"delivery_package_id"`
		RenderedCreativeArtifactID string    `json:"rendered_creative_artifact_id"`
		Platform                   string    `json:"platform"`
		AccountAlias               string    `json:"account_alias"`
		PlatformCreativeID         string    `json:"platform_creative_id"`
		PlatformPostID             string    `json:"platform_post_id"`
		AudienceStrategyVersionID  string    `json:"audience_strategy_version_id"`
		ExperimentID               string    `json:"experiment_id"`
		ExperimentArmID            string    `json:"experiment_arm_id"`
		TestType                   string    `json:"test_type"`
		OfferSnapshotID            string    `json:"offer_snapshot_id,omitempty"`
		PublishedAt                time.Time `json:"published_at"`
	}{v.ProjectID, v.DeliveryPackageID, v.RenderedCreativeArtifactID, v.Platform, v.AccountAlias, v.PlatformCreativeID, v.PlatformPostID, v.AudienceStrategyVersionID, v.ExperimentID, v.ExperimentArmID, v.TestType, v.OfferSnapshotID, v.PublishedAt.UTC()}
	return stablehash.Sum(value)
}

func validAudienceSegments(values []AudienceSegment) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if strings.TrimSpace(value.Code) == "" || strings.TrimSpace(value.Label) == "" || strings.TrimSpace(value.Definition) == "" || seen[value.Code] {
			return false
		}
		seen[value.Code] = true
	}
	return true
}

func uniqueNonEmpty(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validTestType(value string) bool {
	return value == "strict_ab" || value == "exploration_batch" || value == "audience_expression_fit_test"
}

func validExperimentVariable(value string) bool {
	return value == "hook" || value == "audience" || value == "scenario" || value == "visualization" || value == "cta" || value == "duration"
}

func validStoryboardAssetRole(value string) bool {
	return value == "first_frame" || value == "end_frame" || value == "identity_anchor" || value == "review_sheet" || value == "reference_video" || value == "reference_audio"
}

func validAspect(value string) bool {
	return value == "9:16" || value == "16:9" || value == "1:1" || value == "4:5"
}

func validateOptionalContentHash(value AudienceStrategyVersion, contentHash string) error {
	if strings.TrimSpace(contentHash) == "" {
		return nil
	}
	if !strings.HasPrefix(contentHash, "sha256:") || !stablehash.Matches(contentHash) {
		return fault.Invalid("AUDIENCE_STRATEGY_HASH_INVALID", "content_hash 必须是带前缀的 SHA-256")
	}
	value.ContentHash = ""
	computed, err := stablehash.Sum(value)
	if err != nil {
		return err
	}
	if stablehash.Normalize(contentHash) != computed {
		return fault.Conflict("AUDIENCE_STRATEGY_HASH_MISMATCH", "人群策略 content_hash 与内容不一致")
	}
	return nil
}

func SortedAudienceSegments(values []AudienceSegment) []AudienceSegment {
	out := append([]AudienceSegment(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func sortedUniqueV5Strings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
