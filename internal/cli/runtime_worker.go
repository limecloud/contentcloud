package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

// runtimeWorkerCommand is the current worker surface. Runtime owns leasing,
// fencing, heartbeats, and result handoff for every invocation.
type runtimeWorkerRunOptions struct {
	Fixture     bool
	HarnessKind string
	Role        string
	Profile     string
	Workspace   string
	Prompt      string
	ResultFile  string
}

func (r *Root) runtimeWorkerCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "runtime-worker", Short: "通过 Runtime Attempt/fence 协议执行一个远程 Runtime 节点"}
	var once, fixture bool
	var harnessKind, role, profile, workspace, prompt, resultFile string
	run := &cobra.Command{Use: "run", Short: "领取、激活、续租并收敛一个 Runtime 节点", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		binding, ok := cfg.PrimaryBinding()
		if !ok {
			return domain.Conflict("DEVICE_BINDING_MISSING", "Runtime worker 启动前必须先完成设备注册")
		}
		token, err := localconfig.DeviceToken(binding.DeviceID)
		if err != nil {
			return err
		}
		server := r.resolveServer(cfg)
		client := apiclient.New(server, token)
		result, err := r.runRuntimeWorker(cmd.Context(), client, runtimeWorkerRunOptions{Fixture: fixture, HarnessKind: harnessKind, Role: role, Profile: profile, Workspace: workspace, Prompt: prompt, ResultFile: resultFile}, once)
		if err != nil {
			return err
		}
		return r.writeOK("runtime-worker.run", result)
	}}
	run.Flags().BoolVar(&once, "once", false, "没有可调度节点时返回成功")
	run.Flags().BoolVar(&fixture, "fixture", false, "使用确定性 JSON 结果，不启动本地 Agent")
	run.Flags().StringVar(&harnessKind, "harness", "", "Runtime Harness 类型")
	run.Flags().StringVar(&role, "role", "worker", "Runtime Agent 角色")
	run.Flags().StringVar(&profile, "execution-profile", "runtime-worker-v1", "执行配置 ID")
	run.Flags().StringVar(&workspace, "workspace", "", "本地执行工作区")
	run.Flags().StringVar(&prompt, "prompt", "", "传给本地 Agent 的提示摘要")
	run.Flags().StringVar(&resultFile, "result-file", "", "读取结构化业务结果 JSON 的文件")
	cmd.AddCommand(run)
	return cmd
}

func (r *Root) runRuntimeWorker(ctx context.Context, client *apiclient.Client, options runtimeWorkerRunOptions, once bool) (map[string]any, error) {
	fixture := options.Fixture || strings.TrimSpace(options.ResultFile) != ""
	harnessKind, harness, capabilities, err := r.resolveRuntimeWorkerHarness(ctx, options.HarnessKind, fixture)
	if err != nil {
		return nil, err
	}
	role := strings.TrimSpace(options.Role)
	if role == "" {
		role = "worker"
	}
	profile := strings.TrimSpace(options.Profile)
	if profile == "" {
		profile = "runtime-worker-v1"
	}
	var handle contentruntime.DispatchHandle
	prepare := app.RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: app.RuntimeWorkerPrepareInput{HarnessKind: harnessKind, Capabilities: capabilities, Role: role, ExecutionProfileID: profile, Workspace: options.Workspace, Prompt: options.Prompt, MaxTokens: 8192}}
	if err := client.Dispatch(ctx, "runtime.worker.prepare_next", prepare, &handle); err != nil {
		if once && domain.IsNotFound(err) {
			return map[string]any{"leased": false}, nil
		}
		return nil, err
	}
	if fixture {
		return r.runFixtureRuntimeWorker(ctx, client, handle, harnessKind, options)
	}
	var session agentadapter.AgentSessionRef
	var stream agentadapter.EventStream
	if handle.ResumeSession != nil {
		session = *handle.ResumeSession
		stream, err = harness.Resume(ctx, agentadapter.ResumeAgentRequest{TenantID: handle.Attempt.TenantID, Session: session, Workspace: options.Workspace, Prompt: options.Prompt, ContextDigest: handle.ContextView.Digest})
	} else {
		session, stream, err = harness.Start(ctx, agentadapter.StartAgentRequest{TenantID: handle.Attempt.TenantID, JobRunID: handle.Attempt.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Workspace: options.Workspace, Prompt: options.Prompt, ContextDigest: handle.ContextView.Digest})
	}
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptRetryableFailed, err)
	}
	if stream == nil {
		err = domain.Conflict("HARNESS_STREAM_MISSING", "Runtime Harness 未返回结构化事件流")
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptRetryableFailed, err)
	}
	defer stream.Close()
	if err := client.Dispatch(ctx, "runtime.worker.activate", app.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: session}, &handle); err != nil {
		_ = harness.Interrupt(context.WithoutCancel(ctx), session)
		return nil, err
	}
	return r.driveRuntimeWorker(ctx, client, harness, session, stream, handle, harnessKind)
}

func (r *Root) runFixtureRuntimeWorker(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, harnessKind string, options runtimeWorkerRunOptions) (map[string]any, error) {
	session := agentadapter.AgentSessionRef{TenantID: handle.Attempt.TenantID, HarnessKind: harnessKind, SessionID: "fixture:" + handle.Attempt.ID}
	if err := client.Dispatch(ctx, "runtime.worker.activate", app.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: session}, &handle); err != nil {
		return nil, err
	}
	payload, err := runtimeWorkerPayload(options.Fixture, options.ResultFile)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptFailed, err)
	}
	return r.finalizeRuntimeWorkerSuccess(ctx, client, handle, harnessKind, payload)
}

func (r *Root) driveRuntimeWorker(ctx context.Context, client *apiclient.Client, harness agentadapter.AgentHarnessAdapter, session agentadapter.AgentSessionRef, stream agentadapter.EventStream, handle contentruntime.DispatchHandle, harnessKind string) (map[string]any, error) {
	heartbeat := time.NewTimer(runtimeWorkerHeartbeatDelay(handle))
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = harness.Interrupt(context.WithoutCancel(ctx), session)
			return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, domain.RuntimeAttemptRetryableFailed, ctx.Err())
		case <-heartbeat.C:
			if err := client.Dispatch(ctx, "runtime.worker.heartbeat", app.RuntimeWorkerHeartbeatInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken}, &handle); err != nil {
				_ = harness.Interrupt(context.WithoutCancel(ctx), session)
				return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, domain.RuntimeAttemptRetryableFailed, err)
			}
			heartbeat.Reset(runtimeWorkerHeartbeatDelay(handle))
		case event, ok := <-stream.Events():
			if !ok {
				err := domain.Conflict("HARNESS_STREAM_CLOSED", "Runtime Harness 在提交结构化结果前关闭了事件流")
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptRetryableFailed, err)
			}
			if event.Session != session {
				err := domain.Conflict("HARNESS_SESSION_MISMATCH", "Runtime Harness 事件会话与当前 Attempt 不一致")
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptFailed, err)
			}
			switch event.Type {
			case "result.completed":
				return r.finalizeRuntimeWorkerSuccess(ctx, client, handle, harnessKind, event.Data)
			case "session.failed", "session.interrupted":
				code := strings.TrimSpace(event.ErrorCode)
				if code == "" {
					code = "HARNESS_SESSION_FAILED"
				}
				err := &domain.Error{Type: "runtime", Subtype: harnessKind, Code: code, Message: "Runtime Harness 会话未完成", Retryable: true, ExitCode: 5}
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptRetryableFailed, err)
			default:
				if err := client.Dispatch(ctx, "runtime.worker.event", app.RuntimeWorkerEventInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Event: event}, nil); err != nil {
					_ = harness.Interrupt(context.WithoutCancel(ctx), session)
					return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, domain.RuntimeAttemptRetryableFailed, err)
				}
			}
		}
	}
}

func (r *Root) finalizeRuntimeWorkerSuccess(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, harnessKind string, payload json.RawMessage) (map[string]any, error) {
	input, err := runtimeWorkerFinalizeInput(handle, harnessKind, payload)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, domain.RuntimeAttemptFailed, err)
	}
	var finalized app.RuntimeWorkerResult
	if err := client.Dispatch(ctx, "runtime.worker.finalize", input, &finalized); err != nil {
		return nil, err
	}
	return map[string]any{"leased": true, "attempt_id": handle.Attempt.ID, "job_run_id": handle.Attempt.JobRunID, "business_result_ref": finalized.BusinessResultRef, "state": finalized.Handle.Attempt.State}, nil
}

func (r *Root) finalizeRuntimeWorkerError(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, state string, runErr error) error {
	code := "RUNTIME_WORKER_EXECUTION_FAILED"
	var de *domain.Error
	if errors.As(runErr, &de) && strings.TrimSpace(de.Code) != "" {
		code = de.Code
	}
	var finalized app.RuntimeWorkerResult
	if err := client.Dispatch(ctx, "runtime.worker.finalize", app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: state, ErrorCode: code, SafeSummary: map[string]any{"worker_error": code}}, &finalized); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func runtimeWorkerPayload(fixture bool, resultFile string) (json.RawMessage, error) {
	if strings.TrimSpace(resultFile) != "" {
		return os.ReadFile(resultFile)
	}
	if fixture {
		return json.RawMessage(`{"output_refs":[],"safe_summary":{"fixture":true},"used_cost_minor":0}`), nil
	}
	return nil, domain.Invalid("RUNTIME_WORKER_RESULT_REQUIRED", "Runtime worker 缺少 Harness 结果或结构化结果文件")
}

func runtimeWorkerFinalizeInput(handle contentruntime.DispatchHandle, harnessKind string, payload json.RawMessage) (app.RuntimeWorkerFinalizeInput, error) {
	var object map[string]json.RawMessage
	if len(payload) == 0 || json.Unmarshal(payload, &object) != nil {
		return app.RuntimeWorkerFinalizeInput{}, domain.Invalid("HARNESS_RESULT_INVALID", "Runtime Harness 返回的结构化结果无效")
	}
	input := app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, SafeSummary: map[string]any{"worker": harnessKind}}
	if object["output_refs"] == nil && object["output_digest"] == nil && object["used_cost_minor"] == nil && object["safe_summary"] == nil {
		input.BusinessPayload = append(json.RawMessage(nil), payload...)
		return input, nil
	}
	var envelope struct {
		OutputRefs    []string       `json:"output_refs"`
		OutputDigest  string         `json:"output_digest"`
		SafeSummary   map[string]any `json:"safe_summary"`
		UsedCostMinor int64          `json:"used_cost_minor"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return app.RuntimeWorkerFinalizeInput{}, domain.Invalid("HARNESS_RESULT_INVALID", "Runtime Harness 结果信封无效")
	}
	input.OutputRefs = envelope.OutputRefs
	input.OutputDigest = envelope.OutputDigest
	input.UsedCostMinor = envelope.UsedCostMinor
	if envelope.SafeSummary != nil {
		input.SafeSummary = envelope.SafeSummary
		input.SafeSummary["worker"] = harnessKind
	}
	return input, nil
}

func runtimeWorkerHeartbeatDelay(handle contentruntime.DispatchHandle) time.Duration {
	if handle.Attempt.LeaseExpiresAt == nil {
		return 20 * time.Second
	}
	remaining := time.Until(*handle.Attempt.LeaseExpiresAt)
	if remaining <= 0 {
		return time.Second
	}
	delay := remaining / 3
	if delay < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return delay
}

func (r *Root) resolveRuntimeWorkerHarness(ctx context.Context, requested string, fixture bool) (string, agentadapter.AgentHarnessAdapter, agentadapter.HarnessCapabilities, error) {
	if r.runtimeHarnesses == nil {
		r.runtimeHarnesses = agentadapter.NewDefaultHarnessRegistry()
	}
	requested = strings.ToLower(strings.TrimSpace(requested))
	if fixture {
		requested = "fake"
	}
	if requested != "" && requested != "auto" {
		harness, capabilities, err := r.runtimeHarnesses.Resolve(ctx, requested)
		return capabilities.Kind, harness, capabilities, err
	}
	for _, kind := range []string{"codex", "claude"} {
		harness, capabilities, err := r.runtimeHarnesses.Resolve(ctx, kind)
		if err == nil {
			return capabilities.Kind, harness, capabilities, nil
		}
	}
	return "", nil, agentadapter.HarnessCapabilities{}, domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "未检测到支持 Runtime 协议的 Codex 或 Claude Code", "安装并登录支持结构化事件的本机智能体客户端")
}
