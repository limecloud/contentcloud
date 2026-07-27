package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/bootstrapcheck"
	"github.com/limecloud/contentcloud/internal/codexplugin"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

const bootstrapSchemaVersion = "1.0"

type bootstrapPlan struct {
	SchemaVersion        string                  `json:"schema_version"`
	PlanID               string                  `json:"plan_id"`
	State                string                  `json:"state"`
	Host                 string                  `json:"host"`
	CLIPackage           string                  `json:"cli_package"`
	CLIVersion           string                  `json:"cli_version"`
	ServerURL            string                  `json:"server_url"`
	SessionID            string                  `json:"session_id,omitempty"`
	AuthorizationMode    string                  `json:"authorization_mode"`
	Prerequisites        *bootstrapcheck.Report  `json:"prerequisites,omitempty"`
	Plugin               codexplugin.Plan        `json:"plugin"`
	Workspace            localworkspace.InitPlan `json:"workspace"`
	BlockingReasons      []string                `json:"blocking_reasons"`
	RequiresConfirmation bool                    `json:"requires_confirmation"`
	WouldAuthorizeDevice bool                    `json:"would_authorize_device"`
	WouldRegister        bool                    `json:"would_register_workspace"`
	WouldUploadFiles     bool                    `json:"would_upload_files"`
	WouldEnableDaemon    bool                    `json:"would_enable_daemon"`
	WouldOpenNewChat     bool                    `json:"would_open_new_chat"`
}

func (r *Root) bootstrapCommand() *cobra.Command {
	command := &cobra.Command{Use: "bootstrap", Short: "Install the pinned Codex plugin and initialize a ContentCloud workspace"}
	command.AddCommand(r.bootstrapPreflightCommand(), r.bootstrapPlanCommand(), r.bootstrapApplyCommand(), r.bootstrapResumeCommand(), r.bootstrapDiagnosticCommand())
	return command
}

func (r *Root) bootstrapPreflightCommand() *cobra.Command {
	var offline bool
	command := &cobra.Command{
		Use:   "preflight [directory]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Check Node, Codex, network, and workspace prerequisites",
		RunE: func(command *cobra.Command, args []string) error {
			cfg, err := localconfig.Load()
			if err != nil {
				return err
			}
			report := bootstrapcheck.Run(command.Context(), bootstrapcheck.Options{Directory: optionalDirectory(args), ServerURL: r.resolveServer(cfg), Offline: offline})
			if err := bootstrapcheck.ValidateReport(report); err != nil {
				return domain.E("internal", "bootstrap_preflight", "BOOTSTRAP_PREFLIGHT_INVALID", err.Error(), 1)
			}
			return r.writeOK("bootstrap.preflight", report)
		},
	}
	command.Flags().BoolVar(&offline, "offline", false, "skip network reachability checks")
	return command
}

func (r *Root) bootstrapPlanCommand() *cobra.Command {
	var sessionID string
	command := &cobra.Command{
		Use:   "plan [directory]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Inspect Codex and workspace changes without modifying either",
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateBootstrapSession(sessionID); err != nil {
				return err
			}
			plan, _, err := r.buildBootstrapPlan(command.Context(), optionalDirectory(args))
			if err != nil {
				return err
			}
			plan, err = r.withBootstrapPrerequisites(command.Context(), plan, strings.TrimSpace(sessionID))
			if err != nil {
				return err
			}
			return r.writeOK("bootstrap.plan", plan)
		},
	}
	command.Flags().StringVar(&sessionID, "session", "", "public ConnectSession ID used with browser device authorization")
	return command
}

func (r *Root) bootstrapApplyCommand() *cobra.Command {
	var sessionID, name, planID string
	var accept, openCodex bool
	command := &cobra.Command{
		Use:   "apply [directory]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Apply one confirmed Codex plugin and ContentCloud workspace plan",
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateBootstrapSession(sessionID); err != nil {
				return err
			}
			plan, adapter, err := r.buildBootstrapPlan(command.Context(), optionalDirectory(args))
			if err != nil {
				return err
			}
			plan, err = r.withBootstrapPrerequisites(command.Context(), plan, strings.TrimSpace(sessionID))
			if err != nil {
				return err
			}
			if plan.State != "ready" {
				err := domain.Conflict("BOOTSTRAP_PLAN_BLOCKED", "当前 Codex 或目标目录不能执行 bootstrap apply")
				err.Details = plan.BlockingReasons
				if plan.State == "resume_required" {
					err.Hint = "对已初始化的 ContentCloud Workspace 使用 bootstrap resume"
				}
				return err
			}
			if strings.TrimSpace(planID) == "" {
				return domain.Invalid("BOOTSTRAP_PLAN_ID_REQUIRED", "--plan-id 必填；请使用刚刚确认的 bootstrap plan_id")
			}
			if plan.PlanID != strings.TrimSpace(planID) {
				err := domain.Conflict("BOOTSTRAP_PLAN_STALE", "当前 bootstrap 状态与用户确认的 plan_id 不一致")
				err.Details = map[string]any{"approved_plan_id": strings.TrimSpace(planID), "current_plan_id": plan.PlanID}
				err.Hint = "重新运行 bootstrap plan、检查变化并再次确认"
				return err
			}
			if !accept {
				err := domain.Policy("BOOTSTRAP_CONFIRMATION_REQUIRED", "bootstrap 将安装固定 Codex Plugin、绑定设备并写入目标 Workspace", "先检查 bootstrap plan，再传入 --accept")
				err.ExitCode = 2
				return err
			}
			verifier, err := r.environmentManifestVerifier()
			if err != nil {
				return err
			}
			registryVerifier, err := r.environmentRegistryVerifier()
			if err != nil {
				return err
			}

			var cfg localconfig.Config
			var connected app.ConnectDeviceResult
			var progress *bootstrapProgressReporter
			cfg, connected, progress, err = r.authorizeBootstrapDevice(command.Context(), strings.TrimSpace(sessionID), name, plan.Prerequisites)
			if err != nil {
				return err
			}
			if progress != nil {
				progress.append(command.Context(), "plugin_installing", "started", "", "", "")
			}
			pluginResult, err := adapter.Apply(command.Context(), plan.Plugin, true)
			if err != nil {
				if progress != nil {
					progress.append(command.Context(), "plugin_installing", "failed", "codex.plugin.identity", "CODEX_PLUGIN_INSTALL_FAILED", "repair.plugin.install")
				}
				return withBootstrapDetails(err, map[string]any{"plugin": pluginResult})
			}
			if progress != nil {
				progress.append(command.Context(), "plugin_installing", "passed", "codex.plugin.identity", "", "")
			}
			if progress != nil {
				progress.append(command.Context(), "workspace_initializing", "started", "", "", "")
			}
			status, err := initializeCodexPluginWorkspace(plan.Workspace.Root, r.resolveServer(cfg), connected, r.currentTime())
			if err != nil {
				if progress != nil {
					progress.append(command.Context(), "workspace_initializing", "failed", "workspace.binding", "WORKSPACE_INITIALIZATION_FAILED", "repair.bootstrap.resume")
				}
				return withBootstrapDetails(err, map[string]any{"recovery": "retry with bootstrap resume after resolving the workspace error"})
			}
			if progress != nil {
				progress.append(command.Context(), "workspace_initializing", "passed", "workspace.binding", "", "")
			}
			if connected.EnvironmentManifest == nil {
				return withBootstrapDetails(domain.Conflict("ENVIRONMENT_MANIFEST_REQUIRED", "服务端未返回签名 Creative Environment Manifest"), map[string]any{"recovery": "configure the server Environment Control Plane, then run bootstrap resume"})
			}
			registry, err := fetchEnvironmentRegistry(command.Context(), r.resolveServer(cfg), connected.WorkspaceToken)
			if err != nil {
				return withBootstrapDetails(err, map[string]any{"recovery": "retry with bootstrap resume to refresh the signed Plugin Registry"})
			}
			environmentState, err := storeBootstrapEnvironment(status.Root, *connected.EnvironmentManifest, registry, adapter.Spec, verifier, registryVerifier, r.currentTime())
			if err != nil {
				return withBootstrapDetails(err, map[string]any{"recovery": "refresh the trusted Environment configuration, then run bootstrap resume"})
			}
			cfg.WorkspaceRoot = status.Root
			if err := localconfig.Save(cfg); err != nil {
				return withBootstrapDetails(err, map[string]any{"recovery": "retry with bootstrap resume to persist the workspace root"})
			}
			if progress != nil {
				progress.append(command.Context(), "doctor_running", "started", "", "", "")
			}
			report, err := localworkspace.DoctorWithEnvironment(status.Root, verifier, registryVerifier, r.currentTime())
			if err != nil {
				return err
			}
			if err := requireHealthyWorkspace(report); err != nil {
				if progress != nil {
					progress.append(command.Context(), "doctor_running", "failed", "workspace.managed_files", "WORKSPACE_DOCTOR_FAILED", "review.workspace.managed_files")
				}
				return withBootstrapDetails(err, map[string]any{"recovery": "fix the reported workspace check, then run bootstrap resume"})
			}
			if progress != nil {
				progress.append(command.Context(), "doctor_running", "passed", "workspace.managed_files", "", "")
				progress.append(command.Context(), "registering", "started", "", "", "")
			}
			handoff, handoffPath, err := localworkspace.StoreBootstrapHandoff(status.Root, adapter.Spec.PluginID, adapter.Spec.PluginVersion, adapter.Spec.MarketplaceRef, r.currentTime())
			if err != nil {
				return withBootstrapDetails(err, map[string]any{"recovery": "retry with bootstrap resume to regenerate the new-chat handoff"})
			}
			registered, err := registerBootstrapWorkspace(command.Context(), r.resolveServer(cfg), connected.WorkspaceToken, status)
			if err != nil {
				if progress != nil {
					progress.append(command.Context(), "registering", "failed", "workspace.registration", "WORKSPACE_REGISTRATION_FAILED", "retry.bootstrap.resume")
				}
				return withBootstrapDetails(err, map[string]any{"recovery": "network registration can be retried with bootstrap resume"})
			}
			if progress != nil {
				progress.append(command.Context(), "registering", "passed", "workspace.registration", "", "")
				progress.append(command.Context(), "opening_desktop", "started", "", "", "")
			}
			launch := codexplugin.LaunchResult{WorkspacePath: status.Root, RecoveryPrompt: codexplugin.RecoveryPrompt(adapter.Spec)}
			if openCodex {
				launch = adapter.LaunchNewChat(command.Context(), status.Root)
			}
			if progress != nil {
				if launch.Error != "" {
					progress.append(command.Context(), "opening_desktop", "needs_action", "desktop.new_chat", "CODEX_DESKTOP_OPEN_FAILED", "open.codex.recovery_prompt")
				} else {
					progress.append(command.Context(), "opening_desktop", "passed", "desktop.new_chat", "", "")
					progress.append(command.Context(), "complete", "passed", "", "", "")
					progress.complete(command.Context(), "completed")
				}
			}
			return r.writeOK("bootstrap.apply", map[string]any{
				"plugin":                 pluginResult,
				"workspace":              status,
				"environment":            environmentState,
				"doctor":                 report,
				"cloud_binding":          registered,
				"bootstrap_handoff":      handoff,
				"bootstrap_handoff_path": handoffPath,
				"new_chat":               launch,
				"daemon_enabled":         false,
				"uploaded_files":         0,
				"credential_store":       credentialProvider(),
				"authorization_mode":     "browser_device",
			})
		},
	}
	command.Flags().StringVar(&sessionID, "session", "", "public ConnectSession ID used with browser device authorization")
	command.Flags().StringVar(&planID, "plan-id", "", "exact plan_id returned by the user-confirmed bootstrap plan")
	command.Flags().StringVar(&name, "name", "", "workspace device display name")
	command.Flags().BoolVar(&accept, "accept", false, "confirm the exact plugin, user Codex state, project binding, and workspace changes from bootstrap plan")
	command.Flags().BoolVar(&openCodex, "open-codex", true, "open a new Codex project chat after verified registration")
	return command
}

func (r *Root) bootstrapResumeCommand() *cobra.Command {
	var accept, openCodex bool
	command := &cobra.Command{
		Use:   "resume [directory]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Resume doctor and registration after browser device authorization completed",
		RunE: func(command *cobra.Command, args []string) error {
			if !accept {
				err := domain.Policy("BOOTSTRAP_RESUME_CONFIRMATION_REQUIRED", "resume 将修复固定 Codex Plugin 状态并重新注册现有 Workspace", "检查当前 Workspace 后传入 --accept")
				err.ExitCode = 2
				return err
			}
			verifier, err := r.environmentManifestVerifier()
			if err != nil {
				return err
			}
			registryVerifier, err := r.environmentRegistryVerifier()
			if err != nil {
				return err
			}
			directory := optionalDirectory(args)
			cfg, err := localconfig.Load()
			if err != nil {
				return err
			}
			workspacePlan, err := localworkspace.Plan(directory, "codex-plugin")
			if err != nil {
				return err
			}
			var status localworkspace.Status
			needsInitialize := false
			switch workspacePlan.State {
			case "empty", "missing":
				if cfg.DeviceID == "" || cfg.WorkspaceID == "" || cfg.ProjectID == "" {
					return domain.Conflict("BOOTSTRAP_RESUME_BINDING_MISSING", "本机没有可恢复的 ContentCloud device/workspace/project 绑定")
				}
				server := r.resolveServer(cfg)
				if err := validateBootstrapServer(server); err != nil {
					return err
				}
				needsInitialize = true
			case "workspace":
				status, err = localworkspace.LoadStatus(workspacePlan.Root)
				if err != nil {
					return err
				}
				if !hasBootstrapTarget(status.Template.Targets) {
					return domain.Conflict("BOOTSTRAP_TARGET_MISMATCH", "当前 Workspace 不是 codex-plugin 交付模式")
				}
			default:
				return domain.Conflict("BOOTSTRAP_RESUME_DIRECTORY_CONFLICT", "恢复目标包含未知文件，拒绝覆盖")
			}
			adapter, err := r.codexPluginAdapter()
			if err != nil {
				return err
			}
			pluginPlan, err := adapter.Plan(command.Context())
			if err != nil {
				return err
			}
			pluginResult, err := adapter.Apply(command.Context(), pluginPlan, true)
			if err != nil {
				return withBootstrapDetails(err, map[string]any{"plugin": pluginResult})
			}
			if needsInitialize {
				status, err = initializeCodexPluginWorkspace(workspacePlan.Root, r.resolveServer(cfg), app.ConnectDeviceResult{
					Device: domain.Device{ID: cfg.DeviceID}, WorkspaceID: cfg.WorkspaceID, ProjectID: cfg.ProjectID,
				}, r.currentTime())
				if err != nil {
					return err
				}
			}
			workspaceToken, err := localconfig.WorkspaceToken(status.Binding.WorkspaceID)
			if err != nil {
				return domain.E("credential", "secure_store", "WORKSPACE_CREDENTIAL_UNAVAILABLE", err.Error(), 3)
			}
			manifest, err := fetchEnvironmentManifest(command.Context(), status.Binding.ServerURL, workspaceToken)
			if err != nil {
				return err
			}
			registry, err := fetchEnvironmentRegistry(command.Context(), status.Binding.ServerURL, workspaceToken)
			if err != nil {
				return err
			}
			environmentState, err := storeBootstrapEnvironment(status.Root, manifest, registry, adapter.Spec, verifier, registryVerifier, r.currentTime())
			if err != nil {
				return err
			}
			cfg.WorkspaceRoot = status.Root
			if err := localconfig.Save(cfg); err != nil {
				return err
			}
			report, err := localworkspace.DoctorWithEnvironment(status.Root, verifier, registryVerifier, r.currentTime())
			if err != nil {
				return err
			}
			if err := requireHealthyWorkspace(report); err != nil {
				return err
			}
			handoff, handoffPath, err := localworkspace.StoreBootstrapHandoff(status.Root, adapter.Spec.PluginID, adapter.Spec.PluginVersion, adapter.Spec.MarketplaceRef, r.currentTime())
			if err != nil {
				return err
			}
			registered, err := registerBootstrapWorkspace(command.Context(), status.Binding.ServerURL, workspaceToken, status)
			if err != nil {
				return err
			}
			launch := codexplugin.LaunchResult{WorkspacePath: status.Root, RecoveryPrompt: codexplugin.RecoveryPrompt(adapter.Spec)}
			if openCodex {
				launch = adapter.LaunchNewChat(command.Context(), status.Root)
			}
			return r.writeOK("bootstrap.resume", map[string]any{"plugin": pluginResult, "workspace": status, "environment": environmentState, "doctor": report, "cloud_binding": registered, "bootstrap_handoff": handoff, "bootstrap_handoff_path": handoffPath, "new_chat": launch})
		},
	}
	command.Flags().BoolVar(&accept, "accept", false, "confirm plugin repair and workspace registration")
	command.Flags().BoolVar(&openCodex, "open-codex", true, "open a new Codex project chat after verified registration")
	return command
}

func (r *Root) buildBootstrapPlan(ctx context.Context, directory string) (bootstrapPlan, *codexplugin.Adapter, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return bootstrapPlan{}, nil, err
	}
	server := r.resolveServer(cfg)
	if err := validateBootstrapServer(server); err != nil {
		return bootstrapPlan{}, nil, err
	}
	adapter, err := r.codexPluginAdapter()
	if err != nil {
		return bootstrapPlan{}, nil, err
	}
	pluginPlan, err := adapter.Plan(ctx)
	if err != nil {
		return bootstrapPlan{}, nil, err
	}
	workspacePlan, err := localworkspace.Plan(directory, "codex-plugin")
	if err != nil {
		return bootstrapPlan{}, nil, err
	}
	plan := bootstrapPlan{
		SchemaVersion:        bootstrapSchemaVersion,
		State:                "ready",
		Host:                 "codex",
		CLIPackage:           "@limecloud/contentcloud@" + Version,
		CLIVersion:           Version,
		ServerURL:            server,
		Plugin:               pluginPlan,
		Workspace:            workspacePlan,
		BlockingReasons:      []string{},
		RequiresConfirmation: true,
		AuthorizationMode:    "browser_device",
		WouldAuthorizeDevice: true,
		WouldRegister:        true,
		WouldUploadFiles:     false,
		WouldEnableDaemon:    false,
		WouldOpenNewChat:     true,
	}
	if pluginPlan.State == "blocked" {
		plan.State = "blocked"
		plan.BlockingReasons = append(plan.BlockingReasons, pluginPlan.BlockingReasons...)
	}
	switch workspacePlan.State {
	case "non_empty":
		plan.State = "blocked"
		plan.BlockingReasons = append(plan.BlockingReasons, "目标目录包含未知文件")
	case "workspace":
		plan.State = "resume_required"
		plan.BlockingReasons = append(plan.BlockingReasons, "目标目录已经是 ContentCloud Workspace")
	}
	planID, err := bootstrapPlanID(plan)
	if err != nil {
		return bootstrapPlan{}, nil, err
	}
	plan.PlanID = planID
	return plan, adapter, nil
}

func bootstrapPlanID(plan bootstrapPlan) (string, error) {
	plan.PlanID = ""
	body, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "bp_" + hex.EncodeToString(sum[:]), nil
}

func (r *Root) codexPluginAdapter() (*codexplugin.Adapter, error) {
	return codexplugin.New(codexplugin.DefaultSpec(Version), r.codexRunner)
}

func initializeCodexPluginWorkspace(root, server string, connected app.ConnectDeviceResult, now time.Time) (localworkspace.Status, error) {
	return localworkspace.Initialize(localworkspace.InitOptions{
		Root:        root,
		WorkspaceID: connected.WorkspaceID,
		ProjectID:   connected.ProjectID,
		DeviceID:    connected.Device.ID,
		ServerURL:   server,
		CLIVersion:  Version,
		Target:      "codex-plugin",
		Now:         now,
	})
}

func (r *Root) environmentManifestVerifier() (*environment.Verifier, error) {
	if r.manifestVerifierHook != nil {
		return r.manifestVerifierHook()
	}
	if path := strings.TrimSpace(os.Getenv("CONTENTCLOUD_ENVIRONMENT_TRUST_FILE")); path != "" {
		return environment.LoadManifestVerifier(path)
	}
	return environment.DefaultManifestVerifier()
}

func (r *Root) environmentRegistryVerifier() (*environment.RegistryVerifier, error) {
	if r.registryVerifierHook != nil {
		return r.registryVerifierHook()
	}
	if path := strings.TrimSpace(os.Getenv("CONTENTCLOUD_PLUGIN_TRUST_FILE")); path != "" {
		return environment.LoadRegistryVerifier(path)
	}
	return environment.DefaultRegistryVerifier()
}

func storeBootstrapEnvironment(root string, manifest environment.Manifest, registry environment.Registry, spec codexplugin.Spec, verifier *environment.Verifier, registryVerifier *environment.RegistryVerifier, now time.Time) (localworkspace.EnvironmentState, error) {
	if manifest.Distribution.Marketplace != spec.MarketplaceName {
		return localworkspace.EnvironmentState{}, domain.Conflict("BOOTSTRAP_ENVIRONMENT_MARKETPLACE_MISMATCH", "Environment Manifest Marketplace 与固定 Codex Plugin 规格不一致")
	}
	installed := make([]environment.LockedPlugin, 0, 1)
	for _, plugin := range manifest.Distribution.Plugins {
		if !plugin.Required || plugin.Scope != "environment" {
			continue
		}
		if plugin.ID != spec.PluginName || plugin.Kind != "scene_plugin" || plugin.Version != spec.PluginVersion || plugin.SourceRef != spec.MarketplaceRef {
			return localworkspace.EnvironmentState{}, domain.Conflict("BOOTSTRAP_ENVIRONMENT_PLUGIN_MISMATCH", "Environment Manifest 的必装 Plugin 与本次已验证 Codex 安装不一致")
		}
		installed = append(installed, environment.LockedPlugin{ID: plugin.ID, Kind: plugin.Kind, Version: plugin.Version, Digest: plugin.Digest, Installed: true})
	}
	if len(installed) != 1 {
		return localworkspace.EnvironmentState{}, domain.Conflict("BOOTSTRAP_ENVIRONMENT_PLUGIN_UNSUPPORTED", "首版 Bootstrap 只接受一个环境级必装 Scene Plugin")
	}
	verifiedRegistry, err := localworkspace.StoreEnvironmentRegistry(root, registry, registryVerifier)
	if err != nil {
		return localworkspace.EnvironmentState{}, err
	}
	if err := environment.ValidateManifestRegistry(manifest, verifiedRegistry, environment.PurposeNewInstall); err != nil {
		return localworkspace.EnvironmentState{}, err
	}
	return localworkspace.StoreEnvironment(root, manifest, installed, verifier, now)
}

func fetchEnvironmentManifest(ctx context.Context, server, token string) (environment.Manifest, error) {
	var manifest environment.Manifest
	if err := apiclient.New(server, token).Dispatch(ctx, "environment.manifest.get", map[string]any{}, &manifest); err != nil {
		return environment.Manifest{}, err
	}
	return manifest, nil
}

func fetchEnvironmentRegistry(ctx context.Context, server, token string) (environment.Registry, error) {
	var registry environment.Registry
	if err := apiclient.New(server, token).Dispatch(ctx, "environment.registry.get", map[string]any{}, &registry); err != nil {
		return environment.Registry{}, err
	}
	return registry, nil
}

func registerBootstrapWorkspace(ctx context.Context, server, token string, status localworkspace.Status) (domain.WorkspaceBinding, error) {
	var registered domain.WorkspaceBinding
	err := apiclient.New(server, token).Dispatch(ctx, "workspace.register", map[string]any{
		"template_id":      status.Template.TemplateID,
		"template_version": status.Template.TemplateVersion,
		"targets":          status.Template.Targets,
	}, &registered)
	return registered, err
}

func requireHealthyWorkspace(report localworkspace.DoctorReport) error {
	if report.OK {
		return nil
	}
	err := domain.Conflict("WORKSPACE_DOCTOR_FAILED", "Workspace 必需检查未全部通过，已阻止云端注册")
	err.Details = report
	err.Hint = "修复 doctor 中失败的 required checks 后重试"
	return err
}

func validateBootstrapServer(value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return domain.Invalid("SERVER_URL_INVALID", "bootstrap server URL 必须是无内嵌凭据的 HTTP(S) origin")
	}
	return nil
}

func withBootstrapDetails(err error, details map[string]any) error {
	if domainError, ok := err.(*domain.Error); ok {
		copy := *domainError
		if domainError.Details != nil {
			details["cause"] = domainError.Details
		}
		copy.Details = details
		return &copy
	}
	wrapped := domain.E("runtime", "bootstrap", "BOOTSTRAP_FAILED", err.Error(), 5)
	wrapped.Details = details
	return wrapped
}

func hasBootstrapTarget(targets []string) bool {
	for _, target := range targets {
		if target == "codex-plugin" {
			return true
		}
	}
	return false
}
