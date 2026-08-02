package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	builtinSOPContentResearch = "content_research"
	builtinSOPShortVideo      = "short_video_production"
	builtinSOPArticle         = "article_collaboration"
	builtinSOPRetrospective   = "campaign_retrospective"
	builtinSOPSourceRef       = "content-work-os/builtin-sops@1"
	builtinSOPLegacySourceRef = "legacy/default-short-video"
)

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
			Description:  "把 Brief、来源、Evidence 和知识快照整理成可供内容任务引用的事实底座。默认只做确定性检查，不强制人工审批。",
			ContentTypes: []string{domain.ContentTypeVideoScript, domain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "需求 Brief", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
				{ID: "sources", Name: "来源登记", Order: 20, OwnerRoles: []string{"strategist", "editor"}, InputRefs: []string{"brief"}, OutputSchema: "contentcloud.source_bundle/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"source.registered"}, RetryMaxAttempts: 2},
				{ID: "evidence", Name: "Evidence 与知识候选", Order: 30, OwnerRoles: []string{"strategist"}, InputRefs: []string{"brief", "sources"}, OutputSchema: "contentcloud.knowledge_candidates/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, GateIDs: []string{"knowledge_check"}, RetryMaxAttempts: 2},
				{ID: "snapshot", Name: "知识快照", Order: 40, OwnerRoles: []string{"strategist", "project_manager"}, InputRefs: []string{"evidence"}, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local"}, Checks: []string{"knowledge.lint"}, RetryMaxAttempts: 1},
			},
			Gates: []domain.GateDefinition{{ID: "knowledge_check", Name: "知识确定性检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"claim.references", "rights.references"}, OnReject: "changes_requested"}},
		},
		{
			Key: builtinSOPShortVideo, ID: "builtin-sop-short-video", Name: "短视频生产",
			Description:  "从 Brief、知识和策略到短视频脚本与交付的默认生产流程；人工审核不预置为必选，可在管理后台加到任意 Stage。",
			ContentTypes: []string{domain.ContentTypeVideoScript}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "需求 Brief", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
				{ID: "knowledge", Name: "知识与证据", Order: 20, OwnerRoles: []string{"strategist"}, InputRefs: []string{"brief"}, OutputSchema: domain.KnowledgeSnapshotSchema, ExecutionModes: []string{"local", "agent"}, Checks: []string{"claim.references", "rights.references"}, RetryMaxAttempts: 2},
				{ID: "strategy", Name: "受众与策略", Order: 30, OwnerRoles: []string{"strategist", "editor"}, InputRefs: []string{"brief", "knowledge"}, OutputSchema: "contentcloud.strategy/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"strategy.complete"}, RetryMaxAttempts: 2},
				{ID: "script", Name: "脚本创作", Order: 40, OwnerRoles: []string{"editor"}, InputRefs: []string{"strategy"}, OutputSchema: "contentcloud.video_script/1.0", RequiredCapabilities: []string{"content.script.compose"}, ExecutionModes: []string{"local", "agent"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 3},
				{ID: "quality", Name: "品牌与权利检查", Order: 50, OwnerRoles: []string{"editor", "reviewer"}, InputRefs: []string{"script", "knowledge"}, OutputSchema: "contentcloud.quality_report/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema", "claim.references", "rights.references"}, GateIDs: []string{"quality_check"}, RetryMaxAttempts: 2},
				{ID: "delivery", Name: "Accepted 与交付", Order: 60, OwnerRoles: []string{"editor", "project_manager"}, InputRefs: []string{"script", "quality"}, OutputSchema: "contentcloud.delivery/1.0", ExecutionModes: []string{"local"}, Checks: []string{"content.schema"}, RetryMaxAttempts: 1},
			},
			Gates: []domain.GateDefinition{{ID: "quality_check", Name: "内容质量确定性检查", Mode: domain.GateModeRequiredCheck, Blocking: true, Checks: []string{"content.schema", "claim.references", "rights.references"}, OnReject: "changes_requested"}},
		},
		{
			Key: builtinSOPArticle, ID: "builtin-sop-article", Name: "文章协作",
			Description:  "适合编辑、本地客户端和业务负责人协作完成有引用的文章交付；人工协作门禁按租户方法论配置。",
			ContentTypes: []string{domain.ContentTypeWeChatArticle}, DefaultExecutionMode: "local", SourceRef: builtinSOPSourceRef,
			Stages: []domain.StageDefinition{
				{ID: "brief", Name: "文章 Brief", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local", "agent"}, Checks: []string{"brief.required"}, RetryMaxAttempts: 2},
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
		if !found && template.Key == builtinSOPShortVideo {
			if legacy, ok := findLegacyShortVideo(current); ok {
				legacy.Definition.TemplateKey = template.Key
				legacy.Definition.BuiltIn = true
				legacy.Definition.SourceRef = builtinSOPLegacySourceRef
				legacy.Definition.UpdatedAt = s.now().UTC()
				if err := s.store.SaveSOPDefinition(ctx, legacy.Definition); err != nil {
					return nil, err
				}
				summary, found = legacy, true
			}
		}
		if !found {
			now := s.now().UTC()
			definition := domain.SOPDefinition{ID: template.ID, TenantID: actor.TenantID, Name: template.Name, Description: template.Description, ContentTypes: append([]string{}, template.ContentTypes...), CurrentVersion: 1, TemplateKey: template.Key, BuiltIn: true, SourceRef: template.SourceRef, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
			version := builtinSOPVersion(template, definition.ID, 1, actor, now, "published")
			if err := s.store.CreateSOP(ctx, definition, version); err != nil {
				return nil, err
			}
			continue
		}
		adopted, err := s.adoptBuiltinDefinition(ctx, summary, template)
		if err != nil {
			return nil, err
		}
		summary = adopted
		if err := s.ensureBuiltinVersion(ctx, actor, summary, template); err != nil {
			return nil, err
		}
	}
	return s.store.SOPs(ctx, actor.TenantID)
}

// adoptBuiltinDefinition repairs metadata written before built-in templates
// had explicit identity fields. It only adopts a known platform ID or key;
// arbitrary same-name custom SOPs remain untouched.
func (s *Service) adoptBuiltinDefinition(ctx context.Context, summary domain.SOPSummary, template builtinSOPTemplate) (domain.SOPSummary, error) {
	definition := summary.Definition
	if definition.BuiltIn && definition.TemplateKey == template.Key && definition.SourceRef != "" {
		return summary, nil
	}
	if definition.ID != template.ID && definition.TemplateKey != template.Key {
		return summary, nil
	}
	definition.TemplateKey = template.Key
	definition.BuiltIn = true
	if definition.SourceRef == "" {
		definition.SourceRef = template.SourceRef
	}
	definition.UpdatedAt = s.now().UTC()
	if err := s.store.SaveSOPDefinition(ctx, definition); err != nil {
		return domain.SOPSummary{}, err
	}
	summary.Definition = definition
	return summary, nil
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
		if summary.Definition.TemplateKey == template.Key || summary.Definition.ID == template.ID {
			return summary, true
		}
	}
	return domain.SOPSummary{}, false
}

func findLegacyShortVideo(sops []domain.SOPSummary) (domain.SOPSummary, bool) {
	for _, summary := range sops {
		if summary.Definition.BuiltIn || summary.Definition.TemplateKey != "" || summary.Definition.Name != "短视频生产" {
			continue
		}
		for _, version := range summary.Versions {
			if version.Status != "published" || len(version.Stages) != 4 || len(version.Gates) != 0 || len(version.ContentTypes) != 1 || version.ContentTypes[0] != domain.ContentTypeVideoScript {
				continue
			}
			stages := append([]domain.StageDefinition{}, version.Stages...)
			sort.SliceStable(stages, func(i, j int) bool { return stages[i].Order < stages[j].Order })
			ids := []string{stages[0].ID, stages[1].ID, stages[2].ID, stages[3].ID}
			if strings.Join(ids, ",") == "brief,knowledge,draft,delivery" {
				return summary, true
			}
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
