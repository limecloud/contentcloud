package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	experiencestudio "github.com/limecloud/contentcloud/internal/experience/studio"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

const (
	studioExperienceIPVideoID      = "ip_persona_marketing_video"
	studioExperienceIPVideoVersion = "1.0.0"
)

type StudioUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type StudioTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type StudioSession struct {
	User              StudioUser   `json:"user"`
	Tenant            StudioTenant `json:"tenant"`
	Role              string       `json:"role"`
	OperationsPath    string       `json:"operations_path,omitempty"`
	CanCreate         bool         `json:"can_create"`
	CanConnectClient  bool         `json:"can_connect_execution_client"`
	CanManageTeam     bool         `json:"can_manage_team"`
	CanViewOperations bool         `json:"can_view_operations"`
}

type StudioProject struct {
	ID                       string `json:"id"`
	BrandName                string `json:"brand_name"`
	ProductName              string `json:"product_name"`
	ContentType              string `json:"content_type"`
	Channel                  string `json:"channel"`
	Status                   string `json:"status"`
	ExecutionClientConnected bool   `json:"execution_client_connected"`
	ConnectedClientCount     int    `json:"connected_client_count"`
}

type StudioConnectSession struct {
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	Status               string    `json:"status"`
	Message              string    `json:"message"`
	RequiresConfirmation bool      `json:"requires_confirmation"`
	VerificationCode     string    `json:"verification_code,omitempty"`
	SupportCode          string    `json:"support_code,omitempty"`
	ExpiresAt            time.Time `json:"expires_at"`
}

type StudioExperience struct {
	ID                 string   `json:"id"`
	Version            string   `json:"version"`
	SOPID              string   `json:"-"`
	SOPVersion         int      `json:"-"`
	TemplateKey        string   `json:"-"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	ContentType        string   `json:"content_type"`
	Status             string   `json:"status"`
	ProjectIDs         []string `json:"project_ids"`
	StepTitles         []string `json:"step_titles"`
	AvailableMethods   []string `json:"available_collection_methods"`
	UnavailableMethods []string `json:"unavailable_collection_methods"`
}

type StudioBootstrap struct {
	Session     StudioSession      `json:"session"`
	Tenants     []StudioTenant     `json:"tenants"`
	Projects    []StudioProject    `json:"projects"`
	Experiences []StudioExperience `json:"experiences"`
	GeneratedAt time.Time          `json:"generated_at"`
}

type StudioCustomerStep struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Outcome         string `json:"outcome_description"`
	Status          string `json:"status"`
	ProgressSummary string `json:"progress_summary"`
}

type StudioTaskSummary struct {
	ID            string        `json:"id"`
	Project       StudioProject `json:"project"`
	ExperienceID  string        `json:"experience_id"`
	Title         string        `json:"title"`
	Intent        string        `json:"intent"`
	ContentType   string        `json:"content_type"`
	Status        string        `json:"status"`
	StatusLabel   string        `json:"status_label"`
	CurrentStepID string        `json:"current_step_id"`
	NextAction    string        `json:"next_action"`
	AssetCount    int           `json:"asset_count"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type StudioInspiration struct {
	ID                      string    `json:"id"`
	Title                   string    `json:"title"`
	Summary                 string    `json:"summary"`
	SourceType              string    `json:"source_type"`
	SourceLabel             string    `json:"source_label"`
	SavedAsProjectReference bool      `json:"saved_as_project_reference"`
	CreatedAt               time.Time `json:"created_at"`
}

type StudioDecision struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	ResultCount int    `json:"result_count"`
	CanDecide   bool   `json:"can_decide"`
}

type StudioDownload = experiencestudio.Download

type StudioResult struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Status    string           `json:"status"`
	Summary   string           `json:"summary"`
	Downloads []StudioDownload `json:"downloads"`
	CreatedAt time.Time        `json:"created_at"`
}

type StudioTaskView struct {
	Task           StudioTaskSummary    `json:"task"`
	Steps          []StudioCustomerStep `json:"steps"`
	Inspirations   []StudioInspiration  `json:"inspirations"`
	Decisions      []StudioDecision     `json:"pending_decisions"`
	Results        []StudioResult       `json:"results"`
	AttachedAssets []StudioAssetItem    `json:"attached_assets"`
	AllowedActions []string             `json:"allowed_actions"`
	GeneratedAt    time.Time            `json:"generated_at"`
}

type StudioCreateTaskInput struct {
	ExperienceID   string   `json:"experience_id"`
	ProjectID      string   `json:"project_id"`
	Title          string   `json:"title"`
	Goal           string   `json:"goal"`
	Inspiration    string   `json:"inspiration"`
	AssetRefs      []string `json:"asset_refs"`
	MaterialRefs   []string `json:"material_refs"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}

type StudioAddInspirationInput struct {
	Title                  string `json:"title"`
	Body                   string `json:"body"`
	KeepAsProjectReference bool   `json:"keep_as_project_reference"`
	IdempotencyKey         string `json:"idempotency_key,omitempty"`
}

type StudioAttachAssetsInput struct {
	AssetRefs []string `json:"asset_refs"`
}

type StudioAttachMaterialsInput struct {
	MaterialRefs []string `json:"material_refs"`
}

type StudioAssetItem = experiencestudio.CreativeResultItem
type StudioAssetFact = experiencestudio.CreativeResultFact
type StudioAssetMedia = experiencestudio.CreativeResultMedia
type StudioCreativeResultDetail = experiencestudio.CreativeResultDetail
type CreativeResultAssetProjection = experiencestudio.CreativeResultProjection

type StudioDeliveryPackage struct {
	ID          string           `json:"id"`
	ProjectName string           `json:"project_name"`
	Status      string           `json:"status"`
	Files       []StudioDownload `json:"files"`
	CreatedAt   time.Time        `json:"created_at"`
}

type StudioPublication struct {
	ID          string    `json:"id"`
	ProjectName string    `json:"project_name"`
	Destination string    `json:"destination"`
	Status      string    `json:"status"`
	PublishedAt time.Time `json:"published_at"`
}

type StudioDeliveries struct {
	Packages     []StudioDeliveryPackage `json:"packages"`
	Publications []StudioPublication     `json:"publications"`
	GeneratedAt  time.Time               `json:"generated_at"`
}

func (s *Service) CustomerStudioBootstrap(ctx context.Context, actor Actor, user domain.User) (StudioBootstrap, error) {
	tenant, err := s.Tenant(ctx, actor)
	if err != nil {
		return StudioBootstrap{}, err
	}
	tenants, err := s.Tenants(ctx, actor)
	if err != nil {
		return StudioBootstrap{}, err
	}
	projects, err := s.Projects(ctx, actor)
	if err != nil {
		return StudioBootstrap{}, err
	}
	result := StudioBootstrap{
		Session: StudioSession{
			User:             StudioUser{ID: user.ID, DisplayName: user.DisplayName},
			Tenant:           StudioTenant{ID: tenant.ID, Name: tenant.Name},
			Role:             actor.Role,
			CanCreate:        canCreateStudioTask(actor.Role),
			CanConnectClient: canConnectStudioClient(actor.Role),
			CanManageTeam:    actor.Role == "tenant_admin",
		},
		Projects:    make([]StudioProject, 0, len(projects)),
		Tenants:     make([]StudioTenant, 0, len(tenants)),
		GeneratedAt: s.now().UTC(),
	}
	if actor.PlatformAdmin {
		result.Session.OperationsPath = "/admin/dashboard"
		result.Session.CanViewOperations = true
	} else if actor.Role == "tenant_admin" || actor.Role == "project_manager" {
		result.Session.OperationsPath = "/admin/dashboard"
		result.Session.CanViewOperations = true
	}
	for _, value := range tenants {
		result.Tenants = append(result.Tenants, StudioTenant{ID: value.ID, Name: value.Name})
	}
	for _, project := range projects {
		result.Projects = append(result.Projects, studioProject(project))
	}
	result.Experiences, err = s.customerStudioExperiences(ctx, actor, projects)
	if err != nil {
		return StudioBootstrap{}, err
	}
	return result, nil
}

func (s *Service) customerStudioExperiences(ctx context.Context, actor Actor, projects []domain.Project) ([]StudioExperience, error) {
	capabilities, err := s.store.TenantContentCapabilities(ctx, actor.TenantID)
	if err != nil {
		return nil, err
	}
	enabledContentTypes := map[string]bool{domain.ContentTypeVideoScript: true}
	for _, capability := range capabilities {
		if capability.Enabled {
			enabledContentTypes[capability.ContentType] = true
		}
	}
	experiences := map[string]StudioExperience{}
	for _, project := range projects {
		if project.Status == "archived" || !enabledContentTypes[project.ContentType] {
			continue
		}
		_, sop, sopErr := s.ProjectSOP(ctx, actor, project.ID)
		if sopErr != nil || sop.Status != "published" {
			continue
		}
		summary, summaryErr := s.store.SOP(ctx, actor.TenantID, sop.SOPID)
		if summaryErr != nil {
			return nil, summaryErr
		}
		if len(sop.ContentTypes) > 0 && !containsString(sop.ContentTypes, project.ContentType) {
			continue
		}
		id, version := customerStudioExperienceIdentity(summary.Definition, sop)
		experience := experiences[id]
		if experience.ID == "" {
			experience = StudioExperience{
				ID: id, Version: version, SOPID: sop.SOPID, SOPVersion: sop.Version,
				TemplateKey: summary.Definition.TemplateKey, Name: summary.Definition.Name,
				Description: summary.Definition.Description, ContentType: project.ContentType,
				Status: "published", ProjectIDs: []string{}, StepTitles: customerStudioExperienceStepTitles(sop),
				AvailableMethods:   []string{"manual"},
				UnavailableMethods: []string{"platform_search", "controlled_fetch", "local_agent"},
			}
			if summary.Definition.TemplateKey == builtinSOPMarketingVideo || summary.Definition.ID == "builtin-sop-marketing-video" {
				experience.Name = "IP 人设营销视频"
				experience.Description = "从灵感、人物原型和营销剧本，继续到视频分镜、候选成片与交付准备。"
			}
			for _, stage := range sop.Stages {
				for _, mode := range stage.ExecutionModes {
					if mode == "agent" {
						experience.AvailableMethods = appendUnique(experience.AvailableMethods, "local_agent")
						experience.UnavailableMethods = removeString(experience.UnavailableMethods, "local_agent")
					}
					if mode == "provider" {
						experience.AvailableMethods = appendUnique(experience.AvailableMethods, "external_provider")
					}
				}
				for _, capability := range stage.RequiredCapabilities {
					if capability == "source.search" || capability == "source.fetch" {
						experience.AvailableMethods = appendUnique(experience.AvailableMethods, "platform_search")
						experience.UnavailableMethods = removeString(experience.UnavailableMethods, "platform_search")
					}
				}
			}
		}
		if !containsString(experience.ProjectIDs, project.ID) {
			experience.ProjectIDs = append(experience.ProjectIDs, project.ID)
		}
		experiences[id] = experience
	}
	result := make([]StudioExperience, 0, len(experiences))
	for _, experience := range experiences {
		sort.Strings(experience.ProjectIDs)
		if len(experience.StepTitles) == 0 {
			experience.StepTitles = []string{"灵感采集"}
		}
		result = append(result, experience)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func customerStudioExperienceIdentity(definition domain.SOPDefinition, sop domain.SOPVersion) (string, string) {
	if definition.TemplateKey == builtinSOPMarketingVideo || definition.ID == "builtin-sop-marketing-video" {
		return studioExperienceIPVideoID, studioExperienceIPVideoVersion
	}
	return "sop:" + sop.SOPID + ":v" + strconv.Itoa(sop.Version), strconv.Itoa(sop.Version) + ".0.0"
}

func (s *Service) customerStudioExperienceSOP(ctx context.Context, actor Actor, project domain.Project, experienceID string) (StudioExperience, domain.SOPDefinition, domain.SOPVersion, error) {
	experiences, err := s.customerStudioExperiences(ctx, actor, []domain.Project{project})
	if err != nil {
		return StudioExperience{}, domain.SOPDefinition{}, domain.SOPVersion{}, err
	}
	for _, experience := range experiences {
		if experience.ID != experienceID || !containsString(experience.ProjectIDs, project.ID) {
			continue
		}
		definition, sop, loadErr := s.loadTaskSOP(ctx, actor.TenantID, domain.WorkTask{TenantID: actor.TenantID, ProjectID: project.ID, SOPID: experience.SOPID, SOPVersion: experience.SOPVersion})
		if loadErr != nil {
			return StudioExperience{}, domain.SOPDefinition{}, domain.SOPVersion{}, loadErr
		}
		return experience, definition, sop, nil
	}
	return StudioExperience{}, domain.SOPDefinition{}, domain.SOPVersion{}, domain.Policy("STUDIO_EXPERIENCE_UNAVAILABLE", "当前项目尚未启用这条创作流水线", "请联系运营人员检查租户能力、项目和已发布流程版本")
}

func (s *Service) CustomerStudioTasks(ctx context.Context, actor Actor) ([]StudioTaskSummary, error) {
	tasks, err := s.WorkTasks(ctx, actor, "")
	if err != nil {
		return nil, err
	}
	result := make([]StudioTaskSummary, 0, len(tasks))
	for _, task := range tasks {
		project, projectErr := s.store.Project(ctx, actor.TenantID, task.ProjectID)
		if projectErr != nil {
			return nil, projectErr
		}
		_, sop, sopErr := s.loadTaskSOP(ctx, actor.TenantID, task)
		if sopErr != nil {
			return nil, sopErr
		}
		result = append(result, studioTaskSummary(task, project, sop))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Service) CustomerStudioTask(ctx context.Context, actor Actor, taskID string) (StudioTaskView, error) {
	view, err := s.WorkTask(ctx, actor, taskID)
	if err != nil {
		return StudioTaskView{}, err
	}
	items, err := s.InputItems(ctx, actor, InputItemQuery{ProjectID: view.Task.ProjectID})
	if err != nil {
		return StudioTaskView{}, err
	}
	assets, err := s.CustomerStudioCreativeResults(ctx, actor, view.Task.ProjectID)
	if err != nil {
		return StudioTaskView{}, err
	}
	return s.customerStudioTaskView(actor, view, items, assets), nil
}

func (s *Service) CreateCustomerStudioTask(ctx context.Context, actor Actor, input StudioCreateTaskInput, requestID string) (StudioTaskView, error) {
	if !canCreateStudioTask(actor.Role) {
		return StudioTaskView{}, domain.Policy("ROLE_DENIED", "当前角色不能创建创作任务", "联系团队管理员调整创作权限")
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Goal) == "" {
		return StudioTaskView{}, domain.Invalid("STUDIO_TASK_FIELDS_REQUIRED", "任务名称和创作目标必填")
	}
	project, err := s.store.Project(ctx, actor.TenantID, strings.TrimSpace(input.ProjectID))
	if err != nil {
		return StudioTaskView{}, err
	}
	if project.ConnectedDevices == 0 {
		return StudioTaskView{}, domain.Conflict("STUDIO_EXECUTION_CLIENT_REQUIRED", "开始新的创作前，需要先连接你的创作电脑")
	}
	experience, _, sop, err := s.customerStudioExperienceSOP(ctx, actor, project, strings.TrimSpace(input.ExperienceID))
	if err != nil {
		return StudioTaskView{}, err
	}
	assetRefs, err := s.resolveCustomerStudioAssetRefs(ctx, actor, project.ID, input.AssetRefs)
	if err != nil {
		return StudioTaskView{}, err
	}
	materialRefs, err := s.resolveCustomerWorkspaceMaterialRefs(ctx, actor, project.ID, input.MaterialRefs)
	if err != nil {
		return StudioTaskView{}, err
	}
	contentType := defaultString(project.ContentType, domain.DefaultProjectContentType)
	key := strings.TrimSpace(input.IdempotencyKey)
	if key != "" {
		if existing, lookupErr := s.store.WorkTaskByIdempotencyKey(ctx, actor.TenantID, key); lookupErr == nil {
			if existing.ProjectID != project.ID || existing.Title != strings.TrimSpace(input.Title) || existing.Intent != strings.TrimSpace(input.Goal) || existing.ContentType != contentType || existing.SOPID != sop.SOPID || existing.SOPVersion != sop.Version {
				return StudioTaskView{}, domain.Conflict("IDEMPOTENCY_KEY_REUSE", "相同的幂等键已用于不同创作任务，请为新任务生成新的幂等键")
			}
			if err := s.ensureCustomerStudioRuntime(ctx, actor, existing, sop, key, requestID); err != nil {
				return StudioTaskView{}, err
			}
			return s.CustomerStudioTask(ctx, actor, existing.ID)
		} else if !domain.IsNotFound(lookupErr) {
			return StudioTaskView{}, lookupErr
		}
	}
	briefKey := ""
	if key != "" {
		briefKey = "studio-brief:" + key
	}
	body := strings.TrimSpace(input.Goal)
	if inspiration := strings.TrimSpace(input.Inspiration); inspiration != "" {
		body += "\n\n参考灵感：\n" + inspiration
	}
	brief, err := s.CreateInputItem(ctx, actor, CreateInputItemInput{
		ProjectID: project.ID, SourceType: "brief", Title: strings.TrimSpace(input.Title) + " · 创作简报",
		Summary: strings.TrimSpace(input.Goal), Body: body, Disclosure: "project", IdempotencyKey: briefKey,
		Metadata: map[string]any{"collection_method": "customer_brief", "stage": "inspiration", "experience_id": experience.ID, "sop_id": sop.SOPID, "sop_version": sop.Version},
	}, requestID)
	if err != nil {
		return StudioTaskView{}, err
	}
	refs := append([]string{"input:" + brief.ID + "@v" + fmt.Sprint(brief.RowVersion)}, materialRefs...)
	refs = append(refs, assetRefs...)
	created, err := s.CreateWorkTask(ctx, actor, CreateWorkTaskInput{
		ProjectID: project.ID, Title: strings.TrimSpace(input.Title), Intent: strings.TrimSpace(input.Goal),
		SOPID: sop.SOPID, SOPVersion: sop.Version, ContentType: contentType, InputRefs: refs,
		RequestedOutput: map[string]any{"content_count": 1, "format": contentFormat(contentType), "experience_id": experience.ID, "experience_version": experience.Version},
		Priority:        "normal", RiskProfile: "low", IdempotencyKey: key,
	}, requestID)
	if err != nil {
		return StudioTaskView{}, err
	}
	if err := s.markCustomerWorkspaceMaterialsUsed(ctx, actor, project.ID, input.MaterialRefs); err != nil {
		return StudioTaskView{}, err
	}
	if brief.TargetTaskID != created.Task.ID || brief.Status != domain.InputItemTaskMerged {
		if _, err := s.TriageInputItem(ctx, actor, brief.ID, TriageInputItemInput{Action: "merge_task", ExpectedVersion: brief.RowVersion, TaskID: created.Task.ID}, requestID); err != nil {
			return StudioTaskView{}, err
		}
	}
	if err := s.ensureCustomerStudioRuntime(ctx, actor, created.Task, sop, key, requestID); err != nil {
		return StudioTaskView{}, err
	}
	return s.CustomerStudioTask(ctx, actor, created.Task.ID)
}

func (s *Service) ensureCustomerStudioRuntime(ctx context.Context, actor Actor, task domain.WorkTask, sop domain.SOPVersion, idempotencyKey, requestID string) error {
	if s.runtimeService == nil {
		err := domain.Policy("RUNTIME_UNAVAILABLE", "创作任务已建立，但运行时尚未配置", "请联系平台运营人员启用 Agentic Job Runtime")
		s.audit(ctx, actor, task.ProjectID, "runtime.start_failed", "task", task.ID, requestID, map[string]any{"error_code": "RUNTIME_UNAVAILABLE"})
		return err
	}
	bindingDigest, inputDigest, err := customerStudioRuntimeAdmissionDigests(task, sop)
	if err != nil {
		s.audit(ctx, actor, task.ProjectID, "runtime.start_failed", "task", task.ID, requestID, map[string]any{"error_code": "RUNTIME_ADMISSION_DIGEST_FAILED"})
		return domain.Policy("RUNTIME_ADMISSION_DIGEST_FAILED", "创作任务的运行时准入快照无法冻结", "请检查任务输入和执行绑定后重试")
	}
	if _, err := s.runtimeService.Start(ctx, contentruntime.StartInput{
		TenantID: actor.TenantID, ProjectID: task.ProjectID, WorkTaskID: task.ID, SOP: sop,
		BindingDigest: bindingDigest, InputDigest: inputDigest, RuntimePolicyID: "runtime-policy/customer-studio-v1",
		ContractMajor: 1, ContractMinor: 0,
		Priority: runtimePriority(task.Priority), CreatedBy: actor.UserID, IdempotencyKey: idempotencyKey,
		CorrelationID: requestID,
	}); err != nil {
		errorCode := "RUNTIME_START_FAILED"
		if value, ok := err.(*domain.Error); ok && value.Code != "" {
			errorCode = value.Code
		}
		s.audit(ctx, actor, task.ProjectID, "runtime.start_failed", "task", task.ID, requestID, map[string]any{"error_code": errorCode})
		return domain.Policy("RUNTIME_START_FAILED", "创作任务已建立，但执行运行时启动失败", "请在运营后台的 Runtime 运行页刷新或联系平台运营人员处理")
	}
	s.audit(ctx, actor, task.ProjectID, "runtime.started", "task", task.ID, requestID, map[string]any{"sop_id": sop.SOPID, "sop_version": sop.Version})
	return nil
}

func customerStudioRuntimeAdmissionDigests(task domain.WorkTask, sop domain.SOPVersion) (string, string, error) {
	bindingDigest, err := domain.CanonicalHash(struct {
		EnvironmentID        string `json:"environment_id"`
		SOPID                string `json:"sop_id"`
		SOPVersion           int    `json:"sop_version"`
		SOPDigest            string `json:"sop_digest"`
		DefaultExecutionMode string `json:"default_execution_mode"`
	}{task.EnvironmentID, sop.SOPID, sop.Version, task.SOPDigest, sop.DefaultExecutionMode})
	if err != nil {
		return "", "", err
	}
	inputDigest, err := domain.CanonicalHash(struct {
		ContentType     string         `json:"content_type"`
		InputRefs       []string       `json:"input_refs"`
		RequestedOutput map[string]any `json:"requested_output"`
	}{task.ContentType, task.InputRefs, task.RequestedOutput})
	if err != nil {
		return "", "", err
	}
	return "sha256:" + bindingDigest, "sha256:" + inputDigest, nil
}

func contentFormat(contentType string) string {
	switch contentType {
	case domain.ContentTypeMarketingVideo:
		return "vertical_video"
	case domain.ContentTypeVideoScript:
		return "video_script"
	default:
		return "content"
	}
}

func runtimePriority(value string) int {
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		return parsed
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "urgent":
		return 20
	case "high":
		return 10
	default:
		return 0
	}
}

func (s *Service) CustomerStudioTaskAction(ctx context.Context, actor Actor, taskID string, input TaskActionInput, requestID string) (StudioTaskView, error) {
	if _, err := s.TaskAction(ctx, actor, taskID, input, requestID); err != nil {
		return StudioTaskView{}, err
	}
	return s.CustomerStudioTask(ctx, actor, taskID)
}

func (s *Service) AddCustomerStudioInspiration(ctx context.Context, actor Actor, taskID string, input StudioAddInspirationInput, requestID string) (StudioTaskView, error) {
	if !canCreateStudioTask(actor.Role) {
		return StudioTaskView{}, domain.Policy("ROLE_DENIED", "当前角色不能补充创作资料", "联系团队管理员调整创作权限")
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return StudioTaskView{}, err
	}
	if task.Status == domain.TaskStatusDelivered || task.Status == domain.TaskStatusCancelled {
		return StudioTaskView{}, domain.Conflict("STUDIO_TASK_INPUT_CLOSED", "已完成或已取消的任务不能继续添加灵感")
	}
	item, err := s.CreateInputItem(ctx, actor, CreateInputItemInput{
		ProjectID: task.ProjectID, SourceType: "manual_inspiration", Title: strings.TrimSpace(input.Title),
		Summary: strings.TrimSpace(input.Body), Body: strings.TrimSpace(input.Body), Disclosure: "project", IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		Metadata: map[string]any{"collection_method": "manual", "stage": "inspiration", "saved_as_project_reference": input.KeepAsProjectReference},
	}, requestID)
	if err != nil {
		return StudioTaskView{}, err
	}
	if item.TargetTaskID != task.ID || item.Status != domain.InputItemTaskMerged {
		if _, err := s.TriageInputItem(ctx, actor, item.ID, TriageInputItemInput{Action: "merge_task", ExpectedVersion: item.RowVersion, TaskID: task.ID}, requestID); err != nil {
			return StudioTaskView{}, err
		}
	}
	return s.CustomerStudioTask(ctx, actor, task.ID)
}

func (s *Service) DecideCustomerStudioTask(ctx context.Context, actor Actor, taskID, decisionID string, input GateDecisionInput, requestID string) (StudioTaskView, error) {
	if input.Decision == "rejected" {
		input.Decision = "changes_requested"
	}
	if _, err := s.DecideGate(ctx, actor, taskID, decisionID, input, requestID); err != nil {
		return StudioTaskView{}, err
	}
	return s.CustomerStudioTask(ctx, actor, taskID)
}

func (s *Service) AttachCustomerStudioAssets(ctx context.Context, actor Actor, taskID string, input StudioAttachAssetsInput, requestID string) (StudioTaskView, error) {
	if !canCreateStudioTask(actor.Role) {
		return StudioTaskView{}, domain.Policy("ROLE_DENIED", "当前角色不能修改任务资料", "联系团队管理员调整创作权限")
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return StudioTaskView{}, err
	}
	if task.Status == domain.TaskStatusDelivered || task.Status == domain.TaskStatusCancelled {
		return StudioTaskView{}, domain.Conflict("STUDIO_TASK_INPUT_CLOSED", "已完成或已取消的任务不能继续加入资产")
	}
	refs, err := s.resolveCustomerStudioAssetRefs(ctx, actor, task.ProjectID, input.AssetRefs)
	if err != nil {
		return StudioTaskView{}, err
	}
	for _, ref := range refs {
		task.InputRefs = appendUnique(task.InputRefs, ref)
	}
	if task.Status == domain.TaskStatusNeedsInput && len(task.InputRefs) > 0 {
		task.Status = domain.TaskStatusReady
		task.NextAction = "开始第一个流程阶段"
	}
	task.UpdatedAt = s.now().UTC()
	if err := s.store.SaveWorkTask(ctx, task); err != nil {
		return StudioTaskView{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "studio.task_assets_attached", "task", task.ID, requestID, map[string]any{"asset_count": len(refs)})
	return s.CustomerStudioTask(ctx, actor, task.ID)
}

func (s *Service) CustomerStudioCreativeResults(ctx context.Context, actor Actor, projectID string) (CreativeResultAssetProjection, error) {
	projects, err := s.Projects(ctx, actor)
	if err != nil {
		return CreativeResultAssetProjection{}, err
	}
	if projectID != "" {
		filtered := projects[:0]
		for _, project := range projects {
			if project.ID == projectID {
				filtered = append(filtered, project)
			}
		}
		projects = filtered
		if len(projects) == 0 {
			return CreativeResultAssetProjection{}, domain.NotFound("项目")
		}
	}
	result := CreativeResultAssetProjection{Items: []StudioAssetItem{}, Counts: map[string]int{}, GeneratedAt: s.now().UTC()}
	for _, project := range projects {
		items, itemErr := s.customerStudioProjectAssets(ctx, actor, project)
		if itemErr != nil {
			return CreativeResultAssetProjection{}, itemErr
		}
		result.Items = append(result.Items, items...)
	}
	return experiencestudio.BuildCreativeResultProjection(result.Items, s.now()), nil
}

func (s *Service) CustomerStudioCreativeResult(ctx context.Context, actor Actor, taskID, resultID string) (StudioCreativeResultDetail, error) {
	view, err := s.WorkTask(ctx, actor, strings.TrimSpace(taskID))
	if err != nil {
		return StudioCreativeResultDetail{}, err
	}
	targetRef := "result:" + strings.TrimPrefix(strings.TrimSpace(resultID), "result:")
	var item *StudioAssetItem
	for _, candidate := range customerStudioTaskAssets(view) {
		if candidate.Ref == targetRef {
			value := candidate
			item = &value
			break
		}
	}
	if item == nil {
		return StudioCreativeResultDetail{}, domain.NotFound("创作结果")
	}

	content := json.RawMessage(`{}`)
	for _, snapshot := range view.KnowledgeSnapshots {
		if "result:"+snapshot.ID != item.Ref {
			continue
		}
		facts := make([]StudioAssetFact, 0, len(snapshot.Objects))
		for _, object := range snapshot.Objects {
			facts = append(facts, StudioAssetFact{Title: object.Title, Statement: object.Statement, ObjectType: object.ObjectType, Layer: object.Layer})
		}
		content, err = json.Marshal(map[string]any{"facts": facts})
		if err != nil {
			return StudioCreativeResultDetail{}, err
		}
		break
	}
	for _, revision := range view.Revisions {
		if "result:"+revision.ID == item.Ref {
			content, err = studioCustomerRenderableJSON(revision.Content)
			if err != nil {
				return StudioCreativeResultDetail{}, err
			}
			break
		}
	}
	for _, snapshot := range view.ApprovedSnapshots {
		if "result:"+snapshot.ID != item.Ref {
			continue
		}
		var envelope struct {
			Objects []json.RawMessage `json:"objects"`
		}
		if json.Unmarshal(snapshot.CanonicalContent, &envelope) == nil {
			raw, marshalErr := json.Marshal(map[string]any{"objects": envelope.Objects})
			if marshalErr != nil {
				return StudioCreativeResultDetail{}, marshalErr
			}
			content, err = studioCustomerRenderableJSON(raw)
			if err != nil {
				return StudioCreativeResultDetail{}, err
			}
		}
		break
	}

	media := []StudioAssetMedia{}
	for _, artifact := range view.Artifacts {
		if artifact.ApprovedSnapshotID != strings.TrimPrefix(item.Ref, "result:") || (artifact.Visibility != "" && artifact.Visibility != "client") {
			continue
		}
		assetRef := metadataString(artifact.Metadata, "storyboard_asset_id")
		if assetRef == "" {
			continue
		}
		media = append(media, StudioAssetMedia{AssetRef: assetRef, ShotID: metadataString(artifact.Metadata, "shot_id"), Role: metadataString(artifact.Metadata, "role"), File: studioDownload(artifact)})
	}
	return StudioCreativeResultDetail{Item: *item, ContentFormat: item.ResultType, Content: content, Media: media, ReadOnly: true}, nil
}

func studioCustomerRenderableJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	cleaned := studioCustomerRenderableValue(value)
	return json.Marshal(cleaned)
}

func studioCustomerRenderableValue(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = studioCustomerRenderableValue(typed[index])
		}
		return result
	case map[string]any:
		blocked := map[string]bool{
			"tenant_id": true, "workspace_id": true, "submission_id": true, "submission_revision_id": true,
			"approved_snapshot_id": true, "generator_capability": true, "source_digest": true, "locked_digest": true,
			"rights_refs": true, "knowledge_refs": true, "claim_refs": true, "evidence_refs": true,
			"created_by": true, "updated_by": true,
		}
		result := map[string]any{}
		for key, nested := range typed {
			if blocked[key] {
				continue
			}
			result[key] = studioCustomerRenderableValue(nested)
		}
		return result
	default:
		return value
	}
}

func (s *Service) customerStudioProjectAssets(ctx context.Context, actor Actor, project domain.Project) ([]StudioAssetItem, error) {
	result := []StudioAssetItem{}
	tasks, err := s.WorkTasks(ctx, actor, project.ID)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		view, viewErr := s.WorkTask(ctx, actor, task.ID)
		if viewErr != nil {
			return nil, viewErr
		}
		result = append(result, customerStudioTaskAssets(view)...)
	}
	return result, nil
}

func customerStudioTaskAssets(view WorkTaskView) []StudioAssetItem {
	result := []StudioAssetItem{}
	base := func(id, resultType, inputRef, title, summary, version, status string, createdAt time.Time) StudioAssetItem {
		reusable := status == "confirmed" || status == "delivered"
		return StudioAssetItem{
			Ref: "result:" + id, InputRef: inputRef, ResultType: resultType,
			ProjectID: view.Project.ID, ProjectName: view.Project.BrandName, TaskID: view.Task.ID, TaskTitle: view.Task.Title,
			Title: title, Summary: summary, Version: version, Status: status, Reusable: reusable,
			BlockedReason: studioAssetBlockedReason(status), Downloads: []StudioDownload{}, CreatedAt: createdAt,
		}
	}

	personaSnapshotIDs := map[string]bool{}
	for _, output := range view.StageOutputs {
		if output.OutputType == domain.StageOutputKnowledgeSnapshot && studioOutputCustomerStep(view, output.StageRunID) == "persona" {
			personaSnapshotIDs[output.ObjectID] = true
		}
	}
	for _, snapshot := range view.KnowledgeSnapshots {
		if !personaSnapshotIDs[snapshot.ID] {
			continue
		}
		result = append(result, base(
			snapshot.ID, "persona", "knowledge_snapshot:"+snapshot.ID+"@"+snapshot.Digest,
			view.Task.Title+" · 人物原型", fmt.Sprintf("%d 个人物与受众要点已固定", len(snapshot.Objects)), fmt.Sprintf("v%d", snapshot.PackVersion), "confirmed", snapshot.CreatedAt,
		))
	}

	revisionHashes := map[string]bool{}
	for _, revision := range view.Revisions {
		status := studioRevisionAssetStatus(revision.Status)
		inputRef := fmt.Sprintf("task_revision:%s@%s", revision.ID, revision.ContentHash)
		result = append(result, base(
			revision.ID, "script", inputRef, taskRevisionTitle(revision),
			fmt.Sprintf("来自“%s”的第 %d 版剧本", view.Task.Title, revision.RevisionNo), fmt.Sprintf("v%d", revision.RevisionNo), status, revision.CreatedAt,
		))
		revisionHashes[revision.ContentHash] = true
	}

	for _, snapshot := range view.ApprovedSnapshots {
		if revisionHashes[snapshot.ContentHash] {
			continue
		}
		resultType := studioSnapshotResultType(view, snapshot)
		if resultType == "" {
			continue
		}
		result = append(result, base(
			snapshot.ID, resultType, "approved_snapshot:"+snapshot.ID+"@"+snapshot.SubjectHash,
			view.Task.Title+" · "+studioResultTypeLabel(resultType),
			fmt.Sprintf("来自“%s”的%s结果已经确认", view.Task.Title, studioResultTypeLabel(resultType)), "固定版本", "confirmed", snapshot.CreatedAt,
		))
	}

	finalArtifacts := map[string]bool{}
	for _, output := range view.StageOutputs {
		if output.OutputType == domain.StageOutputArtifact && output.Role == domain.StageOutputRoleFinal {
			finalArtifacts[output.ObjectID] = true
		}
	}
	seenArtifacts := map[string]bool{}
	for _, artifact := range view.Artifacts {
		if seenArtifacts[artifact.ID] || artifact.Purpose == "delivery" || artifact.Kind == "delivery" || (artifact.Visibility != "" && artifact.Visibility != "client") {
			continue
		}
		resultType := studioArtifactResultType(artifact)
		if resultType == "" {
			continue
		}
		seenArtifacts[artifact.ID] = true
		status := studioArtifactAssetStatus(view, artifact, finalArtifacts[artifact.ID])
		titleSuffix := studioResultTypeLabel(resultType)
		version := "生成结果"
		if finalArtifacts[artifact.ID] {
			titleSuffix = "最终" + titleSuffix
			version = "最终版本"
		}
		item := base(
			artifact.ID, resultType, "artifact:"+artifact.ID+"@sha256:"+strings.TrimPrefix(artifact.SHA256, "sha256:"),
			view.Task.Title+" · "+titleSuffix, defaultString(artifact.FileName, "由创作流水线生成的"+studioResultTypeLabel(resultType)), version, status, artifact.CreatedAt,
		)
		item.Downloads = []StudioDownload{studioDownload(artifact)}
		result = append(result, item)
	}
	return result
}

func studioOutputCustomerStep(view WorkTaskView, stageRunID string) string {
	for _, run := range view.StageRuns {
		if run.ID == stageRunID {
			return studioCustomerStepID(run.StageID, view.SOP)
		}
	}
	return ""
}

func studioRevisionAssetStatus(status string) string {
	switch status {
	case domain.TaskRevisionAccepted:
		return "confirmed"
	case domain.TaskRevisionRejected:
		return "changes_requested"
	case domain.TaskRevisionSuperseded:
		return "superseded"
	case domain.TaskRevisionDraft:
		return "draft"
	default:
		return "pending_confirmation"
	}
}

func studioSnapshotResultType(view WorkTaskView, snapshot domain.ApprovedSnapshot) string {
	for _, output := range view.StageOutputs {
		if output.ObjectID != snapshot.ID {
			continue
		}
		switch studioOutputCustomerStep(view, output.StageRunID) {
		case "persona":
			return "persona"
		case "script":
			return "script"
		case "storyboard":
			return "storyboard"
		}
	}
	switch snapshot.SubmissionType {
	case "strategy", "offer":
		return "persona"
	case "brief", "content_batch":
		return "script"
	case "storyboard":
		return "storyboard"
	case "asset_batch":
		return "image"
	default:
		return ""
	}
}

func studioArtifactResultType(artifact domain.Artifact) string {
	switch {
	case strings.HasPrefix(artifact.MediaType, "image/"):
		return "image"
	case strings.HasPrefix(artifact.MediaType, "video/"):
		return "video"
	default:
		return ""
	}
}

func studioArtifactAssetStatus(view WorkTaskView, artifact domain.Artifact, final bool) string {
	if final && view.Task.Status == domain.TaskStatusDelivered {
		return "delivered"
	}
	hasReview, pending, changesRequested, approved := false, false, false, false
	for _, review := range view.MediaReviews {
		if review.SubjectArtifactID != artifact.ID {
			continue
		}
		hasReview = true
		switch review.Status {
		case domain.MediaReviewPending:
			pending = true
		case domain.MediaReviewChanges, domain.MediaReviewRejected:
			changesRequested = true
		case domain.MediaReviewApproved:
			approved = true
		}
	}
	if changesRequested {
		return "changes_requested"
	}
	if pending {
		return "pending_confirmation"
	}
	if hasReview && !approved {
		return "pending_confirmation"
	}
	if !hasReview {
		return "pending_confirmation"
	}
	return "confirmed"
}

func studioAssetBlockedReason(status string) string {
	switch status {
	case "draft":
		return "草稿结果需要先提交确认"
	case "pending_confirmation":
		return "当前结果确认后即可用于新的创作"
	case "changes_requested":
		return "当前结果需要完成修改并重新确认"
	case "superseded":
		return "已有更新版本，请使用最新确认结果"
	default:
		return ""
	}
}

func studioResultTypeLabel(resultType string) string {
	return map[string]string{"persona": "人物原型", "script": "剧本", "storyboard": "分镜", "image": "图片", "video": "视频"}[resultType]
}

func (s *Service) resolveCustomerStudioAssetRefs(ctx context.Context, actor Actor, projectID string, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return []string{}, nil
	}
	catalog, err := s.CustomerStudioCreativeResults(ctx, actor, projectID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{}
	for _, item := range catalog.Items {
		if item.Reusable && item.Ref != "" && item.InputRef != "" {
			allowed[item.Ref] = item.InputRef
		}
	}
	result := []string{}
	for _, ref := range uniqueNonEmpty(refs) {
		inputRef, ok := allowed[ref]
		if !ok {
			return nil, domain.Policy("STUDIO_ASSET_NOT_REUSABLE", "所选创作结果尚未确认或不属于当前项目", "刷新创作结果并选择已确认的版本")
		}
		result = append(result, inputRef)
	}
	return result, nil
}

func (s *Service) CustomerStudioDeliveries(ctx context.Context, actor Actor) (StudioDeliveries, error) {
	projects, err := s.Projects(ctx, actor)
	if err != nil {
		return StudioDeliveries{}, err
	}
	result := StudioDeliveries{Packages: []StudioDeliveryPackage{}, Publications: []StudioPublication{}, GeneratedAt: s.now().UTC()}
	projectNames := map[string]string{}
	for _, project := range projects {
		projectNames[project.ID] = project.BrandName
		packages, packageErr := s.DeliveryPackages(ctx, actor, project.ID)
		if packageErr != nil {
			return StudioDeliveries{}, packageErr
		}
		for _, value := range packages {
			files := make([]StudioDownload, 0, len(value.Manifest))
			for _, artifact := range value.Manifest {
				files = append(files, studioDownload(artifact))
			}
			result.Packages = append(result.Packages, StudioDeliveryPackage{ID: value.ID, ProjectName: project.BrandName, Status: value.Status, Files: files, CreatedAt: value.CreatedAt})
		}
	}
	tasks, err := s.WorkTasks(ctx, actor, "")
	if err != nil {
		return StudioDeliveries{}, err
	}
	for _, task := range tasks {
		deliveries, deliveryErr := s.WorkTaskDeliveries(ctx, actor, task.ID)
		if deliveryErr != nil {
			return StudioDeliveries{}, deliveryErr
		}
		for _, value := range deliveries {
			if value.Status != domain.TaskDeliveryDelivered || value.DeliveredAt == nil {
				continue
			}
			result.Publications = append(result.Publications, StudioPublication{ID: value.ID, ProjectName: projectNames[value.ProjectID], Destination: value.Destination, Status: value.Status, PublishedAt: *value.DeliveredAt})
		}
	}
	sort.Slice(result.Packages, func(i, j int) bool { return result.Packages[i].CreatedAt.After(result.Packages[j].CreatedAt) })
	sort.Slice(result.Publications, func(i, j int) bool {
		return result.Publications[i].PublishedAt.After(result.Publications[j].PublishedAt)
	})
	return result, nil
}

func (s *Service) customerStudioTaskView(actor Actor, view WorkTaskView, items []domain.InputItem, catalog CreativeResultAssetProjection) StudioTaskView {
	result := StudioTaskView{
		Task: studioTaskSummary(view.Task, view.Project, view.SOP), Steps: studioCustomerSteps(view.Task, view.SOP),
		Inspirations: []StudioInspiration{}, Decisions: []StudioDecision{}, Results: []StudioResult{}, AttachedAssets: []StudioAssetItem{},
		AllowedActions: customerStudioAllowedActions(actor, view), GeneratedAt: s.now().UTC(),
	}
	for _, item := range items {
		if item.TargetTaskID != view.Task.ID && !containsString(view.Task.InputRefs, "input:"+item.ID) && !containsStringPrefix(view.Task.InputRefs, "input:"+item.ID+"@") {
			continue
		}
		saved := metadataBool(item.Metadata, "saved_as_project_reference")
		result.Inspirations = append(result.Inspirations, StudioInspiration{ID: item.ID, Title: item.Title, Summary: defaultString(item.Summary, item.Body), SourceType: item.SourceType, SourceLabel: inputSourceLabel(item.SourceType), SavedAsProjectReference: saved, CreatedAt: item.CreatedAt})
	}
	currentTitle := studioStepTitle(result.Task.CurrentStepID)
	if currentTitle == "" {
		for _, step := range result.Steps {
			if step.ID == result.Task.CurrentStepID {
				currentTitle = step.Title
				break
			}
		}
	}
	if currentTitle == "" {
		currentTitle = "当前步骤"
	}
	for _, gate := range view.Gates {
		if gate.Status != domain.GateEvaluationPending {
			continue
		}
		result.Decisions = append(result.Decisions, StudioDecision{ID: gate.ID, Title: currentTitle + "等待确认", Summary: "当前结果已经固定，请确认是否继续进入下一步。", ResultCount: len(gate.InputRefs), CanDecide: canDecideCustomerStudioGate(actor.Role, gate.GateMode)})
	}
	for _, revision := range view.Revisions {
		result.Results = append(result.Results, StudioResult{ID: revision.ID, Kind: "content_revision", Title: taskRevisionTitle(revision), Status: revision.Status, Summary: fmt.Sprintf("第 %d 版内容，等待流程确认", revision.RevisionNo), Downloads: []StudioDownload{}, CreatedAt: revision.CreatedAt})
	}
	for _, snapshot := range view.ApprovedSnapshots {
		result.Results = append(result.Results, StudioResult{ID: snapshot.ID, Kind: "approved_result", Title: snapshotTypeLabel(snapshot.SubmissionType), Status: "approved", Summary: fmt.Sprintf("%d 个关联文件，当前版本已经固定", len(snapshot.Artifacts)), Downloads: []StudioDownload{}, CreatedAt: snapshot.CreatedAt})
	}
	for _, pkg := range view.DeliveryPackages {
		files := make([]StudioDownload, 0, len(pkg.Manifest))
		for _, artifact := range pkg.Manifest {
			files = append(files, studioDownload(artifact))
		}
		result.Results = append(result.Results, StudioResult{ID: pkg.ID, Kind: "delivery_package", Title: "正式交付包", Status: pkg.Status, Summary: fmt.Sprintf("%d 个文件已固定；交付包不等于已经发布", len(files)), Downloads: files, CreatedAt: pkg.CreatedAt})
	}
	sort.Slice(result.Results, func(i, j int) bool { return result.Results[i].CreatedAt.After(result.Results[j].CreatedAt) })
	attached := map[string]bool{}
	for _, ref := range view.Task.InputRefs {
		attached[ref] = true
	}
	for _, item := range catalog.Items {
		if attached[item.InputRef] {
			result.AttachedAssets = append(result.AttachedAssets, item)
		}
	}
	return result
}

func metadataBool(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && parsed
	default:
		return false
	}
}

func studioProject(project domain.Project) StudioProject {
	return StudioProject{
		ID: project.ID, BrandName: project.BrandName, ProductName: project.ProductName,
		ContentType: project.ContentType, Channel: project.Channel, Status: project.Status,
		ExecutionClientConnected: project.ConnectedDevices > 0, ConnectedClientCount: project.ConnectedDevices,
	}
}

func (s *Service) CreateCustomerStudioConnectSession(ctx context.Context, actor Actor, projectID, requestID string) (StudioConnectSession, error) {
	if !canConnectStudioClient(actor.Role) {
		return StudioConnectSession{}, domain.Policy("ROLE_DENIED", "当前账号不能连接创作电脑", "请联系团队负责人开通创作权限")
	}
	session, err := s.createConnectSession(ctx, actor, strings.TrimSpace(projectID), requestID)
	return studioConnectSession(session), err
}

func (s *Service) CustomerStudioConnectSession(ctx context.Context, actor Actor, sessionID string) (StudioConnectSession, error) {
	session, err := s.ConnectSession(ctx, actor, strings.TrimSpace(sessionID))
	return studioConnectSession(session), err
}

func (s *Service) ApproveCustomerStudioConnectSession(ctx context.Context, actor Actor, sessionID, requestID string) (StudioConnectSession, error) {
	session, err := s.ConnectSession(ctx, actor, strings.TrimSpace(sessionID))
	if err != nil {
		return StudioConnectSession{}, err
	}
	if session.Progress == nil || session.Progress.AttemptID == "" || session.Progress.UserCode == "" {
		return studioConnectSession(session), domain.Conflict("STUDIO_CLIENT_CONFIRMATION_NOT_READY", "Codex 尚未发起连接确认")
	}
	if !canConnectStudioClient(actor.Role) {
		return studioConnectSession(session), domain.Policy("ROLE_DENIED", "当前账号不能确认创作电脑连接", "请联系团队负责人开通创作权限")
	}
	if _, err := s.approveBootstrapAuthorization(ctx, actor, session.ID, session.Progress.AttemptID, requestID, true); err != nil {
		return studioConnectSession(session), err
	}
	session, err = s.ConnectSession(ctx, actor, session.ID)
	return studioConnectSession(session), err
}

func (s *Service) DenyCustomerStudioConnectSession(ctx context.Context, actor Actor, sessionID, requestID string) (StudioConnectSession, error) {
	session, err := s.ConnectSession(ctx, actor, strings.TrimSpace(sessionID))
	if err != nil {
		return StudioConnectSession{}, err
	}
	if session.Progress == nil || session.Progress.AttemptID == "" || session.Progress.UserCode == "" {
		return studioConnectSession(session), domain.Conflict("STUDIO_CLIENT_CONFIRMATION_NOT_READY", "Codex 尚未发起连接确认")
	}
	if !canConnectStudioClient(actor.Role) {
		return studioConnectSession(session), domain.Policy("ROLE_DENIED", "当前账号不能拒绝创作电脑连接", "请联系团队负责人开通创作权限")
	}
	if _, err := s.denyBootstrapAuthorization(ctx, actor, session.ID, session.Progress.AttemptID, requestID, true); err != nil {
		return studioConnectSession(session), err
	}
	session, err = s.ConnectSession(ctx, actor, session.ID)
	return studioConnectSession(session), err
}

func (s *Service) CancelCustomerStudioConnectSession(ctx context.Context, actor Actor, sessionID, requestID string) (StudioConnectSession, error) {
	if !canConnectStudioClient(actor.Role) {
		return StudioConnectSession{}, domain.Policy("ROLE_DENIED", "当前账号不能取消创作电脑连接", "请联系团队负责人开通创作权限")
	}
	session, err := s.cancelConnectSession(ctx, actor, strings.TrimSpace(sessionID), requestID)
	return studioConnectSession(session), err
}

func studioConnectSession(session domain.ConnectSession) StudioConnectSession {
	view := StudioConnectSession{
		ID: session.ID, ProjectID: session.ProjectID, Status: session.State,
		Message: "等待创作电脑开始连接", ExpiresAt: session.ExpiresAt,
	}
	switch session.State {
	case "verifying":
		view.Status = "connecting"
		view.Message = "正在完成本地环境检查和项目连接"
	case "connected":
		view.Message = "创作电脑已连接，可以开始新的创作任务"
	case "expired":
		view.Message = "本次连接已过期，请重新发起"
	case "canceled":
		view.Message = "本次连接已取消"
	case "failed":
		view.Message = "创作电脑连接失败，请重新发起"
	}
	if session.Progress == nil {
		return view
	}
	view.SupportCode = session.Progress.SupportCode
	if session.Progress.Status == "failed" {
		view.Status = "failed"
		view.Message = "连接未完成，请重新发起；需要协助时可提供支持编号"
		return view
	}
	if session.Progress.Stage == "authorizing" && session.Progress.Status == "needs_action" && session.Progress.UserCode != "" {
		view.Status = "confirmation_required"
		view.Message = "请核对 Codex 中显示的代码，再确认连接这台电脑"
		view.RequiresConfirmation = true
		view.VerificationCode = session.Progress.UserCode
		return view
	}
	if session.State != "connected" {
		view.Status = "connecting"
		view.Message = "创作电脑正在完成连接，请保持当前页面打开"
	}
	return view
}

func studioTaskSummary(task domain.WorkTask, project domain.Project, sop domain.SOPVersion) StudioTaskSummary {
	return StudioTaskSummary{ID: task.ID, Project: studioProject(project), ExperienceID: studioExperienceIDForSOP(sop), Title: task.Title, Intent: task.Intent, ContentType: task.ContentType, Status: task.Status, StatusLabel: studioTaskStatusLabel(task.Status), CurrentStepID: studioCustomerStepID(task.CurrentStageID, sop), NextAction: studioNextAction(task), AssetCount: len(task.InputRefs), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt}
}

func marketingStudioCustomerStepTemplates() []StudioCustomerStep {
	return []StudioCustomerStep{
		{ID: "inspiration", Title: "灵感采集", Outcome: "选出值得继续的人物方向和可信参考"},
		{ID: "persona", Title: "人物原型", Outcome: "确认人物定位、受众关系和表达边界"},
		{ID: "script", Title: "营销剧本", Outcome: "确认可制作的内容方向和剧本版本"},
		{ID: "storyboard", Title: "视频分镜", Outcome: "锁定镜头、画面、素材和连续性要求"},
		{ID: "media", Title: "候选成片", Outcome: "选择通过技术和内容检查的候选结果"},
		{ID: "delivery", Title: "交付准备", Outcome: "生成正式交付包并登记后续结果"},
	}
}

func customerStudioExperienceStepTitles(sop domain.SOPVersion) []string {
	if isMarketingStudioSOP(sop) {
		templates := marketingStudioCustomerStepTemplates()
		result := make([]string, 0, len(templates))
		for _, template := range templates {
			result = append(result, template.Title)
		}
		return result
	}
	result := make([]string, 0, len(sop.Stages))
	for _, stage := range sop.Stages {
		result = append(result, defaultString(strings.TrimSpace(stage.Name), "创作步骤"))
	}
	return result
}

func studioCustomerSteps(task domain.WorkTask, sop domain.SOPVersion) []StudioCustomerStep {
	templates := marketingStudioCustomerStepTemplates()
	if !isMarketingStudioSOP(sop) {
		templates = make([]StudioCustomerStep, 0, len(sop.Stages))
		for index, stage := range sop.Stages {
			id := strings.TrimSpace(stage.ID)
			if id == "" {
				id = fmt.Sprintf("stage-%d", index+1)
			}
			templates = append(templates, StudioCustomerStep{ID: id, Title: defaultString(strings.TrimSpace(stage.Name), fmt.Sprintf("第 %d 步", index+1)), Outcome: "完成本步骤并确认可继续的结果"})
		}
	}
	if len(templates) == 0 {
		templates = append(templates, StudioCustomerStep{ID: "inspiration", Title: "创作准备", Outcome: "补充本次创作需要的目标和资料"})
	}
	current := studioCustomerStepID(task.CurrentStageID, sop)
	currentIndex := 0
	for index := range templates {
		if templates[index].ID == current {
			currentIndex = index
		}
	}
	for index := range templates {
		status := "not_started"
		summary := "等待前序结果确认"
		if task.Status == domain.TaskStatusDelivered || index < currentIndex {
			status, summary = "completed", "本步骤结果已经确认"
		} else if index == currentIndex {
			switch task.Status {
			case domain.TaskStatusNeedsInput:
				status, summary = "needs_input", "还需要补充创作资料"
			case domain.TaskStatusWaitingGate:
				status, summary = "needs_decision", "当前结果等待你确认"
			case domain.TaskStatusBlocked:
				status, summary = "blocked", "当前步骤需要协助"
			case domain.TaskStatusRunning:
				status, summary = "working", "创作流水线正在处理"
			case domain.TaskStatusPaused:
				status, summary = "ready", "任务已暂停，可以继续"
			default:
				status, summary = "ready", "可以开始当前步骤"
			}
		}
		templates[index].Status = status
		templates[index].ProgressSummary = summary
	}
	return templates
}

func studioCustomerStepID(stageID string, sop domain.SOPVersion) string {
	if strings.TrimSpace(stageID) == "" && len(sop.Stages) > 0 {
		stageID = sop.Stages[0].ID
	}
	normalized := strings.ToLower(stageID)
	groups := []struct {
		id     string
		values []string
	}{
		{"inspiration", []string{"onboarding", "methodology", "context", "source", "research", "inspiration", "intelligence", "brief"}},
		{"persona", []string{"knowledge", "audience", "persona", "strategy", "planning"}},
		{"script", []string{"creative", "script", "content", "draft"}},
		{"storyboard", []string{"storyboard"}},
		{"media", []string{"generation", "media", "review", "postproduction", "render", "quality"}},
		{"delivery", []string{"delivery", "learning", "handoff", "automation"}},
	}
	for _, group := range groups {
		for _, value := range group.values {
			if normalized == value || strings.Contains(normalized, value) {
				return group.id
			}
		}
	}
	if strings.TrimSpace(stageID) != "" {
		return stageID
	}
	return "inspiration"
}

func studioExperienceIDForSOP(sop domain.SOPVersion) string {
	if isMarketingStudioSOP(sop) {
		return studioExperienceIPVideoID
	}
	return "sop:" + sop.SOPID + ":v" + strconv.Itoa(sop.Version)
}

func isMarketingStudioSOP(sop domain.SOPVersion) bool {
	return sop.SOPID == "builtin-sop-marketing-video" || sop.SOPID == builtinSOPMarketingVideo
}

func customerStudioAllowedActions(actor Actor, view WorkTaskView) []string {
	if !canCreateStudioTask(actor.Role) {
		return []string{}
	}
	allowed := []string{}
	for _, action := range view.AllowedActions {
		switch action {
		case "start", "resume", "retry", "pause":
			allowed = append(allowed, action)
		}
	}
	return allowed
}

func studioTaskStatusLabel(status string) string {
	return map[string]string{domain.TaskStatusNeedsInput: "需要补充资料", domain.TaskStatusReady: "可以开始", domain.TaskStatusRunning: "正在创作", domain.TaskStatusPaused: "已暂停", domain.TaskStatusWaitingGate: "等待你确认", domain.TaskStatusBlocked: "需要协助", domain.TaskStatusAccepted: "准备交付", domain.TaskStatusDelivered: "已完成", domain.TaskStatusCancelled: "已取消"}[status]
}

func studioNextAction(task domain.WorkTask) string {
	switch task.Status {
	case domain.TaskStatusWaitingGate:
		return "查看并确认当前结果"
	case domain.TaskStatusNeedsInput:
		return "补充缺少的资料"
	case domain.TaskStatusBlocked:
		return "查看需要协助的原因"
	case domain.TaskStatusDelivered:
		return "查看交付成果"
	case domain.TaskStatusReady:
		return "开始创作"
	case domain.TaskStatusPaused:
		return "继续创作"
	}
	return defaultString(strings.TrimSpace(task.NextAction), "查看当前进度")
}

func studioStepTitle(id string) string {
	return map[string]string{"inspiration": "灵感采集", "persona": "人物原型", "script": "营销剧本", "storyboard": "视频分镜", "media": "候选成片", "delivery": "交付准备"}[id]
}

func canCreateStudioTask(role string) bool {
	return containsString([]string{"tenant_admin", "project_manager", "strategist", "editor"}, role)
}

func canDecideCustomerStudioGate(role, mode string) bool {
	if role == "tenant_admin" {
		return true
	}
	if mode == domain.GateModeClientDecision {
		return role == "client_approver"
	}
	return role == "project_manager" || role == "reviewer"
}

func inputSourceLabel(value string) string {
	return map[string]string{"brief": "创作简报", "manual_inspiration": "人工灵感", "workspace_file": "本地资料", "conversation_bundle": "本地创作工具"}[value]
}

func sourceTypeLabel(value string) string {
	if label := map[string]string{"document": "文档资料", "brand_manual": "品牌资料", "website": "网页来源", "api": "公开数据"}[value]; label != "" {
		return label
	}
	return "来源资料"
}

func assetTypeLabel(value string) string {
	if label := map[string]string{"product_image": "产品图片", "brand_mark": "品牌标志", "packaging": "包装素材", "person": "人物素材", "location": "场景素材", "audio": "音频素材"}[value]; label != "" {
		return label
	}
	return "创作素材"
}

func knowledgeCategory(objectType string) string {
	if objectType == "Audience" || objectType == "Scenario" || objectType == "Insight" {
		return "persona"
	}
	if objectType == "Asset" {
		return "media"
	}
	return "knowledge"
}

func snapshotTypeLabel(value string) string {
	if label := map[string]string{"storyboard": "已确认分镜", "content_batch": "已确认内容", "marketing_video": "已确认营销视频", "video_script": "已确认剧本"}[value]; label != "" {
		return label
	}
	return "已确认创作成果"
}

func taskRevisionTitle(value domain.TaskRevision) string {
	var content map[string]any
	if json.Unmarshal(value.Content, &content) == nil {
		if title, _ := content["title"].(string); strings.TrimSpace(title) != "" {
			return title
		}
	}
	return fmt.Sprintf("第 %d 版内容", value.RevisionNo)
}

func studioDownload(artifact domain.Artifact) StudioDownload {
	return StudioDownload{ID: artifact.ID, FileName: artifact.FileName, MediaType: artifact.MediaType, ByteSize: artifact.ByteSize, Href: "/api/studio/artifacts/" + artifact.ID + "/download"}
}

func containsStringPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
