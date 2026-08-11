package app

import (
	"context"
	"strconv"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	builtinSOPContentResearch = "content_research"
	builtinSOPShortVideo      = "short_video_production"
	builtinSOPMarketingVideo  = "marketing_video_production"
	builtinSOPArticle         = "article_collaboration"
	builtinSOPRetrospective   = "campaign_retrospective"
	builtinSOPSourceRef       = "content-work-os/builtin-sops@1"
)

func builtinSOPKeyForContentType(contentType string) string {
	switch contentType {
	case domain.ContentTypeMarketingVideo:
		return builtinSOPMarketingVideo
	case domain.ContentTypeWeChatArticle:
		return builtinSOPArticle
	default:
		return builtinSOPShortVideo
	}
}

type builtinSOPTemplate struct {
	Key                  string
	ID                   string
	Name                 string
	Description          string
	ContentTypes         []string
	Stages               []domain.StageDefinition
	Gates                []domain.GateDefinition
	DefaultExecutionMode string
	SourceRef            string
}

func builtinSOPTemplates() []builtinSOPTemplate {
	return []builtinSOPTemplate{
		{
			Key: builtinSOPContentResearch, ID: "builtin-sop-content-research", Name: "资料与知识建设",
			Description:  "把创作简报、来源、证据和知识快照整理成可供内容任务引用的事实基础。默认只做确定性检查，不强制人工审批。",
			ContentTypes: []string{domain.ContentTypeVideoScript, domain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "需求简报", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
				{ID: "sources", Name: "来源登记", Order: 20, OwnerRoles: []string{"strategist", "editor"}, InputRefs: []string{"brief"}, OutputSchema: "contentcloud.source_bundle/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"source.registered"}, RetryMaxAttempts: 2},
				{ID: "evidence", Name: "证据与知识候选", Order: 30, OwnerRoles: []string{"strategist"}, InputRefs: []string{"brief", "sources"}, OutputSchema: "contentcloud.knowledge_candidates/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, GateIDs: []string{"knowledge_check"}, RetryMaxAttempts: 2},
				{ID: "snapshot", Name: "知识快照", Order: 40, OwnerRoles: []string{"strategist", "project_manager"}, InputRefs: []string{"evidence"}, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local"}, Checks: []string{"knowledge.lint"}, RetryMaxAttempts: 1},
			},
			Gates: []domain.GateDefinition{{ID: "knowledge_check", Name: "知识确定性检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"claim.references", "rights.references"}, OnReject: "changes_requested"}},
		},
		{
			Key: builtinSOPShortVideo, ID: "builtin-sop-short-video", Name: "短视频生产",
			Description:  "从创作简报、知识和策略到短视频脚本与交付的默认生产流程；人工审核不预置为必选，可在管理后台加到任意流程阶段。",
			ContentTypes: []string{domain.ContentTypeVideoScript}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "需求简报", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
				{ID: "knowledge", Name: "知识与证据", Order: 20, OwnerRoles: []string{"strategist"}, InputRefs: []string{"brief"}, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, RetryMaxAttempts: 2},
				{ID: "strategy", Name: "受众与策略", Order: 30, OwnerRoles: []string{"strategist", "editor"}, InputRefs: []string{"brief", "knowledge"}, OutputSchema: "contentcloud.strategy/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"strategy.complete"}, RetryMaxAttempts: 2},
				{ID: "script", Name: "脚本创作", Order: 40, OwnerRoles: []string{"editor"}, InputRefs: []string{"strategy"}, OutputSchema: "contentcloud.video_script/1.0", RequiredCapabilities: []string{"content.script.compose"}, ExecutionModes: []string{"local", "agent"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 3},
				{ID: "quality", Name: "品牌与权利检查", Order: 50, OwnerRoles: []string{"editor", "reviewer"}, InputRefs: []string{"script", "knowledge"}, OutputSchema: "contentcloud.quality_report/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema", "claim.references", "rights.references"}, GateIDs: []string{"quality_check"}, RetryMaxAttempts: 2},
				{ID: "delivery", Name: "接受与交付", Order: 60, OwnerRoles: []string{"editor", "project_manager"}, InputRefs: []string{"script", "quality"}, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 1},
			},
			Gates: []domain.GateDefinition{{ID: "quality_check", Name: "内容质量确定性检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"content.schema", "claim.references", "rights.references"}, OnReject: "changes_requested"}},
		},
		{
			Key: builtinSOPMarketingVideo, ID: "builtin-sop-marketing-video", Name: "营销视频全流程",
			Description:  "从来源、知识、剧本和分镜到视频生成、质检、后期与可验证交付的完整生产流程。",
			ContentTypes: []string{domain.ContentTypeMarketingVideo}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "sources", Name: "来源与证据", Order: 10, OwnerRoles: []string{"strategist", "editor"}, OutputSchema: "contentcloud.source_bundle/1.0", OutputSchemaRefs: []string{"contentcloud.source-revision/1.0"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputSourceRevision, Role: domain.StageOutputRolePrimary, MinStatus: domain.StageOutputStatusValidated, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local", "agent"}, Checks: []string{"source.registered"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 2}},
				{ID: "knowledge", Name: "知识与权利快照", Order: 20, OwnerRoles: []string{"strategist", "reviewer"}, InputRefs: []string{"sources"}, OutputSchema: domain.KnowledgeSnapshotSchema, OutputSchemaRefs: []string{domain.KnowledgeSnapshotSchema}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputKnowledgeSnapshot, Role: domain.StageOutputRolePrimary, MinStatus: domain.StageOutputStatusApproved, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, GateIDs: []string{"knowledge_check"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 2}},
				{ID: "script", Name: "短视频剧本", Order: 30, OwnerRoles: []string{"editor", "strategist"}, InputRefs: []string{"knowledge"}, OutputSchema: "contentcloud.marketing_video_script/1.0", OutputSchemaRefs: []string{"contentcloud.marketing_video_script/1.0"}, RequiredCapabilities: []string{"content.script.compose"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputSubmissionRevision, Role: domain.StageOutputRolePrimary, MinStatus: domain.StageOutputStatusValidated, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local", "agent"}, Checks: []string{"content.schema", "claim.references"}, GateIDs: []string{"script_review"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 3}},
				{ID: "storyboard", Name: "分镜与图片素材", Order: 40, OwnerRoles: []string{"editor", "reviewer"}, InputRefs: []string{"script", "knowledge"}, OutputSchema: "contentcloud.storyboard-package/1.0", OutputSchemaRefs: []string{"contentcloud.storyboard-package/1.0"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputStoryboardPackage, Role: domain.StageOutputRolePrimary, MinStatus: domain.StageOutputStatusApproved, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local", "agent"}, Checks: []string{"storyboard.locked", "rights.references"}, GateIDs: []string{"storyboard_review"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 3, AllowPartialRetry: true}},
				{ID: "generation", Name: "视频生成", Order: 50, OwnerRoles: []string{"editor", "project_manager"}, InputRefs: []string{"storyboard"}, OutputSchema: "contentcloud.media-generation-result/1.0", OutputSchemaRefs: []string{"contentcloud.media-generation-job/1.0", "contentcloud.artifact/1.0"}, RequiredCapabilities: []string{"media.video.generate"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputGenerationJob, Role: domain.StageOutputRolePrimary, MinStatus: domain.StageOutputStatusValidated, MinCount: 1}, {OutputType: domain.StageOutputArtifact, Role: domain.StageOutputRolePreview, MinStatus: domain.StageOutputStatusValidated, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutorPolicy: "media_worker", ExecutionModes: []string{"local"}, Checks: []string{"media.technical", "cost.confirmed"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 3, AllowPartialRetry: true, RetryableErrorCode: []string{"RATE_LIMITED", "PROVIDER_UNAVAILABLE", "DOWNLOAD_EXPIRED"}}, CostPolicy: domain.StageCostPolicy{Currency: "CNY", RequireApprovalAboveMinor: 1, EstimateTTLSeconds: 900}},
				{ID: "review", Name: "成片质检与候选成片选择", Order: 60, OwnerRoles: []string{"reviewer", "editor"}, InputRefs: []string{"generation"}, OutputSchema: "contentcloud.media-review/1.0", OutputSchemaRefs: []string{"contentcloud.media-review/1.0"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputMediaReview, Role: domain.StageOutputRoleSelectedTake, MinStatus: domain.StageOutputStatusApproved, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local"}, Checks: []string{"media.technical", "media.content"}, GateIDs: []string{"take_review"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 3, AllowPartialRetry: true}},
				{ID: "postproduction", Name: "后期与最终批准", Order: 70, OwnerRoles: []string{"editor", "reviewer"}, InputRefs: []string{"review"}, OutputSchema: "contentcloud.final-render/1.0", OutputSchemaRefs: []string{"contentcloud.artifact/1.0", "contentcloud.media-review/1.0"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputArtifact, Role: domain.StageOutputRoleFinal, MinStatus: domain.StageOutputStatusValidated, MinCount: 1}, {OutputType: domain.StageOutputMediaReview, Role: domain.StageOutputRoleFinal, MinStatus: domain.StageOutputStatusApproved, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local"}, Checks: []string{"media.final", "offer.valid", "rights.references"}, GateIDs: []string{"final_review"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 2}},
				{ID: "delivery", Name: "交付包", Order: 80, OwnerRoles: []string{"project_manager", "editor"}, InputRefs: []string{"postproduction"}, OutputSchema: "contentcloud.delivery-package/1.0", OutputSchemaRefs: []string{"contentcloud.delivery-package/1.0"}, RequiredOutputTypes: []domain.StageObjectRequirement{{OutputType: domain.StageOutputDeliveryPackage, Role: domain.StageOutputRoleFinal, MinStatus: domain.StageOutputStatusApproved, MinCount: 1}}, CompletionPolicy: domain.StageCompletionAllRequired, ExecutionModes: []string{"local"}, Checks: []string{"delivery.integrity"}, RetryPolicy: domain.StageRetryPolicy{MaxAttempts: 1}},
			},
			Gates: []domain.GateDefinition{
				{ID: "knowledge_check", Name: "知识与权利检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"claim.references", "rights.references"}, OnReject: "changes_requested"},
				{ID: "script_review", Name: "剧本审核", Mode: domain.GateModeInternalReview, Blocking: true, AssigneeRoles: []string{"reviewer", "project_manager"}, Checks: []string{"content.schema", "claim.references"}, OnReject: "changes_requested"},
				{ID: "storyboard_review", Name: "分镜锁定", Mode: domain.GateModeInternalReview, Blocking: true, AssigneeRoles: []string{"reviewer", "project_manager"}, Checks: []string{"storyboard.locked", "rights.references"}, OnReject: "changes_requested"},
				{ID: "take_review", Name: "候选成片选择确认", Mode: domain.GateModeInternalReview, Blocking: true, AssigneeRoles: []string{"reviewer", "project_manager"}, Checks: []string{"media.technical", "media.content"}, OnReject: "changes_requested"},
				{ID: "final_review", Name: "最终成片批准", Mode: domain.GateModeClientDecision, Blocking: true, AssigneeRoles: []string{"client_approver", "tenant_admin"}, Checks: []string{"media.final", "offer.valid", "rights.references"}, OnReject: "changes_requested"},
			},
		},
		{
			Key: builtinSOPArticle, ID: "builtin-sop-article", Name: "文章协作",
			Description:  "适合编辑、本地客户端和业务负责人协作完成有引用的文章交付；人工协作门禁按租户方法论配置。",
			ContentTypes: []string{domain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "文章创作简报", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
				{ID: "knowledge", Name: "知识引用", Order: 20, OwnerRoles: []string{"strategist"}, InputRefs: []string{"brief"}, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, RetryMaxAttempts: 2},
				{ID: "draft", Name: "文章写作", Order: 30, OwnerRoles: []string{"editor"}, InputRefs: []string{"brief", "knowledge"}, OutputSchema: "contentcloud.article/1.0", RequiredCapabilities: []string{"content.article.compose"}, ExecutionModes: []string{"local", "agent"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 3},
				{ID: "quality", Name: "引用与品牌检查", Order: 40, OwnerRoles: []string{"editor", "reviewer"}, InputRefs: []string{"draft", "knowledge"}, OutputSchema: "contentcloud.quality_report/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema", "claim.references", "rights.references"}, GateIDs: []string{"quality_check"}, RetryMaxAttempts: 2},
				{ID: "delivery", Name: "文章交付", Order: 50, OwnerRoles: []string{"editor", "project_manager"}, InputRefs: []string{"draft", "quality"}, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 1},
			},
			Gates: []domain.GateDefinition{{ID: "quality_check", Name: "文章确定性检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"content.schema", "claim.references", "rights.references"}, OnReject: "changes_requested"}},
		},
		{
			Key: builtinSOPRetrospective, ID: "builtin-sop-retrospective", Name: "活动结果复盘",
			Description:  "把内容版本、渠道观察和指标整理成可审阅的复盘与下一轮假设，不自动改写策略或发布内容。",
			ContentTypes: []string{domain.ContentTypeVideoScript, domain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "result", Name: "结果导入", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.performance_observation/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"metrics.source", "metrics.window"}, RetryMaxAttempts: 2},
				{ID: "binding", Name: "版本绑定", Order: 20, OwnerRoles: []string{"strategist"}, InputRefs: []string{"result"}, OutputSchema: "contentcloud.content_binding/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.id"}, RetryMaxAttempts: 1},
				{ID: "analysis", Name: "问题归因", Order: 30, OwnerRoles: []string{"strategist", "editor"}, InputRefs: []string{"result", "binding"}, OutputSchema: "contentcloud.retro_analysis/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"observation.complete"}, RetryMaxAttempts: 2},
				{ID: "learning", Name: "改进建议", Order: 40, OwnerRoles: []string{"strategist", "project_manager"}, InputRefs: []string{"analysis"}, OutputSchema: "contentcloud.learning_candidate/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"hypothesis.scoped"}, RetryMaxAttempts: 2},
				{ID: "handoff", Name: "复盘交接", Order: 50, OwnerRoles: []string{"project_manager"}, InputRefs: []string{"learning"}, OutputSchema: "contentcloud.retro/1.0", ExecutionModes: []string{"local"}, Checks: []string{"next_action.required"}, RetryMaxAttempts: 1},
			},
		},
	}
}

func (s *Service) ensureBuiltinSOPs(ctx context.Context, actor Actor, current []domain.SOPSummary) ([]domain.SOPSummary, error) {
	for _, template := range builtinSOPTemplates() {
		summary, found := findSOPTemplate(current, template)
		if !found {
			now := s.now().UTC()
			definition := domain.SOPDefinition{ID: template.ID, TenantID: actor.TenantID, Name: template.Name, Description: template.Description, ContentTypes: append([]string{}, template.ContentTypes...), CurrentVersion: 1, TemplateKey: template.Key, BuiltIn: true, SourceRef: template.SourceRef, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
			version := builtinSOPVersion(template, definition.ID, 1, actor, now, "published")
			if err := s.store.CreateSOP(ctx, definition, version); err != nil {
				return nil, err
			}
			continue
		}
		if err := s.ensureBuiltinVersion(ctx, actor, summary, template); err != nil {
			return nil, err
		}
	}
	return s.store.SOPs(ctx, actor.TenantID)
}

func (s *Service) ensureBuiltinVersion(ctx context.Context, actor Actor, summary domain.SOPSummary, template builtinSOPTemplate) error {
	for _, version := range summary.Versions {
		if version.Status == "published" && sopShapeDigest(version) == sopShapeDigest(builtinSOPVersion(template, version.SOPID, 0, actor, time.Time{}, "published")) {
			return nil
		}
	}
	maxVersion := 0
	for _, version := range summary.Versions {
		if version.Version > maxVersion {
			maxVersion = version.Version
		}
	}
	now := s.now().UTC()
	draft := builtinSOPVersion(template, summary.Definition.ID, maxVersion+1, actor, now, "draft")
	if err := s.store.CreateSOPVersion(ctx, draft); err != nil {
		return err
	}
	_, err := s.PublishSOP(ctx, actor, summary.Definition.ID, draft.Version, "builtin-sop-upgrade")
	return err
}

func builtinSOPVersion(template builtinSOPTemplate, sopID string, version int, actor Actor, now time.Time, status string) domain.SOPVersion {
	versionLabel := "template"
	if version > 0 {
		versionLabel = strconv.Itoa(version)
	}
	value := domain.SOPVersion{ID: template.ID + "-v" + versionLabel, TenantID: actor.TenantID, SOPID: sopID, Version: version, SchemaVersion: domain.SOPSchemaVersion, Name: template.Name, Description: template.Description, ContentTypes: append([]string{}, template.ContentTypes...), Stages: append([]domain.StageDefinition{}, template.Stages...), Gates: append([]domain.GateDefinition{}, template.Gates...), DefaultExecutionMode: template.DefaultExecutionMode, Status: status, CreatedBy: actor.UserID, CreatedAt: now}
	if status == "published" && !now.IsZero() {
		value.PublishedBy = actor.UserID
		value.PublishedAt = &now
		if digest, err := value.ContentDigest(); err == nil {
			value.Digest = "sha256:" + digest
		}
	}
	value.NormalizeCollections()
	return value
}

func findSOPTemplate(sops []domain.SOPSummary, template builtinSOPTemplate) (domain.SOPSummary, bool) {
	for _, summary := range sops {
		definition := summary.Definition
		if definition.ID == template.ID && definition.TemplateKey == template.Key && definition.BuiltIn && definition.SourceRef == template.SourceRef {
			return summary, true
		}
	}
	return domain.SOPSummary{}, false
}

func sopShapeDigest(value domain.SOPVersion) string {
	value.NormalizeCollections()
	digest, _ := domain.CanonicalHash(struct {
		SchemaVersion string                   `json:"schema_version"`
		Name          string                   `json:"name"`
		Description   string                   `json:"description"`
		ContentTypes  []string                 `json:"content_types"`
		Stages        []domain.StageDefinition `json:"stages"`
		Gates         []domain.GateDefinition  `json:"gates"`
		ExecutionMode string                   `json:"default_execution_mode"`
	}{value.SchemaVersion, value.Name, value.Description, value.ContentTypes, value.Stages, value.Gates, value.DefaultExecutionMode})
	return digest
}
