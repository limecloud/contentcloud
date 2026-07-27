package localworkspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/exportfmt"
)

const ScriptPackageV2Schema = "contentcloud.script-package/2.0"

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

type CreativeBatch struct {
	ID                   string     `json:"id"`
	Kind                 string     `json:"kind"`
	Status               string     `json:"status"`
	SchemaVersion        string     `json:"schema_version"`
	ProjectID            string     `json:"project_id"`
	BriefVersionID       string     `json:"brief_version_id"`
	BriefSnapshotID      string     `json:"brief_snapshot_id"`
	KnowledgeSnapshotID  string     `json:"knowledge_snapshot_id"`
	ContextSnapshotID    string     `json:"context_snapshot_id"`
	DirectionIDs         []string   `json:"direction_ids"`
	RequestedCount       int        `json:"requested_count"`
	VariantDimension     string     `json:"variant_dimension"`
	ControlledDimensions []string   `json:"controlled_dimensions"`
	OutputSchema         string     `json:"output_schema"`
	DeliveryProfiles     []string   `json:"delivery_profiles"`
	BlockingReasons      []string   `json:"blocking_reasons"`
	ScriptFiles          []string   `json:"script_files"`
	ContentHash          string     `json:"content_hash"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ProducedAt           *time.Time `json:"produced_at,omitempty"`
}

type LocalScriptContext struct {
	SchemaVersion string                `json:"schema_version"`
	Batch         CreativeBatch         `json:"batch"`
	Brief         LocalBrief            `json:"brief"`
	Directions    []CreativeDirection   `json:"directions"`
	Eligible      []KnowledgeQueryEntry `json:"eligible_knowledge"`
	Blocked       []KnowledgeQueryEntry `json:"blocked_knowledge"`
	GeneratedAt   time.Time             `json:"generated_at"`
}

type CreateCreativeBatchOptions struct {
	Root                 string
	BriefID              string
	DirectionsFile       string
	RequestedCount       int
	VariantDimension     string
	ControlledDimensions []string
	BatchID              string
	Now                  time.Time
}

type CreateCreativeBatchResult struct {
	BatchPath   string        `json:"batch_path"`
	ContextPath string        `json:"context_path"`
	Batch       CreativeBatch `json:"batch"`
}

type ScriptPackageV2 struct {
	ID                     string                       `json:"id"`
	Kind                   string                       `json:"kind"`
	Status                 string                       `json:"status"`
	SchemaVersion          string                       `json:"schema_version"`
	Deliverability         string                       `json:"deliverability"`
	ProjectID              string                       `json:"project_id"`
	ScriptID               string                       `json:"script_id"`
	CreativeBatchID        string                       `json:"creative_batch_id"`
	BriefVersionID         string                       `json:"brief_version_id"`
	ContextSnapshotID      string                       `json:"context_snapshot_id"`
	BasedOnVersionID       string                       `json:"based_on_version_id,omitempty"`
	ResolvedCommentIDs     []string                     `json:"resolved_comment_ids,omitempty"`
	ChangeSummary          string                       `json:"change_summary,omitempty"`
	Direction              CreativeDirection            `json:"direction"`
	Title                  string                       `json:"title"`
	Channel                string                       `json:"channel"`
	DurationMS             int                          `json:"duration_ms"`
	AspectRatio            string                       `json:"aspect_ratio"`
	Cover                  ScriptCover                  `json:"cover"`
	NarrativeStructure     []NarrativeSegment           `json:"narrative_structure"`
	Shots                  []ScriptShotV2               `json:"shots"`
	Citations              []ScriptCitationV2           `json:"citations"`
	AssetRequirements      []ScriptAssetRequirement     `json:"asset_requirements"`
	Experiment             ScriptExperiment             `json:"experiment"`
	GlobalConstraints      ScriptGlobalConstraints      `json:"global_constraints"`
	BlockedReasons         []ScriptBlockedReason        `json:"blocked_reasons"`
	MissingInputs          []string                     `json:"missing_inputs"`
	ValidationDeclarations ScriptValidationDeclarations `json:"validation_declarations"`
}

type ScriptCover struct {
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle"`
	VisualIntent    string   `json:"visual_intent"`
	FirstViewSignal string   `json:"first_view_signal"`
	AssetRefs       []string `json:"asset_refs"`
	RightsRefs      []string `json:"rights_refs"`
	SafeArea        string   `json:"safe_area"`
	OcclusionGuards []string `json:"occlusion_guards"`
}

type NarrativeSegment struct {
	Role             string   `json:"role"`
	Purpose          string   `json:"purpose"`
	StartMS          int      `json:"start_ms"`
	EndMS            int      `json:"end_ms"`
	DecisionFunction string   `json:"decision_function"`
	ShotIDs          []string `json:"shot_ids"`
}

type ScriptFrameV2 struct {
	VisualState string   `json:"visual_state"`
	PromptZH    string   `json:"prompt_zh"`
	AssetRefs   []string `json:"asset_refs"`
}

type ScriptContinuityV2 struct {
	IncomingState string   `json:"incoming_state"`
	OutgoingState string   `json:"outgoing_state"`
	MovementAxis  string   `json:"movement_axis"`
	LightingLock  string   `json:"lighting_lock"`
	ProductLock   string   `json:"product_lock"`
	Anchors       []string `json:"anchors"`
}

type ScriptShotV2 struct {
	ShotID               string             `json:"shot_id"`
	StartMS              int                `json:"start_ms"`
	EndMS                int                `json:"end_ms"`
	Role                 string             `json:"role"`
	NarrativePurpose     string             `json:"narrative_purpose"`
	Subject              string             `json:"subject"`
	VisualIntent         string             `json:"visual_intent"`
	SubjectAction        string             `json:"subject_action"`
	Composition          string             `json:"composition"`
	CameraMotion         string             `json:"camera_motion"`
	FirstFrame           ScriptFrameV2      `json:"first_frame"`
	MotionSpec           string             `json:"motion_spec"`
	EndFrame             ScriptFrameV2      `json:"end_frame"`
	Voiceover            string             `json:"voiceover"`
	OnScreenText         string             `json:"on_screen_text"`
	SoundIntent          string             `json:"sound_intent"`
	ProductionMode       string             `json:"production_mode"`
	KnowledgeRefs        []string           `json:"knowledge_refs"`
	ClaimRefs            []string           `json:"claim_refs"`
	AssetRefs            []string           `json:"asset_refs"`
	RightsRefs           []string           `json:"rights_refs"`
	VisualizationPlanID  string             `json:"visualization_plan_id,omitempty"`
	ProductTruthStrategy string             `json:"product_truth_strategy"`
	NegativeConstraints  []string           `json:"negative_constraints"`
	Continuity           ScriptContinuityV2 `json:"continuity"`
	AcceptanceCriteria   []string           `json:"acceptance_criteria"`
	PlanB                string             `json:"plan_b"`
}

type ScriptCitationV2 struct {
	KnowledgeID string `json:"knowledge_id"`
	ShotID      string `json:"shot_id"`
	Usage       string `json:"usage"`
}

type ScriptAssetRequirement struct {
	AssetID       string `json:"asset_id"`
	RightsID      string `json:"rights_id"`
	Purpose       string `json:"purpose"`
	RequiredTruth string `json:"required_truth"`
	Fallback      string `json:"fallback"`
}

type ScriptExperiment struct {
	PrimaryVariable     string   `json:"primary_variable"`
	ControlledVariables []string `json:"controlled_variables"`
	Hypothesis          string   `json:"hypothesis"`
	MeasurementWindow   string   `json:"measurement_window"`
	TargetMetrics       []string `json:"target_metrics"`
}

type ScriptGlobalConstraints struct {
	ForbiddenClaims       []string `json:"forbidden_claims"`
	BrandRules            []string `json:"brand_rules"`
	ProductTruthRules     []string `json:"product_truth_rules"`
	ContinuityLocks       []string `json:"continuity_locks"`
	PlatformSafeAreaRules []string `json:"platform_safe_area_rules"`
}

type ScriptBlockedReason struct {
	Code       string `json:"code"`
	ObjectID   string `json:"object_id,omitempty"`
	Message    string `json:"message"`
	OwnerRole  string `json:"owner_role"`
	NextAction string `json:"next_action"`
}

type ScriptValidationDeclarations struct {
	SchemaChecked     bool `json:"schema_checked"`
	KnowledgeChecked  bool `json:"knowledge_checked"`
	RightsChecked     bool `json:"rights_checked"`
	ContinuityChecked bool `json:"continuity_checked"`
	ExperimentChecked bool `json:"experiment_checked"`
}

type ScriptLintIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

type ScriptLintReport struct {
	Valid          bool              `json:"valid"`
	File           string            `json:"file"`
	ScriptID       string            `json:"script_id,omitempty"`
	Deliverability string            `json:"deliverability,omitempty"`
	ContentHash    string            `json:"content_hash,omitempty"`
	Issues         []ScriptLintIssue `json:"issues"`
}

type ScriptBatchLintReport struct {
	Valid       bool               `json:"valid"`
	BatchID     string             `json:"batch_id"`
	Requested   int                `json:"requested"`
	Received    int                `json:"received"`
	ReviewReady int                `json:"review_ready"`
	Blocked     int                `json:"blocked"`
	Results     []ScriptLintReport `json:"results"`
}

type FinalizeCreativeBatchResult struct {
	Batch  CreativeBatch         `json:"batch"`
	Report ScriptBatchLintReport `json:"report"`
}

type ScriptDiff struct {
	Valid           bool     `json:"valid"`
	BaselineID      string   `json:"baseline_id"`
	CandidateID     string   `json:"candidate_id"`
	ChangedPaths    []string `json:"changed_paths"`
	AllowedPaths    []string `json:"allowed_paths"`
	UnexpectedPaths []string `json:"unexpected_paths"`
}

type ScriptDeliveryFile struct {
	Format    string `json:"format"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	ByteSize  int64  `json:"byte_size"`
	MediaType string `json:"media_type"`
}

type ScriptDeliveryManifest struct {
	SchemaVersion      string               `json:"schema_version"`
	ScriptID           string               `json:"script_id"`
	ApprovedSnapshotID string               `json:"approved_snapshot_id"`
	ScriptHash         string               `json:"script_hash"`
	Files              []ScriptDeliveryFile `json:"files"`
	CreatedAt          time.Time            `json:"created_at"`
}

type RenderedScriptFile struct {
	Format    string
	Name      string
	MediaType string
	Body      []byte
	SHA256    string
}

type RenderedScriptDelivery struct {
	Package    ScriptPackageV2
	ScriptHash string
	Files      []RenderedScriptFile
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
	if brief.SchemaVersion != "2.0" || brief.ID == "" || brief.Kind != "brief" || (brief.Status != "candidate" && brief.Status != "blocked") {
		add("BRIEF_IDENTITY_INVALID", "brief 需要 schema_version=2.0、稳定 id、kind=brief 和 candidate/blocked 状态")
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
	if strings.TrimSpace(brief.StrategyVersionID) != "" {
		if _, _, err := latestApprovedObject(resolved, "strategy", brief.StrategyVersionID); err != nil {
			if domain.IsNotFound(err) {
				add("BRIEF_STRATEGY_NOT_APPROVED", "Brief 引用未进入 strategy ApprovedSnapshot 的策略版本："+brief.StrategyVersionID+"，先执行 contentcloud pull approved --type strategy")
			} else {
				return KnowledgeLintReport{}, brief, err
			}
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

func CreateCreativeBatch(options CreateCreativeBatchOptions) (CreateCreativeBatchResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	briefRaw, briefSnapshot, err := latestApprovedObject(root, "brief", options.BriefID)
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	var brief LocalBrief
	if err := strictUnmarshal(briefRaw, &brief); err != nil {
		return CreateCreativeBatchResult{}, domain.Invalid("APPROVED_BRIEF_INVALID", "批准快照中的 Brief V2 无效："+err.Error())
	}
	if brief.Deliverability != "review_ready" {
		return CreateCreativeBatchResult{}, domain.Policy("APPROVED_BRIEF_BLOCKED", "批准的 Brief 仍为 blocked，不能创建正式批次", "补齐输入并发布新的 Brief revision")
	}
	directionsPath, err := resolveWorkspaceFile(root, options.DirectionsFile)
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	var directions []CreativeDirection
	if err := readStrictJSON(directionsPath, &directions); err != nil {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_DIRECTIONS_INVALID", err.Error())
	}
	if len(directions) == 0 || len(directions) > 20 {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_DIRECTIONS_COUNT_INVALID", "CreativeDirection 数量必须为 1 到 20")
	}
	selected := []CreativeDirection{}
	seen := map[string]bool{}
	for _, direction := range directions {
		if direction.ID == "" || direction.Title == "" || direction.Angle == "" || direction.HookType == "" || direction.VisualMotif == "" || direction.Tone == "" || direction.TargetEmotion == "" || len(direction.Narrative) == 0 || direction.RiskRefs == nil || !validDirectionStatus(direction.Status) || !allUnique(direction.RiskRefs) {
			return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_INVALID", "CreativeDirection 缺少必填字段、数组或 status 无效")
		}
		if seen[direction.ID] {
			return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_DUPLICATE", "CreativeDirection ID 重复："+direction.ID)
		}
		seen[direction.ID] = true
		if direction.Status == "selected" {
			selected = append(selected, direction)
		}
	}
	if len(selected) == 0 {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_DIRECTION_SELECTION_REQUIRED", "至少选择一个 CreativeDirection")
	}
	count := options.RequestedCount
	if count == 0 {
		count = len(selected)
	}
	if count < 1 || count > 10 {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_BATCH_COUNT_INVALID", "requested_count 必须为 1 到 10")
	}
	variant := strings.TrimSpace(options.VariantDimension)
	if !validVariantDimension(variant) {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_BATCH_VARIANT_INVALID", "variant_dimension 只允许 hook、audience、scenario、visualization、cta 或 duration")
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: root, Channel: brief.Channel})
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	if query.ApprovedSnapshotID == "" {
		return CreateCreativeBatchResult{}, domain.Policy("KNOWLEDGE_SNAPSHOT_REQUIRED", "创建正式剧本批次需要已拉取的 Knowledge ApprovedSnapshot", "先执行 contentcloud pull approved --type knowledge")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	directionIDs := []string{}
	for _, direction := range selected {
		directionIDs = append(directionIDs, direction.ID)
	}
	controlled := uniqueStrings(options.ControlledDimensions)
	if containsString(controlled, variant) {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_BATCH_EXPERIMENT_INVALID", "variant_dimension 不能同时被 controlled_dimensions 锁定")
	}
	hashInput := map[string]any{"project_id": status.Binding.ProjectID, "brief_id": brief.ID, "brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "direction_ids": directionIDs, "requested_count": count, "variant_dimension": variant, "controlled_dimensions": controlled}
	hash, err := domain.CanonicalHash(hashInput)
	if err != nil {
		return CreateCreativeBatchResult{}, err
	}
	batchID := strings.TrimSpace(options.BatchID)
	if batchID == "" {
		batchID = "creative-batch-" + hash[:12]
	}
	if !localSourceIDPattern.MatchString(batchID) {
		return CreateCreativeBatchResult{}, domain.Invalid("CREATIVE_BATCH_ID_INVALID", "batch ID 无效")
	}
	contextHash, _ := domain.CanonicalHash(map[string]any{"brief_snapshot_id": briefSnapshot.ID, "knowledge_snapshot_id": query.ApprovedSnapshotID, "eligible_ids": knowledgeEntryIDs(query.Eligible)})
	now := localNow(options.Now)
	batch := CreativeBatch{
		ID: batchID, Kind: "creative_batch", Status: "ready", SchemaVersion: "2.0", ProjectID: status.Binding.ProjectID, BriefVersionID: brief.ID, BriefSnapshotID: briefSnapshot.ID,
		KnowledgeSnapshotID: query.ApprovedSnapshotID, ContextSnapshotID: "project-context-" + contextHash[:12], DirectionIDs: directionIDs, RequestedCount: count, VariantDimension: variant,
		ControlledDimensions: controlled, OutputSchema: ScriptPackageV2Schema, DeliveryProfiles: []string{"json", "markdown", "xlsx"}, BlockingReasons: []string{}, ScriptFiles: []string{}, ContentHash: "sha256:" + hash, CreatedAt: now, UpdatedAt: now,
	}
	batchRoot := filepath.Join(root, "outputs", "scripts", localSafeName(batchID))
	batchPath := filepath.Join(batchRoot, "batch.json")
	if existingBody, readErr := os.ReadFile(batchPath); readErr == nil {
		var existing CreativeBatch
		if json.Unmarshal(existingBody, &existing) == nil && existing.ContentHash == batch.ContentHash {
			return CreateCreativeBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, filepath.Join(batchRoot, "context.json")), Batch: existing}, nil
		}
		return CreateCreativeBatchResult{}, domain.Conflict("CREATIVE_BATCH_IMMUTABLE_CONFLICT", "相同 batch ID 已存在不同内容")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return CreateCreativeBatchResult{}, readErr
	}
	context := LocalScriptContext{SchemaVersion: "2.0", Batch: batch, Brief: brief, Directions: selected, Eligible: query.Eligible, Blocked: query.Blocked, GeneratedAt: now}
	contextPath := filepath.Join(batchRoot, "context.json")
	if err := replaceJSON(batchPath, batch, 0o600); err != nil {
		return CreateCreativeBatchResult{}, err
	}
	if err := replaceJSON(contextPath, context, 0o600); err != nil {
		return CreateCreativeBatchResult{}, err
	}
	return CreateCreativeBatchResult{BatchPath: relativeWorkspacePath(root, batchPath), ContextPath: relativeWorkspacePath(root, contextPath), Batch: batch}, nil
}

func LintScriptPackage(root, file, batchFile string) (ScriptLintReport, ScriptPackageV2, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ScriptLintReport{}, ScriptPackageV2{}, err
	}
	path, err := resolveWorkspaceFile(resolved, file)
	if err != nil {
		return ScriptLintReport{}, ScriptPackageV2{}, err
	}
	var pkg ScriptPackageV2
	if err := readStrictJSON(path, &pkg); err != nil {
		return ScriptLintReport{}, pkg, domain.Invalid("SCRIPT_PACKAGE_JSON_INVALID", err.Error())
	}
	batch, err := loadCreativeBatch(resolved, batchFile, pkg.CreativeBatchID)
	if err != nil {
		return ScriptLintReport{}, pkg, err
	}
	query, err := QueryKnowledge(QueryKnowledgeOptions{Root: resolved, Channel: pkg.Channel})
	if err != nil {
		return ScriptLintReport{}, pkg, err
	}
	references, err := loadKnowledgeReferenceIndex(resolved)
	if err != nil {
		return ScriptLintReport{}, pkg, err
	}
	report := lintScriptPackage(pkg, batch, query, references)
	report.File = relativeWorkspacePath(resolved, path)
	hash, hashErr := domain.CanonicalHash(pkg)
	if hashErr == nil {
		report.ContentHash = "sha256:" + hash
	}
	return report, pkg, nil
}

func LintCreativeBatch(root, batchFile string, scriptFiles []string) (ScriptBatchLintReport, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ScriptBatchLintReport{}, err
	}
	batch, err := loadCreativeBatch(resolved, batchFile, "")
	if err != nil {
		return ScriptBatchLintReport{}, err
	}
	report := ScriptBatchLintReport{Valid: true, BatchID: batch.ID, Requested: batch.RequestedCount, Received: len(scriptFiles), Results: []ScriptLintReport{}}
	if len(scriptFiles) != batch.RequestedCount {
		report.Valid = false
	}
	seen := map[string]bool{}
	for _, file := range scriptFiles {
		item, pkg, err := LintScriptPackage(resolved, file, batchFile)
		if err != nil {
			return ScriptBatchLintReport{}, err
		}
		if seen[pkg.ID] {
			item.Valid = false
			item.Issues = append(item.Issues, ScriptLintIssue{Severity: "error", Code: "SCRIPT_ID_DUPLICATE", Path: "/id", Message: "批次内 script package ID 重复"})
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

func FinalizeCreativeBatch(root, batchFile string, scriptFiles []string, now time.Time) (FinalizeCreativeBatchResult, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return FinalizeCreativeBatchResult{}, err
	}
	report, err := LintCreativeBatch(resolved, batchFile, scriptFiles)
	if err != nil {
		return FinalizeCreativeBatchResult{}, err
	}
	if !report.Valid {
		err := domain.Invalid("CREATIVE_BATCH_LINT_FAILED", "CreativeBatch 校验失败")
		err.Details = report
		return FinalizeCreativeBatchResult{}, err
	}
	batch, err := loadCreativeBatch(resolved, batchFile, report.BatchID)
	if err != nil {
		return FinalizeCreativeBatchResult{}, err
	}
	batch.Status = "produced"
	if report.Blocked > 0 {
		batch.Status = "partially_blocked"
	}
	if report.ReviewReady == 0 {
		batch.Status = "failed"
	}
	files := []string{}
	for _, file := range scriptFiles {
		path, err := resolveWorkspaceFile(resolved, file)
		if err != nil {
			return FinalizeCreativeBatchResult{}, err
		}
		files = append(files, relativeWorkspacePath(resolved, path))
	}
	at := localNow(now)
	batch.ScriptFiles = uniqueStrings(files)
	batch.UpdatedAt = at
	batch.ProducedAt = &at
	path, err := resolveWorkspaceFile(resolved, batchFile)
	if err != nil {
		return FinalizeCreativeBatchResult{}, err
	}
	if err := replaceJSON(path, batch, 0o600); err != nil {
		return FinalizeCreativeBatchResult{}, err
	}
	return FinalizeCreativeBatchResult{Batch: batch, Report: report}, nil
}

func DiffScriptPackages(root, baselineFile, candidateFile string, allowedPaths []string) (ScriptDiff, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ScriptDiff{}, err
	}
	baselinePath, err := resolveWorkspaceFile(resolved, baselineFile)
	if err != nil {
		return ScriptDiff{}, err
	}
	candidatePath, err := resolveWorkspaceFile(resolved, candidateFile)
	if err != nil {
		return ScriptDiff{}, err
	}
	var baseline, candidate ScriptPackageV2
	if err := readStrictJSON(baselinePath, &baseline); err != nil {
		return ScriptDiff{}, domain.Invalid("SCRIPT_BASELINE_INVALID", err.Error())
	}
	if err := readStrictJSON(candidatePath, &candidate); err != nil {
		return ScriptDiff{}, domain.Invalid("SCRIPT_CANDIDATE_INVALID", err.Error())
	}
	if candidate.BasedOnVersionID != baseline.ID || strings.TrimSpace(candidate.ChangeSummary) == "" {
		return ScriptDiff{}, domain.Invalid("SCRIPT_REVISION_METADATA_INVALID", "修订稿必须用 based_on_version_id 引用基线 ID 并填写 change_summary")
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
	return ScriptDiff{Valid: len(unexpected) == 0, BaselineID: baseline.ID, CandidateID: candidate.ID, ChangedPaths: changes, AllowedPaths: allowed, UnexpectedPaths: unexpected}, nil
}

func ExportApprovedScript(root, scriptID, outputDirectory string, now time.Time) (ScriptDeliveryManifest, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ScriptDeliveryManifest{}, err
	}
	raw, snapshot, err := latestApprovedObject(resolved, "script", scriptID)
	if err != nil {
		return ScriptDeliveryManifest{}, err
	}
	rendered, err := RenderScriptPackageV2(raw)
	if err != nil {
		return ScriptDeliveryManifest{}, err
	}
	pkg := rendered.Package
	outputRoot := outputDirectory
	if strings.TrimSpace(outputRoot) == "" {
		outputRoot = filepath.Join(resolved, "outputs", "delivery", localSafeName(pkg.ID))
	} else {
		if !filepath.IsAbs(outputRoot) {
			outputRoot = filepath.Join(resolved, filepath.FromSlash(outputRoot))
		}
		absolute, absErr := filepath.Abs(outputRoot)
		relative, relErr := filepath.Rel(resolved, absolute)
		if absErr != nil || relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return ScriptDeliveryManifest{}, domain.Policy("DELIVERY_PATH_OUTSIDE_WORKSPACE", "交付目录必须位于当前工作区", "使用 outputs/delivery 下的目录")
		}
		outputRoot = absolute
	}
	files := []ScriptDeliveryFile{}
	for _, file := range rendered.Files {
		path := filepath.Join(outputRoot, file.Name)
		if err := replaceFile(path, file.Body, 0o600); err != nil {
			return ScriptDeliveryManifest{}, err
		}
		files = append(files, ScriptDeliveryFile{Format: file.Format, Path: relativeWorkspacePath(resolved, path), SHA256: file.SHA256, ByteSize: int64(len(file.Body)), MediaType: file.MediaType})
	}
	manifest := ScriptDeliveryManifest{SchemaVersion: "1.0", ScriptID: pkg.ID, ApprovedSnapshotID: snapshot.ID, ScriptHash: rendered.ScriptHash, Files: files, CreatedAt: localNow(now)}
	if err := replaceJSON(filepath.Join(outputRoot, "manifest.json"), manifest, 0o600); err != nil {
		return ScriptDeliveryManifest{}, err
	}
	return manifest, nil
}

func RenderScriptPackageV2(raw json.RawMessage) (RenderedScriptDelivery, error) {
	var pkg ScriptPackageV2
	if err := strictUnmarshal(raw, &pkg); err != nil {
		return RenderedScriptDelivery{}, domain.Invalid("APPROVED_SCRIPT_INVALID", "批准快照中的 ScriptPackage V2 无效："+err.Error())
	}
	if pkg.Deliverability != "review_ready" {
		return RenderedScriptDelivery{}, domain.Policy("APPROVED_SCRIPT_BLOCKED", "blocked 剧本不能生成正式交付包", "修订并批准 review_ready ScriptPackage")
	}
	jsonBody, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return RenderedScriptDelivery{}, err
	}
	jsonBody = append(jsonBody, '\n')
	xlsx, err := renderScriptV2XLSX(pkg)
	if err != nil {
		return RenderedScriptDelivery{}, err
	}
	hash, err := domain.CanonicalHash(pkg)
	if err != nil {
		return RenderedScriptDelivery{}, err
	}
	files := []RenderedScriptFile{
		{Format: "json", Name: "script.json", MediaType: "application/json", Body: jsonBody, SHA256: digest(jsonBody)},
		{Format: "markdown", Name: "script.md", MediaType: "text/markdown", Body: []byte(renderScriptV2Markdown(pkg))},
		{Format: "xlsx", Name: "script.xlsx", MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Body: xlsx, SHA256: digest(xlsx)},
	}
	files[1].SHA256 = digest(files[1].Body)
	return RenderedScriptDelivery{Package: pkg, ScriptHash: "sha256:" + hash, Files: files}, nil
}

func lintScriptPackage(pkg ScriptPackageV2, batch CreativeBatch, query KnowledgeQueryResult, refs map[string]LocalKnowledgeItem) ScriptLintReport {
	report := ScriptLintReport{Valid: true, ScriptID: pkg.ID, Deliverability: pkg.Deliverability, Issues: []ScriptLintIssue{}}
	add := func(code, path, message string) {
		report.Issues = append(report.Issues, ScriptLintIssue{Severity: "error", Code: code, Path: path, Message: message})
	}
	if pkg.SchemaVersion != "2.0" || pkg.ID == "" || pkg.Kind != "script_package" || pkg.ScriptID == "" {
		add("SCRIPT_IDENTITY_INVALID", "/", "ScriptPackage 需要 schema_version=2.0、id、script_id 和 kind=script_package")
	}
	if pkg.Deliverability != "review_ready" && pkg.Deliverability != "blocked" {
		add("SCRIPT_DELIVERABILITY_INVALID", "/deliverability", "deliverability 只允许 review_ready 或 blocked")
	}
	if pkg.CreativeBatchID != batch.ID || pkg.BriefVersionID != batch.BriefVersionID || pkg.ContextSnapshotID != batch.ContextSnapshotID || pkg.ProjectID != batch.ProjectID {
		add("SCRIPT_BATCH_CONTEXT_MISMATCH", "/", "project/batch/brief/context 必须与 CreativeBatch 冻结值一致")
	}
	if !containsString(batch.DirectionIDs, pkg.Direction.ID) || pkg.Direction.Status != "selected" {
		add("SCRIPT_DIRECTION_INVALID", "/direction", "direction 必须是批次中已选择的方向")
	}
	if pkg.Status != "candidate" && pkg.Status != "blocked" {
		add("SCRIPT_STATUS_INVALID", "/status", "本地剧本状态只允许 candidate 或 blocked")
	}
	if pkg.Title == "" || pkg.Channel == "" || pkg.DurationMS <= 0 || pkg.AspectRatio == "" {
		add("SCRIPT_TOP_LEVEL_REQUIRED", "/", "剧本需要标题、渠道、时长和画幅")
	}
	if pkg.DurationMS > 600000 || !validAspectRatio(pkg.AspectRatio) {
		add("SCRIPT_TOP_LEVEL_INVALID", "/", "duration_ms 或 aspect_ratio 不受支持")
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
			add("SCRIPT_ARRAY_REQUIRED", field.path, "必填数组不能缺失")
		}
	}
	if pkg.Direction.ID == "" || pkg.Direction.Title == "" || pkg.Direction.Angle == "" || pkg.Direction.HookType == "" || pkg.Direction.VisualMotif == "" || len(pkg.Direction.Narrative) == 0 || !validDirectionStatus(pkg.Direction.Status) {
		add("SCRIPT_DIRECTION_SHAPE_INVALID", "/direction", "direction 缺少必填字段或 status 无效")
	}
	if pkg.Cover.Title == "" || pkg.Cover.VisualIntent == "" || pkg.Cover.FirstViewSignal == "" || pkg.Cover.SafeArea == "" {
		add("SCRIPT_COVER_REQUIRED", "/cover", "cover 缺少必要信息")
	}
	if !validVariantDimension(pkg.Experiment.PrimaryVariable) || pkg.Experiment.Hypothesis == "" || pkg.Experiment.MeasurementWindow == "" || !allUnique(pkg.Experiment.ControlledVariables) {
		add("SCRIPT_EXPERIMENT_SHAPE_INVALID", "/experiment", "experiment 缺少必要信息或 controlled_variables 重复")
	}
	if pkg.Deliverability == "blocked" {
		if pkg.Status != "blocked" || len(pkg.BlockedReasons) == 0 {
			add("SCRIPT_BLOCK_REASON_REQUIRED", "/blocked_reasons", "blocked 输出必须使用 blocked 状态并提供原因")
		}
		for index, reason := range pkg.BlockedReasons {
			if reason.Code == "" || reason.Message == "" || reason.OwnerRole == "" || reason.NextAction == "" {
				add("SCRIPT_BLOCK_REASON_INVALID", "/blocked_reasons/"+strconv.Itoa(index), "blocked reason 缺少 code/message/owner_role/next_action")
			}
		}
		report.Valid = len(report.Issues) == 0
		return report
	}
	if batch.Status != "ready" && batch.Status != "produced" && batch.Status != "partially_blocked" {
		add("CREATIVE_BATCH_NOT_READY", "/creative_batch_id", "CreativeBatch 当前不能接收 review_ready 候选")
	}
	if pkg.Status != "candidate" {
		add("SCRIPT_REVIEW_READY_STATUS_INVALID", "/status", "review_ready 剧本必须保持 candidate 状态")
	}
	if len(pkg.BlockedReasons) > 0 || len(pkg.MissingInputs) > 0 {
		add("SCRIPT_REVIEW_READY_BLOCKED", "/blocked_reasons", "review_ready 剧本不能保留阻断原因或缺失输入")
	}
	if pkg.Experiment.PrimaryVariable != batch.VariantDimension || containsString(pkg.Experiment.ControlledVariables, pkg.Experiment.PrimaryVariable) {
		add("SCRIPT_EXPERIMENT_INVALID", "/experiment", "主要变量必须等于批次 variant_dimension，且不能同时被控制")
	}
	if len(pkg.Shots) == 0 {
		add("SCRIPT_SHOTS_REQUIRED", "/shots", "review_ready 剧本至少需要一个镜头")
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
			add("SCRIPT_SHOT_ID_INVALID", base+"/shot_id", "shot_id 必填且批次内唯一")
		}
		shotIDs[shot.ShotID] = true
		shotKnowledgeRefs[shot.ShotID] = map[string]bool{}
		for _, id := range append(append([]string{}, shot.KnowledgeRefs...), shot.ClaimRefs...) {
			shotKnowledgeRefs[shot.ShotID][id] = true
		}
		roles[shot.Role]++
		if shot.StartMS != expectedStart || shot.EndMS <= shot.StartMS {
			add("SCRIPT_TIMELINE_INVALID", base, "镜头时间必须从 0 开始、连续且 end_ms 大于 start_ms")
		}
		expectedStart = shot.EndMS
		if index > 0 && previousOutgoing != "" && shot.Continuity.IncomingState != previousOutgoing {
			add("SCRIPT_CONTINUITY_HANDOFF_INVALID", base+"/continuity/incoming_state", "相邻镜头 incoming_state 必须等于上一镜头 outgoing_state")
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
				add("SCRIPT_SHOT_FIELD_REQUIRED", base+"/"+field.name, field.name+" 必填")
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
				add("SCRIPT_SHOT_ARRAY_REQUIRED", base+field.path, "必填数组不能缺失")
			}
		}
		if !validShotRole(shot.Role) {
			add("SCRIPT_SHOT_ROLE_INVALID", base+"/role", "role 不受支持")
		}
		if !validProductionMode(shot.ProductionMode) {
			add("SCRIPT_PRODUCTION_MODE_INVALID", base+"/production_mode", "production_mode 不受支持")
		}
		if len(shot.NegativeConstraints) == 0 || len(shot.AcceptanceCriteria) == 0 {
			add("SCRIPT_SHOT_GUARD_REQUIRED", base, "每个镜头都需要 negative_constraints 和 acceptance_criteria")
		}
		if shot.Role == "proof" && shot.VisualizationPlanID == "" {
			add("SCRIPT_PROOF_PLAN_REQUIRED", base+"/visualization_plan_id", "proof 镜头必须引用 VisualizationPlan")
		}
		if (shot.ProductionMode == "real_asset" || shot.ProductionMode == "asset_guided_generation" || shot.ProductionMode == "composite") && len(shot.AssetRefs) == 0 {
			add("SCRIPT_ASSET_REQUIRED", base+"/asset_refs", "当前 production_mode 需要真实素材引用")
		}
		for _, id := range append(append([]string{}, shot.KnowledgeRefs...), shot.ClaimRefs...) {
			if !eligible[id] {
				add("SCRIPT_KNOWLEDGE_NOT_ELIGIBLE", base+"/knowledge_refs", "引用知识未进入当前 Knowledge ApprovedSnapshot："+id)
			}
		}
		for _, id := range shot.AssetRefs {
			if _, ok := refs[id]; !ok {
				add("SCRIPT_ASSET_MISSING", base+"/asset_refs", "素材引用不存在："+id)
			}
		}
		for _, id := range shot.RightsRefs {
			if value, ok := refs[id]; !ok || (value.Status != "valid" && value.Status != "approved") {
				add("SCRIPT_RIGHTS_INVALID", base+"/rights_refs", "权利记录不可用："+id)
			}
		}
		if shot.Voiceover != "" && !hasShotCitation(pkg.Citations, shot.ShotID, "spoken_claim") {
			add("SCRIPT_SPOKEN_CITATION_REQUIRED", base+"/voiceover", "有口播的镜头需要 spoken_claim citation")
		}
		if shot.OnScreenText != "" && !hasShotCitation(pkg.Citations, shot.ShotID, "on_screen_text") {
			add("SCRIPT_TEXT_CITATION_REQUIRED", base+"/on_screen_text", "有屏幕文字的镜头需要 on_screen_text citation")
		}
	}
	if expectedStart != pkg.DurationMS {
		add("SCRIPT_DURATION_MISMATCH", "/duration_ms", "镜头总时长必须等于 duration_ms")
	}
	for _, role := range []string{"hook", "proof", "cta"} {
		if roles[role] == 0 {
			add("SCRIPT_REQUIRED_ROLE_MISSING", "/shots", "缺少必要叙事角色："+role)
		}
	}
	if roles["product_intro"]+roles["product_solution"] == 0 {
		add("SCRIPT_PRODUCT_ROLE_MISSING", "/shots", "缺少 product_intro 或 product_solution 镜头")
	}
	if roles["cta"] != 1 {
		add("SCRIPT_CTA_COUNT_INVALID", "/shots", "review_ready 剧本必须且只能有一个 CTA 镜头")
	}
	for index, citation := range pkg.Citations {
		if !eligible[citation.KnowledgeID] || !shotIDs[citation.ShotID] || !shotKnowledgeRefs[citation.ShotID][citation.KnowledgeID] || !validCitationUsage(citation.Usage) {
			add("SCRIPT_CITATION_INVALID", "/citations/"+strconv.Itoa(index), "citation 的 knowledge_id、shot_id 或 usage 无效")
		}
	}
	for index, requirement := range pkg.AssetRequirements {
		path := "/asset_requirements/" + strconv.Itoa(index)
		if requirement.AssetID == "" || requirement.RightsID == "" || requirement.Purpose == "" || requirement.RequiredTruth == "" || requirement.Fallback == "" {
			add("SCRIPT_ASSET_REQUIREMENT_INVALID", path, "asset requirement 缺少 asset_id/rights_id/purpose/required_truth/fallback")
		}
		if value, ok := refs[requirement.AssetID]; !ok || value.Kind != "asset" {
			add("SCRIPT_ASSET_REQUIREMENT_MISSING", path+"/asset_id", "asset requirement 引用的素材不存在："+requirement.AssetID)
		}
		if value, ok := refs[requirement.RightsID]; !ok || (value.Status != "valid" && value.Status != "approved") {
			add("SCRIPT_ASSET_REQUIREMENT_RIGHTS_INVALID", path+"/rights_id", "asset requirement 引用的权利记录不可用："+requirement.RightsID)
		}
	}
	for index, segment := range pkg.NarrativeStructure {
		path := "/narrative_structure/" + strconv.Itoa(index)
		if segment.Role == "" || segment.Purpose == "" || segment.DecisionFunction == "" || segment.EndMS <= segment.StartMS || segment.ShotIDs == nil {
			add("SCRIPT_NARRATIVE_INVALID", path, "叙事段缺少必要字段、时间无效或 shot_ids 缺失")
		}
		for _, shotID := range segment.ShotIDs {
			if !shotIDs[shotID] {
				add("SCRIPT_NARRATIVE_SHOT_MISSING", path+"/shot_ids", "叙事段引用了不存在的镜头："+shotID)
			}
		}
	}
	declarations := pkg.ValidationDeclarations
	if !declarations.SchemaChecked || !declarations.KnowledgeChecked || !declarations.RightsChecked || !declarations.ContinuityChecked || !declarations.ExperimentChecked {
		add("SCRIPT_VALIDATION_DECLARATION_MISSING", "/validation_declarations", "客户端必须声明五类确定性校验均已执行")
	}
	report.Valid = len(report.Issues) == 0
	return report
}

func loadCreativeBatch(root, file, expectedID string) (CreativeBatch, error) {
	path := file
	if strings.TrimSpace(path) == "" {
		if expectedID == "" {
			return CreativeBatch{}, domain.Invalid("CREATIVE_BATCH_FILE_REQUIRED", "必须指定 batch 文件")
		}
		path = filepath.ToSlash(filepath.Join("outputs", "scripts", localSafeName(expectedID), "batch.json"))
	}
	resolved, err := resolveWorkspaceFile(root, path)
	if err != nil {
		return CreativeBatch{}, err
	}
	var batch CreativeBatch
	if err := readStrictJSON(resolved, &batch); err != nil {
		return CreativeBatch{}, domain.Invalid("CREATIVE_BATCH_INVALID", err.Error())
	}
	if batch.SchemaVersion != "2.0" || batch.Kind != "creative_batch" || batch.ID == "" || (expectedID != "" && batch.ID != expectedID) {
		return CreativeBatch{}, domain.Invalid("CREATIVE_BATCH_INVALID", "CreativeBatch identity 无效")
	}
	return batch, nil
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

func hasShotCitation(values []ScriptCitationV2, shotID, usage string) bool {
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

func renderScriptV2Markdown(pkg ScriptPackageV2) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", pkg.Title)
	fmt.Fprintf(&out, "- Script ID: `%s`\n- Schema: `%s`\n- 渠道: %s\n- 画幅: %s\n- 时长: %.1f 秒\n- 创意方向: %s\n- 主要变量: %s\n\n", pkg.ID, ScriptPackageV2Schema, pkg.Channel, pkg.AspectRatio, float64(pkg.DurationMS)/1000, pkg.Direction.Title, pkg.Experiment.PrimaryVariable)
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

func renderScriptV2XLSX(pkg ScriptPackageV2) ([]byte, error) {
	rows := [][]string{{"镜头ID", "开始(ms)", "结束(ms)", "功能", "叙事目的", "主体", "画面意图", "主体动作", "构图", "相机运动", "首帧", "运动", "尾帧", "口播", "字幕", "声音", "制作方式", "知识引用", "素材", "权利", "可视化方案", "负面约束", "连续性", "真实性策略", "验收", "Plan B"}}
	for _, shot := range pkg.Shots {
		rows = append(rows, []string{shot.ShotID, strconv.Itoa(shot.StartMS), strconv.Itoa(shot.EndMS), shot.Role, shot.NarrativePurpose, shot.Subject, shot.VisualIntent, shot.SubjectAction, shot.Composition, shot.CameraMotion, shot.FirstFrame.PromptZH, shot.MotionSpec, shot.EndFrame.PromptZH, shot.Voiceover, shot.OnScreenText, shot.SoundIntent, shot.ProductionMode, strings.Join(shot.KnowledgeRefs, ","), strings.Join(shot.AssetRefs, ","), strings.Join(shot.RightsRefs, ","), shot.VisualizationPlanID, strings.Join(shot.NegativeConstraints, "；"), shot.Continuity.IncomingState + " -> " + shot.Continuity.OutgoingState, shot.ProductTruthStrategy, strings.Join(shot.AcceptanceCriteria, "；"), shot.PlanB})
	}
	return exportfmt.XLSX("镜头", rows)
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}
