package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type TaskActionInput struct {
	Action string `json:"action"`
}

type StageReportInput struct {
	StageRunID string         `json:"stage_run_id"`
	StageID    string         `json:"stage_id"`
	Status     string         `json:"status"`
	OutputRefs []string       `json:"output_refs"`
	RevisionID string         `json:"revision_id"`
	Checks     map[string]any `json:"checks"`
	ErrorCode  string         `json:"error_code"`
	Summary    string         `json:"summary"`
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
	RevisionID  string   `json:"revision_id"`
	Destination string   `json:"destination"`
	Manifest    []string `json:"manifest"`
	Deliver     *bool    `json:"deliver"`
}

func (s *Service) WorkTaskRuns(ctx context.Context, actor Actor, taskID string) ([]domain.TaskRun, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.store.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.store.WorkTaskRuns(ctx, actor.TenantID, taskID)
}

func (s *Service) WorkTaskGates(ctx context.Context, actor Actor, taskID string) ([]domain.GateEvaluation, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.store.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.store.GateEvaluations(ctx, actor.TenantID, taskID)
}

func (s *Service) WorkTaskRevisions(ctx context.Context, actor Actor, taskID string) ([]domain.TaskRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.store.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.store.TaskRevisions(ctx, actor.TenantID, taskID)
}

func (s *Service) WorkTaskDeliveries(ctx context.Context, actor Actor, taskID string) ([]domain.TaskDelivery, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer", "client_approver", "viewer"); err != nil {
		return nil, err
	}
	if _, err := s.store.WorkTask(ctx, actor.TenantID, taskID); err != nil {
		return nil, err
	}
	return s.store.TaskDeliveries(ctx, actor.TenantID, taskID)
}

func (s *Service) TaskAction(ctx context.Context, actor Actor, taskID string, input TaskActionInput, requestID string) (WorkTaskView, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor", "reviewer"); err != nil {
		return WorkTaskView{}, err
	}
	action := strings.ToLower(strings.TrimSpace(input.Action))
	if action == "" {
		return WorkTaskView{}, domain.Invalid("TASK_ACTION_REQUIRED", "任务动作不能为空")
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
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
			return WorkTaskView{}, domain.Invalid("TASK_CLAIM_ACTOR_REQUIRED", "领取任务需要有效执行者")
		}
		if task.AssigneeUserID != "" && task.AssigneeUserID != assignee {
			return WorkTaskView{}, domain.Conflict("TASK_ALREADY_CLAIMED", "任务已被其他成员领取")
		}
		task.AssigneeUserID = assignee
		if task.Status == domain.TaskStatusNeedsInput {
			task.NextAction = "补充任务输入"
		}
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.claimed", "task", task.ID, requestID, map[string]any{"assignee_user_id": assignee})
	case "start", "resume":
		if task.Status == domain.TaskStatusNeedsInput {
			return WorkTaskView{}, domain.Policy("TASK_INPUT_REQUIRED", "任务仍缺少输入，不能开始执行", "先补充至少一个输入引用")
		}
		if task.Status == domain.TaskStatusCancelled || task.Status == domain.TaskStatusDelivered || task.Status == domain.TaskStatusAccepted {
			return WorkTaskView{}, domain.Conflict("TASK_NOT_STARTABLE", "当前任务状态不能开始执行")
		}
		if task.Status == domain.TaskStatusWaitingGate {
			return WorkTaskView{}, domain.Policy("TASK_GATE_PENDING", "任务仍在等待 Gate 决定", "先处理待决定 Gate")
		}
		if err := s.startCurrentStage(ctx, actor, &task, now, requestID); err != nil {
			return WorkTaskView{}, err
		}
	case "pause":
		if task.Status != domain.TaskStatusRunning {
			return WorkTaskView{}, domain.Conflict("TASK_NOT_RUNNING", "只有运行中的任务可以暂停")
		}
		task.Status = domain.TaskStatusPaused
		task.NextAction = "恢复当前 Stage"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.paused", "task", task.ID, requestID, nil)
	case "cancel":
		if task.Status == domain.TaskStatusDelivered || task.Status == domain.TaskStatusCancelled {
			return s.WorkTask(ctx, actor, task.ID)
		}
		task.Status = domain.TaskStatusCancelled
		task.NextAction = "任务已取消"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		runs, runErr := s.store.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return WorkTaskView{}, runErr
		}
		for _, run := range runs {
			if run.Status != domain.StageRunStatusCompleted && run.Status != domain.StageRunStatusCancelled {
				run.Status = domain.StageRunStatusCancelled
				run.UpdatedAt = now
				if err := s.store.SaveStageRun(ctx, run); err != nil {
					return WorkTaskView{}, err
				}
			}
		}
		s.audit(ctx, actor, task.ProjectID, "task.cancelled", "task", task.ID, requestID, nil)
	case "retry":
		if task.Status != domain.TaskStatusBlocked && task.Status != domain.TaskStatusPaused && task.Status != domain.TaskStatusReady {
			return WorkTaskView{}, domain.Conflict("TASK_NOT_RETRYABLE", "当前任务没有可重试的失败或阻断")
		}
		runs, runErr := s.store.StageRuns(ctx, actor.TenantID, task.ID)
		if runErr != nil {
			return WorkTaskView{}, runErr
		}
		current, currentErr := currentStageRun(task, runs)
		if currentErr != nil {
			return WorkTaskView{}, currentErr
		}
		current.Status = domain.StageRunStatusPending
		current.OutputRefs = []string{}
		current.CompletedAt = nil
		current.UpdatedAt = now
		if err := s.store.SaveStageRun(ctx, current); err != nil {
			return WorkTaskView{}, err
		}
		task.Status = domain.TaskStatusReady
		task.NextAction = "开始当前 Stage"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "task.retry_scheduled", "task", task.ID, requestID, map[string]any{"stage_id": current.StageID})
	default:
		return WorkTaskView{}, domain.Invalid("TASK_ACTION_UNSUPPORTED", "不支持的任务动作: "+action)
	}
	return s.WorkTask(ctx, actor, task.ID)
}

func (s *Service) startCurrentStage(ctx context.Context, actor Actor, task *domain.WorkTask, now time.Time, requestID string) error {
	runs, err := s.store.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return err
	}
	stageRun, err := currentStageRun(*task, runs)
	if err != nil {
		return err
	}
	if stageRun.Status == domain.StageRunStatusWaitingGate {
		return domain.Policy("STAGE_GATE_PENDING", "当前 Stage 正在等待 Gate 决定", "先处理 Gate 决定")
	}
	if stageRun.Status == domain.StageRunStatusCompleted {
		return domain.Conflict("STAGE_ALREADY_COMPLETED", "当前 Stage 已完成")
	}
	if stageRun.Status != domain.StageRunStatusRunning {
		stageRun.Status = domain.StageRunStatusRunning
		if stageRun.StartedAt == nil {
			started := now
			stageRun.StartedAt = &started
		}
		stageRun.UpdatedAt = now
		if err := s.store.SaveStageRun(ctx, stageRun); err != nil {
			return err
		}
	}
	task.Status = domain.TaskStatusRunning
	task.NextAction = "执行 " + stageRun.StageID + " 并上报结果"
	task.UpdatedAt = now
	if err := s.store.SaveWorkTask(ctx, *task); err != nil {
		return err
	}
	if err := s.ensureTaskRun(ctx, actor, *task, stageRun, now); err != nil {
		return err
	}
	s.audit(ctx, actor, task.ProjectID, "task.started", "task", task.ID, requestID, map[string]any{"stage_id": stageRun.StageID, "execution_mode": stageRun.ExecutionMode})
	return nil
}

func (s *Service) ensureTaskRun(ctx context.Context, actor Actor, task domain.WorkTask, stageRun domain.StageRun, now time.Time) error {
	runs, err := s.store.WorkTaskRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.StageID == stageRun.StageID && (run.State == "running" || run.State == "leased") {
			return nil
		}
	}
	inputSnapshot := domain.ContextSnapshot{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, BuilderVersion: "task-input/1.0", SchemaVersion: "contentcloud.task-input/1.0", InputVersions: map[string]string{}, ManifestHash: "sha256:" + strings.Repeat("0", 64), CreatedAt: now}
	if err := s.store.CreateSnapshot(ctx, inputSnapshot); err != nil {
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
	capabilityID := "contentcloud.task.stage"
	capabilityVersion := "1.0.0"
	if stageRun.ExecutionMode == "agent" {
		capabilityID = "contentcloud.agent.stage"
	}
	run := domain.TaskRun{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, WorkTaskID: task.ID, SOPID: task.SOPID, SOPVersion: task.SOPVersion, SOPDigest: task.SOPDigest, StageID: stageRun.StageID, ExecutionMode: defaultString(stageRun.ExecutionMode, "local"), ExecutorKind: defaultString(stageRun.ExecutionMode, "local"), InputSnapshotID: inputSnapshot.ID, IdempotencyKey: "work-task:" + task.ID + ":" + stageRun.StageID + ":" + domain.NewID(), TaskType: task.ContentType, CapabilityID: capabilityID, CapabilityVersion: capabilityVersion, InputSchema: "contentcloud.task-input/1.0", OutputSchema: stageOutputSchema(task, stageRun.StageID), OutputCount: 1, DeliveryProfiles: []string{"workspace"}, State: "running", Priority: priority, ProgressLabel: "Stage 已开始", CreatedAt: now, UpdatedAt: now}
	return s.store.CreateRun(ctx, run)
}

func stageOutputSchema(task domain.WorkTask, stageID string) string {
	return "contentcloud.stage/1.0#" + task.ContentType + "/" + stageID
}

func currentStageRun(task domain.WorkTask, runs []domain.StageRun) (domain.StageRun, error) {
	for _, run := range runs {
		if run.StageID == task.CurrentStageID {
			return run, nil
		}
	}
	return domain.StageRun{}, domain.NotFound("当前 StageRun")
}

func (s *Service) ReportStage(ctx context.Context, actor Actor, taskID string, input StageReportInput, requestID string) (WorkTaskView, error) {
	if actor.Type != "device" && actor.Type != "worker" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
			return WorkTaskView{}, err
		}
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if task.Status != domain.TaskStatusRunning {
		return WorkTaskView{}, domain.Conflict("TASK_NOT_RUNNING", "只有运行中的任务可以上报 Stage 结果")
	}
	runs, err := s.store.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	stageRun, err := currentStageRun(task, runs)
	if err != nil {
		return WorkTaskView{}, err
	}
	if input.StageRunID != "" && input.StageRunID != stageRun.ID {
		return WorkTaskView{}, domain.Conflict("STAGE_RUN_NOT_CURRENT", "上报的 StageRun 不是当前 Stage")
	}
	if input.StageID != "" && input.StageID != stageRun.StageID {
		return WorkTaskView{}, domain.Conflict("STAGE_NOT_CURRENT", "上报的 Stage 不是当前 Stage")
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = domain.StageRunStatusCompleted
	}
	now := s.now().UTC()
	if status == "failed" || status == domain.StageRunStatusBlocked {
		stageRun.Status = domain.StageRunStatusBlocked
		stageRun.OutputRefs = append([]string{}, input.OutputRefs...)
		stageRun.UpdatedAt = now
		if err := s.store.SaveStageRun(ctx, stageRun); err != nil {
			return WorkTaskView{}, err
		}
		task.Status = domain.TaskStatusBlocked
		task.NextAction = "修复输出后重试当前 Stage"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.updateActiveRun(ctx, actor.TenantID, task.ID, stageRun.StageID, "failed", input.OutputRefs, "STAGE_REPORT_FAILED", now)
		s.audit(ctx, actor, task.ProjectID, "stage.reported_failed", "stage_run", stageRun.ID, requestID, map[string]any{"error_code": input.ErrorCode, "summary": input.Summary})
		return s.WorkTask(ctx, actor, task.ID)
	}
	if status != domain.StageRunStatusCompleted {
		return WorkTaskView{}, domain.Invalid("STAGE_REPORT_STATUS_INVALID", "Stage 上报状态必须是 completed 或 failed")
	}
	stageRun.OutputRefs = append([]string{}, input.OutputRefs...)
	stageRun.UpdatedAt = now
	if err := s.store.SaveStageRun(ctx, stageRun); err != nil {
		return WorkTaskView{}, err
	}
	s.updateActiveRun(ctx, actor.TenantID, task.ID, stageRun.StageID, "succeeded", input.OutputRefs, "", now)
	if err := s.finishStageOrOpenGate(ctx, actor, &task, &stageRun, input, now, requestID); err != nil {
		return WorkTaskView{}, err
	}
	return s.WorkTask(ctx, actor, task.ID)
}

func (s *Service) updateActiveRun(ctx context.Context, tenantID, taskID, stageID, state string, outputRefs []string, errorCode string, now time.Time) {
	runs, err := s.store.WorkTaskRuns(ctx, tenantID, taskID)
	if err != nil {
		return
	}
	for index := len(runs) - 1; index >= 0; index-- {
		if runs[index].StageID == stageID && (runs[index].State == "running" || runs[index].State == "leased") {
			run := runs[index]
			run.State = state
			run.OutputRefs = append([]string{}, outputRefs...)
			run.ErrorCode = errorCode
			run.ProgressLabel = "Stage 已上报"
			run.UpdatedAt = now
			_ = s.store.SaveRun(ctx, run)
			return
		}
	}
}

func (s *Service) finishStageOrOpenGate(ctx context.Context, actor Actor, task *domain.WorkTask, stageRun *domain.StageRun, input StageReportInput, now time.Time, requestID string) error {
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
		if mode == domain.GateModeNone {
			continue
		}
		passed := checksPassed(checks, gate.Checks)
		if mode == domain.GateModeRequiredCheck {
			evaluationStatus := domain.GateEvaluationApproved
			if !passed {
				evaluationStatus = domain.GateEvaluationRejected
			}
			evaluation := domain.GateEvaluation{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: evaluationStatus, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, Decision: map[bool]string{true: "passed", false: "failed"}[passed], CreatedAt: now, UpdatedAt: now}
			if err := s.store.CreateGateEvaluation(ctx, evaluation); err != nil {
				return err
			}
			if !passed {
				stageRun.Status = domain.StageRunStatusBlocked
				task.Status = domain.TaskStatusBlocked
				task.NextAction = "修复检查失败后重试"
				stageRun.UpdatedAt = now
				if err := s.store.SaveStageRun(ctx, *stageRun); err != nil {
					return err
				}
				task.UpdatedAt = now
				return s.store.SaveWorkTask(ctx, *task)
			}
			continue
		}
		if mode == domain.GateModeAdvisory {
			evaluation := domain.GateEvaluation{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: domain.GateEvaluationApproved, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, Decision: "advisory_passed", CreatedAt: now, UpdatedAt: now}
			if err := s.store.CreateGateEvaluation(ctx, evaluation); err != nil {
				return err
			}
			continue
		}
		evaluation := domain.GateEvaluation{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, StageRunID: stageRun.ID, GateID: gate.ID, GateMode: mode, Status: domain.GateEvaluationPending, InputRefs: append([]string{}, stageRun.OutputRefs...), Checks: checks, CreatedAt: now, UpdatedAt: now}
		if gate.EscalationHours > 0 {
			expires := now.Add(time.Duration(gate.EscalationHours) * time.Hour)
			evaluation.ExpiresAt = &expires
		}
		if err := s.store.CreateGateEvaluation(ctx, evaluation); err != nil {
			return err
		}
		stageRun.Status = domain.StageRunStatusWaitingGate
		stageRun.UpdatedAt = now
		task.Status = domain.TaskStatusWaitingGate
		task.NextAction = "处理 Gate：" + gate.Name
		task.UpdatedAt = now
		if err := s.store.SaveStageRun(ctx, *stageRun); err != nil {
			return err
		}
		if err := s.store.SaveWorkTask(ctx, *task); err != nil {
			return err
		}
		s.audit(ctx, actor, task.ProjectID, "gate.created", "gate_evaluation", evaluation.ID, requestID, map[string]any{"gate_id": gate.ID, "mode": mode})
		return nil
	}
	return s.completeStage(ctx, task, stageRun, sop, now)
}

func (s *Service) DecideGate(ctx context.Context, actor Actor, taskID, gateID string, input GateDecisionInput, requestID string) (WorkTaskView, error) {
	if actor.Role != "client_approver" {
		if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
			return WorkTaskView{}, err
		}
	}
	if actor.UserID == "" {
		return WorkTaskView{}, domain.Policy("GATE_DECIDER_REQUIRED", "Gate 决定需要已认证成员", "请使用受邀的内部审核人或客户决定人账号")
	}
	if actor.Role == "client_approver" && actor.Type != "user" {
		return WorkTaskView{}, domain.Policy("CLIENT_APPROVER_SESSION_REQUIRED", "客户决定人必须通过成员账号登录", "请使用受邀客户决定人账号登录")
	}
	if actor.Role == "client_approver" && actor.TenantID == "" {
		return WorkTaskView{}, domain.Policy("CLIENT_APPROVER_TENANT_REQUIRED", "客户决定人缺少租户作用域", "请重新登录后重试")
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return WorkTaskView{}, err
	}
	evaluations, err := s.store.GateEvaluations(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	var evaluation domain.GateEvaluation
	for _, candidate := range evaluations {
		if candidate.ID == gateID || candidate.GateID == gateID {
			if candidate.Status == domain.GateEvaluationPending {
				evaluation = candidate
				break
			}
		}
	}
	if evaluation.ID == "" {
		return WorkTaskView{}, domain.NotFound("待决定 Gate")
	}
	if actor.Role == "client_approver" && evaluation.GateMode != domain.GateModeClientDecision {
		return WorkTaskView{}, domain.Policy("CLIENT_GATE_ROLE_REQUIRED", "客户决定人只能处理 client_decision Gate", "请由内部审核角色处理当前 Gate")
	}
	_, taskSOP, sopErr := s.loadTaskSOP(ctx, actor.TenantID, task)
	if sopErr != nil {
		return WorkTaskView{}, sopErr
	}
	gate, gateErr := gateDefinition(taskSOP, evaluation.GateID)
	if gateErr != nil {
		return WorkTaskView{}, gateErr
	}
	if evaluation.GateMode == domain.GateModeClientDecision && actor.Role != "client_approver" && actor.Role != "tenant_admin" {
		return WorkTaskView{}, domain.Policy("CLIENT_GATE_ROLE_REQUIRED", "client_decision Gate 只能由客户决定人或租户管理员处理", "请邀请客户决定人处理该 Gate")
	}
	if len(gate.AssigneeRoles) > 0 && actor.Role != "tenant_admin" && !containsString(gate.AssigneeRoles, actor.Role) {
		return WorkTaskView{}, domain.Policy("GATE_ASSIGNEE_ROLE_REQUIRED", "当前成员不在该 Gate 的决定角色范围内", "请由 Gate 配置的决定角色处理")
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != "approved" && decision != "rejected" && decision != "changes_requested" {
		return WorkTaskView{}, domain.Invalid("GATE_DECISION_INVALID", "Gate 决定必须是 approved、rejected 或 changes_requested")
	}
	now := s.now().UTC()
	evaluation.Status = map[string]string{"approved": domain.GateEvaluationApproved, "rejected": domain.GateEvaluationRejected, "changes_requested": domain.GateEvaluationChangesRequested}[decision]
	evaluation.Decision = decision
	evaluation.Reason = strings.TrimSpace(input.Reason)
	evaluation.DecidedBy = actor.UserID
	evaluation.DecidedAt = &now
	evaluation.UpdatedAt = now
	if err := s.store.SaveGateEvaluation(ctx, evaluation); err != nil {
		return WorkTaskView{}, err
	}
	runs, err := s.store.StageRuns(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	stageRun, err := stageRunByID(runs, evaluation.StageRunID)
	if err != nil {
		return WorkTaskView{}, err
	}
	if decision != "approved" {
		stageRun.Status = domain.StageRunStatusBlocked
		task.Status = domain.TaskStatusBlocked
		task.NextAction = "根据 Gate 意见修改并重试"
		stageRun.UpdatedAt = now
		task.UpdatedAt = now
		if err := s.store.SaveStageRun(ctx, stageRun); err != nil {
			return WorkTaskView{}, err
		}
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return WorkTaskView{}, err
		}
		s.audit(ctx, actor, task.ProjectID, "gate.decided", "gate_evaluation", evaluation.ID, requestID, map[string]any{"decision": decision, "reason": evaluation.Reason})
		return s.WorkTask(ctx, actor, task.ID)
	}
	allEvaluations, err := s.store.GateEvaluations(ctx, actor.TenantID, task.ID)
	if err != nil {
		return WorkTaskView{}, err
	}
	for _, candidate := range allEvaluations {
		if candidate.StageRunID == stageRun.ID && candidate.Status == domain.GateEvaluationPending {
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

func (s *Service) completeStage(ctx context.Context, task *domain.WorkTask, stageRun *domain.StageRun, sop domain.SOPVersion, now time.Time) error {
	stageRun.Status = domain.StageRunStatusCompleted
	completed := now
	stageRun.CompletedAt = &completed
	stageRun.UpdatedAt = now
	if err := s.store.SaveStageRun(ctx, *stageRun); err != nil {
		return err
	}
	var next *domain.StageDefinition
	for index := range sop.Stages {
		if sop.Stages[index].ID == stageRun.StageID && index+1 < len(sop.Stages) {
			candidate := sop.Stages[index+1]
			next = &candidate
			break
		}
	}
	if next == nil {
		task.Status = domain.TaskStatusAccepted
		task.CurrentStageID = ""
		task.NextAction = "提交 Revision"
	} else {
		task.Status = domain.TaskStatusReady
		task.CurrentStageID = next.ID
		task.NextAction = "开始 " + next.Name
		if err := s.store.CreateStageRun(ctx, domain.StageRun{ID: domain.NewID(), TenantID: task.TenantID, TaskID: task.ID, StageID: next.ID, Status: domain.StageRunStatusPending, ExecutionMode: defaultString(firstString(next.ExecutionModes), sop.DefaultExecutionMode), InputRefs: append([]string{}, next.InputRefs...), UpdatedAt: now}); err != nil {
			return err
		}
	}
	task.UpdatedAt = now
	return s.store.SaveWorkTask(ctx, *task)
}

func (s *Service) CreateTaskRevision(ctx context.Context, actor Actor, taskID string, input CreateTaskRevisionInput, requestID string) (domain.TaskRevision, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return domain.TaskRevision{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.TaskRevision{}, err
	}
	if task.Status == domain.TaskStatusCancelled || task.Status == domain.TaskStatusDelivered {
		return domain.TaskRevision{}, domain.Conflict("TASK_REVISION_NOT_ALLOWED", "已取消或已交付任务不能提交新 Revision")
	}
	contentType := defaultString(input.ContentType, task.ContentType)
	schemaVersion := defaultString(input.SchemaVersion, contentSchemaVersion(contentType))
	if err := validateTaskContent(contentType, schemaVersion, input.Content); err != nil {
		return domain.TaskRevision{}, err
	}
	revisions, err := s.store.TaskRevisions(ctx, actor.TenantID, task.ID)
	if err != nil {
		return domain.TaskRevision{}, err
	}
	revisionNo := len(revisions) + 1
	hash, err := domain.CanonicalHash(json.RawMessage(input.Content))
	if err != nil {
		return domain.TaskRevision{}, err
	}
	now := s.now().UTC()
	status := domain.TaskRevisionSubmitted
	if task.Status == domain.TaskStatusAccepted {
		status = domain.TaskRevisionAccepted
	}
	revision := domain.TaskRevision{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, RevisionNo: revisionNo, ContentType: contentType, SchemaVersion: schemaVersion, Content: append([]byte{}, input.Content...), ContentHash: "sha256:" + hash, SOPDigest: task.SOPDigest, KnowledgeSnapshotIDs: append([]string{}, input.KnowledgeSnapshotIDs...), EvidenceSummary: input.EvidenceSummary, RightsSummary: input.RightsSummary, Status: status, SubmittedBy: actor.UserID, SubmittedAt: &now, CreatedAt: now}
	revision.NormalizeCollections()
	if err := s.store.CreateTaskRevision(ctx, revision); err != nil {
		return domain.TaskRevision{}, err
	}
	if task.Status == domain.TaskStatusBlocked {
		task.Status = domain.TaskStatusReady
		task.NextAction = "重试当前 Stage"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return domain.TaskRevision{}, err
		}
	}
	if task.Status == domain.TaskStatusAccepted {
		task.NextAction = "交付 Revision"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return domain.TaskRevision{}, err
		}
	}
	s.audit(ctx, actor, task.ProjectID, "task.revision_submitted", "task_revision", revision.ID, requestID, map[string]any{"task_id": task.ID, "revision_no": revision.RevisionNo, "content_hash": revision.ContentHash, "schema_version": revision.SchemaVersion})
	return revision, nil
}

func (s *Service) CreateTaskDelivery(ctx context.Context, actor Actor, taskID string, input CreateTaskDeliveryInput, requestID string) (domain.TaskDelivery, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return domain.TaskDelivery{}, err
	}
	task, err := s.store.WorkTask(ctx, actor.TenantID, taskID)
	if err != nil {
		return domain.TaskDelivery{}, err
	}
	if task.Status != domain.TaskStatusAccepted {
		return domain.TaskDelivery{}, domain.Policy("TASK_NOT_ACCEPTED", "只有已接受任务可以交付", "先完成所有 Stage 并提交 Revision")
	}
	revisionID := input.RevisionID
	var revision domain.TaskRevision
	if revisionID != "" {
		revision, err = s.store.TaskRevision(ctx, actor.TenantID, revisionID)
	} else {
		var revisions []domain.TaskRevision
		revisions, err = s.store.TaskRevisions(ctx, actor.TenantID, task.ID)
		if err == nil {
			for index := len(revisions) - 1; index >= 0; index-- {
				if revisions[index].Status == domain.TaskRevisionAccepted {
					revision = revisions[index]
					break
				}
			}
		}
	}
	if err != nil {
		return domain.TaskDelivery{}, err
	}
	if revision.ID == "" || revision.TaskID != task.ID || revision.Status != domain.TaskRevisionAccepted {
		return domain.TaskDelivery{}, domain.Policy("TASK_REVISION_NOT_ACCEPTED", "交付必须引用当前任务的已接受 Revision", "先提交最终 Revision")
	}
	destination := defaultString(input.Destination, "workspace")
	manifest := append([]string{}, input.Manifest...)
	digest, err := domain.CanonicalHash(struct {
		TaskID      string   `json:"task_id"`
		RevisionID  string   `json:"revision_id"`
		Destination string   `json:"destination"`
		Manifest    []string `json:"manifest"`
	}{task.ID, revision.ID, destination, manifest})
	if err != nil {
		return domain.TaskDelivery{}, err
	}
	now := s.now().UTC()
	delivery := domain.TaskDelivery{ID: domain.NewID(), TenantID: task.TenantID, ProjectID: task.ProjectID, TaskID: task.ID, RevisionID: revision.ID, Destination: destination, Status: domain.TaskDeliveryReady, Manifest: manifest, DeliveryDigest: "sha256:" + digest, CreatedAt: now, UpdatedAt: now}
	delivery.NormalizeCollections()
	deliver := input.Deliver == nil || *input.Deliver
	if deliver {
		delivery.Status = domain.TaskDeliveryDelivered
		delivery.DeliveredBy = actor.UserID
		delivery.DeliveredAt = &now
	}
	if err := s.store.CreateTaskDelivery(ctx, delivery); err != nil {
		return domain.TaskDelivery{}, err
	}
	if deliver {
		task.Status = domain.TaskStatusDelivered
		task.NextAction = "已交付"
		task.UpdatedAt = now
		if err := s.store.SaveWorkTask(ctx, task); err != nil {
			return domain.TaskDelivery{}, err
		}
	}
	s.audit(ctx, actor, task.ProjectID, "task.delivery_created", "task_delivery", delivery.ID, requestID, map[string]any{"revision_id": revision.ID, "destination": destination, "status": delivery.Status})
	return delivery, nil
}

func (s *Service) loadTaskSOP(ctx context.Context, tenantID string, task domain.WorkTask) (domain.SOPDefinition, domain.SOPVersion, error) {
	summary, err := s.store.SOP(ctx, tenantID, task.SOPID)
	if err != nil {
		return domain.SOPDefinition{}, domain.SOPVersion{}, err
	}
	for _, version := range summary.Versions {
		if version.Version == task.SOPVersion {
			return summary.Definition, version, nil
		}
	}
	return domain.SOPDefinition{}, domain.SOPVersion{}, domain.NotFound("SOP 版本")
}

func stageDefinition(sop domain.SOPVersion, id string) (domain.StageDefinition, error) {
	for _, stage := range sop.Stages {
		if stage.ID == id {
			return stage, nil
		}
	}
	return domain.StageDefinition{}, domain.NotFound("Stage")
}

func gateDefinition(sop domain.SOPVersion, id string) (domain.GateDefinition, error) {
	for _, gate := range sop.Gates {
		if gate.ID == id {
			return gate, nil
		}
	}
	return domain.GateDefinition{}, domain.NotFound("Gate")
}

func stageRunByID(runs []domain.StageRun, id string) (domain.StageRun, error) {
	for _, run := range runs {
		if run.ID == id {
			return run, nil
		}
	}
	return domain.StageRun{}, domain.NotFound("StageRun")
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
	case domain.ContentTypeVideoScript:
		return "contentcloud.video_script/1.0"
	case domain.ContentTypeWeChatArticle:
		return "contentcloud.article/1.0"
	default:
		return "contentcloud.content/1.0"
	}
}

func validateTaskContent(contentType, schemaVersion string, content json.RawMessage) error {
	if !domain.ValidTenantContentType(contentType) {
		return domain.Invalid("TASK_CONTENT_TYPE_INVALID", "任务内容类型不受支持")
	}
	if len(content) == 0 || !json.Valid(content) {
		return domain.Invalid("TASK_CONTENT_INVALID", "Revision 内容必须是有效 JSON")
	}
	want := contentSchemaVersion(contentType)
	if schemaVersion != want && !(contentType == domain.ContentTypeVideoScript && schemaVersion == "contentcloud.content_batch/3.0") {
		return domain.Invalid("TASK_CONTENT_SCHEMA_INVALID", fmt.Sprintf("%s 必须使用 %s", contentType, want))
	}
	var object map[string]any
	if err := json.Unmarshal(content, &object); err != nil {
		return domain.Invalid("TASK_CONTENT_OBJECT_REQUIRED", "Revision 内容必须是对象")
	}
	if strings.TrimSpace(fmt.Sprint(object["title"])) == "" && strings.TrimSpace(fmt.Sprint(object["name"])) == "" {
		return domain.Invalid("TASK_CONTENT_TITLE_REQUIRED", "Revision 需要标题或名称")
	}
	if contentType == domain.ContentTypeVideoScript {
		if _, ok := object["scenes"]; !ok {
			if _, ok := object["items"]; !ok {
				return domain.Invalid("VIDEO_SCRIPT_SCENES_REQUIRED", "视频脚本 Revision 需要 scenes 或 items")
			}
		}
	}
	if contentType == domain.ContentTypeWeChatArticle {
		if _, ok := object["blocks"]; !ok {
			if _, ok := object["paragraphs"]; !ok {
				return domain.Invalid("ARTICLE_BLOCKS_REQUIRED", "文章 Revision 需要 blocks 或 paragraphs")
			}
		}
	}
	return nil
}
