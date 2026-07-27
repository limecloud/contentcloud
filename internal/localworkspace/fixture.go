package localworkspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/fixturev3"
)

type MaterializeFixtureOptions struct {
	Root        string
	ProjectID   string
	WorkspaceID string
	DeviceID    string
	ServerURL   string
	CLIVersion  string
	Target      string
}

type FixtureMaterialization struct {
	FixtureVersion string                     `json:"fixture_version"`
	Workspace      Status                     `json:"workspace"`
	Sources        SourceVerification         `json:"sources"`
	Knowledge      KnowledgeLintReport        `json:"knowledge"`
	Diagnosis      KnowledgeDiagnosis         `json:"diagnosis"`
	Pack           KnowledgePackResult        `json:"pack"`
	Run            LocalRunContext            `json:"run"`
	Runs           LocalRunValidation         `json:"runs"`
	ContentBatch   FinalizeContentBatchResult `json:"content_batch"`
	ContentFiles   []string                   `json:"content_files"`
}

type fixtureMethodologyDocument struct {
	SchemaVersion string                           `yaml:"schema_version"`
	VersionID     string                           `yaml:"methodology_version_id"`
	Dimensions    []fixturev3.MethodologyDimension `yaml:"dimensions"`
	Stages        []fixturev3.MethodologyStage     `yaml:"stages"`
}

func MaterializeFixture(fixture fixturev3.Fixture, options MaterializeFixtureOptions) (FixtureMaterialization, error) {
	if err := fixture.Validate(); err != nil {
		return FixtureMaterialization{}, domain.Invalid("FIXTURE_V3_INVALID", err.Error())
	}
	if fixture.Scenario == nil {
		return FixtureMaterialization{}, domain.Invalid("FIXTURE_SCENARIO_REQUIRED", "本地 Workspace Fixture 必须包含 scenario")
	}
	if strings.TrimSpace(options.Root) == "" || strings.TrimSpace(options.ProjectID) == "" || strings.TrimSpace(options.WorkspaceID) == "" {
		return FixtureMaterialization{}, domain.Invalid("FIXTURE_WORKSPACE_CONTEXT_REQUIRED", "fixture apply 需要 directory、project ID 和 workspace ID")
	}
	target := strings.TrimSpace(options.Target)
	if target == "" {
		target = fixtureTarget(fixture.Workspace.Targets)
	}
	plan, err := Plan(options.Root, target)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	if plan.State != "missing" && plan.State != "empty" {
		return FixtureMaterialization{}, domain.Conflict("FIXTURE_DIRECTORY_NOT_EMPTY", "Fixture 只能物化到空目录，拒绝修改已有 Workspace 或文件")
	}
	cliVersion := strings.TrimSpace(options.CLIVersion)
	if cliVersion == "" {
		cliVersion = "fixture-v3"
	}
	now := fixture.Scenario.GeneratedAt.UTC()
	status, err := Initialize(InitOptions{
		Root: options.Root, ProjectID: options.ProjectID, WorkspaceID: options.WorkspaceID, DeviceID: options.DeviceID,
		ServerURL: options.ServerURL, CLIVersion: cliVersion, Target: target, EnvironmentDigest: fixture.EnvironmentDigest, Now: now,
	})
	if err != nil {
		return FixtureMaterialization{}, err
	}
	root := status.Root
	if err := materializeFixtureContext(root, fixture); err != nil {
		return FixtureMaterialization{}, err
	}
	evidence, err := materializeFixtureSources(root, fixture.Scenario.Sources, now)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	knowledgeItems, err := materializeFixtureKnowledge(root, fixture, evidence, now)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	knowledgeLint, err := LintKnowledge(root)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	if !knowledgeLint.Valid {
		err := domain.Invalid("FIXTURE_KNOWLEDGE_LINT_FAILED", "Fixture 生成的知识未通过确定性校验")
		err.Details = knowledgeLint
		return FixtureMaterialization{}, err
	}
	pack, err := PackKnowledge(PackKnowledgeOptions{Root: root, PackID: fixture.Scenario.KnowledgePack.ID, Name: fixture.Scenario.KnowledgePack.Name, Now: now.Add(time.Minute)})
	if err != nil {
		return FixtureMaterialization{}, err
	}
	if err := storeFixtureApprovedObject(root, options.ProjectID, options.WorkspaceID, fixture.Scenario.KnowledgePack.ApprovedSnapshotID, "knowledge", knowledgeItems, knowledgeItemIDs(knowledgeItems), now.Add(2*time.Minute)); err != nil {
		return FixtureMaterialization{}, err
	}
	diagnosis, err := DiagnoseKnowledge(root, fixture.Project.Channel, now.Add(3*time.Minute))
	if err != nil {
		return FixtureMaterialization{}, err
	}
	batch, contentFiles, err := materializeFixtureContent(root, fixture, knowledgeItems, options.ProjectID, options.WorkspaceID, now.Add(4*time.Minute))
	if err != nil {
		return FixtureMaterialization{}, err
	}
	run, err := materializeFixtureRun(root, *fixture.Scenario, knowledgeItems, pack, batch, contentFiles, now.Add(10*time.Minute))
	if err != nil {
		return FixtureMaterialization{}, err
	}
	sourceVerification, err := VerifyLocalSources(root)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	runs, err := ValidateLocalRuns(root)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	if !sourceVerification.Valid || !runs.Valid || run.Status != "completed" || batch.Batch.Status != "blocked" || batch.Report.Blocked != 10 {
		return FixtureMaterialization{}, domain.Conflict("FIXTURE_MATERIALIZATION_INCOMPLETE", "Fixture 未收敛为完整的 V3 验收状态")
	}
	status, err = LoadStatus(root)
	if err != nil {
		return FixtureMaterialization{}, err
	}
	return FixtureMaterialization{
		FixtureVersion: fixture.FixtureVersion, Workspace: status, Sources: sourceVerification, Knowledge: knowledgeLint,
		Diagnosis: diagnosis, Pack: pack, Run: run, Runs: runs, ContentBatch: batch, ContentFiles: contentFiles,
	}, nil
}

func materializeFixtureContext(root string, fixture fixturev3.Fixture) error {
	context := fixtureMethodologyDocument{
		SchemaVersion: "contentcloud.methodology-context/3.0", VersionID: fixture.Scenario.Methodology.VersionID,
		Dimensions: append([]fixturev3.MethodologyDimension(nil), fixture.Scenario.Methodology.Dimensions...),
		Stages:     append([]fixturev3.MethodologyStage(nil), fixture.Scenario.Methodology.Stages...),
	}
	return replaceYAML(filepath.Join(root, "10-context", "methodology.yaml"), context)
}

func materializeFixtureSources(root string, sources []fixturev3.SourceSpec, now time.Time) (map[string]LocalEvidenceBundle, error) {
	staging, err := os.MkdirTemp("", "contentcloud-fixture-sources-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	result := make(map[string]LocalEvidenceBundle, len(sources))
	for index, source := range sources {
		path := filepath.Join(staging, source.FileName)
		if err := os.WriteFile(path, []byte(strings.TrimSpace(source.Content)+"\n"), 0o600); err != nil {
			return nil, err
		}
		registered, err := RegisterLocalSource(RegisterLocalSourceOptions{
			Root: root, File: path, ID: source.ID, Title: source.Title, SourceKind: source.SourceKind, StorageMode: "copy", Now: now.Add(time.Duration(index) * time.Second),
		})
		if err != nil {
			return nil, err
		}
		bundle, err := IngestLocalSource(root, registered.ID, now.Add(time.Duration(index+20)*time.Second))
		if err != nil {
			return nil, err
		}
		if bundle.Status != "ready" || len(bundle.Evidence) == 0 {
			return nil, domain.Invalid("FIXTURE_SOURCE_INGEST_FAILED", "Fixture 来源未生成可接受 Evidence："+source.ID)
		}
		result[source.ID] = bundle
	}
	return result, nil
}

func materializeFixtureKnowledge(root string, fixture fixturev3.Fixture, evidence map[string]LocalEvidenceBundle, now time.Time) ([]LocalKnowledgeItem, error) {
	items := make([]LocalKnowledgeItem, 0, len(fixture.Scenario.Methodology.Dimensions))
	for index, dimension := range fixture.Scenario.Methodology.Dimensions {
		bundle := evidence[dimension.SourceID]
		span := bundle.Evidence[0]
		locator, err := json.Marshal(span.Locator)
		if err != nil {
			return nil, err
		}
		item := LocalKnowledgeItem{
			ID: dimension.Kind + ":" + dimension.Key, Version: 1, Kind: dimension.Kind, Title: dimension.Title, Statement: dimension.Statement,
			Subject: fixture.Project.ProductName, Predicate: dimension.Label, Value: domain.TypedValue{Type: "text", Text: dimension.Statement},
			Scope:  domain.KnowledgeScope{Regions: []string{"CN"}, Channels: []string{fixture.Project.Channel}, Audiences: []string{}, ProductVariants: []string{}},
			Status: dimension.Status, RiskLevel: "low", AllowedChannels: []string{fixture.Project.Channel},
			Evidence: []domain.EvidenceRef{{SourceRevisionID: dimension.SourceID, LocatorKind: span.LocatorKind, Locator: string(locator), Quote: span.Quote}}, EvidenceIDs: []string{span.ID},
			ForbiddenExtensions: []string{}, DependsOnFactIDs: []string{}, AssetRefs: []string{}, RightsRefs: []string{}, ConflictRefs: []string{}, DecisionRefs: []string{},
			Dimensions: []string{dimension.Key}, Layers: []string{dimension.Layer}, OriginRunID: fixture.Scenario.CompletedRun.ID,
			CreatedAt: now.Add(time.Duration(index) * time.Second), UpdatedAt: now.Add(time.Duration(index) * time.Second),
		}
		path := filepath.Join(root, "30-knowledge", "pages", knowledgeDirectory(item.Kind), localSafeName(item.ID)+".md")
		if err := writeKnowledgePage(path, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for index, spec := range fixture.Scenario.Governance {
		item := LocalKnowledgeItem{
			ID: spec.ID, Version: 1, Kind: spec.Kind, Title: spec.Title, Statement: spec.Statement, Subject: fixture.Project.ProductName,
			Predicate: spec.Kind, Value: domain.TypedValue{Type: "text", Text: spec.Statement}, Scope: domain.KnowledgeScope{Regions: []string{}, Channels: []string{}, Audiences: []string{}, ProductVariants: []string{}},
			Status: spec.Status, RiskLevel: "low", AllowedChannels: []string{}, Evidence: []domain.EvidenceRef{}, EvidenceIDs: []string{}, ForbiddenExtensions: []string{},
			DependsOnFactIDs: []string{}, AssetRefs: []string{}, RightsRefs: []string{}, ConflictRefs: []string{}, DecisionRefs: []string{}, Dimensions: []string{}, Layers: []string{},
			OriginRunID: fixture.Scenario.CompletedRun.ID, CreatedAt: now.Add(time.Duration(index+30) * time.Second), UpdatedAt: now.Add(time.Duration(index+30) * time.Second),
		}
		path := filepath.Join(root, "30-knowledge", "pages", knowledgeDirectory(item.Kind), localSafeName(item.ID)+".md")
		if err := writeKnowledgePage(path, item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func materializeFixtureContent(root string, fixture fixturev3.Fixture, knowledge []LocalKnowledgeItem, projectID, workspaceID string, now time.Time) (FinalizeContentBatchResult, []string, error) {
	spec := fixture.Scenario.ContentBatch
	knowledgeID := knowledge[0].ID
	brief := LocalBrief{
		ID: spec.Brief.ID, Kind: "brief", Status: "candidate", SchemaVersion: BriefSchema, Deliverability: "review_ready",
		StrategyVersionID: fixture.Scenario.Methodology.VersionID, CampaignID: fixture.Scenario.CompletedRun.Intent, ExperimentID: "experiment:" + spec.ID,
		Channel: fixture.Project.Channel, Objective: spec.Brief.Objective, Audience: spec.Brief.Audience, Scenario: spec.Brief.Scenario, DemandMoment: spec.Brief.DemandMoment,
		PainPoint: spec.Brief.PainPoint, PrimarySellingPoint: spec.Brief.PrimarySellingPoint, SupportPoints: []string{knowledge[0].Statement}, Positioning: spec.Brief.Positioning,
		VisualizationPlanIDs: []string{"visualization:" + spec.Direction.ID}, AssetIDs: []string{}, TruthStrategy: "产品细节仅使用经授权素材", PlanB: "缺少授权素材时保持 blocked",
		Tone: spec.Brief.Tone, BrandRuleIDs: []string{}, ApprovedClaimIDs: []string{}, ForbiddenClaims: []string{"未经证据支持的功效承诺"}, HookExpectation: spec.Direction.HookType,
		NarrativeConstraints: []string{"保持单一主张"}, CTA: spec.Brief.CTA, PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, MeasurementWindow: "发布后24小时",
		EligibleKnowledgeIDs: []string{knowledgeID}, BlockedKnowledgeIDs: []string{}, RightsIDs: []string{}, RiskDecisionIDs: []string{}, DurationMinMS: 4000, DurationMaxMS: 15000,
		AspectRatio: "9:16", BlockedReasons: []string{}, MissingInputs: []string{},
	}
	briefPath := filepath.Join(root, "50-production", "briefs", localSafeName(brief.ID)+".json")
	if err := replaceJSON(briefPath, brief, 0o600); err != nil {
		return FinalizeContentBatchResult{}, nil, err
	}
	briefLint, _, err := LintBrief(root, relativeWorkspacePath(root, briefPath))
	if err != nil || !briefLint.Valid {
		if err != nil {
			return FinalizeContentBatchResult{}, nil, err
		}
		lintErr := domain.Invalid("FIXTURE_BRIEF_LINT_FAILED", "Fixture Brief 未通过确定性校验")
		lintErr.Details = briefLint
		return FinalizeContentBatchResult{}, nil, lintErr
	}
	if err := storeFixtureApprovedObject(root, projectID, workspaceID, spec.Brief.ApprovedSnapshotID, "brief", []LocalBrief{brief}, []string{brief.ID}, now); err != nil {
		return FinalizeContentBatchResult{}, nil, err
	}
	direction := CreativeDirection{
		ID: spec.Direction.ID, Title: spec.Direction.Title, Angle: spec.Direction.Angle, HookType: spec.Direction.HookType, VisualMotif: spec.Direction.VisualMotif,
		Narrative: append([]string(nil), spec.Direction.Narrative...), Tone: spec.Direction.Tone, TargetEmotion: spec.Direction.TargetEmotion, RiskRefs: []string{}, Status: "selected",
	}
	directionsPath := filepath.Join(root, "50-production", "plans", "fixture-directions.json")
	if err := replaceJSON(directionsPath, []CreativeDirection{direction}, 0o600); err != nil {
		return FinalizeContentBatchResult{}, nil, err
	}
	created, err := CreateContentBatch(CreateContentBatchOptions{
		Root: root, BriefID: brief.ID, DirectionsFile: relativeWorkspacePath(root, directionsPath), RequestedCount: len(spec.Items),
		VariantDimension: "hook", ControlledDimensions: []string{"audience", "cta"}, BatchID: spec.ID, Now: now.Add(time.Minute),
	})
	if err != nil {
		return FinalizeContentBatchResult{}, nil, err
	}
	files := make([]string, 0, len(spec.Items))
	for index, itemSpec := range spec.Items {
		item := blockedFixtureContentItem(created.Batch, direction, itemSpec, spec.BlockedReason, fixture.Project.Channel)
		path := filepath.Join(root, "50-production", "batches", localSafeName(spec.ID), fmt.Sprintf("content-%02d.json", index+1))
		if err := replaceJSON(path, item, 0o600); err != nil {
			return FinalizeContentBatchResult{}, nil, err
		}
		files = append(files, relativeWorkspacePath(root, path))
	}
	finalized, err := FinalizeContentBatch(root, created.BatchPath, files, now.Add(2*time.Minute))
	if err != nil {
		return FinalizeContentBatchResult{}, nil, err
	}
	return finalized, files, nil
}

func blockedFixtureContentItem(batch ContentBatch, direction CreativeDirection, spec fixturev3.ContentItemSpec, reason fixturev3.BlockedReasonSpec, channel string) ContentItem {
	return ContentItem{
		ID: spec.ID, Type: "content_item", Status: "blocked", SchemaVersion: ContentItemSchema, Deliverability: "blocked", ProjectID: batch.ProjectID,
		ContentID: spec.ContentID, ContentBatchID: batch.ID, BriefRef: batch.BriefRef, ContextSnapshotID: batch.ContextSnapshotID, ResolvedCommentIDs: []string{},
		Direction: direction, Title: spec.Title, Channel: channel, DurationMS: 8000, AspectRatio: "9:16",
		Cover:              ContentCover{Title: spec.Title, Subtitle: "", VisualIntent: direction.VisualMotif, FirstViewSignal: direction.HookType, AssetRefs: []string{}, RightsRefs: []string{}, SafeArea: "中央安全区", OcclusionGuards: []string{"不遮挡主体"}},
		NarrativeStructure: []ContentNarrativeSegment{}, Shots: []ContentShot{}, Citations: []ContentCitation{}, AssetRequirements: []ContentAssetRequirement{},
		Experiment:        ContentExperiment{PrimaryVariable: "hook", ControlledVariables: []string{"audience", "cta"}, Hypothesis: "不同开场影响首屏停留", MeasurementWindow: "24h", TargetMetrics: []string{"3s_retention"}},
		GlobalConstraints: ContentGlobalConstraints{ForbiddenClaims: []string{"未经证据支持的功效承诺"}, BrandRules: []string{}, ProductTruthRules: []string{"产品细节必须来自授权素材"}, ContinuityLocks: []string{}, PlatformSafeAreaRules: []string{"中央安全区"}},
		BlockedReasons:    []ContentBlockedReason{{Code: reason.Code, Message: reason.Message, OwnerRole: reason.OwnerRole, NextAction: reason.NextAction}}, MissingInputs: []string{spec.MissingInput},
		ValidationDeclarations: ContentValidationDeclarations{SchemaChecked: true, KnowledgeChecked: true, RightsChecked: true, ContinuityChecked: true, ExperimentChecked: true},
	}
}

func materializeFixtureRun(root string, scenario fixturev3.ScenarioSpec, knowledge []LocalKnowledgeItem, pack KnowledgePackResult, batch FinalizeContentBatchResult, contentFiles []string, now time.Time) (LocalRunContext, error) {
	sourceIDs := make([]string, 0, len(scenario.Sources))
	for _, source := range scenario.Sources {
		sourceIDs = append(sourceIDs, source.ID)
	}
	eligible := knowledgeItemIDs(knowledge)
	run, err := InitLocalRun(InitLocalRunOptions{Root: root, RunID: scenario.CompletedRun.ID, Intent: scenario.CompletedRun.Intent, InputIDs: sourceIDs, Now: now})
	if err != nil {
		return LocalRunContext{}, err
	}
	if _, err = CheckLocalRun(CheckLocalRunOptions{Root: root, RunID: run.RunID, Name: "kb-lint", Status: "passed", Command: "contentcloud local knowledge lint", Detail: "15 dimensions and seven layers validated", Now: now.Add(time.Minute)}); err != nil {
		return LocalRunContext{}, err
	}
	if _, err = AdvanceLocalRun(root, run.RunID, "query", RecordLocalRunOptions{}, now.Add(2*time.Minute)); err != nil {
		return LocalRunContext{}, err
	}
	if _, err = AdvanceLocalRun(root, run.RunID, "compile", RecordLocalRunOptions{EligibleIDs: eligible}, now.Add(3*time.Minute)); err != nil {
		return LocalRunContext{}, err
	}
	outputs := append([]string{pack.PackPath, batch.Batch.ContentItemRefs[0]}, contentFiles...)
	outputs = append(outputs, filepath.ToSlash(filepath.Join("50-production", "batches", localSafeName(batch.Batch.ID), "manifest.yaml")))
	if _, err = AdvanceLocalRun(root, run.RunID, "output-lint", RecordLocalRunOptions{ChangedIDs: append([]string{batch.Batch.ID}, eligible...), EligibleIDs: eligible, OutputPaths: outputs}, now.Add(4*time.Minute)); err != nil {
		return LocalRunContext{}, err
	}
	if _, err = CheckLocalRun(CheckLocalRunOptions{Root: root, RunID: run.RunID, Name: "content-lint", Status: "passed", Command: "contentcloud local content batch lint", Detail: "10 blocked ContentItems are structurally valid", Now: now.Add(5 * time.Minute)}); err != nil {
		return LocalRunContext{}, err
	}
	return AdvanceLocalRun(root, run.RunID, "done", RecordLocalRunOptions{BlockedIDs: contentFiles, Findings: []string{"等待客户补充授权素材"}}, now.Add(6*time.Minute))
}

func storeFixtureApprovedObject[T any](root, projectID, workspaceID, snapshotID, submissionType string, objects []T, eligibleIDs []string, now time.Time) error {
	canonical, err := json.Marshal(map[string]any{
		"schema_version": domain.SubmissionSchemaVersion(submissionType), "submission_type": submissionType, "objects": objects,
	})
	if err != nil {
		return err
	}
	hash, err := domain.CanonicalHash(json.RawMessage(canonical))
	if err != nil {
		return err
	}
	_, err = StoreApprovedSnapshot(root, domain.ApprovedSnapshot{
		ID: snapshotID, ProjectID: projectID, WorkspaceID: workspaceID, SubmissionType: submissionType, SchemaVersion: domain.SubmissionSchemaVersion(submissionType),
		ContentHash: "sha256:" + hash, SubjectHash: "sha256:" + hash, CanonicalContent: canonical, EligibleIDs: append([]string(nil), eligibleIDs...),
		Artifacts: []domain.SubmissionArtifact{}, CreatedBy: "fixture-v3", CreatedAt: now,
	}, now)
	return err
}

func knowledgeItemIDs(items []LocalKnowledgeItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

func knowledgeDirectory(kind string) string {
	switch kind {
	case "fact":
		return "facts"
	case "claim":
		return "claims"
	case "asset":
		return "assets"
	case "rights":
		return "rights"
	case "conflict":
		return "conflicts"
	default:
		return "domain"
	}
}

func fixtureTarget(targets []string) string {
	for _, target := range targets {
		if target == "codex" {
			return "codex"
		}
	}
	return "codex-plugin"
}
