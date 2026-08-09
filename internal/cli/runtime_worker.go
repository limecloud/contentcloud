package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"

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
		if strings.TrimSpace(cfg.DeviceID) == "" {
			return domain.Conflict("DEVICE_BINDING_MISSING", "Runtime worker 启动前必须先完成设备注册")
		}
		token, err := localconfig.DeviceToken(cfg.DeviceID)
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
	harnessKind := strings.TrimSpace(options.HarnessKind)
	if harnessKind == "" {
		harnessKind = "codex"
		if options.Fixture {
			harnessKind = "fake"
		}
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
	prepare := app.RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: app.RuntimeWorkerPrepareInput{HarnessKind: harnessKind, Role: role, ExecutionProfileID: profile, Workspace: options.Workspace, Prompt: options.Prompt, MaxTokens: 8192}}
	if err := client.Dispatch(ctx, "runtime.worker.prepare_next", prepare, &handle); err != nil {
		var de *domain.Error
		if once && errors.As(err, &de) && de.Code == "NOT_FOUND" {
			return map[string]any{"leased": false}, nil
		}
		return nil, err
	}
	session := agentadapter.AgentSessionRef{HarnessKind: harnessKind, SessionID: "runtime-worker:" + handle.Attempt.ID}
	if err := client.Dispatch(ctx, "runtime.worker.activate", app.RuntimeWorkerActivateInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: session}, &handle); err != nil {
		return nil, err
	}
	if err := client.Dispatch(ctx, "runtime.worker.heartbeat", app.RuntimeWorkerHeartbeatInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken}, &handle); err != nil {
		return nil, err
	}
	payload, err := runtimeWorkerPayload(ctx, options.Fixture, options.ResultFile, options.Workspace, harnessKind)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, err)
	}
	var finalized app.RuntimeWorkerResult
	if err := client.Dispatch(ctx, "runtime.worker.finalize", app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptSucceeded, BusinessPayload: payload, SafeSummary: map[string]any{"worker": harnessKind}}, &finalized); err != nil {
		return nil, err
	}
	return map[string]any{"leased": true, "attempt_id": handle.Attempt.ID, "job_run_id": handle.Attempt.JobRunID, "business_result_ref": finalized.BusinessResultRef, "state": finalized.Handle.Attempt.State}, nil
}

func (r *Root) finalizeRuntimeWorkerError(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, runErr error) error {
	code := "RUNTIME_WORKER_EXECUTION_FAILED"
	var de *domain.Error
	if errors.As(runErr, &de) && strings.TrimSpace(de.Code) != "" {
		code = de.Code
	}
	var finalized app.RuntimeWorkerResult
	if err := client.Dispatch(ctx, "runtime.worker.finalize", app.RuntimeWorkerFinalizeInput{AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: domain.RuntimeAttemptFailed, ErrorCode: code, SafeSummary: map[string]any{"worker_error": code}}, &finalized); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func runtimeWorkerPayload(ctx context.Context, fixture bool, resultFile, workspace, harnessKind string) (json.RawMessage, error) {
	if strings.TrimSpace(resultFile) != "" {
		return os.ReadFile(resultFile)
	}
	if fixture {
		return json.RawMessage(`{"output_refs":[],"safe_summary":{"fixture":true},"used_cost_minor":0}`), nil
	}
	adapter, err := agentadapter.Select(harnessKind)
	if err != nil {
		return nil, err
	}
	if err := adapter.Detect(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, domain.Invalid("RUNTIME_WORKER_WORKSPACE_REQUIRED", "未提供本地 Agent 工作区或结构化结果文件")
	}
	return adapter.Run(ctx, workspace)
}
