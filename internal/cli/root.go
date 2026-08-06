package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/contracts"
	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/automationworkspace"
	"github.com/limecloud/contentcloud/internal/bootstrapcheck"
	"github.com/limecloud/contentcloud/internal/capabilitycatalog"
	"github.com/limecloud/contentcloud/internal/codexplugin"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	builtinskills "github.com/limecloud/contentcloud/plugins/contentcloud-video-production/skills"
)

const Version = "0.17.0"

type Root struct {
	json                   bool
	serverURL              string
	projectID              string
	stdout                 io.Writer
	stderr                 io.Writer
	mcpCWD                 string
	now                    func() time.Time
	codexRunner            codexplugin.CommandRunner
	bootstrapCheckHook     func(context.Context, bootstrapcheck.Options) bootstrapcheck.Report
	bootstrapAuthorizeHook func(context.Context, string, string) (localconfig.Config, app.ConnectDeviceResult, *bootstrapProgressReporter, error)
	manifestVerifierHook   func() (*environment.Verifier, error)
	registryVerifierHook   func() (*environment.RegistryVerifier, error)
	daemonFactory          func() (userDaemonService, error)
}
type success struct {
	OK        bool           `json:"ok"`
	Command   string         `json:"command"`
	RequestID string         `json:"request_id"`
	Data      any            `json:"data"`
	Meta      map[string]any `json:"meta"`
}
type failure struct {
	OK        bool          `json:"ok"`
	Command   string        `json:"command"`
	RequestID string        `json:"request_id"`
	Error     *domain.Error `json:"error"`
}

func Execute() int {
	root := &Root{stdout: os.Stdout, stderr: os.Stderr}
	cmd := root.command()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		return root.writeError(cmd.CommandPath(), err)
	}
	return 0
}

func (r *Root) command() *cobra.Command {
	cmd := &cobra.Command{Use: "contentcloud", Short: "Content Work OS 本地创作运行工具", Long: "管理 Content Work OS 项目并运行本地创作能力，无需暴露服务端私有接口。", SilenceErrors: true, SilenceUsage: true}
	cmd.SetOut(r.stdout)
	cmd.SetErr(r.stderr)
	cmd.PersistentFlags().BoolVar(&r.json, "json", false, "在标准输出中返回稳定的 JSON 响应结构")
	cmd.PersistentFlags().StringVar(&r.serverURL, "server-url", "", "Content Work OS 服务地址")
	cmd.PersistentFlags().StringVar(&r.projectID, "project", "", "明确指定项目 ID")
	cmd.AddCommand(r.authCommand(), r.doctor(), r.bootstrapCommand(), r.workspaceCommand(), r.localCommand(), r.mcpCommand(), r.publishCommand(), r.pullCommand(), r.submissionCommand(), r.down(), r.updateCommand(), r.status(), r.contextCommand(), r.skillsCommand(), r.daemonCommand(), r.schemaCommand(), r.tenantCommand(), r.teamCommand(), r.fullProjectCommand(), r.deviceCommand(), r.sourceCommand(), r.assetCommand(), r.knowledgeCommand(), r.runCommand(), r.artifactCommand(), r.deliveryCommand(), r.reviewCommand(), r.resultCommand(), r.lineageCommand(), r.auditCommand(), r.requestCommand())
	cmd.Version = Version
	return cmd
}

func (r *Root) doctor() *cobra.Command {
	var offline bool
	cmd := &cobra.Command{Use: "doctor", Short: "检查安装、配置、凭据存储、服务连接和本地能力", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		server := r.resolveServer(cfg)
		bootstrapReport := bootstrapcheck.Run(cmd.Context(), bootstrapcheck.Options{Directory: ".", ServerURL: server, Offline: offline})
		if r.bootstrapCheckHook != nil {
			bootstrapReport = r.bootstrapCheckHook(cmd.Context(), bootstrapcheck.Options{Directory: ".", ServerURL: server, Offline: offline})
		}
		checks := map[string]any{"version": map[string]any{"ok": true, "value": Version}, "config": map[string]any{"ok": err == nil, "path": mustConfigPath()}, "credential_store": map[string]any{"ok": runtime.GOOS == "darwin", "provider": credentialProvider()}, "temp_directory": map[string]any{"ok": writableTemp()}, "capabilities": detectCapabilities(), "bootstrap": bootstrapReport}
		if offline {
			checks["server"] = map[string]any{"ok": true, "skipped": true}
		} else {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			healthErr := apiclient.New(server, "").Health(ctx)
			checks["server"] = map[string]any{"ok": healthErr == nil, "url": server, "error": errorString(healthErr)}
		}
		return r.writeOK("doctor", map[string]any{"checks": checks, "offline": offline})
	}}
	cmd.Flags().BoolVar(&offline, "offline", false, "跳过服务端连通性检查")
	return cmd
}

func (r *Root) down() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{Use: "down", Short: "撤销当前创作运行环境并清除本机设备绑定", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		if cfg.DeviceID == "" {
			return r.writeOK("down", map[string]any{"disconnected": true, "already_down": true})
		}
		if dryRun {
			return r.writeOK("down", map[string]any{"dry_run": true, "device_id": cfg.DeviceID, "would_revoke": true, "would_clear_local_binding": true})
		}
		if !yes {
			return confirmationRequired("断开会撤销当前设备并清除本机设备凭据")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		if err := client.Dispatch(cmd.Context(), "device.revoke", map[string]any{"device_id": cfg.DeviceID}, nil); err != nil {
			return err
		}
		if err := localconfig.DeleteDeviceToken(cfg.DeviceID); err != nil {
			return err
		}
		if err := uninstallUserDaemon(); err != nil {
			return err
		}
		cfg.DeviceID, cfg.ProjectID = "", ""
		if err := localconfig.Save(cfg); err != nil {
			return err
		}
		return r.writeOK("down", map[string]any{"disconnected": true})
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "确认执行此高风险写入操作")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只验证，不修改服务端或本地状态")
	return cmd
}

func (r *Root) updateCommand() *cobra.Command {
	return &cobra.Command{Use: "update", Short: "显示用于更新当前程序的已验证安装命令", RunE: func(cmd *cobra.Command, args []string) error {
		return r.writeOK("update", map[string]any{"current_version": Version, "installer": "npx --yes @limecloud/contentcloud@latest update", "automatic_update": false, "installer_owned": true, "daemon_restart_after_update": true, "reason": "the verified npm installer owns checksum validation, binary replacement, and restart of an installed daemon"})
	}}
}

func (r *Root) status() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "显示本地运行环境和项目连接状态", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		credential := "missing"
		if _, err := localconfig.DeviceToken(cfg.DeviceID); err == nil {
			credential = "available"
		}
		var daemon any = map[string]any{"supported": runtime.GOOS == "darwin", "installed": false, "running": false}
		if service, serviceErr := r.localDaemonService(); serviceErr == nil {
			if state, statusErr := service.Status(); statusErr == nil {
				daemon = state
			}
		}
		pending, dead, _ := daemonJournalCounts()
		return r.writeOK("status", map[string]any{"server_url": cfg.ServerURL, "device_id": cfg.DeviceID, "project_id": cfg.ProjectID, "device_credential": credential, "version": Version, "daemon": daemon, "daemon_bindings": cfg.RuntimeBindings(), "pending_attempt_reports": pending, "dead_letters": dead})
	}}
}

func (r *Root) contextCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "管理明确指定的项目上下文"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "显示当前解析到的项目上下文", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		id, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "未解析到唯一项目上下文")
		}
		return r.writeOK("context.show", map[string]any{"project_id": id})
	}}, &cobra.Command{Use: "use <project-id>", Args: cobra.ExactArgs(1), Short: "在当前目录写入项目上下文", RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(".contentcloud")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]string{"project_id": args[0]}, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "project.json"), b, 0600); err != nil {
			return err
		}
		return r.writeOK("context.use", map[string]any{"project_id": args[0], "path": filepath.Join(dir, "project.json")})
	}}, &cobra.Command{Use: "clear", Short: "移除当前目录中的项目上下文", RunE: func(cmd *cobra.Command, args []string) error {
		path := filepath.Join(".contentcloud", "project.json")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return r.writeOK("context.clear", map[string]any{"cleared": true, "path": path})
	}})
	return cmd
}

func (r *Root) skillsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "查看并安装与命令行版本匹配的 Content Work OS 本地技能"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "列出内置技能", RunE: func(cmd *cobra.Command, args []string) error {
		items := []map[string]any{}
		for _, name := range builtinskills.Names() {
			files, _ := builtinskills.Files(name)
			body, _ := builtinskills.Read(name, "SKILL.md")
			sum := sha256.Sum256(body)
			items = append(items, map[string]any{"name": name, "version": Version, "digest": hex.EncodeToString(sum[:]), "files": len(files)})
		}
		return r.writeOK("skills.list", items)
	}})
	var path string
	read := &cobra.Command{Use: "read <name>", Args: cobra.ExactArgs(1), Short: "读取内置技能说明或参考资料", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := builtinskills.Read(args[0], path)
		if err != nil {
			return domain.NotFound("技能")
		}
		if r.json {
			return r.writeOK("skills.read", map[string]any{"name": args[0], "path": defaultValue(path, "SKILL.md"), "content": string(body)})
		}
		_, err = fmt.Fprint(r.stdout, string(body))
		return err
	}}
	read.Flags().StringVar(&path, "path", "SKILL.md", "技能目录内的相对文件路径")
	cmd.AddCommand(read)
	var target string
	install := &cobra.Command{Use: "install <name>", Args: cobra.ExactArgs(1), Short: "把内置技能安装到本地智能体技能目录", RunE: func(cmd *cobra.Command, args []string) error {
		dest, err := skillDestination(target, args[0])
		if err != nil {
			return err
		}
		files, err := builtinskills.Files(args[0])
		if err != nil {
			return err
		}
		for _, file := range files {
			body, err := builtinskills.Read(args[0], file)
			if err != nil {
				return err
			}
			path := filepath.Join(dest, file)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(path, body, 0600); err != nil {
				return err
			}
		}
		return r.writeOK("skills.install", map[string]any{"name": args[0], "target": target, "path": dest, "version": Version})
	}}
	install.Flags().StringVar(&target, "target", "codex", "目标智能体：codex 或 claude")
	cmd.AddCommand(install, &cobra.Command{Use: "status", Short: "检查内置技能是否可用", RunE: func(cmd *cobra.Command, args []string) error {
		return r.writeOK("skills.status", map[string]any{"binary_version": Version, "embedded": builtinskills.Names(), "in_sync": true})
	}})
	return cmd
}

func (r *Root) daemonCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon", Short: "运行仅主动连接服务端的本地创作后台服务"}
	start := &cobra.Command{Use: "start", Short: "安装并启动当前用户的自动化后台服务", RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemonStartPrerequisites(); err != nil {
			return err
		}
		service, err := r.localDaemonService()
		if err != nil {
			return err
		}
		state, err := service.Start()
		if err != nil {
			return err
		}
		return r.writeOK("daemon.start", state)
	}}
	stop := &cobra.Command{Use: "stop", Short: "停止当前用户的自动化后台服务，但不卸载", RunE: func(cmd *cobra.Command, args []string) error {
		service, err := r.localDaemonService()
		if err != nil {
			return err
		}
		state, err := service.Stop()
		if err != nil {
			return err
		}
		return r.writeOK("daemon.stop", state)
	}}
	status := &cobra.Command{Use: "status", Short: "显示后台服务的安装、进程、日志和版本状态", RunE: func(cmd *cobra.Command, args []string) error {
		service, err := r.localDaemonService()
		if err != nil {
			return err
		}
		state, err := service.Status()
		if err != nil {
			return err
		}
		return r.writeOK("daemon.status", state)
	}}
	var ifInstalled bool
	restart := &cobra.Command{Use: "restart", Short: "使用当前 Content Work OS 程序重新加载后台服务", RunE: func(cmd *cobra.Command, args []string) error {
		service, err := r.localDaemonService()
		if err != nil {
			return err
		}
		current, err := service.Status()
		if err != nil {
			return err
		}
		if ifInstalled && !current.Installed {
			return r.writeOK("daemon.restart", map[string]any{"restarted": false, "skipped": true, "reason": "not_installed", "daemon": current})
		}
		if err := daemonStartPrerequisites(); err != nil {
			return err
		}
		state, err := service.Restart()
		if err != nil {
			return err
		}
		return r.writeOK("daemon.restart", state)
	}}
	restart.Flags().BoolVar(&ifInstalled, "if-installed", false, "后台服务未安装时直接跳过并返回成功")
	var once, fixture bool
	var adapterKind string
	var logFile string
	run := &cobra.Command{Use: "run", Short: "轮询已分配任务并执行本地能力", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(logFile) != "" {
			managedLog, logErr := newRotatingLogWriter(logFile)
			if logErr != nil {
				return logErr
			}
			defer managedLog.Close()
			r.stdout, r.stderr = managedLog, managedLog
		}
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		bindings := cfg.RuntimeBindings()
		if len(bindings) == 0 && cfg.DeviceID != "" {
			bindings = []localconfig.DaemonBinding{{ServerURL: r.resolveServer(cfg), DeviceID: cfg.DeviceID, Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: cfg.WorkspaceID, ProjectID: cfg.ProjectID, Root: cfg.WorkspaceRoot}}}}
		}
		if len(bindings) == 0 {
			return domain.Conflict("DEVICE_BINDING_MISSING", "启动自动化后台服务前必须先完成设备注册")
		}
		journal, err := newDaemonJournal()
		if err != nil {
			return err
		}
		type bindingRuntime struct {
			config           localconfig.Config
			binding          localconfig.DaemonBinding
			client           *apiclient.Client
			interactiveRoots []string
		}
		runtimes := make([]bindingRuntime, 0, len(bindings))
		for _, binding := range bindings {
			bindingConfig := cfg
			bindingConfig.ServerURL, bindingConfig.DeviceID = binding.ServerURL, binding.DeviceID
			bindingConfig.DaemonBindings = []localconfig.DaemonBinding{binding}
			bindingConfig.WorkspaceID, bindingConfig.ProjectID, bindingConfig.WorkspaceRoot = "", "", ""
			for _, workspace := range binding.Workspaces {
				if bindingConfig.WorkspaceID == "" {
					bindingConfig.WorkspaceID, bindingConfig.ProjectID = workspace.WorkspaceID, workspace.ProjectID
				}
				if bindingConfig.WorkspaceRoot == "" && strings.TrimSpace(workspace.Root) != "" {
					bindingConfig.WorkspaceRoot = workspace.Root
				}
			}
			token, tokenErr := localconfig.DeviceToken(binding.DeviceID)
			if tokenErr != nil {
				return &domain.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: tokenErr.Error(), ExitCode: 3}
			}
			runtimes = append(runtimes, bindingRuntime{
				config: bindingConfig, binding: binding, client: apiclient.New(r.resolveServer(bindingConfig), token),
				interactiveRoots: configuredWorkspaceRoots(bindingConfig),
			})
		}
		for _, runtime := range runtimes {
			if flushErr := journal.flushMatching(cmd.Context(), runtime.client, r.resolveServer(runtime.config), runtime.binding.DeviceID); flushErr != nil {
				if once {
					return flushErr
				}
				fmt.Fprintln(r.stderr, flushErr)
			}
		}
		var adapter agentadapter.Adapter
		if !fixture {
			adapter, err = agentadapter.Select(adapterKind)
			if err != nil {
				return err
			}
			if err := adapter.Detect(); err != nil {
				return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "指定的本地智能体不可用", "检查安装与登录状态")
			}
		}
		provider := "fixture"
		if adapter != nil {
			provider = adapter.Kind()
		}
		maxConcurrent := daemonMaxConcurrentTasks()
		tracker, err := newDaemonRuntimeTracker(bindings, provider, maxConcurrent, r.currentTime())
		if err != nil {
			return err
		}
		execute := func(runtime bindingRuntime) (returnErr error) {
			cfg, client := runtime.config, runtime.client
			capabilities := builtinCapabilities()
			claims, err := daemonEnvironmentClaims(cfg)
			if err != nil {
				return err
			}
			var poll struct {
				Leased      bool                    `json:"leased"`
				Lease       *app.Lease              `json:"lease,omitempty"`
				Runtime     app.DaemonRuntimePolicy `json:"runtime"`
				PollAfterMS int                     `json:"poll_after_ms"`
				// Legacy direct Lease fields remain accepted during rolling upgrades.
				Run             domain.TaskRun                       `json:"run"`
				Attempt         domain.RunAttempt                    `json:"attempt"`
				Contract        domain.TaskContract                  `json:"contract"`
				ExecutionBundle *environment.CreativeExecutionBundle `json:"execution_bundle,omitempty"`
				LeaseExpiresAt  time.Time                            `json:"lease_expires_at"`
				RunToken        string                               `json:"run_token"`
			}
			waitMS := 0
			if !once {
				waitMS = 20000
			}
			err = client.Dispatch(cmd.Context(), "daemon.poll", map[string]any{"capabilities": capabilities, "environments": claims, "daemon_version": Version, "wait_ms": waitMS}, &poll)
			if err != nil {
				tracker.recordPoll(app.DaemonRuntimePolicy{CurrentVersion: Version}, false, err)
				var de *domain.Error
				if errors.As(err, &de) && de.Code == "NO_TASK" {
					if once {
						return r.writeOK("daemon.poll", map[string]any{"leased": false})
					}
					return nil
				}
				return err
			}
			tracker.recordPoll(poll.Runtime, poll.Leased || poll.Lease != nil || poll.Run.ID != "", nil)
			if !poll.Leased && poll.Lease == nil && poll.Run.ID == "" {
				if once {
					return r.writeOK("daemon.poll", map[string]any{"leased": false, "runtime": poll.Runtime, "poll_after_ms": poll.PollAfterMS})
				}
				return nil
			}
			var lease app.Lease
			if poll.Lease != nil {
				lease = *poll.Lease
			} else {
				lease = app.Lease{Run: poll.Run, Attempt: poll.Attempt, Contract: poll.Contract, ExecutionBundle: poll.ExecutionBundle, LeaseExpiresAt: poll.LeaseExpiresAt, RunToken: poll.RunToken}
			}
			if err := journal.begin(lease, r.resolveServer(cfg), cfg.DeviceID); err != nil {
				return err
			}
			tracker.taskStarted()
			defer func() { tracker.taskFinished(returnErr) }()
			schema, skillName, resourceErr := taskRuntimeResources(lease.Run)
			if resourceErr != nil {
				return finishAttemptError(journal, client, lease, "runtime_resources", "本地任务资源选择失败", resourceErr)
			}
			skillBody, resourceErr := builtinskills.Read(skillName, "SKILL.md")
			if resourceErr != nil {
				return finishAttemptError(journal, client, lease, "skill_load", "本地 Skill 加载失败", resourceErr)
			}
			executionWorkspace, workspaceErr := automationworkspace.Begin(automationworkspace.Options{
				BaseDir: strings.TrimSpace(os.Getenv("CONTENTCLOUD_AUTOMATION_ROOT")), ForbiddenRoots: runtime.interactiveRoots,
				AttemptID: lease.Attempt.ID, RunID: lease.Run.ID, ProjectID: lease.Run.ProjectID,
				Contract: lease.Contract, Bundle: lease.ExecutionBundle, OutputSchema: schema, Skill: skillBody,
				Now: r.currentTime(), ExpiresAt: lease.LeaseExpiresAt,
			})
			if workspaceErr != nil {
				return finishAttemptError(journal, client, lease, "workspace_isolation", "Automation 隔离工作区创建失败", workspaceErr)
			}
			defer func() {
				if cleanupErr := executionWorkspace.Cleanup(); cleanupErr != nil {
					returnErr = errors.Join(returnErr, cleanupErr)
				}
			}()
			var output json.RawMessage
			if fixture {
				switch lease.Run.TaskType {
				case "knowledge_extract":
					output, _ = json.Marshal(GenerateFixtureKnowledge(lease.Contract, lease.Run.OutputCount))
				default:
					runErr := domain.Invalid("TASK_TYPE_UNSUPPORTED", "测试适配器不支持该任务类型")
					return finishAttemptError(journal, client, lease, "runtime_resources", "本地开发 Fixture 不支持该任务类型", runErr)
				}
			} else {
				var heartbeatResult struct {
					CancelRequested bool           `json:"cancel_requested"`
					Run             domain.TaskRun `json:"run"`
				}
				if err := client.Dispatch(cmd.Context(), "run.heartbeat", map[string]any{"run_id": lease.Run.ID, "attempt_id": lease.Attempt.ID, "run_token": lease.RunToken, "heartbeat": domain.RunHeartbeat{Sequence: 1, Phase: "contract_ready", Step: 1, Label: "上下文校验完成"}}, &heartbeatResult); err != nil {
					return finishAttemptError(journal, client, lease, "heartbeat_failed", "首次心跳未完成", err)
				}
				if heartbeatResult.Run.LeaseExpiresAt == nil {
					leaseErr := domain.Conflict("AUTOMATION_WORKSPACE_LEASE_EXPIRY_MISSING", "服务端心跳未返回续租时间")
					return finishAttemptError(journal, client, lease, "workspace_isolation", "本地 Automation lease 无法续租", leaseErr)
				}
				if err := executionWorkspace.Renew(*heartbeatResult.Run.LeaseExpiresAt); err != nil {
					return finishAttemptError(journal, client, lease, "workspace_isolation", "本地 Automation lease 续租失败", err)
				}
				if heartbeatResult.CancelRequested {
					cancelErr := domain.Conflict("RUN_CANCEL_REQUESTED", "任务已被用户取消")
					if err := finishAttempt(journal, client, lease, "canceled", "user_canceled", "服务端取消请求已由本地客户端确认", nil); err != nil {
						return errors.Join(cancelErr, err)
					}
					return cancelErr
				}
				agentCtx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
				heartbeatDone := make(chan struct{})
				heartbeatErrors := make(chan error, 1)
				go func() {
					defer close(heartbeatDone)
					ticker := time.NewTicker(25 * time.Second)
					defer ticker.Stop()
					sequence := 2
					for {
						select {
						case <-agentCtx.Done():
							return
						case <-ticker.C:
							var response struct {
								CancelRequested bool           `json:"cancel_requested"`
								Run             domain.TaskRun `json:"run"`
							}
							err := client.Dispatch(agentCtx, "run.heartbeat", map[string]any{"run_id": lease.Run.ID, "attempt_id": lease.Attempt.ID, "run_token": lease.RunToken, "heartbeat": domain.RunHeartbeat{Sequence: sequence, Phase: "client_executing", Step: sequence, Label: "本地 Agent 生成中"}}, &response)
							if err != nil {
								heartbeatErrors <- err
								cancel()
								return
							}
							if response.CancelRequested {
								heartbeatErrors <- domain.Conflict("RUN_CANCEL_REQUESTED", "任务已被用户取消")
								cancel()
								return
							}
							if response.Run.LeaseExpiresAt == nil {
								heartbeatErrors <- domain.Conflict("AUTOMATION_WORKSPACE_LEASE_EXPIRY_MISSING", "服务端心跳未返回续租时间")
								cancel()
								return
							}
							if err := executionWorkspace.Renew(*response.Run.LeaseExpiresAt); err != nil {
								heartbeatErrors <- err
								cancel()
								return
							}
							sequence++
						}
					}
				}()
				output, err = adapter.Run(agentCtx, executionWorkspace.Root)
				cancel()
				<-heartbeatDone
				select {
				case heartbeatErr := <-heartbeatErrors:
					var de *domain.Error
					if errors.As(heartbeatErr, &de) && de.Code == "RUN_CANCEL_REQUESTED" {
						if finishErr := finishAttempt(journal, client, lease, "canceled", "user_canceled", "服务端取消请求已由本地客户端确认", nil); finishErr != nil {
							return errors.Join(heartbeatErr, finishErr)
						}
						return heartbeatErr
					}
					failureClass := "heartbeat_failed"
					summary := "后台心跳中断"
					if errors.As(heartbeatErr, &de) && strings.HasPrefix(de.Code, "AUTOMATION_WORKSPACE_") {
						failureClass = "workspace_isolation"
						summary = "本地 Automation lease 续租失败"
					}
					return finishAttemptError(journal, client, lease, failureClass, summary, heartbeatErr)
				default:
				}
				if err != nil {
					failureClass, summary, exitCode := classifyAttemptFailure(err)
					return finishAttemptErrorWithExitCode(journal, client, lease, failureClass, summary, exitCode, err)
				}
			}
			if err := journal.queueReport(lease, output); err != nil {
				return err
			}
			if err := journal.deliverAttempt(cmd.Context(), client, lease.Attempt.ID); err != nil {
				return err
			}
			report := map[string]any{"delivered": true}
			return r.writeOK("daemon.run", map[string]any{"leased": true, "run_id": lease.Run.ID, "attempt_id": lease.Attempt.ID, "task_type": lease.Run.TaskType, "isolated_workspace": true, "result": report})
		}
		if once {
			return execute(runtimes[0])
		}
		results := make(chan error, maxConcurrent)
		active, nextRuntime := 0, 0
		for {
			for active < maxConcurrent {
				runtime := runtimes[nextRuntime]
				nextRuntime = (nextRuntime + 1) % len(runtimes)
				active++
				go func() { results <- execute(runtime) }()
			}
			select {
			case <-cmd.Context().Done():
				for active > 0 {
					<-results
					active--
				}
				return nil
			case runErr := <-results:
				active--
				if runErr != nil {
					fmt.Fprintln(r.stderr, runErr)
				}
			}
		}
	}}
	run.Flags().BoolVar(&once, "once", false, "最多轮询一次")
	run.Flags().BoolVar(&fixture, "fixture", false, "开发时使用结果固定的本地测试适配器")
	run.Flags().StringVar(&adapterKind, "adapter", "auto", "本地智能体适配器：auto、codex 或 claude-code；其他已登记客户端仍在规划中")
	run.Flags().StringVar(&logFile, "log-file", "", "受管后台服务日志路径")
	cmd.AddCommand(start, stop, status, restart, run)
	return cmd
}

func daemonStartPrerequisites() error {
	cfg, err := localconfig.Load()
	if err != nil {
		return err
	}
	if len(cfg.RuntimeBindings()) == 0 && cfg.DeviceID == "" {
		return domain.Conflict("DEVICE_BINDING_MISSING", "启动自动化后台服务前必须先完成设备注册")
	}
	for _, binding := range cfg.RuntimeBindings() {
		if _, err := localconfig.DeviceToken(binding.DeviceID); err != nil {
			return &domain.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: err.Error(), ExitCode: 3}
		}
	}
	adapter, err := agentadapter.Select("auto")
	if err != nil {
		return err
	}
	if err := adapter.Detect(); err != nil {
		return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "未检测到可用于自动化任务的 Codex 或 Claude Code", "安装并登录本机智能体客户端后重试")
	}
	return nil
}

func daemonEnvironmentClaims(config localconfig.Config) ([]app.AutomationEnvironmentClaim, error) {
	roots := []string{}
	if root := strings.TrimSpace(os.Getenv("CONTENTCLOUD_WORKSPACE_ROOT")); root != "" {
		roots = append(roots, root)
	} else {
		for _, binding := range config.RuntimeBindings() {
			for _, workspace := range binding.Workspaces {
				if root := strings.TrimSpace(workspace.Root); root != "" {
					roots = append(roots, root)
				}
			}
		}
		if len(roots) == 0 && strings.TrimSpace(config.WorkspaceRoot) != "" {
			roots = append(roots, strings.TrimSpace(config.WorkspaceRoot))
		}
	}
	if len(roots) == 0 {
		return []app.AutomationEnvironmentClaim{}, nil
	}
	claims := make([]app.AutomationEnvironmentClaim, 0, len(roots))
	projects := map[string]bool{}
	for _, root := range roots {
		state, err := localworkspace.ReadEnvironmentClaim(root)
		if err != nil {
			wrapped := domain.Conflict("AUTOMATION_ENVIRONMENT_CLAIM_UNAVAILABLE", "无法读取完整的本地环境清单和锁文件")
			wrapped.Hint = "完成创作环境检查后重试后台服务轮询"
			wrapped.Details = map[string]any{"workspace_root": root, "cause": err.Error()}
			return nil, wrapped
		}
		if projects[state.Manifest.ProjectID] {
			continue
		}
		projects[state.Manifest.ProjectID] = true
		claims = append(claims, app.AutomationEnvironmentClaim{Manifest: state.Manifest, Lock: state.Lock})
	}
	return claims, nil
}

func configuredWorkspaceRoots(config localconfig.Config) []string {
	if root := strings.TrimSpace(os.Getenv("CONTENTCLOUD_WORKSPACE_ROOT")); root != "" {
		return []string{root}
	}
	roots := []string{}
	seen := map[string]bool{}
	for _, binding := range config.RuntimeBindings() {
		for _, workspace := range binding.Workspaces {
			root := strings.TrimSpace(workspace.Root)
			if root != "" && !seen[root] {
				seen[root] = true
				roots = append(roots, root)
			}
		}
	}
	if root := strings.TrimSpace(config.WorkspaceRoot); root != "" && !seen[root] {
		roots = append(roots, root)
	}
	return roots
}

func daemonMaxConcurrentTasks() int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CONTENTCLOUD_DAEMON_MAX_CONCURRENT_TASKS")))
	if err != nil || value < 1 {
		return 2
	}
	if value > 8 {
		return 8
	}
	return value
}

func finishAttempt(journal *daemonJournal, client *apiclient.Client, lease app.Lease, outcome, failureClass, summary string, exitCode *int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := journal.queueFinish(lease, outcome, failureClass, summary, exitCode); err != nil {
		return err
	}
	return journal.deliverAttempt(ctx, client, lease.Attempt.ID)
}

func finishAttemptError(journal *daemonJournal, client *apiclient.Client, lease app.Lease, failureClass, summary string, runErr error) error {
	return finishAttemptErrorWithExitCode(journal, client, lease, failureClass, summary, nil, runErr)
}

func finishAttemptErrorWithExitCode(journal *daemonJournal, client *apiclient.Client, lease app.Lease, failureClass, summary string, exitCode *int, runErr error) error {
	if finishErr := finishAttempt(journal, client, lease, "failed", failureClass, summary, exitCode); finishErr != nil {
		return errors.Join(runErr, finishErr)
	}
	return runErr
}

func classifyAttemptFailure(err error) (string, string, *int) {
	var de *domain.Error
	if !errors.As(err, &de) {
		return "agent_runtime", "本地 Agent 执行失败", nil
	}
	var exitCode *int
	if details, ok := de.Details.(map[string]any); ok {
		switch value := details["process_exit_code"].(type) {
		case int:
			exitCode = &value
		case float64:
			code := int(value)
			exitCode = &code
		}
	}
	switch de.Code {
	case "AGENT_PROCESS_FAILED":
		return "agent_process", "本地 Agent 进程执行失败", exitCode
	case "AGENT_CANCELED":
		return "agent_timeout", "本地 Agent 执行被中断或超时", exitCode
	case "AGENT_OUTPUT_INVALID", "AGENT_OUTPUT_MISSING":
		return "agent_output", "本地 Agent 未生成有效结构化输出", exitCode
	default:
		return "agent_runtime", "本地 Agent 执行失败", exitCode
	}
}

func (r *Root) schemaCommand() *cobra.Command {
	return &cobra.Command{Use: "schema [command]", Args: cobra.MaximumNArgs(1), Short: "显示稳定的命令契约和风险等级", RunE: func(cmd *cobra.Command, args []string) error {
		schemas := commandSchemas()
		if len(args) == 1 {
			value, ok := schemas[args[0]]
			if !ok {
				return domain.NotFound("命令结构定义")
			}
			return r.writeOK("schema", value)
		}
		return r.writeOK("schema", schemas)
	}}
}
func (r *Root) projectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "发现并查看项目"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "显示当前配置的项目身份", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		id, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "需要显式项目上下文")
		}
		return r.writeOK("project.show", map[string]any{"project_id": id, "server_url": r.resolveServer(cfg), "note": "业务读取需要用户 CLI 会话；设备凭据仅用于 Daemon"})
	}})
	return cmd
}

func (r *Root) resolveServer(cfg localconfig.Config) string {
	if r.serverURL != "" {
		return strings.TrimRight(r.serverURL, "/")
	}
	if env := os.Getenv("CONTENTCLOUD_SERVER_URL"); env != "" {
		return strings.TrimRight(env, "/")
	}
	if cfg.ServerURL != "" {
		return strings.TrimRight(cfg.ServerURL, "/")
	}
	return "http://localhost:8080"
}
func (r *Root) writeOK(command string, data any) error {
	if r.json {
		return json.NewEncoder(r.stdout).Encode(success{OK: true, Command: command, RequestID: "local_" + domain.NewID(), Data: data, Meta: map[string]any{}})
	}
	switch v := data.(type) {
	case string:
		_, _ = fmt.Fprintln(r.stdout, v)
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		_, _ = fmt.Fprintln(r.stdout, string(b))
	}
	return nil
}
func (r *Root) writeError(command string, err error) int {
	de := &domain.Error{}
	if !errors.As(err, &de) {
		de = domain.E("internal", "local", "INTERNAL_ERROR", err.Error(), 1)
	}
	if r.json {
		_ = json.NewEncoder(r.stderr).Encode(failure{OK: false, Command: command, RequestID: "local_" + domain.NewID(), Error: de})
	} else {
		fmt.Fprintf(r.stderr, "错误 [%s]: %s\n", de.Code, de.Message)
		if de.Hint != "" {
			fmt.Fprintf(r.stderr, "提示: %s\n", de.Hint)
		}
	}
	if de.ExitCode == 0 {
		return 1
	}
	return de.ExitCode
}

func builtinCapabilities() []domain.Capability {
	return capabilitycatalog.Builtins(Version)
}
func detectCapabilities() map[string]any {
	return map[string]any{"knowledge.extract": map[string]any{"ok": true, "version": "1.0.0"}, "codex": binaryStatus("codex"), "claude": binaryStatus("claude")}
}

func taskRuntimeResources(run domain.TaskRun) ([]byte, string, error) {
	switch run.TaskType {
	case "knowledge_extract":
		if run.OutputSchema != domain.KnowledgeCandidatesSchema {
			return nil, "", domain.Conflict("OUTPUT_SCHEMA_MISMATCH", "知识提取任务的输出结构定义与本机能力不匹配")
		}
		return contracts.KnowledgeCandidatesSchema, builtinskills.KnowledgeExtraction, nil
	default:
		return nil, "", domain.Invalid("TASK_TYPE_UNSUPPORTED", "本机运行环境不支持该任务类型")
	}
}
func binaryStatus(name string) map[string]any {
	path, err := exec.LookPath(name)
	return map[string]any{"available": err == nil, "path": path}
}
func credentialProvider() string {
	if runtime.GOOS == "darwin" {
		return "macos_keychain"
	}
	return "unsupported_fail_closed"
}
func writableTemp() bool {
	dir, err := os.MkdirTemp("", "contentcloud-doctor-")
	if err != nil {
		return false
	}
	_ = os.Remove(dir)
	return true
}
func mustConfigPath() string { p, _ := localconfig.Path(); return p }
func errorString(err error) any {
	if err == nil {
		return nil
	}
	return err.Error()
}
func defaultValue(v, f string) string {
	if v == "" {
		return f
	}
	return v
}
func skillDestination(target, name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch target {
	case "codex":
		return filepath.Join(home, ".codex", "skills", name), nil
	case "claude":
		return filepath.Join(home, ".claude", "skills", name), nil
	default:
		return "", domain.Invalid("SKILL_TARGET_INVALID", "--target 必须为 codex 或 claude")
	}
}

func commandSchemas() map[string]any {
	read := func(args []string, output string) map[string]any { return schemaEntry("read", "none", args, output) }
	userRead := func(args []string, output string) map[string]any { return schemaEntry("read", "user", args, output) }
	workspaceRead := func(args []string, output string) map[string]any {
		return schemaEntry("read", "workspace", args, output)
	}
	write := func(auth string, args []string, output string) map[string]any {
		return schemaEntry("write", auth, args, output)
	}
	high := func(args []string, output string) map[string]any {
		return schemaEntry("high-risk-write", "user", append(args, "--yes", "--dry-run"), output)
	}
	return map[string]any{
		"doctor": read([]string{"--offline"}, "诊断检查"), "status": read(nil, "本地运行环境和后台服务状态"), "update": read(nil, "已验证安装程序的更新指引"),
		"bootstrap.preflight": read([]string{"directory", "--offline"}, "稳定的前置检查 ID 和受管下一步"), "bootstrap.plan": schemaEntry("read", "browser-device", []string{"directory", "--session"}, "固定版本且只读的 Codex 插件和工作区计划"), "bootstrap.apply": write("browser-device", []string{"directory", "--session", "--plan-id", "--accept", "--open-codex"}, "已授权插件、已注册工作区和新对话交接信息"), "bootstrap.resume": write("workspace", []string{"directory", "--accept", "--open-codex"}, "重新验证并注册现有初始化工作区"), "bootstrap.diagnostics": schemaEntry("read", "workspace-for-upload", []string{"directory", "--attempt", "--upload", "--accept-upload"}, "脱敏诊断预览或已确认上传结果"),
		"workspace.status": read([]string{"directory"}, "本地工作区绑定、模板和同步状态"), "workspace.doctor": read([]string{"directory", "--offline"}, "工作区、技能、MCP 和云端检查"), "workspace.fixture.apply": write("none", []string{"fixture.json", "--directory", "--project-id", "--workspace-id", "--device-id", "--server-url", "--target"}, "完整且确定性的 V3 验收工作区"), "workspace.execution-plan": read([]string{"--directory", "--run", "--intent", "--capability", "--input"}, "已验证的离线 LocalExecutionPlan 和精确能力包准备信息"), "workspace.prepare.plan": read([]string{"--directory", "--run", "--intent", "--capability", "--input"}, "已签名能力包的权限、数据流、费用和新对话影响"), "workspace.prepare.apply": write("none", []string{"--directory", "--run", "--intent", "--capability", "--input", "--preparation-id", "--accept"}, "已安装任务能力包、已验证环境锁、检查结果和新对话交接信息"), "workspace.conversation-context": read([]string{"directory", "--offline"}, "离线跨对话工作区上下文"), "workspace.project-brief.save": write("none", []string{"--directory", "--client", "--brand", "--product-or-service", "--objective", "--channel", "--audience", "--material-ref", "--notes"}, "已确认的本地项目简报和下一业务步骤"), "workspace.approved.list": read([]string{"--directory", "--type"}, "已验证的本地批准快照摘要"), "workspace.approved.show": read([]string{"snapshot-id", "--directory"}, "已验证的本地批准快照"),
		"local.source.register": write("none", []string{"file", "--directory", "--id", "--title", "--kind", "--storage"}, "不可变的本地来源记录"), "local.source.list": read([]string{"--directory"}, "本地来源目录"), "local.source.show": read([]string{"source-id", "--directory"}, "本地来源记录"), "local.source.ingest": write("none", []string{"source-id", "--directory"}, "本地证据包"), "local.source.verify": read([]string{"--directory"}, "来源完整性报告"),
		"local.run.init": write("none", []string{"--directory", "--id", "--intent", "--source-ref", "--with-ingest"}, "本地运行上下文"), "local.run.show": read([]string{"run-id", "--directory"}, "本地运行上下文"), "local.run.record": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--source-ref", "--changed-id", "--eligible-id", "--blocked-id", "--finding", "--output-path"}, "已更新的本地运行上下文"), "local.run.check": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--name", "--status", "--command", "--detail"}, "已记录的本地检查"), "local.run.advance": write("none", []string{"stage", "--directory", "--run", "--claim-token", "--revision", "--eligible-id", "--blocked-id", "--output-path"}, "已推进的本地运行上下文"), "local.run.resume": write("none", []string{"--directory", "--run", "--claim-token", "--revision"}, "已恢复的本地运行上下文"), "local.run.fail": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--finding"}, "失败的本地运行上下文"), "local.run.validate": read([]string{"--directory"}, "本地运行校验报告"),
		"local.run.claim": write("none", []string{"--directory", "--run", "--owner", "--revision", "--ttl", "--takeover-expired"}, "单写入方运行占用"), "local.run.renew": write("none", []string{"--directory", "--run", "--claim-token", "--ttl"}, "已续期的运行占用"), "local.run.release": write("none", []string{"--directory", "--run", "--claim-token"}, "已释放的运行占用"), "local.run.claim-status": read([]string{"--directory", "--run"}, "不含敏感信息的运行占用状态"),
		"local.handoff.create-ready": write("none", []string{"--directory", "--id", "--run", "--claim-token", "--revision", "--next-capability", "--next-action", "--input", "--blocker", "--pending-decision"}, "已就绪且摘要验证通过的交接记录"), "local.handoff.list-ready": read([]string{"--directory"}, "已就绪的交接记录"), "local.handoff.accept": write("none", []string{"--directory", "--id", "--owner", "--ttl", "--takeover-expired"}, "已领取的交接记录和运行占用"), "local.handoff.complete": write("none", []string{"--directory", "--id", "--claim-token"}, "已完成的交接记录"), "local.handoff.supersede": write("none", []string{"--directory", "--id"}, "已被新版本替代的交接记录"),
		"local.knowledge.import": write("none", []string{"knowledge-candidates.json", "--directory", "--run"}, "候选知识对象"), "local.knowledge.lint": read([]string{"--directory"}, "确定性知识校验报告"), "local.knowledge.query": read([]string{"--directory", "--channel", "--at"}, "可用、已阻断和参考知识"), "local.knowledge.diagnose": read([]string{"--directory", "--channel", "--at"}, "15 维诊断结果"), "local.knowledge.pack": write("none", []string{"--directory", "--id", "--name"}, "七层知识包和来源披露"),
		"local.audience.taxonomy.lint": read([]string{"taxonomy.json", "--directory"}, "已拉取人群分类的校验结果"), "local.audience.strategy.scaffold": write("none", []string{"--taxonomy", "--mode", "--audience", "--objective", "--test-type", "--primary-variable", "--directory"}, "本地受众策略候选"), "local.audience.strategy.lint": read([]string{"strategy.json", "--directory"}, "受众策略校验结果"), "local.offer.lint": read([]string{"offer.json", "--directory", "--at"}, "商业报价快照校验结果"),
		"local.brief.lint":         read([]string{"brief.json", "--directory"}, "V3 简报治理报告"),
		"local.content.batch.init": write("none", []string{"--directory", "--brief", "--directions", "--count", "--variant", "--control", "--id"}, "内容批次和冻结的本地上下文"), "local.content.batch.lint": read([]string{"--directory", "--batch", "--file"}, "内容批次候选校验结果"), "local.content.batch.finalize": write("none", []string{"--directory", "--batch", "--file"}, "已定稿的内容批次"), "local.content.item.lint": read([]string{"content-item.json", "--directory", "--batch"}, "内容对象校验结果"), "local.content.item.diff": read([]string{"--directory", "--baseline", "--candidate", "--allow"}, "声明范围内的内容对象修订差异"), "local.content.delivery.export": write("none", []string{"approved-content-item-id", "--directory", "--out"}, "已批准的 JSON、Markdown 和 XLSX 交付包"),
		"local.storyboard.create": write("none", []string{"--snapshot", "--content-item", "--capability-id", "--capability-version", "--capability-digest", "--id", "--directory"}, "本地分镜候选"), "local.storyboard.prepare": write("none", []string{"manifest.json", "--directory"}, "已达到审核条件的本地分镜包"), "local.storyboard.lint": read([]string{"manifest.json", "--directory"}, "分镜摘要和媒体校验结果"),
		"local.seedance.export": write("none", []string{"--snapshot", "--storyboard", "--profile-version", "--adapter-id", "--adapter-version", "--adapter-digest", "--mode", "--aspect-ratio", "--sound", "--min-duration", "--max-duration", "--max-images", "--max-videos", "--max-audios", "--id", "--directory"}, "可直接复制的本地 Seedance 包"),
		"local.seedance.lint":   read([]string{"package.json", "--directory"}, "Seedance 包、提示词、媒体和锁定输入校验结果"),
		"mcp.status":            read([]string{"directory"}, "项目本地 MCP 安装状态"), "mcp.serve": read(nil, "stdio MCP 服务"),
		"publish.knowledge": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的知识提交修订版本"),
		"publish.research":  write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的调研提交修订版本"),
		"publish.strategy":  write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的策略提交修订版本"), "publish.offer": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的商业报价提交修订版本"), "publish.storyboard": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的分镜提交修订版本"),
		"publish.brief":       write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的简报提交修订版本"),
		"publish.script":      write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的剧本提交修订版本"),
		"publish.delivery":    write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的交付提交修订版本"),
		"publish.performance": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "不可变的效果提交修订版本"),
		"pull.feedback":       workspaceRead([]string{"--dry-run"}, "本地收件箱中的审核反馈包"), "pull.decisions": workspaceRead([]string{"--dry-run"}, "本地收件箱中的决定增量"), "pull.approved": workspaceRead([]string{"--type", "--id", "--dry-run"}, "只读的批准快照缓存"),
		"submission.list": workspaceRead(nil, "工作区提交列表"), "submission.show": workspaceRead([]string{"submission-id"}, "包含不可变修订版本的提交"), "submission.status": workspaceRead([]string{"submission-id"}, "提交治理状态"), "submission.approve": high([]string{"revision-id", "--reason"}, "不可变的批准快照"),
		"submission.request_changes": high([]string{"revision-id", "--reason", "--json-pointer"}, "不可变的修改要求和审核反馈"),
		"down":                       high(nil, "已撤销设备并清除本地绑定"),
		"auth.login":                 write("none", []string{"--no-wait", "--device-code"}, "设备登录状态"), "auth.status": read(nil, "用户会话状态"), "auth.logout": write("user", nil, "已撤销的用户会话"),
		"context.show": read(nil, "已解析的项目 ID"), "context.use": write("none", []string{"project-id"}, "本地上下文路径"), "context.clear": write("none", nil, "已清除的本地上下文"),
		"tenant.list": userRead(nil, "租户列表"), "tenant.switch": write("user", []string{"tenant-id", "--dry-run"}, "已轮换的租户凭据"),
		"membership.list": userRead(nil, "租户成员列表"), "membership.invite.list": userRead(nil, "租户邀请列表"), "membership.invite.create": write("user", []string{"email", "--role", "--dry-run"}, "一次性租户邀请"), "membership.invite.accept": write("user", []string{"invite-token", "--dry-run"}, "已接受的成员资格"), "membership.invite.revoke": high([]string{"invite-id"}, "已撤销的租户邀请"), "membership.update": write("user", []string{"user-id", "role", "--dry-run"}, "已更新的固定成员角色"), "membership.revoke": high([]string{"user-id"}, "已撤销的成员资格和租户会话"),
		"project.list": userRead(nil, "项目列表"), "project.show": userRead([]string{"project-id"}, "项目"), "project.resolve": userRead([]string{"name-or-slug"}, "稳定的项目 ID"), "project.create": write("user", []string{"--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--template", "--dry-run"}, "单一产品项目"), "project.update": write("user", []string{"project-id", "--row-version", "--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--dry-run"}, "通过乐观并发控制更新的项目"), "project.archive": high([]string{"project-id", "--row-version"}, "已归档的只读项目"), "project.restore": high([]string{"project-id", "--row-version"}, "已恢复的活跃项目"), "project_template.list": userRead(nil, "已脱敏的项目模板列表"), "project_template.create": write("user", []string{"--name", "--channel", "--objective", "--dry-run"}, "已脱敏的项目模板"),
		"device.connect_session.create": write("user", []string{"project-id", "--project", "--dry-run"}, "一次性项目连接会话"), "device.connect_session.show": userRead([]string{"session-id"}, "项目连接会话"), "device.connect_session.cancel": high([]string{"session-id"}, "已取消的项目连接会话"),
		"device.list": userRead([]string{"--project"}, "设备列表"), "device.show": userRead([]string{"device-id"}, "设备"), "device.attach": write("user", []string{"device-id", "--project", "--dry-run"}, "项目设备授权"), "device.detach": high([]string{"device-id", "--project"}, "已撤销的项目设备授权"), "device.revoke": high([]string{"device-id"}, "已撤销的设备"),
		"source.list": userRead([]string{"--project"}, "来源列表"), "source.upload": write("user", []string{"file", "--project", "--name", "--type", "--mime", "--dry-run"}, "来源修订版本"), "source.status": userRead([]string{"revision-id"}, "来源修订版本状态"),
		"source.revisions": userRead([]string{"source-id"}, "不可变的来源修订版本列表"), "source.revise": write("user", []string{"source-id", "file", "--mime", "--dry-run"}, "新的不可变来源修订版本"), "source.impact": userRead([]string{"source-id"}, "受影响对象列表"), "evidence.review": write("user", []string{"evidence-id", "decision", "--dry-run"}, "已审核的证据片段"),
		"asset.list": userRead([]string{"--project"}, "受治理素材列表"), "asset.create": write("user", []string{"--project", "--name", "--type", "--source-revision", "--usage", "--dry-run"}, "受治理素材"), "rights.list": userRead([]string{"asset-id"}, "素材权利记录"), "rights.create": write("user", []string{"asset-id", "--holder", "--type", "--territory", "--channel", "--proof-source-revision", "--valid-from", "--valid-until", "--restriction", "--dry-run"}, "权利记录"), "rights.review": write("user", []string{"rights-id", "decision", "--dry-run"}, "已审核的权利记录"),
		"knowledge.list": userRead([]string{"--project"}, "知识对象列表"), "knowledge.show": userRead([]string{"knowledge-id"}, "知识对象"), "knowledge.extract": write("user", []string{"--project", "--source-revision", "--count", "--idempotency-key", "--dry-run"}, "已排队的本地知识提取运行"), "knowledge.review": write("user", []string{"id", "decision", "--reason", "--dry-run"}, "已审核的知识对象"),
		"run.list": userRead([]string{"--project"}, "运行列表"), "run.show": userRead([]string{"run-id"}, "任务运行"), "run.attempts": userRead([]string{"run-id"}, "不可变的执行尝试列表"), "run.events": userRead([]string{"run-id", "--after"}, "不可变的增量进度事件"), "run.log": userRead([]string{"run-id"}, "已脱敏的持久化进度"), "run.cancel": high([]string{"run-id"}, "已取消的任务运行"),
		"artifact.export": write("user", []string{"approved-snapshot-id", "--content-item", "--format"}, "由快照派生的成果文件"), "delivery.create": write("user", []string{"approved-snapshot-id", "--content-item"}, "包含三种格式的交付包"), "delivery.list": userRead([]string{"--project"}, "交付包列表"), "delivery.show": userRead([]string{"delivery-package-id"}, "交付包"), "artifact.download": userRead([]string{"artifact-id", "--out"}, "托管成果文件路径"),
		"review.create": write("user", []string{"submission-revision-id", "--email", "--dry-run"}, "一次性客户审核链接"), "review.list": userRead([]string{"submission-revision-id"}, "客户审核授权列表"), "review.revoke": high([]string{"grant-id", "--dry-run"}, "已撤销的客户审核授权"), "review.status": userRead([]string{"submission-revision-id"}, "客户审核状态"),
		"result.list": userRead([]string{"--project"}, "观察数据列表"), "result.import": write("user", []string{"json-or-csv-or-xlsx-file", "--project", "--dry-run"}, "原子化效果数据导入批次"), "result.batches": userRead([]string{"--project"}, "不可变的导入批次列表"), "result.batch-show": userRead([]string{"batch-id"}, "导入批次及其观察数据"), "result.rate": write("user", []string{"subject-type", "subject-id", "--project", "--observation", "--rating", "--reason", "--next-action", "--dry-run"}, "人工评分决定"), "result.ratings": userRead([]string{"--project"}, "人工评分决定列表"),
		"lineage.show": userRead([]string{"--project", "--type", "--id", "--direction"}, "双向项目血缘图"), "lineage.impact": userRead([]string{"--project", "--type", "--id"}, "包含原因和动作的受影响对象"), "audit.list": userRead([]string{"--project", "--limit"}, "不可变的审计事件列表"),
		"daemon.start": write("device", nil, "已安装并运行的用户级后台服务"), "daemon.stop": write("none", nil, "已停止的后台服务"), "daemon.status": read(nil, "后台服务进程、日志、版本和最近运行健康状态"), "daemon.restart": write("device", []string{"--if-installed"}, "已使用当前二进制文件重新加载的后台服务"), "daemon.run": write("device", []string{"--once", "--fixture", "--adapter", "--log-file"}, "租约任务运行结果"), "skills.list": read(nil, "内置技能列表"), "skills.read": read([]string{"name", "--path"}, "技能内容"), "skills.status": read(nil, "技能版本状态"), "skills.install": write("none", []string{"name", "--target"}, "本地安装路径"), "schema": read([]string{"command"}, "CLI 契约"), "request.get": userRead([]string{"projects|tenants|runs"}, "允许列表中的资源"),
	}
}

func schemaEntry(risk, auth string, arguments []string, output string) map[string]any {
	if arguments == nil {
		arguments = []string{}
	}
	return map[string]any{"risk": risk, "auth": auth, "arguments": arguments, "output": output, "supports_json": true}
}
