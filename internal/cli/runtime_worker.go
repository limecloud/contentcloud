package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/automationworkspace"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

const (
	runtimeDispatchRetryBase = 200 * time.Millisecond
	runtimeDispatchRetryMax  = 5 * time.Second
	runtimeEventBufferMax    = 256
	runtimeHarnessIdleMax    = 2 * time.Minute
	runtimeInterruptTimeout  = 5 * time.Second
)

// runtimeWorkerCommand is the current worker surface. Runtime owns leasing,
// fencing, heartbeats, and result handoff for every invocation.
type runtimeWorkerRunOptions struct {
	Fixture          bool
	HarnessKind      string
	DaemonInstanceID string
	Workspace        string
	Workspaces       map[string]string
	ResultFile       string
	IdleTimeout      time.Duration
	Observe          func(runtimeWorkerObservation)
}

func (r *Root) runtimeWorkerCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "runtime-worker", Short: "通过 Runtime Attempt/fence 协议执行一个远程 Runtime 节点"}
	var once, fixture bool
	var harnessKind, workspace, resultFile string
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
		resolvedHarnessKind, _, capabilities, err := r.resolveRuntimeWorkerHarness(cmd.Context(), harnessKind, fixture)
		if err != nil {
			return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "指定的本地智能体不可用", "检查安装与登录状态")
		}
		harnessKind = resolvedHarnessKind
		controlState := newRuntimeWakeClientState(Version, runtimeDaemonCapabilities(capabilities))
		workspaces := map[string]string{}
		for _, candidate := range binding.Workspaces {
			if projectID, root := strings.TrimSpace(candidate.ProjectID), strings.TrimSpace(candidate.Root); projectID != "" && root != "" {
				workspaces[projectID] = root
			}
		}
		observations := observeDaemonWorkspaces(binding.Workspaces)
		controlState.setWorkspaceObservations(observations)
		controlState.setCapabilities(withWorkspaceEnvironmentStatus(runtimeDaemonCapabilities(capabilities), observations))
		controlCtx, cancelControl := context.WithCancel(cmd.Context())
		controlReady := make(chan struct{}, 1)
		controlDone := make(chan struct{})
		wake := make(chan struct{}, 1)
		go func() {
			defer close(controlDone)
			runRuntimeWakeClientWithState(controlCtx, client.BaseURL, client.Token, wake, r.stderr, nil, controlState, controlReady)
		}()
		select {
		case <-cmd.Context().Done():
			cancelControl()
			<-controlDone
			return cmd.Context().Err()
		case <-controlDone:
			cancelControl()
			return domain.Policy("RUNTIME_CONTROL_UNAVAILABLE", "Runtime 控制通道未能完成 DaemonInstance 同步", "检查设备凭据和服务端连接")
		case <-controlReady:
		}
		defer func() {
			cancelControl()
			<-controlDone
		}()
		options := runtimeWorkerRunOptions{Fixture: fixture, HarnessKind: harnessKind, DaemonInstanceID: controlState.instanceID, Workspace: workspace, Workspaces: workspaces, ResultFile: resultFile}
		options.Observe = func(observation runtimeWorkerObservation) {
			active := observation.State == "prepared" || observation.State == "running" || observation.State == "event" || observation.State == "heartbeat" || observation.State == "finalizing"
			controlState.setAttempt(observation.AttemptID, active)
		}
		result, err := r.runRuntimeWorker(cmd.Context(), client, options, once)
		if err != nil {
			return err
		}
		return r.writeOK("runtime-worker.run", result)
	}}
	run.Flags().BoolVar(&once, "once", false, "没有可调度节点时返回成功")
	run.Flags().BoolVar(&fixture, "fixture", false, "使用确定性 JSON 结果，不启动本地 Agent")
	run.Flags().StringVar(&harnessKind, "harness", "", "Runtime Harness 类型")
	run.Flags().StringVar(&workspace, "workspace", "", "本地执行工作区")
	run.Flags().StringVar(&resultFile, "result-file", "", "读取结构化业务结果 JSON 的文件")
	cmd.AddCommand(run)
	return cmd
}

func (r *Root) runRuntimeWorker(ctx context.Context, client *apiclient.Client, options runtimeWorkerRunOptions, once bool) (result map[string]any, runErr error) {
	currentAttemptID := ""
	defer func() {
		observation := runtimeWorkerObservation{State: "idle", AttemptID: currentAttemptID, At: time.Now().UTC()}
		if runErr != nil {
			observation.State = "failed"
			observation.ErrorCode = runtimeObservationError(runErr)
		} else if leased, _ := result["leased"].(bool); leased {
			observation.State = "succeeded"
		}
		notifyRuntimeWorker(options.Observe, observation)
	}()
	fixture := options.Fixture || strings.TrimSpace(options.ResultFile) != ""
	harnessKind, harness, capabilities, err := r.resolveRuntimeWorkerHarness(ctx, options.HarnessKind, fixture)
	if err != nil {
		return nil, err
	}
	var handle contentruntime.DispatchHandle
	prepare := app.RuntimeWorkerPrepareNextInput{RuntimeWorkerPrepareInput: app.RuntimeWorkerPrepareInput{DaemonInstanceID: options.DaemonInstanceID, HarnessKind: harnessKind, Capabilities: capabilities}}
	if err := client.Dispatch(ctx, "runtime.worker.prepare_next", prepare, &handle); err != nil {
		if once && domain.IsNotFound(err) {
			return map[string]any{"leased": false}, nil
		}
		return nil, err
	}
	currentAttemptID = handle.Attempt.ID
	notifyRuntimeWorker(options.Observe, runtimeWorkerObservation{State: "prepared", AttemptID: currentAttemptID, At: time.Now().UTC()})
	interactiveWorkspace := runtimeWorkerWorkspace(options, handle.ExecutionSpec.ProjectID)
	prompt := strings.TrimSpace(handle.ExecutionSpec.Prompt)
	gatewayURL, err := resolveRuntimeGatewayURL(client.BaseURL, handle.GatewayURL)
	if err != nil {
		return nil, err
	}
	runtimeGateway := agentadapter.RuntimeGatewayConfig{URL: gatewayURL, Token: handle.GatewayToken, AllowedTools: append([]string(nil), handle.ContextView.AllowedTools...)}
	if fixture {
		return r.runFixtureRuntimeWorker(ctx, client, handle, harnessKind, options)
	}
	automation, err := beginRuntimeAttemptWorkspace(handle, interactiveWorkspace)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, options.DaemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
	}
	defer func() {
		if cleanupErr := automation.Cleanup(); cleanupErr != nil {
			fmt.Fprintf(r.stderr, "automation workspace cleanup failed: %v\n", cleanupErr)
		}
	}()
	workspace := automation.Root
	var session agentadapter.AgentSessionRef
	var stream agentadapter.EventStream
	if handle.ResumeSession != nil {
		session = *handle.ResumeSession
		stream, err = harness.Resume(ctx, agentadapter.ResumeAgentRequest{TenantID: handle.Attempt.TenantID, Session: session, Workspace: workspace, Prompt: prompt, OutputSchema: handle.ExecutionSpec.OutputSchema, ContextDigest: handle.ContextView.Digest, RuntimeGateway: runtimeGateway})
	} else {
		session, stream, err = harness.Start(ctx, agentadapter.StartAgentRequest{TenantID: handle.Attempt.TenantID, JobRunID: handle.Attempt.JobRunID, NodeRunID: handle.Node.ID, AttemptID: handle.Attempt.ID, Workspace: workspace, Prompt: prompt, OutputSchema: handle.ExecutionSpec.OutputSchema, ContextDigest: handle.ContextView.Digest, RuntimeGateway: runtimeGateway})
	}
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, options.DaemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
	}
	if stream == nil {
		err = domain.Conflict("HARNESS_STREAM_MISSING", "Runtime Harness 未返回结构化事件流")
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, options.DaemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
	}
	defer stream.Close()
	if err := r.refreshRuntimeHandle(ctx, client, "runtime.worker.activate", app.RuntimeWorkerActivateInput{DaemonInstanceID: options.DaemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: session}, &handle); err != nil {
		r.interruptRuntimeHarness(harness, session)
		return nil, err
	}
	notifyRuntimeWorker(options.Observe, runtimeWorkerObservation{State: "running", AttemptID: currentAttemptID, At: time.Now().UTC()})
	return r.driveRuntimeWorker(ctx, client, harness, session, stream, handle, automation, harnessKind, options.DaemonInstanceID, options.IdleTimeout, options.Observe)
}

func runtimeWorkerWorkspace(options runtimeWorkerRunOptions, projectID string) string {
	if root := strings.TrimSpace(options.Workspaces[strings.TrimSpace(projectID)]); root != "" {
		return root
	}
	return strings.TrimSpace(options.Workspace)
}

func (r *Root) runFixtureRuntimeWorker(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, harnessKind string, options runtimeWorkerRunOptions) (map[string]any, error) {
	session := agentadapter.AgentSessionRef{TenantID: handle.Attempt.TenantID, HarnessKind: harnessKind, SessionID: "fixture:" + handle.Attempt.ID}
	if err := r.refreshRuntimeHandle(ctx, client, "runtime.worker.activate", app.RuntimeWorkerActivateInput{DaemonInstanceID: options.DaemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Session: session}, &handle); err != nil {
		return nil, err
	}
	payload, err := runtimeWorkerPayload(options.Fixture, options.ResultFile)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, options.DaemonInstanceID, domain.RuntimeAttemptFailed, err)
	}
	return r.finalizeRuntimeWorkerSuccess(ctx, client, handle, harnessKind, options.DaemonInstanceID, payload)
}

func (r *Root) driveRuntimeWorker(ctx context.Context, client *apiclient.Client, harness agentadapter.AgentHarnessAdapter, session agentadapter.AgentSessionRef, stream agentadapter.EventStream, handle contentruntime.DispatchHandle, workspace *automationworkspace.Workspace, harnessKind, daemonInstanceID string, idleTimeout time.Duration, observe func(runtimeWorkerObservation)) (map[string]any, error) {
	heartbeat := time.NewTimer(runtimeWorkerHeartbeatDelay(handle))
	defer heartbeat.Stop()
	if idleTimeout <= 0 {
		idleTimeout = runtimeHarnessIdleMax
	}
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()
	pendingEvents := make([]agentadapter.AgentEvent, 0)
	for {
		select {
		case <-ctx.Done():
			r.interruptRuntimeHarness(harness, session)
			return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, ctx.Err())
		case <-idle.C:
			err := &domain.Error{Type: "runtime", Subtype: harnessKind, Code: "HARNESS_PROGRESS_TIMEOUT", Message: "Runtime Harness 长时间没有结构化进展", Retryable: true, ExitCode: 5}
			r.interruptRuntimeHarness(harness, session)
			return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
		case <-heartbeat.C:
			var flushErr error
			pendingEvents, flushErr = r.flushRuntimeEvents(ctx, client, handle, pendingEvents, daemonInstanceID)
			if flushErr != nil && !isRetryableDispatchError(flushErr) {
				r.interruptRuntimeHarness(harness, session)
				return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, flushErr)
			}
			if err := r.refreshRuntimeHandleWithRetry(ctx, client, "runtime.worker.heartbeat", app.RuntimeWorkerHeartbeatInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken}, &handle, handle.Attempt.LeaseExpiresAt); err != nil {
				r.interruptRuntimeHarness(harness, session)
				return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
			}
			if err := workspace.Renew(runtimeAttemptExpiry(handle)); err != nil {
				r.interruptRuntimeHarness(harness, session)
				return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
			}
			notifyRuntimeWorker(observe, runtimeWorkerObservation{State: "heartbeat", AttemptID: handle.Attempt.ID, At: time.Now().UTC()})
			heartbeat.Reset(runtimeWorkerHeartbeatDelay(handle))
		case event, ok := <-stream.Events():
			if !ok {
				err := domain.Conflict("HARNESS_STREAM_CLOSED", "Runtime Harness 在提交结构化结果前关闭了事件流")
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
			}
			if event.Session != session {
				err := domain.Conflict("HARNESS_SESSION_MISMATCH", "Runtime Harness 事件会话与当前 Attempt 不一致")
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, daemonInstanceID, domain.RuntimeAttemptFailed, err)
			}
			if runtimeEventAdvancesProgress(event) {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(idleTimeout)
			}
			notifyRuntimeWorker(observe, runtimeWorkerObservation{State: "event", AttemptID: handle.Attempt.ID, At: time.Now().UTC()})
			switch event.Type {
			case "result.completed":
				if len(pendingEvents) > 0 {
					var flushErr error
					pendingEvents, flushErr = r.retryRuntimeEvents(ctx, client, handle, pendingEvents, daemonInstanceID)
					if flushErr != nil {
						return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, flushErr)
					}
				}
				return r.finalizeRuntimeWorkerSuccess(ctx, client, handle, harnessKind, daemonInstanceID, event.Data)
			case "session.failed", "session.interrupted":
				code := strings.TrimSpace(event.ErrorCode)
				if code == "" {
					code = "HARNESS_SESSION_FAILED"
				}
				err := &domain.Error{Type: "runtime", Subtype: harnessKind, Code: code, Message: "Runtime Harness 会话未完成", Retryable: true, ExitCode: 5}
				return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
			default:
				pendingEvents = append(pendingEvents, event)
				if len(pendingEvents) > runtimeEventBufferMax {
					err := domain.Conflict("RUNTIME_EVENT_BUFFER_FULL", "Runtime 事件缓冲已满，无法在当前网络状态下继续可靠执行")
					r.interruptRuntimeHarness(harness, session)
					return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, err)
				}
				var flushErr error
				pendingEvents, flushErr = r.flushRuntimeEvents(ctx, client, handle, pendingEvents, daemonInstanceID)
				if flushErr != nil && !isRetryableDispatchError(flushErr) {
					r.interruptRuntimeHarness(harness, session)
					return nil, r.finalizeRuntimeWorkerError(context.WithoutCancel(ctx), client, handle, daemonInstanceID, domain.RuntimeAttemptRetryableFailed, flushErr)
				}
			}
		}
	}
}

func (r *Root) refreshRuntimeHandle(ctx context.Context, client *apiclient.Client, command string, params any, handle *contentruntime.DispatchHandle) error {
	gatewayToken, gatewayURL := handle.GatewayToken, handle.GatewayURL
	executionSpec := handle.ExecutionSpec
	var refreshed contentruntime.DispatchHandle
	if err := client.Dispatch(ctx, command, params, &refreshed); err != nil {
		return err
	}
	refreshed.GatewayToken, refreshed.GatewayURL = gatewayToken, gatewayURL
	refreshed.ExecutionSpec = executionSpec
	*handle = refreshed
	return nil
}

func beginRuntimeAttemptWorkspace(handle contentruntime.DispatchHandle, interactiveRoot string) (*automationworkspace.Workspace, error) {
	if handle.Attempt.LeaseExpiresAt == nil {
		return nil, domain.Invalid("RUNTIME_ATTEMPT_LEASE_INVALID", "RuntimeAttempt 缺少自动化工作区租约截止时间")
	}
	forbidden := []string{}
	if strings.TrimSpace(interactiveRoot) != "" {
		forbidden = append(forbidden, interactiveRoot)
	}
	return automationworkspace.Begin(automationworkspace.Options{
		AttemptID: handle.Attempt.ID, RunID: handle.ExecutionSpec.TaskContract.RunID, ProjectID: handle.ExecutionSpec.ProjectID,
		Contract: handle.ExecutionSpec.TaskContract, OutputSchema: handle.ExecutionSpec.OutputSchema, Skill: []byte(handle.ExecutionSpec.Skill),
		ForbiddenRoots: forbidden, Now: time.Now().UTC(), ExpiresAt: runtimeAttemptExpiry(handle),
	})
}

func runtimeAttemptExpiry(handle contentruntime.DispatchHandle) time.Time {
	if handle.Attempt.LeaseExpiresAt != nil {
		return handle.Attempt.LeaseExpiresAt.UTC()
	}
	return time.Now().UTC().Add(time.Minute)
}

func resolveRuntimeGatewayURL(serverBaseURL, gatewayURL string) (string, error) {
	gatewayURL = strings.TrimSpace(gatewayURL)
	if gatewayURL == "" {
		return "", nil
	}
	base, err := url.Parse(strings.TrimSpace(serverBaseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", domain.Invalid("RUNTIME_GATEWAY_BASE_URL_INVALID", "Runtime Gateway 缺少有效的服务端来源")
	}
	target, err := url.Parse(gatewayURL)
	if err != nil {
		return "", domain.Invalid("RUNTIME_GATEWAY_URL_INVALID", "Runtime Gateway 地址无效")
	}
	if target.IsAbs() {
		if !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
			return "", domain.Policy("RUNTIME_GATEWAY_ORIGIN_MISMATCH", "Runtime Gateway 地址不是当前 ContentCloud 服务端来源", "拒绝向跨源地址发送 Attempt 凭据")
		}
		if target.RawQuery != "" || target.Fragment != "" || target.User != nil {
			return "", domain.Invalid("RUNTIME_GATEWAY_URL_INVALID", "Runtime Gateway 地址不能包含查询、片段或用户信息")
		}
		return target.String(), nil
	}
	if !strings.HasPrefix(target.Path, "/") {
		return "", domain.Invalid("RUNTIME_GATEWAY_URL_INVALID", "Runtime Gateway 相对地址必须以 / 开头")
	}
	if target.RawQuery != "" || target.Fragment != "" {
		return "", domain.Invalid("RUNTIME_GATEWAY_URL_INVALID", "Runtime Gateway 地址不能包含查询或片段")
	}
	base.RawQuery = ""
	base.Fragment = ""
	base.User = nil
	return strings.TrimRight(base.String(), "/") + target.Path, nil
}

func (r *Root) refreshRuntimeHandleWithRetry(ctx context.Context, client *apiclient.Client, command string, params any, handle *contentruntime.DispatchHandle, leaseExpiresAt *time.Time) error {
	for {
		if err := r.refreshRuntimeHandle(ctx, client, command, params, handle); err == nil || !isRetryableDispatchError(err) {
			return err
		} else {
			delay := runtimeDispatchRetryDelay(0, rand.Float64())
			if leaseExpiresAt != nil && time.Until(*leaseExpiresAt) <= delay+100*time.Millisecond {
				return err
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func runtimeEventAdvancesProgress(event agentadapter.AgentEvent) bool {
	if event.Type != "session.progress" || len(event.Data) == 0 {
		return true
	}
	var metadata struct {
		Provider  string `json:"provider"`
		EventType string `json:"event_type"`
	}
	if json.Unmarshal(event.Data, &metadata) != nil || metadata.Provider != "claude" {
		return true
	}
	switch metadata.EventType {
	case "system", "unknown":
		return false
	default:
		return true
	}
}

func notifyRuntimeWorker(observe func(runtimeWorkerObservation), observation runtimeWorkerObservation) {
	if observe != nil {
		observe(observation)
	}
}

func firstDaemonInstanceID(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func (r *Root) interruptRuntimeHarness(harness agentadapter.AgentHarnessAdapter, session agentadapter.AgentSessionRef) {
	interruptCtx, cancel := context.WithTimeout(context.Background(), runtimeInterruptTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- harness.Interrupt(interruptCtx, session)
	}()
	select {
	case err := <-done:
		if err != nil {
			fmt.Fprintf(r.stderr, "runtime harness interrupt failed: %v\n", err)
		}
	case <-interruptCtx.Done():
		fmt.Fprintln(r.stderr, "runtime harness interrupt timed out")
	}
}

func (r *Root) flushRuntimeEvents(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, pending []agentadapter.AgentEvent, daemonInstanceIDs ...string) ([]agentadapter.AgentEvent, error) {
	daemonInstanceID := firstDaemonInstanceID(daemonInstanceIDs)
	for len(pending) > 0 {
		err := client.Dispatch(ctx, "runtime.worker.event", app.RuntimeWorkerEventInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Event: pending[0]}, nil)
		if err != nil {
			if isRetryableDispatchError(err) {
				fmt.Fprintf(r.stderr, "runtime event upload deferred after transient failure: %v\n", err)
			}
			return pending, err
		}
		pending = pending[1:]
	}
	return pending, nil
}

func (r *Root) retryRuntimeEvents(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, pending []agentadapter.AgentEvent, daemonInstanceIDs ...string) ([]agentadapter.AgentEvent, error) {
	daemonInstanceID := firstDaemonInstanceID(daemonInstanceIDs)
	for len(pending) > 0 {
		err := r.retryRuntimeDispatch(ctx, client, "runtime.worker.event", app.RuntimeWorkerEventInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, Event: pending[0]}, nil, handle.Attempt.LeaseExpiresAt)
		if err != nil {
			return pending, err
		}
		pending = pending[1:]
	}
	return pending, nil
}

func (r *Root) finalizeRuntimeWorkerSuccess(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, harnessKind, daemonInstanceID string, payload json.RawMessage) (map[string]any, error) {
	input, err := runtimeWorkerFinalizeInput(handle, harnessKind, payload)
	if err != nil {
		return nil, r.finalizeRuntimeWorkerError(ctx, client, handle, daemonInstanceID, domain.RuntimeAttemptFailed, err)
	}
	input.DaemonInstanceID = daemonInstanceID
	var finalized app.RuntimeWorkerResult
	if err := r.retryRuntimeDispatch(ctx, client, "runtime.worker.finalize", input, &finalized, handle.Attempt.LeaseExpiresAt); err != nil {
		return nil, err
	}
	return map[string]any{"leased": true, "attempt_id": handle.Attempt.ID, "job_run_id": handle.Attempt.JobRunID, "business_result_ref": finalized.BusinessResultRef, "state": finalized.Handle.Attempt.State}, nil
}

func (r *Root) finalizeRuntimeWorkerError(ctx context.Context, client *apiclient.Client, handle contentruntime.DispatchHandle, daemonInstanceID, state string, runErr error) error {
	code := "RUNTIME_WORKER_EXECUTION_FAILED"
	var de *domain.Error
	if errors.As(runErr, &de) && strings.TrimSpace(de.Code) != "" {
		code = de.Code
	}
	var finalized app.RuntimeWorkerResult
	if err := r.retryRuntimeDispatch(ctx, client, "runtime.worker.finalize", app.RuntimeWorkerFinalizeInput{DaemonInstanceID: daemonInstanceID, AttemptID: handle.Attempt.ID, FenceToken: handle.Attempt.FenceToken, State: state, ErrorCode: code, SafeSummary: map[string]any{"worker_error": code}}, &finalized, handle.Attempt.LeaseExpiresAt); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func (r *Root) retryRuntimeDispatch(ctx context.Context, client *apiclient.Client, command string, params, out any, leaseExpiresAt *time.Time) error {
	attempt := 0
	for {
		err := client.Dispatch(ctx, command, params, out)
		if err == nil || !isRetryableDispatchError(err) {
			return err
		}
		delay := runtimeDispatchRetryDelay(attempt, rand.Float64())
		if leaseExpiresAt != nil {
			remaining := time.Until(*leaseExpiresAt)
			if remaining <= delay+100*time.Millisecond {
				return err
			}
		}
		fmt.Fprintf(r.stderr, "%s transient failure; retrying in %s: %v\n", command, delay.Round(time.Millisecond), err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		attempt++
	}
}

func isRetryableDispatchError(err error) bool {
	var domainError *domain.Error
	return errors.As(err, &domainError) && (domainError.Retryable || domainError.Type == "network")
}

func runtimeDispatchRetryDelay(attempt int, jitter float64) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	delay := runtimeDispatchRetryBase
	for i := 0; i < attempt && delay < runtimeDispatchRetryMax; i++ {
		delay *= 2
		if delay > runtimeDispatchRetryMax {
			delay = runtimeDispatchRetryMax
		}
	}
	return delay + time.Duration(float64(delay/4)*jitter)
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
