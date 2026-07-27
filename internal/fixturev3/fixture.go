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
		return fmt.Errorf("fixture_version must be 3.0")
	}
	if !validSHA256(f.EnvironmentDigest) {
		return fmt.Errorf("environment_digest must be a prefixed SHA-256 digest")
	}
	if strings.TrimSpace(f.Project.BrandName) == "" || strings.TrimSpace(f.Project.ProductName) == "" {
		return fmt.Errorf("project brand_name and product_name are required")
	}
	if strings.TrimSpace(f.Workspace.TemplateID) == "" || strings.TrimSpace(f.Workspace.TemplateVersion) == "" || len(f.Workspace.Targets) == 0 {
		return fmt.Errorf("workspace template and targets are required")
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
			return fmt.Errorf("invalid or duplicate submission_type %q", submission.SubmissionType)
		}
		seenTypes[submission.SubmissionType] = true
		if !allowedOutcomes[submission.Outcome] {
			return fmt.Errorf("invalid outcome %q for %s", submission.Outcome, submission.SubmissionType)
		}
		if submission.Outcome == "changes_requested" && strings.TrimSpace(submission.ChangeReason) == "" {
			return fmt.Errorf("changes_requested submission %s requires change_reason", submission.SubmissionType)
		}
		if len(submission.Objects) == 0 {
			return fmt.Errorf("submission %s requires objects", submission.SubmissionType)
		}
		seenObjects := map[string]bool{}
		for _, object := range submission.Objects {
			if strings.TrimSpace(object.ID) == "" || strings.TrimSpace(object.Type) == "" || object.Version < 1 || strings.TrimSpace(object.Path) == "" || !json.Valid(object.Content) || seenObjects[object.ID] {
				return fmt.Errorf("submission %s contains an invalid object", submission.SubmissionType)
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
		return fmt.Errorf("scenario generated_at is required")
	}
	if len(s.Sources) != 20 {
		return fmt.Errorf("scenario must contain exactly 20 sources")
	}
	sourceIDs := map[string]bool{}
	fileNames := map[string]bool{}
	for _, source := range s.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Title) == "" || strings.TrimSpace(source.SourceKind) == "" || strings.TrimSpace(source.Content) == "" {
			return fmt.Errorf("scenario sources require id, title, source_kind, and content")
		}
		if sourceIDs[source.ID] {
			return fmt.Errorf("duplicate scenario source id %q", source.ID)
		}
		if source.FileName == "" || filepath.Base(source.FileName) != source.FileName || strings.ContainsAny(source.FileName, `/\\`) || fileNames[source.FileName] {
			return fmt.Errorf("invalid or duplicate scenario source file_name %q", source.FileName)
		}
		sourceIDs[source.ID] = true
		fileNames[source.FileName] = true
	}
	if strings.TrimSpace(s.Methodology.VersionID) == "" || len(s.Methodology.Dimensions) != len(requiredDimensionKeys) || len(s.Methodology.Stages) != 4 {
		return fmt.Errorf("scenario methodology requires a version, 15 dimensions, and 4 stages")
	}
	dimensionKeys := map[string]bool{}
	layerCoverage := map[string]bool{}
	stateCoverage := map[string]map[string]bool{"fact": {}, "claim": {}}
	for _, dimension := range s.Methodology.Dimensions {
		if !contains(requiredDimensionKeys, dimension.Key) || dimensionKeys[dimension.Key] {
			return fmt.Errorf("invalid or duplicate methodology dimension %q", dimension.Key)
		}
		if !sourceIDs[dimension.SourceID] || !contains(requiredLayerNames, dimension.Layer) {
			return fmt.Errorf("dimension %q references an unknown source or layer", dimension.Key)
		}
		if (dimension.Kind != "fact" && dimension.Kind != "claim") || strings.TrimSpace(dimension.Status) == "" || strings.TrimSpace(dimension.Label) == "" || strings.TrimSpace(dimension.Title) == "" || strings.TrimSpace(dimension.Statement) == "" {
			return fmt.Errorf("dimension %q is incomplete", dimension.Key)
		}
		dimensionKeys[dimension.Key] = true
		layerCoverage[dimension.Layer] = true
		stateCoverage[dimension.Kind][dimension.Status] = true
	}
	for _, key := range requiredDimensionKeys {
		if !dimensionKeys[key] {
			return fmt.Errorf("scenario methodology is missing dimension %q", key)
		}
	}
	stageIDs := map[string]bool{}
	for _, stage := range s.Methodology.Stages {
		if strings.TrimSpace(stage.ID) == "" || strings.TrimSpace(stage.Title) == "" || strings.TrimSpace(stage.Status) == "" || stageIDs[stage.ID] {
			return fmt.Errorf("scenario methodology contains an invalid stage")
		}
		stageIDs[stage.ID] = true
	}
	if strings.TrimSpace(s.KnowledgePack.ID) == "" || strings.TrimSpace(s.KnowledgePack.Name) == "" || strings.TrimSpace(s.KnowledgePack.ApprovedSnapshotID) == "" {
		return fmt.Errorf("scenario knowledge_pack identity is required")
	}
	layers := append([]string(nil), s.KnowledgePack.Layers...)
	sort.Strings(layers)
	expectedLayers := append([]string(nil), requiredLayerNames...)
	sort.Strings(expectedLayers)
	if strings.Join(layers, "\x00") != strings.Join(expectedLayers, "\x00") {
		return fmt.Errorf("scenario knowledge_pack must declare the seven canonical layers")
	}
	for _, layer := range requiredLayerNames {
		if !layerCoverage[layer] {
			return fmt.Errorf("scenario knowledge has no item in layer %q", layer)
		}
	}
	governanceIDs := map[string]bool{}
	for _, item := range s.Governance {
		if item.Kind != "asset" && item.Kind != "rights" && item.Kind != "conflict" {
			return fmt.Errorf("scenario governance kind %q is unsupported", item.Kind)
		}
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Status) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Statement) == "" || governanceIDs[item.ID] {
			return fmt.Errorf("scenario governance contains an invalid object")
		}
		governanceIDs[item.ID] = true
		if stateCoverage[item.Kind] == nil {
			stateCoverage[item.Kind] = map[string]bool{}
		}
		stateCoverage[item.Kind][item.Status] = true
	}
	for _, kind := range []string{"fact", "claim", "asset", "rights", "conflict"} {
		if len(stateCoverage[kind]) < 2 {
			return fmt.Errorf("scenario %s objects must cover at least two states", kind)
		}
	}
	if strings.TrimSpace(s.CompletedRun.ID) == "" || !strings.HasPrefix(s.CompletedRun.Intent, "intent:") {
		return fmt.Errorf("scenario completed_run requires an id and intent:<name>")
	}
	if err := s.ContentBatch.validate(); err != nil {
		return err
	}
	return nil
}

func (s ContentBatchSpec) validate() error {
	if strings.TrimSpace(s.ID) == "" || len(s.Items) != 10 {
		return fmt.Errorf("scenario content_batch requires an id and exactly 10 items")
	}
	brief := s.Brief
	if strings.TrimSpace(brief.ID) == "" || strings.TrimSpace(brief.ApprovedSnapshotID) == "" || strings.TrimSpace(brief.Objective) == "" || strings.TrimSpace(brief.Audience) == "" || strings.TrimSpace(brief.Scenario) == "" || strings.TrimSpace(brief.DemandMoment) == "" || strings.TrimSpace(brief.PainPoint) == "" || strings.TrimSpace(brief.PrimarySellingPoint) == "" || strings.TrimSpace(brief.Positioning) == "" || strings.TrimSpace(brief.Tone) == "" || strings.TrimSpace(brief.CTA) == "" {
		return fmt.Errorf("scenario content_batch brief is incomplete")
	}
	direction := s.Direction
	if strings.TrimSpace(direction.ID) == "" || strings.TrimSpace(direction.Title) == "" || strings.TrimSpace(direction.Angle) == "" || strings.TrimSpace(direction.HookType) == "" || strings.TrimSpace(direction.VisualMotif) == "" || len(direction.Narrative) == 0 || strings.TrimSpace(direction.Tone) == "" || strings.TrimSpace(direction.TargetEmotion) == "" {
		return fmt.Errorf("scenario content_batch direction is incomplete")
	}
	reason := s.BlockedReason
	if strings.TrimSpace(reason.Code) == "" || strings.TrimSpace(reason.Message) == "" || strings.TrimSpace(reason.OwnerRole) == "" || strings.TrimSpace(reason.NextAction) == "" {
		return fmt.Errorf("scenario content_batch blocked_reason is incomplete")
	}
	ids := map[string]bool{}
	contentIDs := map[string]bool{}
	for _, item := range s.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ContentID) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.MissingInput) == "" || ids[item.ID] || contentIDs[item.ContentID] {
			return fmt.Errorf("scenario content_batch contains an invalid item")
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
