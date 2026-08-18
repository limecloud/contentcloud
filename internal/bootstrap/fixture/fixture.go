package fixturev3

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Fixture struct {
	FixtureVersion    string           `json:"fixture_version"`
	EnvironmentDigest string           `json:"environment_digest"`
	Project           ProjectSpec      `json:"project"`
	Workspace         WorkspaceSpec    `json:"workspace"`
	Scenario          *ScenarioSpec    `json:"scenario,omitempty"`
	Submissions       []SubmissionSpec `json:"submissions"`
}

type ProjectSpec struct {
	BrandName      string `json:"brand_name"`
	ProductName    string `json:"product_name"`
	Channel        string `json:"channel"`
	StageObjective string `json:"stage_objective"`
	OwnerName      string `json:"owner_name"`
	ReviewerName   string `json:"reviewer_name"`
	ClientApprover string `json:"client_approver"`
}

type WorkspaceSpec struct {
	TemplateID      string   `json:"template_id"`
	TemplateVersion string   `json:"template_version"`
	Targets         []string `json:"targets"`
	DeviceName      string   `json:"device_name"`
}

type ScenarioSpec struct {
	GeneratedAt   time.Time         `json:"generated_at"`
	Sources       []SourceSpec      `json:"sources"`
	Methodology   MethodologySpec   `json:"methodology"`
	KnowledgePack KnowledgePackSpec `json:"knowledge_pack"`
	Governance    []GovernanceSpec  `json:"governance"`
	CompletedRun  CompletedRunSpec  `json:"completed_run"`
	ContentBatch  ContentBatchSpec  `json:"content_batch"`
}

type SourceSpec struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	FileName   string `json:"file_name"`
	SourceKind string `json:"source_kind"`
	Content    string `json:"content"`
}

type MethodologySpec struct {
	VersionID  string                 `json:"version_id"`
	Dimensions []MethodologyDimension `json:"dimensions"`
	Stages     []MethodologyStage     `json:"stages"`
}

type MethodologyDimension struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	SourceID  string `json:"source_id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Statement string `json:"statement"`
	Layer     string `json:"layer"`
}

type MethodologyStage struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type KnowledgePackSpec struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ApprovedSnapshotID string   `json:"approved_snapshot_id"`
	Layers             []string `json:"layers"`
}

type GovernanceSpec struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Statement string `json:"statement"`
}

type CompletedRunSpec struct {
	ID     string `json:"id"`
	Intent string `json:"intent"`
}

type ContentBatchSpec struct {
	ID            string            `json:"id"`
	Brief         BriefSpec         `json:"brief"`
	Direction     DirectionSpec     `json:"direction"`
	BlockedReason BlockedReasonSpec `json:"blocked_reason"`
	Items         []ContentItemSpec `json:"items"`
}

type BriefSpec struct {
	ID                  string `json:"id"`
	ApprovedSnapshotID  string `json:"approved_snapshot_id"`
	Objective           string `json:"objective"`
	Audience            string `json:"audience"`
	Scenario            string `json:"scenario"`
	DemandMoment        string `json:"demand_moment"`
	PainPoint           string `json:"pain_point"`
	PrimarySellingPoint string `json:"primary_selling_point"`
	Positioning         string `json:"positioning"`
	Tone                string `json:"tone"`
	CTA                 string `json:"cta"`
}

type DirectionSpec struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Angle         string   `json:"angle"`
	HookType      string   `json:"hook_type"`
	VisualMotif   string   `json:"visual_motif"`
	Narrative     []string `json:"narrative"`
	Tone          string   `json:"tone"`
	TargetEmotion string   `json:"target_emotion"`
}

type BlockedReasonSpec struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	OwnerRole  string `json:"owner_role"`
	NextAction string `json:"next_action"`
}

type ContentItemSpec struct {
	ID           string `json:"id"`
	ContentID    string `json:"content_id"`
	Title        string `json:"title"`
	MissingInput string `json:"missing_input"`
}

type SubmissionSpec struct {
	SubmissionType    string             `json:"submission_type"`
	Outcome           string             `json:"outcome"`
	BaseSnapshotTypes []string           `json:"base_snapshot_types"`
	Message           string             `json:"message"`
	ChangeReason      string             `json:"change_reason,omitempty"`
	ChangePointer     string             `json:"change_pointer,omitempty"`
	Objects           []SubmissionObject `json:"objects"`
}

type SubmissionObject struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Path    string          `json:"path"`
	Content json.RawMessage `json:"content"`
}

func Decode(reader io.Reader) (Fixture, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var fixture Fixture
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, err
	}
	if err := fixture.Validate(); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func (f Fixture) Validate() error {
	if f.FixtureVersion != "3.0" {
		return fmt.Errorf("fixture_version 必须为 3.0")
	}
	if !validSHA256(f.EnvironmentDigest) {
		return fmt.Errorf("environment_digest 必须是带前缀的 SHA-256 摘要")
	}
	if strings.TrimSpace(f.Project.BrandName) == "" || strings.TrimSpace(f.Project.ProductName) == "" {
		return fmt.Errorf("project 的 brand_name 和 product_name 必填")
	}
	if strings.TrimSpace(f.Workspace.TemplateID) == "" || strings.TrimSpace(f.Workspace.TemplateVersion) == "" || len(f.Workspace.Targets) == 0 {
		return fmt.Errorf("workspace 的 template 和 targets 必填")
	}
	if f.Scenario != nil {
		if err := f.Scenario.Validate(); err != nil {
			return err
		}
	}
	allowedTypes := map[string]bool{"context": true, "knowledge": true, "brief": true, "content_batch": true, "asset_batch": true, "delivery": true, "result": true}
	allowedOutcomes := map[string]bool{"approved": true, "submitted": true, "changes_requested": true}
	seenTypes := map[string]bool{}
	for _, submission := range f.Submissions {
		if !allowedTypes[submission.SubmissionType] || seenTypes[submission.SubmissionType] {
			return fmt.Errorf("submission_type %q 无效或重复", submission.SubmissionType)
		}
		seenTypes[submission.SubmissionType] = true
		if !allowedOutcomes[submission.Outcome] {
			return fmt.Errorf("%s 的 outcome %q 无效", submission.SubmissionType, submission.Outcome)
		}
		if submission.Outcome == "changes_requested" && strings.TrimSpace(submission.ChangeReason) == "" {
			return fmt.Errorf("状态为 changes_requested 的提交 %s 必须提供 change_reason", submission.SubmissionType)
		}
		if len(submission.Objects) == 0 {
			return fmt.Errorf("提交 %s 必须包含 objects", submission.SubmissionType)
		}
		seenObjects := map[string]bool{}
		for _, object := range submission.Objects {
			if strings.TrimSpace(object.ID) == "" || strings.TrimSpace(object.Type) == "" || object.Version < 1 || strings.TrimSpace(object.Path) == "" || !json.Valid(object.Content) || seenObjects[object.ID] {
				return fmt.Errorf("提交 %s 包含无效对象", submission.SubmissionType)
			}
			seenObjects[object.ID] = true
		}
	}
	return nil
}

var requiredDimensionKeys = []string{
	"customer-pain", "customer-solution", "benchmark", "competitors", "sales-channel",
	"theme-subbrand", "culture-story", "scent-formula", "usage-scenario", "solution-value",
	"category", "form", "materials-factories", "packaging-assembly", "spec-cost-price",
}

var requiredLayerNames = []string{"identity", "product", "market", "expression", "operations", "content_engine", "compliance"}

func (s ScenarioSpec) Validate() error {
	if s.GeneratedAt.IsZero() {
		return fmt.Errorf("scenario 的 generated_at 必填")
	}
	if len(s.Sources) != 20 {
		return fmt.Errorf("scenario 必须正好包含 20 个来源")
	}
	sourceIDs := map[string]bool{}
	fileNames := map[string]bool{}
	for _, source := range s.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Title) == "" || strings.TrimSpace(source.SourceKind) == "" || strings.TrimSpace(source.Content) == "" {
			return fmt.Errorf("scenario 来源必须包含 id、title、source_kind 和 content")
		}
		if sourceIDs[source.ID] {
			return fmt.Errorf("scenario 来源 ID %q 重复", source.ID)
		}
		if source.FileName == "" || filepath.Base(source.FileName) != source.FileName || strings.ContainsAny(source.FileName, `/\\`) || fileNames[source.FileName] {
			return fmt.Errorf("scenario 来源的 file_name %q 无效或重复", source.FileName)
		}
		sourceIDs[source.ID] = true
		fileNames[source.FileName] = true
	}
	if strings.TrimSpace(s.Methodology.VersionID) == "" || len(s.Methodology.Dimensions) != len(requiredDimensionKeys) || len(s.Methodology.Stages) != 4 {
		return fmt.Errorf("scenario 方法论必须包含版本、15 个维度和 4 个阶段")
	}
	dimensionKeys := map[string]bool{}
	layerCoverage := map[string]bool{}
	stateCoverage := map[string]map[string]bool{"fact": {}, "claim": {}}
	for _, dimension := range s.Methodology.Dimensions {
		if !contains(requiredDimensionKeys, dimension.Key) || dimensionKeys[dimension.Key] {
			return fmt.Errorf("方法论维度 %q 无效或重复", dimension.Key)
		}
		if !sourceIDs[dimension.SourceID] || !contains(requiredLayerNames, dimension.Layer) {
			return fmt.Errorf("维度 %q 引用了未知来源或层级", dimension.Key)
		}
		if (dimension.Kind != "fact" && dimension.Kind != "claim") || strings.TrimSpace(dimension.Status) == "" || strings.TrimSpace(dimension.Label) == "" || strings.TrimSpace(dimension.Title) == "" || strings.TrimSpace(dimension.Statement) == "" {
			return fmt.Errorf("维度 %q 不完整", dimension.Key)
		}
		dimensionKeys[dimension.Key] = true
		layerCoverage[dimension.Layer] = true
		stateCoverage[dimension.Kind][dimension.Status] = true
	}
	for _, key := range requiredDimensionKeys {
		if !dimensionKeys[key] {
			return fmt.Errorf("scenario 方法论缺少维度 %q", key)
		}
	}
	stageIDs := map[string]bool{}
	for _, stage := range s.Methodology.Stages {
		if strings.TrimSpace(stage.ID) == "" || strings.TrimSpace(stage.Title) == "" || strings.TrimSpace(stage.Status) == "" || stageIDs[stage.ID] {
			return fmt.Errorf("scenario 方法论包含无效阶段")
		}
		stageIDs[stage.ID] = true
	}
	if strings.TrimSpace(s.KnowledgePack.ID) == "" || strings.TrimSpace(s.KnowledgePack.Name) == "" || strings.TrimSpace(s.KnowledgePack.ApprovedSnapshotID) == "" {
		return fmt.Errorf("scenario 的 knowledge_pack 标识必填")
	}
	layers := append([]string(nil), s.KnowledgePack.Layers...)
	sort.Strings(layers)
	expectedLayers := append([]string(nil), requiredLayerNames...)
	sort.Strings(expectedLayers)
	if strings.Join(layers, "\x00") != strings.Join(expectedLayers, "\x00") {
		return fmt.Errorf("scenario 的 knowledge_pack 必须声明标准七层知识")
	}
	for _, layer := range requiredLayerNames {
		if !layerCoverage[layer] {
			return fmt.Errorf("scenario 知识在层级 %q 中没有对象", layer)
		}
	}
	governanceIDs := map[string]bool{}
	for _, item := range s.Governance {
		if item.Kind != "asset" && item.Kind != "rights" && item.Kind != "conflict" {
			return fmt.Errorf("scenario 治理类型 %q 不受支持", item.Kind)
		}
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Status) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Statement) == "" || governanceIDs[item.ID] {
			return fmt.Errorf("scenario 治理数据包含无效对象")
		}
		governanceIDs[item.ID] = true
		if stateCoverage[item.Kind] == nil {
			stateCoverage[item.Kind] = map[string]bool{}
		}
		stateCoverage[item.Kind][item.Status] = true
	}
	for _, kind := range []string{"fact", "claim", "asset", "rights", "conflict"} {
		if len(stateCoverage[kind]) < 2 {
			return fmt.Errorf("scenario 的 %s 对象必须覆盖至少两种状态", kind)
		}
	}
	if strings.TrimSpace(s.CompletedRun.ID) == "" || !strings.HasPrefix(s.CompletedRun.Intent, "intent:") {
		return fmt.Errorf("scenario 的 completed_run 必须包含 id 和 intent:<name>")
	}
	if err := s.ContentBatch.validate(); err != nil {
		return err
	}
	return nil
}

func (s ContentBatchSpec) validate() error {
	if strings.TrimSpace(s.ID) == "" || len(s.Items) != 10 {
		return fmt.Errorf("scenario 的 content_batch 必须包含 id 和正好 10 个对象")
	}
	brief := s.Brief
	if strings.TrimSpace(brief.ID) == "" || strings.TrimSpace(brief.ApprovedSnapshotID) == "" || strings.TrimSpace(brief.Objective) == "" || strings.TrimSpace(brief.Audience) == "" || strings.TrimSpace(brief.Scenario) == "" || strings.TrimSpace(brief.DemandMoment) == "" || strings.TrimSpace(brief.PainPoint) == "" || strings.TrimSpace(brief.PrimarySellingPoint) == "" || strings.TrimSpace(brief.Positioning) == "" || strings.TrimSpace(brief.Tone) == "" || strings.TrimSpace(brief.CTA) == "" {
		return fmt.Errorf("scenario 的 content_batch 简报不完整")
	}
	direction := s.Direction
	if strings.TrimSpace(direction.ID) == "" || strings.TrimSpace(direction.Title) == "" || strings.TrimSpace(direction.Angle) == "" || strings.TrimSpace(direction.HookType) == "" || strings.TrimSpace(direction.VisualMotif) == "" || len(direction.Narrative) == 0 || strings.TrimSpace(direction.Tone) == "" || strings.TrimSpace(direction.TargetEmotion) == "" {
		return fmt.Errorf("scenario 的 content_batch 创意方向不完整")
	}
	reason := s.BlockedReason
	if strings.TrimSpace(reason.Code) == "" || strings.TrimSpace(reason.Message) == "" || strings.TrimSpace(reason.OwnerRole) == "" || strings.TrimSpace(reason.NextAction) == "" {
		return fmt.Errorf("scenario 的 content_batch 阻断原因不完整")
	}
	ids := map[string]bool{}
	contentIDs := map[string]bool{}
	for _, item := range s.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ContentID) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.MissingInput) == "" || ids[item.ID] || contentIDs[item.ContentID] {
			return fmt.Errorf("scenario 的 content_batch 包含无效对象")
		}
		ids[item.ID] = true
		contentIDs[item.ContentID] = true
	}
	return nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func DeterministicID(prefix, seed string) string {
	digest := sha256.Sum256([]byte(prefix + "\x00" + seed))
	return prefix + "-" + hex.EncodeToString(digest[:12])
}

func SHA256Hex(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
