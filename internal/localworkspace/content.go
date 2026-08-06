package localworkspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/exportfmt"
	"gopkg.in/yaml.v3"
)

const (
	BriefSchema          = "contentcloud.brief/3.0"
	ContentBatchSchema   = "contentcloud.content-batch/3.0"
	ContentContextSchema = "contentcloud.content-context/3.0"
	ContentItemSchema    = "contentcloud.content-item/3.0"
)

type LocalBrief struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Status               string   `json:"status"`
	SchemaVersion        string   `json:"schema_version"`
	Deliverability       string   `json:"deliverability"`
	StrategyVersionID    string   `json:"strategy_version_id"`
	CampaignID           string   `json:"campaign_id"`
	ExperimentID         string   `json:"experiment_id"`
	Channel              string   `json:"channel"`
	Objective            string   `json:"objective"`
	Audience             string   `json:"audience"`
	Scenario             string   `json:"scenario"`
	DemandMoment         string   `json:"demand_moment"`
	PainPoint            string   `json:"pain_point"`
	PrimarySellingPoint  string   `json:"primary_selling_point"`
	SupportPoints        []string `json:"support_points"`
	Positioning          string   `json:"positioning"`
	VisualizationPlanIDs []string `json:"visualization_plan_ids"`
	AssetIDs             []string `json:"asset_ids"`
	TruthStrategy        string   `json:"truth_strategy"`
	PlanB                string   `json:"plan_b"`
	Tone                 string   `json:"tone"`
	BrandRuleIDs         []string `json:"brand_rule_ids"`
	ApprovedClaimIDs     []string `json:"approved_claim_ids"`
	ForbiddenClaims      []string `json:"forbidden_claims"`
	HookExpectation      string   `json:"hook_expectation"`
	NarrativeConstraints []string `json:"narrative_constraints"`
	CTA                  string   `json:"cta"`
	PrimaryVariable      string   `json:"primary_variable"`
	ControlledVariables  []string `json:"controlled_variables"`
	MeasurementWindow    string   `json:"measurement_window"`
	EligibleKnowledgeIDs []string `json:"eligible_knowledge_ids"`
	BlockedKnowledgeIDs  []string `json:"blocked_knowledge_ids"`
	RightsIDs            []string `json:"rights_ids"`
	RiskDecisionIDs      []string `json:"risk_decision_ids"`
	DurationMinMS        int      `json:"duration_min_ms"`
	DurationMaxMS        int      `json:"duration_max_ms"`
	AspectRatio          string   `json:"aspect_ratio"`
	BlockedReasons       []string `json:"blocked_reasons"`
	MissingInputs        []string `json:"missing_inputs"`
}

type CreativeDirection struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Angle         string   `json:"angle"`
	HookType      string   `json:"hook_type"`
	VisualMotif   string   `json:"visual_motif"`
	Narrative     []string `json:"narrative"`
	Tone          string   `json:"tone"`
	TargetEmotion string   `json:"target_emotion"`
	RiskRefs      []string `json:"risk_refs"`
	Status        string   `json:"status"`
}

type ContentBatch struct {
	SchemaVersion         string              `json:"schema_version" yaml:"schema_version"`
	ID                    string              `json:"id" yaml:"id"`
	IntentID              string              `json:"intent_id" yaml:"intent_id"`
	ContentKind           string              `json:"content_kind" yaml:"content_kind"`
	ContentSchemaRef      string              `json:"content_schema_ref" yaml:"content_schema_ref"`
	DeliveryProfiles      []string            `json:"delivery_profiles" yaml:"delivery_profiles"`
	BriefRef              string              `json:"brief_ref" yaml:"brief_ref"`
	KnowledgeSnapshotRefs []string            `json:"knowledge_snapshot_refs" yaml:"knowledge_snapshot_refs"`
	Status                string              `json:"status" yaml:"status"`
	Publishable           bool                `json:"publishable" yaml:"publishable"`
	ContentItemRefs       []string            `json:"content_item_refs" yaml:"content_item_refs"`
	BlockedReasons        []string            `json:"blocked_reasons" yaml:"blocked_reasons"`
	Checks                []ContentBatchCheck `json:"checks" yaml:"checks"`

	ProjectID            string          `json:"-" yaml:"-"`
	BriefSnapshotID      string          `json:"-" yaml:"-"`
	ContextSnapshotID    string          `json:"-" yaml:"-"`
	DirectionIDs         []string        `json:"-" yaml:"-"`
	RequestedCount       int             `json:"-" yaml:"-"`
	VariantDimension     string          `json:"-" yaml:"-"`
	ControlledDimensions []string        `json:"-" yaml:"-"`
	ContentHash          string          `json:"-" yaml:"-"`
	BriefRaw             json.RawMessage `json:"-" yaml:"-"`
	CreatedAt            time.Time       `json:"-" yaml:"-"`
	UpdatedAt            time.Time       `json:"-" yaml:"-"`
	ProducedAt           *time.Time      `json:"-" yaml:"-"`
}

type ContentBatchCheck struct {
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status" yaml:"status"`
}

type LocalContentContext struct {
	SchemaVersion        string                `json:"schema_version"`
	Batch                ContentBatch          `json:"batch"`
	ProjectID            string                `json:"project_id"`
	BriefSnapshotID      string                `json:"brief_snapshot_id"`
	ContextSnapshotID    string                `json:"context_snapshot_id"`
	DirectionIDs         []string              `json:"direction_ids"`
	RequestedCount       int                   `json:"requested_count"`
	VariantDimension     string                `json:"variant_dimension"`
	ControlledDimensions []string              `json:"controlled_dimensions"`
	ContentKind          string                `json:"content_kind"`
	ContentSchemaRef     string                `json:"content_schema_ref"`
	DeliveryProfiles     []string              `json:"delivery_profiles"`
	ContentHash          string                `json:"content_hash"`
	Brief                json.RawMessage       `json:"brief"`
	Plan                 json.RawMessage       `json:"plan"`
	Eligible             []KnowledgeQueryEntry `json:"eligible_knowledge"`
	Blocked              []KnowledgeQueryEntry `json:"blocked_knowledge"`
	GeneratedAt          time.Time             `json:"generated_at"`
}

type CreateContentBatchOptions struct {
	Root                 string
	BriefID              string
	DirectionsFile       string
	RequestedCount       int
	VariantDimension     string
	ControlledDimensions []string
	BatchID              string
	Now                  time.Time
}

type CreateContentBatchResult struct {
	BatchPath   string       `json:"batch_path"`
	ContextPath string       `json:"context_path"`
	Batch       ContentBatch `json:"batch"`
}

type ContentItem struct {
	ID                     string                        `json:"id"`
	Type                   string                        `json:"type"`
	Status                 string                        `json:"status"`
	SchemaVersion          string                        `json:"schema_version"`
	Deliverability         string                        `json:"deliverability"`
	ProjectID              string                        `json:"project_id"`
	ContentID              string                        `json:"content_id"`
	ContentBatchID         string                        `json:"content_batch_id"`
	BriefRef               string                        `json:"brief_ref"`
	ContextSnapshotID      string                        `json:"context_snapshot_id"`
	BasedOnVersionID       string                        `json:"based_on_version_id,omitempty"`
	ResolvedCommentIDs     []string                      `json:"resolved_comment_ids,omitempty"`
	ChangeSummary          string                        `json:"change_summary,omitempty"`
	Direction              CreativeDirection             `json:"direction"`
	Title                  string                        `json:"title"`
	Channel                string                        `json:"channel"`
	DurationMS             int                           `json:"duration_ms"`
	AspectRatio            string                        `json:"aspect_ratio"`
	Cover                  ContentCover                  `json:"cover"`
	NarrativeStructure     []ContentNarrativeSegment     `json:"narrative_structure"`
	Shots                  []ContentShot                 `json:"shots"`
	Citations              []ContentCitation             `json:"citations"`
	AssetRequirements      []ContentAssetRequirement     `json:"asset_requirements"`
	Experiment             ContentExperiment             `json:"experiment"`
	GlobalConstraints      ContentGlobalConstraints      `json:"global_constraints"`
	BlockedReasons         []ContentBlockedReason        `json:"blocked_reasons"`
	MissingInputs          []string                      `json:"missing_inputs"`
	ValidationDeclarations ContentValidationDeclarations `json:"validation_declarations"`
}

type ContentCover struct {
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle"`
	VisualIntent    string   `json:"visual_intent"`
	FirstViewSignal string   `json:"first_view_signal"`
	AssetRefs       []string `json:"asset_refs"`
	RightsRefs      []string `json:"rights_refs"`
	SafeArea        string   `json:"safe_area"`
	OcclusionGuards []string `json:"occlusion_guards"`
}

type ContentNarrativeSegment struct {
	Role             string   `json:"role"`
	Purpose          string   `json:"purpose"`
	StartMS          int      `json:"start_ms"`
	EndMS            int      `json:"end_ms"`
	DecisionFunction string   `json:"decision_function"`
	ShotIDs          []string `json:"shot_ids"`
}

type ContentFrame struct {
	VisualState string   `json:"visual_state"`
	PromptZH    string   `json:"prompt_zh"`
	AssetRefs   []string `json:"asset_refs"`
}

type ContentContinuity struct {
	IncomingState string   `json:"incoming_state"`
	OutgoingState string   `json:"outgoing_state"`
	MovementAxis  string   `json:"movement_axis"`
	LightingLock  string   `json:"lighting_lock"`
	ProductLock   string   `json:"product_lock"`
	Anchors       []string `json:"anchors"`
}

type ContentShot struct {
	ShotID               string            `json:"shot_id"`
	StartMS              int               `json:"start_ms"`
	EndMS                int               `json:"end_ms"`
	Role                 string            `json:"role"`
	NarrativePurpose     string            `json:"narrative_purpose"`
	Subject              string            `json:"subject"`
	VisualIntent         string            `json:"visual_intent"`
	SubjectAction        string            `json:"subject_action"`
	Composition          string            `json:"composition"`
	CameraMotion         string            `json:"camera_motion"`
	FirstFrame           ContentFrame      `json:"first_frame"`
	MotionSpec           string            `json:"motion_spec"`
	EndFrame             ContentFrame      `json:"end_frame"`
	Voiceover            string            `json:"voiceover"`
	OnScreenText         string            `json:"on_screen_text"`
	SoundIntent          string            `json:"sound_intent"`
	ProductionMode       string            `json:"production_mode"`
	KnowledgeRefs        []string          `json:"knowledge_refs"`
	ClaimRefs            []string          `json:"claim_refs"`
	AssetRefs            []string          `json:"asset_refs"`
	RightsRefs           []string          `json:"rights_refs"`
	VisualizationPlanID  string            `json:"visualization_plan_id,omitempty"`
	ProductTruthStrategy string            `json:"product_truth_strategy"`
	NegativeConstraints  []string          `json:"negative_constraints"`
	Continuity           ContentContinuity `json:"continuity"`
	AcceptanceCriteria   []string          `json:"acceptance_criteria"`
	PlanB                string            `json:"plan_b"`
}

type ContentCitation struct {
	KnowledgeID string `json:"knowledge_id"`
	ShotID      string `json:"shot_id"`
	Usage       string `json:"usage"`
}

type ContentAssetRequirement struct {
	AssetID       string `json:"asset_id"`
	RightsID      string `json:"rights_id"`
	Purpose       string `json:"purpose"`
	RequiredTruth string `json:"required_truth"`
	Fallback      string `json:"fallback"`
}

type ContentExperiment struct {
	PrimaryVariable     string   `json:"primary_variable"`
	ControlledVariables []string `json:"controlled_variables"`
	Hypothesis          string   `json:"hypothesis"`
	MeasurementWindow   string   `json:"measurement_window"`
	TargetMetrics       []string `json:"target_metrics"`
}

type ContentGlobalConstraints struct {
	ForbiddenClaims       []string `json:"forbidden_claims"`
	BrandRules            []string `json:"brand_rules"`
	ProductTruthRules     []string `json:"product_truth_rules"`
	ContinuityLocks       []string `json:"continuity_locks"`
	PlatformSafeAreaRules []string `json:"platform_safe_area_rules"`
}

type ContentBlockedReason struct {
	Code       string `json:"code"`
	ObjectID   string `json:"object_id,omitempty"`
	Message    string `json:"message"`
	OwnerRole  string `json:"owner_role"`
	NextAction string `json:"next_action"`
}

type ContentValidationDeclarations struct {
	SchemaChecked     bool `json:"schema_checked"`
	KnowledgeChecked  bool `json:"knowledge_checked"`
	RightsChecked     bool `json:"rights_checked"`
	ContinuityChecked bool `json:"continuity_checked"`
	ExperimentChecked bool `json:"experiment_checked"`
}

type ContentLintIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type ContentItemLintReport struct {
	Valid          bool               `json:"valid"`
	File           string             `json:"file"`
	ContentItemID  string             `json:"content_item_id,omitempty"`
	Deliverability string             `json:"deliverability,omitempty"`
	ContentHash    string             `json:"content_hash,omitempty"`
	Issues         []ContentLintIssue `json:"issues"`
}

type ContentBatchLintReport struct {
	Valid       bool                    `json:"valid"`
	BatchID     string                  `json:"batch_id"`
	Requested   int                     `json:"requested"`
	Received    int                     `json:"received"`
	ReviewReady int                     `json:"review_ready"`
	Blocked     int                     `json:"blocked"`
	Results     []ContentItemLintReport `json:"results"`
}

type FinalizeContentBatchResult struct {
	Batch  ContentBatch           `json:"batch"`
	Report ContentBatchLintReport `json:"report"`
}

type ContentItemDiff struct {
	Valid           bool     `json:"valid"`
	BaselineID      string   `json:"baseline_id"`
	CandidateID     string   `json:"candidate_id"`
	ChangedPaths    []string `json:"changed_paths"`
	AllowedPaths    []string `json:"allowed_paths"`
	UnexpectedPaths []string `json:"unexpected_paths"`
}

type ContentDeliveryFile struct {
	Format    string `json:"format"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	ByteSize  int64  `json:"byte_size"`
	MediaType string `json:"media_type"`
}

type ContentDeliveryManifest struct {
	SchemaVersion      string                `json:"schema_version"`
	ContentItemID      string                `json:"content_item_id"`
	ApprovedSnapshotID string                `json:"approved_snapshot_id"`
	ContentHash        string                `json:"content_hash"`
	Files              []ContentDeliveryFile `json:"files"`
	CreatedAt          time.Time             `json:"created_at"`
}

type RenderedContentFile struct {
	Format    string
	Name      string
	MediaType string
	Body      []byte
	SHA256    string
}

type RenderedContentDelivery struct {
	Item        ContentItem
	ContentHash string
	Files       []RenderedContentFile
}

func LintBrief(root, file string) (KnowledgeLintReport, LocalBrief, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return KnowledgeLintReport{}, LocalBrief{}, err
	}
	path, err := resolveWorkspaceFile(resolved, file)
	if err != nil {
		return KnowledgeLintReport{}, LocalBrief{}, err
	}
	var brief LocalBrief
	if err := readStrictJSON(path, &brief); err != nil {
		return KnowledgeLintReport{}, brief, domain.Invalid("BRIEF_JSON_INVALID", err.Error())
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: resolved, Channel: brief.Channel})
	if err != nil {
		return KnowledgeLintReport{}, brief, err
	}
	report := KnowledgeLintReport{Valid: true, ItemCount: 1, Issues: []KnowledgeLintIssue{}}
	add := func(code, message string) {
		report.Issues = append(report.Issues, KnowledgeLintIssue{Severity: "error", Code: code, ItemID: brief.ID, Path: relativeWorkspacePath(resolved, path), Message: message})
	}
	if brief.SchemaVersion != BriefSchema || brief.ID == "" || brief.Kind != "brief" || (brief.Status != "candidate" && brief.Status != "blocked") {
		add("BRIEF_IDENTITY_INVALID", "Brief 需要 V3 schema_version、稳定 id、kind=brief 和 candidate/blocked 状态")
	}
	if brief.Deliverability != "review_ready" && brief.Deliverability != "blocked" {
		add("BRIEF_DELIVERABILITY_INVALID", "deliverability 只允许 review_ready 或 blocked")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"strategy_version_id", brief.StrategyVersionID},
		{"campaign_id", brief.CampaignID},
		{"experiment_id", brief.ExperimentID},
		{"channel", brief.Channel},
		{"objective", brief.Objective},
		{"audience", brief.Audience},
		{"scenario", brief.Scenario},
		{"demand_moment", brief.DemandMoment},
		{"pain_point", brief.PainPoint},
		{"primary_selling_point", brief.PrimarySellingPoint},
		{"positioning", brief.Positioning},
		{"truth_strategy", brief.TruthStrategy},
		{"plan_b", brief.PlanB},
		{"tone", brief.Tone},
		{"hook_expectation", brief.HookExpectation},
		{"cta", brief.CTA},
		{"primary_variable", brief.PrimaryVariable},
		{"measurement_window", brief.MeasurementWindow},
		{"aspect_ratio", brief.AspectRatio},
	} {
		if strings.TrimSpace(field.value) == "" {
			add("BRIEF_FIELD_REQUIRED", field.name+" 必填")
		}
	}
	for _, field := range []struct {
		name  string
		value []string
	}{
		{"support_points", brief.SupportPoints},
		{"visualization_plan_ids", brief.VisualizationPlanIDs},
		{"asset_ids", brief.AssetIDs},
		{"brand_rule_ids", brief.BrandRuleIDs},
		{"approved_claim_ids", brief.ApprovedClaimIDs},
		{"forbidden_claims", brief.ForbiddenClaims},
		{"narrative_constraints", brief.NarrativeConstraints},
		{"controlled_variables", brief.ControlledVariables},
		{"eligible_knowledge_ids", brief.EligibleKnowledgeIDs},
		{"blocked_knowledge_ids", brief.BlockedKnowledgeIDs},
		{"rights_ids", brief.RightsIDs},
		{"risk_decision_ids", brief.RiskDecisionIDs},
		{"blocked_reasons", brief.BlockedReasons},
		{"missing_inputs", brief.MissingInputs},
	} {
		if field.value == nil {
			add("BRIEF_ARRAY_REQUIRED", field.name+" 必须显式为数组")
		}
	}
	if len(brief.SupportPoints) > 3 || !allUnique(brief.VisualizationPlanIDs) || !allUnique(brief.AssetIDs) || !allUnique(brief.BrandRuleIDs) || !allUnique(brief.ApprovedClaimIDs) || !allUnique(brief.ForbiddenClaims) || !allUnique(brief.ControlledVariables) || !allUnique(brief.EligibleKnowledgeIDs) || !allUnique(brief.BlockedKnowledgeIDs) || !allUnique(brief.RightsIDs) || !allUnique(brief.RiskDecisionIDs) {
		add("BRIEF_ARRAY_INVALID", "数组字段存在重复值，或 support_points 超过三项")
	}
	if brief.DurationMinMS < 1000 || brief.DurationMaxMS < brief.DurationMinMS || brief.DurationMaxMS > 600000 {
		add("BRIEF_DURATION_INVALID", "duration_min_ms/duration_max_ms 无效")
	}
	if !validVariantDimension(brief.PrimaryVariable) || !validAspectRatio(brief.AspectRatio) {
		add("BRIEF_ENUM_INVALID", "primary_variable 或 aspect_ratio 不受支持")
	}
	if containsString(brief.ControlledVariables, brief.PrimaryVariable) {
		add("BRIEF_EXPERIMENT_INVALID", "primary_variable 不能同时出现在 controlled_variables")
	}
	eligible := map[string]bool{}
	for _, entry := range query.Eligible {
		eligible[entry.Item.ID] = true
	}
	for _, id := range append(append(append([]string{}, brief.EligibleKnowledgeIDs...), brief.ApprovedClaimIDs...), brief.BrandRuleIDs...) {
		if !eligible[id] {
			add("BRIEF_KNOWLEDGE_NOT_ELIGIBLE", "Brief 引用未进入 ApprovedSnapshot 的知识："+id)
		}
	}
	for _, id := range brief.BlockedKnowledgeIDs {
		if containsString(brief.EligibleKnowledgeIDs, id) {
			add("BRIEF_KNOWLEDGE_CONFLICT", "同一知识不能同时 eligible 和 blocked："+id)
		}
	}
	if brief.Deliverability == "review_ready" {
		if brief.Status != "candidate" {
			add("BRIEF_REVIEW_READY_STATUS_INVALID", "review_ready Brief 必须保持 candidate 状态")
		}
		if len(brief.EligibleKnowledgeIDs) == 0 || len(brief.VisualizationPlanIDs) == 0 {
			add("BRIEF_INPUTS_INSUFFICIENT", "review_ready Brief 需要 eligible knowledge 和 visualization plan")
		}
		if len(brief.BlockedReasons) > 0 || len(brief.MissingInputs) > 0 {
			add("BRIEF_REVIEW_READY_BLOCKED", "review_ready Brief 不能保留 blocked_reasons 或 missing_inputs")
		}
	} else {
		if brief.Status != "blocked" || len(brief.BlockedReasons) == 0 {
			add("BRIEF_BLOCK_REASON_REQUIRED", "blocked Brief 必须使用 blocked 状态并说明 blocked_reasons")
		}
	}
	for range report.Issues {
		report.ErrorCount++
	}
	report.Valid = report.ErrorCount == 0
	return report, brief, nil
}

func CreateContentBatch(options CreateContentBatchOptions) (CreateContentBatchResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	briefRaw, briefSnapshot, err := latestApprovedObject(root, "brief", options.BriefID)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	var brief LocalBrief
	if err := strictUnmarshal(briefRaw, &brief); err != nil {
		return CreateContentBatchResult{}, domain.Invalid("APPROVED_BRIEF_INVALID", "批准快照中的 Brief V3 无效："+err.Error())
	}
	if brief.Deliverability != "review_ready" {
		return CreateContentBatchResult{}, domain.Policy("APPROVED_BRIEF_BLOCKED", "批准的 Brief 仍为 blocked，不能创建正式批次", "补齐输入并发布新的 Brief revision")
	}
	directionsPath, err := resolveWorkspaceFile(root, options.DirectionsFile)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	var directions []CreativeDirection
	if err := readStrictJSON(directionsPath, &directions); err != nil {
		return CreateContentBatchResult{}, domain.Invalid("CREATIVE_DIRECTIONS_INVALID", err.Error())
	}
	if len(directions) == 0 || len(directions) > 20 {
		return CreateContentBatchResult{}, domain.Invalid("CREATIVE_DIRECTIONS_COUNT_INVALID", "创意方向数量必须为 1 到 20")
	}
	selected := []CreativeDirection{}
	seen := map[string]bool{}
	for _, direction := range directions {
		if direction.ID == "" || direction.Title == "" || direction.Angle == "" || direction.HookType == "" || direction.VisualMotif == "" || direction.Tone == "" || direction.TargetEmotion == "" || len(direction.Narrative) == 0 || direction.RiskRefs == nil || !validDirectionStatus(direction.Status) || !allUnique(direction.RiskRefs) {
			return CreateContentBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_INVALID", "创意方向缺少必填字段或数组，或 status 无效")
		}
		if seen[direction.ID] {
			return CreateContentBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_DUPLICATE", "创意方向 ID 重复："+direction.ID)
		}
		seen[direction.ID] = true
		if direction.Status == "selected" {
			selected = append(selected, direction)
		}
	}
	if len(selected) == 0 {
		return CreateContentBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_SELECTION_REQUIRED", "至少选择一个创意方向")
	}
	count := options.RequestedCount
	if count == 0 {
		count = len(selected)
	}
	if count < 1 || count > 10 {
		return CreateContentBatchResult{}, domain.Invalid("CONTENT_BATCH_COUNT_INVALID", "requested_count 必须为 1 到 10")
	}
	variant := strings.TrimSpace(options.VariantDimension)
	if !validVariantDimension(variant) {
		return CreateContentBatchResult{}, domain.Invalid("CONTENT_BATCH_VARIANT_INVALID", "variant_dimension 只允许 hook、audience、scenario、visualization、cta 或 duration")
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root, Channel: brief.Channel})
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	if query.ApprovedSnapshotID == "" {
		return CreateContentBatchResult{}, domain.Policy("KNOWLEDGE_SNAPSHOT_REQUIRED", "创建正式剧本批次需要已拉取的知识批准快照", "先执行 contentcloud pull approved --type knowledge")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	directionIDs := []string{}
	for _, direction := range selected {
		directionIDs = append(directionIDs, direction.ID)
	}
	controlled := uniqueStrings(options.ControlledDimensions)
	if containsString(controlled, variant) {
		return CreateContentBatchResult{}, domain.Invalid("CONTENT_BATCH_EXPERIMENT_INVALID", "variant_dimension 不能同时被 controlled_dimensions 锁定")
	}
	deliveryProfiles := []string{"json", "markdown", "xlsx"}
	hashInput := map[string]any{"project_id": status.Binding.ProjectID, "content_kind": domain.ContentTypeVideoScript, "content_schema_ref": ContentItemSchema, "delivery_profiles": deliveryProfiles, "brief_id": brief.ID, "brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "direction_ids": directionIDs, "requested_count": count, "variant_dimension": variant, "controlled_dimensions": controlled}
	hash, err := domain.CanonicalHash(hashInput)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	batchID := strings.TrimSpace(options.BatchID)
	if batchID == "" {
		batchID = "content-batch-" + hash[:12]
	}
	if !localSourceIDPattern.MatchString(batchID) {
		return CreateContentBatchResult{}, domain.Invalid("CONTENT_BATCH_ID_INVALID", "batch ID 无效")
	}
	contextHash, _ := domain.CanonicalHash(map[string]any{"brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "eligible_ids": knowledgeEntryIDs(query.Eligible)})
	now := localNow(options.Now)
	intentID := brief.CampaignID
	if !strings.HasPrefix(intentID, "intent:") {
		intentID = "intent:" + intentID
	}
	batch := ContentBatch{
		SchemaVersion: ContentBatchSchema, ID: batchID, IntentID: intentID, ContentKind: domain.ContentTypeVideoScript, ContentSchemaRef: ContentItemSchema, DeliveryProfiles: deliveryProfiles, BriefRef: brief.ID, KnowledgeSnapshotRefs: []string{query.ApprovedSnapshotID}, Status: "candidate", Publishable: false,
		ContentItemRefs: []string{}, BlockedReasons: []string{"批次尚未完成本地内容校验"}, Checks: []ContentBatchCheck{{Name: "context_freeze", Status: "passed"}},
		ProjectID: status.Binding.ProjectID, BriefSnapshotID: briefSnapshot.ID, ContextSnapshotID: "project-context-" + contextHash[:12], DirectionIDs: directionIDs, RequestedCount: count, VariantDimension: variant,
		ControlledDimensions: controlled, ContentHash: "sha256:" + hash, CreatedAt: now, UpdatedAt: now,
	}
	briefJSON, err := json.Marshal(brief)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	planJSON, err := json.Marshal(selected)
	if err != nil {
		return CreateContentBatchResult{}, err
	}
	batchRoot := filepath.Join(root, "50-production", "batches", localSafeName(batchID))
	batchPath := filepath.Join(batchRoot, "manifest.yaml")
	if _, readErr := os.Stat(batchPath); readErr == nil {
		existing, loadErr := loadContentBatch(root, relativeWorkspacePath(root, batchPath), batchID)
		if loadErr == nil && existing.ContentHash == batch.ContentHash {
			return CreateContentBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, filepath.Join(batchRoot, "context.json")), Batch: existing}, nil
		}
		return CreateContentBatchResult{}, domain.Conflict("CONTENT_BATCH_IMMUTABLE_CONFLICT", "相同 batch ID 已存在不同内容")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return CreateContentBatchResult{}, readErr
	}
	context := LocalContentContext{
		SchemaVersion: ContentContextSchema, Batch: batch, ProjectID: batch.ProjectID, BriefSnapshotID: batch.BriefSnapshotID, ContextSnapshotID: batch.ContextSnapshotID,
		DirectionIDs: batch.DirectionIDs, RequestedCount: batch.RequestedCount, VariantDimension: batch.VariantDimension, ControlledDimensions: batch.ControlledDimensions,
		ContentKind: batch.ContentKind, ContentSchemaRef: batch.ContentSchemaRef, DeliveryProfiles: batch.DeliveryProfiles, ContentHash: batch.ContentHash,
		Brief: briefJSON, Plan: planJSON, Eligible: query.Eligible, Blocked: query.Blocked, GeneratedAt: now,
	}
	contextPath := filepath.Join(batchRoot, "context.json")
	if err := replaceYAML(batchPath, batch); err != nil {
		return CreateContentBatchResult{}, err
	}
	if err := replaceJSON(contextPath, context, 0o600); err != nil {
		return CreateContentBatchResult{}, err
	}
	return CreateContentBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, contextPath), Batch: batch}, nil
}

func LintContentItem(root, file, batchFile string) (ContentItemLintReport, ContentItem, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentItemLintReport{}, ContentItem{}, err
	}
	path, err := resolveWorkspaceFile(resolved, file)
	if err != nil {
		return ContentItemLintReport{}, ContentItem{}, err
	}
	var pkg ContentItem
	if err := readStrictJSON(path, &pkg); err != nil {
		return ContentItemLintReport{}, pkg, domain.Invalid("CONTENT_ITEM_JSON_INVALID", err.Error())
	}
	batch, err := loadContentBatch(resolved, batchFile, pkg.ContentBatchID)
	if err != nil {
		return ContentItemLintReport{}, pkg, err
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: resolved, Channel: pkg.Channel})
	if err != nil {
		return ContentItemLintReport{}, pkg, err
	}
	references, err := loadKnowledgeReferenceIndex(resolved)
	if err != nil {
		return ContentItemLintReport{}, pkg, err
	}
	report := lintContentItem(pkg, batch, query, references)
	report.File = relativeWorkspacePath(resolved, path)
	hash, hashErr := domain.CanonicalHash(pkg)
	if hashErr == nil {
		report.ContentHash = "sha256:" + hash
	}
	return report, pkg, nil
}

func LintContentBatch(root, batchFile string, contentFiles []string) (ContentBatchLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentBatchLintReport{}, err
	}
	batch, err := loadContentBatch(resolved, batchFile, "")
	if err != nil {
		return ContentBatchLintReport{}, err
	}
	report := ContentBatchLintReport{Valid: true, BatchID: batch.ID, Requested: batch.RequestedCount, Received: len(contentFiles), Results: []ContentItemLintReport{}}
	if len(contentFiles) != batch.RequestedCount {
		report.Valid = false
	}
	seen := map[string]bool{}
	for _, file := range contentFiles {
		item, pkg, err := LintContentItem(resolved, file, batchFile)
		if err != nil {
			return ContentBatchLintReport{}, err
		}
		if seen[pkg.ID] {
			item.Valid = false
			item.Issues = append(item.Issues, ContentLintIssue{Severity: "error", Code: "CONTENT_ITEM_ID_DUPLICATE", Path: "/id", Message: "批次内 ContentItem ID 重复"})
		}
		seen[pkg.ID] = true
		if !item.Valid {
			report.Valid = false
		}
		if pkg.Deliverability == "review_ready" {
			report.ReviewReady++
		} else if pkg.Deliverability == "blocked" {
			report.Blocked++
		}
		report.Results = append(report.Results, item)
	}
	return report, nil
}

func FinalizeContentBatch(root, batchFile string, contentFiles []string, now time.Time) (FinalizeContentBatchResult, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return FinalizeContentBatchResult{}, err
	}
	report, err := LintContentBatch(resolved, batchFile, contentFiles)
	if err != nil {
		return FinalizeContentBatchResult{}, err
	}
	if !report.Valid {
		err := domain.Invalid("CONTENT_BATCH_LINT_FAILED", "内容批次校验失败")
		err.Details = report
		return FinalizeContentBatchResult{}, err
	}
	batch, err := loadContentBatch(resolved, batchFile, report.BatchID)
	if err != nil {
		return FinalizeContentBatchResult{}, err
	}
	batch.Status = "review_ready"
	batch.Publishable = true
	batch.BlockedReasons = []string{}
	if report.Blocked > 0 {
		batch.Status = "blocked"
		batch.Publishable = false
		batch.BlockedReasons = []string{"批次包含 blocked ContentItem"}
	}
	if report.ReviewReady == 0 {
		batch.Status = "blocked"
		batch.Publishable = false
		batch.BlockedReasons = []string{"批次没有 review_ready ContentItem"}
	}
	files := []string{}
	for _, file := range contentFiles {
		path, err := resolveWorkspaceFile(resolved, file)
		if err != nil {
			return FinalizeContentBatchResult{}, err
		}
		files = append(files, relativeWorkspacePath(resolved, path))
	}
	at := localNow(now)
	batch.ContentItemRefs = uniqueStrings(files)
	batch.Checks = []ContentBatchCheck{{Name: "content_item_lint", Status: "passed"}, {Name: "batch_completeness", Status: "passed"}}
	batch.UpdatedAt = at
	batch.ProducedAt = &at
	path, err := resolveWorkspaceFile(resolved, batchFile)
	if err != nil {
		return FinalizeContentBatchResult{}, err
	}
	if err := replaceYAML(path, batch); err != nil {
		return FinalizeContentBatchResult{}, err
	}
	return FinalizeContentBatchResult{Batch: batch, Report: report}, nil
}

func DiffContentItems(root, baselineFile, candidateFile string, allowedPaths []string) (ContentItemDiff, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentItemDiff{}, err
	}
	baselinePath, err := resolveWorkspaceFile(resolved, baselineFile)
	if err != nil {
		return ContentItemDiff{}, err
	}
	candidatePath, err := resolveWorkspaceFile(resolved, candidateFile)
	if err != nil {
		return ContentItemDiff{}, err
	}
	var baseline, candidate ContentItem
	if err := readStrictJSON(baselinePath, &baseline); err != nil {
		return ContentItemDiff{}, domain.Invalid("CONTENT_ITEM_BASELINE_INVALID", err.Error())
	}
	if err := readStrictJSON(candidatePath, &candidate); err != nil {
		return ContentItemDiff{}, domain.Invalid("CONTENT_ITEM_CANDIDATE_INVALID", err.Error())
	}
	if candidate.BasedOnVersionID != baseline.ID || strings.TrimSpace(candidate.ChangeSummary) == "" {
		return ContentItemDiff{}, domain.Invalid("CONTENT_ITEM_REVISION_METADATA_INVALID", "修订稿必须用 based_on_version_id 引用基线 ID 并填写 change_summary")
	}
	baseBody, _ := json.Marshal(baseline)
	candidateBody, _ := json.Marshal(candidate)
	var left, right any
	_ = json.Unmarshal(baseBody, &left)
	_ = json.Unmarshal(candidateBody, &right)
	changes := []string{}
	collectJSONDiff("", left, right, &changes)
	bookkeeping := []string{"/id", "/status", "/based_on_version_id", "/resolved_comment_ids", "/change_summary"}
	allowed := uniqueStrings(append(append([]string{}, allowedPaths...), bookkeeping...))
	unexpected := []string{}
	for _, path := range changes {
		if !pathAllowed(path, allowed) {
			unexpected = append(unexpected, path)
		}
	}
	return ContentItemDiff{Valid: len(unexpected) == 0, BaselineID: baseline.ID, CandidateID: candidate.ID, ChangedPaths: changes, AllowedPaths: allowed, UnexpectedPaths: unexpected}, nil
}

func ExportApprovedContentItem(root, contentItemID, outputDirectory string, now time.Time) (ContentDeliveryManifest, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentDeliveryManifest{}, err
	}
	raw, snapshot, err := latestApprovedObject(resolved, "content_batch", contentItemID)
	if err != nil {
		return ContentDeliveryManifest{}, err
	}
	rendered, err := RenderContentItem(raw)
	if err != nil {
		return ContentDeliveryManifest{}, err
	}
	pkg := rendered.Item
	outputRoot := outputDirectory
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = filepath.Join(resolved, "60-delivery", "packages", localSafeName(pkg.ID))
	} else {
		if !filepath.IsAbs(outputRoot) {
			outputRoot = filepath.Join(resolved, filepath.FromSlash(outputRoot))
		}
		absolute, absErr := filepath.Abs(outputRoot)
		relative, relErr := filepath.Rel(resolved, absolute)
		if absErr != nil || relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return ContentDeliveryManifest{}, domain.Policy("DELIVERY_PATH_OUTSIDE_WORKSPACE", "交付目录必须位于当前工作区", "使用 outputs/delivery 下的目录")
		}
		outputRoot = absolute
	}
	files := []ContentDeliveryFile{}
	for _, file := range rendered.Files {
		path := filepath.Join(outputRoot, file.Name)
		if err := replaceFile(path, file.Body, 0o600); err != nil {
			return ContentDeliveryManifest{}, err
		}
		files = append(files, ContentDeliveryFile{Format: file.Format, Path: relativeWorkspacePath(resolved, path), SHA256: file.SHA256, ByteSize: int64(len(file.Body)), MediaType: file.MediaType})
	}
	manifest := ContentDeliveryManifest{SchemaVersion: "contentcloud.content-delivery/3.0", ContentItemID: pkg.ID, ApprovedSnapshotID: snapshot.ID, ContentHash: rendered.ContentHash, Files: files, CreatedAt: localNow(now)}
	if err := replaceJSON(filepath.Join(outputRoot, "manifest.json"), manifest, 0o600); err != nil {
		return ContentDeliveryManifest{}, err
	}
	return manifest, nil
}

func RenderContentItem(raw json.RawMessage) (RenderedContentDelivery, error) {
	var pkg ContentItem
	if err := strictUnmarshal(raw, &pkg); err != nil {
		return RenderedContentDelivery{}, domain.Invalid("APPROVED_CONTENT_ITEM_INVALID", "批准快照中的 ContentItem V3 无效："+err.Error())
	}
	if pkg.SchemaVersion != ContentItemSchema || pkg.Type != "content_item" || pkg.Deliverability != "review_ready" {
		return RenderedContentDelivery{}, domain.Policy("APPROVED_CONTENT_ITEM_BLOCKED", "只有处于 review_ready 状态的内容对象才能生成正式交付包", "修订并批准内容对象")
	}
	jsonBody, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	xlsx, err := renderContentItemXLSX(pkg)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	hash, err := domain.CanonicalHash(pkg)
	if err != nil {
		return RenderedContentDelivery{}, err
	}
	files := []RenderedContentFile{
		{Format: "json", Name: "content.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "content.md", MediaType: "text/markdown", Body: []byte(renderContentItemMarkdown(pkg))},
		{Format: "xlsx", Name: "content.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	files[1].SHA256 = digest(files[1].Body)
	return RenderedContentDelivery{Item: pkg, ContentHash: "sha256:" + hash, Files: files}, nil
}

func lintContentItem(pkg ContentItem, batch ContentBatch, query KnowledgeQueryResult, refs map[string]LocalKnowledgeItem) ContentItemLintReport {
	report := ContentItemLintReport{Valid: true, ContentItemID: pkg.ID, Deliverability: pkg.Deliverability, Issues: []ContentLintIssue{}}
	add := func(code, path, message string) {
		report.Issues = append(report.Issues, ContentLintIssue{Severity: "error", Code: code, Path: path, Message: message})
	}
	if batch.ContentKind != domain.ContentTypeVideoScript || batch.ContentSchemaRef != ContentItemSchema {
		add("CONTENT_BATCH_ROUTE_MISMATCH", "/content_batch_id", "视频剧本只能属于 video_script / contentcloud.content-item/3.0 批次")
	}
	if pkg.SchemaVersion != ContentItemSchema || pkg.ID == "" || pkg.Type != "content_item" || pkg.ContentID == "" {
		add("CONTENT_ITEM_IDENTITY_INVALID", "/", "ContentItem 需要 V3 schema_version、id、content_id 和 type=content_item")
	}
	if pkg.Deliverability != "review_ready" && pkg.Deliverability != "blocked" {
		add("CONTENT_ITEM_DELIVERABILITY_INVALID", "/deliverability", "deliverability 只允许 review_ready 或 blocked")
	}
	if pkg.ContentBatchID != batch.ID || pkg.BriefRef != batch.BriefRef || pkg.ContextSnapshotID != batch.ContextSnapshotID || pkg.ProjectID != batch.ProjectID {
		add("CONTENT_ITEM_BATCH_CONTEXT_MISMATCH", "/", "project/batch/brief/context 必须与 ContentBatch 冻结值一致")
	}
	if !containsString(batch.DirectionIDs, pkg.Direction.ID) || pkg.Direction.Status != "selected" {
		add("CONTENT_ITEM_DIRECTION_INVALID", "/direction", "direction 必须是批次中已选择的方向")
	}
	if pkg.Status != "candidate" && pkg.Status != "blocked" {
		add("CONTENT_ITEM_STATUS_INVALID", "/status", "本地 ContentItem 状态只允许 candidate 或 blocked")
	}
	if pkg.Title == "" || pkg.Channel == "" || pkg.DurationMS <= 0 || pkg.AspectRatio == "" {
		add("CONTENT_ITEM_TOP_LEVEL_REQUIRED", "/", "ContentItem 需要标题、渠道、时长和画幅")
	}
	if pkg.DurationMS > 600000 || !validAspectRatio(pkg.AspectRatio) {
		add("CONTENT_ITEM_TOP_LEVEL_INVALID", "/", "duration_ms 或 aspect_ratio 不受支持")
	}
	for _, field := range []struct {
		path    string
		missing bool
	}{
		{"/direction/narrative", pkg.Direction.Narrative == nil},
		{"/direction/risk_refs", pkg.Direction.RiskRefs == nil},
		{"/cover/asset_refs", pkg.Cover.AssetRefs == nil},
		{"/cover/rights_refs", pkg.Cover.RightsRefs == nil},
		{"/cover/occlusion_guards", pkg.Cover.OcclusionGuards == nil},
		{"/narrative_structure", pkg.NarrativeStructure == nil},
		{"/shots", pkg.Shots == nil},
		{"/citations", pkg.Citations == nil},
		{"/asset_requirements", pkg.AssetRequirements == nil},
		{"/experiment/controlled_variables", pkg.Experiment.ControlledVariables == nil},
		{"/experiment/target_metrics", pkg.Experiment.TargetMetrics == nil},
		{"/global_constraints/forbidden_claims", pkg.GlobalConstraints.ForbiddenClaims == nil},
		{"/global_constraints/brand_rules", pkg.GlobalConstraints.BrandRules == nil},
		{"/global_constraints/product_truth_rules", pkg.GlobalConstraints.ProductTruthRules == nil},
		{"/global_constraints/continuity_locks", pkg.GlobalConstraints.ContinuityLocks == nil},
		{"/global_constraints/platform_safe_area_rules", pkg.GlobalConstraints.PlatformSafeAreaRules == nil},
		{"/blocked_reasons", pkg.BlockedReasons == nil},
		{"/missing_inputs", pkg.MissingInputs == nil},
	} {
		if field.missing {
			add("CONTENT_ITEM_ARRAY_REQUIRED", field.path, "必填数组不能缺失")
		}
	}
	if pkg.Direction.ID == "" || pkg.Direction.Title == "" || pkg.Direction.Angle == "" || pkg.Direction.HookType == "" || pkg.Direction.VisualMotif == "" || len(pkg.Direction.Narrative) == 0 || !validDirectionStatus(pkg.Direction.Status) {
		add("CONTENT_ITEM_DIRECTION_SHAPE_INVALID", "/direction", "direction 缺少必填字段或 status 无效")
	}
	if pkg.Cover.Title == "" || pkg.Cover.VisualIntent == "" || pkg.Cover.FirstViewSignal == "" || pkg.Cover.SafeArea == "" {
		add("CONTENT_ITEM_COVER_REQUIRED", "/cover", "cover 缺少必要信息")
	}
	if !validVariantDimension(pkg.Experiment.PrimaryVariable) || pkg.Experiment.Hypothesis == "" || pkg.Experiment.MeasurementWindow == "" || !allUnique(pkg.Experiment.ControlledVariables) {
		add("CONTENT_ITEM_EXPERIMENT_SHAPE_INVALID", "/experiment", "experiment 缺少必要信息或 controlled_variables 重复")
	}
	if pkg.Deliverability == "blocked" {
		if pkg.Status != "blocked" || len(pkg.BlockedReasons) == 0 {
			add("CONTENT_ITEM_BLOCK_REASON_REQUIRED", "/blocked_reasons", "blocked 输出必须使用 blocked 状态并提供原因")
		}
		for index, reason := range pkg.BlockedReasons {
			if reason.Code == "" || reason.Message == "" || reason.OwnerRole == "" || reason.NextAction == "" {
				add("CONTENT_ITEM_BLOCK_REASON_INVALID", "/blocked_reasons/"+strconv.Itoa(index), "blocked reason 缺少 code/message/owner_role/next_action")
			}
		}
		report.Valid = len(report.Issues) == 0
		return report
	}
	if batch.Status != "candidate" && batch.Status != "blocked" && batch.Status != "review_ready" {
		add("CONTENT_BATCH_NOT_READY", "/content_batch_id", "ContentBatch 当前不能接收 review_ready 候选")
	}
	if pkg.Status != "candidate" {
		add("CONTENT_ITEM_REVIEW_READY_STATUS_INVALID", "/status", "review_ready ContentItem 必须保持 candidate 状态")
	}
	if len(pkg.BlockedReasons) > 0 || len(pkg.MissingInputs) > 0 {
		add("CONTENT_ITEM_REVIEW_READY_BLOCKED", "/blocked_reasons", "review_ready ContentItem 不能保留阻断原因或缺失输入")
	}
	if pkg.Experiment.PrimaryVariable != batch.VariantDimension || containsString(pkg.Experiment.ControlledVariables, pkg.Experiment.PrimaryVariable) {
		add("CONTENT_ITEM_EXPERIMENT_INVALID", "/experiment", "主要变量必须等于批次 variant_dimension，且不能同时被控制")
	}
	if len(pkg.Shots) == 0 {
		add("CONTENT_ITEM_SHOTS_REQUIRED", "/shots", "review_ready ContentItem 至少需要一个镜头")
	}
	eligible := map[string]bool{}
	for _, entry := range query.Eligible {
		eligible[entry.Item.ID] = true
	}
	shotIDs := map[string]bool{}
	shotKnowledgeRefs := map[string]map[string]bool{}
	roles := map[string]int{}
	expectedStart := 0
	previousOutgoing := ""
	for index, shot := range pkg.Shots {
		base := "/shots/" + strconv.Itoa(index)
		if shot.ShotID == "" || shotIDs[shot.ShotID] {
			add("CONTENT_ITEM_SHOT_ID_INVALID", base+"/shot_id", "shot_id 必填且批次内唯一")
		}
		shotIDs[shot.ShotID] = true
		shotKnowledgeRefs[shot.ShotID] = map[string]bool{}
		for _, id := range append(append([]string{}, shot.KnowledgeRefs...), shot.ClaimRefs...) {
			shotKnowledgeRefs[shot.ShotID][id] = true
		}
		roles[shot.Role]++
		if shot.StartMS != expectedStart || shot.EndMS <= shot.StartMS {
			add("CONTENT_ITEM_TIMELINE_INVALID", base, "镜头时间必须从 0 开始、连续且 end_ms 大于 start_ms")
		}
		expectedStart = shot.EndMS
		if index > 0 && previousOutgoing != "" && shot.Continuity.IncomingState != previousOutgoing {
			add("CONTENT_ITEM_CONTINUITY_HANDOFF_INVALID", base+"/continuity/incoming_state", "相邻镜头 incoming_state 必须等于上一镜头 outgoing_state")
		}
		previousOutgoing = shot.Continuity.OutgoingState
		for _, field := range []struct {
			name  string
			value string
		}{
			{"role", shot.Role},
			{"narrative_purpose", shot.NarrativePurpose},
			{"subject", shot.Subject},
			{"visual_intent", shot.VisualIntent},
			{"subject_action", shot.SubjectAction},
			{"composition", shot.Composition},
			{"camera_motion", shot.CameraMotion},
			{"first_frame.visual_state", shot.FirstFrame.VisualState},
			{"first_frame.prompt_zh", shot.FirstFrame.PromptZH},
			{"motion_spec", shot.MotionSpec},
			{"end_frame.visual_state", shot.EndFrame.VisualState},
			{"end_frame.prompt_zh", shot.EndFrame.PromptZH},
			{"sound_intent", shot.SoundIntent},
			{"product_truth_strategy", shot.ProductTruthStrategy},
			{"plan_b", shot.PlanB},
			{"continuity.incoming_state", shot.Continuity.IncomingState},
			{"continuity.outgoing_state", shot.Continuity.OutgoingState},
			{"continuity.movement_axis", shot.Continuity.MovementAxis},
			{"continuity.lighting_lock", shot.Continuity.LightingLock},
			{"continuity.product_lock", shot.Continuity.ProductLock},
		} {
			if strings.TrimSpace(field.value) == "" {
				add("CONTENT_ITEM_SHOT_FIELD_REQUIRED", base+"/"+field.name, field.name+" 必填")
			}
		}
		for _, field := range []struct {
			path  string
			value []string
		}{
			{"/first_frame/asset_refs", shot.FirstFrame.AssetRefs},
			{"/end_frame/asset_refs", shot.EndFrame.AssetRefs},
			{"/knowledge_refs", shot.KnowledgeRefs},
			{"/claim_refs", shot.ClaimRefs},
			{"/asset_refs", shot.AssetRefs},
			{"/rights_refs", shot.RightsRefs},
			{"/negative_constraints", shot.NegativeConstraints},
			{"/continuity/anchors", shot.Continuity.Anchors},
			{"/acceptance_criteria", shot.AcceptanceCriteria},
		} {
			if field.value == nil {
				add("CONTENT_ITEM_SHOT_ARRAY_REQUIRED", base+field.path, "必填数组不能缺失")
			}
		}
		if !validShotRole(shot.Role) {
			add("CONTENT_ITEM_SHOT_ROLE_INVALID", base+"/role", "role 不受支持")
		}
		if !validProductionMode(shot.ProductionMode) {
			add("CONTENT_ITEM_PRODUCTION_MODE_INVALID", base+"/production_mode", "production_mode 不受支持")
		}
		if len(shot.NegativeConstraints) == 0 || len(shot.AcceptanceCriteria) == 0 {
			add("CONTENT_ITEM_SHOT_GUARD_REQUIRED", base, "每个镜头都需要 negative_constraints 和 acceptance_criteria")
		}
		if shot.Role == "proof" && shot.VisualizationPlanID == "" {
			add("CONTENT_ITEM_PROOF_PLAN_REQUIRED", base+"/visualization_plan_id", "proof 镜头必须引用 VisualizationPlan")
		}
		if (shot.ProductionMode == "real_asset" || shot.ProductionMode == "asset_guided_generation" || shot.ProductionMode == "composite") && len(shot.AssetRefs) == 0 {
			add("CONTENT_ITEM_ASSET_REQUIRED", base+"/asset_refs", "当前 production_mode 需要真实素材引用")
		}
		for _, id := range append(append([]string{}, shot.KnowledgeRefs...), shot.ClaimRefs...) {
			if !eligible[id] {
				add("CONTENT_ITEM_KNOWLEDGE_NOT_ELIGIBLE", base+"/knowledge_refs", "引用知识未进入当前 Knowledge ApprovedSnapshot："+id)
			}
		}
		for _, id := range shot.AssetRefs {
			if _, ok := refs[id]; !ok {
				add("CONTENT_ITEM_ASSET_MISSING", base+"/asset_refs", "素材引用不存在："+id)
			}
		}
		for _, id := range shot.RightsRefs {
			if value, ok := refs[id]; !ok || (value.Status != "valid" && value.Status != "approved") {
				add("CONTENT_ITEM_RIGHTS_INVALID", base+"/rights_refs", "权利记录不可用："+id)
			}
		}
		if shot.Voiceover != "" && !hasShotCitation(pkg.Citations, shot.ShotID, "spoken_claim") {
			add("CONTENT_ITEM_SPOKEN_CITATION_REQUIRED", base+"/voiceover", "有口播的镜头需要 spoken_claim citation")
		}
		if shot.OnScreenText != "" && !hasShotCitation(pkg.Citations, shot.ShotID, "on_screen_text") {
			add("CONTENT_ITEM_TEXT_CITATION_REQUIRED", base+"/on_screen_text", "有屏幕文字的镜头需要 on_screen_text citation")
		}
	}
	if expectedStart != pkg.DurationMS {
		add("CONTENT_ITEM_DURATION_MISMATCH", "/duration_ms", "镜头总时长必须等于 duration_ms")
	}
	for _, role := range []string{"hook", "proof", "cta"} {
		if roles[role] == 0 {
			add("CONTENT_ITEM_REQUIRED_ROLE_MISSING", "/shots", "缺少必要叙事角色："+role)
		}
	}
	if roles["product_intro"]+roles["product_solution"] == 0 {
		add("CONTENT_ITEM_PRODUCT_ROLE_MISSING", "/shots", "缺少 product_intro 或 product_solution 镜头")
	}
	if roles["cta"] != 1 {
		add("CONTENT_ITEM_CTA_COUNT_INVALID", "/shots", "review_ready ContentItem 必须且只能有一个 CTA 镜头")
	}
	for index, citation := range pkg.Citations {
		if !eligible[citation.KnowledgeID] || !shotIDs[citation.ShotID] || !shotKnowledgeRefs[citation.ShotID][citation.KnowledgeID] || !validCitationUsage(citation.Usage) {
			add("CONTENT_ITEM_CITATION_INVALID", "/citations/"+strconv.Itoa(index), "citation 的 knowledge_id、shot_id 或 usage 无效")
		}
	}
	for index, requirement := range pkg.AssetRequirements {
		path := "/asset_requirements/" + strconv.Itoa(index)
		if requirement.AssetID == "" || requirement.RightsID == "" || requirement.Purpose == "" || requirement.RequiredTruth == "" || requirement.Fallback == "" {
			add("CONTENT_ITEM_ASSET_REQUIREMENT_INVALID", path, "asset requirement 缺少 asset_id/rights_id/purpose/required_truth/fallback")
		}
		if value, ok := refs[requirement.AssetID]; !ok || value.Kind != "asset" {
			add("CONTENT_ITEM_ASSET_REQUIREMENT_MISSING", path+"/asset_id", "asset requirement 引用的素材不存在："+requirement.AssetID)
		}
		if value, ok := refs[requirement.RightsID]; !ok || (value.Status != "valid" && value.Status != "approved") {
			add("CONTENT_ITEM_ASSET_REQUIREMENT_RIGHTS_INVALID", path+"/rights_id", "asset requirement 引用的权利记录不可用："+requirement.RightsID)
		}
	}
	for index, segment := range pkg.NarrativeStructure {
		path := "/narrative_structure/" + strconv.Itoa(index)
		if segment.Role == "" || segment.Purpose == "" || segment.DecisionFunction == "" || segment.EndMS <= segment.StartMS || segment.ShotIDs == nil {
			add("CONTENT_ITEM_NARRATIVE_INVALID", path, "叙事段缺少必要字段、时间无效或 shot_ids 缺失")
		}
		for _, shotID := range segment.ShotIDs {
			if !shotIDs[shotID] {
				add("CONTENT_ITEM_NARRATIVE_SHOT_MISSING", path+"/shot_ids", "叙事段引用了不存在的镜头："+shotID)
			}
		}
	}
	declarations := pkg.ValidationDeclarations
	if !declarations.SchemaChecked || !declarations.KnowledgeChecked || !declarations.RightsChecked || !declarations.ContinuityChecked || !declarations.ExperimentChecked {
		add("CONTENT_ITEM_VALIDATION_DECLARATION_MISSING", "/validation_declarations", "客户端必须声明五类确定性校验均已执行")
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func loadContentBatch(root, file, expectedID string) (ContentBatch, error) {
	path := file
	if strings.TrimSpace(path) == "" {
		if expectedID == "" {
			return ContentBatch{}, domain.Invalid("CONTENT_BATCH_FILE_REQUIRED", "必须指定内容批次 manifest")
		}
		path = filepath.ToSlash(filepath.Join("50-production", "batches", localSafeName(expectedID), "manifest.yaml"))
	}
	resolved, err := resolveWorkspaceFile(root, path)
	if err != nil {
		return ContentBatch{}, err
	}
	var batch ContentBatch
	if err := readYAML(resolved, &batch); err != nil {
		return ContentBatch{}, domain.Invalid("CONTENT_BATCH_INVALID", err.Error())
	}
	if batch.SchemaVersion != ContentBatchSchema || batch.ID == "" || batch.IntentID == "" || !domain.ValidTenantContentType(batch.ContentKind) || batch.ContentSchemaRef == "" || len(batch.DeliveryProfiles) == 0 || !allUnique(batch.DeliveryProfiles) || batch.BriefRef == "" || len(batch.KnowledgeSnapshotRefs) == 0 || batch.ContentItemRefs == nil || batch.BlockedReasons == nil || batch.Checks == nil || (expectedID != "" && batch.ID != expectedID) {
		return ContentBatch{}, domain.Invalid("CONTENT_BATCH_INVALID", "内容批次 manifest 标识或必填数组无效")
	}
	if batch.Publishable && (batch.Status != "review_ready" && batch.Status != "approved" && batch.Status != "delivered" || len(batch.BlockedReasons) > 0) {
		return ContentBatch{}, domain.Invalid("CONTENT_BATCH_INVALID", "可发布内容批次的状态或 blocked_reasons 无效")
	}
	if !batch.Publishable && len(batch.BlockedReasons) == 0 {
		return ContentBatch{}, domain.Invalid("CONTENT_BATCH_INVALID", "不可发布的内容批次必须说明 blocked_reasons")
	}
	contextPath := filepath.Join(filepath.Dir(resolved), "context.json")
	var frozen LocalContentContext
	if err := readStrictJSON(contextPath, &frozen); err != nil {
		return ContentBatch{}, domain.Invalid("CONTENT_CONTEXT_INVALID", err.Error())
	}
	if frozen.SchemaVersion != ContentContextSchema || frozen.Batch.ID != batch.ID || frozen.ProjectID == "" || frozen.BriefSnapshotID == "" || frozen.ContextSnapshotID == "" || frozen.RequestedCount < 1 || frozen.ContentKind != batch.ContentKind || frozen.ContentSchemaRef != batch.ContentSchemaRef || !slices.Equal(frozen.DeliveryProfiles, batch.DeliveryProfiles) {
		return ContentBatch{}, domain.Invalid("CONTENT_CONTEXT_INVALID", "内容批次冻结上下文无效")
	}
	batch.ProjectID = frozen.ProjectID
	batch.BriefSnapshotID = frozen.BriefSnapshotID
	batch.ContextSnapshotID = frozen.ContextSnapshotID
	batch.DirectionIDs = append([]string(nil), frozen.DirectionIDs...)
	batch.RequestedCount = frozen.RequestedCount
	batch.VariantDimension = frozen.VariantDimension
	batch.ControlledDimensions = append([]string(nil), frozen.ControlledDimensions...)
	batch.ContentHash = frozen.ContentHash
	batch.BriefRaw = append(json.RawMessage(nil), frozen.Brief...)
	batch.CreatedAt = frozen.GeneratedAt
	batch.UpdatedAt = frozen.GeneratedAt
	return batch, nil
}

func LoadContentBatch(root, file string) (ContentBatch, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ContentBatch{}, err
	}
	return loadContentBatch(resolved, file, "")
}

func replaceYAML(path string, value any) error {
	body, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return replaceFile(path, body, 0o600)
}

func latestApprovedObject(root, submissionType, objectID string) (json.RawMessage, domain.ApprovedSnapshot, error) {
	summaries, err := ApprovedSnapshotInbox(root, submissionType)
	if err != nil {
		return nil, domain.ApprovedSnapshot{}, err
	}
	for _, summary := range summaries {
		record, err := ShowApprovedSnapshot(root, summary.ID)
		if err != nil {
			return nil, domain.ApprovedSnapshot{}, err
		}
		snapshot := record.Snapshot
		eligible := map[string]bool{}
		for _, id := range snapshot.EligibleIDs {
			eligible[id] = true
		}
		var canonical struct {
			Objects json.RawMessage `json:"objects"`
		}
		if json.Unmarshal(snapshot.CanonicalContent, &canonical) != nil {
			continue
		}
		var objects []json.RawMessage
		if json.Unmarshal(canonical.Objects, &objects) != nil {
			continue
		}
		for _, raw := range objects {
			var identity struct {
				ID   string `json:"id"`
				Kind string `json:"kind"`
			}
			if json.Unmarshal(raw, &identity) != nil || identity.ID == "" || !eligible[identity.ID] {
				continue
			}
			if objectID == "" || identity.ID == objectID {
				return raw, snapshot, nil
			}
		}
	}
	return nil, domain.ApprovedSnapshot{}, domain.NotFound("已拉取的 " + submissionType + " ApprovedSnapshot 对象")
}

func validVariantDimension(value string) bool {
	switch value {
	case "hook", "audience", "scenario", "visualization", "cta", "duration":
		return true
	default:
		return false
	}
}

func validAspectRatio(value string) bool {
	switch value {
	case "9:16", "16:9", "1:1", "4:5":
		return true
	default:
		return false
	}
}

func validDirectionStatus(value string) bool {
	return value == "candidate" || value == "selected" || value == "rejected"
}

func validShotRole(value string) bool {
	switch value {
	case "hook", "context", "pain", "product_intro", "product_solution", "usage", "proof", "resolution", "payoff", "cta":
		return true
	default:
		return false
	}
}

func validProductionMode(value string) bool {
	switch value {
	case "real_asset", "asset_guided_generation", "generated_non_product", "composite", "external_capture":
		return true
	default:
		return false
	}
}

func validCitationUsage(value string) bool {
	return value == "spoken_claim" || value == "on_screen_text" || value == "visual_fact" || value == "style_rule"
}

func hasShotCitation(values []ContentCitation, shotID, usage string) bool {
	for _, value := range values {
		if value.ShotID == shotID && value.Usage == usage {
			return true
		}
	}
	return false
}

func knowledgeEntryIDs(values []KnowledgeQueryEntry) []string {
	result := []string{}
	for _, value := range values {
		result = append(result, value.Item.ID)
	}
	return result
}

func collectJSONDiff(path string, left, right any, result *[]string) {
	leftMap, leftOK := left.(map[string]any)
	rightMap, rightOK := right.(map[string]any)
	if leftOK && rightOK {
		keys := map[string]bool{}
		for key := range leftMap {
			keys[key] = true
		}
		for key := range rightMap {
			keys[key] = true
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			collectJSONDiff(path+"/"+escapeJSONPointer(key), leftMap[key], rightMap[key], result)
		}
		return
	}
	leftArray, leftOK := left.([]any)
	rightArray, rightOK := right.([]any)
	if leftOK && rightOK {
		length := len(leftArray)
		if len(rightArray) > length {
			length = len(rightArray)
		}
		for index := 0; index < length; index++ {
			var leftValue, rightValue any
			if index < len(leftArray) {
				leftValue = leftArray[index]
			}
			if index < len(rightArray) {
				rightValue = rightArray[index]
			}
			collectJSONDiff(path+"/"+strconv.Itoa(index), leftValue, rightValue, result)
		}
		return
	}
	leftBody, _ := json.Marshal(left)
	rightBody, _ := json.Marshal(right)
	if !bytes.Equal(leftBody, rightBody) {
		if path == "" {
			path = "/"
		}
		*result = append(*result, path)
	}
}

func pathAllowed(path string, allowed []string) bool {
	for _, prefix := range allowed {
		if path == prefix || strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/") {
			return true
		}
	}
	return false
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func renderContentItemMarkdown(pkg ContentItem) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", pkg.Title)
	fmt.Fprintf(&out, "- Content Item ID: `%s`\n- Schema: `%s`\n- 渠道: %s\n- 画幅: %s\n- 时长: %.1f 秒\n- 创意方向: %s\n- 主要变量: %s\n\n", pkg.ID, ContentItemSchema, pkg.Channel, pkg.AspectRatio, float64(pkg.DurationMS)/1000, pkg.Direction.Title, pkg.Experiment.PrimaryVariable)
	out.WriteString("## 封面\n\n")
	fmt.Fprintf(&out, "%s\n\n%s\n\n", pkg.Cover.Title, pkg.Cover.VisualIntent)
	out.WriteString("## 镜头表\n\n| 镜头 | 时码 | 功能 | 画面与动作 | 口播/字幕 | 制作方式 | 知识/素材 | 验收与 Plan B |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, shot := range pkg.Shots {
		visual := shot.VisualIntent + "；" + shot.SubjectAction + "；" + shot.Composition + "；" + shot.CameraMotion
		dialogue := shot.Voiceover + " / " + shot.OnScreenText
		refs := strings.Join(append(append([]string{}, shot.KnowledgeRefs...), shot.AssetRefs...), "、")
		acceptance := strings.Join(shot.AcceptanceCriteria, "；") + "；Plan B：" + shot.PlanB
		fmt.Fprintf(&out, "| %s | %.1f-%.1fs | %s | %s | %s | %s | %s | %s |\n", markdownCell(shot.ShotID), float64(shot.StartMS)/1000, float64(shot.EndMS)/1000, markdownCell(shot.Role), markdownCell(visual), markdownCell(dialogue), markdownCell(shot.ProductionMode), markdownCell(refs), markdownCell(acceptance))
	}
	out.WriteString("\n## 引用\n\n")
	for _, citation := range pkg.Citations {
		fmt.Fprintf(&out, "- `%s` -> `%s` (%s)\n", citation.KnowledgeID, citation.ShotID, citation.Usage)
	}
	return out.String()
}

func renderContentItemXLSX(pkg ContentItem) ([]byte, error) {
	rows := [][]string{{"镜头ID", "开始(ms)", "结束(ms)", "功能", "叙事目的", "主体", "画面意图", "主体动作", "构图", "相机运动", "首帧", "运动", "尾帧", "口播", "字幕", "声音", "制作方式", "知识引用", "素材", "权利", "可视化方案", "负面约束", "连续性", "真实性策略", "验收", "Plan B"}}
	for _, shot := range pkg.Shots {
		rows = append(rows, []string{shot.ShotID, strconv.Itoa(shot.StartMS), strconv.Itoa(shot.EndMS), shot.Role, shot.NarrativePurpose, shot.Subject, shot.VisualIntent, shot.SubjectAction, shot.Composition, shot.CameraMotion, shot.FirstFrame.PromptZH, shot.MotionSpec, shot.EndFrame.PromptZH, shot.Voiceover, shot.OnScreenText, shot.SoundIntent, shot.ProductionMode, strings.Join(shot.KnowledgeRefs, ","), strings.Join(shot.AssetRefs, ","), strings.Join(shot.RightsRefs, ","), shot.VisualizationPlanID, strings.Join(shot.NegativeConstraints, "；"), shot.Continuity.IncomingState + " -> " + shot.Continuity.OutgoingState, shot.ProductTruthStrategy, strings.Join(shot.AcceptanceCriteria, "；"), shot.PlanB})
	}
	return exportfmt.XLSX("镜头", rows)
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}
