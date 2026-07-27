package codexplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const PlanSchemaVersion = "1.0"

type Spec struct {
	CodexBinary       string `json:"codex_binary"`
	MarketplaceName   string `json:"marketplace_name"`
	MarketplaceSource string `json:"marketplace_source"`
	MarketplaceRef    string `json:"marketplace_ref"`
	PluginName        string `json:"plugin_name"`
	PluginID          string `json:"plugin_id"`
	PluginVersion     string `json:"plugin_version"`
}

func DefaultSpec(version string) Spec {
	return Spec{
		CodexBinary:       "codex",
		MarketplaceName:   "contentcloud",
		MarketplaceSource: "limecloud/contentcloud",
		MarketplaceRef:    "v" + version,
		PluginName:        "contentcloud-video-production",
		PluginID:          "contentcloud-video-production@contentcloud",
		PluginVersion:     version,
	}
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandRunner interface {
	Run(context.Context, string, ...string) (CommandResult, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	command := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return result, err
}

type ComponentState struct {
	Status  string `json:"status"`
	Current string `json:"current,omitempty"`
	Wanted  string `json:"wanted"`
	Reason  string `json:"reason,omitempty"`
}

type State struct {
	Status      string         `json:"status"`
	Marketplace ComponentState `json:"marketplace"`
	Plugin      ComponentState `json:"plugin"`
}

type Action struct {
	Kind        string   `json:"kind"`
	Command     string   `json:"command"`
	Arguments   []string `json:"arguments"`
	Description string   `json:"description"`
}

type Plan struct {
	SchemaVersion        string   `json:"schema_version"`
	State                string   `json:"state"`
	Spec                 Spec     `json:"spec"`
	Detected             State    `json:"detected"`
	Actions              []Action `json:"actions"`
	BlockingReasons      []string `json:"blocking_reasons"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
}

type Receipt struct {
	MarketplaceAdded     bool   `json:"marketplace_added"`
	AddedMarketplaceName string `json:"added_marketplace_name,omitempty"`
	PluginAdded          bool   `json:"plugin_added"`
	AddedPluginID        string `json:"added_plugin_id,omitempty"`
}

type ApplyResult struct {
	Applied        bool     `json:"applied"`
	Idempotent     bool     `json:"idempotent"`
	Receipt        Receipt  `json:"receipt"`
	State          State    `json:"state"`
	RollbackErrors []string `json:"rollback_errors"`
}

type LaunchResult struct {
	Opened         bool   `json:"opened"`
	Method         string `json:"method,omitempty"`
	WorkspacePath  string `json:"workspace_path"`
	DeepLink       string `json:"deep_link"`
	RecoveryPrompt string `json:"recovery_prompt"`
	Error          string `json:"error,omitempty"`
}

type Adapter struct {
	Spec   Spec
	Runner CommandRunner
	GOOS   string
}

func New(spec Spec, runner CommandRunner) (*Adapter, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Adapter{Spec: spec, Runner: runner, GOOS: runtime.GOOS}, nil
}

func (a *Adapter) Detect(ctx context.Context) (State, error) {
	marketplaces, err := a.marketplaces(ctx)
	if err != nil {
		return State{}, err
	}
	plugins, err := a.plugins(ctx)
	if err != nil {
		return State{}, err
	}
	marketplace := detectMarketplace(a.Spec, marketplaces)
	plugin := detectPlugin(a.Spec, plugins)
	status := combinedStatus(marketplace.Status, plugin.Status)
	return State{Status: status, Marketplace: marketplace, Plugin: plugin}, nil
}

func (a *Adapter) Plan(ctx context.Context) (Plan, error) {
	detected, err := a.Detect(ctx)
	if err != nil {
		return Plan{}, err
	}
	return planForState(a.Spec, detected), nil
}

func (a *Adapter) Validate(ctx context.Context) (State, error) {
	state, err := a.Detect(ctx)
	if err != nil {
		return State{}, err
	}
	if state.Status != "current" {
		err := domain.Conflict("CODEX_PLUGIN_VALIDATION_FAILED", "Codex Marketplace 或 ContentCloud Plugin 未达到固定版本")
		err.Details = state
		return state, err
	}
	return state, nil
}

func (a *Adapter) Apply(ctx context.Context, approved Plan, confirmed bool) (result ApplyResult, returnErr error) {
	if approved.SchemaVersion != PlanSchemaVersion || !reflect.DeepEqual(approved.Spec, a.Spec) {
		return result, domain.Invalid("CODEX_PLUGIN_PLAN_INVALID", "安装计划与当前 ContentCloud Codex Plugin 规格不一致")
	}
	fresh, err := a.Plan(ctx)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(approved, fresh) {
		err := domain.Conflict("CODEX_PLUGIN_PLAN_STALE", "Codex Plugin 状态在确认后发生变化，请重新生成安装计划")
		err.Details = map[string]any{"approved": approved, "current": fresh}
		return result, err
	}
	if fresh.State == "blocked" {
		err := domain.Conflict("CODEX_PLUGIN_INSTALL_BLOCKED", "现有 Codex Marketplace 或 Plugin 与固定规格冲突")
		err.Details = fresh.BlockingReasons
		return result, err
	}
	if len(fresh.Actions) == 0 {
		state, validateErr := a.Validate(ctx)
		return ApplyResult{Applied: false, Idempotent: true, State: state}, validateErr
	}
	if !confirmed {
		err := domain.Policy("CODEX_PLUGIN_INSTALL_CONFIRMATION_REQUIRED", "安装将修改当前用户的 Codex Marketplace 和 Plugin 状态", "检查 bootstrap plan 后传入明确确认参数")
		err.ExitCode = 2
		return result, err
	}

	defer func() {
		if returnErr == nil {
			return
		}
		rollbackContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		result.RollbackErrors = a.Rollback(rollbackContext, result.Receipt)
	}()

	for _, action := range fresh.Actions {
		switch action.Kind {
		case "marketplace.add":
			name, added, applyErr := a.addMarketplace(ctx, action)
			result.Receipt.MarketplaceAdded = added
			result.Receipt.AddedMarketplaceName = name
			if applyErr != nil {
				return result, applyErr
			}
		case "plugin.add":
			pluginID, added, applyErr := a.addPlugin(ctx, action)
			result.Receipt.PluginAdded = added
			result.Receipt.AddedPluginID = pluginID
			if applyErr != nil {
				return result, applyErr
			}
		default:
			return result, domain.Invalid("CODEX_PLUGIN_ACTION_INVALID", "安装计划包含未知动作")
		}
	}
	state, err := a.Validate(ctx)
	if err != nil {
		return result, err
	}
	result.Applied = true
	result.State = state
	return result, nil
}

func (a *Adapter) Rollback(ctx context.Context, receipt Receipt) []string {
	errorsFound := []string{}
	if receipt.PluginAdded {
		pluginID := receipt.AddedPluginID
		if pluginID == "" {
			pluginID = a.Spec.PluginID
		}
		if err := a.runMutation(ctx, "plugin.remove", "plugin", "remove", pluginID, "--json"); err != nil {
			errorsFound = append(errorsFound, err.Error())
		}
	}
	if receipt.MarketplaceAdded {
		marketplaceName := receipt.AddedMarketplaceName
		if marketplaceName == "" {
			marketplaceName = a.Spec.MarketplaceName
		}
		if err := a.runMutation(ctx, "marketplace.remove", "plugin", "marketplace", "remove", marketplaceName, "--json"); err != nil {
			errorsFound = append(errorsFound, err.Error())
		}
	}
	return errorsFound
}

func (a *Adapter) LaunchNewChat(ctx context.Context, workspace string) LaunchResult {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return LaunchResult{WorkspacePath: workspace, Error: err.Error()}
	}
	prompt := RecoveryPrompt(a.Spec)
	deepLink, err := NewChatDeepLink(a.Spec, absolute, prompt)
	if err != nil {
		return LaunchResult{WorkspacePath: absolute, RecoveryPrompt: prompt, Error: err.Error()}
	}
	result := LaunchResult{WorkspacePath: absolute, DeepLink: deepLink, RecoveryPrompt: prompt}
	launchErrors := []string{}
	if a.GOOS == "darwin" {
		commandResult, runErr := a.Runner.Run(ctx, "open", deepLink)
		if runErr == nil && commandResult.ExitCode == 0 {
			result.Opened = true
			result.Method = "deep_link"
			return result
		}
		launchErrors = append(launchErrors, commandFailure("open", commandResult, runErr).Error())
	}
	commandResult, runErr := a.Runner.Run(ctx, a.Spec.CodexBinary, "app", absolute)
	if runErr == nil && commandResult.ExitCode == 0 {
		result.Opened = true
		result.Method = "workspace"
		return result
	}
	launchErrors = append(launchErrors, commandFailure("codex app", commandResult, runErr).Error())
	result.Error = strings.Join(launchErrors, "; ")
	return result
}

func RecoveryPrompt(spec Spec) string {
	return fmt.Sprintf("[@ContentCloud Video Production](plugin://%s) Call contentcloud_workspace_conversation_context for this workspace, then show the available next tasks and ready handoffs.", spec.PluginID)
}

func NewChatDeepLink(spec Spec, workspace, prompt string) (string, error) {
	if err := validateSpec(spec); err != nil {
		return "", err
	}
	if prompt != RecoveryPrompt(spec) {
		return "", domain.Invalid("CODEX_PLUGIN_RECOVERY_PROMPT_INVALID", "新 Codex 对话必须使用固定 Plugin mention 和恢复 Prompt")
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("path", absolute)
	query.Set("prompt", prompt)
	return (&url.URL{Scheme: "codex", Host: "new", RawQuery: query.Encode()}).String(), nil
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
	SourceType string   `json:"sourceType"`
	Source     string   `json:"source"`
	Ref        string   `json:"ref"`
	RefName    string   `json:"ref_name"`
	Sparse     []string `json:"sparsePaths"`
}

type pluginListResponse struct {
	Installed []pluginListItem `json:"installed"`
	Available []pluginListItem `json:"available"`
}

type pluginListItem struct {
	PluginID        string `json:"pluginId"`
	Name            string `json:"name"`
	MarketplaceName string `json:"marketplaceName"`
	Version         string `json:"version"`
	Installed       bool   `json:"installed"`
	Enabled         bool   `json:"enabled"`
}

func (a *Adapter) marketplaces(ctx context.Context) ([]marketplaceListItem, error) {
	var response marketplaceListResponse
	if err := a.runJSON(ctx, "marketplace.list", &response, "plugin", "marketplace", "list", "--json"); err != nil {
		return nil, err
	}
	if response.Marketplaces == nil {
		return nil, domain.Invalid("CODEX_MARKETPLACE_LIST_INVALID", "Codex marketplace list JSON 缺少 marketplaces")
	}
	return response.Marketplaces, nil
}

func (a *Adapter) plugins(ctx context.Context) ([]pluginListItem, error) {
	var response pluginListResponse
	if err := a.runJSON(ctx, "plugin.list", &response, "plugin", "list", "--json"); err != nil {
		return nil, err
	}
	if response.Installed == nil || response.Available == nil {
		return nil, domain.Invalid("CODEX_PLUGIN_LIST_INVALID", "Codex plugin list JSON 缺少 installed 或 available")
	}
	return response.Installed, nil
}

func (a *Adapter) addMarketplace(ctx context.Context, action Action) (string, bool, error) {
	var response struct {
		MarketplaceName string `json:"marketplaceName"`
		InstalledRoot   string `json:"installedRoot"`
		AlreadyAdded    bool   `json:"alreadyAdded"`
	}
	if err := a.runJSON(ctx, action.Kind, &response, action.Arguments...); err != nil {
		return "", false, err
	}
	added := !response.AlreadyAdded
	if response.MarketplaceName != a.Spec.MarketplaceName || strings.TrimSpace(response.InstalledRoot) == "" {
		return response.MarketplaceName, added, domain.Conflict("CODEX_MARKETPLACE_ADD_MISMATCH", "Codex 返回的 Marketplace 安装结果与固定规格不一致")
	}
	return response.MarketplaceName, added, nil
}

func (a *Adapter) addPlugin(ctx context.Context, action Action) (string, bool, error) {
	var response struct {
		PluginID        string `json:"pluginId"`
		Name            string `json:"name"`
		MarketplaceName string `json:"marketplaceName"`
		Version         string `json:"version"`
		InstalledPath   string `json:"installedPath"`
	}
	if err := a.runJSON(ctx, action.Kind, &response, action.Arguments...); err != nil {
		return "", false, err
	}
	pluginID := response.PluginID
	if pluginID == "" {
		pluginID = a.Spec.PluginID
	}
	if response.PluginID != a.Spec.PluginID || response.Name != a.Spec.PluginName || response.MarketplaceName != a.Spec.MarketplaceName || response.Version != a.Spec.PluginVersion || strings.TrimSpace(response.InstalledPath) == "" {
		return pluginID, true, domain.Conflict("CODEX_PLUGIN_ADD_MISMATCH", "Codex 返回的 Plugin 安装结果与固定规格不一致")
	}
	return pluginID, true, nil
}

func (a *Adapter) runJSON(ctx context.Context, operation string, target any, args ...string) error {
	result, err := a.Runner.Run(ctx, a.Spec.CodexBinary, args...)
	if err != nil || result.ExitCode != 0 {
		return commandDomainError(operation, result, err)
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

func (a *Adapter) runMutation(ctx context.Context, operation string, args ...string) error {
	var response map[string]any
	return a.runJSON(ctx, operation, &response, args...)
}

func planForState(spec Spec, detected State) Plan {
	plan := Plan{SchemaVersion: PlanSchemaVersion, Spec: spec, Detected: detected, Actions: []Action{}, BlockingReasons: []string{}}
	if detected.Marketplace.Status == "outdated" || detected.Marketplace.Status == "broken" {
		plan.BlockingReasons = append(plan.BlockingReasons, detected.Marketplace.Reason)
	}
	if detected.Plugin.Status == "outdated" || detected.Plugin.Status == "broken" {
		plan.BlockingReasons = append(plan.BlockingReasons, detected.Plugin.Reason)
	}
	if len(plan.BlockingReasons) > 0 {
		plan.State = "blocked"
		return plan
	}
	if detected.Marketplace.Status == "absent" {
		arguments := []string{"plugin", "marketplace", "add", spec.MarketplaceSource}
		if spec.MarketplaceRef != "" {
			arguments = append(arguments, "--ref", spec.MarketplaceRef)
		}
		arguments = append(arguments, "--json")
		plan.Actions = append(plan.Actions, Action{Kind: "marketplace.add", Command: spec.CodexBinary, Arguments: arguments, Description: "Add the pinned ContentCloud Codex marketplace"})
	}
	if detected.Plugin.Status == "absent" {
		plan.Actions = append(plan.Actions, Action{Kind: "plugin.add", Command: spec.CodexBinary, Arguments: []string{"plugin", "add", spec.PluginID, "--json"}, Description: "Install the pinned ContentCloud video-production plugin"})
	}
	if len(plan.Actions) == 0 {
		plan.State = "noop"
		return plan
	}
	plan.State = "ready"
	plan.RequiresConfirmation = true
	return plan
}

func detectMarketplace(spec Spec, items []marketplaceListItem) ComponentState {
	wanted := marketplaceIdentity(spec)
	for _, item := range items {
		if item.Name != spec.MarketplaceName {
			continue
		}
		current := marketplaceSourceIdentity(item.MarketplaceSource)
		expectedSourceType := "git"
		if filepath.IsAbs(spec.MarketplaceSource) {
			expectedSourceType = "local"
		}
		if item.MarketplaceSource.SourceType != expectedSourceType {
			return ComponentState{Status: "broken", Current: current, Wanted: wanted, Reason: "ContentCloud Marketplace 来源类型与固定规格不一致"}
		}
		if !sameSource(spec.MarketplaceSource, item.MarketplaceSource.Source) {
			return ComponentState{Status: "outdated", Current: current, Wanted: wanted, Reason: "同名 ContentCloud Marketplace 指向了非受管来源"}
		}
		if spec.MarketplaceRef != "" {
			currentRef := item.MarketplaceSource.Ref
			if currentRef == "" {
				currentRef = item.MarketplaceSource.RefName
			}
			if currentRef != spec.MarketplaceRef {
				return ComponentState{Status: "outdated", Current: current, Wanted: wanted, Reason: "ContentCloud Marketplace Git ref 与固定版本不一致"}
			}
		}
		return ComponentState{Status: "current", Current: current, Wanted: wanted}
	}
	return ComponentState{Status: "absent", Wanted: wanted, Reason: "ContentCloud Marketplace 尚未安装"}
}

func detectPlugin(spec Spec, items []pluginListItem) ComponentState {
	wanted := spec.PluginID + "@" + spec.PluginVersion
	for _, item := range items {
		if item.PluginID != spec.PluginID {
			continue
		}
		current := item.PluginID + "@" + item.Version
		if item.Name != spec.PluginName || item.MarketplaceName != spec.MarketplaceName || !item.Installed || !item.Enabled {
			return ComponentState{Status: "broken", Current: current, Wanted: wanted, Reason: "ContentCloud Plugin 已存在，但身份、安装或启用状态无效"}
		}
		if item.Version != spec.PluginVersion {
			return ComponentState{Status: "outdated", Current: current, Wanted: wanted, Reason: "ContentCloud Plugin 版本与固定版本不一致"}
		}
		return ComponentState{Status: "current", Current: current, Wanted: wanted}
	}
	return ComponentState{Status: "absent", Wanted: wanted, Reason: "ContentCloud Plugin 尚未安装"}
}

func combinedStatus(values ...string) string {
	status := "current"
	for _, value := range values {
		switch value {
		case "broken":
			return "broken"
		case "outdated":
			status = "outdated"
		case "absent":
			if status == "current" {
				status = "absent"
			}
		}
	}
	return status
}

func validateSpec(spec Spec) error {
	if strings.TrimSpace(spec.CodexBinary) == "" || strings.TrimSpace(spec.MarketplaceName) == "" || strings.TrimSpace(spec.MarketplaceSource) == "" || strings.TrimSpace(spec.PluginName) == "" || strings.TrimSpace(spec.PluginID) == "" || strings.TrimSpace(spec.PluginVersion) == "" {
		return domain.Invalid("CODEX_PLUGIN_SPEC_INVALID", "Codex Plugin 固定规格字段不完整")
	}
	if spec.PluginID != spec.PluginName+"@"+spec.MarketplaceName {
		return domain.Invalid("CODEX_PLUGIN_SPEC_INVALID", "plugin_id 必须由固定 plugin 和 Marketplace 名称组成")
	}
	if !filepath.IsAbs(spec.MarketplaceSource) && strings.TrimSpace(spec.MarketplaceRef) == "" {
		return domain.Invalid("CODEX_PLUGIN_SPEC_INVALID", "远程 Marketplace 必须固定 Git ref")
	}
	return nil
}

func marketplaceIdentity(spec Spec) string {
	value := spec.MarketplaceName + "=" + spec.MarketplaceSource
	if spec.MarketplaceRef != "" {
		value += "@" + spec.MarketplaceRef
	}
	return value
}

func marketplaceSourceIdentity(source marketplaceSource) string {
	value := source.Source
	ref := source.Ref
	if ref == "" {
		ref = source.RefName
	}
	if ref != "" {
		value += "@" + ref
	}
	return value
}

func sameSource(expected, actual string) bool {
	return canonicalSource(expected) == canonicalSource(actual)
}

func canonicalSource(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimSuffix(value, "/")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "ssh://git@github.com/", "git@github.com:"} {
		if strings.HasPrefix(value, prefix) {
			return strings.ToLower(strings.TrimPrefix(value, prefix))
		}
	}
	if filepath.IsAbs(value) {
		absolute, err := filepath.Abs(value)
		if err == nil {
			return filepath.Clean(absolute)
		}
	}
	return strings.ToLower(value)
}

func commandDomainError(operation string, result CommandResult, err error) *domain.Error {
	domainErr := domain.E("runtime", "codex", "CODEX_COMMAND_FAILED", "Codex Plugin 命令执行失败", 5)
	domainErr.Retryable = true
	domainErr.Details = map[string]any{"operation": operation, "exit_code": result.ExitCode, "stderr": strings.TrimSpace(string(result.Stderr)), "cause": errorString(err)}
	return domainErr
}

func commandFailure(operation string, result CommandResult, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s exited with %d: %s", operation, result.ExitCode, strings.TrimSpace(string(result.Stderr)))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
