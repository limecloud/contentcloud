package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

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
	bindings := cfg.RuntimeBindings()
	if len(bindings) == 0 {
		return domain.Conflict("DEVICE_BINDING_MISSING", "启动 Runtime worker 前必须先完成设备注册")
	}
	if !fixture {
		adapter, selectErr := agentadapter.Select(harnessKind)
		if selectErr != nil {
			return selectErr
		}
		if detectErr := adapter.Detect(); detectErr != nil {
			return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "指定的本地智能体不可用", "检查安装与登录状态")
		}
	}
	options := runtimeWorkerRunOptions{Fixture: fixture, HarnessKind: harnessKind, Role: "worker", Profile: "runtime-worker-v1"}
	runBinding := func(ctx context.Context, binding localconfig.DaemonBinding) (map[string]any, error) {
		token, tokenErr := localconfig.DeviceToken(binding.DeviceID)
		if tokenErr != nil {
			return nil, &domain.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: tokenErr.Error(), ExitCode: 3}
		}
		workspace := ""
		for _, candidate := range binding.Workspaces {
			if strings.TrimSpace(candidate.Root) != "" {
				workspace = strings.TrimSpace(candidate.Root)
				break
			}
		}
		options.Workspace = workspace
		client := apiclient.New(r.resolveServer(localconfig.Config{ServerURL: binding.ServerURL}), token)
		return r.runRuntimeWorker(ctx, client, options, true)
	}
	if once {
		for _, binding := range bindings {
			result, runErr := runBinding(cmd.Context(), binding)
			if runErr != nil {
				return runErr
			}
			if leased, _ := result["leased"].(bool); leased {
				return r.writeOK("daemon.run", result)
			}
		}
		return r.writeOK("daemon.run", map[string]any{"leased": false})
	}
	for {
		leased := false
		for _, binding := range bindings {
			result, runErr := runBinding(cmd.Context(), binding)
			if runErr != nil {
				fmt.Fprintln(r.stderr, runErr)
				continue
			}
			if value, _ := result["leased"].(bool); value {
				leased = true
			}
		}
		if !leased {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
		select {
		case <-cmd.Context().Done():
			return nil
		default:
		}
	}
}
