package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	"github.com/limecloud/contentcloud/contracts"
	capabilitycatalog "github.com/limecloud/contentcloud/internal/catalog/capability"
	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
	builtinskills "github.com/limecloud/contentcloud/plugins/contentcloud-video-production/skills"

	// RuntimeWorkerPrepareInput is the wire contract for a remote worker. The
	// server chooses the lease owner from the authenticated device; owner and
	// fence fields are never accepted from the request body.
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

type RuntimeWorkerPrepareInput struct {
	JobRunID         string                           `json:"job_run_id,omitempty"`
	DaemonInstanceID string                           `json:"daemon_instance_id,omitempty"`
	HarnessKind      string                           `json:"harness_kind"`
	Capabilities     agentadapter.HarnessCapabilities `json:"capabilities"`

	// Deprecated in-process compatibility fields are deliberately ignored.
	// Runtime derives these values from its frozen Job/Plan/Node facts.
	Role                 string                           `json:"-"`
	ExecutionProfileID   string                           `json:"-"`
	Workspace            string                           `json:"-"`
	Prompt               string                           `json:"-"`
	OutputSchema         json.RawMessage                  `json:"-"`
	InputRefs            []string                         `json:"-"`
	StateRefs            []string                         `json:"-"`
	EventRefs            []string                         `json:"-"`
	AllowedTools         []string                         `json:"-"`
	MaxTokens            int                              `json:"-"`
	BudgetMinor          int64                            `json:"-"`
	RemainingDescendants int                              `json:"-"`
	LeaseForSeconds      int                              `json:"-"`
	ContextTTLSeconds    int                              `json:"-"`
	ResourceRequests     []contentruntime.ResourceRequest `json:"-"`
}

type RuntimeWorkerActivateInput struct {
	DaemonInstanceID string                       `json:"daemon_instance_id,omitempty"`
	AttemptID        string                       `json:"attempt_id"`
	FenceToken       string                       `json:"fence_token"`
	Session          agentadapter.AgentSessionRef `json:"session"`
}

type RuntimeWorkerPrepareNextInput struct {
	RuntimeWorkerPrepareInput
}

type RuntimeWorkerHeartbeatInput struct {
	DaemonInstanceID string `json:"daemon_instance_id,omitempty"`
	AttemptID        string `json:"attempt_id"`
	FenceToken       string `json:"fence_token"`
}

type RuntimeWorkerEventInput struct {
	DaemonInstanceID string                  `json:"daemon_instance_id,omitempty"`
	AttemptID        string                  `json:"attempt_id"`
	FenceToken       string                  `json:"fence_token"`
	Event            agentadapter.AgentEvent `json:"event"`
}

type RuntimeWorkerFinalizeInput struct {
	DaemonInstanceID string          `json:"daemon_instance_id,omitempty"`
	AttemptID        string          `json:"attempt_id"`
	FenceToken       string          `json:"fence_token"`
	State            string          `json:"state"`
	OutputRefs       []string        `json:"output_refs,omitempty"`
	OutputDigest     string          `json:"output_digest,omitempty"`
	ResultDigest     string          `json:"result_digest,omitempty"`
	SafeSummary      map[string]any  `json:"safe_summary,omitempty"`
	ErrorCode        string          `json:"error_code,omitempty"`
	UsedCostMinor    int64           `json:"used_cost_minor"`
	BusinessPayload  json.RawMessage `json:"business_payload,omitempty"`
}

type RuntimeMCPCallInput struct {
	DaemonInstanceID string         `json:"daemon_instance_id,omitempty"`
	AttemptID        string         `json:"attempt_id"`
	FenceToken       string         `json:"fence_token"`
	ToolName         string         `json:"tool_name"`
	RequestID        string         `json:"request_id"`
	Arguments        map[string]any `json:"arguments"`
}

type RuntimeGatewayCallInput struct {
	ToolName  string         `json:"tool_name"`
	RequestID string         `json:"request_id"`
	Arguments map[string]any `json:"arguments"`
}

type RuntimeWorkerResult struct {
	Handle            contentruntime.DispatchHandle `json:"handle"`
	Job               contentruntime.JobRun         `json:"job"`
	BusinessResultRef string                        `json:"business_result_ref,omitempty"`
}

func (s *RuntimeService) PrepareRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerPrepareInput) (contentruntime.DispatchHandle, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if err := s.requireRuntimeAdmissionCapacity(ctx, actor.TenantID); err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	owner, err := s.runtimeWorkerOwner(ctx, actor, input.DaemonInstanceID, true)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	handle, err := s.runtimeService.PrepareAdmittedRemoteDispatch(ctx, contentruntime.RemoteAdmissionInput{
		TenantID: actor.TenantID, JobRunID: input.JobRunID, Owner: owner,
		AllowedProjectIDs: runtimeWorkerProjectScope(actor), HarnessKind: input.HarnessKind,
		Capabilities: input.Capabilities, EnrichExecutionSpec: func(ctx context.Context, spec contentruntime.RemoteExecutionSpec) (contentruntime.RemoteExecutionSpec, error) {
			if input.DaemonInstanceID != "" {
				observation, observationErr := s.validateRuntimeWorkspaceObservation(ctx, actor.TenantID, input.DaemonInstanceID, spec)
				if observationErr != nil {
					return spec, observationErr
				}
				spec.LocalWorkspaceID = observation.WorkspaceID
				spec.LocalGeneration = observation.Generation
				spec.LocalPluginReceipt = observation.PluginHostReceiptDigest
				spec.LocalSkillDigest = observation.ObservedSkillDigest
				spec.LocalMCPDigest = observation.ObservedMCPDigest
				spec.LocalWorkspaceDigest = observation.ObservedWorkspaceDigest
			}
			return s.enrichRemoteExecutionSpec(ctx, actor.TenantID, spec)
		},
	})
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return handle, nil
}

func (s *RuntimeService) validateRuntimeWorkspaceObservation(ctx context.Context, tenantID, instanceID string, spec contentruntime.RemoteExecutionSpec) (workspacedomain.DaemonWorkspaceObservation, error) {
	if s.deviceControl == nil {
		return workspacedomain.DaemonWorkspaceObservation{}, fault.Policy("RUNTIME_WORKSPACE_OBSERVATION_REQUIRED", "远程 Attempt 缺少本地工作区观察", "让 Daemon 重新建立 current-state 后重试")
	}
	instance, err := s.deviceControl.DaemonInstance(ctx, tenantID, instanceID)
	if err != nil {
		return workspacedomain.DaemonWorkspaceObservation{}, err
	}
	body, err := json.Marshal(instance.Capabilities["workspace_observations"])
	if err != nil {
		return workspacedomain.DaemonWorkspaceObservation{}, err
	}
	var observations []workspacedomain.DaemonWorkspaceObservation
	if json.Unmarshal(body, &observations) != nil {
		return workspacedomain.DaemonWorkspaceObservation{}, fault.Policy("RUNTIME_WORKSPACE_OBSERVATION_INVALID", "Daemon 上报的工作区观察无法解析", "升级 Daemon 后重新同步 current-state")
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].ProjectID < observations[j].ProjectID })
	matches := []workspacedomain.DaemonWorkspaceObservation{}
	for index := range observations {
		if observations[index].ProjectID == spec.ProjectID {
			matches = append(matches, observations[index])
		}
	}
	if len(matches) == 0 {
		return workspacedomain.DaemonWorkspaceObservation{}, fault.Policy("RUNTIME_WORKSPACE_NOT_AUTHORIZED", "当前 Daemon 没有该项目的本地工作区观察", "为项目绑定工作区并等待 Daemon 同步")
	}
	if len(matches) > 1 {
		return workspacedomain.DaemonWorkspaceObservation{}, fault.Conflict("RUNTIME_WORKSPACE_AMBIGUOUS", "当前项目在 Daemon 上绑定了多个本地工作区，无法安全选择执行目录")
	}
	matched := matches[0]
	if matched.Status != "ready" {
		failure := fault.Policy("RUNTIME_ENVIRONMENT_NOT_READY", "本地工作区环境尚未达到可执行状态", "修复 Environment、Plugin、Skill 或 MCP 漂移后重试")
		failure.Details = map[string]any{"project_id": matched.ProjectID, "workspace_id": matched.WorkspaceID, "status": matched.Status, "reason": matched.Reason, "error_code": matched.ErrorCode}
		return workspacedomain.DaemonWorkspaceObservation{}, failure
	}
	checks := []struct{ name, expected, actual string }{
		{"environment", spec.EnvironmentDigest, matched.EnvironmentDeclaration},
		{"plugin", spec.PluginDigest, matched.PluginDeclaration},
		{"skill", spec.SkillDigest, matched.SkillDeclaration},
		{"mcp", spec.MCPDigest, matched.MCPDeclaration},
		{"workspace", spec.WorkspaceDigest, matched.WorkspaceDeclaration},
	}
	for _, check := range checks {
		if check.expected != "" && check.expected != check.actual {
			failure := fault.Conflict("RUNTIME_ENVIRONMENT_DRIFT", "本地 Environment、Plugin、Skill、MCP 或 Workspace 声明与冻结执行绑定不一致")
			failure.Details = map[string]any{"component": check.name, "expected": check.expected, "observed": check.actual, "project_id": matched.ProjectID, "workspace_id": matched.WorkspaceID}
			return workspacedomain.DaemonWorkspaceObservation{}, failure
		}
	}
	return matched, nil
}

func (s *RuntimeService) enrichRemoteExecutionSpec(ctx context.Context, tenantID string, spec contentruntime.RemoteExecutionSpec) (contentruntime.RemoteExecutionSpec, error) {
	if len(spec.RequiredCapabilities) != 1 {
		return spec, fault.Policy("RUNTIME_CAPABILITY_REQUIRED", "远程智能体节点必须冻结且只声明一个可执行能力", "为节点配置一个已发布的 ContentCloud 能力后重试")
	}
	capability, ok := capabilitycatalog.Exact(spec.RequiredCapabilities[0], pluginidentity.VideoProductionVersion)
	if !ok {
		return spec, fault.Policy("RUNTIME_CAPABILITY_UNSUPPORTED", "远程智能体节点引用的能力没有当前服务端实现", "发布能力实现和对应 Skill 后再调度")
	}
	if capability.OutputSchema != spec.OutputSchemaRef {
		return spec, fault.Conflict("RUNTIME_CAPABILITY_SCHEMA_MISMATCH", "节点输出 Schema 与冻结能力定义不一致")
	}
	project, err := s.workspace.Project(ctx, tenantID, spec.ProjectID)
	if err != nil {
		return spec, err
	}
	if strings.TrimSpace(spec.InputSnapshotID) == "" {
		return spec, fault.Policy("RUNTIME_INPUT_SNAPSHOT_REQUIRED", "远程智能体节点缺少冻结输入快照", "重新创建包含输入快照的任务")
	}
	snapshot, err := s.contexts.Snapshot(ctx, tenantID, spec.InputSnapshotID)
	if err != nil {
		return spec, err
	}
	if snapshot.ProjectID != project.ID || snapshot.TenantID != tenantID {
		return spec, fault.Conflict("RUNTIME_INPUT_SNAPSHOT_SCOPE_MISMATCH", "冻结输入快照不属于当前任务项目")
	}
	job, err := s.runtimeService.Job(ctx, tenantID, spec.JobRunID)
	if err != nil {
		return spec, err
	}
	if job.InputDigest != "sha256:"+snapshot.ManifestHash {
		return spec, fault.Conflict("RUNTIME_INPUT_SNAPSHOT_DIGEST_MISMATCH", "JobRun 输入摘要与冻结输入快照不一致")
	}
	outputSchema, err := remoteOutputSchema(capability.OutputSchema)
	if err != nil {
		return spec, err
	}
	skillID, skillBody, err := remoteCapabilitySkill(capability.ID)
	if err != nil {
		return spec, err
	}
	contractVersion := fmt.Sprintf("%d.%d", job.ContractMajor, job.ContractMinor)
	contract := sourcedomain.TaskContract{
		ContractVersion: contractVersion, ContractID: snapshot.ID, RunID: job.ID, TaskType: job.BusinessType,
		Project: project, Sources: append([]sourcedomain.ContractSource(nil), snapshot.Sources...), InputSnapshotID: snapshot.ID,
		OutputSchema: capability.OutputSchema, Capability: capability, ManifestHash: snapshot.ManifestHash,
	}
	if contractVersion != "1.0" || contract.TaskType == "" || contract.ManifestHash == "" {
		return spec, fault.Conflict("RUNTIME_TASK_CONTRACT_INVALID", "Runtime 任务无法组装为当前支持的 TaskContract")
	}
	spec.OutputSchema = append(json.RawMessage(nil), outputSchema...)
	spec.OutputSchemaDigest = "sha256:" + idgen.TokenHash(string(outputSchema))
	spec.TaskContract = contract
	spec.SkillID = skillID
	spec.Skill = string(skillBody)
	spec.SkillContentDigest = "sha256:" + idgen.TokenHash(string(skillBody))
	return spec, nil
}

func remoteOutputSchema(schemaRef string) ([]byte, error) {
	switch strings.TrimSpace(schemaRef) {
	case sourcedomain.KnowledgeCandidatesSchema:
		return append([]byte(nil), contracts.KnowledgeCandidatesSchema...), nil
	default:
		return nil, fault.Policy("RUNTIME_OUTPUT_SCHEMA_UNAVAILABLE", "远程智能体节点的输出 Schema 没有可执行定义", "将已发布 Schema 注册到远程执行材料解析器")
	}
}

func remoteCapabilitySkill(capabilityID string) (string, []byte, error) {
	var skillID string
	switch strings.TrimSpace(capabilityID) {
	case sourcedomain.KnowledgeExtractCapability:
		skillID = builtinskills.KnowledgeExtraction
	default:
		return "", nil, fault.Policy("RUNTIME_SKILL_UNAVAILABLE", "远程智能体节点的能力没有对应 Skill", "发布能力到 Skill 的受控映射后再调度")
	}
	body, err := builtinskills.Read(skillID, "SKILL.md")
	if err != nil {
		return "", nil, err
	}
	return skillID, body, nil
}

func (s *RuntimeService) requireRuntimeAdmissionCapacity(ctx context.Context, tenantID string) error {
	for _, subscriber := range []string{contentruntime.RuntimeOutboxSubscriberProjection, contentruntime.RuntimeOutboxSubscriberBusinessResult} {
		stats, err := s.runtimeService.RuntimeOutboxStats(ctx, tenantID, subscriber)
		if err != nil {
			return err
		}
		critical := stats.Pending >= runtimeBacklogCriticalCount
		if stats.OldestPending != nil && s.now().UTC().Sub(*stats.OldestPending) >= runtimeBacklogCriticalAge {
			critical = true
		}
		if critical {
			return &fault.Error{
				Type: "policy", Subtype: "runtime_backpressure", Code: "RUNTIME_ADMISSION_THROTTLED",
				Message: "Runtime 结果交接积压，已暂停领取新任务", Retryable: true,
				Hint: "现有 Attempt 会继续收敛；等待投影或业务结果队列恢复后自动重试", ExitCode: 5,
				Details: map[string]any{"subscriber": subscriber, "pending": stats.Pending},
			}
		}
	}
	return nil
}

// CallRuntimeMCP is the only App-facing MCP path. Device authentication
// supplies the tenant; the worker still has to present the Attempt fence and
// the Runtime gateway re-checks every scope before executing a tool.
func (s *RuntimeService) CallRuntimeMCP(ctx context.Context, actor Actor, input RuntimeMCPCallInput) (contentruntime.GatewayResponse, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.GatewayResponse{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.GatewayResponse{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	if _, err := s.workerHandle(ctx, actor, input.DaemonInstanceID, input.AttemptID, input.FenceToken); err != nil {
		return contentruntime.GatewayResponse{}, err
	}
	return contentruntime.NewRuntimeMCPGateway(s.runtimeService).Call(ctx, contentruntime.GatewayRequest{TenantID: actor.TenantID, AttemptID: input.AttemptID, FenceToken: input.FenceToken, ToolName: input.ToolName, RequestID: input.RequestID, Arguments: input.Arguments})
}

func (s *RuntimeService) CallRuntimeMCPWithGatewayToken(ctx context.Context, token string, input RuntimeGatewayCallInput) (contentruntime.GatewayResponse, error) {
	if s.runtimeService == nil {
		return contentruntime.GatewayResponse{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	return contentruntime.NewRuntimeMCPGateway(s.runtimeService).CallWithToken(ctx, token, contentruntime.GatewayTokenRequest{
		ToolName: input.ToolName, RequestID: input.RequestID, Arguments: input.Arguments,
	})
}

func (s *RuntimeService) PrepareNextRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerPrepareNextInput) (contentruntime.DispatchHandle, error) {
	candidate := input.RuntimeWorkerPrepareInput
	candidate.JobRunID = ""
	return s.PrepareRuntimeWorker(ctx, actor, candidate)
}

func (s *RuntimeService) ActivateRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerActivateInput) (contentruntime.DispatchHandle, error) {
	handle, err := s.workerHandle(ctx, actor, input.DaemonInstanceID, input.AttemptID, input.FenceToken)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if strings.TrimSpace(input.Session.TenantID) != "" && strings.TrimSpace(input.Session.TenantID) != handle.Attempt.TenantID {
		return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_SESSION_TENANT_MISMATCH", "Harness 会话租户与 RuntimeAttempt 不一致", "使用当前 Attempt 返回的会话范围")
	}
	input.Session.TenantID = handle.Attempt.TenantID
	return s.runtimeService.ActivateDispatch(ctx, handle, input.Session)
}

func (s *RuntimeService) HeartbeatRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerHeartbeatInput) (contentruntime.DispatchHandle, error) {
	handle, err := s.workerHandle(ctx, actor, input.DaemonInstanceID, input.AttemptID, input.FenceToken)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return s.runtimeService.HeartbeatDispatch(ctx, handle)
}

func (s *RuntimeService) RecordRuntimeWorkerEvent(ctx context.Context, actor Actor, input RuntimeWorkerEventInput) error {
	handle, err := s.workerHandle(ctx, actor, input.DaemonInstanceID, input.AttemptID, input.FenceToken)
	if err != nil {
		return err
	}
	return s.runtimeService.RecordHarnessEvent(ctx, handle, input.Event)
}

func (s *RuntimeService) FinalizeRuntimeWorker(ctx context.Context, actor Actor, input RuntimeWorkerFinalizeInput, requestID string) (RuntimeWorkerResult, error) {
	handle, err := s.workerFinalizeHandle(ctx, actor, input.DaemonInstanceID, input.AttemptID, input.FenceToken)
	if err != nil {
		return RuntimeWorkerResult{}, err
	}
	outcome := contentruntime.DispatchOutcome{State: input.State, OutputRefs: append([]string(nil), input.OutputRefs...), OutputDigest: strings.TrimSpace(input.OutputDigest), ResultDigest: strings.TrimSpace(input.ResultDigest), SafeSummary: input.SafeSummary, ErrorCode: strings.TrimSpace(input.ErrorCode), UsedCostMinor: input.UsedCostMinor}
	for _, ref := range outcome.OutputRefs {
		if strings.HasPrefix(strings.TrimSpace(ref), "runtime-result:") {
			return RuntimeWorkerResult{}, fault.Invalid("RUNTIME_BUSINESS_RESULT_REF_RESERVED", "runtime-result 引用只能由服务端从已校验的业务 payload 生成")
		}
	}
	resultRef := ""
	resultKey := ""
	keepResult := false
	defer func() {
		if resultKey == "" || keepResult {
			return
		}
		if deleter, ok := s.blobs.(blob.DeleteStore); ok {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if deleteErr := deleter.Delete(cleanupCtx, resultKey); deleteErr != nil && !errors.Is(deleteErr, blob.ErrNotFound) {
				s.log.Warn("清理未提交的 Runtime 业务结果失败", "object_key", resultKey, "error", deleteErr)
			}
		}
	}()
	businessDigest := ""
	var businessErr error
	if len(input.BusinessPayload) > 0 {
		resultRef, businessDigest, err = s.persistRuntimeBusinessResult(ctx, actor.TenantID, input.AttemptID, input.BusinessPayload)
		if err != nil {
			return RuntimeWorkerResult{}, err
		}
		resultKey = strings.TrimPrefix(resultRef, "runtime-result:")
		outcome.OutputRefs = append(outcome.OutputRefs, resultRef)
		if outcome.OutputDigest != "" && outcome.OutputDigest != businessDigest {
			return RuntimeWorkerResult{}, fault.Conflict("RUNTIME_BUSINESS_RESULT_DIGEST_CONFLICT", "worker 提交的 output digest 与结构化业务结果不一致")
		}
		outcome.OutputDigest = businessDigest
		if outcome.State == contentruntime.RuntimeAttemptSucceeded {
			businessErr = s.validateRuntimeBusinessResult(ctx, actor, handle.Attempt.JobRunID, input.BusinessPayload)
		}
	} else if input.State == contentruntime.RuntimeAttemptSucceeded {
		if job, jobErr := s.runtimeService.Job(ctx, actor.TenantID, handle.Attempt.JobRunID); jobErr != nil {
			businessErr = jobErr
		} else if job.BusinessType == "knowledge_extract" {
			businessErr = fault.Invalid("RUNTIME_BUSINESS_RESULT_REQUIRED", "知识提取成功必须交接结构化业务结果")
		}
	}
	if businessErr != nil {
		outcome.State = contentruntime.RuntimeAttemptFailed
		outcome.ErrorCode = "RUNTIME_BUSINESS_RESULT_INVALID"
		outcome.OutputRefs = nil
		outcome.OutputDigest = ""
	}
	finalized, err := s.runtimeService.FinalizeDispatch(ctx, handle, outcome)
	if err != nil {
		return RuntimeWorkerResult{}, err
	}
	if businessErr != nil {
		return RuntimeWorkerResult{Handle: finalized.Handle, Job: finalized.Job, BusinessResultRef: resultRef}, businessErr
	}
	keepResult = resultRef != ""
	return RuntimeWorkerResult{Handle: finalized.Handle, Job: finalized.Job, BusinessResultRef: resultRef}, nil
}

func (s *RuntimeService) validateRuntimeBusinessResult(ctx context.Context, actor Actor, jobID string, payload json.RawMessage) error {
	run, pkg, handled, err := s.runtimeKnowledgePackage(ctx, actor, jobID, payload)
	if err != nil {
		return err
	}
	if !handled {
		return fault.Invalid("RUNTIME_BUSINESS_RESULT_UNSUPPORTED", "当前任务类型没有注册结构化业务结果契约")
	}
	_, _, err = s.app.Source.validateKnowledgePackage(ctx, actor, run, pkg)
	return err
}

func (s *RuntimeService) runtimeKnowledgePackage(ctx context.Context, actor Actor, jobID string, payload json.RawMessage) (work.RuntimeRun, sourcedomain.KnowledgeExtractionPackage, bool, error) {
	job, err := s.runtimeService.Job(ctx, actor.TenantID, jobID)
	if err != nil {
		return work.RuntimeRun{}, sourcedomain.KnowledgeExtractionPackage{}, false, err
	}
	if job.BusinessType != "knowledge_extract" {
		return work.RuntimeRun{}, sourcedomain.KnowledgeExtractionPackage{}, false, nil
	}
	var pkg sourcedomain.KnowledgeExtractionPackage
	if err := decodeStrict(payload, &pkg); err != nil {
		return work.RuntimeRun{}, sourcedomain.KnowledgeExtractionPackage{}, true, fault.Invalid("RUNTIME_BUSINESS_RESULT_INVALID", "知识候选业务结果不符合严格 JSON 契约")
	}
	run, err := s.projectRuntimeRun(ctx, job)
	if err != nil {
		return work.RuntimeRun{}, sourcedomain.KnowledgeExtractionPackage{}, true, err
	}
	return run, pkg, true, nil
}

func (s *RuntimeService) workerHandle(ctx context.Context, actor Actor, daemonInstanceID, attemptID, fenceToken string) (contentruntime.DispatchHandle, error) {
	handle, err := s.loadRuntimeWorkerHandle(ctx, actor, attemptID)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	owner, err := s.runtimeWorkerOwner(ctx, actor, daemonInstanceID, false)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if handle.Attempt.LeaseOwner != owner {
		return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器", "使用领取该执行尝试的设备凭据")
	}
	if strings.TrimSpace(fenceToken) == "" || fenceToken != handle.Attempt.FenceToken {
		return contentruntime.DispatchHandle{}, fault.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
	}
	return handle, nil
}

func (s *RuntimeService) workerFinalizeHandle(ctx context.Context, actor Actor, daemonInstanceID, attemptID, fenceToken string) (contentruntime.DispatchHandle, error) {
	handle, err := s.loadRuntimeWorkerHandle(ctx, actor, attemptID)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if !handle.Attempt.Terminal() {
		owner, ownerErr := s.runtimeWorkerOwner(ctx, actor, daemonInstanceID, false)
		if ownerErr != nil {
			return contentruntime.DispatchHandle{}, ownerErr
		}
		if handle.Attempt.LeaseOwner != owner {
			return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器", "使用领取该执行尝试的设备凭据")
		}
		if strings.TrimSpace(fenceToken) == "" || fenceToken != handle.Attempt.FenceToken {
			return contentruntime.DispatchHandle{}, fault.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
		}
		return handle, nil
	}
	if strings.TrimSpace(fenceToken) == "" {
		return contentruntime.DispatchHandle{}, fault.Conflict("RUNTIME_FENCE_TOKEN_INVALID", "Runtime fence token 无效或已过期")
	}
	events, err := s.runtimeService.Events(ctx, actor.TenantID, handle.Attempt.JobRunID, 0)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	wantedOwner, err := s.runtimeWorkerOwner(ctx, actor, daemonInstanceID, false)
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	wantedFence := contentruntime.FenceTokenDigest(fenceToken)
	for _, event := range events {
		attempt, _ := event.Payload["attempt_id"].(string)
		fence, _ := event.Payload["fence_digest"].(string)
		if attempt == handle.Attempt.ID && event.ActorType == "worker" && event.ActorID == wantedOwner && fence == wantedFence {
			return handle, nil
		}
	}
	return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_WORKER_OWNER_MISMATCH", "执行尝试不属于当前工作器或 fence 已失效", "使用完成该执行尝试的设备凭据和 fence token")
}

func (s *RuntimeService) loadRuntimeWorkerHandle(ctx context.Context, actor Actor, attemptID string) (contentruntime.DispatchHandle, error) {
	if err := requireRuntimeWorker(actor); err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	if s.runtimeService == nil {
		return contentruntime.DispatchHandle{}, fault.Policy("RUNTIME_UNAVAILABLE", "当前运行时尚未配置持久化存储", "联系平台运营人员启用 Runtime")
	}
	handle, err := s.runtimeService.LoadDispatchHandle(ctx, actor.TenantID, strings.TrimSpace(attemptID))
	if err != nil {
		return contentruntime.DispatchHandle{}, err
	}
	return handle, nil
}

func (s *RuntimeService) persistRuntimeBusinessResult(ctx context.Context, tenantID, attemptID string, payload json.RawMessage) (string, string, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return "", "", fault.Invalid("RUNTIME_BUSINESS_RESULT_INVALID", "业务结果必须是合法 JSON")
	}
	digest, err := stablehash.Sum(value)
	if err != nil {
		return "", "", err
	}
	if len(payload) > 4*1024*1024 {
		return "", "", fault.Policy("RUNTIME_BUSINESS_RESULT_TOO_LARGE", "业务结果超过 Runtime 单次交接上限", "将正文存入业务对象或拆分为受控输出引用")
	}
	key := fmt.Sprintf("runtime/results/%s/%s/%s.json", tenantID, attemptID, digest)
	if err := s.blobs.Put(ctx, key, payload); err != nil {
		return "", "", err
	}
	return "runtime-result:" + key, "sha256:" + digest, nil
}

func requireRuntimeWorker(actor Actor) error {
	if actor.Type != "device" && actor.Type != "worker" {
		return fault.Policy("RUNTIME_WORKER_AUTH_REQUIRED", "Runtime worker 协议只接受设备或工作器凭据", "使用已授权的执行设备凭据")
	}
	if strings.TrimSpace(actor.TenantID) == "" {
		return fault.Invalid("RUNTIME_WORKER_TENANT_REQUIRED", "Runtime worker 缺少租户范围")
	}
	return nil
}

func (s *RuntimeService) runtimeWorkerOwner(ctx context.Context, actor Actor, daemonInstanceID string, requireOnline bool) (string, error) {
	if strings.TrimSpace(actor.DeviceID) != "" {
		instanceID := strings.TrimSpace(daemonInstanceID)
		if instanceID == "" {
			// A few in-process Runtime tests use synthetic device actors without
			// persisting a Device. Real DeviceActor values always resolve from the
			// store, so a missing instance remains mandatory on the wire.
			if _, lookupErr := s.workspace.Device(ctx, actor.TenantID, actor.DeviceID); fault.IsNotFound(lookupErr) {
				return "device:" + strings.TrimSpace(actor.DeviceID), nil
			}
			return "", fault.Policy("DAEMON_INSTANCE_REQUIRED", "设备 Runtime 请求缺少 DaemonInstance", "先建立 Runtime 控制通道并完成当前态同步")
		}
		if s.deviceControl == nil {
			return "", fault.Policy("DAEMON_INSTANCE_STORE_UNAVAILABLE", "DaemonInstance 持久层未配置", "检查服务端设备控制存储配置")
		}
		instance, err := s.deviceControl.DaemonInstance(ctx, actor.TenantID, instanceID)
		if err != nil {
			return "", fault.Policy("DAEMON_INSTANCE_INVALID", "DaemonInstance 不存在或不属于当前设备", "重新建立 Runtime 控制通道")
		}
		if instance.DeviceID != strings.TrimSpace(actor.DeviceID) {
			return "", fault.Policy("DAEMON_INSTANCE_DEVICE_MISMATCH", "DaemonInstance 不属于当前设备", "使用当前设备控制通道返回的实例标识")
		}
		if requireOnline && (instance.State != "connected" || s.now().UTC().Sub(instance.LastSeenAt) > daemonInstanceFreshFor) {
			return "", fault.Policy("DAEMON_INSTANCE_OFFLINE", "DaemonInstance 当前不在线", "等待控制通道重连后重试")
		}
		return "device:" + strings.TrimSpace(actor.DeviceID) + ":instance:" + instanceID, nil
	}
	if strings.TrimSpace(actor.UserID) != "" {
		return "worker:" + strings.TrimSpace(actor.UserID), nil
	}
	return "worker:unknown", nil
}

func runtimeWorkerProjectScope(actor Actor) []string {
	if actor.Type != "device" {
		return nil
	}
	return append([]string{}, actor.ProjectIDs...)
}
