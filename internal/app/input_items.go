package app

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type CreateInputItemInput struct {
	ProjectID      string         `json:"project_id"`
	SourceType     string         `json:"source_type"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	Body           string         `json:"body"`
	SourceRef      string         `json:"source_ref"`
	SourceDigest   string         `json:"source_digest"`
	Disclosure     string         `json:"disclosure"`
	Metadata       map[string]any `json:"metadata"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type InputItemQuery struct {
	ProjectID      string `json:"project_id"`
	Status         string `json:"status"`
	AssigneeUserID string `json:"assignee_user_id"`
	Mine           bool   `json:"mine"`
}

type TriageInputItemInput struct {
	Action          string         `json:"action"`
	ExpectedVersion int            `json:"expected_version"`
	ProjectID       string         `json:"project_id"`
	TaskID          string         `json:"task_id"`
	AssigneeUserID  string         `json:"assignee_user_id"`
	MissingFields   []string       `json:"missing_fields"`
	Title           string         `json:"title"`
	Intent          string         `json:"intent"`
	ContentType     string         `json:"content_type"`
	Priority        string         `json:"priority"`
	RiskProfile     string         `json:"risk_profile"`
	RequestedOutput map[string]any `json:"requested_output"`
}

func (s *Service) CreateInputItem(ctx context.Context, actor Actor, input CreateInputItemInput, requestID string) (domain.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return domain.InputItem{}, err
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key != "" {
		if existing, err := s.store.InputItemByIdempotencyKey(ctx, actor.TenantID, key); err == nil {
			return existing, nil
		}
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID != "" {
		if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
			return domain.InputItem{}, err
		}
	}
	disclosure := defaultString(strings.TrimSpace(input.Disclosure), "project")
	if projectID == "" && disclosure == "project" {
		disclosure = "tenant"
	}
	now := s.now().UTC()
	value := domain.InputItem{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: projectID, SourceType: strings.TrimSpace(input.SourceType), Title: strings.TrimSpace(input.Title), Summary: strings.TrimSpace(input.Summary), Body: input.Body, SourceRef: strings.TrimSpace(input.SourceRef), SourceDigest: strings.TrimSpace(input.SourceDigest), Disclosure: disclosure, Status: domain.InputItemUntriaged, Metadata: input.Metadata, IdempotencyKey: key, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return domain.InputItem{}, err
	}
	if err := s.store.CreateInputItem(ctx, value); err != nil {
		if key != "" {
			if existing, lookupErr := s.store.InputItemByIdempotencyKey(ctx, actor.TenantID, key); lookupErr == nil {
				return existing, nil
			}
		}
		return domain.InputItem{}, err
	}
	s.audit(ctx, actor, projectID, "input_item.created", "input_item", value.ID, requestID, map[string]any{"source_type": value.SourceType, "status": value.Status})
	return value, nil
}

func (s *Service) InputItems(ctx context.Context, actor Actor, query InputItemQuery) ([]domain.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil {
		return nil, err
	}
	if query.Mine {
		query.AssigneeUserID = actor.UserID
	}
	return s.store.InputItems(ctx, actor.TenantID, strings.TrimSpace(query.ProjectID), strings.TrimSpace(query.Status), strings.TrimSpace(query.AssigneeUserID))
}

func (s *Service) InputItem(ctx context.Context, actor Actor, id string) (domain.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil {
		return domain.InputItem{}, err
	}
	return s.store.InputItem(ctx, actor.TenantID, id)
}

func (s *Service) TriageInputItem(ctx context.Context, actor Actor, id string, input TriageInputItemInput, requestID string) (domain.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return domain.InputItem{}, err
	}
	value, err := s.store.InputItem(ctx, actor.TenantID, id)
	if err != nil {
		return domain.InputItem{}, err
	}
	if value.Status == domain.InputItemArchived {
		return domain.InputItem{}, domain.Conflict("INPUT_ITEM_ARCHIVED", "已归档的输入收集记录不可再次分流")
	}
	expectedVersion := input.ExpectedVersion
	if expectedVersion == 0 {
		expectedVersion = value.RowVersion
	}
	if value.RowVersion != expectedVersion {
		return domain.InputItem{}, domain.Conflict("INPUT_ITEM_VERSION_CONFLICT", "输入收集记录已被其他人更新，请刷新后重试")
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case "mark_missing":
		if len(input.MissingFields) == 0 {
			return domain.InputItem{}, domain.Invalid("INPUT_ITEM_MISSING_FIELDS_REQUIRED", "标记缺少信息时必须填写缺口")
		}
		value.Status = domain.InputItemNeedsInfo
		value.MissingFields = uniqueNonEmpty(input.MissingFields)
	case "route_owner":
		if strings.TrimSpace(input.AssigneeUserID) == "" {
			return domain.InputItem{}, domain.Invalid("INPUT_ITEM_ASSIGNEE_REQUIRED", "转给流程负责人时必须指定负责人")
		}
		value.AssigneeUserID = strings.TrimSpace(input.AssigneeUserID)
		value.Status = domain.InputItemRouted
	case "archive_project":
		projectID := defaultString(strings.TrimSpace(input.ProjectID), value.ProjectID)
		if projectID == "" {
			return domain.InputItem{}, domain.Invalid("INPUT_ITEM_PROJECT_REQUIRED", "归档为项目资料时必须指定项目")
		}
		if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
			return domain.InputItem{}, err
		}
		value.ProjectID = projectID
		value.Status = domain.InputItemProjectMaterial
	case "archive":
		value.Status = domain.InputItemArchived
	case "merge_task":
		taskID := defaultString(strings.TrimSpace(input.TaskID), value.TargetTaskID)
		if taskID == "" {
			return domain.InputItem{}, domain.Invalid("INPUT_ITEM_TASK_REQUIRED", "合并输入时必须指定已有任务")
		}
		task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
		if err != nil {
			return domain.InputItem{}, err
		}
		if value.ProjectID != "" && task.ProjectID != value.ProjectID {
			return domain.InputItem{}, domain.Policy("INPUT_ITEM_PROJECT_MISMATCH", "输入和目标任务不属于同一项目", "选择同一项目下的任务")
		}
		inputRef := "input:" + value.ID
		if !containsStringPrefix(task.InputRefs, inputRef+"@") {
			task.InputRefs = appendUnique(task.InputRefs, inputRef)
		}
		if task.Status == domain.TaskStatusNeedsInput {
			task.Status = domain.TaskStatusReady
			task.NextAction = "开始第一个流程阶段"
		}
		task.UpdatedAt = s.now().UTC()
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return domain.InputItem{}, err
		}
		value.ProjectID = task.ProjectID
		value.TargetTaskID = task.ID
		value.Status = domain.InputItemTaskMerged
	case "create_task":
		projectID := defaultString(strings.TrimSpace(input.ProjectID), value.ProjectID)
		if projectID == "" {
			return domain.InputItem{}, domain.Invalid("INPUT_ITEM_PROJECT_REQUIRED", "从输入创建任务时必须指定项目")
		}
		project, err := s.store.Project(ctx, actor.TenantID, projectID)
		if err != nil {
			return domain.InputItem{}, err
		}
		if value.ProjectID != "" && value.ProjectID != projectID {
			return domain.InputItem{}, domain.Policy("INPUT_ITEM_PROJECT_MISMATCH", "输入和目标任务不属于同一项目", "选择输入所属项目下的任务")
		}
		if value.Status == domain.InputItemTaskCreated && value.TargetTaskID != "" {
			return s.store.InputItem(ctx, actor.TenantID, value.ID)
		}
		title := defaultString(strings.TrimSpace(input.Title), value.Title)
		created, err := s.CreateWorkTask(ctx, actor, CreateWorkTaskInput{ProjectID: projectID, Title: title, Intent: defaultString(strings.TrimSpace(input.Intent), value.Summary), ContentType: defaultString(strings.TrimSpace(input.ContentType), defaultString(project.ContentType, domain.DefaultProjectContentType)), InputRefs: []string{"input:" + value.ID}, Priority: input.Priority, RiskProfile: input.RiskProfile, RequestedOutput: input.RequestedOutput}, requestID)
		if err != nil {
			return domain.InputItem{}, err
		}
		value.ProjectID = created.Task.ProjectID
		value.TargetTaskID = created.Task.ID
		value.Status = domain.InputItemTaskCreated
	default:
		return domain.InputItem{}, domain.Invalid("INPUT_ITEM_ACTION_INVALID", "不支持的输入收集分流动作")
	}
	value.RowVersion = expectedVersion + 1
	value.UpdatedAt = s.now().UTC()
	if err := s.store.SaveInputItem(ctx, value, expectedVersion); err != nil {
		return domain.InputItem{}, err
	}
	s.audit(ctx, actor, value.ProjectID, "input_item.triaged", "input_item", value.ID, requestID, map[string]any{"action": action, "status": value.Status, "target_task_id": value.TargetTaskID, "row_version": value.RowVersion})
	return value, nil
}

func appendUnique(values []string, value string) []string {
	for _, candidate := range values {
		if candidate == value {
			return values
		}
	}
	return append(append([]string{}, values...), value)
}
