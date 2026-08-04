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
	cmd := &cobra.Command{Use: "contentcloud", Short: "ContentCloud CLI-first creative runtime", Long: "Manage ContentCloud projects and run local creative capabilities without exposing private service APIs.", SilenceErrors: true, SilenceUsage: true}
	cmd.SetOut(r.stdout)
	cmd.SetErr(r.stderr)
	cmd.PersistentFlags().BoolVar(&r.json, "json", false, "emit a stable JSON envelope on stdout")
	cmd.PersistentFlags().StringVar(&r.serverURL, "server-url", "", "ContentCloud server URL")
	cmd.PersistentFlags().StringVar(&r.projectID, "project", "", "explicit project ID")
	cmd.AddCommand(r.authCommand(), r.doctor(), r.bootstrapCommand(), r.workspaceCommand(), r.localCommand(), r.mcpCommand(), r.publishCommand(), r.pullCommand(), r.submissionCommand(), r.down(), r.updateCommand(), r.status(), r.contextCommand(), r.skillsCommand(), r.daemonCommand(), r.schemaCommand(), r.tenantCommand(), r.teamCommand(), r.fullProjectCommand(), r.deviceCommand(), r.sourceCommand(), r.assetCommand(), r.knowledgeCommand(), r.runCommand(), r.artifactCommand(), r.deliveryCommand(), r.reviewCommand(), r.resultCommand(), r.lineageCommand(), r.auditCommand(), r.requestCommand())
	cmd.Version = Version
	return cmd
}

func (r *Root) doctor() *cobra.Command {
	var offline bool
	cmd := &cobra.Command{Use: "doctor", Short: "Check installation, config, credential storage, server, and local capabilities", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.Flags().BoolVar(&offline, "offline", false, "skip server reachability check")
	return cmd
}

func (r *Root) down() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{Use: "down", Short: "Revoke this creative runtime and clear its local device binding", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm this high-risk write")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing server or local state")
	return cmd
}

func (r *Root) updateCommand() *cobra.Command {
	return &cobra.Command{Use: "update", Short: "Show the verified installer command for updating this binary", RunE: func(cmd *cobra.Command, args []string) error {
		return r.writeOK("update", map[string]any{"current_version": Version, "installer": "npx --yes @limecloud/contentcloud@latest update", "automatic_update": false, "installer_owned": true, "daemon_restart_after_update": true, "reason": "the verified npm installer owns checksum validation, binary replacement, and restart of an installed daemon"})
	}}
}

func (r *Root) status() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show local runtime and project connection state", RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd := &cobra.Command{Use: "context", Short: "Manage explicit project context"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "Show resolved project context", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		id, err := localconfig.ResolveProject(r.projectID, cfg)
		if err != nil {
			return domain.Invalid("PROJECT_CONTEXT_REQUIRED", "未解析到唯一项目上下文")
		}
		return r.writeOK("context.show", map[string]any{"project_id": id})
	}}, &cobra.Command{Use: "use <project-id>", Args: cobra.ExactArgs(1), Short: "Write project context in the current directory", RunE: func(cmd *cobra.Command, args []string) error {
		dir := filepath.Join(".contentcloud")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		b, _ := json.MarshalIndent(map[string]string{"project_id": args[0]}, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "project.json"), b, 0600); err != nil {
			return err
		}
		return r.writeOK("context.use", map[string]any{"project_id": args[0], "path": filepath.Join(dir, "project.json")})
	}}, &cobra.Command{Use: "clear", Short: "Remove project context from the current directory", RunE: func(cmd *cobra.Command, args []string) error {
		path := filepath.Join(".contentcloud", "project.json")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return r.writeOK("context.clear", map[string]any{"cleared": true, "path": path})
	}})
	return cmd
}

func (r *Root) skillsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "skills", Short: "Inspect and install CLI-versioned local ContentCloud skills"}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List embedded skills", RunE: func(cmd *cobra.Command, args []string) error {
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
	read := &cobra.Command{Use: "read <name>", Args: cobra.ExactArgs(1), Short: "Read an embedded skill instruction or reference", RunE: func(cmd *cobra.Command, args []string) error {
		body, err := builtinskills.Read(args[0], path)
		if err != nil {
			return domain.NotFound("Skill")
		}
		if r.json {
			return r.writeOK("skills.read", map[string]any{"name": args[0], "path": defaultValue(path, "SKILL.md"), "content": string(body)})
		}
		_, err = fmt.Fprint(r.stdout, string(body))
		return err
	}}
	read.Flags().StringVar(&path, "path", "SKILL.md", "relative skill file")
	cmd.AddCommand(read)
	var target string
	install := &cobra.Command{Use: "install <name>", Args: cobra.ExactArgs(1), Short: "Install the embedded skill into a local Agent skill directory", RunE: func(cmd *cobra.Command, args []string) error {
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
	install.Flags().StringVar(&target, "target", "codex", "agent target: codex or claude")
	cmd.AddCommand(install, &cobra.Command{Use: "status", Short: "Check embedded skill availability", RunE: func(cmd *cobra.Command, args []string) error {
		return r.writeOK("skills.status", map[string]any{"binary_version": Version, "embedded": builtinskills.Names(), "in_sync": true})
	}})
	return cmd
}

func (r *Root) daemonCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon", Short: "Run the local outbound-only creative runtime"}
	start := &cobra.Command{Use: "start", Short: "Install and start the user-level Automation daemon", RunE: func(cmd *cobra.Command, args []string) error {
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
	stop := &cobra.Command{Use: "stop", Short: "Stop the user-level Automation daemon without removing it", RunE: func(cmd *cobra.Command, args []string) error {
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
	status := &cobra.Command{Use: "status", Short: "Show daemon installation, process, logs, and version", RunE: func(cmd *cobra.Command, args []string) error {
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
	restart := &cobra.Command{Use: "restart", Short: "Reload the daemon with the current ContentCloud binary", RunE: func(cmd *cobra.Command, args []string) error {
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
	restart.Flags().BoolVar(&ifInstalled, "if-installed", false, "skip successfully when the daemon is not installed")
	var once, fixture bool
	var adapterKind string
	var logFile string
	run := &cobra.Command{Use: "run", Short: "Poll for leased work and execute a local capability", RunE: func(cmd *cobra.Command, args []string) error {
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
			return domain.Conflict("DEVICE_BINDING_MISSING", "启动 Automation Daemon 前必须先完成设备注册")
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
				return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "指定的本地 Agent 不可用", "检查安装与登录状态")
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
					runErr := domain.Invalid("TASK_TYPE_UNSUPPORTED", "fixture 不支持该任务类型")
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
	run.Flags().BoolVar(&once, "once", false, "poll at most once")
	run.Flags().BoolVar(&fixture, "fixture", false, "use deterministic local fixture adapter for development")
	run.Flags().StringVar(&adapterKind, "adapter", "auto", "local Agent adapter: auto, codex, or claude-code; other registered clients are planned")
	run.Flags().StringVar(&logFile, "log-file", "", "managed daemon log path")
	cmd.AddCommand(start, stop, status, restart, run)
	return cmd
}

func daemonStartPrerequisites() error {
	cfg, err := localconfig.Load()
	if err != nil {
		return err
	}
	if len(cfg.RuntimeBindings()) == 0 && cfg.DeviceID == "" {
		return domain.Conflict("DEVICE_BINDING_MISSING", "启动 Automation Daemon 前必须先完成设备注册")
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
		return domain.Policy("AGENT_ADAPTER_UNAVAILABLE", "未检测到可用于 Automation 的 Codex 或 Claude Code", "安装并登录本机 Agent 后重试")
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
			wrapped := domain.Conflict("AUTOMATION_ENVIRONMENT_CLAIM_UNAVAILABLE", "无法读取完整的本地 Environment Manifest/Lock")
			wrapped.Hint = "完成 Environment doctor 后重试 daemon poll"
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
	return &cobra.Command{Use: "schema [command]", Args: cobra.MaximumNArgs(1), Short: "Show stable CLI command contracts and risk levels", RunE: func(cmd *cobra.Command, args []string) error {
		schemas := commandSchemas()
		if len(args) == 1 {
			value, ok := schemas[args[0]]
			if !ok {
				return domain.NotFound("命令 Schema")
			}
			return r.writeOK("schema", value)
		}
		return r.writeOK("schema", schemas)
	}}
}
func (r *Root) projectCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Discover and inspect projects"}
	cmd.AddCommand(&cobra.Command{Use: "show", Short: "Show the configured project identity", RunE: func(cmd *cobra.Command, args []string) error {
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
			return nil, "", domain.Conflict("OUTPUT_SCHEMA_MISMATCH", "知识提取任务输出 Schema 与本机能力不匹配")
		}
		return contracts.KnowledgeCandidatesSchema, builtinskills.KnowledgeExtraction, nil
	default:
		return nil, "", domain.Invalid("TASK_TYPE_UNSUPPORTED", "本机 Runtime 不支持该任务类型")
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
		"doctor": read([]string{"--offline"}, "diagnostic checks"), "status": read(nil, "local runtime and daemon status"), "update": read(nil, "verified installer guidance"),
		"bootstrap.preflight": read([]string{"directory", "--offline"}, "stable prerequisite check IDs and managed next actions"), "bootstrap.plan": schemaEntry("read", "browser-device", []string{"directory", "--session"}, "read-only pinned Codex Plugin and Workspace plan"), "bootstrap.apply": write("browser-device", []string{"directory", "--session", "--plan-id", "--accept", "--open-codex"}, "authorized plugin, registered Workspace, and new-chat handoff"), "bootstrap.resume": write("workspace", []string{"directory", "--accept", "--open-codex"}, "revalidated and registered existing bootstrap Workspace"), "bootstrap.diagnostics": schemaEntry("read", "workspace-for-upload", []string{"directory", "--attempt", "--upload", "--accept-upload"}, "redacted diagnostic preview or confirmed upload"),
		"workspace.status": read([]string{"directory"}, "local workspace binding, template, and synchronization state"), "workspace.doctor": read([]string{"directory", "--offline"}, "workspace, Skill, MCP, and cloud checks"), "workspace.fixture.apply": write("none", []string{"fixture.json", "--directory", "--project-id", "--workspace-id", "--device-id", "--server-url", "--target"}, "complete deterministic V3 acceptance workspace"), "workspace.execution-plan": read([]string{"--directory", "--run", "--intent", "--capability", "--input"}, "verified offline LocalExecutionPlan and exact Pack preparation"), "workspace.prepare.plan": read([]string{"--directory", "--run", "--intent", "--capability", "--input"}, "signed Pack permissions, data flow, cost, and new-chat impact"), "workspace.prepare.apply": write("none", []string{"--directory", "--run", "--intent", "--capability", "--input", "--preparation-id", "--accept"}, "installed task Packs, verified environment lock, doctor, and new-chat handoff"), "workspace.conversation-context": read([]string{"directory", "--offline"}, "offline cross-conversation workspace context"), "workspace.project-brief.save": write("none", []string{"--directory", "--client", "--brand", "--product-or-service", "--objective", "--channel", "--audience", "--material-ref", "--notes"}, "confirmed local project brief and next business step"), "workspace.approved.list": read([]string{"--directory", "--type"}, "verified local ApprovedSnapshot summaries"), "workspace.approved.show": read([]string{"snapshot-id", "--directory"}, "verified local ApprovedSnapshot"),
		"local.source.register": write("none", []string{"file", "--directory", "--id", "--title", "--kind", "--storage"}, "immutable local source record"), "local.source.list": read([]string{"--directory"}, "local source registry"), "local.source.show": read([]string{"source-id", "--directory"}, "local source record"), "local.source.ingest": write("none", []string{"source-id", "--directory"}, "local evidence bundle"), "local.source.verify": read([]string{"--directory"}, "source integrity report"),
		"local.run.init": write("none", []string{"--directory", "--id", "--intent", "--source-ref", "--with-ingest"}, "LocalRunContext"), "local.run.show": read([]string{"run-id", "--directory"}, "LocalRunContext"), "local.run.record": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--source-ref", "--changed-id", "--eligible-id", "--blocked-id", "--finding", "--output-path"}, "updated LocalRunContext"), "local.run.check": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--name", "--status", "--command", "--detail"}, "recorded local check"), "local.run.advance": write("none", []string{"stage", "--directory", "--run", "--claim-token", "--revision", "--eligible-id", "--blocked-id", "--output-path"}, "advanced LocalRunContext"), "local.run.resume": write("none", []string{"--directory", "--run", "--claim-token", "--revision"}, "resumed LocalRunContext"), "local.run.fail": write("none", []string{"--directory", "--run", "--claim-token", "--revision", "--finding"}, "failed LocalRunContext"), "local.run.validate": read([]string{"--directory"}, "LocalRun validation report"),
		"local.run.claim": write("none", []string{"--directory", "--run", "--owner", "--revision", "--ttl", "--takeover-expired"}, "single-writer RunClaim"), "local.run.renew": write("none", []string{"--directory", "--run", "--claim-token", "--ttl"}, "renewed RunClaim"), "local.run.release": write("none", []string{"--directory", "--run", "--claim-token"}, "released RunClaim"), "local.run.claim-status": read([]string{"--directory", "--run"}, "non-secret RunClaim status"),
		"local.handoff.create-ready": write("none", []string{"--directory", "--id", "--run", "--claim-token", "--revision", "--next-capability", "--next-action", "--input", "--blocker", "--pending-decision"}, "ready digest-verified HandoffRecord"), "local.handoff.list-ready": read([]string{"--directory"}, "ready HandoffRecords"), "local.handoff.accept": write("none", []string{"--directory", "--id", "--owner", "--ttl", "--takeover-expired"}, "claimed HandoffRecord and RunClaim"), "local.handoff.complete": write("none", []string{"--directory", "--id", "--claim-token"}, "completed HandoffRecord"), "local.handoff.supersede": write("none", []string{"--directory", "--id"}, "superseded HandoffRecord"),
		"local.knowledge.import": write("none", []string{"knowledge-candidates.json", "--directory", "--run"}, "candidate knowledge objects"), "local.knowledge.lint": read([]string{"--directory"}, "deterministic knowledge lint report"), "local.knowledge.query": read([]string{"--directory", "--channel", "--at"}, "eligible, blocked, and informational knowledge"), "local.knowledge.diagnose": read([]string{"--directory", "--channel", "--at"}, "15-dimension diagnosis"), "local.knowledge.pack": write("none", []string{"--directory", "--id", "--name"}, "seven-layer knowledge pack and source disclosures"),
		"local.audience.taxonomy.lint": read([]string{"taxonomy.json", "--directory"}, "pulled audience taxonomy validation"), "local.audience.strategy.scaffold": write("none", []string{"--taxonomy", "--mode", "--audience", "--objective", "--test-type", "--primary-variable", "--directory"}, "local audience strategy candidates"), "local.audience.strategy.lint": read([]string{"strategy.json", "--directory"}, "audience strategy validation"), "local.offer.lint": read([]string{"offer.json", "--directory", "--at"}, "CommerceOfferSnapshot validation"),
		"local.brief.lint":         read([]string{"brief.json", "--directory"}, "V3 Brief governance report"),
		"local.content.batch.init": write("none", []string{"--directory", "--brief", "--directions", "--count", "--variant", "--control", "--id"}, "ContentBatch and frozen local context"), "local.content.batch.lint": read([]string{"--directory", "--batch", "--file"}, "ContentBatch candidate validation"), "local.content.batch.finalize": write("none", []string{"--directory", "--batch", "--file"}, "finalized ContentBatch"), "local.content.item.lint": read([]string{"content-item.json", "--directory", "--batch"}, "ContentItem validation"), "local.content.item.diff": read([]string{"--directory", "--baseline", "--candidate", "--allow"}, "declared ContentItem revision diff"), "local.content.delivery.export": write("none", []string{"approved-content-item-id", "--directory", "--out"}, "approved JSON, Markdown, and XLSX delivery package"),
		"local.storyboard.create": write("none", []string{"--snapshot", "--content-item", "--capability-id", "--capability-version", "--capability-digest", "--id", "--directory"}, "local storyboard candidate"), "local.storyboard.prepare": write("none", []string{"manifest.json", "--directory"}, "review-ready local storyboard package"), "local.storyboard.lint": read([]string{"manifest.json", "--directory"}, "storyboard digest and media validation"),
		"local.seedance.export": write("none", []string{"--snapshot", "--storyboard", "--profile-version", "--adapter-id", "--adapter-version", "--adapter-digest", "--mode", "--aspect-ratio", "--sound", "--min-duration", "--max-duration", "--max-images", "--max-videos", "--max-audios", "--id", "--directory"}, "copy-ready local Seedance package"),
		"local.seedance.lint":   read([]string{"package.json", "--directory"}, "Seedance package, prompt, media, and locked-input validation"),
		"mcp.status":            read([]string{"directory"}, "project-local MCP installation"), "mcp.serve": read(nil, "stdio MCP server"),
		"publish.knowledge": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable knowledge SubmissionRevision"),
		"publish.research":  write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable research SubmissionRevision"),
		"publish.strategy":  write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable strategy SubmissionRevision"), "publish.offer": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable offer SubmissionRevision"), "publish.storyboard": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable storyboard SubmissionRevision"),
		"publish.brief":       write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable brief SubmissionRevision"),
		"publish.script":      write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable script SubmissionRevision"),
		"publish.delivery":    write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable delivery SubmissionRevision"),
		"publish.performance": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable performance SubmissionRevision"),
		"pull.feedback":       workspaceRead([]string{"--dry-run"}, "review feedback bundles in local inbox"), "pull.decisions": workspaceRead([]string{"--dry-run"}, "decision delta in local inbox"), "pull.approved": workspaceRead([]string{"--type", "--id", "--dry-run"}, "read-only ApprovedSnapshot cache"),
		"submission.list": workspaceRead(nil, "workspace submission list"), "submission.show": workspaceRead([]string{"submission-id"}, "submission with immutable revisions"), "submission.status": workspaceRead([]string{"submission-id"}, "submission governance status"), "submission.approve": high([]string{"revision-id", "--reason"}, "immutable ApprovedSnapshot"),
		"submission.request_changes": high([]string{"revision-id", "--reason", "--json-pointer"}, "immutable change request and review feedback"),
		"down":                       high(nil, "revoked device and cleared local binding"),
		"auth.login":                 write("none", []string{"--no-wait", "--device-code"}, "device login state"), "auth.status": read(nil, "user session state"), "auth.logout": write("user", nil, "revoked user session"),
		"context.show": read(nil, "resolved project ID"), "context.use": write("none", []string{"project-id"}, "local context path"), "context.clear": write("none", nil, "cleared local context"),
		"tenant.list": userRead(nil, "tenant list"), "tenant.switch": write("user", []string{"tenant-id", "--dry-run"}, "rotated tenant credential"),
		"membership.list": userRead(nil, "tenant member list"), "membership.invite.list": userRead(nil, "tenant invitation list"), "membership.invite.create": write("user", []string{"email", "--role", "--dry-run"}, "one-time tenant invitation"), "membership.invite.accept": write("user", []string{"invite-token", "--dry-run"}, "accepted membership"), "membership.invite.revoke": high([]string{"invite-id"}, "revoked tenant invitation"), "membership.update": write("user", []string{"user-id", "role", "--dry-run"}, "updated fixed membership role"), "membership.revoke": high([]string{"user-id"}, "revoked membership and tenant sessions"),
		"project.list": userRead(nil, "project list"), "project.show": userRead([]string{"project-id"}, "project"), "project.resolve": userRead([]string{"name-or-slug"}, "stable project ID"), "project.create": write("user", []string{"--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--template", "--dry-run"}, "single-product project"), "project.update": write("user", []string{"project-id", "--row-version", "--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--dry-run"}, "optimistically updated project"), "project.archive": high([]string{"project-id", "--row-version"}, "archived read-only project"), "project.restore": high([]string{"project-id", "--row-version"}, "restored active project"), "project_template.list": userRead(nil, "sanitized project template list"), "project_template.create": write("user", []string{"--name", "--channel", "--objective", "--dry-run"}, "sanitized project template"),
		"device.connect_session.create": write("user", []string{"project-id", "--project", "--dry-run"}, "one-time project connection session"), "device.connect_session.show": userRead([]string{"session-id"}, "project connection session"), "device.connect_session.cancel": high([]string{"session-id"}, "canceled project connection session"),
		"device.list": userRead([]string{"--project"}, "device list"), "device.show": userRead([]string{"device-id"}, "device"), "device.attach": write("user", []string{"device-id", "--project", "--dry-run"}, "project device grant"), "device.detach": high([]string{"device-id", "--project"}, "revoked project device grant"), "device.revoke": high([]string{"device-id"}, "revoked device"),
		"source.list": userRead([]string{"--project"}, "source list"), "source.upload": write("user", []string{"file", "--project", "--name", "--type", "--mime", "--dry-run"}, "source revision"), "source.status": userRead([]string{"revision-id"}, "source revision status"),
		"source.revisions": userRead([]string{"source-id"}, "immutable source revision list"), "source.revise": write("user", []string{"source-id", "file", "--mime", "--dry-run"}, "new immutable source revision"), "source.impact": userRead([]string{"source-id"}, "affected object list"), "evidence.review": write("user", []string{"evidence-id", "decision", "--dry-run"}, "reviewed evidence span"),
		"asset.list": userRead([]string{"--project"}, "governed asset list"), "asset.create": write("user", []string{"--project", "--name", "--type", "--source-revision", "--usage", "--dry-run"}, "governed asset"), "rights.list": userRead([]string{"asset-id"}, "asset rights records"), "rights.create": write("user", []string{"asset-id", "--holder", "--type", "--territory", "--channel", "--proof-source-revision", "--valid-from", "--valid-until", "--restriction", "--dry-run"}, "rights record"), "rights.review": write("user", []string{"rights-id", "decision", "--dry-run"}, "reviewed rights record"),
		"knowledge.list": userRead([]string{"--project"}, "knowledge object list"), "knowledge.show": userRead([]string{"knowledge-id"}, "knowledge object"), "knowledge.extract": write("user", []string{"--project", "--source-revision", "--count", "--idempotency-key", "--dry-run"}, "queued local knowledge extraction run"), "knowledge.review": write("user", []string{"id", "decision", "--reason", "--dry-run"}, "reviewed knowledge object"),
		"run.list": userRead([]string{"--project"}, "run list"), "run.show": userRead([]string{"run-id"}, "task run"), "run.attempts": userRead([]string{"run-id"}, "immutable execution attempt list"), "run.events": userRead([]string{"run-id", "--after"}, "immutable incremental progress events"), "run.log": userRead([]string{"run-id"}, "sanitized persisted progress"), "run.cancel": high([]string{"run-id"}, "canceled task run"),
		"artifact.export": write("user", []string{"approved-snapshot-id", "--content-item", "--format"}, "snapshot-derived artifact"), "delivery.create": write("user", []string{"approved-snapshot-id", "--content-item"}, "three-format delivery package"), "delivery.list": userRead([]string{"--project"}, "delivery package list"), "delivery.show": userRead([]string{"delivery-package-id"}, "delivery package"), "artifact.download": userRead([]string{"artifact-id", "--out"}, "hosted artifact path"),
		"review.create": write("user", []string{"submission-revision-id", "--email", "--dry-run"}, "one-time customer review link"), "review.list": userRead([]string{"submission-revision-id"}, "customer review grants"), "review.revoke": high([]string{"grant-id", "--dry-run"}, "revoked customer review grant"), "review.status": userRead([]string{"submission-revision-id"}, "customer review state"),
		"result.list": userRead([]string{"--project"}, "observation list"), "result.import": write("user", []string{"json-or-csv-or-xlsx-file", "--project", "--dry-run"}, "atomic performance import batch"), "result.batches": userRead([]string{"--project"}, "immutable import batch list"), "result.batch-show": userRead([]string{"batch-id"}, "import batch and observations"), "result.rate": write("user", []string{"subject-type", "subject-id", "--project", "--observation", "--rating", "--reason", "--next-action", "--dry-run"}, "manual rating decision"), "result.ratings": userRead([]string{"--project"}, "manual rating decision list"),
		"lineage.show": userRead([]string{"--project", "--type", "--id", "--direction"}, "bidirectional project lineage graph"), "lineage.impact": userRead([]string{"--project", "--type", "--id"}, "affected objects with reasons and actions"), "audit.list": userRead([]string{"--project", "--limit"}, "immutable audit event list"),
		"daemon.start": write("device", nil, "installed and running user daemon"), "daemon.stop": write("none", nil, "stopped installed daemon"), "daemon.status": read(nil, "daemon process, logs, version, and last runtime health"), "daemon.restart": write("device", []string{"--if-installed"}, "daemon reloaded with current binary"), "daemon.run": write("device", []string{"--once", "--fixture", "--adapter", "--log-file"}, "leased run result"), "skills.list": read(nil, "embedded skills"), "skills.read": read([]string{"name", "--path"}, "skill content"), "skills.status": read(nil, "skill version state"), "skills.install": write("none", []string{"name", "--target"}, "local install path"), "schema": read([]string{"command"}, "CLI contract"), "request.get": userRead([]string{"projects|tenants|runs"}, "allowlisted resource"),
	}
}

func schemaEntry(risk, auth string, arguments []string, output string) map[string]any {
	if arguments == nil {
		arguments = []string{}
	}
	return map[string]any{"risk": risk, "auth": auth, "arguments": arguments, "output": output, "supports_json": true}
}
