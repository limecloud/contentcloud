package codex

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

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
)

const minimumCodexVersion = "0.147.0"

type Config struct {
	Binary          string
	CodexHome       string
	MarketplaceName string
	ProjectionRoot  string
}

type Host struct {
	config Config
	runner CommandRunner
}

type nativeChange struct {
	Operation                string                `json:"operation"`
	PluginName               string                `json:"plugin_name"`
	PluginID                 string                `json:"plugin_id"`
	PreviousProjection       json.RawMessage       `json:"previous_projection"`
	MarketplaceWasConfigured bool                  `json:"marketplace_was_configured"`
	MarketplaceAdded         bool                  `json:"marketplace_added"`
	AddedMarketplaceName     string                `json:"added_marketplace_name,omitempty"`
	PluginWasInstalled       bool                  `json:"plugin_was_installed"`
	PreviousPlugin           *pluginListItem       `json:"previous_plugin,omitempty"`
	PluginAdded              bool                  `json:"plugin_added"`
	AddedPluginID            string                `json:"added_plugin_id,omitempty"`
	PluginRemoved            bool                  `json:"plugin_removed"`
	MarketplaceRemoved       bool                  `json:"marketplace_removed"`
	Target                   pluginhost.ReleaseRef `json:"target"`
}

type inventory struct {
	marketplace *marketplaceListItem
	plugin      *pluginListItem
	mcp         map[string]mcpListItem
	generation  string
}

type marketplaceListResponse struct {
	Marketplaces []marketplaceListItem `json:"marketplaces"`
}

type marketplaceListItem struct {
	Name              string            `json:"name"`
	Root              string            `json:"root"`
	MarketplaceSource marketplaceSource `json:"marketplaceSource"`
}

type marketplaceSource struct {
	SourceType string `json:"sourceType"`
	Source     string `json:"source"`
}

type pluginListResponse struct {
	Installed []pluginListItem `json:"installed"`
	Available []pluginListItem `json:"available"`
}

type pluginListItem struct {
	PluginID        string             `json:"pluginId"`
	Name            string             `json:"name"`
	MarketplaceName string             `json:"marketplaceName"`
	Version         string             `json:"version"`
	Installed       bool               `json:"installed"`
	Enabled         bool               `json:"enabled"`
	Source          pluginSource       `json:"source"`
	Marketplace     *marketplaceSource `json:"marketplaceSource,omitempty"`
}

type pluginSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type mcpListItem struct {
	Name           string       `json:"name"`
	Enabled        bool         `json:"enabled"`
	DisabledReason *string      `json:"disabled_reason"`
	Transport      mcpTransport `json:"transport"`
}

type mcpTransport struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	CWD     string            `json:"cwd"`
}

func New(config Config, runner CommandRunner) (*Host, error) {
	if strings.TrimSpace(config.Binary) == "" {
		config.Binary = "codex"
	}
	if strings.TrimSpace(config.MarketplaceName) == "" {
		config.MarketplaceName = "contentcloud"
	}
	if strings.TrimSpace(config.ProjectionRoot) == "" {
		return nil, domain.Invalid("CODEX_PLUGIN_HOST_CONFIG_INVALID", "Codex 插件宿主缺少本地 Marketplace 投影目录")
	}
	projectionRoot, err := filepath.Abs(config.ProjectionRoot)
	if err != nil {
		return nil, err
	}
	config.ProjectionRoot = filepath.Clean(projectionRoot)
	if config.CodexHome != "" {
		codexHome, err := filepath.Abs(config.CodexHome)
		if err != nil {
			return nil, err
		}
		config.CodexHome = filepath.Clean(codexHome)
		if err := os.MkdirAll(config.CodexHome, 0o700); err != nil {
			return nil, fmt.Errorf("create Codex home: %w", err)
		}
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Host{config: config, runner: runner}, nil
}

func (h *Host) ID() pluginhost.HostID {
	return pluginhost.HostCodex
}

func (h *Host) Capabilities(ctx context.Context) (pluginhost.Capabilities, error) {
	version, err := h.version(ctx)
	if err != nil {
		return pluginhost.Capabilities{}, err
	}
	supported := pluginhost.CompareSemanticVersion(version, minimumCodexVersion) >= 0
	return pluginhost.Capabilities{
		PluginDirectoryInstall: false,
		Skills:                 supported,
		MCPStdio:               supported,
		MCPStreamableHTTP:      false,
		NewSessionRequired:     true,
		AtomicInstall:          false,
		Rollback:               true,
		NativeExtensions:       []string{"agent-plugins/1.0.0", "codex/local-marketplace"},
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
		state.Reason = "Codex 版本低于 Agent Plugins 1.0.0 的已验证最低版本 " + minimumCodexVersion
		return state, nil
	}
	current, err := h.readInventory(ctx, target, true)
	if err != nil {
		return pluginhost.State{}, err
	}
	state.Generation = current.generation
	if current.marketplace == nil && current.plugin == nil {
		for _, name := range target.Skills {
			state.Components = append(state.Components, component("skill", name, pluginhost.StatusAbsent, "插件尚未安装"))
		}
		for _, server := range target.MCP {
			state.Components = append(state.Components, component("mcp", server.Name, pluginhost.StatusAbsent, "插件尚未安装"))
		}
		return state, nil
	}
	if current.marketplace != nil {
		projectionRoot, pathErr := canonicalPath(h.config.ProjectionRoot)
		marketplaceRoot, marketplaceErr := canonicalPath(current.marketplace.Root)
		if pathErr != nil || marketplaceErr != nil || current.marketplace.MarketplaceSource.SourceType != "local" || marketplaceRoot != projectionRoot {
			state.Status = pluginhost.StatusBlocked
			state.Reason = "同名 Codex Marketplace 不属于 ContentCloud 管理的设备投影"
			return h.populateComponents(state, target, current), nil
		}
	}
	if current.marketplace == nil || current.plugin == nil {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Codex Marketplace 投影与已安装插件状态不完整"
		return h.populateComponents(state, target, current), nil
	}
	state.Release = &target.Release
	if !current.plugin.Installed || !current.plugin.Enabled || current.plugin.Name != target.Release.PluginID || current.plugin.MarketplaceName != h.config.MarketplaceName {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Codex 插件身份、安装状态或启用状态无效"
		return h.populateComponents(state, target, current), nil
	}
	if current.plugin.Version != target.Release.Version {
		state.Status = pluginhost.StatusInstalled
		state.Reason = "Codex 已安装插件版本与目标 Release 不一致"
		return h.populateComponents(state, target, current), nil
	}
	expectedPackageRoot, err := h.projectedPackageRoot(target.Release.PluginID)
	if err != nil {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = err.Error()
		return h.populateComponents(state, target, current), nil
	}
	pluginSourcePath, err := canonicalPath(current.plugin.Source.Path)
	if err != nil || current.plugin.Source.Source != "local" || pluginSourcePath != expectedPackageRoot {
		state.Status = pluginhost.StatusRepairRequired
		state.Reason = "Codex 插件来源与 ContentCloud Release 投影不一致"
		return h.populateComponents(state, target, current), nil
	}
	state = h.populateComponents(state, target, current)
	state.Status = pluginhost.StatusReady
	for _, item := range state.Components {
		if item.Status != pluginhost.StatusReady {
			state.Status = pluginhost.StatusRepairRequired
			state.Reason = "Codex 未完整激活标准包组件"
			break
		}
	}
	return state, nil
}

func (h *Host) Apply(ctx context.Context, request pluginhost.NativeApply) (pluginhost.NativeChange, []pluginhost.InstalledComponent, error) {
	current, err := h.readInventory(ctx, request.Target, false)
	if err != nil {
		return pluginhost.NativeChange{}, nil, err
	}
	change := nativeChange{
		Operation:                "install",
		PluginName:               request.Target.Release.PluginID,
		PluginID:                 h.pluginID(request.Target.Release.PluginID),
		MarketplaceWasConfigured: current.marketplace != nil,
		PluginWasInstalled:       current.plugin != nil && current.plugin.Installed,
		PreviousPlugin:           current.plugin,
		Target:                   request.Target.Release,
	}
	previous, err := h.upsertProjection(request.Target.Release.PluginID, request.PackageRoot)
	if err != nil {
		return h.change(change), nil, err
	}
	change.PreviousProjection = previous

	marketplace, err := h.addMarketplace(ctx)
	if err != nil {
		return h.change(change), nil, err
	}
	change.MarketplaceAdded = !marketplace.AlreadyAdded
	change.AddedMarketplaceName = marketplace.MarketplaceName
	if marketplace.MarketplaceName != h.config.MarketplaceName {
		return h.change(change), nil, domain.Conflict("CODEX_MARKETPLACE_ADD_MISMATCH", "Codex 返回的 Marketplace 身份与设备投影不一致")
	}

	installed, err := h.addPlugin(ctx, change.PluginID)
	if err != nil {
		return h.change(change), nil, err
	}
	change.PluginAdded = true
	change.AddedPluginID = installed.PluginID
	if installed.PluginID != change.PluginID || installed.Name != change.PluginName || installed.MarketplaceName != h.config.MarketplaceName || installed.Version != request.Target.Release.Version {
		return h.change(change), nil, domain.Conflict("CODEX_PLUGIN_ADD_MISMATCH", "Codex 返回的插件身份或版本与目标 Release 不一致")
	}

	state, err := h.Detect(ctx, request.Target)
	if err != nil {
		return h.change(change), nil, err
	}
	if state.Status != pluginhost.StatusReady {
		return h.change(change), nil, domain.Conflict("CODEX_PLUGIN_VERIFY_FAILED", state.Reason)
	}
	components := make([]pluginhost.InstalledComponent, 0, len(state.Components))
	for _, item := range state.Components {
		installedComponent := pluginhost.InstalledComponent{ComponentRef: item.Component}
		if item.Component.Type == "mcp" {
			installedComponent.Transport = "stdio"
		}
		components = append(components, installedComponent)
	}
	return h.change(change), components, nil
}

func (h *Host) Remove(ctx context.Context, request pluginhost.NativeRemove) (pluginhost.NativeChange, error) {
	current, err := h.readInventory(ctx, request.Target, false)
	if err != nil {
		return pluginhost.NativeChange{}, err
	}
	_, previousProjection, err := h.readProjection()
	if err != nil {
		return pluginhost.NativeChange{}, err
	}
	change := nativeChange{
		Operation:                "remove",
		PluginName:               request.Target.Release.PluginID,
		PluginID:                 h.pluginID(request.Target.Release.PluginID),
		PreviousProjection:       previousProjection,
		MarketplaceWasConfigured: current.marketplace != nil,
		PluginWasInstalled:       current.plugin != nil && current.plugin.Installed,
		PreviousPlugin:           current.plugin,
		Target:                   request.Target.Release,
	}
	if change.PluginWasInstalled {
		if err := h.removePlugin(ctx, change.PluginID); err != nil {
			return h.change(change), err
		}
		change.PluginRemoved = true
	}
	_, empty, err := h.removeProjection(change.PluginName)
	if err != nil {
		return h.change(change), err
	}
	if empty && change.MarketplaceWasConfigured {
		if err := h.removeMarketplace(ctx, h.config.MarketplaceName); err != nil {
			return h.change(change), err
		}
		change.MarketplaceRemoved = true
	}
	state, err := h.Detect(ctx, request.Target)
	if err != nil {
		return h.change(change), err
	}
	if state.Status != pluginhost.StatusAbsent {
		return h.change(change), domain.Conflict("CODEX_PLUGIN_REMOVE_VERIFY_FAILED", "Codex 插件删除后仍处于可见状态")
	}
	return h.change(change), nil
}

func (h *Host) Rollback(ctx context.Context, native pluginhost.NativeChange) error {
	var change nativeChange
	if err := json.Unmarshal(native.Data, &change); err != nil {
		return domain.Invalid("CODEX_PLUGIN_ROLLBACK_INVALID", "Codex 插件原生回滚状态无法解析")
	}
	if len(change.PreviousProjection) == 0 {
		return domain.Invalid("CODEX_PLUGIN_ROLLBACK_INVALID", "Codex 插件原生回滚状态缺少先前投影")
	}
	var rollbackErrors []string
	if change.Operation == "install" && change.PluginAdded {
		pluginID := change.AddedPluginID
		if pluginID == "" {
			pluginID = change.PluginID
		}
		if !change.PluginWasInstalled {
			if err := h.removePlugin(ctx, pluginID); err != nil {
				rollbackErrors = append(rollbackErrors, err.Error())
			}
		}
	}
	if err := h.restoreProjection(change.PreviousProjection); err != nil {
		rollbackErrors = append(rollbackErrors, err.Error())
	}
	if change.Operation == "remove" && change.MarketplaceRemoved {
		if _, err := h.addMarketplace(ctx); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if change.PluginWasInstalled && (change.PluginAdded || change.PluginRemoved) {
		if _, err := h.addPlugin(ctx, change.PluginID); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if change.Operation == "install" && change.MarketplaceAdded && !change.MarketplaceWasConfigured {
		if err := h.removeMarketplace(ctx, change.AddedMarketplaceName); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("Codex 插件回滚失败: %s", strings.Join(rollbackErrors, "; "))
	}
	return nil
}

func (h *Host) Commit(context.Context, pluginhost.NativeChange) error {
	return nil
}

func (h *Host) populateComponents(state pluginhost.State, target pluginhost.HostTarget, current inventory) pluginhost.State {
	pluginReady := current.plugin != nil && current.plugin.Installed && current.plugin.Enabled && current.plugin.Version == target.Release.Version
	for _, name := range target.Skills {
		status := pluginhost.StatusRepairRequired
		reason := "Codex 插件未按目标 Release 启用"
		if pluginReady {
			status = pluginhost.StatusReady
			reason = ""
		}
		state.Components = append(state.Components, component("skill", name, status, reason))
	}
	expectedPluginRoot := h.expectedInstalledPluginRoot(target.Release)
	for _, targetMCP := range target.MCP {
		status := pluginhost.StatusRepairRequired
		reason := "Codex 未加载该 MCP Server"
		if server, found := current.mcp[targetMCP.Name]; found && server.Enabled && server.Transport.Type == "stdio" {
			root, rootErr := canonicalPath(server.Transport.Env["PLUGIN_ROOT"])
			data, dataErr := canonicalPath(server.Transport.Env["PLUGIN_DATA"])
			cwd, cwdErr := canonicalPath(server.Transport.CWD)
			if rootErr == nil && cwdErr == nil && root == expectedPluginRoot && cwd == expectedPluginRoot && dataErr == nil && h.isCodexPluginDataPath(data) {
				status = pluginhost.StatusReady
				reason = ""
			} else {
				reason = "Codex MCP 的 PLUGIN_ROOT、PLUGIN_DATA 或 cwd 不属于目标插件安装"
			}
		}
		state.Components = append(state.Components, component("mcp", targetMCP.Name, status, reason))
	}
	return state
}

func component(kind, name string, status pluginhost.Status, reason string) pluginhost.ComponentState {
	return pluginhost.ComponentState{
		Component: pluginhost.ComponentRef{Type: kind, Name: name},
		Status:    status,
		Reason:    reason,
	}
}

func (h *Host) readInventory(ctx context.Context, target pluginhost.HostTarget, includeMCP bool) (inventory, error) {
	marketplaces, err := h.marketplaces(ctx)
	if err != nil {
		return inventory{}, err
	}
	plugins, err := h.plugins(ctx)
	if err != nil {
		return inventory{}, err
	}
	current := inventory{mcp: map[string]mcpListItem{}}
	for index := range marketplaces {
		if marketplaces[index].Name == h.config.MarketplaceName {
			current.marketplace = &marketplaces[index]
			break
		}
	}
	pluginID := h.pluginID(target.Release.PluginID)
	for index := range plugins {
		if plugins[index].PluginID == pluginID {
			current.plugin = &plugins[index]
			break
		}
	}
	if includeMCP && current.plugin != nil {
		servers, err := h.mcpServers(ctx)
		if err != nil {
			return inventory{}, err
		}
		current.mcp = servers
	}
	body, err := json.Marshal(struct {
		Marketplace *marketplaceListItem   `json:"marketplace"`
		Plugin      *pluginListItem        `json:"plugin"`
		MCP         map[string]mcpListItem `json:"mcp"`
	}{current.marketplace, current.plugin, current.mcp})
	if err != nil {
		return inventory{}, err
	}
	sum := sha256.Sum256(body)
	current.generation = "sha256:" + hex.EncodeToString(sum[:])
	return current, nil
}

func (h *Host) marketplaces(ctx context.Context) ([]marketplaceListItem, error) {
	var response marketplaceListResponse
	if err := h.runJSON(ctx, "marketplace.list", &response, "plugin", "marketplace", "list", "--json"); err != nil {
		return nil, err
	}
	if response.Marketplaces == nil {
		return nil, domain.Invalid("CODEX_MARKETPLACE_LIST_INVALID", "Codex Marketplace 列表缺少 marketplaces 字段")
	}
	return response.Marketplaces, nil
}

func (h *Host) plugins(ctx context.Context) ([]pluginListItem, error) {
	var response pluginListResponse
	if err := h.runJSON(ctx, "plugin.list", &response, "plugin", "list", "--marketplace", h.config.MarketplaceName, "--json"); err != nil {
		return nil, err
	}
	if response.Installed == nil || response.Available == nil {
		return nil, domain.Invalid("CODEX_PLUGIN_LIST_INVALID", "Codex 插件列表缺少 installed 或 available 字段")
	}
	return response.Installed, nil
}

func (h *Host) mcpServers(ctx context.Context) (map[string]mcpListItem, error) {
	var response []mcpListItem
	if err := h.runJSON(ctx, "mcp.list", &response, "mcp", "list", "--json"); err != nil {
		return nil, err
	}
	servers := make(map[string]mcpListItem, len(response))
	for _, item := range response {
		servers[item.Name] = item
	}
	return servers, nil
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
	return "", domain.Invalid("CODEX_VERSION_INVALID", "Codex --version 未返回可识别的语义版本")
}

type marketplaceAddResult struct {
	MarketplaceName string `json:"marketplaceName"`
	InstalledRoot   string `json:"installedRoot"`
	AlreadyAdded    bool   `json:"alreadyAdded"`
}

func (h *Host) addMarketplace(ctx context.Context) (marketplaceAddResult, error) {
	var response marketplaceAddResult
	if err := h.runJSON(ctx, "marketplace.add", &response, "plugin", "marketplace", "add", h.config.ProjectionRoot, "--json"); err != nil {
		return response, err
	}
	if strings.TrimSpace(response.MarketplaceName) == "" || strings.TrimSpace(response.InstalledRoot) == "" {
		return response, domain.Invalid("CODEX_MARKETPLACE_ADD_INVALID", "Codex Marketplace 安装结果缺少身份或路径")
	}
	return response, nil
}

type pluginAddResult struct {
	PluginID        string `json:"pluginId"`
	Name            string `json:"name"`
	MarketplaceName string `json:"marketplaceName"`
	Version         string `json:"version"`
	InstalledPath   string `json:"installedPath"`
}

func (h *Host) addPlugin(ctx context.Context, pluginID string) (pluginAddResult, error) {
	var response pluginAddResult
	if err := h.runJSON(ctx, "plugin.add", &response, "plugin", "add", pluginID, "--json"); err != nil {
		return response, err
	}
	if strings.TrimSpace(response.PluginID) == "" || strings.TrimSpace(response.InstalledPath) == "" {
		return response, domain.Invalid("CODEX_PLUGIN_ADD_INVALID", "Codex 插件安装结果缺少身份或路径")
	}
	return response, nil
}

func (h *Host) removePlugin(ctx context.Context, pluginID string) error {
	var response map[string]any
	return h.runJSON(ctx, "plugin.remove", &response, "plugin", "remove", pluginID, "--json")
}

func (h *Host) removeMarketplace(ctx context.Context, marketplaceName string) error {
	if strings.TrimSpace(marketplaceName) == "" {
		marketplaceName = h.config.MarketplaceName
	}
	var response map[string]any
	return h.runJSON(ctx, "marketplace.remove", &response, "plugin", "marketplace", "remove", marketplaceName, "--json")
}

func (h *Host) runJSON(ctx context.Context, operation string, target any, args ...string) error {
	result, err := h.run(ctx, args...)
	if err != nil {
		return commandError(operation, result, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	if err := decoder.Decode(target); err != nil {
		domainErr := domain.Invalid("CODEX_JSON_INVALID", "Codex 命令没有返回可解析的 JSON")
		domainErr.Details = map[string]any{"operation": operation, "error": err.Error()}
		return domainErr
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.Invalid("CODEX_JSON_INVALID", "Codex 命令返回了多个 JSON 值")
	}
	return nil
}

func (h *Host) run(ctx context.Context, args ...string) (CommandResult, error) {
	environment := map[string]string{}
	if h.config.CodexHome != "" {
		environment["CODEX_HOME"] = h.config.CodexHome
	}
	result, err := h.runner.Run(ctx, Command{Name: h.config.Binary, Args: args, Env: environment})
	if err != nil || result.ExitCode != 0 {
		return result, commandError(strings.Join(args, " "), result, err)
	}
	return result, nil
}

func (h *Host) change(change nativeChange) pluginhost.NativeChange {
	body, _ := json.Marshal(change)
	return pluginhost.NativeChange{Data: body}
}

func (h *Host) pluginID(pluginName string) string {
	return pluginName + "@" + h.config.MarketplaceName
}

func (h *Host) projectedPackageRoot(pluginName string) (string, error) {
	manifest, _, err := h.readProjection()
	if err != nil {
		return "", err
	}
	for _, entry := range manifest.Plugins {
		if entry.Name == pluginName {
			return h.resolvePluginSource(entry.Source.Path)
		}
	}
	return "", fmt.Errorf("Codex Marketplace 投影缺少插件 %s", pluginName)
}

func (h *Host) expectedInstalledPluginRoot(release pluginhost.ReleaseRef) string {
	root := h.codexHome()
	path, err := canonicalPath(filepath.Join(root, "plugins", "cache", h.config.MarketplaceName, release.PluginID, release.Version))
	if err != nil {
		return filepath.Clean(filepath.Join(root, "plugins", "cache", h.config.MarketplaceName, release.PluginID, release.Version))
	}
	return path
}

func (h *Host) isCodexPluginDataPath(path string) bool {
	wanted, err := canonicalPath(filepath.Join(h.codexHome(), "plugins", "data", "agent-plugins"))
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(wanted, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (h *Host) codexHome() string {
	if h.config.CodexHome != "" {
		return h.config.CodexHome
	}
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}

func canonicalPath(path string) (string, error) {
	return pluginhost.CanonicalPath(path)
}

func commandError(operation string, result CommandResult, cause error) *domain.Error {
	domainErr := domain.E("runtime", "codex", "CODEX_COMMAND_FAILED", "Codex 插件命令执行失败", 5)
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
