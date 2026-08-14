package cli

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/plugin"
	"github.com/limecloud/contentcloud/internal/integration/pluginbuiltin"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost/claude"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost/codex"
	"github.com/limecloud/contentcloud/internal/integration/pluginidentity"
)

type hostPluginRuntime struct {
	Adapter *pluginhost.Adapter
	Package plugin.Package
	HostID  pluginhost.HostID
}

type hostLaunchResult struct {
	Opened         bool   `json:"opened"`
	Method         string `json:"method,omitempty"`
	WorkspacePath  string `json:"workspace_path"`
	DeepLink       string `json:"deep_link,omitempty"`
	RecoveryPrompt string `json:"recovery_prompt"`
	Error          string `json:"error,omitempty"`
}

func (r *Root) pluginRuntime(hostName string) (*hostPluginRuntime, error) {
	if r.pluginRuntimeHook != nil {
		return r.pluginRuntimeHook(hostName)
	}
	hostID, err := parsePluginHost(hostName)
	if err != nil {
		return nil, err
	}
	storeRoot, err := pluginStoreRoot()
	if err != nil {
		return nil, err
	}
	store, err := pluginhost.NewStore(storeRoot)
	if err != nil {
		return nil, err
	}
	pkg, err := pluginbuiltin.Load(store.Root, pluginidentity.VideoProduction, Version)
	if err != nil {
		return nil, err
	}
	var native pluginhost.NativeHost
	switch hostID {
	case pluginhost.HostCodex:
		native, err = codex.New(codex.Config{ProjectionRoot: store.Root}, r.pluginRunner)
	case pluginhost.HostClaude:
		native, err = claude.New(claude.Config{ProjectionRoot: filepath.Join(store.HostPath(pluginhost.HostClaude), "marketplace")}, r.pluginRunner)
	}
	if err != nil {
		return nil, err
	}
	adapter, err := pluginhost.New(native, store)
	if err != nil {
		return nil, err
	}
	return &hostPluginRuntime{Adapter: adapter, Package: pkg, HostID: hostID}, nil
}

func parsePluginHost(value string) (pluginhost.HostID, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", string(pluginhost.HostCodex):
		return pluginhost.HostCodex, nil
	case string(pluginhost.HostClaude):
		return pluginhost.HostClaude, nil
	default:
		return "", domain.Invalid("PLUGIN_HOST_INVALID", "插件宿主必须是 codex 或 claude")
	}
}

func pluginStoreRoot() (string, error) {
	return pluginhost.DefaultStoreRoot()
}

func recoveryPrompt(pluginID string) string {
	return fmt.Sprintf("[%s] 请先调用工作区上下文工具（workspace_context）。如果 onboarding.state 是 needs_project_brief，只收集并确认项目简报；确认后重新读取上下文，只展示 onboarding.next_step。不要罗列内部工具，也不要执行未经用户确认的写入、领取任务或云端同步。", pluginID)
}

func launchHost(ctx context.Context, runner pluginhost.CommandRunner, host pluginhost.HostID, workspace, pluginID string) hostLaunchResult {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return hostLaunchResult{WorkspacePath: workspace, Error: err.Error()}
	}
	prompt := recoveryPrompt(pluginID)
	result := hostLaunchResult{WorkspacePath: absolute, RecoveryPrompt: prompt}
	if host != pluginhost.HostCodex {
		return result
	}
	deepLink := codexNewChatDeepLink(absolute, prompt)
	result.DeepLink = deepLink
	if runner == nil {
		runner = pluginhost.ExecRunner{}
	}
	launchErrors := []string{}
	if runtime.GOOS == "darwin" {
		commandResult, runErr := runner.Run(ctx, pluginhost.Command{Name: "open", Args: []string{deepLink}})
		if runErr == nil && commandResult.ExitCode == 0 {
			result.Opened = true
			result.Method = "deep_link"
			return result
		}
		launchErrors = append(launchErrors, launchError("open", commandResult, runErr))
	}
	commandResult, runErr := runner.Run(ctx, pluginhost.Command{Name: "codex", Args: []string{"app", absolute}})
	if runErr == nil && commandResult.ExitCode == 0 {
		result.Opened = true
		result.Method = "workspace"
		return result
	}
	launchErrors = append(launchErrors, launchError("codex app", commandResult, runErr))
	result.Error = strings.Join(launchErrors, "; ")
	return result
}

func codexNewChatDeepLink(workspace, prompt string) string {
	query := url.Values{}
	query.Set("path", workspace)
	query.Set("prompt", prompt)
	return (&url.URL{Scheme: "codex", Host: "new", RawQuery: query.Encode()}).String()
}

func launchError(operation string, result pluginhost.CommandResult, err error) string {
	if err != nil {
		return operation + ": " + err.Error()
	}
	return fmt.Sprintf("%s 执行结束，退出码为 %d：%s", operation, result.ExitCode, strings.TrimSpace(string(result.Stderr)))
}
