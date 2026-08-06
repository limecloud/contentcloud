package localworkspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"gopkg.in/yaml.v3"
)

const ProjectBriefSchemaVersion = "contentcloud.project-brief/1.0"

const (
	OnboardingNeedsProjectBrief  = "needs_project_brief"
	OnboardingNeedsSourceIntake  = "needs_source_intake"
	OnboardingNeedsSourceIngest  = "needs_source_ingest"
	OnboardingReadyForKnowledge  = "ready_for_knowledge"
	OnboardingReadyForBrief      = "ready_for_brief"
	OnboardingReadyForProduction = "ready_for_production"
	OnboardingContinueRun        = "continue_run"
	OnboardingHandoffReady       = "handoff_ready"
	OnboardingReviewReady        = "review_ready"
)

type WorkspaceInputRequirement struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type WorkspaceWorkflowStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Capability  string `json:"capability,omitempty"`
}

type WorkspaceOnboarding struct {
	State           string                      `json:"state"`
	Title           string                      `json:"title"`
	Summary         string                      `json:"summary"`
	NextStep        WorkspaceWorkflowStep       `json:"next_step"`
	RequiredInputs  []WorkspaceInputRequirement `json:"required_inputs"`
	CompletedInputs []string                    `json:"completed_inputs"`
	Workflow        []WorkspaceWorkflowStep     `json:"workflow"`
}

type ProjectBrief struct {
	SchemaVersion    string    `json:"schema_version" yaml:"schema_version"`
	Status           string    `json:"status" yaml:"status"`
	Client           string    `json:"client" yaml:"client"`
	Brand            string    `json:"brand" yaml:"brand"`
	ProductOrService string    `json:"product_or_service" yaml:"product_or_service"`
	Objective        string    `json:"objective" yaml:"objective"`
	Channels         []string  `json:"channels" yaml:"channels"`
	Audience         string    `json:"audience" yaml:"audience"`
	MaterialRefs     []string  `json:"material_refs" yaml:"material_refs"`
	Notes            string    `json:"notes,omitempty" yaml:"notes,omitempty"`
	ConfirmedAt      time.Time `json:"confirmed_at" yaml:"confirmed_at"`
	UpdatedAt        time.Time `json:"updated_at" yaml:"updated_at"`
}

type SaveProjectBriefOptions struct {
	Root             string
	Client           string
	Brand            string
	ProductOrService string
	Objective        string
	Channels         []string
	Audience         string
	MaterialRefs     []string
	Notes            string
	Confirm          bool
	Now              time.Time
}

func ProjectBriefPath(root string) string {
	return filepath.Join(root, "10-context", "project-brief.yaml")
}

func LoadProjectBrief(root string) (*ProjectBrief, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	var brief ProjectBrief
	err = readYAML(ProjectBriefPath(resolved), &brief)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, domain.Invalid("PROJECT_BRIEF_INVALID", "项目简报文件无法读取")
	}
	if brief.SchemaVersion == ProjectBriefSchemaVersion && brief.Status == "draft" {
		return nil, nil
	}
	if brief.SchemaVersion != ProjectBriefSchemaVersion || brief.Status != "confirmed" || strings.TrimSpace(brief.Client) == "" || strings.TrimSpace(brief.Brand) == "" || strings.TrimSpace(brief.ProductOrService) == "" || strings.TrimSpace(brief.Objective) == "" || strings.TrimSpace(brief.Audience) == "" || len(brief.Channels) == 0 || brief.ConfirmedAt.IsZero() || brief.UpdatedAt.IsZero() {
		return nil, domain.Invalid("PROJECT_BRIEF_INVALID", "项目简报必须先完成确认，并包含客户、品牌、产品或服务、目标、渠道和受众")
	}
	brief.Channels = uniqueNonEmpty(brief.Channels)
	brief.MaterialRefs = uniqueNonEmpty(brief.MaterialRefs)
	return &brief, nil
}

func SaveProjectBrief(options SaveProjectBriefOptions) (ProjectBrief, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return ProjectBrief{}, err
	}
	if !options.Confirm {
		return ProjectBrief{}, domain.Conflict("PROJECT_BRIEF_CONFIRMATION_REQUIRED", "保存项目简报前必须确认已核对内容")
	}
	brief := ProjectBrief{
		SchemaVersion:    ProjectBriefSchemaVersion,
		Status:           "confirmed",
		Client:           strings.TrimSpace(options.Client),
		Brand:            strings.TrimSpace(options.Brand),
		ProductOrService: strings.TrimSpace(options.ProductOrService),
		Objective:        strings.TrimSpace(options.Objective),
		Channels:         uniqueNonEmpty(options.Channels),
		Audience:         strings.TrimSpace(options.Audience),
		MaterialRefs:     uniqueNonEmpty(options.MaterialRefs),
		Notes:            strings.TrimSpace(options.Notes),
	}
	if brief.Client == "" || brief.Brand == "" || brief.ProductOrService == "" || brief.Objective == "" || brief.Audience == "" || len(brief.Channels) == 0 {
		return ProjectBrief{}, domain.Invalid("PROJECT_BRIEF_REQUIRED_FIELDS", "项目简报需要客户、品牌、产品或服务、目标、渠道和受众")
	}
	if len(brief.Client) > 300 || len(brief.Brand) > 300 || len(brief.ProductOrService) > 1000 || len(brief.Objective) > 2000 || len(brief.Audience) > 1000 || len(brief.Notes) > 5000 {
		return ProjectBrief{}, domain.Invalid("PROJECT_BRIEF_TOO_LARGE", "项目简报字段超过允许长度")
	}
	for _, channel := range brief.Channels {
		if len(channel) > 100 {
			return ProjectBrief{}, domain.Invalid("PROJECT_BRIEF_CHANNEL_INVALID", "内容渠道名称过长")
		}
	}
	now := localNow(options.Now)
	existing, loadErr := LoadProjectBrief(root)
	if loadErr != nil {
		return ProjectBrief{}, loadErr
	}
	if existing != nil {
		brief.ConfirmedAt = existing.ConfirmedAt
	} else {
		brief.ConfirmedAt = now
	}
	brief.UpdatedAt = now
	if err := replaceYAML(ProjectBriefPath(root), brief); err != nil {
		return ProjectBrief{}, err
	}
	if err := syncContextFiles(root, brief); err != nil {
		return ProjectBrief{}, err
	}
	return brief, nil
}

func syncContextFiles(root string, brief ProjectBrief) error {
	clientPath := filepath.Join(root, "10-context", "client.yaml")
	client, err := loadYAMLMap(clientPath)
	if err != nil {
		return fmt.Errorf("读取客户上下文失败：%w", err)
	}
	client["schema_version"] = "contentcloud.client-context/3.0"
	client["client_name"] = brief.Client
	client["brand_name"] = brief.Brand
	client["brand_refs"] = []string{"brand:" + stableSlug(brief.Brand)}
	client["product_name"] = brief.ProductOrService
	client["product_refs"] = []string{"product:" + stableSlug(brief.ProductOrService)}
	if err := replaceYAML(clientPath, client); err != nil {
		return err
	}

	projectPath := filepath.Join(root, "10-context", "project.yaml")
	project, err := loadYAMLMap(projectPath)
	if err != nil {
		return fmt.Errorf("读取项目上下文失败：%w", err)
	}
	project["schema_version"] = "contentcloud.project-context/3.0"
	project["stage"] = "brief"
	project["gate"] = "brief_confirmed"
	project["objectives"] = []string{brief.Objective}
	project["channels"] = brief.Channels
	project["audience"] = brief.Audience
	project["brief_ref"] = "10-context/project-brief.yaml"
	if err := replaceYAML(projectPath, project); err != nil {
		return err
	}

	servicePath := filepath.Join(root, "10-context", "service-plan.yaml")
	service, err := loadYAMLMap(servicePath)
	if err != nil {
		return fmt.Errorf("读取服务方案失败：%w", err)
	}
	service["schema_version"] = "contentcloud.service-plan/3.0"
	service["phase"] = "brief"
	service["sop_mode"] = "configurable"
	service["approval_policy"] = "optional"
	if values, ok := service["deliverables"].([]any); !ok || len(values) == 0 {
		service["deliverables"] = []string{"项目简报", "素材知识库", "内容交付"}
	}
	if err := replaceYAML(servicePath, service); err != nil {
		return err
	}
	return nil
}

func DeriveWorkspaceOnboarding(root string, status Status, runs []WorkspaceRunSummary, handoffs []WorkspaceHandoffSummary, approved []string) (WorkspaceOnboarding, error) {
	brief, err := LoadProjectBrief(root)
	if err != nil {
		return WorkspaceOnboarding{}, err
	}
	required := projectBriefInputs()
	completed := []string{}
	if brief != nil {
		for _, input := range required {
			if input.ID == "material_refs" && len(brief.MaterialRefs) == 0 {
				continue
			}
			completed = append(completed, input.ID)
		}
	}
	workflow := []WorkspaceWorkflowStep{
		{ID: "project_brief", Title: "建立项目简报", Description: "确认客户、内容目标和受众，形成后续工作的共同上下文。", Status: "pending", Capability: "workspace_project_brief"},
		{ID: "source_intake", Title: "接入已有素材", Description: "登记本地原件并保留来源、哈希和权限边界。", Status: "pending", Capability: "source_register"},
		{ID: "knowledge", Title: "提取项目知识", Description: "解析素材证据，整理为可追溯的知识页面。", Status: "pending", Capability: "knowledge_extraction"},
		{ID: "brief", Title: "形成内容简报", Description: "基于项目简报和可用知识确定具体内容方向。", Status: "pending", Capability: "brief"},
		{ID: "production", Title: "生产与交付", Description: "生成内容候选，按项目配置的 SOP 检查后交付。", Status: "pending", Capability: "marketing_video_script"},
	}
	if brief == nil {
		workflow[0].Status = "next"
		return WorkspaceOnboarding{State: OnboardingNeedsProjectBrief, Title: "先建立项目简报", Summary: "当前工作区已连接，但还没有业务上下文。先确认一次项目简报，后续素材、知识和内容工作会沿着同一条链路推进。", NextStep: workflow[0], RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	workflow[0].Status = "completed"
	if len(runs) > 0 {
		workflow[1].Status = "in_progress"
		return WorkspaceOnboarding{State: OnboardingContinueRun, Title: "继续当前工作", Summary: "已有进行中的本地任务，先选择一个任务继续，避免并行修改同一份业务状态。", NextStep: WorkspaceWorkflowStep{ID: "continue_run", Title: "选择并继续任务", Description: "查看进行中的任务、阶段和写入占用，再决定是否继续。", Status: "next", Capability: "local_run_show"}, RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	if len(handoffs) > 0 {
		workflow[1].Status = "in_progress"
		return WorkspaceOnboarding{State: OnboardingHandoffReady, Title: "接收待处理交接", Summary: "上一个对话已经留下可验证的工作交接，接收后再继续业务流程。", NextStep: WorkspaceWorkflowStep{ID: "handoff", Title: "接收工作交接", Description: "核对交接内容和版本后接管任务。", Status: "next", Capability: "handoff_accept"}, RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	sources, err := LocalSources(root)
	if err != nil {
		return WorkspaceOnboarding{}, err
	}
	if len(sources) == 0 {
		workflow[1].Status = "next"
		return WorkspaceOnboarding{State: OnboardingNeedsSourceIntake, Title: "接入已有素材", Summary: "项目简报已经确认。下一步登记本地素材，原件默认留在本机，不会自动上传。", NextStep: workflow[1], RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	workflow[1].Status = "completed"
	for _, source := range sources {
		if source.IngestStatus != "ready" {
			workflow[1].Status = "next"
			title := "解析已登记素材"
			summary := "已有素材还没有生成可定位证据。先完成解析，知识提取才有可靠输入。"
			if source.IngestStatus != "registered" {
				title = "复核素材解析"
				summary = "部分素材解析结果需要复核，先处理证据状态，再进入知识提取。"
			}
			return WorkspaceOnboarding{State: OnboardingNeedsSourceIngest, Title: title, Summary: summary, NextStep: WorkspaceWorkflowStep{ID: "source_ingest", Title: "解析并复核素材证据", Description: "对登记来源生成或复核本地证据包，不上传原件。", Status: "next", Capability: "source_ingest"}, RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
		}
	}
	if len(approved) == 0 && countFiles(filepath.Join(root, "30-knowledge", "pages")) == 0 {
		workflow[2].Status = "next"
		return WorkspaceOnboarding{State: OnboardingReadyForKnowledge, Title: "提取项目知识", Summary: "素材已经登记并完成解析。现在可以从证据中整理知识页面，之后再进入内容简报。", NextStep: workflow[2], RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	workflow[2].Status = "completed"
	if status.PendingFeedbackCount > 0 {
		return WorkspaceOnboarding{State: OnboardingReviewReady, Title: "处理审核反馈", Summary: "工作区有待处理审核反馈，先核对反馈和对应版本，再决定是否开始修订。", NextStep: WorkspaceWorkflowStep{ID: "review", Title: "查看审核反馈", Description: "读取本地反馈并核对目标版本，修订前不自动写入。", Status: "next", Capability: "review_feedback_inbox"}, RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	if !hasLocalBrief(root) {
		workflow[3].Status = "next"
		return WorkspaceOnboarding{State: OnboardingReadyForBrief, Title: "形成内容简报", Summary: "项目知识已经具备。下一步明确本次内容主题、受众承诺和表达方向，再进入生产。", NextStep: workflow[3], RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
	}
	workflow[3].Status = "completed"
	workflow[4].Status = "next"
	return WorkspaceOnboarding{State: OnboardingReadyForProduction, Title: "进入内容生产", Summary: "项目上下文和知识快照已具备，可以选择内容方向并开始生产。审核和发布是否必需由项目 SOP 配置决定。", NextStep: workflow[4], RequiredInputs: required, CompletedInputs: completed, Workflow: workflow}, nil
}

func hasLocalBrief(root string) bool {
	paths, err := filepath.Glob(filepath.Join(root, "50-production", "briefs", "*.json"))
	if err != nil {
		return false
	}
	for _, path := range paths {
		var identity struct {
			Kind           string `json:"kind"`
			Status         string `json:"status"`
			SchemaVersion  string `json:"schema_version"`
			Deliverability string `json:"deliverability"`
		}
		if err := readJSON(path, &identity); err != nil {
			continue
		}
		if identity.Kind == "brief" && identity.SchemaVersion == BriefSchema && identity.Status == "candidate" && identity.Deliverability == "review_ready" {
			return true
		}
		if identity.Kind == "article_brief" && identity.SchemaVersion == ArticleBriefSchema && identity.Status == "candidate" && identity.Deliverability == "review_ready" {
			return true
		}
	}
	return false
}

func projectBriefInputs() []WorkspaceInputRequirement {
	return []WorkspaceInputRequirement{
		{ID: "client", Label: "客户", Description: "客户或组织名称。", Required: true},
		{ID: "brand", Label: "品牌", Description: "本次内容对应的品牌名称。", Required: true},
		{ID: "product_or_service", Label: "产品或服务", Description: "要被介绍、推广或解释的产品或服务。", Required: true},
		{ID: "objective", Label: "本次内容目标", Description: "希望内容完成什么业务目标。", Required: true},
		{ID: "channels", Label: "目标渠道", Description: "例如抖音、视频号、官网或内部培训。", Required: true},
		{ID: "audience", Label: "目标受众", Description: "谁会看到或使用这次内容。", Required: true},
		{ID: "material_refs", Label: "已有素材位置", Description: "可选；填写本地文件或目录位置，后续在素材接入步骤中登记。", Required: false},
	}
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stableSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	original := value
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "local-" + digest([]byte(original))[:12]
	}
	return result
}

func loadYAMLMap(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := yaml.Unmarshal(body, &value); err != nil {
		return nil, err
	}
	return value, nil
}
