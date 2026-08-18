package application

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/work"
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

func (s *WorkService) CreateInputItem(ctx context.Context, actor Actor, input CreateInputItemInput, requestID string) (work.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return work.InputItem{}, err
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key != "" {
		if existing, err := s.tasks.InputItemByIdempotencyKey(ctx, actor.TenantID, key); err == nil {
			return existing, nil
		}
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID != "" {
		if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
			return work.InputItem{}, err
		}
	}
	disclosure := defaultString(strings.TrimSpace(input.Disclosure), "project")
	if projectID == "" && disclosure == "project" {
		disclosure = "tenant"
	}
	now := s.now().UTC()
	value := work.InputItem{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: projectID, SourceType: strings.TrimSpace(input.SourceType), Title: strings.TrimSpace(input.Title), Summary: strings.TrimSpace(input.Summary), Body: input.Body, SourceRef: strings.TrimSpace(input.SourceRef), SourceDigest: strings.TrimSpace(input.SourceDigest), Disclosure: disclosure, Status: work.InputItemUntriaged, Metadata: input.Metadata, IdempotencyKey: key, RowVersion: 1, CreatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return work.InputItem{}, err
	}
	if err := s.tasks.CreateInputItem(ctx, value); err != nil {
		if key != "" {
			if existing, lookupErr := s.tasks.InputItemByIdempotencyKey(ctx, actor.TenantID, key); lookupErr == nil {
				return existing, nil
			}
		}
		return work.InputItem{}, err
	}
	s.audit(ctx, actor, projectID, "input_item.created", "input_item", value.ID, requestID, map[string]any{"source_type": value.SourceType, "status": value.Status})
	return value, nil
}

func (s *WorkService) InputItems(ctx context.Context, actor Actor, query InputItemQuery) ([]work.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil {
		return nil, err
	}
	if query.Mine {
		query.AssigneeUserID = actor.UserID
	}
	return s.tasks.InputItems(ctx, actor.TenantID, strings.TrimSpace(query.ProjectID), strings.TrimSpace(query.Status), strings.TrimSpace(query.AssigneeUserID))
}

func (s *WorkService) InputItem(ctx context.Context, actor Actor, id string) (work.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "viewer"); err != nil {
		return work.InputItem{}, err
	}
	return s.tasks.InputItem(ctx, actor.TenantID, id)
}

func (s *WorkService) TriageInputItem(ctx context.Context, actor Actor, id string, input TriageInputItemInput, requestID string) (work.InputItem, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return work.InputItem{}, err
	}
	value, err := s.tasks.InputItem(ctx, actor.TenantID, id)
	if err != nil {
		return work.InputItem{}, err
	}
	if value.Status == work.InputItemArchived {
		return work.InputItem{}, fault.Conflict("INPUT_ITEM_ARCHIVED", "已归档的输入收集记录不可再次分流")
	}
	expectedVersion := input.ExpectedVersion
	if expectedVersion == 0 {
		expectedVersion = value.RowVersion
	}
	if value.RowVersion != expectedVersion {
		return work.InputItem{}, fault.Conflict("INPUT_ITEM_VERSION_CONFLICT", "输入收集记录已被其他人更新，请刷新后重试")
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case "mark_missing":
		if len(input.MissingFields) == 0 {
			return work.InputItem{}, fault.Invalid("INPUT_ITEM_MISSING_FIELDS_REQUIRED", "标记缺少信息时必须填写缺口")
		}
		value.Status = work.InputItemNeedsInfo
		value.MissingFields = uniqueNonEmpty(input.MissingFields)
	case "route_owner":
		if strings.TrimSpace(input.AssigneeUserID) == "" {
			return work.InputItem{}, fault.Invalid("INPUT_ITEM_ASSIGNEE_REQUIRED", "转给流程负责人时必须指定负责人")
		}
		value.AssigneeUserID = strings.TrimSpace(input.AssigneeUserID)
		value.Status = work.InputItemRouted
	case "archive_project":
		projectID := defaultString(strings.TrimSpace(input.ProjectID), value.ProjectID)
		if projectID == "" {
			return work.InputItem{}, fault.Invalid("INPUT_ITEM_PROJECT_REQUIRED", "归档为项目资料时必须指定项目")
		}
		if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
			return work.InputItem{}, err
		}
		value.ProjectID = projectID
		value.Status = work.InputItemProjectMaterial
	case "archive":
		value.Status = work.InputItemArchived
	case "merge_task":
		taskID := defaultString(strings.TrimSpace(input.TaskID), value.TargetTaskID)
		if taskID == "" {
			return work.InputItem{}, fault.Invalid("INPUT_ITEM_TASK_REQUIRED", "合并输入时必须指定已有任务")
		}
		task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
		if err != nil {
			return work.InputItem{}, err
		}
		if value.ProjectID != "" && task.ProjectID != value.ProjectID {
			return work.InputItem{}, fault.Policy("INPUT_ITEM_PROJECT_MISMATCH", "输入和目标任务不属于同一项目", "选择同一项目下的任务")
		}
		inputRef := "input:" + value.ID
		if !containsStringPrefix(task.InputRefs, inputRef+"@") {
			task.InputRefs = appendUnique(task.InputRefs, inputRef)
		}
		if task.Status == work.TaskStatusNeedsInput {
			task.Status = work.TaskStatusReady
			task.NextAction = "开始第一个流程阶段"
		}
		task.UpdatedAt = s.now().UTC()
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return work.InputItem{}, err
		}
		value.ProjectID = task.ProjectID
		value.TargetTaskID = task.ID
		value.Status = work.InputItemTaskMerged
	case "create_task":
		projectID := defaultString(strings.TrimSpace(input.ProjectID), value.ProjectID)
		if projectID == "" {
			return work.InputItem{}, fault.Invalid("INPUT_ITEM_PROJECT_REQUIRED", "从输入创建任务时必须指定项目")
		}
		project, err := s.workspace.Project(ctx, actor.TenantID, projectID)
		if err != nil {
			return work.InputItem{}, err
		}
		if value.ProjectID != "" && value.ProjectID != projectID {
			return work.InputItem{}, fault.Policy("INPUT_ITEM_PROJECT_MISMATCH", "输入和目标任务不属于同一项目", "选择输入所属项目下的任务")
		}
		if value.Status == work.InputItemTaskCreated && value.TargetTaskID != "" {
			return s.tasks.InputItem(ctx, actor.TenantID, value.ID)
		}
		title := defaultString(strings.TrimSpace(input.Title), value.Title)
		created, err := s.CreateWorkTask(ctx, actor, CreateWorkTaskInput{ProjectID: projectID, Title: title, Intent: defaultString(strings.TrimSpace(input.Intent), value.Summary), ContentType: defaultString(strings.TrimSpace(input.ContentType), defaultString(project.ContentType, identitydomain.DefaultProjectContentType)), InputRefs: []string{"input:" + value.ID}, Priority: input.Priority, RiskProfile: input.RiskProfile, RequestedOutput: input.RequestedOutput}, requestID)
		if err != nil {
			return work.InputItem{}, err
		}
		value.ProjectID = created.Task.ProjectID
		value.TargetTaskID = created.Task.ID
		value.Status = work.InputItemTaskCreated
	default:
		return work.InputItem{}, fault.Invalid("INPUT_ITEM_ACTION_INVALID", "不支持的输入收集分流动作")
	}
	value.RowVersion = expectedVersion + 1
	value.UpdatedAt = s.now().UTC()
	if err := s.tasks.SaveInputItem(ctx, value, expectedVersion); err != nil {
		return work.InputItem{}, err
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
