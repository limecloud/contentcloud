package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type TaskActionInput struct {
	Action string `json:"action"`
}

type StageReportInput struct {
	StageRunID string                 `json:"stage_run_id"`
	StageID    string                 `json:"stage_id"`
	Status     string                 `json:"status"`
	OutputRefs []string               `json:"output_refs"`
	Outputs    []work.TaskStageOutput `json:"outputs"`
	RevisionID string                 `json:"revision_id"`
	Checks     map[string]any         `json:"checks"`
	ErrorCode  string                 `json:"error_code"`
	Summary    string                 `json:"summary"`
}

type GateDecisionInput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type CreateTaskRevisionInput struct {
	ContentType          string          `json:"content_type"`
	SchemaVersion        string          `json:"schema_version"`
	Content              json.RawMessage `json:"content"`
	KnowledgeSnapshotIDs []string        `json:"knowledge_snapshot_ids"`
	EvidenceSummary      map[string]any  `json:"evidence_summary"`
	RightsSummary        map[string]any  `json:"rights_summary"`
}

type CreateTaskDeliveryInput struct {
	RevisionID        string   `json:"revision_id"`
	DeliveryPackageID string   `json:"delivery_package_id"`
	Destination       string   `json:"destination"`
	Manifest          []string `json:"manifest,omitempty"`
	Deliver           *bool    `json:"deliver"`
}

func (s *WorkService) RunsForWorkTask(ctx context.Context, actor Actor, taskID string) ([]work.RuntimeRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.app.Runtime.runtimeRunsForWorkTask(ctx, actor.TenantID, taskID)
}

func (s *WorkService) WorkTaskGates(ctx context.Context, actor Actor, taskID string) ([]reviewdomain.GateEvaluation, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.delivery.GateEvaluations(ctx, actor.TenantID, taskID)
}

func (s *WorkService) WorkTaskRevisions(ctx context.Context, actor Actor, taskID string) ([]reviewdomain.TaskRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return nil, err
	}
	return s.taskRevisions(ctx, task)
}

func (s *WorkService) WorkTaskDeliveries(ctx context.Context, actor Actor, taskID string) ([]deliverydomain.TaskDelivery, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.delivery.TaskDeliveries(ctx, actor.TenantID, taskID)
}

func (s *WorkService) TaskAction(ctx context.Context, actor Actor, taskID string, input TaskActionInput, requestID string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return WorkTaskView{}, err
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	if action == "" {
		return WorkTaskView{}, fault.Invalid("TASK_ACTION_REQUIRED", "任务动作不能为空")
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return WorkTaskView{}, err
	}
	now := s.now().UTC()
	switch action {
	case "claim":
		assignee := actor.UserID
		if assignee == "" {
			assignee = actor.DeviceID
		}
		if assignee == "" {
			return WorkTaskView{}, fault.Invalid("TASK_CLAIM_ACTOR_REQUIRED", "领取任务需要有效执行者")
		}
		if task.AssigneeUserID != "" && task.AssigneeUserID != assignee {
			return WorkTaskView{}, fault.Conflict("TASK_ALREADY_CLAIMED", "任务已被其他成员领取")
		}
		task.AssigneeUserID = assignee
		if task.Status == work.TaskStatusNeedsInput {
			task.NextAction = "补充任务输入"
		}
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.claimed", "task", task.ID, requestID, map[string]any{"assignee_user_id": assignee})
	case "start", "resume":
		if task.Status == work.TaskStatusNeedsInput {
			return WorkTaskView{}, fault.Policy("TASK_INPUT_REQUIRED", "任务仍缺少输入，不能开始执行", "先补充至少一个输入引用")
		}
		if task.Status == work.TaskStatusCancelled || task.Status == work.TaskStatusDelivered || task.Status == work.TaskStatusAccepted {
			return WorkTaskView{}, fault.Conflict("TASK_NOT_STARTABLE", "当前任务状态不能开始执行")
		}
		if task.Status == work.TaskStatusWaitingGate {
			return WorkTaskView{}, fault.Policy("TASK_GATE_PENDING", "任务仍在等待审核决定", "先完成待处理的审核")
		}
		if err := s.startCurrentStage(ctx, actor, &task, now, requestID); err != nil {
			return WorkTaskView{}, err
		}
	case "pause":
		if task.Status != work.TaskStatusRunning {
			return WorkTaskView{}, fault.Conflict("TASK_NOT_RUNNING", "只有运行中的任务可以暂停")
		}
		task.Status = work.TaskStatusPaused
		task.NextAction = "恢复当前流程阶段"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.paused", "task", task.ID, requestID, nil)
	case "cancel":
		if task.Status == work.TaskStatusDelivered || task.Status == work.TaskStatusCancelled {
			return s.WorkTask(ctx, actor, task.ID)
		}
		task.Status = work.TaskStatusCancelled
		task.NextAction = "任务已取消"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		runs, runErr := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return WorkTaskView{}, runErr
		}
		for _, run := range runs {
			if run.Status != work.StageRunStatusCompleted && run.Status != work.StageRunStatusCancelled {
				run.Status = work.StageRunStatusCancelled
				run.UpdatedAt = now
				if err := s.tasks.SaveStageRun(ctx, run); err != nil {
					return WorkTaskView{}, err
				}
			}
		}
		s.audit(ctx, actor, task.ProjectID, "task.cancelled", "task", task.ID, requestID, nil)
	case "retry":
		if task.Status != work.TaskStatusBlocked && task.Status != work.TaskStatusPaused && task.Status != work.TaskStatusReady {
			return WorkTaskView{}, fault.Conflict("TASK_NOT_RETRYABLE", "当前任务没有可重试的失败或阻断")
		}
		runs, runErr := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return WorkTaskView{}, runErr
		}
		current, currentErr := currentStageRun(task, runs)
		if currentErr != nil {
			return WorkTaskView{}, currentErr
		}
		current.Status = work.StageRunStatusPending
		current.OutputRefs = []string{}
		current.CompletedAt = nil
		current.UpdatedAt = now
		if err := s.tasks.SaveStageRun(ctx, current); err != nil {
			return WorkTaskView{}, err
		}
		task.Status = work.TaskStatusReady
		task.NextAction = "开始当前流程阶段"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.retry_scheduled", "task", task.ID, requestID, map[string]any{"stage_id": current.StageID})
	default:
		return WorkTaskView{}, fault.Invalid("TASK_ACTION_UNSUPPORTED", "不支持的任务动作: "+action)
	}
	return s.WorkTask(ctx, actor, task.ID)
}

func (s *WorkService) startCurrentStage(ctx context.Context, actor Actor, task *work.WorkTask, now time.Time, requestID string) error {
	runs, err := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return err
	}
	stageRun, err := currentStageRun(*task, runs)
	if err != nil {
		return err
	}
	if stageRun.Status == work.StageRunStatusWaitingGate {
		return fault.Policy("STAGE_GATE_PENDING", "当前流程阶段正在等待审核决定", "先处理审核决定")
	}
	if stageRun.Status == work.StageRunStatusCompleted {
		return fault.Conflict("STAGE_ALREADY_COMPLETED", "当前流程阶段已完成")
	}
	if stageRun.Status != work.StageRunStatusRunning {
		stageRun.Status = work.StageRunStatusRunning
		if stageRun.StartedAt == nil {
			started := now
			stageRun.StartedAt = &started
		}
		stageRun.UpdatedAt = now
		if err := s.tasks.SaveStageRun(ctx, stageRun); err != nil {
			return err
		}
	}
	task.Status = work.TaskStatusRunning
	task.NextAction = "执行流程阶段“" + stageRun.StageID + "”并上报结果"
	task.UpdatedAt = now
	if err := s.tasks.SaveWorkTask(ctx, *task); err != nil {
		return err
	}
	if err := s.ensureRuntimeRun(ctx, actor, *task, stageRun); err != nil {
		return err
	}
	s.audit(ctx, actor, task.ProjectID, "task.started", "task", task.ID, requestID, map[string]any{"stage_id": stageRun.StageID, "execution_mode": stageRun.ExecutionMode})
	return nil
}

func (s *WorkService) ensureRuntimeRun(ctx context.Context, actor Actor, task work.WorkTask, stageRun work.StageRun) error {
	if s.runtimeService == nil {
		return fault.Policy("RUNTIME_UNAVAILABLE", "流程阶段需要已配置的 Runtime", "联系平台运营人员启用 Runtime")
	}
	jobs, err := s.runtimeService.Jobs(ctx, actor.TenantID, task.ID)
	if err != nil {
		return err
	}
	if len(jobs) > 0 {
		return nil
	}
	_, sop, err := s.loadTaskSOP(ctx, actor.TenantID, task)
	if err != nil {
		return err
	}
	priority := 0
	if value, parseErr := strconv.Atoi(task.Priority); parseErr == nil {
		priority = value
	} else if task.Priority == "high" {
		priority = 10
	} else if task.Priority == "urgent" {
		priority = 20
	}
	executionBinding, err := s.app.Catalog.buildRuntimeExecutionBinding(ctx, runtimeExecutionBindingInput{
		TenantID: task.TenantID, ProjectID: task.ProjectID, EnvironmentID: task.EnvironmentID,
		ContentTypes: sop.ContentTypes, RuntimePolicyID: "runtime-policy/work-task-v1",
	})
	if err != nil {
		return err
	}
	inputDigest, err := stablehash.Sum(struct {
		TaskID    string   `json:"task_id"`
		InputRefs []string `json:"input_refs"`
	}{task.ID, task.InputRefs})
	if err != nil {
		return err
	}
	_, err = s.runtimeService.Start(ctx, contentruntime.StartInput{
		TenantID: task.TenantID, ProjectID: task.ProjectID, WorkTaskID: task.ID,
		BusinessType: "work_task." + task.ContentType, SOP: sop,
		ExecutionBinding: &executionBinding, InputDigest: "sha256:" + inputDigest,
		RuntimePolicyID: "runtime-policy/work-task-v1", ContractMajor: 1, ContractMinor: 0,
		Priority: priority, CreatedBy: actor.UserID, IdempotencyKey: "work-task:" + task.ID + ":" + stageRun.StageID,
		CorrelationID: "task-start:" + task.ID,
	})
	return err
}

func currentStageRun(task work.WorkTask, runs []work.StageRun) (work.StageRun, error) {
	for _, run := range runs {
		if run.StageID == task.CurrentStageID {
			return run, nil
		}
	}
	return work.StageRun{}, fault.NotFound("当前流程阶段执行记录")
}

func (s *WorkService) ReportStage(ctx context.Context, actor Actor, taskID string, input StageReportInput, requestID string) (WorkTaskView, error) {
	if actor.Type != "device" && actor.Type != "worker" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
			return WorkTaskView{}, err
		}
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if task.Status != work.TaskStatusRunning {
		return WorkTaskView{}, fault.Conflict("TASK_NOT_RUNNING", "只有运行中的任务可以上报流程阶段结果")
	}
	runs, err := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	stageRun, err := currentStageRun(task, runs)
	if err != nil {
		return WorkTaskView{}, err
	}
	if input.StageRunID != "" && input.StageRunID != stageRun.ID {
		return WorkTaskView{}, fault.Conflict("STAGE_RUN_NOT_CURRENT", "上报的执行记录不属于当前流程阶段")
	}
	if input.StageID != "" && input.StageID != stageRun.StageID {
		return WorkTaskView{}, fault.Conflict("STAGE_NOT_CURRENT", "上报的流程阶段不是当前阶段")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = work.StageRunStatusCompleted
	}
	now := s.now().UTC()
	outputs, outputRefs, err := s.prepareStageOutputs(ctx, actor, task, stageRun, input.Outputs, now)
	if err != nil {
		return WorkTaskView{}, err
	}
	if task.ContentType == identitydomain.ContentTypeMarketingVideo && len(input.OutputRefs) > 0 {
		return WorkTaskView{}, fault.Invalid("LEGACY_OUTPUT_REFS_NOT_ALLOWED", "营销视频任务必须按类型上报流程阶段输出")
	}
	if task.ContentType != identitydomain.ContentTypeMarketingVideo && len(outputRefs) == 0 {
		outputRefs = append([]string{}, input.OutputRefs...)
	}
	if status == "failed" || status == work.StageRunStatusBlocked {
		stageRun.Status = work.StageRunStatusBlocked
		stageRun.OutputRefs = outputRefs
		stageRun.UpdatedAt = now
		if err := s.tasks.CompleteStageRun(ctx, stageRun, outputs); err != nil {
			return WorkTaskView{}, err
		}
		task.Status = work.TaskStatusBlocked
		task.NextAction = "修复输出后重试当前流程阶段"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "stage.reported_failed", "stage_run", stageRun.ID, requestID, map[string]any{"error_code": input.ErrorCode, "summary": input.Summary})
		return s.WorkTask(ctx, actor, task.ID)
	}
	if status != work.StageRunStatusCompleted {
		return WorkTaskView{}, fault.Invalid("STAGE_REPORT_STATUS_INVALID", "流程阶段上报状态必须是“已完成”或“失败”")
	}
	_, sop, err := s.loadTaskSOP(ctx, actor.TenantID, task)
	if err != nil {
		return WorkTaskView{}, err
	}
	stage, err := stageDefinition(sop, stageRun.StageID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if err := validateStageOutputContract(stage, outputs, task.ContentType == identitydomain.ContentTypeMarketingVideo); err != nil {
		return WorkTaskView{}, err
	}
	stageRun.OutputRefs = outputRefs
	stageRun.Outputs = outputs
	stageRun.UpdatedAt = now
	if err := s.tasks.CompleteStageRun(ctx, stageRun, outputs); err != nil {
		return WorkTaskView{}, err
	}
	if err := s.finishStageOrOpenGate(ctx, actor, &task, &stageRun, input, now, requestID); err != nil {
		return WorkTaskView{}, err
	}
	return s.WorkTask(ctx, actor, task.ID)
}

func (s *WorkService) finishStageOrOpenGate(ctx context.Context, actor Actor, task *work.WorkTask, stageRun *work.StageRun, input StageReportInput, now time.Time, requestID string) error {
	_, sop, err := s.loadTaskSOP(ctx, actor.TenantID, *task)
	if err != nil {
		return err
	}
	stage, err := stageDefinition(sop, stageRun.StageID)
	if err != nil {
		return err
	}
	checks := input.Checks
	if checks == nil {
		checks = map[string]any{}
	}
	for _, gateID := range stage.GateIDs {
		gate, gateErr := gateDefinition(sop, gateID)
		if gateErr != nil {
			return gateErr
		}
		mode := gate.Mode
		if mode == catalogdomain.GateModeNone {
			continue
		}
		passed := checksPassed(checks, gate.Checks)
		if mode == catalogdomain.GateModeRequiredCheck {
			evaluationStatus := reviewdomain.GateEvaluationApproved
			if !passed {
				evaluationStatus = reviewdomain.GateEvaluationRejected
			}
			evaluation := reviewdomain.GateEvaluation{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: evaluationStatus, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, Decision: map[bool]string{true: "passed", false: "failed"}[passed], CreatedAt: now, UpdatedAt: now}
			if err := s.delivery.CreateGateEvaluation(ctx, evaluation); err != nil {
				return err
			}
			if !passed {
				stageRun.Status = work.StageRunStatusBlocked
				task.Status = work.TaskStatusBlocked
				task.NextAction = "修复检查失败后重试"
				stageRun.UpdatedAt = now
				if err := s.tasks.SaveStageRun(ctx, *stageRun); err != nil {
					return err
				}
				task.UpdatedAt = now
				return s.tasks.SaveWorkTask(ctx, *task)
			}
			continue
		}
		if mode == catalogdomain.GateModeAdvisory {
			evaluation := reviewdomain.GateEvaluation{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: reviewdomain.GateEvaluationApproved, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, Decision: "advisory_passed", CreatedAt: now, UpdatedAt: now}
			if err := s.delivery.CreateGateEvaluation(ctx, evaluation); err != nil {
				return err
			}
			continue
		}
		evaluation := reviewdomain.GateEvaluation{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: reviewdomain.GateEvaluationPending, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, CreatedAt: now, UpdatedAt: now}
		if gate.EscalationHours > 0 {
			expires := now.Add(time.Duration(gate.EscalationHours) * time.Hour)
			evaluation.ExpiresAt = &expires
		}
		if err := s.delivery.CreateGateEvaluation(ctx, evaluation); err != nil {
			return err
		}
		stageRun.Status = work.StageRunStatusWaitingGate
		stageRun.UpdatedAt = now
		task.Status = work.TaskStatusWaitingGate
		task.NextAction = "处理审核：" + gate.Name
		task.UpdatedAt = now
		if err := s.tasks.SaveStageRun(ctx, *stageRun); err != nil {
			return err
		}
		if err := s.tasks.SaveWorkTask(ctx, *task); err != nil {
			return err
		}
		s.audit(ctx, actor, task.ProjectID, "gate.created", "gate_evaluation", evaluation.ID, requestID, map[string]any{"gate_id": gate.ID, "mode": mode})
		return nil
	}
	return s.completeStage(ctx, task, stageRun, sop, now)
}

func (s *WorkService) DecideGate(ctx context.Context, actor Actor, taskID, gateID string, input GateDecisionInput, requestID string) (WorkTaskView, error) {
	if actor.Role != "client_approver" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
			return WorkTaskView{}, err
		}
	}
	if actor.UserID == "" {
		return WorkTaskView{}, fault.Policy("GATE_DECIDER_REQUIRED", "审核决定需要已认证成员", "请使用受邀的内部审核人或客户决定人账号")
	}
	if actor.Role == "client_approver" && actor.Type != "user" {
		return WorkTaskView{}, fault.Policy("CLIENT_APPROVER_SESSION_REQUIRED", "客户决定人必须通过成员账号登录", "请使用受邀客户决定人账号登录")
	}
	if actor.Role == "client_approver" && actor.TenantID == "" {
		return WorkTaskView{}, fault.Policy("CLIENT_APPROVER_TENANT_REQUIRED", "客户决定人缺少租户作用域", "请重新登录后重试")
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return WorkTaskView{}, err
	}
	evaluations, err := s.delivery.GateEvaluations(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	var evaluation reviewdomain.GateEvaluation
	for _, candidate := range evaluations {
		if candidate.ID == gateID || candidate.GateID == gateID {
			if candidate.Status == reviewdomain.GateEvaluationPending {
				evaluation = candidate
				break
			}
		}
	}
	if evaluation.ID == "" {
		return WorkTaskView{}, fault.NotFound("待处理的审核")
	}
	if actor.Role == "client_approver" && evaluation.GateMode != catalogdomain.GateModeClientDecision {
		return WorkTaskView{}, fault.Policy("CLIENT_GATE_ROLE_REQUIRED", "客户决定人只能处理客户确认类型（client_decision）的审核", "请由内部审核角色处理当前审核")
	}
	_, taskSOP, sopErr := s.loadTaskSOP(ctx, actor.TenantID, task)
	if sopErr != nil {
		return WorkTaskView{}, sopErr
	}
	gate, gateErr := gateDefinition(taskSOP, evaluation.GateID)
	if gateErr != nil {
		return WorkTaskView{}, gateErr
	}
	if evaluation.GateMode == catalogdomain.GateModeClientDecision && actor.Role != "client_approver" && actor.Role != "tenant_admin" {
		return WorkTaskView{}, fault.Policy("CLIENT_GATE_ROLE_REQUIRED", "客户确认类型（client_decision）的审核只能由客户决定人或租户管理员处理", "请邀请客户决定人处理该审核")
	}
	if len(gate.AssigneeRoles) > 0 && actor.Role != "tenant_admin" && !containsString(gate.AssigneeRoles, actor.Role) {
		return WorkTaskView{}, fault.Policy("GATE_ASSIGNEE_ROLE_REQUIRED", "当前成员不在该审核的决定角色范围内", "请由审核配置中的决定角色处理")
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != "approved" && decision != "rejected" && decision != "changes_requested" {
		return WorkTaskView{}, fault.Invalid("GATE_DECISION_INVALID", "审核决定必须是“批准（approved）”“拒绝（rejected）”或“要求修改（changes_requested）”")
	}
	if decision == "approved" && evaluation.GateID == "script_review" && task.ContentType == identitydomain.ContentTypeMarketingVideo {
		if err := s.approveMarketingVideoScript(ctx, actor, task, evaluation.StageRunID, input.Reason, requestID); err != nil {
			return WorkTaskView{}, err
		}
	}
	now := s.now().UTC()
	evaluation.Status = map[string]string{"approved": reviewdomain.GateEvaluationApproved, "rejected": reviewdomain.GateEvaluationRejected, "changes_requested": reviewdomain.GateEvaluationChangesRequested}[decision]
	evaluation.Decision = decision
	evaluation.Reason = strings.TrimSpace(input.Reason)
	evaluation.DecidedBy = actor.UserID
	evaluation.DecidedAt = &now
	evaluation.UpdatedAt = now
	if err := s.delivery.SaveGateEvaluation(ctx, evaluation); err != nil {
		return WorkTaskView{}, err
	}
	runs, err := s.tasks.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	stageRun, err := stageRunByID(runs, evaluation.StageRunID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if decision != "approved" {
		stageRun.Status = work.StageRunStatusBlocked
		task.Status = work.TaskStatusBlocked
		task.NextAction = "根据审核意见修改并重试"
		stageRun.UpdatedAt = now
		task.UpdatedAt = now
		if err := s.tasks.SaveStageRun(ctx, stageRun); err != nil {
			return WorkTaskView{}, err
		}
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "gate.decided", "gate_evaluation", evaluation.ID, requestID, map[string]any{"decision": decision, "reason": evaluation.Reason})
		return s.WorkTask(ctx, actor, task.ID)
	}
	allEvaluations, err := s.delivery.GateEvaluations(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	for _, candidate := range allEvaluations {
		if candidate.StageRunID == stageRun.ID && candidate.Status == reviewdomain.GateEvaluationPending {
			return s.WorkTask(ctx, actor, task.ID)
		}
	}
	_, sop, err := s.loadTaskSOP(ctx, actor.TenantID, task)
	if err != nil {
		return WorkTaskView{}, err
	}
	if err := s.completeStage(ctx, &task, &stageRun, sop, now); err != nil {
		return WorkTaskView{}, err
	}
	s.audit(ctx, actor, task.ProjectID, "gate.decided", "gate_evaluation", evaluation.ID, requestID, map[string]any{"decision": decision})
	return s.WorkTask(ctx, actor, task.ID)
}

func (s *WorkService) completeStage(ctx context.Context, task *work.WorkTask, stageRun *work.StageRun, sop catalogdomain.SOPVersion, now time.Time) error {
	stageRun.Status = work.StageRunStatusCompleted
	completed := now
	stageRun.CompletedAt = &completed
	stageRun.UpdatedAt = now
	if err := s.tasks.SaveStageRun(ctx, *stageRun); err != nil {
		return err
	}
	var next *catalogdomain.StageDefinition
	for index := range sop.Stages {
		if sop.Stages[index].ID == stageRun.StageID && index+1 < len(sop.Stages) {
			candidate := sop.Stages[index+1]
			next = &candidate
			break
		}
	}
	if next == nil {
		task.Status = work.TaskStatusAccepted
		task.CurrentStageID = ""
		task.NextAction = "提交内容版本"
	} else {
		task.Status = work.TaskStatusReady
		task.CurrentStageID = next.ID
		task.NextAction = "开始 " + next.Name
		if err := s.tasks.CreateStageRun(ctx, work.StageRun{ID: idgen.New(), TenantID: task.TenantID, TaskID: task.ID, StageID: next.ID, Status: work.StageRunStatusPending, ExecutionMode: defaultString(firstString(next.ExecutionModes), sop.DefaultExecutionMode), InputRefs: append([]string{}, next.InputRefs...), UpdatedAt: now}); err != nil {
			return err
		}
	}
	task.UpdatedAt = now
	return s.tasks.SaveWorkTask(ctx, *task)
}

func (s *WorkService) CreateTaskRevision(ctx context.Context, actor Actor, taskID string, input CreateTaskRevisionInput, requestID string) (reviewdomain.TaskRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	if task.Status == work.TaskStatusCancelled || task.Status == work.TaskStatusDelivered {
		return reviewdomain.TaskRevision{}, fault.Conflict("TASK_REVISION_NOT_ALLOWED", "已取消或已交付任务不能提交新的内容版本")
	}
	contentType := defaultString(input.ContentType, task.ContentType)
	schemaVersion := defaultString(input.SchemaVersion, contentSchemaVersion(contentType))
	if err := validateTaskContent(contentType, schemaVersion, input.Content); err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	if contentType == identitydomain.ContentTypeMarketingVideo {
		return s.createMarketingVideoSubmissionRevision(ctx, actor, task, input, requestID)
	}
	revisions, err := s.delivery.TaskRevisions(ctx, actor.TenantID, task.ID)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	revisionNo := len(revisions) + 1
	hash, err := stablehash.Sum(json.RawMessage(input.Content))
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	now := s.now().UTC()
	status := reviewdomain.TaskRevisionSubmitted
	if task.Status == work.TaskStatusAccepted {
		status = reviewdomain.TaskRevisionAccepted
	}
	revision := reviewdomain.TaskRevision{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, RevisionNo: revisionNo, ContentType: contentType, SchemaVersion: schemaVersion, Content: append([]byte{}, input.Content...), ContentHash: "sha256:" + hash, SOPDigest: task.SOPDigest, KnowledgeSnapshotIDs: append([]string{}, input.KnowledgeSnapshotIDs...), EvidenceSummary: input.EvidenceSummary, RightsSummary: input.RightsSummary, Status: status, SubmittedBy: actor.UserID, SubmittedAt: &now, CreatedAt: now}
	revision.NormalizeCollections()
	if err := s.delivery.CreateTaskRevision(ctx, revision); err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	if task.Status == work.TaskStatusBlocked {
		task.Status = work.TaskStatusReady
		task.NextAction = "重试当前流程阶段"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return reviewdomain.TaskRevision{}, err
		}
	}
	if task.Status == work.TaskStatusAccepted {
		task.NextAction = "交付内容版本"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return reviewdomain.TaskRevision{}, err
		}
	}
	s.audit(ctx, actor, task.ProjectID, "task.revision_submitted", "task_revision", revision.ID, requestID, map[string]any{"task_id": task.ID, "revision_no": revision.RevisionNo, "content_hash": revision.ContentHash, "schema_version": revision.SchemaVersion})
	return revision, nil
}

func (s *WorkService) CreateTaskDelivery(ctx context.Context, actor Actor, taskID string, input CreateTaskDeliveryInput, requestID string) (deliverydomain.TaskDelivery, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return deliverydomain.TaskDelivery{}, err
	}
	task, err := s.tasks.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return deliverydomain.TaskDelivery{}, err
	}
	if task.Status != work.TaskStatusAccepted {
		return deliverydomain.TaskDelivery{}, fault.Policy("TASK_NOT_ACCEPTED", "只有已接受的任务可以交付", "先完成所有流程阶段并提交内容版本")
	}
	revisions, err := s.taskRevisions(ctx, task)
	if err != nil {
		return deliverydomain.TaskDelivery{}, err
	}
	var revision reviewdomain.TaskRevision
	for index := len(revisions) - 1; index >= 0; index-- {
		if revisions[index].Status != reviewdomain.TaskRevisionAccepted || (input.RevisionID != "" && revisions[index].ID != input.RevisionID) {
			continue
		}
		revision = revisions[index]
		break
	}
	if revision.ID == "" {
		return deliverydomain.TaskDelivery{}, fault.Policy("TASK_REVISION_NOT_ACCEPTED", "交付必须引用当前任务已接受的内容版本", "先提交最终内容版本")
	}
	if len(input.Manifest) > 0 {
		return deliverydomain.TaskDelivery{}, fault.Invalid("TASK_DELIVERY_MANIFEST_SERVER_OWNED", "交付文件清单由服务端交付包生成，不能由客户端提交")
	}
	destination := defaultString(input.Destination, "workspace")
	deliver := input.Deliver != nil && *input.Deliver
	manifest := []string{}
	integrityStatus := "script_only"
	packageID := strings.TrimSpace(input.DeliveryPackageID)
	if packageID == "" && deliver {
		return deliverydomain.TaskDelivery{}, fault.Policy("DELIVERY_PACKAGE_REQUIRED", "完成交付必须引用服务端已就绪的交付包", "先完成最终成片批准并构建交付包")
	}
	if deliver && destination != "workspace" {
		return deliverydomain.TaskDelivery{}, fault.Policy("CHANNEL_PUBLICATION_RECEIPT_REQUIRED", "渠道交付不能在创建交付记录时直接标记成功", "先创建 ready 交付，再通过渠道发布和外部回执推进状态")
	}
	if packageID != "" {
		pkg, packageErr := s.artifacts.DeliveryPackage(ctx, actor.TenantID, packageID)
		if packageErr != nil {
			return deliverydomain.TaskDelivery{}, packageErr
		}
		if pkg.ProjectID != task.ProjectID || pkg.ContentItemID != task.ID || pkg.Status != "ready" || len(pkg.Manifest) == 0 {
			return deliverydomain.TaskDelivery{}, fault.Policy("DELIVERY_PACKAGE_INVALID", "交付包不属于当前任务、尚未就绪或文件清单为空", "重新构建当前任务的交付包")
		}
		for _, artifact := range pkg.Manifest {
			if artifact.TenantID != task.TenantID || artifact.ProjectID != task.ProjectID || artifact.ByteSize <= 0 || strings.TrimSpace(artifact.SHA256) == "" {
				return deliverydomain.TaskDelivery{}, fault.Conflict("DELIVERY_ARTIFACT_INTEGRITY_FAILED", "交付包中存在缺失或摘要无效的成果文件")
			}
			if quarantined, _ := artifact.Metadata["quarantined"].(bool); quarantined {
				return deliverydomain.TaskDelivery{}, fault.Policy("DELIVERY_ARTIFACT_QUARANTINED", "交付包中存在隔离中的成果文件", "处理媒体安全问题后重新构建交付包")
			}
			stored, artifactErr := s.artifacts.Artifact(ctx, actor.TenantID, artifact.ID)
			if artifactErr != nil || stored.ByteSize != artifact.ByteSize || normalizedSHA256(stored.SHA256) != normalizedSHA256(artifact.SHA256) {
				return deliverydomain.TaskDelivery{}, fault.Conflict("DELIVERY_ARTIFACT_DIGEST_MISMATCH", "交付包中的成果文件与服务端存储不一致")
			}
			manifest = append(manifest, artifact.ID+"@"+normalizedSHA256(artifact.SHA256))
		}
		if task.ContentType == identitydomain.ContentTypeMarketingVideo {
			reviews, reviewErr := s.delivery.MediaReviews(ctx, actor.TenantID, task.ID)
			if reviewErr != nil {
				return deliverydomain.TaskDelivery{}, reviewErr
			}
			finalApproved := false
			for _, review := range reviews {
				if review.ReviewKind != deliverydomain.MediaReviewFinal || review.Status != deliverydomain.MediaReviewApproved || !review.Selected {
					continue
				}
				for _, artifact := range pkg.Manifest {
					if review.SubjectArtifactID == artifact.ID && review.SubjectDigest == normalizedSHA256(artifact.SHA256) {
						finalApproved = true
					}
				}
			}
			if !finalApproved {
				return deliverydomain.TaskDelivery{}, fault.Policy("FINAL_MEDIA_REVIEW_REQUIRED", "营销视频交付包缺少精确绑定最终成果文件的批准记录", "先完成最终成片批准")
			}
		}
		integrityStatus = "complete"
	}
	digest, err := stablehash.Sum(struct {
		TaskID            string   `json:"task_id"`
		RevisionID        string   `json:"revision_id"`
		DeliveryPackageID string   `json:"delivery_package_id"`
		Destination       string   `json:"destination"`
		Manifest          []string `json:"manifest"`
	}{task.ID, revision.ID, packageID, destination, manifest})
	if err != nil {
		return deliverydomain.TaskDelivery{}, err
	}
	now := s.now().UTC()
	delivery := deliverydomain.TaskDelivery{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, RevisionID: revision.ID, Destination: destination, Status: deliverydomain.TaskDeliveryReady, Manifest: manifest, DeliveryPackageID: packageID, IntegrityStatus: integrityStatus, DeliveryDigest: "sha256:" + digest, CreatedAt: now, UpdatedAt: now}
	delivery.NormalizeCollections()
	if deliver {
		delivery.Status = deliverydomain.TaskDeliveryDelivered
		delivery.DeliveredBy = actor.UserID
		delivery.DeliveredAt = &now
	}
	if err := s.delivery.CreateTaskDelivery(ctx, delivery); err != nil {
		return deliverydomain.TaskDelivery{}, err
	}
	if deliver {
		task.Status = work.TaskStatusDelivered
		task.NextAction = "已交付"
		task.UpdatedAt = now
		if err := s.tasks.SaveWorkTask(ctx, task); err != nil {
			return deliverydomain.TaskDelivery{}, err
		}
	}
	s.audit(ctx, actor, task.ProjectID, "task.delivery_created", "task_delivery", delivery.ID, requestID, map[string]any{"revision_id": revision.ID, "destination": destination, "status": delivery.Status})
	return delivery, nil
}

func (s *WorkService) createMarketingVideoSubmissionRevision(ctx context.Context, actor Actor, task work.WorkTask, input CreateTaskRevisionInput, requestID string) (reviewdomain.TaskRevision, error) {
	if task.CurrentStageID != "script" || (task.Status != work.TaskStatusRunning && task.Status != work.TaskStatusBlocked) {
		return reviewdomain.TaskRevision{}, fault.Policy("MARKETING_VIDEO_SCRIPT_STAGE_REQUIRED", "营销视频剧本只能在运行中的剧本阶段提交", "开始或重试短视频剧本阶段")
	}
	binding, err := s.ensureTaskWorkspace(ctx, actor, task)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	var content map[string]any
	if err := json.Unmarshal(input.Content, &content); err != nil {
		return reviewdomain.TaskRevision{}, fault.Invalid("TASK_CONTENT_OBJECT_REQUIRED", "内容版本的正文必须是对象")
	}
	contentItemID := "content-item:" + task.ID
	content["id"] = contentItemID
	content["type"] = "content_item"
	content["status"] = "review_ready"
	content["schema_version"] = localworkspace.ContentItemSchema
	content["deliverability"] = "review_ready"
	content["project_id"] = task.ProjectID
	content["task_id"] = task.ID
	content["content_id"] = contentItemID
	content["content_batch_id"] = "content-batch:" + task.ID
	content["content_kind"] = identitydomain.ContentTypeMarketingVideo
	content["knowledge_snapshot_ids"] = append([]string{}, input.KnowledgeSnapshotIDs...)
	content["evidence_summary"] = input.EvidenceSummary
	content["rights_summary"] = input.RightsSummary
	content["sop_digest"] = task.SOPDigest
	if _, ok := content["aspect_ratio"]; !ok {
		content["aspect_ratio"] = "9:16"
	}

	probe, err := reviewdomain.NewSubmissionObjectRef(contentItemID, "content_item", 1, "50-production/tasks/"+task.ID+"/script.json", content)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	existingSubmission, existingErr := s.review.SubmissionByWorkspaceType(ctx, task.TenantID, task.ProjectID, binding.ID, "content_batch")
	existingRevisions := []reviewdomain.SubmissionRevision{}
	if existingErr == nil {
		existingRevisions, err = s.review.SubmissionRevisions(ctx, task.TenantID, existingSubmission.ID)
		if err != nil {
			return reviewdomain.TaskRevision{}, err
		}
		for _, existing := range existingRevisions {
			if len(existing.Objects) == 1 && existing.Objects[0].Digest == probe.Digest {
				return taskRevisionFromSubmission(task, existingSubmission, existing), nil
			}
		}
	} else if !fault.IsNotFound(existingErr) {
		return reviewdomain.TaskRevision{}, existingErr
	}
	object, err := reviewdomain.NewSubmissionObjectRef(contentItemID, "content_item", len(existingRevisions)+1, "50-production/tasks/"+task.ID+"/script.json", content)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	bundle := reviewdomain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: "content_batch", ProjectID: task.ProjectID, WorkspaceID: binding.ID,
		BaseSnapshotIDs: []string{}, Objects: []reviewdomain.SubmissionObjectRef{object}, SourceDisclosures: []reviewdomain.SourceDisclosure{},
		LocalRunSummary:   reviewdomain.LocalRunSummary{Stage: "script", Checks: []reviewdomain.LocalRunCheck{{Name: "content.schema", Status: "passed"}, {Name: "claim.references", Status: "passed"}}},
		EnvironmentDigest: task.SOPDigest, Artifacts: []reviewdomain.SubmissionArtifact{}, Message: "营销视频剧本提交",
		IdempotencyKey: "task-script:" + task.ID + ":" + strings.TrimPrefix(object.Digest, "sha256:"),
	}
	if err := bundle.SetComputedHash(); err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	workspaceActor := Actor{TenantID: task.TenantID, WorkspaceID: binding.ID, Type: "workspace", Role: "workspace"}
	revision, err := s.app.Review.CreateSubmission(ctx, workspaceActor, binding, bundle, requestID)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	submission, err := s.review.Submission(ctx, task.TenantID, revision.SubmissionID)
	if err != nil {
		return reviewdomain.TaskRevision{}, err
	}
	return taskRevisionFromSubmission(task, submission, revision), nil
}

func (s *WorkService) ensureTaskWorkspace(ctx context.Context, actor Actor, task work.WorkTask) (workspacedomain.WorkspaceBinding, error) {
	binding, err := s.workspace.WorkspaceBinding(ctx, task.TenantID, task.ID)
	if err == nil {
		return binding, nil
	}
	if !fault.IsNotFound(err) {
		return workspacedomain.WorkspaceBinding{}, err
	}
	_, credentialHash, err := idgen.NewOpaqueToken("task_", 32)
	if err != nil {
		return workspacedomain.WorkspaceBinding{}, err
	}
	now := s.now().UTC()
	binding = workspacedomain.WorkspaceBinding{ID: task.ID, TenantID: task.TenantID, ProjectID: task.ProjectID, OwnerUserID: actor.UserID, TemplateID: localworkspace.TemplateID, TemplateVersion: localworkspace.TemplateVersion, Targets: []string{"web"}, CredentialHash: credentialHash, Status: "active", InitializedAt: now, LastSeenAt: now}
	if err := s.workspace.CreateWorkspaceBinding(ctx, binding); err != nil {
		return workspacedomain.WorkspaceBinding{}, err
	}
	binding.CredentialHash = ""
	return binding, nil
}

func (s *WorkService) approveMarketingVideoScript(ctx context.Context, actor Actor, task work.WorkTask, stageRunID, reason, requestID string) error {
	outputs, err := s.tasks.TaskStageOutputs(ctx, task.TenantID, task.ID)
	if err != nil {
		return err
	}
	revisionID := ""
	for _, output := range outputs {
		if output.StageRunID == stageRunID && output.OutputType == catalogdomain.StageOutputSubmissionRevision && output.Role == catalogdomain.StageOutputRolePrimary {
			revisionID = output.ObjectID
			break
		}
	}
	if revisionID == "" {
		return fault.Policy("SCRIPT_SUBMISSION_REQUIRED", "剧本审核缺少规范的提交内容版本", "重新上报当前剧本内容版本")
	}
	revision, err := s.review.SubmissionRevision(ctx, task.TenantID, revisionID)
	if err != nil {
		return err
	}
	submission, err := s.review.Submission(ctx, task.TenantID, revision.SubmissionID)
	if err != nil {
		return err
	}
	if submission.WorkspaceID != task.ID || submission.ProjectID != task.ProjectID || submission.SubmissionType != "content_batch" {
		return fault.Policy("SCRIPT_SUBMISSION_SCOPE_INVALID", "剧本提交内容版本不属于当前营销视频任务", "重新提交当前任务剧本")
	}
	if submission.Status == "approved" {
		return nil
	}
	if submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review") {
		return fault.Conflict("SUBMISSION_STATE_INVALID", "只能批准当前待审核的剧本内容版本")
	}
	now := s.now().UTC()
	canonical, err := canonicalSubmissionContent(submission, revision)
	if err != nil {
		return err
	}
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "approve", Reason: defaultString(strings.TrimSpace(reason), "剧本审核通过"), PreviousState: submission.Status, ResultingState: "approved", CreatedAt: now}
	snapshot := reviewdomain.ApprovedSnapshot{ID: idgen.New(), TenantID: task.TenantID, ProjectID: task.ProjectID, WorkspaceID: submission.WorkspaceID, SubmissionID: submission.ID, SubmissionRevisionID: revision.ID, SubmissionType: submission.SubmissionType, SchemaVersion: revision.SchemaVersion, ContentHash: revision.ContentHash, SubjectHash: revision.ContentHash, CanonicalContent: canonical, EligibleIDs: revision.EligibleObjectIDs(), Artifacts: revision.Artifacts, DecisionID: decision.ID, CreatedBy: actor.UserID, CreatedAt: now}
	submission.Status = "approved"
	submission.UpdatedAt = now
	if err := s.review.ApproveSubmissionRevision(ctx, submission, snapshot, decision); err != nil {
		return err
	}
	s.audit(ctx, actor, task.ProjectID, "task.script_approved", "submission_revision", revision.ID, requestID, map[string]any{"task_id": task.ID, "snapshot_id": snapshot.ID, "content_hash": revision.ContentHash})
	return nil
}

func (s *WorkService) taskRevisions(ctx context.Context, task work.WorkTask) ([]reviewdomain.TaskRevision, error) {
	if task.ContentType != identitydomain.ContentTypeMarketingVideo {
		return s.delivery.TaskRevisions(ctx, task.TenantID, task.ID)
	}
	submission, err := s.review.SubmissionByWorkspaceType(ctx, task.TenantID, task.ProjectID, task.ID, "content_batch")
	if fault.IsNotFound(err) {
		return []reviewdomain.TaskRevision{}, nil
	}
	if err != nil {
		return nil, err
	}
	revisions, err := s.review.SubmissionRevisions(ctx, task.TenantID, submission.ID)
	if err != nil {
		return nil, err
	}
	values := make([]reviewdomain.TaskRevision, 0, len(revisions))
	for _, revision := range revisions {
		values = append(values, taskRevisionFromSubmission(task, submission, revision))
	}
	sort.Slice(values, func(i, j int) bool { return values[i].RevisionNo < values[j].RevisionNo })
	return values, nil
}

func taskRevisionFromSubmission(task work.WorkTask, submission reviewdomain.Submission, revision reviewdomain.SubmissionRevision) reviewdomain.TaskRevision {
	content := json.RawMessage(`{}`)
	knowledgeSnapshotIDs := []string{}
	evidenceSummary := map[string]any{}
	rightsSummary := map[string]any{}
	if len(revision.Objects) > 0 {
		content = append(json.RawMessage(nil), revision.Objects[0].Content...)
		var metadata struct {
			KnowledgeSnapshotIDs []string       `json:"knowledge_snapshot_ids"`
			EvidenceSummary      map[string]any `json:"evidence_summary"`
			RightsSummary        map[string]any `json:"rights_summary"`
		}
		if json.Unmarshal(content, &metadata) == nil {
			knowledgeSnapshotIDs = metadata.KnowledgeSnapshotIDs
			evidenceSummary = metadata.EvidenceSummary
			rightsSummary = metadata.RightsSummary
		}
	}
	status := reviewdomain.TaskRevisionSuperseded
	if revision.ID == submission.CurrentRevisionID {
		switch submission.Status {
		case "approved", "internally_approved", "client_review":
			status = reviewdomain.TaskRevisionAccepted
		case "changes_requested", "rejected":
			status = reviewdomain.TaskRevisionRejected
		default:
			status = reviewdomain.TaskRevisionSubmitted
		}
	}
	submittedAt := revision.CreatedAt
	value := reviewdomain.TaskRevision{ID: revision.ID, TenantID: revision.TenantID, ProjectID: revision.ProjectID, TaskID: task.ID, RevisionNo: revision.RevisionNo, ContentType: task.ContentType, SchemaVersion: contentSchemaVersion(task.ContentType), Content: content, ContentHash: revision.ContentHash, SOPDigest: task.SOPDigest, KnowledgeSnapshotIDs: knowledgeSnapshotIDs, EvidenceSummary: evidenceSummary, RightsSummary: rightsSummary, Status: status, SubmittedBy: task.CreatedBy, SubmittedAt: &submittedAt, CreatedAt: revision.CreatedAt}
	value.NormalizeCollections()
	return value
}

func (s *WorkService) loadTaskSOP(ctx context.Context, tenantID string, task work.WorkTask) (catalogdomain.SOPDefinition, catalogdomain.SOPVersion, error) {
	summary, err := s.catalog.SOP(ctx, tenantID, task.SOPID)
	if err != nil {
		return catalogdomain.SOPDefinition{}, catalogdomain.SOPVersion{}, err
	}
	for _, version := range summary.Versions {
		if version.Version == task.SOPVersion {
			return summary.Definition, version, nil
		}
	}
	return catalogdomain.SOPDefinition{}, catalogdomain.SOPVersion{}, fault.NotFound("流程规范版本")
}

func stageDefinition(sop catalogdomain.SOPVersion, id string) (catalogdomain.StageDefinition, error) {
	for _, stage := range sop.Stages {
		if stage.ID == id {
			return stage, nil
		}
	}
	return catalogdomain.StageDefinition{}, fault.NotFound("流程阶段")
}

func gateDefinition(sop catalogdomain.SOPVersion, id string) (catalogdomain.GateDefinition, error) {
	for _, gate := range sop.Gates {
		if gate.ID == id {
			return gate, nil
		}
	}
	return catalogdomain.GateDefinition{}, fault.NotFound("检查与审批项")
}

func stageRunByID(runs []work.StageRun, id string) (work.StageRun, error) {
	for _, run := range runs {
		if run.ID == id {
			return run, nil
		}
	}
	return work.StageRun{}, fault.NotFound("流程阶段执行记录")
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func checksPassed(checks map[string]any, names []string) bool {
	if value, ok := checks["passed"]; ok && !checkValuePassed(value) {
		return false
	}
	if value, ok := checks["status"]; ok {
		if status, ok := value.(string); ok {
			switch strings.ToLower(strings.TrimSpace(status)) {
			case "failed", "blocked", "rejected", "error":
				return false
			}
		}
	}
	if len(names) == 0 {
		if value, ok := checks["passed"]; ok {
			return checkValuePassed(value)
		}
		if value, ok := checks["status"]; ok {
			return checkValuePassed(value)
		}
		return false
	}
	for _, name := range names {
		value, ok := checks[name]
		if !ok || !checkValuePassed(value) {
			return false
		}
	}
	return true
}

func checkValuePassed(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "passed", "pass", "ok", "success", "succeeded", "true":
			return true
		case "failed", "fail", "blocked", "rejected", "error", "false":
			return false
		}
	case map[string]any:
		if status, ok := value["status"]; ok {
			return checkValuePassed(status)
		}
		if passed, ok := value["passed"]; ok {
			return checkValuePassed(passed)
		}
	}
	return false
}

func contentSchemaVersion(contentType string) string {
	switch contentType {
	case identitydomain.ContentTypeVideoScript:
		return "contentcloud.video_script/1.0"
	case identitydomain.ContentTypeWeChatArticle:
		return "contentcloud.article/1.0"
	case identitydomain.ContentTypeMarketingVideo:
		return "contentcloud.marketing_video_script/1.0"
	default:
		return "contentcloud.content/1.0"
	}
}

func validateTaskContent(contentType, schemaVersion string, content json.RawMessage) error {
	if !identitydomain.ValidTenantContentType(contentType) {
		return fault.Invalid("TASK_CONTENT_TYPE_INVALID", "任务内容类型不受支持")
	}
	if len(content) == 0 || !json.Valid(content) {
		return fault.Invalid("TASK_CONTENT_INVALID", "内容版本的正文必须是有效 JSON")
	}
	want := contentSchemaVersion(contentType)
	if schemaVersion != want && !(contentType == identitydomain.ContentTypeVideoScript && schemaVersion == "contentcloud.content_batch/3.0") {
		return fault.Invalid("TASK_CONTENT_SCHEMA_INVALID", fmt.Sprintf("%s 必须使用 %s", contentType, want))
	}
	var object map[string]any
	if err := json.Unmarshal(content, &object); err != nil {
		return fault.Invalid("TASK_CONTENT_OBJECT_REQUIRED", "内容版本的正文必须是对象")
	}
	if strings.TrimSpace(fmt.Sprint(object["title"])) == "" && strings.TrimSpace(fmt.Sprint(object["name"])) == "" {
		return fault.Invalid("TASK_CONTENT_TITLE_REQUIRED", "内容版本需要标题或名称")
	}
	if contentType == identitydomain.ContentTypeVideoScript || contentType == identitydomain.ContentTypeMarketingVideo {
		if _, ok := object["scenes"]; !ok {
			if _, ok := object["items"]; !ok {
				return fault.Invalid("VIDEO_SCRIPT_SCENES_REQUIRED", "视频脚本内容版本需要场景列表（scenes）或内容项列表（items）")
			}
		}
	}
	if contentType == identitydomain.ContentTypeWeChatArticle {
		if _, ok := object["blocks"]; !ok {
			if _, ok := object["paragraphs"]; !ok {
				return fault.Invalid("ARTICLE_BLOCKS_REQUIRED", "文章内容版本需要内容块（blocks）或段落（paragraphs）")
			}
		}
	}
	return nil
}
