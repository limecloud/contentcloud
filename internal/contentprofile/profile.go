package contentprofile

import (
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Stage struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	InputRefs        []string `json:"input_refs"`
	CapabilityID     string   `json:"capability_id"`
	ExecutorKinds    []string `json:"executor_kinds"`
	Deterministic    bool     `json:"deterministic"`
	OutputSchema     string   `json:"output_schema"`
	Checks           []string `json:"checks"`
	GateIDs          []string `json:"gate_ids"`
	RetryMaxAttempts int      `json:"retry_max_attempts"`
}

type Profile struct {
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	Digest         string   `json:"digest"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	ContentTypes   []string `json:"content_types"`
	Channels       []string `json:"channels"`
	Stages         []Stage  `json:"stages"`
	RequiredGates  []string `json:"required_gates"`
	DeliverySchema string   `json:"delivery_schema"`
}

var builtins = map[string]Profile{
	"douyin-commerce-video": {
		ID: "douyin-commerce-video", Version: "1.0.0", Name: "抖音电商短视频", Description: "商品事实、受众、Offer、剧本、分镜、成片、渠道预览和回执的完整生产链。",
		ContentTypes: []string{domain.ContentTypeMarketingVideo}, Channels: []string{"douyin"}, RequiredGates: []string{"offer_valid", "rights_review", "final_media_review", "publication_preview"}, DeliverySchema: "contentcloud.douyin-commerce-delivery/1.0",
		Stages: []Stage{
			{ID: "audience_strategy", Name: "受众与需求时刻", CapabilityID: "content.audience.strategy", ExecutorKinds: []string{"agent", "agent_saas", "human"}, OutputSchema: "contentcloud.audience-strategy/1.0", Checks: []string{"audience.evidence"}, RetryMaxAttempts: 2},
			{ID: "brief_script", Name: "Offer 与短视频剧本", InputRefs: []string{"audience_strategy"}, CapabilityID: "content.script.compose", ExecutorKinds: []string{"agent", "model", "agent_saas", "human"}, OutputSchema: "contentcloud.content-item/3.0", Checks: []string{"offer.valid", "content.schema", "claim.references"}, GateIDs: []string{"offer_valid"}, RetryMaxAttempts: 3},
			{ID: "storyboard", Name: "分镜与首尾帧", InputRefs: []string{"brief_script"}, CapabilityID: "content.storyboard.compose", ExecutorKinds: []string{"agent", "creative_saas", "human"}, OutputSchema: "contentcloud.storyboard-package/1.0", Checks: []string{"storyboard.locked", "rights.references"}, GateIDs: []string{"rights_review"}, RetryMaxAttempts: 3},
			{ID: "render", Name: "视频生成与剪辑", InputRefs: []string{"storyboard"}, CapabilityID: "media.video.generate", ExecutorKinds: []string{"media_provider", "creative_saas", "human"}, OutputSchema: "video/mp4", Checks: []string{"cost.confirmed"}, RetryMaxAttempts: 3},
			{ID: "transcode_validate", Name: "字幕、规格与权利校验", InputRefs: []string{"render"}, CapabilityID: "media.video.validate", ExecutorKinds: []string{"worker"}, Deterministic: true, OutputSchema: "contentcloud.media-validation/1.0", Checks: []string{"media.technical", "subtitle.consistency", "rights.references"}, GateIDs: []string{"final_media_review"}, RetryMaxAttempts: 2},
			{ID: "publish", Name: "渠道预览、发布与回执", InputRefs: []string{"transcode_validate"}, CapabilityID: "channel.publish", ExecutorKinds: []string{"channel_adapter", "browser", "human"}, OutputSchema: "contentcloud.channel-publication/1.0", Checks: []string{"delivery.integrity", "channel.preview"}, GateIDs: []string{"publication_preview"}, RetryMaxAttempts: 1},
		},
	},
	"wechat-official-article": {
		ID: "wechat-official-article", Version: "1.0.0", Name: "微信公众号文章", Description: "证据研究、文章候选、编辑、确定性排版、移动预览、人工或 API 发布的完整生产链。",
		ContentTypes: []string{domain.ContentTypeWeChatArticle}, Channels: []string{"wechat_official_account"}, RequiredGates: []string{"citation_review", "rights_review", "mobile_preview", "publication_preview"}, DeliverySchema: "contentcloud.wechat-delivery/1.0",
		Stages: []Stage{
			{ID: "research", Name: "搜索、采集与证据", CapabilityID: "source.research", ExecutorKinds: []string{"agent", "search", "connector", "agent_saas", "human"}, OutputSchema: "contentcloud.evidence-bundle/3.0", Checks: []string{"source.fixed", "claim.references"}, RetryMaxAttempts: 2},
			{ID: "draft", Name: "文章候选", InputRefs: []string{"research"}, CapabilityID: "content.article.compose", ExecutorKinds: []string{"agent", "model", "agent_saas", "human"}, OutputSchema: "contentcloud.article/1.0", Checks: []string{"content.schema", "claim.references"}, GateIDs: []string{"citation_review"}, RetryMaxAttempts: 3},
			{ID: "copyedit", Name: "编辑、品牌与权利复核", InputRefs: []string{"draft"}, CapabilityID: "content.article.edit", ExecutorKinds: []string{"agent", "agent_saas", "human"}, OutputSchema: "contentcloud.article/1.0", Checks: []string{"content.schema", "rights.references"}, GateIDs: []string{"rights_review"}, RetryMaxAttempts: 2},
			{ID: "layout_compile", Name: "微信排版编译", InputRefs: []string{"copyedit"}, CapabilityID: "content.wechat.layout", ExecutorKinds: []string{"worker"}, Deterministic: true, OutputSchema: "contentcloud.wechat-delivery/1.0", Checks: []string{"layout.template_digest", "html.inline_css", "asset.mapping"}, RetryMaxAttempts: 1},
			{ID: "layout_lint", Name: "移动预览与平台清洗差异", InputRefs: []string{"layout_compile"}, CapabilityID: "content.wechat.layout.validate", ExecutorKinds: []string{"worker", "browser", "human"}, Deterministic: true, OutputSchema: "contentcloud.validation-report/1.0", Checks: []string{"layout.mobile", "layout.sanitize_diff", "layout.overflow"}, GateIDs: []string{"mobile_preview"}, RetryMaxAttempts: 2},
			{ID: "publish", Name: "渠道预览、发布与回执", InputRefs: []string{"layout_lint"}, CapabilityID: "channel.publish", ExecutorKinds: []string{"channel_adapter", "browser", "human"}, OutputSchema: "contentcloud.channel-publication/1.0", Checks: []string{"delivery.integrity", "channel.preview"}, GateIDs: []string{"publication_preview"}, RetryMaxAttempts: 1},
		},
	},
	"serialized-novel": {
		ID: "serialized-novel", Version: "1.0.0", Name: "连载小说", Description: "Canon、卷章规划、章节候选、连续性校验、编辑、交付和平台回执的长期生产链。",
		ContentTypes: []string{domain.ContentTypeSerializedNovel}, Channels: []string{"ebook", "web_novel", "content_store"}, RequiredGates: []string{"canon_consistency", "editor_review", "rights_review", "release_preview"}, DeliverySchema: "contentcloud.novel-release/1.0",
		Stages: []Stage{
			{ID: "canon", Name: "世界观、角色与时间线 Canon", CapabilityID: "content.novel.canon", ExecutorKinds: []string{"agent", "agent_saas", "human"}, OutputSchema: "contentcloud.novel-canon/1.0", Checks: []string{"canon.schema"}, RetryMaxAttempts: 2},
			{ID: "outline", Name: "卷、幕与章节规划", InputRefs: []string{"canon"}, CapabilityID: "content.novel.outline", ExecutorKinds: []string{"agent", "agent_saas", "human"}, OutputSchema: "contentcloud.novel-outline/1.0", Checks: []string{"outline.canon_refs"}, RetryMaxAttempts: 3},
			{ID: "chapter_draft", Name: "章节候选", InputRefs: []string{"canon", "outline"}, CapabilityID: "content.novel.chapter.compose", ExecutorKinds: []string{"agent", "model", "agent_saas", "human"}, OutputSchema: "contentcloud.novel-chapter/1.0", Checks: []string{"chapter.schema"}, RetryMaxAttempts: 3},
			{ID: "continuity_lint", Name: "连续性与伏笔校验", InputRefs: []string{"canon", "chapter_draft"}, CapabilityID: "content.novel.continuity.validate", ExecutorKinds: []string{"worker"}, Deterministic: true, OutputSchema: "contentcloud.continuity-report/1.0", Checks: []string{"canon.character_refs", "canon.timeline", "canon.open_threads"}, GateIDs: []string{"canon_consistency"}, RetryMaxAttempts: 1},
			{ID: "edit", Name: "发展编辑、文风编辑与校对", InputRefs: []string{"chapter_draft", "continuity_lint"}, CapabilityID: "content.novel.edit", ExecutorKinds: []string{"agent", "agent_saas", "human"}, OutputSchema: "contentcloud.novel-chapter/1.0", Checks: []string{"chapter.style", "chapter.compliance"}, GateIDs: []string{"editor_review", "rights_review"}, RetryMaxAttempts: 3},
			{ID: "package", Name: "平台格式、封面与元数据打包", InputRefs: []string{"edit"}, CapabilityID: "content.novel.package", ExecutorKinds: []string{"worker"}, Deterministic: true, OutputSchema: "contentcloud.novel-release/1.0", Checks: []string{"delivery.integrity"}, RetryMaxAttempts: 1},
			{ID: "publish", Name: "连载排期、发布与回执", InputRefs: []string{"package"}, CapabilityID: "channel.publish", ExecutorKinds: []string{"channel_adapter", "browser", "human"}, OutputSchema: "contentcloud.channel-publication/1.0", Checks: []string{"channel.preview"}, GateIDs: []string{"release_preview"}, RetryMaxAttempts: 1},
		},
	},
}

func (v Profile) Validate() error {
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.Version) == "" || strings.TrimSpace(v.Name) == "" || len(v.ContentTypes) == 0 || len(v.Stages) == 0 || strings.TrimSpace(v.DeliverySchema) == "" {
		return domain.Invalid("CONTENT_PROFILE_INVALID", "内容 Profile 缺少 ID、版本、内容类型、Stage 或交付 Schema")
	}
	gates := map[string]bool{}
	for _, gate := range v.RequiredGates {
		if strings.TrimSpace(gate) == "" || gates[gate] {
			return domain.Invalid("CONTENT_PROFILE_GATE_INVALID", "内容 Profile Gate 为空或重复")
		}
		gates[gate] = true
	}
	stages := map[string]bool{}
	for _, stage := range v.Stages {
		if stage.ID == "" || stage.Name == "" || stage.CapabilityID == "" || stage.OutputSchema == "" || len(stage.ExecutorKinds) == 0 || stages[stage.ID] {
			return domain.Invalid("CONTENT_PROFILE_STAGE_INVALID", "内容 Profile Stage 缺少唯一 ID、能力、执行者或输出 Schema")
		}
		stages[stage.ID] = true
		for _, gate := range stage.GateIDs {
			if !gates[gate] {
				return domain.Invalid("CONTENT_PROFILE_GATE_REFERENCE_INVALID", "内容 Profile Stage 引用了未知 Gate")
			}
		}
	}
	return nil
}

func withDigest(value Profile) (Profile, error) {
	value.Digest = ""
	if err := value.Validate(); err != nil {
		return value, err
	}
	digest, err := domain.CanonicalHash(value)
	if err != nil {
		return value, err
	}
	value.Digest = "sha256:" + digest
	return value, nil
}

func Get(id string) (Profile, bool) {
	value, ok := builtins[strings.ToLower(strings.TrimSpace(id))]
	if !ok {
		return Profile{}, false
	}
	value, err := withDigest(value)
	return value, err == nil
}

func List() []Profile {
	ids := make([]string, 0, len(builtins))
	for id := range builtins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	values := make([]Profile, 0, len(ids))
	for _, id := range ids {
		if value, ok := Get(id); ok {
			values = append(values, value)
		}
	}
	return values
}
