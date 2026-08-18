package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/spf13/cobra"

	agentadapter "github.com/limecloud/contentcloud/internal/integration/agent"
	localconfig "github.com/limecloud/contentcloud/internal/local/config"
	"github.com/limecloud/contentcloud/internal/local/desktopapi"
	localsync "github.com/limecloud/contentcloud/internal/local/sync"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	apiclient "github.com/limecloud/contentcloud/internal/transport/client"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

const (
	daemonIdleBackoffBase          = 500 * time.Millisecond
	daemonIdleBackoffMax           = 30 * time.Second
	daemonHarnessProbeInterval     = 5 * time.Minute
	daemonWorkspaceObserveInterval = 30 * time.Second
)

type daemonBindingRuntime struct {
	binding        localconfig.DaemonBinding
	client         *apiclient.Client
	options        runtimeWorkerRunOptions
	wake           chan struct{}
	observeControl func(runtimeWakeObservation)
	controlState   *runtimeWakeClientState
}

// runtimeDaemonRun is the daemon adapter for the current Runtime worker
// protocol. It keeps launchd as a process supervisor while Runtime owns all
// scheduling, fencing, retries, and business-result handoff.
func (r *Root) runtimeDaemonRun(cmd *cobra.Command, once, fixture bool, harnessKind, logFile string) error {
	if strings.TrimSpace(logFile) != "" {
		managedLog, err := newRotatingLogWriter(logFile)
		if err != nil {
			return err
		}
		defer managedLog.Close()
		r.stdout, r.stderr = managedLog, managedLog
	}
	cfg, err := localconfig.Load()
	if err != nil {
		return err
	}
	bindings := cfg.Bindings()
	if len(bindings) == 0 {
		return fault.Conflict("DEVICE_BINDING_MISSING", "启动 Runtime worker 前必须先完成设备注册")
	}
	resolvedHarnessKind, _, harnessCapabilities, err := r.resolveRuntimeWorkerHarness(cmd.Context(), harnessKind, fixture)
	if err != nil {
		return fault.Policy("AGENT_ADAPTER_UNAVAILABLE", "指定的本地智能体不可用", "检查安装与登录状态")
	}
	baseOptions := runtimeWorkerRunOptions{Fixture: fixture, HarnessKind: resolvedHarnessKind}
	daemonCapabilities := r.runtimeDaemonCapabilities(cmd.Context(), harnessCapabilities, fixture, false)
	unavailable := map[string]string{}
	prepareBinding := func(binding localconfig.DaemonBinding) (daemonBindingRuntime, error) {
		token, tokenErr := localconfig.DeviceToken(binding.DeviceID)
		if tokenErr != nil {
			return daemonBindingRuntime{}, &fault.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: tokenErr.Error(), ExitCode: 3}
		}
		workspaces := map[string]string{}
		for _, candidate := range binding.Workspaces {
			if projectID, root := strings.TrimSpace(candidate.ProjectID), strings.TrimSpace(candidate.Root); projectID != "" && root != "" {
				workspaces[projectID] = root
			}
		}
		options := baseOptions
		daemonInstance := newRuntimeWakeClientState(Version, daemonCapabilities)
		options.DaemonInstanceID = daemonInstance.instanceID
		options.Workspaces = workspaces
		observations := observeDaemonWorkspaces(binding.Workspaces)
		daemonInstance.setWorkspaceObservations(observations)
		daemonInstance.setCapabilities(withWorkspaceEnvironmentStatus(daemonCapabilities, observations))
		return daemonBindingRuntime{
			binding:      binding,
			client:       apiclient.New(r.resolveServer(localconfig.Config{ServerURL: binding.ServerURL}), token),
			options:      options,
			wake:         make(chan struct{}, 1),
			controlState: daemonInstance,
		}, nil
	}
	runtimes := make([]daemonBindingRuntime, 0, len(bindings))
	for _, binding := range bindings {
		runtime, prepareErr := prepareBinding(binding)
		if prepareErr != nil {
			if once {
				return prepareErr
			}
			fmt.Fprintln(r.stderr, prepareErr)
			unavailable[binding.DeviceID] = runtimeObservationError(prepareErr)
			continue
		}
		runtimes = append(runtimes, runtime)
	}
	if len(runtimes) == 0 {
		return fault.Conflict("DAEMON_BINDINGS_UNAVAILABLE", "没有可用的 Daemon 设备绑定")
	}
	if once {
		for _, runtime := range runtimes {
			controlCtx, cancelControl := context.WithCancel(cmd.Context())
			controlReady := make(chan struct{}, 1)
			controlDone := make(chan struct{})
			go func(runtime daemonBindingRuntime) {
				defer close(controlDone)
				runRuntimeWakeClientWithState(controlCtx, runtime.client.BaseURL, runtime.client.Token, runtime.wake, r.stderr, runtime.observeControl, runtime.controlState, controlReady)
			}(runtime)
			select {
			case <-cmd.Context().Done():
				cancelControl()
				<-controlDone
				return cmd.Context().Err()
			case <-controlDone:
				cancelControl()
				return fault.Policy("RUNTIME_CONTROL_UNAVAILABLE", "Runtime 控制通道未能完成 DaemonInstance 同步", "检查设备凭据和服务端连接")
			case <-controlReady:
			}
			result, runErr := r.runRuntimeWorker(cmd.Context(), runtime.client, runtime.options, true)
			cancelControl()
			<-controlDone
			if runErr != nil {
				return runErr
			}
			if leased, _ := result["leased"].(bool); leased {
				return r.writeOK("daemon.run", result)
			}
		}
		return r.writeOK("daemon.run", map[string]any{"leased": false})
	}
	statePath, err := desktopapi.DefaultStatePath()
	if err != nil {
		return err
	}
	syncStore, err := localsync.Open(statePath)
	if err != nil {
		return fmt.Errorf("打开 Desktop 本地同步状态失败：%w", err)
	}
	defer syncStore.Close()
	reviewClients := make(map[string]*apiclient.Client)
	for _, runtime := range runtimes {
		for _, workspace := range runtime.binding.Workspaces {
			if projectID := strings.TrimSpace(workspace.ProjectID); projectID != "" {
				reviewClients[projectID] = runtime.client
			}
		}
	}
	reviewDispatcher := func(ctx context.Context, projectID, command string, params any) (json.RawMessage, error) {
		client := reviewClients[strings.TrimSpace(projectID)]
		if client == nil {
			return nil, fault.NotFound("Desktop 项目设备绑定")
		}
		var raw json.RawMessage
		if err := client.Dispatch(ctx, command, params, &raw); err != nil {
			return nil, err
		}
		return raw, nil
	}
	desktopServer, err := desktopapi.Start(desktopapi.Options{Bindings: bindings, Version: Version, SyncStore: syncStore, ReviewDispatcher: reviewDispatcher})
	if err != nil {
		return fmt.Errorf("启动 Desktop 本地 API 失败：%w", err)
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = desktopServer.Close(shutdownContext)
	}()
	statusPath, err := daemonRuntimeStatusPath(logFile)
	if err != nil {
		return err
	}
	recorder := newDaemonRuntimeStatusRecorder(statusPath, bindings, os.Getpid())
	for deviceID, errorCode := range unavailable {
		recorder.observeControl(deviceID, runtimeWakeObservation{State: "stopped", ErrorCode: errorCode})
		recorder.observeWorker(deviceID, runtimeWorkerObservation{State: "failed", ErrorCode: errorCode})
	}
	daemonCtx, cancelDaemon := context.WithCancel(cmd.Context())
	defer cancelDaemon()
	recorderDone := make(chan struct{})
	go func() {
		recorder.run(daemonCtx)
		close(recorderDone)
	}()
	syncDone := make(chan struct{})
	go func() {
		defer close(syncDone)
		runDesktopSyncLoop(daemonCtx, syncStore, runtimes, func(format string, args ...any) { fmt.Fprintf(r.stderr, format+"\n", args...) })
	}()
	var workers sync.WaitGroup
	for _, runtime := range runtimes {
		runtime := runtime
		deviceID := runtime.binding.DeviceID
		runtime.options.Observe = func(observation runtimeWorkerObservation) {
			active := observation.State == "prepared" || observation.State == "running" || observation.State == "event" || observation.State == "heartbeat" || observation.State == "finalizing"
			runtime.controlState.setAttempt(observation.AttemptID, active)
			recorder.observeWorker(deviceID, observation)
		}
		runtime.observeControl = func(observation runtimeWakeObservation) {
			recorder.observeControl(deviceID, observation)
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			r.runDaemonBindingLoop(daemonCtx, runtime)
		}()
	}
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		ticker := time.NewTicker(daemonHarnessProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-daemonCtx.Done():
				return
			case <-ticker.C:
				capabilities := r.runtimeDaemonCapabilities(daemonCtx, harnessCapabilities, fixture, true)
				for _, runtime := range runtimes {
					observations := observeDaemonWorkspaces(runtime.binding.Workspaces)
					runtime.controlState.setWorkspaceObservations(observations)
					runtime.controlState.setCapabilities(withWorkspaceEnvironmentStatus(capabilities, observations))
				}
			}
		}
	}()
	workspaceObserveDone := make(chan struct{})
	go func() {
		defer close(workspaceObserveDone)
		ticker := time.NewTicker(daemonWorkspaceObserveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-daemonCtx.Done():
				return
			case <-ticker.C:
				for _, runtime := range runtimes {
					observations := observeDaemonWorkspaces(runtime.binding.Workspaces)
					runtime.controlState.setWorkspaceObservations(observations)
					runtime.controlState.setCapabilities(withWorkspaceEnvironmentStatus(runtime.controlState.capabilitiesSnapshot(), observations))
				}
			}
		}
	}()
	workers.Wait()
	cancelDaemon()
	<-probeDone
	<-workspaceObserveDone
	<-syncDone
	<-recorderDone
	return nil
}

func observeDaemonWorkspaces(workspaces []localconfig.DaemonWorkspace) []workspacedomain.DaemonWorkspaceObservation {
	observations := make([]workspacedomain.DaemonWorkspaceObservation, 0, len(workspaces))
	for _, workspace := range workspaces {
		projectID, workspaceID, root := strings.TrimSpace(workspace.ProjectID), strings.TrimSpace(workspace.WorkspaceID), strings.TrimSpace(workspace.Root)
		if projectID == "" || workspaceID == "" {
			continue
		}
		if root == "" {
			observations = append(observations, workspacedomain.DaemonWorkspaceObservation{ProjectID: projectID, WorkspaceID: workspaceID, Status: "unknown", Reason: "workspace_root_unavailable", ObservedAt: time.Now().UTC()})
			continue
		}
		observation, err := localworkspace.ObserveWorkspace(root, time.Now().UTC())
		if err != nil {
			observation = workspacedomain.DaemonWorkspaceObservation{ProjectID: projectID, WorkspaceID: workspaceID, Status: "unknown", Reason: "workspace_observation_failed", ErrorCode: runtimeObservationError(err), ObservedAt: time.Now().UTC()}
		} else if observation.ProjectID != projectID || observation.WorkspaceID != workspaceID {
			observation = workspacedomain.DaemonWorkspaceObservation{ProjectID: projectID, WorkspaceID: workspaceID, Status: "blocked", Reason: "workspace_binding_mismatch", ErrorCode: "WORKSPACE_BINDING_MISMATCH", ObservedAt: time.Now().UTC()}
		}
		observations = append(observations, observation)
	}
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].ProjectID != observations[j].ProjectID {
			return observations[i].ProjectID < observations[j].ProjectID
		}
		return observations[i].WorkspaceID < observations[j].WorkspaceID
	})
	return observations
}

func withWorkspaceEnvironmentStatus(capabilities map[string]any, observations []workspacedomain.DaemonWorkspaceObservation) map[string]any {
	result := cloneRuntimeCapabilities(capabilities)
	if len(observations) == 0 {
		result["environment_status"], result["environment_reason"] = "unknown", "workspace_not_observed"
		return result
	}
	status, reason := "ready", "all_workspaces_ready"
	for _, observation := range observations {
		switch observation.Status {
		case "blocked":
			status, reason = "blocked", observation.Reason
		case "repair_required":
			if status != "blocked" {
				status, reason = "repair_required", observation.Reason
			}
		case "unknown":
			if status == "ready" {
				status, reason = "unknown", observation.Reason
			}
		}
	}
	result["environment_status"], result["environment_reason"] = status, reason
	return result
}

func runtimeDaemonCapabilities(capabilities agentadapter.HarnessCapabilities) map[string]any {
	return map[string]any{
		"harness_kind":          capabilities.Kind,
		"harness_version":       capabilities.Version,
		"events":                capabilities.Events,
		"resume":                capabilities.Resume,
		"fork":                  capabilities.Fork,
		"mcp_stdio":             capabilities.MCPStdio,
		"mcp_http":              capabilities.MCPHTTP,
		"structured_output":     capabilities.StructuredOutput,
		"sandbox_profile":       capabilities.SandboxProfile,
		"max_parallel_sessions": capabilities.MaxParallelSessions,
		"transcript_export":     capabilities.TranscriptExport,
	}
}

func (r *Root) runtimeDaemonCapabilities(ctx context.Context, selected agentadapter.HarnessCapabilities, fixture, refresh bool) map[string]any {
	capabilities := runtimeDaemonCapabilities(selected)
	ids := r.runtimeHarnesses.IDs()
	if fixture {
		ids = []string{"fake"}
	}
	seen := map[string]bool{}
	probes := make([]agentadapter.HarnessProbe, 0, len(ids))
	selectedHealthy := false
	for _, kind := range ids {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" || (!fixture && kind == "fake") || seen[kind] {
			continue
		}
		seen[kind] = true
		probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		probe := r.runtimeHarnesses.Probe(probeCtx, kind, refresh)
		cancel()
		probes = append(probes, probe)
		if probe.Kind == selected.Kind && probe.Status == "healthy" {
			selectedHealthy = true
			capabilities = runtimeDaemonCapabilities(probe.Capabilities)
		}
	}
	if selectedHealthy {
		capabilities["runtime_status"] = "healthy"
		capabilities["runtime_reason"] = "selected_harness_ready"
	} else {
		capabilities = runtimeDaemonCapabilities(agentadapter.HarnessCapabilities{Kind: selected.Kind})
		capabilities["runtime_status"] = "unavailable"
		capabilities["runtime_reason"] = "selected_harness_unavailable"
	}
	capabilities["runtimes"] = probes
	return capabilities
}

func (r *Root) runDaemonBindingLoop(ctx context.Context, runtime daemonBindingRuntime) {
	bindingCtx, cancel := context.WithCancel(ctx)
	observeControl := func(observation runtimeWakeObservation) {
		if runtime.observeControl != nil {
			runtime.observeControl(observation)
		}
		if observation.State == "auth_rejected" {
			cancel()
		}
	}
	wakeDone := make(chan struct{})
	controlReady := make(chan struct{}, 1)
	go func() {
		defer close(wakeDone)
		runRuntimeWakeClientWithState(bindingCtx, runtime.client.BaseURL, runtime.client.Token, runtime.wake, r.stderr, observeControl, runtime.controlState, controlReady)
	}()
	defer func() {
		cancel()
		<-wakeDone
	}()
	backoffAttempt := 0
	select {
	case <-bindingCtx.Done():
		return
	case <-controlReady:
	}
	for bindingCtx.Err() == nil {
		result := map[string]any{"leased": false}
		var err error
		if runtime.controlState.runtimeAvailable() {
			result, err = r.runRuntimeWorker(bindingCtx, runtime.client, runtime.options, true)
		}
		leased, _ := result["leased"].(bool)
		attemptID, _ := result["attempt_id"].(string)
		if attemptID != "" {
			runtime.controlState.setAttempt(attemptID, false)
		}
		if err != nil {
			fmt.Fprintf(r.stderr, "daemon binding %s: %v\n", runtime.binding.DeviceID, err)
			if isTerminalDeviceCredentialError(err) {
				return
			}
		} else if leased {
			backoffAttempt = 0
			continue
		}
		delay := daemonBackoffDelay(backoffAttempt, rand.Float64())
		if backoffAttempt < 30 {
			backoffAttempt++
		}
		timer := time.NewTimer(delay)
		select {
		case <-bindingCtx.Done():
			timer.Stop()
			return
		case <-runtime.wake:
			if !timer.Stop() {
				<-timer.C
			}
			backoffAttempt = 0
		case <-timer.C:
		}
	}
}

func isTerminalDeviceCredentialError(err error) bool {
	var domainError *fault.Error
	return errors.As(err, &domainError) && domainError.Code == "DEVICE_TOKEN_INVALID"
}

func daemonBackoffDelay(attempt int, jitter float64) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if jitter < 0 {
		jitter = 0
	}
	if jitter > 1 {
		jitter = 1
	}
	delay := daemonIdleBackoffBase
	for i := 0; i < attempt && delay < daemonIdleBackoffMax; i++ {
		delay *= 2
		if delay > daemonIdleBackoffMax {
			delay = daemonIdleBackoffMax
		}
	}
	return delay + time.Duration(float64(delay/4)*jitter)
}
