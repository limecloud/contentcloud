package app

import (
	"context"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type SaveEnvironmentInput struct {
	Name              string                         `json:"name"`
	Slug              string                         `json:"slug"`
	Status            string                         `json:"status"`
	DefaultSOPID      string                         `json:"default_sop_id"`
	DefaultSOPVersion int                            `json:"default_sop_version"`
	Capabilities      []domain.EnvironmentCapability `json:"capabilities"`
}

type BindProjectSOPInput struct {
	EnvironmentID string `json:"environment_id"`
	SOPID         string `json:"sop_id"`
	SOPVersion    int    `json:"sop_version"`
}

type ProjectSOPBindingResult struct {
	Binding  domain.ProjectSOPBinding  `json:"binding"`
	SOP      domain.SOPVersion         `json:"sop"`
	Previous *domain.ProjectSOPBinding `json:"previous,omitempty"`
}

type CreateSOPInput struct {
	Name                 string                   `json:"name"`
	Description          string                   `json:"description"`
	ContentTypes         []string                 `json:"content_types"`
	Stages               []domain.StageDefinition `json:"stages"`
	Gates                []domain.GateDefinition  `json:"gates"`
	DefaultExecutionMode string                   `json:"default_execution_mode"`
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
	Version              int                      `json:"version"`
	Name                 string                   `json:"name"`
	Description          string                   `json:"description"`
	ContentTypes         []string                 `json:"content_types"`
	Stages               []domain.StageDefinition `json:"stages"`
	Gates                []domain.GateDefinition  `json:"gates"`
	DefaultExecutionMode string                   `json:"default_execution_mode"`
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
	TaskID       string `json:"task_id"`
	ProjectID    string `json:"project_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	SOPVersion   int    `json:"sop_version"`
	TaskRunBound bool   `json:"task_run_bound"`
}

type SOPVersionImpact struct {
	SOPID        string                 `json:"sop_id"`
	Version      int                    `json:"version"`
	Environments []SOPEnvironmentImpact `json:"environments"`
	Projects     []SOPProjectImpact     `json:"projects"`
	Tasks        []SOPTaskImpact        `json:"tasks"`
	Counts       map[string]int         `json:"counts"`
}

type SOPRollbackResult struct {
	Version             domain.SOPVersion `json:"version"`
	TargetVersion       int               `json:"target_version"`
	PreviousVersion     int               `json:"previous_version"`
	ReboundEnvironments int               `json:"rebound_environments"`
	ReboundProjects     int               `json:"rebound_projects"`
	Impact              SOPVersionImpact  `json:"impact"`
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
	Task           domain.WorkTask         `json:"task"`
	Project        domain.Project          `json:"project"`
	Environment    domain.Environment      `json:"environment"`
	SOP            domain.SOPVersion       `json:"sop"`
	StageRuns      []domain.StageRun       `json:"stage_runs"`
	Runs           []domain.TaskRun        `json:"runs"`
	Gates          []domain.GateEvaluation `json:"gates"`
	Revisions      []domain.TaskRevision   `json:"revisions"`
	Deliveries     []domain.TaskDelivery   `json:"deliveries"`
	AllowedActions []string                `json:"allowed_actions"`
	GeneratedAt    time.Time               `json:"generated_at"`
}

func (s *Service) ensureOrchestrationDefaults(ctx context.Context, actor Actor) (domain.Environment, domain.SOPVersion, error) {
	environments, err := s.store.Environments(ctx, actor.TenantID)
	if err != nil {
		return domain.Environment{}, domain.SOPVersion{}, err
	}
	sops, err := s.store.SOPs(ctx, actor.TenantID)
	if err != nil {
		return domain.Environment{}, domain.SOPVersion{}, err
	}
	sops, err = s.ensureBuiltinSOPs(ctx, actor, sops)
	if err != nil {
		return domain.Environment{}, domain.SOPVersion{}, err
	}
	var sop domain.SOPVersion
	for _, summary := range sops {
		for _, candidate := range summary.Versions {
			if candidate.Status == "published" && summary.Definition.TemplateKey == builtinSOPShortVideo && (sop.ID == "" || candidate.Version > sop.Version) {
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
		environment := domain.Environment{ID: domain.NewID(), TenantID: actor.TenantID, Name: "内容生产默认环境", Slug: "production", Status: "active", DefaultSOPID: sop.SOPID, DefaultSOPVersion: sop.Version, ManifestDigest: "sha256:" + strings.Repeat("0", 64), Capabilities: []domain.EnvironmentCapability{{ID: "content.script.compose", Version: "1.0.0", Enabled: true}, {ID: "content.article.compose", Version: "1.0.0", Enabled: true}}, CreatedAt: now, UpdatedAt: now}
		if err := s.store.CreateEnvironment(ctx, environment); err != nil {
			return domain.Environment{}, domain.SOPVersion{}, err
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
		if summary, summaryErr := s.store.SOP(ctx, actor.TenantID, environment.DefaultSOPID); summaryErr == nil {
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
		if err := s.store.SaveEnvironment(ctx, environment); err != nil {
			return domain.Environment{}, domain.SOPVersion{}, err
		}
	}
	return environment, sop, nil
}

func (s *Service) AdminWorkOS(ctx context.Context, actor Actor) (domain.AdminWorkOSView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.AdminWorkOSView{}, err
	}
	_, _, err := s.ensureOrchestrationDefaults(ctx, actor)
	if err != nil {
		return domain.AdminWorkOSView{}, err
	}
	environments, err := s.store.Environments(ctx, actor.TenantID)
	if err != nil {
		return domain.AdminWorkOSView{}, err
	}
	sops, err := s.store.SOPs(ctx, actor.TenantID)
	if err != nil {
		return domain.AdminWorkOSView{}, err
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
	gates := []domain.GateSummary{}
	for _, summary := range sops {
		for _, version := range summary.Versions {
			if version.Status != "published" && version.Status != "draft" {
				continue
			}
			for _, gate := range version.Gates {
				gates = append(gates, domain.GateSummary{SOPID: summary.Definition.ID, SOPName: summary.Definition.Name, SOPVersion: version.Version, ID: gate.ID, Name: gate.Name, Mode: gate.Mode, Blocking: gate.Blocking})
			}
		}
	}
	audit, err := s.store.AuditEvents(ctx, actor.TenantID, "", 30)
	if err != nil {
		return domain.AdminWorkOSView{}, err
	}
	tasks, err := s.store.WorkTasks(ctx, actor.TenantID, "")
	if err != nil {
		return domain.AdminWorkOSView{}, err
	}
	for index := range gates {
		for _, task := range tasks {
			if task.SOPID == gates[index].SOPID && task.SOPVersion == gates[index].SOPVersion {
				gates[index].UsageCount++
			}
		}
	}
	devices, err := s.store.Devices(ctx, actor.TenantID, "")
	if err != nil {
		return domain.AdminWorkOSView{}, err
	}
	usage := domain.UsageSummary{TaskCount: len(tasks), ByExecutionMode: map[string]int{}}
	capabilityByKey := map[string]domain.Capability{}
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
		runs, runErr := s.store.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return domain.AdminWorkOSView{}, runErr
		}
		for _, run := range runs {
			mode := defaultString(run.ExecutionMode, "unspecified")
			usage.ByExecutionMode[mode]++
		}
	}
	capabilities := make([]domain.Capability, 0, len(capabilityByKey))
	for _, capability := range capabilityByKey {
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool {
		if capabilities[i].ID == capabilities[j].ID {
			return capabilities[i].Version < capabilities[j].Version
		}
		return capabilities[i].ID < capabilities[j].ID
	})
	return domain.AdminWorkOSView{Environments: environments, SOPs: sops, Gates: gates, Capabilities: capabilities, Audit: audit, Usage: usage, GeneratedAt: s.now().UTC()}, nil
}

func (s *Service) SaveEnvironment(ctx context.Context, actor Actor, id string, input SaveEnvironmentInput, requestID string) (domain.Environment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.Environment{}, err
	}
	value, err := s.store.Environment(ctx, actor.TenantID, id)
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
			return value, domain.Invalid("ENVIRONMENT_STATUS_INVALID", "Environment 状态只能是 active 或 paused")
		}
		value.Status = input.Status
	}
	if input.DefaultSOPID != "" {
		summary, summaryErr := s.store.SOP(ctx, actor.TenantID, input.DefaultSOPID)
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
			return value, domain.Policy("DEFAULT_SOP_NOT_PUBLISHED", "Environment 默认 SOP 必须是当前租户已发布版本", "先发布该 SOP 版本，再设为默认")
		}
		value.DefaultSOPID = input.DefaultSOPID
		value.DefaultSOPVersion = input.DefaultSOPVersion
	}
	if value.DefaultSOPID == "" || value.DefaultSOPVersion < 1 {
		return value, domain.Policy("ENVIRONMENT_DEFAULT_SOP_REQUIRED", "Environment 必须绑定已发布 SOP 版本", "先选择一条已发布 SOP，再保存 Environment")
	}
	if input.Capabilities != nil {
		value.Capabilities = input.Capabilities
	}
	value.UpdatedAt = s.now().UTC()
	if err := s.store.SaveEnvironment(ctx, value); err != nil {
		return value, err
	}
	s.audit(ctx, actor, "", "environment.updated", "environment", value.ID, requestID, map[string]any{"status": value.Status, "default_sop_id": value.DefaultSOPID, "default_sop_version": value.DefaultSOPVersion})
	return value, nil
}

func (s *Service) CreateEnvironment(ctx context.Context, actor Actor, input SaveEnvironmentInput, requestID string) (domain.Environment, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.Environment{}, err
	}
	name := strings.TrimSpace(input.Name)
	slug := strings.TrimSpace(input.Slug)
	if name == "" || slug == "" {
		return domain.Environment{}, domain.Invalid("ENVIRONMENT_FIELDS_REQUIRED", "Environment 名称和标识不能为空")
	}
	value := domain.Environment{ID: domain.NewID(), TenantID: actor.TenantID, Name: name, Slug: slug, Status: defaultString(input.Status, "active"), Capabilities: append([]domain.EnvironmentCapability(nil), input.Capabilities...), CreatedAt: s.now().UTC(), UpdatedAt: s.now().UTC()}
	value.NormalizeCollections()
	if value.Status != "active" && value.Status != "paused" {
		return domain.Environment{}, domain.Invalid("ENVIRONMENT_STATUS_INVALID", "Environment 状态只能是 active 或 paused")
	}
	if input.DefaultSOPID == "" || input.DefaultSOPVersion < 1 {
		return domain.Environment{}, domain.Policy("ENVIRONMENT_DEFAULT_SOP_REQUIRED", "Environment 必须绑定已发布 SOP 版本", "先选择一条已发布 SOP，再创建 Environment")
	}
	if input.DefaultSOPID != "" {
		summary, err := s.store.SOP(ctx, actor.TenantID, input.DefaultSOPID)
		if err != nil {
			return domain.Environment{}, err
		}
		for _, candidate := range summary.Versions {
			if candidate.Version == input.DefaultSOPVersion && candidate.Status == "published" {
				value.DefaultSOPID = input.DefaultSOPID
				value.DefaultSOPVersion = input.DefaultSOPVersion
				break
			}
		}
		if value.DefaultSOPID == "" {
			return domain.Environment{}, domain.Policy("DEFAULT_SOP_NOT_PUBLISHED", "Environment 默认 SOP 必须是当前租户已发布版本", "先发布该 SOP 版本，再创建 Environment")
		}
	}
	if err := value.Validate(); err != nil {
		return domain.Environment{}, err
	}
	if err := s.store.CreateEnvironment(ctx, value); err != nil {
		return domain.Environment{}, err
	}
	s.audit(ctx, actor, "", "environment.created", "environment", value.ID, requestID, map[string]any{"slug": value.Slug, "status": value.Status, "default_sop_id": value.DefaultSOPID, "default_sop_version": value.DefaultSOPVersion})
	return value, nil
}

func (s *Service) CreateSOP(ctx context.Context, actor Actor, input CreateSOPInput, requestID string) (domain.SOPSummary, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.SOPSummary{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return domain.SOPSummary{}, domain.Invalid("SOP_NAME_REQUIRED", "SOP 名称不能为空")
	}
	contentTypes := append([]string(nil), input.ContentTypes...)
	if len(contentTypes) == 0 {
		contentTypes = []string{domain.ContentTypeVideoScript}
	}
	stages := append([]domain.StageDefinition(nil), input.Stages...)
	if len(stages) == 0 {
		stages = []domain.StageDefinition{{ID: "input", Name: "需求输入", Order: 10, OwnerRoles: []string{"project_manager", "strategist"}, OutputSchema: "contentcloud.brief/1.0", ExecutionModes: []string{"local"}, Checks: []string{"brief.required"}}}
	}
	now := s.now().UTC()
	sopID := domain.NewID()
	definition := domain.SOPDefinition{ID: sopID, TenantID: actor.TenantID, Name: name, Description: input.Description, ContentTypes: contentTypes, CurrentVersion: 0, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	version := domain.SOPVersion{ID: domain.NewID(), TenantID: actor.TenantID, SOPID: sopID, Version: 1, SchemaVersion: domain.SOPSchemaVersion, Name: name, Description: input.Description, ContentTypes: contentTypes, Stages: stages, Gates: append([]domain.GateDefinition(nil), input.Gates...), DefaultExecutionMode: defaultString(input.DefaultExecutionMode, "local"), Status: "draft", CreatedBy: actor.UserID, CreatedAt: now}
	definition.NormalizeCollections()
	version.NormalizeCollections()
	if err := version.Validate(); err != nil {
		return domain.SOPSummary{}, err
	}
	if err := s.store.CreateSOP(ctx, definition, version); err != nil {
		return domain.SOPSummary{}, err
	}
	s.audit(ctx, actor, "", "sop.created", "sop", sopID, requestID, map[string]any{"version": 1, "status": "draft"})
	return domain.SOPSummary{Definition: definition, Versions: []domain.SOPVersion{version}}, nil
}

func (s *Service) SaveSOPVersion(ctx context.Context, actor Actor, sopID string, version int, input SaveSOPVersionInput, requestID string) (domain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.SOPVersion{}, err
	}
	value, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return domain.SOPVersion{}, err
	}
	var current domain.SOPVersion
	for _, candidate := range value.Versions {
		if candidate.Version == version {
			current = candidate
			break
		}
	}
	if current.ID == "" {
		return current, domain.NotFound("SOP 版本")
	}
	if current.Status != "draft" {
		return current, domain.Conflict("SOP_VERSION_IMMUTABLE", "已发布 SOP 版本不可直接修改，请复制为新草稿")
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = input.Description
	current.ContentTypes = input.ContentTypes
	current.Stages = input.Stages
	current.Gates = input.Gates
	current.DefaultExecutionMode = input.DefaultExecutionMode
	current.Digest = ""
	current.NormalizeCollections()
	if err := s.store.SaveSOPVersion(ctx, current); err != nil {
		return current, err
	}
	s.audit(ctx, actor, "", "sop.version_saved", "sop_version", current.ID, requestID, map[string]any{"sop_id": sopID, "version": version})
	return current, nil
}

func (s *Service) SOPVersionDiff(ctx context.Context, actor Actor, sopID string, fromVersion, toVersion int) (SOPVersionDiff, error) {
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

func diffSOPVersions(from, to domain.SOPVersion) SOPVersionDiff {
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
	fromStages := map[string]domain.StageDefinition{}
	toStages := map[string]domain.StageDefinition{}
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
	fromGates := map[string]domain.GateDefinition{}
	toGates := map[string]domain.GateDefinition{}
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

func (s *Service) SOPVersionImpact(ctx context.Context, actor Actor, sopID string, version int) (SOPVersionImpact, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPVersionImpact{}, err
	}
	if _, err := s.sopVersion(ctx, actor.TenantID, sopID, version); err != nil {
		return SOPVersionImpact{}, err
	}
	environments, err := s.store.Environments(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	bindings, err := s.store.ProjectSOPBindings(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	projects, err := s.store.Projects(ctx, actor.TenantID)
	if err != nil {
		return SOPVersionImpact{}, err
	}
	tasks, err := s.store.WorkTasks(ctx, actor.TenantID, "")
	if err != nil {
		return SOPVersionImpact{}, err
	}
	result := SOPVersionImpact{SOPID: sopID, Version: version, Environments: []SOPEnvironmentImpact{}, Projects: []SOPProjectImpact{}, Tasks: []SOPTaskImpact{}, Counts: map[string]int{"environments": 0, "projects": 0, "tasks": 0, "active_tasks": 0}}
	for _, environment := range environments {
		if environment.DefaultSOPID == sopID && environment.DefaultSOPVersion == version {
			result.Environments = append(result.Environments, SOPEnvironmentImpact{EnvironmentID: environment.ID, Name: environment.Name, Status: environment.Status, DefaultSOPID: environment.DefaultSOPID, DefaultSOPVersion: environment.DefaultSOPVersion})
		}
	}
	projectByID := map[string]domain.Project{}
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
		result.Tasks = append(result.Tasks, SOPTaskImpact{TaskID: task.ID, ProjectID: task.ProjectID, Title: task.Title, Status: task.Status, SOPVersion: task.SOPVersion, TaskRunBound: task.Status == domain.TaskStatusRunning || task.Status == domain.TaskStatusWaitingGate || task.Status == domain.TaskStatusAccepted || task.Status == domain.TaskStatusDelivered})
	}
	result.Counts["environments"] = len(result.Environments)
	result.Counts["projects"] = len(result.Projects)
	result.Counts["tasks"] = len(result.Tasks)
	for _, task := range result.Tasks {
		if task.Status == domain.TaskStatusRunning || task.Status == domain.TaskStatusWaitingGate {
			result.Counts["active_tasks"]++
		}
	}
	return result, nil
}

func (s *Service) RollbackSOPVersion(ctx context.Context, actor Actor, sopID string, targetVersion int, requestID string) (SOPRollbackResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPRollbackResult{}, err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	target, err := s.sopVersionFromSummary(summary, targetVersion)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	if target.Status != "published" && target.Status != "retired" {
		return SOPRollbackResult{}, domain.Policy("SOP_ROLLBACK_SOURCE_INVALID", "只能从已发布或已退休版本回滚", "选择一个有 digest 的历史版本")
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
		return SOPRollbackResult{}, domain.Policy("SOP_ROLLBACK_NOOP", "目标版本已经是当前版本", "选择更早的已发布版本")
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
	environments, err := s.store.Environments(ctx, actor.TenantID)
	if err != nil {
		return SOPRollbackResult{}, err
	}
	for _, environment := range environments {
		if environment.DefaultSOPID != sopID || environment.DefaultSOPVersion != previousVersion {
			continue
		}
		environment.DefaultSOPVersion = published.Version
		environment.UpdatedAt = now
		if err := s.store.SaveEnvironment(ctx, environment); err != nil {
			return SOPRollbackResult{}, err
		}
		reboundEnvironments++
	}
	reboundProjects := 0
	bindings, err := s.store.ProjectSOPBindings(ctx, actor.TenantID)
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
		if err := s.store.SaveProjectSOPBinding(ctx, binding); err != nil {
			return SOPRollbackResult{}, err
		}
		reboundProjects++
	}
	s.audit(ctx, actor, "", "sop.version_rolled_back", "sop", sopID, requestID, map[string]any{"target_version": targetVersion, "previous_version": previousVersion, "published_version": published.Version, "rebound_environments": reboundEnvironments, "rebound_projects": reboundProjects, "active_tasks": impact.Counts["active_tasks"]})
	return SOPRollbackResult{Version: published, TargetVersion: targetVersion, PreviousVersion: previousVersion, ReboundEnvironments: reboundEnvironments, ReboundProjects: reboundProjects, Impact: impact}, nil
}

func (s *Service) sopVersion(ctx context.Context, tenantID, sopID string, version int) (domain.SOPVersion, error) {
	summary, err := s.store.SOP(ctx, tenantID, sopID)
	if err != nil {
		return domain.SOPVersion{}, err
	}
	return s.sopVersionFromSummary(summary, version)
}

func (s *Service) sopVersionFromSummary(summary domain.SOPSummary, version int) (domain.SOPVersion, error) {
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			return candidate, nil
		}
	}
	return domain.SOPVersion{}, domain.NotFound("SOP 版本")
}

func (s *Service) CreateSOPDraft(ctx context.Context, actor Actor, sopID string, sourceVersion int, requestID string) (domain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.SOPVersion{}, err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return domain.SOPVersion{}, err
	}
	var source domain.SOPVersion
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
		return domain.SOPVersion{}, domain.NotFound("SOP 源版本")
	}
	now := s.now().UTC()
	draft := source
	draft.ID = domain.NewID()
	draft.Version = maxVersion + 1
	draft.Status = "draft"
	draft.Digest = ""
	draft.CreatedBy = actor.UserID
	draft.PublishedBy = ""
	draft.PublishedAt = nil
	draft.CreatedAt = now
	draft.NormalizeCollections()
	if err := s.store.CreateSOPVersion(ctx, draft); err != nil {
		return domain.SOPVersion{}, err
	}
	s.audit(ctx, actor, "", "sop.version_created", "sop_version", draft.ID, requestID, map[string]any{"sop_id": sopID, "version": draft.Version, "source_version": source.Version})
	return draft, nil
}

func (s *Service) PublishSOP(ctx context.Context, actor Actor, sopID string, version int, requestID string) (domain.SOPVersion, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return domain.SOPVersion{}, err
	}
	report, err := s.LintSOPVersion(ctx, actor, sopID, version)
	if err != nil {
		return domain.SOPVersion{}, err
	}
	if !report.Valid {
		return domain.SOPVersion{}, domain.Policy("SOP_LINT_FAILED", "SOP 发布前检查未通过", "修正 Stage、Gate 和执行方式配置后重试")
	}
	value, err := s.store.PublishSOPVersion(ctx, actor.TenantID, sopID, version, actor.UserID, s.now().UTC())
	if err != nil {
		return value, err
	}
	s.audit(ctx, actor, "", "sop.version_published", "sop_version", value.ID, requestID, map[string]any{"sop_id": sopID, "version": version, "digest": value.Digest})
	return value, nil
}

func (s *Service) RetireSOPVersion(ctx context.Context, actor Actor, sopID string, version int, requestID string) error {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return err
	}
	value, err := s.sopVersionFromSummary(summary, version)
	if err != nil {
		return err
	}
	if value.Status != "published" {
		return domain.Conflict("SOP_VERSION_STATE_INVALID", "只有已发布 SOP 版本可以退休")
	}
	if err := s.store.RetireSOPVersion(ctx, actor.TenantID, sopID, version, s.now().UTC()); err != nil {
		return err
	}
	s.audit(ctx, actor, "", "sop.version_retired", "sop_version", value.ID, requestID, map[string]any{"sop_id": sopID, "version": version})
	return nil
}

func (s *Service) LintSOPVersion(ctx context.Context, actor Actor, sopID string, version int) (SOPLintReport, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return SOPLintReport{}, err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return SOPLintReport{}, err
	}
	var value domain.SOPVersion
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			value = candidate
			break
		}
	}
	if value.ID == "" {
		return SOPLintReport{}, domain.NotFound("SOP 版本")
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
		addError("stage.required", "stages", "SOP 至少需要一个 Stage")
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
				addError("stage.input_ref.invalid", path+".input_refs", "输入引用必须指向当前 Stage 之前的 Stage")
			}
		}
		if len(stage.ExecutionModes) == 0 {
			addError("stage.execution_mode.required", path+".execution_modes", "Stage 至少需要一种执行方式")
		}
		for _, gateID := range stage.GateIDs {
			usedGates[gateID] = true
		}
		seenStages[stage.ID] = true
	}
	for index, gate := range value.Gates {
		path := "gates[" + strconv.Itoa(index) + "]"
		humanGate := gate.Mode == domain.GateModeRequired || gate.Mode == domain.GateModeInternalReview || gate.Mode == domain.GateModeClientDecision
		if humanGate && !gate.Blocking {
			addError("gate.required_not_blocking", path+".blocking", "人工 Gate 必须阻断后续 Stage")
		}
		if !humanGate && gate.Mode != domain.GateModeRequiredCheck && gate.Blocking {
			addError("gate.optional_blocking", path+".blocking", "none、advisory 或检查 Gate 不能阻断后续 Stage")
		}
		if !usedGates[gate.ID] {
			addWarning("gate.unused", path, "Gate 尚未绑定到任何 Stage")
		}
		if humanGate && len(gate.AssigneeRoles) == 0 {
			addWarning("gate.assignee_roles.empty", path+".assignee_roles", "人工 Gate 建议配置决定角色")
		}
	}
	report.Valid = len(report.Errors) == 0
	return report, nil
}

func (s *Service) ProjectSOP(ctx context.Context, actor Actor, projectID string) (domain.ProjectSOPBinding, domain.SOPVersion, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return domain.ProjectSOPBinding{}, domain.SOPVersion{}, err
	}
	binding, err := s.store.ProjectSOPBinding(ctx, actor.TenantID, projectID)
	if err == nil {
		summary, summaryErr := s.store.SOP(ctx, actor.TenantID, binding.SOPID)
		if summaryErr != nil {
			return binding, domain.SOPVersion{}, summaryErr
		}
		for _, version := range summary.Versions {
			if version.Version == binding.SOPVersion {
				return binding, version, nil
			}
		}
		return binding, domain.SOPVersion{}, domain.NotFound("SOP 版本")
	}
	environment, sop, err := s.ensureOrchestrationDefaults(ctx, actor)
	if err != nil {
		return domain.ProjectSOPBinding{}, domain.SOPVersion{}, err
	}
	binding = domain.ProjectSOPBinding{TenantID: actor.TenantID, ProjectID: projectID, EnvironmentID: environment.ID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, BoundBy: actor.UserID, BoundAt: s.now().UTC()}
	if err := s.store.SaveProjectSOPBinding(ctx, binding); err != nil {
		return binding, domain.SOPVersion{}, err
	}
	return binding, sop, nil
}

// BindProjectSOP explicitly selects the SOP configured by an Environment for
// future work in a Project. Existing Tasks keep their own immutable pin.
func (s *Service) BindProjectSOP(ctx context.Context, actor Actor, projectID string, input BindProjectSOPInput, requestID string) (ProjectSOPBindingResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return ProjectSOPBindingResult{}, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return ProjectSOPBindingResult{}, err
	}
	environmentID := strings.TrimSpace(input.EnvironmentID)
	if environmentID == "" {
		return ProjectSOPBindingResult{}, domain.Invalid("PROJECT_SOP_ENVIRONMENT_REQUIRED", "Project SOP 绑定必须指定 Environment")
	}
	environment, err := s.store.Environment(ctx, actor.TenantID, environmentID)
	if err != nil {
		return ProjectSOPBindingResult{}, err
	}
	if environment.DefaultSOPID == "" || environment.DefaultSOPVersion < 1 {
		return ProjectSOPBindingResult{}, domain.Policy("ENVIRONMENT_SOP_REQUIRED", "Environment 尚未配置已发布 SOP", "先在管理后台为 Environment 配置默认 SOP")
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
		return ProjectSOPBindingResult{}, domain.Policy("PROJECT_SOP_NOT_ALLOWED", "Project 只能绑定当前 Environment 配置的 SOP 版本", "先切换 Environment 的默认 SOP，再绑定 Project")
	}
	sop, err := s.publishedSOPVersion(ctx, actor.TenantID, sopID, version)
	if err != nil {
		return ProjectSOPBindingResult{}, err
	}
	previous, previousErr := s.store.ProjectSOPBinding(ctx, actor.TenantID, projectID)
	if previousErr != nil && !domain.IsNotFound(previousErr) {
		return ProjectSOPBindingResult{}, previousErr
	}
	now := s.now().UTC()
	binding := domain.ProjectSOPBinding{TenantID: actor.TenantID, ProjectID: projectID, EnvironmentID: environment.ID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, BoundBy: actor.UserID, BoundAt: now}
	if err := s.store.SaveProjectSOPBinding(ctx, binding); err != nil {
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

func (s *Service) publishedSOPVersion(ctx context.Context, tenantID, sopID string, version int) (domain.SOPVersion, error) {
	summary, err := s.store.SOP(ctx, tenantID, sopID)
	if err != nil {
		return domain.SOPVersion{}, err
	}
	for _, candidate := range summary.Versions {
		if candidate.Version == version {
			if candidate.Status != "published" {
				return domain.SOPVersion{}, domain.Policy("SOP_NOT_PUBLISHED", "只能绑定已发布 SOP 版本", "先发布该 SOP 版本，再绑定 Environment 或 Project")
			}
			return candidate, nil
		}
	}
	return domain.SOPVersion{}, domain.NotFound("SOP 版本")
}

func (s *Service) CreateWorkTask(ctx context.Context, actor Actor, input CreateWorkTaskInput, requestID string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return WorkTaskView{}, err
	}
	project, err := s.store.Project(ctx, actor.TenantID, input.ProjectID)
	if err != nil {
		return WorkTaskView{}, err
	}
	binding, boundSOP, err := s.ProjectSOP(ctx, actor, project.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	environment, err := s.store.Environment(ctx, actor.TenantID, binding.EnvironmentID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if input.EnvironmentID != "" && input.EnvironmentID != environment.ID {
		environment, err = s.store.Environment(ctx, actor.TenantID, input.EnvironmentID)
		if err != nil {
			return WorkTaskView{}, err
		}
	}
	if environment.Status != "active" {
		return WorkTaskView{}, domain.Policy("ENVIRONMENT_PAUSED", "当前 Environment 已暂停，不能创建新任务", "在管理后台恢复 Environment 后重试")
	}
	// An explicit Environment override selects that Environment's configured
	// SOP. A Project binding may be used only when it belongs to the selected
	// Environment; arbitrary tenant SOPs cannot bypass Environment policy.
	environmentSOP, envErr := s.publishedSOPVersion(ctx, actor.TenantID, environment.DefaultSOPID, environment.DefaultSOPVersion)
	if envErr != nil {
		return WorkTaskView{}, domain.Policy("ENVIRONMENT_SOP_INVALID", "Environment 的默认 SOP 不可用于创建任务", "先在管理后台绑定已发布 SOP")
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
			return WorkTaskView{}, domain.Policy("TASK_SOP_NOT_ALLOWED", "任务只能使用 Project 绑定或 Environment 配置的 SOP 版本", "先在管理后台调整 Project 或 Environment SOP 配置")
		}
		sop, err = s.publishedSOPVersion(ctx, actor.TenantID, requestedID, requestedVersion)
		if err != nil {
			return WorkTaskView{}, err
		}
	}
	if sop.Status != "published" {
		return WorkTaskView{}, domain.Policy("SOP_NOT_PUBLISHED", "只能使用已发布 SOP 创建任务", "先在管理后台发布 SOP 版本")
	}
	now := s.now().UTC()
	status := "ready"
	nextAction := "开始第一个 Stage"
	if len(input.InputRefs) == 0 {
		status = "needs_input"
		nextAction = "补充任务输入"
	}
	stageID := ""
	if len(sop.Stages) > 0 {
		stageID = sop.Stages[0].ID
	}
	contentType := defaultString(input.ContentType, domain.ContentTypeVideoScript)
	if !domain.ValidTenantContentType(contentType) {
		return WorkTaskView{}, domain.Invalid("TASK_CONTENT_TYPE_INVALID", "任务内容类型不受支持")
	}
	contentTypeAllowed := false
	for _, candidate := range sop.ContentTypes {
		if candidate == contentType {
			contentTypeAllowed = true
			break
		}
	}
	if len(sop.ContentTypes) > 0 && !contentTypeAllowed {
		return WorkTaskView{}, domain.Policy("TASK_CONTENT_TYPE_NOT_IN_SOP", "当前 SOP 未启用该内容类型", "在 SOP Builder 中启用内容类型后重试")
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if len(idempotencyKey) > 128 {
		return WorkTaskView{}, domain.Invalid("IDEMPOTENCY_KEY_INVALID", "Idempotency-Key 不能超过 128 字符")
	}
	task := domain.WorkTask{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: project.ID, EnvironmentID: environment.ID, SOPID: sop.SOPID, SOPVersion: sop.Version, SOPDigest: sop.Digest, Title: strings.TrimSpace(input.Title), Intent: input.Intent, ContentType: contentType, InputRefs: input.InputRefs, RequestedOutput: input.RequestedOutput, AssigneeUserID: input.AssigneeUserID, Priority: defaultString(input.Priority, "normal"), DueAt: input.DueAt, RiskProfile: defaultString(input.RiskProfile, "low"), IdempotencyKey: idempotencyKey, Status: status, CurrentStageID: stageID, NextAction: nextAction, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := task.Validate(); err != nil {
		return WorkTaskView{}, err
	}
	if idempotencyKey != "" {
		if existing, lookupErr := s.store.WorkTaskByIdempotencyKey(ctx, actor.TenantID, idempotencyKey); lookupErr == nil {
			if !sameTaskCreateRequest(existing, task) {
				return WorkTaskView{}, domain.Conflict("IDEMPOTENCY_KEY_REUSE", "相同 Idempotency-Key 已用于不同任务参数")
			}
			return s.WorkTask(ctx, actor, existing.ID)
		} else if !domain.IsNotFound(lookupErr) {
			return WorkTaskView{}, lookupErr
		}
	}
	if err := s.store.CreateWorkTask(ctx, task); err != nil {
		if idempotencyKey != "" {
			if existing, lookupErr := s.store.WorkTaskByIdempotencyKey(ctx, actor.TenantID, idempotencyKey); lookupErr == nil {
				if !sameTaskCreateRequest(existing, task) {
					return WorkTaskView{}, domain.Conflict("IDEMPOTENCY_KEY_REUSE", "相同 Idempotency-Key 已用于不同任务参数")
				}
				return s.WorkTask(ctx, actor, existing.ID)
			}
		}
		return WorkTaskView{}, err
	}
	if stageID != "" {
		stage := sop.Stages[0]
		_ = s.store.CreateStageRun(ctx, domain.StageRun{ID: domain.NewID(), TenantID: actor.TenantID, TaskID: task.ID, StageID: stage.ID, Status: "pending", ExecutionMode: sop.DefaultExecutionMode, InputRefs: append([]string(nil), stage.InputRefs...), UpdatedAt: now})
	}
	s.audit(ctx, actor, project.ID, "task.created", "task", task.ID, requestID, map[string]any{"sop_id": task.SOPID, "sop_version": task.SOPVersion, "sop_digest": task.SOPDigest})
	return s.WorkTask(ctx, actor, task.ID)
}

func sameTaskCreateRequest(existing, candidate domain.WorkTask) bool {
	return existing.TenantID == candidate.TenantID && existing.ProjectID == candidate.ProjectID && existing.EnvironmentID == candidate.EnvironmentID && existing.SOPID == candidate.SOPID && existing.SOPVersion == candidate.SOPVersion && existing.SOPDigest == candidate.SOPDigest && existing.Title == candidate.Title && existing.Intent == candidate.Intent && existing.ContentType == candidate.ContentType && reflect.DeepEqual(existing.InputRefs, candidate.InputRefs) && reflect.DeepEqual(existing.RequestedOutput, candidate.RequestedOutput) && existing.AssigneeUserID == candidate.AssigneeUserID && existing.Priority == candidate.Priority && sameTime(existing.DueAt, candidate.DueAt) && existing.RiskProfile == candidate.RiskProfile
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func (s *Service) WorkTasks(ctx context.Context, actor Actor, projectID string) ([]domain.WorkTask, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	return s.store.WorkTasks(ctx, actor.TenantID, projectID)
}

func (s *Service) WorkTask(ctx context.Context, actor Actor, id string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return WorkTaskView{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	project, err := s.store.Project(ctx, actor.TenantID, task.ProjectID)
	if err != nil {
		return WorkTaskView{}, err
	}
	environment, err := s.store.Environment(ctx, actor.TenantID, task.EnvironmentID)
	if err != nil {
		return WorkTaskView{}, err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, task.SOPID)
	if err != nil {
		return WorkTaskView{}, err
	}
	var sop domain.SOPVersion
	for _, candidate := range summary.Versions {
		if candidate.Version == task.SOPVersion {
			sop = candidate
			break
		}
	}
	if sop.ID == "" {
		return WorkTaskView{}, domain.NotFound("SOP 版本")
	}
	stageRuns, err := s.store.StageRuns(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	runs, err := s.store.WorkTaskRuns(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	gates, err := s.store.GateEvaluations(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	revisions, err := s.store.TaskRevisions(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	deliveries, err := s.store.TaskDeliveries(ctx, actor.TenantID, id)
	if err != nil {
		return WorkTaskView{}, err
	}
	actions := []string{}
	if task.Status == domain.TaskStatusNeedsInput {
		actions = append(actions, "add_input")
	} else if task.Status == domain.TaskStatusReady {
		actions = append(actions, "claim", "start", "cancel")
	} else if task.Status == domain.TaskStatusRunning {
		actions = append(actions, "pause", "cancel")
	} else if task.Status == domain.TaskStatusPaused {
		actions = append(actions, "resume", "retry", "cancel")
	} else if task.Status == domain.TaskStatusWaitingGate {
		actions = append(actions, "decide", "cancel")
	} else if task.Status == domain.TaskStatusBlocked {
		actions = append(actions, "retry", "cancel")
	} else if task.Status == domain.TaskStatusAccepted {
		actions = append(actions, "submit_revision", "deliver")
	}
	return WorkTaskView{Task: task, Project: project, Environment: environment, SOP: sop, StageRuns: stageRuns, Runs: runs, Gates: gates, Revisions: revisions, Deliveries: deliveries, AllowedActions: actions, GeneratedAt: s.now().UTC()}, nil
}
