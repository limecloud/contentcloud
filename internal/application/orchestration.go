package application

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type SaveEnvironmentInput struct {
	Name              string                                `json:"name"`
	Slug              string                                `json:"slug"`
	Status            string                                `json:"status"`
	DefaultSOPID      string                                `json:"default_sop_id"`
	DefaultSOPVersion int                                   `json:"default_sop_version"`
	Capabilities      []catalogdomain.EnvironmentCapability `json:"capabilities"`
}

type BindProjectSOPInput struct {
	EnvironmentID string `json:"environment_id"`
	SOPID         string `json:"sop_id"`
	SOPVersion    int    `json:"sop_version"`
	ContentType   string `json:"content_type,omitempty"`
}

type ProjectSOPBindingResult struct {
	Binding  catalogdomain.ProjectSOPBinding  `json:"binding"`
	SOP      catalogdomain.SOPVersion         `json:"sop"`
	Previous *catalogdomain.ProjectSOPBinding `json:"previous,omitempty"`
}

type CreateSOPInput struct {
	Name                 string                          `json:"name"`
	Description          string                          `json:"description"`
	ContentTypes         []string                        `json:"content_types"`
	Stages               []catalogdomain.StageDefinition `json:"stages"`
	Gates                []catalogdomain.GateDefinition  `json:"gates"`
	DefaultExecutionMode string                          `json:"default_execution_mode"`
}

type SOPLintIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type SOPLintReport struct {
	Valid    bool           `json:"valid"`
	Errors   []SOPLintIssue `json:"errors"`
	Warnings []SOPLintIssue `json:"warnings"`
}

type SaveSOPVersionInput struct {
	Version              int                             `json:"version"`
	Name                 string                          `json:"name"`
	Description          string                          `json:"description"`
	ContentTypes         []string                        `json:"content_types"`
	Stages               []catalogdomain.StageDefinition `json:"stages"`
	Gates                []catalogdomain.GateDefinition  `json:"gates"`
	DefaultExecutionMode string                          `json:"default_execution_mode"`
}

type SOPDiffChange struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type SOPVersionDiff struct {
	SOPID       string          `json:"sop_id"`
	FromVersion int             `json:"from_version"`
	ToVersion   int             `json:"to_version"`
	Same        bool            `json:"same"`
	Changes     []SOPDiffChange `json:"changes"`
}

type SOPEnvironmentImpact struct {
	EnvironmentID     string `json:"environment_id"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	DefaultSOPID      string `json:"default_sop_id"`
	DefaultSOPVersion int    `json:"default_sop_version"`
}

type SOPProjectImpact struct {
	ProjectID       string `json:"project_id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	BoundSOPVersion int    `json:"bound_sop_version"`
}

type SOPTaskImpact struct {
	TaskID          string `json:"task_id"`
	ProjectID       string `json:"project_id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	SOPVersion      int    `json:"sop_version"`
	RuntimeRunBound bool   `json:"runtime_run_bound"`
}

type SOPVersionImpact struct {
	SOPID        string                 `json:"sop_id"`
	Version      int                    `json:"version"`
	Environments []SOPEnvironmentImpact `json:"environments"`
	Projects     []SOPProjectImpact     `json:"projects"`
	Tasks        []SOPTaskImpact        `json:"tasks"`
	Counts       map[string]int         `json:"counts"`
}

// SOPCapabilityCoverage is a read-only projection for release review. It
// deliberately reports registered executor facts separately from the
// environment entitlement; an enabled environment capability is not evidence
// that a matching executor is actually registered.
type SOPCapabilityCoverage struct {
	ID                      string   `json:"id"`
	RequiredByStages        []string `json:"required_by_stages"`
	RegisteredVersions      []string `json:"registered_versions"`
	RegisteredExecutorCount int      `json:"registered_executor_count"`
}

type SOPEnvironmentBindingPreview struct {
	EnvironmentID          string                                `json:"environment_id"`
	Name                   string                                `json:"name"`
	Status                 string                                `json:"status"`
	ConfiguredCapabilities []catalogdomain.EnvironmentCapability `json:"configured_capabilities"`
	RequiredCapabilities   []string                              `json:"required_capabilities"`
	AvailableCapabilities  []string                              `json:"available_capabilities"`
	MissingCapabilities    []string                              `json:"missing_capabilities"`
	CandidateExecutorCount int                                   `json:"candidate_executor_count"`
	Ready                  bool                                  `json:"ready"`
	Reasons                []string                              `json:"reasons"`
}

// SOPVersionPreview aggregates only durable facts needed before publish or
// environment binding. It has no provider choice, execution side effect or
// synthetic canary result.
type SOPVersionPreview struct {
	SOP                   catalogdomain.SOPVersion       `json:"sop"`
	Lint                  SOPLintReport                  `json:"lint"`
	Impact                SOPVersionImpact               `json:"impact"`
	RequiredCapabilities  []string                       `json:"required_capabilities"`
	Capabilities          []SOPCapabilityCoverage        `json:"capabilities"`
	Environments          []SOPEnvironmentBindingPreview `json:"environments"`
	SelectedEnvironmentID string                         `json:"selected_environment_id,omitempty"`
	Publishable           bool                           `json:"publishable"`
	Blockers              []string                       `json:"blockers"`
	Warnings              []string                       `json:"warnings"`
}

type SOPRollbackResult struct {
	Version             catalogdomain.SOPVersion `json:"version"`
	TargetVersion       int                      `json:"target_version"`
	PreviousVersion     int                      `json:"previous_version"`
	ReboundEnvironments int                      `json:"rebound_environments"`
	ReboundProjects     int                      `json:"rebound_projects"`
	Impact              SOPVersionImpact         `json:"impact"`
}

type CreateWorkTaskInput struct {
	ProjectID       string         `json:"project_id"`
	EnvironmentID   string         `json:"environment_id"`
	SOPID           string         `json:"sop_id"`
	SOPVersion      int            `json:"sop_version"`
	Title           string         `json:"title"`
	Intent          string         `json:"intent"`
	ContentType     string         `json:"content_type"`
	InputRefs       []string       `json:"input_refs"`
	RequestedOutput map[string]any `json:"requested_output"`
	AssigneeUserID  string         `json:"assignee_user_id"`
	Priority        string         `json:"priority"`
	DueAt           *time.Time     `json:"due_at"`
	RiskProfile     string         `json:"risk_profile"`
	IdempotencyKey  string         `json:"idempotency_key,omitempty"`
}

type WorkTaskView struct {
	Task               work.WorkTask                       `json:"task"`
	Project            workspacedomain.Project             `json:"project"`
	Environment        catalogdomain.Environment           `json:"environment"`
	SOP                catalogdomain.SOPVersion            `json:"sop"`
	SourceRevisions    []sourcedomain.SourceRevision       `json:"source_revisions"`
	KnowledgeSnapshots []sourcedomain.KnowledgeSnapshot    `json:"knowledge_snapshots"`
	ApprovedSnapshots  []reviewdomain.ApprovedSnapshot     `json:"approved_snapshots"`
	StageRuns          []work.StageRun                     `json:"stage_runs"`
	Runs               []work.RuntimeRun                   `json:"runs"`
	Gates              []reviewdomain.GateEvaluation       `json:"gates"`
	Revisions          []reviewdomain.TaskRevision         `json:"revisions"`
	Deliveries         []deliverydomain.TaskDelivery       `json:"deliveries"`
	StageOutputs       []work.TaskStageOutput              `json:"stage_outputs"`
	MediaJobs          []deliverydomain.MediaGenerationJob `json:"media_jobs"`
	ProviderAttempts   []deliverydomain.ProviderAttempt    `json:"provider_attempts"`
	MediaReviews       []deliverydomain.MediaReview        `json:"media_reviews"`
	DeliveryPackages   []deliverydomain.DeliveryPackage    `json:"delivery_packages"`
	Artifacts          []deliverydomain.Artifact           `json:"artifacts"`
	AllowedActions     []string                            `json:"allowed_actions"`
	GeneratedAt        time.Time                           `json:"generated_at"`
}

func (s *WorkService) ensureOrchestrationDefaults(ctx context.Context, actor Actor) (catalogdomain.Environment, catalogdomain.SOPVersion, error) {
	environments, err := s.catalog.Environments(ctx, actor.TenantID)
	if err != nil {
		return catalogdomain.Environment{}, catalogdomain.SOPVersion{}, err
	}
	sops, err := s.catalog.SOPs(ctx, actor.TenantID)
	if err != nil {
		return catalogdomain.Environment{}, catalogdomain.SOPVersion{}, err
	}
	sops, err = s.app.Catalog.ensureBuiltinSOPs(ctx, actor, sops)
	if err != nil {
		return catalogdomain.Environment{}, catalogdomain.SOPVersion{}, err
	}
	var sop catalogdomain.SOPVersion
	for _, summary := range sops {
		for _, candidate := range summary.Versions {
			if candidate.Status == "published" && summary.Definition.TemplateKey == builtinSOPMarketingVideo && (sop.ID == "" || candidate.Version > sop.Version) {
				sop = candidate
			}
		}
	}
	if sop.ID == "" {
		for _, summary := range sops {
			for _, candidate := range summary.Versions {
				if candidate.Status == "published" && (sop.ID == "" || candidate.Version > sop.Version) {
					sop = candidate
				}
			}
		}
	}
	if len(environments) == 0 {
		now := s.now().UTC()
		environment := catalogdomain.Environment{ID: idgen.New(), TenantID: actor.TenantID, Name: "内容生产默认环境", Slug: "production", Status: "active", DefaultSOPID: sop.SOPID, DefaultSOPVersion: sop.Version, ManifestDigest: "sha256:" + strings.Repeat("0", 64), Capabilities: []catalogdomain.EnvironmentCapability{{ID: "content.script.compose", Version: "1.0.0", Enabled: true}, {ID: "content.article.compose", Version: "1.0.0", Enabled: true}}, CreatedAt: now, UpdatedAt: now}
		if err := s.catalog.CreateEnvironment(ctx, environment); err != nil {
			return catalogdomain.Environment{}, catalogdomain.SOPVersion{}, err
		}
		return environment, sop, nil
	}
	environment := environments[0]
	for _, candidate := range environments {
		if candidate.Status == "active" {
			environment = candidate
			break
		}
	}
	if environment.DefaultSOPID != "" {
		defaultSOPValid := false
		if summary, summaryErr := s.catalog.SOP(ctx, actor.TenantID, environment.DefaultSOPID); summaryErr == nil {
			for _, candidate := range summary.Versions {
				if candidate.Version == environment.DefaultSOPVersion && candidate.Status == "published" {
					sop = candidate
					defaultSOPValid = true
					break
				}
			}
		}
		if !defaultSOPValid {
			environment.DefaultSOPID = ""
			environment.DefaultSOPVersion = 0
		}
	}
	if environment.DefaultSOPID == "" && sop.SOPID != "" {
		environment.DefaultSOPID, environment.DefaultSOPVersion = sop.SOPID, sop.Version
		environment.UpdatedAt = s.now().UTC()
		if err := s.catalog.SaveEnvironment(ctx, environment); err != nil {
			return catalogdomain.Environment{}, catalogdomain.SOPVersion{}, err
		}
	}
	return environment, sop, nil
}

func (s *WorkService) AdminWorkOS(ctx context.Context, actor Actor) (catalogdomain.AdminWorkOSView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.AdminWorkOSView{}, err
	}
	_, _, err := s.ensureOrchestrationDefaults(ctx, actor)
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	environments, err := s.catalog.Environments(ctx, actor.TenantID)
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	sops, err := s.catalog.SOPs(ctx, actor.TenantID)
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	for index := range environments {
		environments[index].NormalizeCollections()
	}
	for index := range sops {
		sops[index].Definition.NormalizeCollections()
		for versionIndex := range sops[index].Versions {
			sops[index].Versions[versionIndex].NormalizeCollections()
		}
	}
	gates := []catalogdomain.GateSummary{}
	for _, summary := range sops {
		for _, version := range summary.Versions {
			if version.Status != "published" && version.Status != "draft" {
				continue
			}
			for _, gate := range version.Gates {
				gates = append(gates, catalogdomain.GateSummary{SOPID: summary.Definition.ID, SOPName: summary.Definition.Name, SOPVersion: version.Version, ID: gate.ID, Name: gate.Name, Mode: gate.Mode, Blocking: gate.Blocking})
			}
		}
	}
	audit, err := s.auditRepo.AuditEvents(ctx, actor.TenantID, "", 30)
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	tasks, err := s.tasks.WorkTasks(ctx, actor.TenantID, "")
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	for index := range gates {
		for _, task := range tasks {
			if task.SOPID == gates[index].SOPID && task.SOPVersion == gates[index].SOPVersion {
				gates[index].UsageCount++
			}
		}
	}
	devices, err := s.workspace.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return catalogdomain.AdminWorkOSView{}, err
	}
	usage := catalogdomain.UsageSummary{TaskCount: len(tasks), ByExecutionMode: map[string]int{}}
	capabilityByKey := map[string]catalogdomain.Capability{}
	for _, device := range devices {
		for _, capability := range device.Capabilities {
			capabilityByKey[capability.ID+":"+capability.Version] = capability
		}
	}
	for _, task := range tasks {
		if task.Status == "running" {
			usage.RunningCount++
		}
		if task.Status == "waiting_gate" {
			usage.WaitingGateCount++
		}
		runs, runErr := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return catalogdomain.AdminWorkOSView{}, runErr
		}
		for _, run := range runs {
			mode := defaultString(run.ExecutionMode, "unspecified")
			usage.ByExecutionMode[mode]++
		}
	}
	capabilities := make([]catalogdomain.Capability, 0, len(capabilityByKey))
	for _, capability := range capabilityByKey {
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool {
		if capabilities[i].ID == capabilities[j].ID {
			return capabilities[i].Version < capabilities[j].Version
		}
		return capabilities[i].ID < capabilities[j].ID
	})
	return catalogdomain.AdminWorkOSView{Environments: environments, SOPs: sops, Gates: gates, Capabilities: capabilities, Audit: audit, Usage: usage, GeneratedAt: s.now().UTC()}, nil
}

func (s *WorkService) SaveEnvironment(ctx context.Context, actor Actor, id string, input SaveEnvironmentInput, requestID string) (catalogdomain.Environment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.Environment{}, err
	}
	value, err := s.catalog.Environment(ctx, actor.TenantID, id)
	if err != nil {
		return value, err
	}
	if strings.TrimSpace(input.Name) != "" {
		value.Name = strings.TrimSpace(input.Name)
	}
	if strings.TrimSpace(input.Slug) != "" {
		value.Slug = strings.TrimSpace(input.Slug)
	}
	if input.Status != "" {
		if input.Status != "active" && input.Status != "paused" {
			return value, fault.Invalid("ENVIRONMENT_STATUS_INVALID", "执行环境状态只能是“运行中（active）”或“已暂停（paused）”")
		}
		value.Status = input.Status
	}
	if input.DefaultSOPID != "" {
		summary, summaryErr := s.catalog.SOP(ctx, actor.TenantID, input.DefaultSOPID)
		if summaryErr != nil {
			return value, summaryErr
		}
		validVersion := false
		for _, candidate := range summary.Versions {
			if candidate.Version == input.DefaultSOPVersion && candidate.Status == "published" {
				validVersion = true
				break
			}
		}
		if !validVersion {
			return value, fault.Policy("DEFAULT_SOP_NOT_PUBLISHED", "执行环境的默认流程规范必须是当前租户已发布的版本", "先发布该流程规范版本，再设为默认")
		}
		value.DefaultSOPID = input.DefaultSOPID
		value.DefaultSOPVersion = input.DefaultSOPVersion
	}
	if value.DefaultSOPID == "" || value.DefaultSOPVersion < 1 {
		return value, fault.Policy("ENVIRONMENT_DEFAULT_SOP_REQUIRED", "执行环境必须绑定已发布的流程规范版本", "先选择一条已发布的流程规范，再保存执行环境")
	}
	if input.Capabilities != nil {
		value.Capabilities = input.Capabilities
	}
	value.UpdatedAt = s.now().UTC()
	if err := s.catalog.SaveEnvironment(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, "", "environment.updated", "environment", value.ID, requestID, map[string]any{"status": value.Status, "default_sop_id": value.DefaultSOPID, "default_sop_version": value.DefaultSOPVersion})
	return value, nil
}

func (s *WorkService) CreateEnvironment(ctx context.Context, actor Actor, input SaveEnvironmentInput, requestID string) (catalogdomain.Environment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.Environment{}, err
	}
	name := strings.TrimSpace(input.Name)
	slug := strings.TrimSpace(input.Slug)
	if name == "" || slug == "" {
		return catalogdomain.Environment{}, fault.Invalid("ENVIRONMENT_FIELDS_REQUIRED", "执行环境名称和标识不能为空")
	}
	value := catalogdomain.Environment{ID: idgen.New(), TenantID: actor.TenantID, Name: name, Slug: slug, Status: defaultString(input.Status, "active"), Capabilities: append([]catalogdomain.EnvironmentCapability(nil), input.Capabilities...), CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
	value.NormalizeCollections()
	if value.Status != "active" && value.Status != "paused" {
		return catalogdomain.Environment{}, fault.Invalid("ENVIRONMENT_STATUS_INVALID", "执行环境状态只能是“运行中（active）”或“已暂停（paused）”")
	}
	if input.DefaultSOPID == "" || input.DefaultSOPVersion < 1 {
		return catalogdomain.Environment{}, fault.Policy("ENVIRONMENT_DEFAULT_SOP_REQUIRED", "执行环境必须绑定已发布的流程规范版本", "先选择一条已发布的流程规范，再创建执行环境")
	}
	if input.DefaultSOPID != "" {
		summary, err := s.catalog.SOP(ctx, actor.TenantID, input.DefaultSOPID)
		if err != nil {
			return catalogdomain.Environment{}, err
		}
		for _, candidate := range summary.Versions {
			if candidate.Version == input.DefaultSOPVersion && candidate.Status == "published" {
				value.DefaultSOPID = input.DefaultSOPID
				value.DefaultSOPVersion = input.DefaultSOPVersion
				break
			}
		}
		if value.DefaultSOPID == "" {
			return catalogdomain.Environment{}, fault.Policy("DEFAULT_SOP_NOT_PUBLISHED", "执行环境的默认流程规范必须是当前租户已发布的版本", "先发布该流程规范版本，再创建执行环境")
		}
	}
	if err := value.Validate(); err != nil {
		return catalogdomain.Environment{}, err
	}
	if err := s.catalog.CreateEnvironment(ctx, value); err != nil {
		return catalogdomain.Environment{}, err
	}
	s.audit(ctx, actor, "", "environment.created", "environment", value.ID, requestID, map[string]any{"slug": value.Slug, "status": value.Status, "default_sop_id": value.DefaultSOPID, "default_sop_version": value.DefaultSOPVersion})
	return value, nil
}

func (s *WorkService) CreateSOP(ctx context.Context, actor Actor, input CreateSOPInput, requestID string) (catalogdomain.SOPSummary, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.SOPSummary{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return catalogdomain.SOPSummary{}, fault.Invalid("SOP_NAME_REQUIRED", "流程规范名称不能为空")
	}
	contentTypes := append([]string(nil), input.ContentTypes...)
	if len(contentTypes) == 0 {
		contentTypes = []string{identitydomain.ContentTypeVideoScript}
	}
	stages := append([]catalogdomain.StageDefinition(nil), input.Stages...)
	if len(stages) == 0 {
		stages = []catalogdomain.StageDefinition{{ID: "input", Name: "需求输入", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}, Checks: []string{"brief.required"}}}
	}
	now := s.now().UTC()
	sopID := idgen.New()
	definition := catalogdomain.SOPDefinition{ID: sopID, TenantID: actor.TenantID, Name: name, Description: input.Description, ContentTypes: contentTypes, CurrentVersion: 0, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	version := catalogdomain.SOPVersion{ID: idgen.New(), TenantID: actor.TenantID, SOPID: sopID, Version: 1, SchemaVersion: catalogdomain.SOPSchemaVersion, Name: name, Description: input.Description, ContentTypes: contentTypes, Stages: stages, Gates: append([]catalogdomain.GateDefinition(nil), input.Gates...), DefaultExecutionMode: defaultString(input.DefaultExecutionMode, "local"), Status: "draft", CreatedBy: actor.UserID, CreatedAt: now}
	definition.NormalizeCollections()
	version.NormalizeCollections()
	if err := version.Validate(); err != nil {
		return catalogdomain.SOPSummary{}, err
	}
	if err := s.catalog.CreateSOP(ctx, definition, version); err != nil {
		return catalogdomain.SOPSummary{}, err
	}
	s.audit(ctx, actor, "", "sop.created", "sop", sopID, requestID, map[string]any{"version": 1, "status": "draft"})
	return catalogdomain.SOPSummary{Definition: definition, Versions: []catalogdomain.SOPVersion{version}}, nil
}

func (s *WorkService) SaveSOPVersion(ctx context.Context, actor Actor, sopID string, version int, input SaveSOPVersionInput, requestID string) (catalogdomain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.SOPVersion{}, err
	}
	value, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	var current catalogdomain.SOPVersion
	for _, candidate := range value.Versions {
		if candidate.Version == version {
			current = candidate
			break
		}
	}
	if current.ID == "" {
		return current, fault.NotFound("流程规范版本")
	}
	if current.Status != "draft" {
		return current, fault.Conflict("SOP_VERSION_IMMUTABLE", "已发布的流程规范版本不可直接修改，请复制为新草稿")
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = input.Description
	current.ContentTypes = input.ContentTypes
	current.Stages = input.Stages
	current.Gates = input.Gates
	current.DefaultExecutionMode = input.DefaultExecutionMode
	current.Digest = ""
	current.NormalizeCollections()
	if err := s.catalog.SaveSOPVersion(ctx, current); err != nil {
		return current, err
	}
	s.audit(ctx, actor, "", "sop.version_saved", "sop_version", current.ID, requestID, map[string]any{"sop_id": sopID, "version": version})
	return current, nil
}

func (s *WorkService) SOPVersionDiff(ctx context.Context, actor Actor, sopID string, fromVersion, toVersion int) (SOPVersionDiff, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPVersionDiff{}, err
	}
	from, err := s.sopVersion(ctx, actor.TenantID, sopID, fromVersion)
	if err != nil {
		return SOPVersionDiff{}, err
	}
	to, err := s.sopVersion(ctx, actor.TenantID, sopID, toVersion)
	if err != nil {
		return SOPVersionDiff{}, err
	}
	return diffSOPVersions(from, to), nil
}

func diffSOPVersions(from, to catalogdomain.SOPVersion) SOPVersionDiff {
	from.NormalizeCollections()
	to.NormalizeCollections()
	result := SOPVersionDiff{SOPID: to.SOPID, FromVersion: from.Version, ToVersion: to.Version, Changes: []SOPDiffChange{}}
	add := func(path string, before, after any) {
		if reflect.DeepEqual(before, after) {
			return
		}
		result.Changes = append(result.Changes, SOPDiffChange{Path: path, Before: before, After: after})
	}
	add("name", from.Name, to.Name)
	add("description", from.Description, to.Description)
	add("content_types", from.ContentTypes, to.ContentTypes)
	add("default_execution_mode", from.DefaultExecutionMode, to.DefaultExecutionMode)
	fromStages := map[string]catalogdomain.StageDefinition{}
	toStages := map[string]catalogdomain.StageDefinition{}
	for _, value := range from.Stages {
		fromStages[value.ID] = value
	}
	for _, value := range to.Stages {
		toStages[value.ID] = value
	}
	stageIDs := make([]string, 0, len(fromStages)+len(toStages))
	seenStages := map[string]bool{}
	for id := range fromStages {
		seenStages[id] = true
		stageIDs = append(stageIDs, id)
	}
	for id := range toStages {
		if !seenStages[id] {
			stageIDs = append(stageIDs, id)
		}
	}
	sort.Strings(stageIDs)
	for _, id := range stageIDs {
		before, beforeOK := fromStages[id]
		after, afterOK := toStages[id]
		if !beforeOK {
			add("stages["+id+"]", nil, after)
		} else if !afterOK {
			add("stages["+id+"]", before, nil)
		} else {
			add("stages["+id+"]", before, after)
		}
	}
	fromGates := map[string]catalogdomain.GateDefinition{}
	toGates := map[string]catalogdomain.GateDefinition{}
	for _, value := range from.Gates {
		fromGates[value.ID] = value
	}
	for _, value := range to.Gates {
		toGates[value.ID] = value
	}
	gateIDs := make([]string, 0, len(fromGates)+len(toGates))
	seenGates := map[string]bool{}
	for id := range fromGates {
		seenGates[id] = true
		gateIDs = append(gateIDs, id)
	}
	for id := range toGates {
		if !seenGates[id] {
			gateIDs = append(gateIDs, id)
		}
	}
	sort.Strings(gateIDs)
	for _, id := range gateIDs {
		before, beforeOK := fromGates[id]
		after, afterOK := toGates[id]
		if !beforeOK {
			add("gates["+id+"]", nil, after)
		} else if !afterOK {
			add("gates["+id+"]", before, nil)
		} else {
			add("gates["+id+"]", before, after)
		}
	}
	result.Same = len(result.Changes) == 0
	return result
}

func (s *WorkService) SOPVersionImpact(ctx context.Context, actor Actor, sopID string, version int) (SOPVersionImpact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPVersionImpact{}, err
	}
	if _, err := s.sopVersion(ctx, actor.TenantID, sopID, version); err != nil {
		return SOPVersionImpact{}, err
	}
	environments, err := s.catalog.Environments(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	bindings, err := s.catalog.ProjectSOPBindings(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	projects, err := s.workspace.Projects(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	tasks, err := s.tasks.WorkTasks(ctx, actor.TenantID, "")
	if err != nil {
		return SOPVersionImpact{}, err
	}
	result := SOPVersionImpact{SOPID: sopID, Version: version, Environments: []SOPEnvironmentImpact{}, Projects: []SOPProjectImpact{}, Tasks: []SOPTaskImpact{}, Counts: map[string]int{"environments": 0, "projects": 0, "tasks": 0, "active_tasks": 0}}
	for _, environment := range environments {
		if environment.DefaultSOPID == sopID && environment.DefaultSOPVersion == version {
			result.Environments = append(result.Environments, SOPEnvironmentImpact{EnvironmentID: environment.ID, Name: environment.Name, Status: environment.Status, DefaultSOPID: environment.DefaultSOPID, DefaultSOPVersion: environment.DefaultSOPVersion})
		}
	}
	projectByID := map[string]workspacedomain.Project{}
	for _, project := range projects {
		projectByID[project.ID] = project
	}
	for _, binding := range bindings {
		if binding.SOPID != sopID || binding.SOPVersion != version {
			continue
		}
		project := projectByID[binding.ProjectID]
		result.Projects = append(result.Projects, SOPProjectImpact{ProjectID: binding.ProjectID, Name: project.BrandName + " · " + project.ProductName, Status: project.Status, BoundSOPVersion: binding.SOPVersion})
	}
	for _, task := range tasks {
		if task.SOPID != sopID || task.SOPVersion != version {
			continue
		}
		result.Tasks = append(result.Tasks, SOPTaskImpact{TaskID: task.ID, ProjectID: task.ProjectID, Title: task.Title, Status: task.Status, SOPVersion: task.SOPVersion, RuntimeRunBound: task.Status == work.TaskStatusRunning || task.Status == work.TaskStatusWaitingGate || task.Status == work.TaskStatusAccepted || task.Status == work.TaskStatusDelivered})
	}
	result.Counts["environments"] = len(result.Environments)
	result.Counts["projects"] = len(result.Projects)
	result.Counts["tasks"] = len(result.Tasks)
	for _, task := range result.Tasks {
		if task.Status == work.TaskStatusRunning || task.Status == work.TaskStatusWaitingGate {
			result.Counts["active_tasks"]++
		}
	}
	return result, nil
}

// SOPVersionPreview performs the complete read-only release and binding
// preflight. It does not mutate defaults, create snapshots or choose an
// executor. When environmentID is empty, all tenant environments are shown;
// when it is set, publishability also reflects that exact binding target.
func (s *WorkService) SOPVersionPreview(ctx context.Context, actor Actor, sopID string, version int, environmentID string) (SOPVersionPreview, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPVersionPreview{}, err
	}
	sop, err := s.sopVersion(ctx, actor.TenantID, sopID, version)
	if err != nil {
		return SOPVersionPreview{}, err
	}
	lint, err := s.LintSOPVersion(ctx, actor, sopID, version)
	if err != nil {
		return SOPVersionPreview{}, err
	}
	impact, err := s.SOPVersionImpact(ctx, actor, sopID, version)
	if err != nil {
		return SOPVersionPreview{}, err
	}
	environments, err := s.catalog.Environments(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionPreview{}, err
	}
	if strings.TrimSpace(environmentID) != "" {
		found := false
		for _, environment := range environments {
			if environment.ID == environmentID {
				found = true
				break
			}
		}
		if !found {
			return SOPVersionPreview{}, fault.NotFound("执行环境")
		}
	}

	devices, err := s.workspace.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return SOPVersionPreview{}, err
	}
	type capabilityFact struct {
		versions  map[string]bool
		executors map[string]bool
	}
	facts := map[string]*capabilityFact{}
	for _, device := range devices {
		if device.RevokedAt != nil {
			continue
		}
		for _, capability := range device.Capabilities {
			id := strings.TrimSpace(capability.ID)
			if id == "" {
				continue
			}
			fact := facts[id]
			if fact == nil {
				fact = &capabilityFact{versions: map[string]bool{}, executors: map[string]bool{}}
				facts[id] = fact
			}
			if version := strings.TrimSpace(capability.Version); version != "" {
				fact.versions[version] = true
			}
			fact.executors[device.ID] = true
		}
	}

	requiredByStage := map[string][]string{}
	for _, stage := range sop.Stages {
		for _, capabilityID := range uniqueNonEmpty(stage.RequiredCapabilities) {
			if !containsString(requiredByStage[capabilityID], stage.ID) {
				requiredByStage[capabilityID] = append(requiredByStage[capabilityID], stage.ID)
			}
		}
	}
	required := make([]string, 0, len(requiredByStage))
	for capabilityID := range requiredByStage {
		required = append(required, capabilityID)
	}
	sort.Strings(required)

	coverage := make([]SOPCapabilityCoverage, 0, len(required))
	for _, capabilityID := range required {
		fact := facts[capabilityID]
		versions := []string{}
		executorCount := 0
		if fact != nil {
			for registeredVersion := range fact.versions {
				versions = append(versions, registeredVersion)
			}
			sort.Strings(versions)
			executorCount = len(fact.executors)
		}
		stages := append([]string{}, requiredByStage[capabilityID]...)
		sort.Strings(stages)
		coverage = append(coverage, SOPCapabilityCoverage{ID: capabilityID, RequiredByStages: stages, RegisteredVersions: versions, RegisteredExecutorCount: executorCount})
	}

	previewEnvironments := make([]SOPEnvironmentBindingPreview, 0, len(environments))
	blockers := []string{}
	warnings := []string{}
	if !lint.Valid {
		for _, issue := range lint.Errors {
			blockers = append(blockers, issue.Path+"："+issue.Message)
		}
	}
	for _, environment := range environments {
		if selected := strings.TrimSpace(environmentID); selected != "" && environment.ID != selected {
			continue
		}
		environment.NormalizeCollections()
		configured := map[string]catalogdomain.EnvironmentCapability{}
		for _, capability := range environment.Capabilities {
			configured[capability.ID] = capability
		}
		available := []string{}
		missing := []string{}
		reasons := []string{}
		for _, capabilityID := range required {
			configuredCapability, configuredOK := configured[capabilityID]
			if !configuredOK || !configuredCapability.Enabled {
				missing = append(missing, capabilityID)
				reasons = append(reasons, capabilityID+"：执行环境未启用")
				continue
			}
			fact := facts[capabilityID]
			if fact == nil || !fact.versions[configuredCapability.Version] {
				missing = append(missing, capabilityID)
				reasons = append(reasons, capabilityID+"：没有登记匹配版本的执行端能力")
				continue
			}
			available = append(available, capabilityID)
		}
		candidateExecutorCount := 0
		for _, device := range devices {
			if device.RevokedAt != nil {
				continue
			}
			if len(required) == 0 {
				candidateExecutorCount++
				continue
			}
			for _, capabilityID := range required {
				configuredCapability, configuredOK := configured[capabilityID]
				if configuredOK && configuredCapability.Enabled && deviceHasCapability(device, capabilityID, configuredCapability.Version) {
					candidateExecutorCount++
					break
				}
			}
		}
		ready := len(missing) == 0
		preview := SOPEnvironmentBindingPreview{EnvironmentID: environment.ID, Name: environment.Name, Status: environment.Status, ConfiguredCapabilities: append([]catalogdomain.EnvironmentCapability{}, environment.Capabilities...), RequiredCapabilities: append([]string{}, required...), AvailableCapabilities: available, MissingCapabilities: missing, CandidateExecutorCount: candidateExecutorCount, Ready: ready, Reasons: reasons}
		previewEnvironments = append(previewEnvironments, preview)
		if len(missing) > 0 {
			message := environment.Name + "：" + strings.Join(reasons, "；")
			if strings.TrimSpace(environmentID) != "" {
				blockers = append(blockers, message)
			} else {
				warnings = append(warnings, message)
			}
		}
	}
	publishable := len(blockers) == 0
	return SOPVersionPreview{SOP: sop, Lint: lint, Impact: impact, RequiredCapabilities: required, Capabilities: coverage, Environments: previewEnvironments, SelectedEnvironmentID: strings.TrimSpace(environmentID), Publishable: publishable, Blockers: blockers, Warnings: warnings}, nil
}

func deviceHasCapability(device workspacedomain.Device, capabilityID, version string) bool {
	for _, capability := range device.Capabilities {
		if capability.ID == capabilityID && capability.Version == version {
			return true
		}
	}
	return false
}

func (s *WorkService) RollbackSOPVersion(ctx context.Context, actor Actor, sopID string, targetVersion int, requestID string) (SOPRollbackResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPRollbackResult{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	target, err := s.sopVersionFromSummary(summary, targetVersion)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	if target.Status != "published" && target.Status != "retired" {
		return SOPRollbackResult{}, fault.Policy("SOP_ROLLBACK_SOURCE_INVALID", "只能从已发布或已停用的版本回滚", "选择一个有摘要的历史版本")
	}
	previousVersion := summary.Definition.CurrentVersion
	if previousVersion == 0 {
		for _, candidate := range summary.Versions {
			if candidate.Status == "published" && candidate.Version > previousVersion {
				previousVersion = candidate.Version
			}
		}
	}
	if targetVersion == previousVersion {
		return SOPRollbackResult{}, fault.Policy("SOP_ROLLBACK_NOOP", "目标版本已经是当前版本", "选择更早的已发布版本")
	}
	impact, err := s.SOPVersionImpact(ctx, actor, sopID, previousVersion)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	draft, err := s.CreateSOPDraft(ctx, actor, sopID, targetVersion, requestID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	published, err := s.PublishSOP(ctx, actor, sopID, draft.Version, requestID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	now := s.now().UTC()
	reboundEnvironments := 0
	environments, err := s.catalog.Environments(ctx, actor.TenantID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	for _, environment := range environments {
		if environment.DefaultSOPID != sopID || environment.DefaultSOPVersion != previousVersion {
			continue
		}
		environment.DefaultSOPVersion = published.Version
		environment.UpdatedAt = now
		if err := s.catalog.SaveEnvironment(ctx, environment); err != nil {
			return SOPRollbackResult{}, err
		}
		reboundEnvironments++
	}
	reboundProjects := 0
	bindings, err := s.catalog.ProjectSOPBindings(ctx, actor.TenantID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	for _, binding := range bindings {
		if binding.SOPID != sopID || binding.SOPVersion != previousVersion {
			continue
		}
		binding.SOPVersion = published.Version
		binding.SOPDigest = published.Digest
		binding.BoundBy = actor.UserID
		binding.BoundAt = now
		if err := s.catalog.SaveProjectSOPBinding(ctx, binding); err != nil {
			return SOPRollbackResult{}, err
		}
		reboundProjects++
	}
	s.audit(ctx, actor, "", "sop.version_rolled_back", "sop", sopID, requestID, map[string]any{"target_version": targetVersion, "previous_version": previousVersion, "published_version": published.Version, "rebound_environments": reboundEnvironments, "rebound_projects": reboundProjects, "active_tasks": impact.Counts["active_tasks"]})
	return SOPRollbackResult{Version: published, TargetVersion: targetVersion, PreviousVersion: previousVersion, ReboundEnvironments: reboundEnvironments, ReboundProjects: reboundProjects, Impact: impact}, nil
}

func (s *WorkService) sopVersion(ctx context.Context, tenantID, sopID string, version int) (catalogdomain.SOPVersion, error) {
	summary, err := s.catalog.SOP(ctx, tenantID, sopID)
	if err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	return s.sopVersionFromSummary(summary, version)
}

func (s *WorkService) sopVersionFromSummary(summary catalogdomain.SOPSummary, version int) (catalogdomain.SOPVersion, error) {
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			return candidate, nil
		}
	}
	return catalogdomain.SOPVersion{}, fault.NotFound("流程规范版本")
}

func (s *WorkService) CreateSOPDraft(ctx context.Context, actor Actor, sopID string, sourceVersion int, requestID string) (catalogdomain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.SOPVersion{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	var source catalogdomain.SOPVersion
	maxVersion := 0
	for _, candidate := range summary.Versions {
		if candidate.Version > maxVersion {
			maxVersion = candidate.Version
		}
		if sourceVersion > 0 && candidate.Version == sourceVersion {
			source = candidate
		}
		if sourceVersion == 0 && source.ID == "" && candidate.Status == "published" {
			source = candidate
		}
	}
	if source.ID == "" {
		return catalogdomain.SOPVersion{}, fault.NotFound("流程规范源版本")
	}
	now := s.now().UTC()
	draft := source
	draft.ID = idgen.New()
	draft.Version = maxVersion + 1
	draft.Status = "draft"
	draft.Digest = ""
	draft.CreatedBy = actor.UserID
	draft.PublishedBy = ""
	draft.PublishedAt = nil
	draft.CreatedAt = now
	draft.NormalizeCollections()
	if err := s.catalog.CreateSOPVersion(ctx, draft); err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	s.audit(ctx, actor, "", "sop.version_created", "sop_version", draft.ID, requestID, map[string]any{"sop_id": sopID, "version": draft.Version, "source_version": source.Version})
	return draft, nil
}

func (s *WorkService) PublishSOP(ctx context.Context, actor Actor, sopID string, version int, requestID string) (catalogdomain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return catalogdomain.SOPVersion{}, err
	}
	report, err := s.LintSOPVersion(ctx, actor, sopID, version)
	if err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	if !report.Valid {
		return catalogdomain.SOPVersion{}, fault.Policy("SOP_LINT_FAILED", "流程规范发布前检查未通过", "修正流程阶段、检查与审批及执行方式配置后重试")
	}
	value, err := s.catalog.PublishSOPVersion(ctx, actor.TenantID, sopID, version, actor.UserID, s.now().UTC())
	if err != nil {
		return value, err
	}
	s.audit(ctx, actor, "", "sop.version_published", "sop_version", value.ID, requestID, map[string]any{"sop_id": sopID, "version": version, "digest": value.Digest})
	return value, nil
}

func (s *WorkService) RetireSOPVersion(ctx context.Context, actor Actor, sopID string, version int, requestID string) error {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return err
	}
	value, err := s.sopVersionFromSummary(summary, version)
	if err != nil {
		return err
	}
	if value.Status != "published" {
		return fault.Conflict("SOP_VERSION_STATE_INVALID", "只有已发布的流程规范版本可以停用")
	}
	if err := s.catalog.RetireSOPVersion(ctx, actor.TenantID, sopID, version, s.now().UTC()); err != nil {
		return err
	}
	s.audit(ctx, actor, "", "sop.version_retired", "sop_version", value.ID, requestID, map[string]any{"sop_id": sopID, "version": version})
	return nil
}

func (s *WorkService) LintSOPVersion(ctx context.Context, actor Actor, sopID string, version int) (SOPLintReport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPLintReport{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return SOPLintReport{}, err
	}
	var value catalogdomain.SOPVersion
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			value = candidate
			break
		}
	}
	if value.ID == "" {
		return SOPLintReport{}, fault.NotFound("流程规范版本")
	}
	report := SOPLintReport{Errors: []SOPLintIssue{}, Warnings: []SOPLintIssue{}}
	addError := func(code, path, message string) {
		report.Errors = append(report.Errors, SOPLintIssue{Code: code, Path: path, Message: message})
	}
	addWarning := func(code, path, message string) {
		report.Warnings = append(report.Warnings, SOPLintIssue{Code: code, Path: path, Message: message})
	}
	if err := value.Validate(); err != nil {
		addError("schema.invalid", "sop", err.Error())
	}
	if len(value.ContentTypes) == 0 {
		addError("content_type.required", "content_types", "至少选择一种内容类型")
	}
	if len(value.Stages) == 0 {
		addError("stage.required", "stages", "流程规范至少需要一个流程阶段")
	}
	if strings.TrimSpace(value.DefaultExecutionMode) == "" {
		addError("execution_mode.required", "default_execution_mode", "必须配置默认执行方式")
	}
	seenStages := map[string]bool{}
	usedGates := map[string]bool{}
	for index, stage := range value.Stages {
		path := "stages[" + strconv.Itoa(index) + "]"
		for _, ref := range stage.InputRefs {
			if !seenStages[ref] {
				addError("stage.input_ref.invalid", path+".input_refs", "输入引用必须指向当前流程阶段之前的阶段")
			}
		}
		if len(stage.ExecutionModes) == 0 {
			addError("stage.execution_mode.required", path+".execution_modes", "流程阶段至少需要一种执行方式")
		}
		for _, gateID := range stage.GateIDs {
			usedGates[gateID] = true
		}
		seenStages[stage.ID] = true
	}
	for index, gate := range value.Gates {
		path := "gates[" + strconv.Itoa(index) + "]"
		humanGate := gate.Mode == catalogdomain.GateModeInternalReview || gate.Mode == catalogdomain.GateModeClientDecision
		if humanGate && !gate.Blocking {
			addError("gate.required_not_blocking", path+".blocking", "人工审核必须阻断后续流程阶段")
		}
		if !humanGate && gate.Mode != catalogdomain.GateModeRequiredCheck && gate.Blocking {
			addError("gate.optional_blocking", path+".blocking", "无审批（none）、可选建议（advisory）或必做检查不能阻断后续流程阶段")
		}
		if !usedGates[gate.ID] {
			addWarning("gate.unused", path, "检查与审批项尚未绑定到任何流程阶段")
		}
		if humanGate && len(gate.AssigneeRoles) == 0 {
			addWarning("gate.assignee_roles.empty", path+".assignee_roles", "人工审核建议配置决定角色")
		}
	}
	report.Valid = len(report.Errors) == 0
	return report, nil
}

func (s *WorkService) ProjectSOP(ctx context.Context, actor Actor, projectID string) (catalogdomain.ProjectSOPBinding, catalogdomain.SOPVersion, error) {
	project, err := s.workspace.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return catalogdomain.ProjectSOPBinding{}, catalogdomain.SOPVersion{}, err
	}
	projectContentType := defaultString(project.ContentType, identitydomain.DefaultProjectContentType)
	environment, defaultSOP, err := s.ensureOrchestrationDefaults(ctx, actor)
	if err != nil {
		return catalogdomain.ProjectSOPBinding{}, catalogdomain.SOPVersion{}, err
	}
	sops, err := s.catalog.SOPs(ctx, actor.TenantID)
	if err != nil {
		return catalogdomain.ProjectSOPBinding{}, catalogdomain.SOPVersion{}, err
	}
	desiredSOP, found := latestPublishedBuiltinSOP(sops, builtinSOPKeyForContentType(projectContentType))
	if !found {
		desiredSOP = defaultSOP
	}
	if configuredSOP, configuredErr := s.publishedSOPVersion(ctx, actor.TenantID, environment.DefaultSOPID, environment.DefaultSOPVersion); configuredErr == nil && containsString(configuredSOP.ContentTypes, projectContentType) {
		desiredSOP = configuredSOP
	}
	binding, err := s.catalog.ProjectSOPBinding(ctx, actor.TenantID, projectID)
	if err == nil {
		summary, summaryErr := s.catalog.SOP(ctx, actor.TenantID, binding.SOPID)
		if summaryErr != nil {
			return binding, catalogdomain.SOPVersion{}, summaryErr
		}
		for _, version := range summary.Versions {
			if version.Version == binding.SOPVersion {
				if desiredSOP.SOPID == "" || binding.SOPID == desiredSOP.SOPID || containsString(version.ContentTypes, projectContentType) {
					return binding, version, nil
				}
				binding.EnvironmentID = defaultString(binding.EnvironmentID, environment.ID)
				binding.SOPID = desiredSOP.SOPID
				binding.SOPVersion = desiredSOP.Version
				binding.SOPDigest = desiredSOP.Digest
				binding.BoundBy = actor.UserID
				binding.BoundAt = s.now().UTC()
				if err := s.catalog.SaveProjectSOPBinding(ctx, binding); err != nil {
					return binding, catalogdomain.SOPVersion{}, err
				}
				return binding, desiredSOP, nil
			}
		}
		return binding, catalogdomain.SOPVersion{}, fault.NotFound("流程规范版本")
	}
	if !fault.IsNotFound(err) {
		return catalogdomain.ProjectSOPBinding{}, catalogdomain.SOPVersion{}, err
	}
	binding = catalogdomain.ProjectSOPBinding{TenantID: actor.TenantID, ProjectID: projectID, EnvironmentID: environment.ID, SOPID: desiredSOP.SOPID, SOPVersion: desiredSOP.Version, SOPDigest: desiredSOP.Digest, BoundBy: actor.UserID, BoundAt: s.now().UTC()}
	if err := s.catalog.SaveProjectSOPBinding(ctx, binding); err != nil {
		return binding, catalogdomain.SOPVersion{}, err
	}
	return binding, desiredSOP, nil
}

func latestPublishedBuiltinSOP(sops []catalogdomain.SOPSummary, templateKey string) (catalogdomain.SOPVersion, bool) {
	var result catalogdomain.SOPVersion
	for _, summary := range sops {
		if summary.Definition.TemplateKey != templateKey {
			continue
		}
		for _, version := range summary.Versions {
			if version.Status == "published" && (result.ID == "" || version.Version > result.Version) {
				result = version
			}
		}
	}
	return result, result.ID != ""
}

// BindProjectSOP explicitly selects the SOP configured by an Environment for
// future work in a Project. Existing Tasks keep their own immutable pin.
func (s *WorkService) BindProjectSOP(ctx context.Context, actor Actor, projectID string, input BindProjectSOPInput, requestID string) (ProjectSOPBindingResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return ProjectSOPBindingResult{}, err
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return ProjectSOPBindingResult{}, err
	}
	environmentID := strings.TrimSpace(input.EnvironmentID)
	if environmentID == "" {
		return ProjectSOPBindingResult{}, fault.Invalid("PROJECT_SOP_ENVIRONMENT_REQUIRED", "项目流程规范绑定必须指定执行环境")
	}
	environment, err := s.catalog.Environment(ctx, actor.TenantID, environmentID)
	if err != nil {
		return ProjectSOPBindingResult{}, err
	}
	if environment.DefaultSOPID == "" || environment.DefaultSOPVersion < 1 {
		return ProjectSOPBindingResult{}, fault.Policy("ENVIRONMENT_SOP_REQUIRED", "执行环境尚未配置已发布的流程规范", "先在管理后台为执行环境配置默认流程规范")
	}
	sopID := strings.TrimSpace(input.SOPID)
	version := input.SOPVersion
	if sopID == "" {
		sopID = environment.DefaultSOPID
	}
	if version < 1 {
		version = environment.DefaultSOPVersion
	}
	if sopID != environment.DefaultSOPID || version != environment.DefaultSOPVersion {
		return ProjectSOPBindingResult{}, fault.Policy("PROJECT_SOP_NOT_ALLOWED", "项目只能绑定当前执行环境配置的流程规范版本", "先切换执行环境的默认流程规范，再绑定项目")
	}
	sop, err := s.publishedSOPVersion(ctx, actor.TenantID, sopID, version)
	if err != nil {
		return ProjectSOPBindingResult{}, err
	}
	projectContentType := strings.TrimSpace(input.ContentType)
	if projectContentType == "" && containsString(sop.ContentTypes, project.ContentType) {
		projectContentType = project.ContentType
	}
	if projectContentType == "" && len(sop.ContentTypes) == 1 {
		projectContentType = sop.ContentTypes[0]
	}
	if !identitydomain.ValidTenantContentType(projectContentType) || !containsString(sop.ContentTypes, projectContentType) {
		return ProjectSOPBindingResult{}, fault.Policy("PROJECT_SOP_CONTENT_TYPE_REQUIRED", "绑定流程规范时必须明确适用于项目的内容类型", "选择流程规范支持的内容类型后重试")
	}
	if project.ContentType != projectContentType {
		project.ContentType = projectContentType
		project.RowVersion++
		project.UpdatedAt = s.now().UTC()
		if err := s.workspace.UpdateProject(ctx, project, project.RowVersion-1); err != nil {
			return ProjectSOPBindingResult{}, err
		}
	}
	previous, previousErr := s.catalog.ProjectSOPBinding(ctx, actor.TenantID, projectID)
	if previousErr != nil && !fault.IsNotFound(previousErr) {
		return ProjectSOPBindingResult{}, previousErr
	}
	now := s.now().UTC()
	binding := catalogdomain.ProjectSOPBinding{TenantID: actor.TenantID, ProjectID: projectID, EnvironmentID: environment.ID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, BoundBy: actor.UserID, BoundAt: now}
	if err := s.catalog.SaveProjectSOPBinding(ctx, binding); err != nil {
		return ProjectSOPBindingResult{}, err
	}
	meta := map[string]any{"environment_id": environment.ID, "sop_id": sop.SOPID, "sop_version": sop.Version, "sop_digest": sop.Digest}
	if previousErr == nil {
		meta["previous_environment_id"] = previous.EnvironmentID
		meta["previous_sop_id"] = previous.SOPID
		meta["previous_sop_version"] = previous.SOPVersion
	}
	s.audit(ctx, actor, projectID, "project.sop_bound", "project_sop_binding", projectID, requestID, meta)
	result := ProjectSOPBindingResult{Binding: binding, SOP: sop}
	if previousErr == nil {
		result.Previous = &previous
	}
	return result, nil
}

func (s *WorkService) publishedSOPVersion(ctx context.Context, tenantID, sopID string, version int) (catalogdomain.SOPVersion, error) {
	summary, err := s.catalog.SOP(ctx, tenantID, sopID)
	if err != nil {
		return catalogdomain.SOPVersion{}, err
	}
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			if candidate.Status != "published" {
				return catalogdomain.SOPVersion{}, fault.Policy("SOP_NOT_PUBLISHED", "只能绑定已发布的流程规范版本", "先发布该流程规范版本，再绑定执行环境或项目")
			}
			return candidate, nil
		}
	}
	return catalogdomain.SOPVersion{}, fault.NotFound("流程规范版本")
}

func (s *WorkService) CreateWorkTask(ctx context.Context, actor Actor, input CreateWorkTaskInput, requestID string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return WorkTaskView{}, err
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, input.ProjectID)
	if err != nil {
		return WorkTaskView{}, err
	}
	binding, boundSOP, err := s.ProjectSOP(ctx, actor, project.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	environment, err := s.catalog.Environment(ctx, actor.TenantID, binding.EnvironmentID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if input.EnvironmentID != "" && input.EnvironmentID != environment.ID {
		environment, err = s.catalog.Environment(ctx, actor.TenantID, input.EnvironmentID)
		if err != nil {
			return WorkTaskView{}, err
		}
	}
	if environment.Status != "active" {
		return WorkTaskView{}, fault.Policy("ENVIRONMENT_PAUSED", "当前执行环境已暂停，不能创建新任务", "在管理后台恢复执行环境后重试")
	}
	// An explicit Environment override selects that Environment's configured
	// SOP. A Project binding may be used only when it belongs to the selected
	// Environment; arbitrary tenant SOPs cannot bypass Environment policy.
	environmentSOP, envErr := s.publishedSOPVersion(ctx, actor.TenantID, environment.DefaultSOPID, environment.DefaultSOPVersion)
	if envErr != nil {
		return WorkTaskView{}, fault.Policy("ENVIRONMENT_SOP_INVALID", "执行环境的默认流程规范不可用于创建任务", "先在管理后台绑定已发布的流程规范")
	}
	sop := environmentSOP
	if environment.ID == binding.EnvironmentID && input.EnvironmentID == "" {
		sop = boundSOP
	}
	if input.SOPID != "" || input.SOPVersion > 0 {
		requestedID := input.SOPID
		if requestedID == "" {
			requestedID = sop.SOPID
		}
		requestedVersion := input.SOPVersion
		if requestedVersion < 1 {
			requestedVersion = sop.Version
		}
		allowedBound := environment.ID == binding.EnvironmentID && requestedID == binding.SOPID && requestedVersion == binding.SOPVersion
		allowedEnvironment := requestedID == environment.DefaultSOPID && requestedVersion == environment.DefaultSOPVersion
		if !allowedBound && !allowedEnvironment {
			return WorkTaskView{}, fault.Policy("TASK_SOP_NOT_ALLOWED", "任务只能使用项目绑定或执行环境配置的流程规范版本", "先在管理后台调整项目或执行环境的流程规范配置")
		}
		sop, err = s.publishedSOPVersion(ctx, actor.TenantID, requestedID, requestedVersion)
		if err != nil {
			return WorkTaskView{}, err
		}
	}
	if sop.Status != "published" {
		return WorkTaskView{}, fault.Policy("SOP_NOT_PUBLISHED", "只能使用已发布的流程规范创建任务", "先在管理后台发布流程规范版本")
	}
	now := s.now().UTC()
	status := "ready"
	nextAction := "开始第一个流程阶段"
	if len(input.InputRefs) == 0 {
		status = "needs_input"
		nextAction = "补充任务输入"
	}
	stageID := ""
	if len(sop.Stages) > 0 {
		stageID = sop.Stages[0].ID
	}
	contentType := defaultString(input.ContentType, defaultString(project.ContentType, identitydomain.DefaultProjectContentType))
	if !identitydomain.ValidTenantContentType(contentType) {
		return WorkTaskView{}, fault.Invalid("TASK_CONTENT_TYPE_INVALID", "任务内容类型不受支持")
	}
	if project.ContentType != "" && contentType != project.ContentType {
		return WorkTaskView{}, fault.Policy("TASK_CONTENT_TYPE_NOT_IN_PROJECT", "任务内容类型必须与项目生产类型一致", "在对应内容类型的项目中创建任务")
	}
	if contentType == identitydomain.ContentTypeMarketingVideo {
		capabilities, capabilityErr := s.identity.TenantContentCapabilities(ctx, actor.TenantID)
		if capabilityErr != nil {
			return WorkTaskView{}, capabilityErr
		}
		enabled := false
		for _, capability := range capabilities {
			if capability.ContentType == contentType && capability.Enabled {
				enabled = true
				break
			}
		}
		if !enabled {
			return WorkTaskView{}, fault.Policy("MARKETING_VIDEO_CAPABILITY_DISABLED", "当前租户未启用营销视频全流程能力", "由平台管理员启用营销视频（marketing_video）内容能力")
		}
	}
	contentTypeAllowed := false
	for _, candidate := range sop.ContentTypes {
		if candidate == contentType {
			contentTypeAllowed = true
			break
		}
	}
	if len(sop.ContentTypes) > 0 && !contentTypeAllowed {
		return WorkTaskView{}, fault.Policy("TASK_CONTENT_TYPE_NOT_IN_SOP", "当前流程规范未启用该内容类型", "在流程规范编辑器中启用内容类型后重试")
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if len(idempotencyKey) > 128 {
		return WorkTaskView{}, fault.Invalid("IDEMPOTENCY_KEY_INVALID", "幂等键（Idempotency-Key）不能超过 128 个字符")
	}
	task := work.WorkTask{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: project.ID, EnvironmentID: environment.ID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, Title: strings.TrimSpace(input.Title), Intent: input.Intent, ContentType: contentType, InputRefs: input.InputRefs, RequestedOutput: input.RequestedOutput, AssigneeUserID: input.AssigneeUserID, Priority: defaultString(input.Priority, "normal"), DueAt: input.DueAt, RiskProfile: defaultString(input.RiskProfile, "low"), IdempotencyKey: idempotencyKey, Status: status, CurrentStageID: stageID, NextAction: nextAction, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := task.Validate(); err != nil {
		return WorkTaskView{}, err
	}
	if idempotencyKey != "" {
		if existing, lookupErr := s.tasks.WorkTaskByIdempotencyKey(ctx, actor.TenantID, idempotencyKey); lookupErr == nil {
			if !sameTaskCreateRequest(existing, task) {
				return WorkTaskView{}, fault.Conflict("IDEMPOTENCY_KEY_REUSE", "相同的幂等键（Idempotency-Key）已用于不同任务参数")
			}
			return s.WorkTask(ctx, actor, existing.ID)
		} else if !fault.IsNotFound(lookupErr) {
			return WorkTaskView{}, lookupErr
		}
	}
	if err := s.tasks.CreateWorkTask(ctx, task); err != nil {
		if idempotencyKey != "" {
			if existing, lookupErr := s.tasks.WorkTaskByIdempotencyKey(ctx, actor.TenantID, idempotencyKey); lookupErr == nil {
				if !sameTaskCreateRequest(existing, task) {
					return WorkTaskView{}, fault.Conflict("IDEMPOTENCY_KEY_REUSE", "相同的幂等键（Idempotency-Key）已用于不同任务参数")
				}
				return s.WorkTask(ctx, actor, existing.ID)
			}
		}
		return WorkTaskView{}, err
	}
	if stageID != "" {
		stage := sop.Stages[0]
		_ = s.tasks.CreateStageRun(ctx, work.StageRun{ID: idgen.New(), TenantID: actor.TenantID, TaskID: task.ID, StageID: stage.ID, Status: "pending", ExecutionMode: sop.DefaultExecutionMode, InputRefs: append([]string(nil), stage.InputRefs...), UpdatedAt: now})
	}
	s.audit(ctx, actor, project.ID, "task.created", "task", task.ID, requestID, map[string]any{"sop_id": task.SOPID, "sop_version": task.SOPVersion, "sop_digest": task.SOPDigest})
	return s.WorkTask(ctx, actor, task.ID)
}

func sameTaskCreateRequest(existing, candidate work.WorkTask) bool {
	return existing.TenantID == candidate.TenantID && existing.ProjectID == candidate.ProjectID && existing.EnvironmentID == candidate.EnvironmentID && existing.SOPID == candidate.SOPID && existing.SOPVersion == candidate.SOPVersion && existing.SOPDigest == candidate.SOPDigest && existing.Title == candidate.Title && existing.Intent == candidate.Intent && existing.ContentType == candidate.ContentType && reflect.DeepEqual(existing.InputRefs, candidate.InputRefs) && reflect.DeepEqual(existing.RequestedOutput, candidate.RequestedOutput) && existing.AssigneeUserID == candidate.AssigneeUserID && existing.Priority == candidate.Priority && sameTime(existing.DueAt, candidate.DueAt) && existing.RiskProfile == candidate.RiskProfile
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func (s *WorkService) WorkTasks(ctx context.Context, actor Actor, projectID string) ([]work.WorkTask, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	return s.tasks.WorkTasks(ctx, actor.TenantID, projectID)
}

func (s *WorkService) WorkTask(ctx context.Context, actor Actor, id string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return WorkTaskView{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, task.ProjectID)
	if err != nil {
		return WorkTaskView{}, err
	}
	environment, err := s.catalog.Environment(ctx, actor.TenantID, task.EnvironmentID)
	if err != nil {
		return WorkTaskView{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, task.SOPID)
	if err != nil {
		return WorkTaskView{}, err
	}
	var sop catalogdomain.SOPVersion
	for _, candidate := range summary.Versions {
		if candidate.Version == task.SOPVersion {
			sop = candidate
			break
		}
	}
	if sop.ID == "" {
		return WorkTaskView{}, fault.NotFound("流程规范版本")
	}
	stageRuns, err := s.tasks.StageRuns(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	runs, err := s.app.Runtime.runtimeRunsForWorkTask(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	gates, err := s.delivery.GateEvaluations(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	revisions, err := s.taskRevisions(ctx, task)
	if err != nil {
		return WorkTaskView{}, err
	}
	deliveries, err := s.delivery.TaskDeliveries(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	actions := []string{}
	if task.Status == work.TaskStatusNeedsInput {
		actions = append(actions, "add_input")
	} else if task.Status == work.TaskStatusReady {
		actions = append(actions, "claim", "start", "cancel")
	} else if task.Status == work.TaskStatusRunning {
		actions = append(actions, "pause", "cancel")
	} else if task.Status == work.TaskStatusPaused {
		actions = append(actions, "resume", "retry", "cancel")
	} else if task.Status == work.TaskStatusWaitingGate {
		actions = append(actions, "decide", "cancel")
	} else if task.Status == work.TaskStatusBlocked {
		actions = append(actions, "retry", "cancel")
	} else if task.Status == work.TaskStatusAccepted {
		if task.ContentType == identitydomain.ContentTypeMarketingVideo {
			actions = append(actions, "deliver")
		} else {
			actions = append(actions, "submit_revision", "deliver")
		}
	}
	stageOutputs, err := s.tasks.TaskStageOutputs(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	mediaJobs, err := s.delivery.MediaGenerationJobs(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	attempts := []deliverydomain.ProviderAttempt{}
	for _, job := range mediaJobs {
		values, attemptErr := s.delivery.ProviderAttempts(ctx, actor.TenantID, job.ID)
		if attemptErr != nil {
			return WorkTaskView{}, attemptErr
		}
		attempts = append(attempts, values...)
	}
	mediaReviews, err := s.delivery.MediaReviews(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	packages, err := s.artifacts.DeliveryPackages(ctx, actor.TenantID, task.ProjectID)
	if err != nil {
		return WorkTaskView{}, err
	}
	sourceRevisions := []sourcedomain.SourceRevision{}
	knowledgeSnapshots := []sourcedomain.KnowledgeSnapshot{}
	approvedSnapshots := []reviewdomain.ApprovedSnapshot{}
	sourceSeen := map[string]bool{}
	knowledgeSeen := map[string]bool{}
	snapshotSeen := map[string]bool{}
	for _, output := range stageOutputs {
		switch output.OutputType {
		case catalogdomain.StageOutputSourceRevision:
			if sourceSeen[output.ObjectID] {
				continue
			}
			value, loadErr := s.source.SourceRevision(ctx, actor.TenantID, output.ObjectID)
			if loadErr != nil {
				return WorkTaskView{}, loadErr
			}
			sourceSeen[value.ID] = true
			sourceRevisions = append(sourceRevisions, value)
		case catalogdomain.StageOutputKnowledgeSnapshot:
			if knowledgeSeen[output.ObjectID] {
				continue
			}
			value, loadErr := s.knowledge.KnowledgeSnapshot(ctx, actor.TenantID, output.ObjectID)
			if loadErr != nil {
				return WorkTaskView{}, loadErr
			}
			knowledgeSeen[value.ID] = true
			knowledgeSnapshots = append(knowledgeSnapshots, value)
		case catalogdomain.StageOutputApprovedSnapshot, catalogdomain.StageOutputStoryboardPackage:
			if snapshotSeen[output.ObjectID] {
				continue
			}
			value, loadErr := s.review.ApprovedSnapshot(ctx, actor.TenantID, output.ObjectID)
			if loadErr != nil {
				return WorkTaskView{}, loadErr
			}
			snapshotSeen[value.ID] = true
			approvedSnapshots = append(approvedSnapshots, value)
		}
	}
	taskPackages := []deliverydomain.DeliveryPackage{}
	artifacts := []deliverydomain.Artifact{}
	artifactSeen := map[string]bool{}
	for _, snapshot := range approvedSnapshots {
		values, artifactErr := s.artifacts.ArtifactsByApprovedSnapshot(ctx, actor.TenantID, snapshot.ID)
		if artifactErr != nil {
			return WorkTaskView{}, artifactErr
		}
		for _, artifact := range values {
			if artifactSeen[artifact.ID] {
				continue
			}
			artifactSeen[artifact.ID] = true
			artifacts = append(artifacts, artifact)
		}
	}
	for _, value := range packages {
		if value.ContentItemID != task.ID {
			continue
		}
		taskPackages = append(taskPackages, value)
		for _, artifact := range value.Manifest {
			if !artifactSeen[artifact.ID] {
				artifactSeen[artifact.ID] = true
				artifacts = append(artifacts, artifact)
			}
		}
	}
	for _, review := range mediaReviews {
		if artifactSeen[review.SubjectArtifactID] {
			continue
		}
		artifact, artifactErr := s.artifacts.Artifact(ctx, actor.TenantID, review.SubjectArtifactID)
		if artifactErr == nil {
			artifactSeen[artifact.ID] = true
			artifacts = append(artifacts, artifact)
		} else if !fault.IsNotFound(artifactErr) {
			return WorkTaskView{}, artifactErr
		}
	}
	return WorkTaskView{Task: task, Project: project, Environment: environment, SOP: sop, SourceRevisions: sourceRevisions, KnowledgeSnapshots: knowledgeSnapshots, ApprovedSnapshots: approvedSnapshots, StageRuns: stageRuns, Runs: runs, Gates: gates, Revisions: revisions, Deliveries: deliveries, StageOutputs: stageOutputs, MediaJobs: mediaJobs, ProviderAttempts: attempts, MediaReviews: mediaReviews, DeliveryPackages: taskPackages, Artifacts: artifacts, AllowedActions: actions, GeneratedAt: s.now().UTC()}, nil
}
