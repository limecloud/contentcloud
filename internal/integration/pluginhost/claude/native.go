package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"

	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

const minimumClaudeVersion = "2.1.220"

type Config struct {
	Binary          string
	ConfigDir       string
	MarketplaceName string
	ProjectionRoot  string
}

type Host struct {
	config Config
	runner CommandRunner
}

type nativeChange struct {
	Operation                string                `json:"operation"`
	PluginID                 string                `json:"plugin_id"`
	PreviousProjection       json.RawMessage       `json:"previous_projection"`
	MarketplaceWasConfigured bool                  `json:"marketplace_was_configured"`
	MarketplaceAdded         bool                  `json:"marketplace_added"`
	MarketplaceRemoved       bool                  `json:"marketplace_removed"`
	PluginWasInstalled       bool                  `json:"plugin_was_installed"`
	PreviousPlugin           *pluginListItem       `json:"previous_plugin,omitempty"`
	PluginMutationStarted    bool                  `json:"plugin_mutation_started"`
	Target                   pluginhost.ReleaseRef `json:"target"`
}

type marketplaceListItem struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	Path            string `json:"path"`
	InstallLocation string `json:"installLocation"`
}

type pluginListItem struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Scope       string `json:"scope"`
	Enabled     bool   `json:"enabled"`
	InstallPath string `json:"installPath"`
	InstalledAt string `json:"installedAt"`
	LastUpdated string `json:"lastUpdated"`
}

type inventory struct {
	marketplace *marketplaceListItem
	plugin      *pluginListItem
	generation  string
}

func New(config Config, runner CommandRunner) (*Host, error) {
	if strings.TrimSpace(config.Binary) == "" {
		config.Binary = "claude"
	}
	if strings.TrimSpace(config.MarketplaceName) == "" {
		config.MarketplaceName = "contentcloud"
	}
	if strings.TrimSpace(config.ProjectionRoot) == "" {
		return nil, fault.Invalid("CLAUDE_PLUGIN_HOST_CONFIG_INVALID", "Claude 插件宿主缺少本地 Marketplace 投影目录")
	}
	projectionRoot, err := filepath.Abs(config.ProjectionRoot)
	if err != nil {
		return nil, err
	}
	config.ProjectionRoot = filepath.Clean(projectionRoot)
	if strings.TrimSpace(config.ConfigDir) != "" {
		configDir, err := filepath.Abs(config.ConfigDir)
		if err != nil {
			return nil, err
		}
		config.ConfigDir = filepath.Clean(configDir)
		if err := os.MkdirAll(config.ConfigDir, 0o700); err != nil {
			return nil, fmt.Errorf("create Claude config directory: %w", err)
		}
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Host{config: config, runner: runner}, nil
}

func (h *Host) ID() pluginhost.HostID {
	return pluginhost.HostClaude
}

func (h *Host) Capabilities(ctx context.Context) (pluginhost.Capabilities, error) {
	version, err := h.version(ctx)
	if err != nil {
		return pluginhost.Capabilities{}, err
	}
	supported := pluginhost.CompareSemanticVersion(version, minimumClaudeVersion) >= 0
	return pluginhost.Capabilities{
		PluginDirectoryInstall: false,
		Skills:                 supported,
		MCPStdio:               supported,
		MCPStreamableHTTP:      false,
		NewSessionRequired:     true,
		AtomicInstall:          false,
		Rollback:               true,
		NativeExtensions:       []string{"agent-plugins/1.0.0", "claude-code/local-marketplace"},
	}, nil
}

func (h *Host) Detect(ctx context.Context, target pluginhost.HostTarget) (pluginhost.State, error) {
	capabilities, err := h.Capabilities(ctx)
	if err != nil {
		return pluginhost.State{}, err
	}
	state := pluginhost.State{
		SchemaVersion: pluginhost.SchemaVersion,
		HostID:        h.ID(),
		Status:        pluginhost.StatusAbsent,
		Capabilities:  capabilities,
		Components:    []pluginhost.ComponentState{},
	}
	if !capabilities.Skills {
		state.Status = pluginhost.StatusUnsupportedHost
		state.Reason = "Claude Code 版本低于已验证最低版本 " + minimumClaudeVersion
		return state, nil
	}
	current, err := h.readInventory(ctx, target)
	if err != nil {
		return pluginhost.State{}, err
	}
	state.Generation = current.generation
	projectionRoot, projectionErr := h.projectedPackageRoot(target.Release.PluginID)
	projectionPresent := projectionErr == nil
	if current.marketplace == nil && current.plugin == nil && !projectionPresent {
		return h.populateComponents(state, target, current), nil
	}
	if current.marketplace != nil {
		managedRoot, managedErr := canonicalPath(h.config.ProjectionRoot)
		marketplaceRoot, marketplaceErr := canonicalPath(current.marketplace.Path)
		installRoot, installErr := canonicalPath(current.marketplace.InstallLocation)
		if managedErr != nil || marketplaceErr != nil || installErr != nil || current.marketplace.Source != "directory" || marketplaceRoot != managedRoot || installRoot != managedRoot {
			state.Status = pluginhost.StatusBlocked
			state.Reason = "同名 Claude Marketplace 不属于 ContentCloud 管理的设备投影"
			return h.populateComponents(state, target, current), nil
		}
	}
	if current.plugin == nil && !projectionPresent {
		return h.populateComponents(state, target, current), nil
	}
	if current.marketplace == nil || current.plugin == nil || !projectionPresent {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Claude Marketplace 投影与已安装插件状态不完整"
		return h.populateComponents(state, target, current), nil
	}
	state.Release = &target.Release
	if current.plugin.ID != h.pluginID(target.Release.PluginID) || current.plugin.Scope != "user" || !current.plugin.Enabled {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Claude 插件身份、安装范围或启用状态无效"
		return h.populateComponents(state, target, current), nil
	}
	if current.plugin.Version != target.Release.Version {
		state.Status = pluginhost.StatusInstalled
		state.Reason = "Claude 已安装插件版本与目标 Release 不一致"
		return h.populateComponents(state, target, current), nil
	}
	expectedProjection, err := h.expectedProjectedPackageRoot(target.Release)
	if err != nil || projectionRoot != expectedProjection {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Claude 插件来源与目标 Release 投影不一致"
		return h.populateComponents(state, target, current), nil
	}
	marker, err := readProjectionMarker(current.plugin.InstallPath)
	if err != nil || marker.PluginID != target.Release.PluginID || marker.Version != target.Release.Version || marker.Digest != target.Release.Digest {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Claude 插件缓存不是目标标准包摘要"
		return h.populateComponents(state, target, current), nil
	}
	if err := h.validateProjection(ctx, projectionRoot); err != nil {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Claude 私有投影未通过原生严格校验"
		return h.populateComponents(state, target, current), nil
	}
	state = h.populateComponents(state, target, current)
	state.Status = pluginhost.StatusReady
	for _, item := range state.Components {
		if item.Status != pluginhost.StatusReady {
			state.Status = pluginhost.StatusRepairRequired
			state.Reason = "Claude 未完整激活标准包组件"
			break
		}
	}
	return state, nil
}

func (h *Host) Apply(ctx context.Context, request pluginhost.NativeApply) (pluginhost.NativeChange, []pluginhost.InstalledComponent, error) {
	current, err := h.readInventory(ctx, request.Target)
	if err != nil {
		return pluginhost.NativeChange{}, nil, err
	}
	change := nativeChange{
		Operation:                "install",
		PluginID:                 h.pluginID(request.Target.Release.PluginID),
		MarketplaceWasConfigured: current.marketplace != nil,
		PluginWasInstalled:       current.plugin != nil,
		PreviousPlugin:           current.plugin,
		Target:                   request.Target.Release,
	}
	projectedRoot, err := h.materializePackage(request.Package, request.PackageRoot)
	if err != nil {
		return h.change(change), nil, err
	}
	previous, err := h.upsertMarketplaceProjection(request.Package, projectedRoot)
	if err != nil {
		return h.change(change), nil, err
	}
	change.PreviousProjection = previous
	if err := h.validateProjection(ctx, projectedRoot); err != nil {
		return h.change(change), nil, err
	}
	if current.marketplace == nil {
		if err := h.addMarketplace(ctx); err != nil {
			return h.change(change), nil, err
		}
		change.MarketplaceAdded = true
	} else if err := h.updateMarketplace(ctx); err != nil {
		return h.change(change), nil, err
	}
	change.PluginMutationStarted = true
	if current.plugin == nil {
		err = h.installPlugin(ctx, change.PluginID)
	} else if current.plugin.Version == request.Target.Release.Version {
		if err = h.uninstallPlugin(ctx, change.PluginID); err == nil {
			err = h.installPlugin(ctx, change.PluginID)
		}
	} else {
		err = h.updatePlugin(ctx, change.PluginID)
	}
	if err != nil {
		return h.change(change), nil, err
	}
	if current.plugin != nil && !current.plugin.Enabled {
		if err := h.enablePlugin(ctx, change.PluginID); err != nil {
			return h.change(change), nil, err
		}
	}
	state, err := h.Detect(ctx, request.Target)
	if err != nil {
		return h.change(change), nil, err
	}
	if state.Status != pluginhost.StatusReady {
		return h.change(change), nil, fault.Conflict("CLAUDE_PLUGIN_VERIFY_FAILED", state.Reason)
	}
	installedInventory, err := h.readInventory(ctx, request.Target)
	if err != nil || installedInventory.plugin == nil {
		if err == nil {
			err = fault.Conflict("CLAUDE_PLUGIN_VERIFY_FAILED", "Claude 插件安装后未返回缓存路径")
		}
		return h.change(change), nil, err
	}
	installed := make([]pluginhost.InstalledComponent, 0, len(state.Components))
	for _, item := range state.Components {
		component := pluginhost.InstalledComponent{ComponentRef: item.Component, InstalledPath: installedInventory.plugin.InstallPath}
		if item.Component.Type == "mcp" {
			component.Transport = "stdio"
		}
		installed = append(installed, component)
	}
	return h.change(change), installed, nil
}

func (h *Host) Remove(ctx context.Context, request pluginhost.NativeRemove) (pluginhost.NativeChange, error) {
	current, err := h.readInventory(ctx, request.Target)
	if err != nil {
		return pluginhost.NativeChange{}, err
	}
	change := nativeChange{
		Operation:                "remove",
		PluginID:                 h.pluginID(request.Target.Release.PluginID),
		MarketplaceWasConfigured: current.marketplace != nil,
		PluginWasInstalled:       current.plugin != nil,
		PreviousPlugin:           current.plugin,
		Target:                   request.Target.Release,
	}
	if current.plugin != nil {
		change.PluginMutationStarted = true
		if err := h.uninstallPlugin(ctx, change.PluginID); err != nil {
			return h.change(change), err
		}
	}
	previous, empty, err := h.removeMarketplaceProjection(request.Target.Release.PluginID)
	if err != nil {
		return h.change(change), err
	}
	change.PreviousProjection = previous
	if empty && current.marketplace != nil {
		if err := h.removeMarketplace(ctx); err != nil {
			return h.change(change), err
		}
		change.MarketplaceRemoved = true
	} else if current.marketplace != nil {
		if err := h.updateMarketplace(ctx); err != nil {
			return h.change(change), err
		}
	}
	state, err := h.Detect(ctx, request.Target)
	if err != nil {
		return h.change(change), err
	}
	if state.Status != pluginhost.StatusAbsent {
		return h.change(change), fault.Conflict("CLAUDE_PLUGIN_REMOVE_VERIFY_FAILED", "Claude 插件删除后仍处于可见状态")
	}
	return h.change(change), nil
}

func (h *Host) Rollback(ctx context.Context, native pluginhost.NativeChange) error {
	if len(native.Data) == 0 {
		return nil
	}
	var change nativeChange
	if err := json.Unmarshal(native.Data, &change); err != nil {
		return fault.Invalid("CLAUDE_PLUGIN_ROLLBACK_INVALID", "Claude 插件原生回滚状态无法解析")
	}
	if len(change.PreviousProjection) == 0 {
		return nil
	}
	rollbackErrors := []string{}
	plugins, err := h.plugins(ctx)
	if err != nil {
		rollbackErrors = append(rollbackErrors, err.Error())
	} else if change.PluginMutationStarted || change.Operation == "remove" {
		for _, item := range plugins {
			if item.ID == change.PluginID {
				if err := h.uninstallPlugin(ctx, change.PluginID); err != nil {
					rollbackErrors = append(rollbackErrors, err.Error())
				}
				break
			}
		}
	}
	if err := h.restoreMarketplaceProjection(change.PreviousProjection); err != nil {
		rollbackErrors = append(rollbackErrors, err.Error())
	}
	if change.MarketplaceWasConfigured {
		if change.MarketplaceRemoved {
			if err := h.addMarketplace(ctx); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		} else if err := h.updateMarketplace(ctx); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	} else if change.MarketplaceAdded {
		if err := h.removeMarketplace(ctx); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if change.PluginWasInstalled && change.PreviousPlugin != nil {
		if err := h.installPlugin(ctx, change.PluginID); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		} else if !change.PreviousPlugin.Enabled {
			if err := h.disablePlugin(ctx, change.PluginID); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		}
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("Claude 插件回滚失败: %s", strings.Join(rollbackErrors, "; "))
	}
	return nil
}

func (h *Host) Commit(context.Context, pluginhost.NativeChange) error {
	return nil
}

func (h *Host) populateComponents(state pluginhost.State, target pluginhost.HostTarget, current inventory) pluginhost.State {
	for _, name := range target.Skills {
		status := pluginhost.StatusAbsent
		reason := "Claude 插件尚未安装"
		if current.plugin != nil {
			status = pluginhost.StatusRepairRequired
			reason = "Claude 插件缓存缺少 Skill"
			if regularFile(filepath.Join(current.plugin.InstallPath, "skills", name, "SKILL.md")) {
				status = pluginhost.StatusReady
				reason = ""
			}
		}
		state.Components = append(state.Components, component("skill", name, status, reason))
	}
	servers := map[string]claudeMCPServer{}
	if current.plugin != nil {
		if manifest, err := readClaudeMCP(filepath.Join(current.plugin.InstallPath, ".mcp.json")); err == nil {
			servers = manifest.Servers
		}
	}
	for _, targetMCP := range target.MCP {
		status := pluginhost.StatusAbsent
		reason := "Claude 插件尚未安装"
		if current.plugin != nil {
			status = pluginhost.StatusRepairRequired
			reason = "Claude 插件缓存缺少 MCP Server"
			if server, found := servers[targetMCP.Name]; found && server.Type == targetMCP.Type && strings.TrimSpace(server.Command) != "" {
				status = pluginhost.StatusReady
				reason = ""
			}
		}
		state.Components = append(state.Components, component("mcp", targetMCP.Name, status, reason))
	}
	return state
}

func component(kind, name string, status pluginhost.Status, reason string) pluginhost.ComponentState {
	return pluginhost.ComponentState{Component: pluginhost.ComponentRef{Type: kind, Name: name}, Status: status, Reason: reason}
}

func readClaudeMCP(path string) (claudeMCPManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return claudeMCPManifest{}, err
	}
	var manifest claudeMCPManifest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return claudeMCPManifest{}, err
	}
	return manifest, nil
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (h *Host) readInventory(ctx context.Context, target pluginhost.HostTarget) (inventory, error) {
	marketplaces, err := h.marketplaces(ctx)
	if err != nil {
		return inventory{}, err
	}
	plugins, err := h.plugins(ctx)
	if err != nil {
		return inventory{}, err
	}
	current := inventory{}
	for index := range marketplaces {
		if marketplaces[index].Name == h.config.MarketplaceName {
			current.marketplace = &marketplaces[index]
			break
		}
	}
	pluginID := h.pluginID(target.Release.PluginID)
	for index := range plugins {
		if plugins[index].ID == pluginID {
			current.plugin = &plugins[index]
			break
		}
	}
	body, err := json.Marshal(struct {
		Marketplace *marketplaceListItem `json:"marketplace"`
		Plugin      *pluginListItem      `json:"plugin"`
	}{current.marketplace, current.plugin})
	if err != nil {
		return inventory{}, err
	}
	sum := sha256.Sum256(body)
	current.generation = "sha256:" + hex.EncodeToString(sum[:])
	return current, nil
}

func (h *Host) marketplaces(ctx context.Context) ([]marketplaceListItem, error) {
	var response []marketplaceListItem
	if err := h.runJSON(ctx, "marketplace.list", &response, "plugin", "marketplace", "list", "--json"); err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fault.Invalid("CLAUDE_MARKETPLACE_LIST_INVALID", "Claude Marketplace 列表必须是 JSON 数组")
	}
	return response, nil
}

func (h *Host) plugins(ctx context.Context) ([]pluginListItem, error) {
	var response []pluginListItem
	if err := h.runJSON(ctx, "plugin.list", &response, "plugin", "list", "--json"); err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fault.Invalid("CLAUDE_PLUGIN_LIST_INVALID", "Claude 插件列表必须是 JSON 数组")
	}
	return response, nil
}

func (h *Host) version(ctx context.Context) (string, error) {
	result, err := h.run(ctx, "--version")
	if err != nil {
		return "", err
	}
	for _, field := range strings.Fields(string(result.Stdout)) {
		candidate := strings.TrimPrefix(field, "v")
		parts := strings.SplitN(candidate, "-", 2)
		if _, ok := pluginhost.ParseSemanticVersion(parts[0]); ok {
			return parts[0], nil
		}
	}
	return "", fault.Invalid("CLAUDE_VERSION_INVALID", "Claude --version 未返回可识别的语义版本")
}

func (h *Host) validateProjection(ctx context.Context, root string) error {
	return h.runMutation(ctx, "plugin.validate", "plugin", "validate", root, "--strict")
}

func (h *Host) addMarketplace(ctx context.Context) error {
	return h.runMutation(ctx, "marketplace.add", "plugin", "marketplace", "add", h.config.ProjectionRoot, "--scope", "user")
}

func (h *Host) updateMarketplace(ctx context.Context) error {
	return h.runMutation(ctx, "marketplace.update", "plugin", "marketplace", "update", h.config.MarketplaceName)
}

func (h *Host) removeMarketplace(ctx context.Context) error {
	return h.runMutation(ctx, "marketplace.remove", "plugin", "marketplace", "remove", h.config.MarketplaceName, "--scope", "user")
}

func (h *Host) installPlugin(ctx context.Context, pluginID string) error {
	return h.runMutation(ctx, "plugin.install", "plugin", "install", pluginID, "--scope", "user")
}

func (h *Host) updatePlugin(ctx context.Context, pluginID string) error {
	return h.runMutation(ctx, "plugin.update", "plugin", "update", pluginID, "--scope", "user")
}

func (h *Host) uninstallPlugin(ctx context.Context, pluginID string) error {
	return h.runMutation(ctx, "plugin.uninstall", "plugin", "uninstall", pluginID, "--scope", "user", "--keep-data")
}

func (h *Host) enablePlugin(ctx context.Context, pluginID string) error {
	return h.runMutation(ctx, "plugin.enable", "plugin", "enable", pluginID, "--scope", "user")
}

func (h *Host) disablePlugin(ctx context.Context, pluginID string) error {
	return h.runMutation(ctx, "plugin.disable", "plugin", "disable", pluginID, "--scope", "user")
}

func (h *Host) runJSON(ctx context.Context, operation string, target any, args ...string) error {
	result, err := h.run(ctx, args...)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	if err := decoder.Decode(target); err != nil {
		domainErr := fault.Invalid("CLAUDE_JSON_INVALID", "Claude 命令没有返回可解析的 JSON")
		domainErr.Details = map[string]any{"operation": operation, "error": err.Error()}
		return domainErr
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fault.Invalid("CLAUDE_JSON_INVALID", "Claude 命令返回了多个 JSON 值")
	}
	return nil
}

func (h *Host) runMutation(ctx context.Context, operation string, args ...string) error {
	result, err := h.runner.Run(ctx, Command{Name: h.config.Binary, Args: args, Env: h.environment()})
	if err != nil || result.ExitCode != 0 {
		return commandError(operation, result, err)
	}
	return nil
}

func (h *Host) run(ctx context.Context, args ...string) (CommandResult, error) {
	result, err := h.runner.Run(ctx, Command{Name: h.config.Binary, Args: args, Env: h.environment()})
	if err != nil || result.ExitCode != 0 {
		return result, commandError(strings.Join(args, " "), result, err)
	}
	return result, nil
}

func (h *Host) environment() map[string]string {
	environment := map[string]string{}
	if h.config.ConfigDir != "" {
		environment["CLAUDE_CONFIG_DIR"] = h.config.ConfigDir
	}
	return environment
}

func (h *Host) change(change nativeChange) pluginhost.NativeChange {
	body, _ := json.Marshal(change)
	return pluginhost.NativeChange{Data: body}
}

func (h *Host) pluginID(pluginName string) string {
	return pluginName + "@" + h.config.MarketplaceName
}

func (h *Host) expectedProjectedPackageRoot(release pluginhost.ReleaseRef) (string, error) {
	digest := strings.TrimPrefix(release.Digest, "sha256:")
	return canonicalPath(filepath.Join(h.config.ProjectionRoot, "plugins", release.PluginID, digest))
}

func commandError(operation string, result CommandResult, cause error) *fault.Error {
	domainErr := fault.E("runtime", "claude", "CLAUDE_COMMAND_FAILED", "Claude 插件命令执行失败", 5)
	domainErr.Retryable = true
	domainErr.Details = map[string]any{
		"operation": operation,
		"exit_code": result.ExitCode,
		"stderr":    strings.TrimSpace(string(result.Stderr)),
	}
	if cause != nil {
		domainErr.Details.(map[string]any)["cause"] = cause.Error()
	}
	return domainErr
}
