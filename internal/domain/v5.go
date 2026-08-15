package domain

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

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
		return Invalid("AUDIENCE_TAXONOMY_IDENTITY_INVALID", "人群目录缺少稳定 ID、类型、Schema、平台或版本")
	}
	if len(v.Segments) != 8 || !validAudienceSegments(v.Segments) {
		return Invalid("AUDIENCE_TAXONOMY_SEGMENTS_INVALID", "八大人群目录必须包含恰好 8 个代码、名称和定义均有效且不重复的分群")
	}
	if strings.TrimSpace(v.SourceURL) == "" || v.CapturedAt.IsZero() || v.EffectiveFrom.IsZero() || v.ExpiresAt.IsZero() || !v.ExpiresAt.After(v.EffectiveFrom) || !sha256Pattern.MatchString(v.SourceSHA256) {
		return Invalid("AUDIENCE_TAXONOMY_PROVENANCE_INVALID", "人群目录来源、采集时间、有效期或 SHA-256 无效")
	}
	if v.VerificationStatus != "unverified" && v.VerificationStatus != "human_verified" && v.VerificationStatus != "expired" {
		return Invalid("AUDIENCE_TAXONOMY_VERIFICATION_INVALID", "人群目录 verification_status 无效")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "deprecated" {
		return Invalid("AUDIENCE_TAXONOMY_STATUS_INVALID", "人群目录 status 无效")
	}
	if requireReviewReady && (v.Status != "review_ready" || v.VerificationStatus != "human_verified" || !now.Before(v.ExpiresAt)) {
		return Policy("AUDIENCE_TAXONOMY_NOT_REVIEW_READY", "只有人工验证且未过期的人群目录可以发布审核", "更新来源和有效期，并将状态设为 review_ready")
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
		return Invalid("AUDIENCE_STRATEGY_IDENTITY_INVALID", "人群策略缺少稳定 ID、Schema、项目或目录引用")
	}
	for _, value := range []string{v.SegmentDefinition, v.Objective, v.DemandMoment, v.InsightStatement, v.Scenario, v.CTAStrategy} {
		if strings.TrimSpace(value) == "" {
			return Invalid("AUDIENCE_STRATEGY_FIELD_REQUIRED", "人群策略定义、目标、需求时刻、洞察、场景和 CTA 必填")
		}
	}
	if len(v.HookHypotheses) == 0 || len(v.ProofOrder) == 0 || len(v.TargetMetrics) == 0 || !uniqueNonEmpty(v.EvidenceRefs) || !uniqueNonEmpty(v.ControlledVariables) || !uniqueNonEmpty(v.TargetMetrics) {
		return Invalid("AUDIENCE_STRATEGY_ARRAY_INVALID", "人群策略数组缺失、含空值或重复值")
	}
	if v.Confidence != "low" && v.Confidence != "medium" && v.Confidence != "high" {
		return Invalid("AUDIENCE_STRATEGY_CONFIDENCE_INVALID", "confidence 只允许 low、medium 或 high")
	}
	if !validTestType(v.TestType) || !validExperimentVariable(v.PrimaryVariable) || containsValue(v.ControlledVariables, v.PrimaryVariable) {
		return Invalid("AUDIENCE_STRATEGY_EXPERIMENT_INVALID", "测试类型、主变量或受控变量无效")
	}
	if v.TestType == "strict_ab" && len(v.ControlledVariables) == 0 {
		return Invalid("AUDIENCE_STRATEGY_CONTROLS_REQUIRED", "strict_ab 必须明确受控变量")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "deprecated" {
		return Invalid("AUDIENCE_STRATEGY_STATUS_INVALID", "人群策略 status 无效")
	}
	if requireReviewReady {
		if v.Status != "review_ready" {
			return Policy("AUDIENCE_STRATEGY_NOT_REVIEW_READY", "只有 review_ready 人群策略可以发布审核", "补齐证据与策略字段后重试")
		}
		if len(v.EvidenceRefs) == 0 || v.Confidence == "low" {
			return Policy("AUDIENCE_STRATEGY_EVIDENCE_INSUFFICIENT", "review_ready 人群策略必须有当前证据且置信度不能为 low", "补充项目或平台证据")
		}
	}
	return validateOptionalContentHash(v, v.ContentHash)
}

func (v AudienceStrategyVersion) ValidateAgainstTaxonomy(taxonomy AudienceTaxonomySnapshot, now time.Time) error {
	if v.TaxonomySnapshotID != taxonomy.ID {
		return Conflict("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_INVALID", "AudienceStrategyVersion 未引用所提供的 taxonomy 基线")
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
	return Conflict("AUDIENCE_STRATEGY_TAXONOMY_MISMATCH", "AudienceStrategyVersion 的人群代码、名称或定义与批准 taxonomy 不一致")
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
		return Invalid("COMMERCE_OFFER_IDENTITY_INVALID", "Offer 缺少稳定 ID、Schema、商品版本、价格或币种")
	}
	if len(v.EvidenceRefs) == 0 || !uniqueNonEmpty(v.EvidenceRefs) || !uniqueNonEmpty(v.ApprovedClaimRefs) || v.CapturedAt.IsZero() || v.ValidFrom.IsZero() || v.ValidUntil.IsZero() || !v.ValidUntil.After(v.ValidFrom) {
		return Invalid("COMMERCE_OFFER_PROVENANCE_INVALID", "Offer 必须包含有效证据和时间窗口")
	}
	if v.Status != "candidate" && v.Status != "verified" && v.Status != "expired" && v.Status != "revoked" {
		return Invalid("COMMERCE_OFFER_STATUS_INVALID", "Offer status 无效")
	}
	if requireVerified && (v.Status != "verified" || at.Before(v.ValidFrom) || !at.Before(v.ValidUntil)) {
		return Policy("COMMERCE_OFFER_NOT_VALID", "Offer 未验证、尚未生效或已过期", "更新并人工验证 OfferSnapshot")
	}
	return nil
}

type CapabilityRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (v CapabilityRef) Validate() error {
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.Version) == "" || !strings.HasPrefix(v.Digest, "sha256:") || !sha256Pattern.MatchString(v.Digest) {
		return Invalid("CAPABILITY_REF_INVALID", "能力引用需要 ID、版本和带前缀的 SHA-256")
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
		return Invalid("STORYBOARD_IDENTITY_INVALID", "分镜包缺少稳定 ID、Schema、项目、批准快照或 ContentItem")
	}
	if err := v.GeneratorCapability.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(v.SourceDigest, "sha256:") || !sha256Pattern.MatchString(v.SourceDigest) || !strings.HasPrefix(v.LockedDigest, "sha256:") || !sha256Pattern.MatchString(v.LockedDigest) {
		return Invalid("STORYBOARD_DIGEST_INVALID", "分镜包 source_digest 或 locked_digest 无效")
	}
	if v.Status != "candidate" && v.Status != "review_ready" && v.Status != "superseded" {
		return Invalid("STORYBOARD_STATUS_INVALID", "分镜包 status 无效")
	}
	if len(v.Shots) == 0 {
		return Invalid("STORYBOARD_SHOTS_REQUIRED", "分镜包必须包含至少一个镜头")
	}
	assetIndex := map[string]StoryboardAsset{}
	for _, asset := range v.Assets {
		if err := asset.Validate(); err != nil {
			return err
		}
		if _, exists := assetIndex[asset.ID]; exists {
			return Invalid("STORYBOARD_ASSET_DUPLICATE", "分镜素材 ID 不能重复")
		}
		assetIndex[asset.ID] = asset
	}
	shotIDs := map[string]bool{}
	for _, shot := range v.Shots {
		if err := shot.Validate(assetIndex, requireReviewReady); err != nil {
			return err
		}
		if shotIDs[shot.ShotID] {
			return Invalid("STORYBOARD_SHOT_DUPLICATE", "分镜 shot_id 不能重复")
		}
		shotIDs[shot.ShotID] = true
	}
	if requireReviewReady && (v.Status != "review_ready" || strings.TrimSpace(v.ReviewSheetArtifactID) == "") {
		return Policy("STORYBOARD_NOT_REVIEW_READY", "发布审核前必须生成 review sheet 并将分镜设为 review_ready", "补齐独立首尾帧和审核接触图")
	}
	if v.ReviewSheetArtifactID != "" {
		asset, ok := assetIndex[v.ReviewSheetArtifactID]
		if !ok || asset.Role != "review_sheet" {
			return Invalid("STORYBOARD_REVIEW_SHEET_INVALID", "review_sheet_artifact_id 必须引用 review_sheet 素材")
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
	hash, err := CanonicalHash(v)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (v StoryboardAsset) Validate() error {
	clean := path.Clean(strings.TrimSpace(v.Path))
	if strings.TrimSpace(v.ID) == "" || !validStoryboardAssetRole(v.Role) || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) || strings.TrimSpace(v.MediaType) == "" || !sha256Pattern.MatchString(v.SHA256) || strings.HasPrefix(v.SHA256, "sha256:") || v.ByteSize < 0 || !uniqueNonEmpty(v.RightsRefs) {
		return Invalid("STORYBOARD_ASSET_INVALID", "分镜素材缺少安全相对路径、类型、摘要、大小或权利引用")
	}
	if (v.Role == "first_frame" || v.Role == "end_frame") && strings.TrimSpace(v.ShotID) == "" {
		return Invalid("STORYBOARD_ASSET_SHOT_REQUIRED", "首尾帧素材必须引用 shot_id")
	}
	return nil
}

func (v StoryboardShot) Validate(assets map[string]StoryboardAsset, requireMedia bool) error {
	if !storyboardShotIDPattern.MatchString(v.ShotID) || v.StartMS < 0 || v.EndMS <= v.StartMS || strings.TrimSpace(v.Role) == "" || strings.TrimSpace(v.ImagePromptZH) == "" || strings.TrimSpace(v.PlanB) == "" || len(v.NegativeConstraints) == 0 || len(v.AcceptanceCriteria) == 0 {
		return Invalid("STORYBOARD_SHOT_INVALID", "分镜镜头缺少 ID、时间、提示词、禁止项、验收或 Plan B")
	}
	if requireMedia {
		first, ok := assets[v.FirstFrameArtifactID]
		if !ok || first.Role != "first_frame" || first.ShotID != v.ShotID {
			return Policy("STORYBOARD_FIRST_FRAME_REQUIRED", "review_ready 镜头必须引用自己的独立首帧素材", "生成并登记首帧后重试")
		}
		if v.EndFrameArtifactID != "" {
			end, ok := assets[v.EndFrameArtifactID]
			if !ok || end.Role != "end_frame" || end.ShotID != v.ShotID {
				return Invalid("STORYBOARD_END_FRAME_INVALID", "尾帧必须引用同一镜头的 end_frame 素材")
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
	if strings.TrimSpace(v.ID) == "" || v.Type != "seedance_prompt_package" || v.SchemaVersion != SeedancePromptPackageSchema || strings.TrimSpace(v.StoryboardSnapshotID) == "" || strings.TrimSpace(v.StoryboardPackageID) == "" || !strings.HasPrefix(v.StoryboardLockedDigest, "sha256:") || !sha256Pattern.MatchString(v.StoryboardLockedDigest) || v.Provider != "seedance" || strings.TrimSpace(v.ProviderProfileVersion) == "" {
		return Invalid("SEEDANCE_PACKAGE_IDENTITY_INVALID", "Seedance 包缺少 ID、锁定分镜、Provider Profile 或摘要")
	}
	if err := v.AdapterCapability.Validate(); err != nil {
		return err
	}
	if v.Mode != "text_to_video" && v.Mode != "image_to_video" && v.Mode != "first_last_frame" && v.Mode != "all_reference" && v.Mode != "extend" {
		return Invalid("SEEDANCE_MODE_INVALID", "Seedance 模式无效")
	}
	if !validAspect(v.Settings.AspectRatio) || v.Settings.DurationSeconds < 1 || (v.Mode != "text_to_video" && len(v.UploadManifest) == 0) || len(v.Segments) == 0 {
		return Invalid("SEEDANCE_PACKAGE_CONTENT_INVALID", "Seedance 设置、上传清单或分段缺失")
	}
	references := map[string]bool{}
	for _, upload := range v.UploadManifest {
		cleanFile := path.Clean(strings.TrimSpace(upload.File))
		matchedReference := seedanceReferencePattern.FindString(upload.Reference)
		if matchedReference != upload.Reference || strings.TrimSpace(upload.ArtifactID) == "" || cleanFile == "." || strings.HasPrefix(cleanFile, "../") || strings.HasPrefix(cleanFile, "/") || strings.Contains(cleanFile, `\`) || !sha256Pattern.MatchString(upload.SHA256) || strings.HasPrefix(upload.SHA256, "sha256:") || references[upload.Reference] {
			return Invalid("SEEDANCE_UPLOAD_INVALID", "Seedance 上传项缺失、摘要无效或引用重复")
		}
		references[upload.Reference] = true
	}
	for index, segment := range v.Segments {
		if strings.TrimSpace(segment.ID) == "" || segment.Order != index+1 || segment.EndMS <= segment.StartMS || strings.TrimSpace(segment.PromptZH) == "" || len(segment.AcceptanceCriteria) == 0 {
			return Invalid("SEEDANCE_SEGMENT_INVALID", "Seedance 分段顺序、时间或提示词无效")
		}
		used := seedanceReferencePattern.FindAllString(segment.PromptZH, -1)
		if v.Mode != "text_to_video" && len(used) == 0 {
			return Invalid("SEEDANCE_SEGMENT_REFERENCE_REQUIRED", "每个 Seedance 分段必须引用至少一个已上传素材")
		}
		for _, reference := range used {
			if !references[reference] {
				return Invalid("SEEDANCE_SEGMENT_REFERENCE_UNKNOWN", "Seedance 提示词包含未映射引用："+reference)
			}
		}
	}
	if v.Status != "draft" && v.Status != "validated" && v.Status != "exported" && v.Status != "stale" && v.Status != "superseded" {
		return Invalid("SEEDANCE_STATUS_INVALID", "Seedance package status 无效")
	}
	if v.Status == "validated" || v.Status == "exported" {
		if !v.Validation.ReferencesChecked || !v.Validation.LimitsChecked || !v.Validation.RightsChecked || !v.Validation.OfferChecked || !v.Validation.DigestChecked {
			return Policy("SEEDANCE_VALIDATION_INCOMPLETE", "validated/exported Seedance 包必须通过全部门禁", "重新运行 package validator")
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
	if strings.TrimSpace(v.ID) == "" || v.SchemaVersion != PublishedCreativeBindingSchema || strings.TrimSpace(v.ProjectID) == "" || strings.TrimSpace(v.DeliveryPackageID) == "" || strings.TrimSpace(v.RenderedCreativeArtifactID) == "" || v.Platform != "douyin" || strings.TrimSpace(v.AccountAlias) == "" || (strings.TrimSpace(v.PlatformCreativeID) == "" && strings.TrimSpace(v.PlatformPostID) == "") || strings.TrimSpace(v.AudienceStrategyVersionID) == "" || strings.TrimSpace(v.ExperimentID) == "" || strings.TrimSpace(v.ExperimentArmID) == "" || !validTestType(v.TestType) || v.PublishedAt.IsZero() || !strings.HasPrefix(v.BindingHash, "sha256:") || !sha256Pattern.MatchString(v.BindingHash) {
		return Invalid("PUBLISHED_CREATIVE_BINDING_INVALID", "发布绑定缺少交付、成片、平台、人群、实验、时间或摘要")
	}
	computed, err := v.ComputedHash()
	if err != nil {
		return err
	}
	if normalizeHash(v.BindingHash) != computed {
		return Conflict("PUBLISHED_CREATIVE_BINDING_HASH_MISMATCH", "发布绑定摘要与服务端复算不一致")
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
	return CanonicalHash(value)
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
	if !strings.HasPrefix(contentHash, "sha256:") || !sha256Pattern.MatchString(contentHash) {
		return Invalid("AUDIENCE_STRATEGY_HASH_INVALID", "content_hash 必须是带前缀的 SHA-256")
	}
	value.ContentHash = ""
	computed, err := CanonicalHash(value)
	if err != nil {
		return err
	}
	if normalizeHash(contentHash) != computed {
		return Conflict("AUDIENCE_STRATEGY_HASH_MISMATCH", "人群策略 content_hash 与内容不一致")
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
