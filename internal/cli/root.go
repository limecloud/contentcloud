package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/contracts"
	"github.com/limecloud/contentcloud/internal/agentadapter"
	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
	builtinskills "github.com/limecloud/contentcloud/skills"
)

const Version = "0.1.0-dev"

type Root struct {
	json      bool
	serverURL string
	projectID string
	stdout    io.Writer
	stderr    io.Writer
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
	cmd.AddCommand(r.authCommand(), r.doctor(), r.initCommand(), r.workspaceCommand(), r.mcpCommand(), r.publishCommand(), r.pullCommand(), r.submissionCommand(), r.up(), r.down(), r.updateCommand(), r.status(), r.contextCommand(), r.skillsCommand(), r.daemonCommand(), r.schemaCommand(), r.tenantCommand(), r.teamCommand(), r.fullProjectCommand(), r.deviceCommand(), r.sourceCommand(), r.assetCommand(), r.knowledgeCommand(), r.briefCommand(), r.runCommand(), r.scriptCommand(), r.artifactCommand(), r.reviewCommand(), r.resultCommand(), r.lineageCommand(), r.auditCommand(), r.requestCommand())
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
		checks := map[string]any{"version": map[string]any{"ok": true, "value": Version}, "config": map[string]any{"ok": err == nil, "path": mustConfigPath()}, "credential_store": map[string]any{"ok": runtime.GOOS == "darwin", "provider": credentialProvider()}, "temp_directory": map[string]any{"ok": writableTemp()}, "capabilities": detectCapabilities()}
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

func (r *Root) up() *cobra.Command {
	var key, name string
	cmd := &cobra.Command{Use: "up", Short: "Connect this computer to an existing ContentCloud project", RunE: func(cmd *cobra.Command, args []string) error {
		if key == "" {
			return domain.Invalid("CONNECT_KEY_REQUIRED", "--connect-key 必填")
		}
		_, result, err := r.connectDevice(cmd.Context(), key, name)
		if err != nil {
			return err
		}
		if err := installUserDaemon(); err != nil {
			return &domain.Error{Type: "runtime", Subtype: "service_manager", Code: "DAEMON_INSTALL_FAILED", Message: err.Error(), Hint: "凭据已安全保存；运行 contentcloud daemon run，或修复 LaunchAgent 后重新注册后台服务", ExitCode: 5}
		}
		return r.writeOK("up", map[string]any{"device": result.Device, "project_id": result.ProjectID, "state": "verifying", "daemon_registered": true, "credential_store": credentialProvider()})
	}}
	cmd.Flags().StringVar(&key, "connect-key", "", "one-time project connection key")
	cmd.Flags().StringVar(&name, "name", "", "device display name")
	return cmd
}

func (r *Root) connectDevice(ctx context.Context, key, name string) (localconfig.Config, app.ConnectDeviceResult, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return cfg, app.ConnectDeviceResult{}, err
	}
	server := r.resolveServer(cfg)
	if _, err := url.ParseRequestURI(server); err != nil {
		return cfg, app.ConnectDeviceResult{}, domain.Invalid("SERVER_URL_INVALID", "server URL 无效")
	}
	host, _ := os.Hostname()
	var result app.ConnectDeviceResult
	err = apiclient.New(server, "").Dispatch(ctx, "device.connect", app.ConnectDeviceInput{ConnectKey: key, DisplayName: name, Hostname: host, Platform: runtime.GOOS, Arch: runtime.GOARCH, Version: Version, Capabilities: builtinCapabilities()}, &result)
	if err != nil {
		return cfg, result, err
	}
	if err := localconfig.SaveDeviceToken(result.Device.ID, result.DeviceToken); err != nil {
		return cfg, result, &domain.Error{Type: "credential", Subtype: "secure_store", Code: "CREDENTIAL_STORE_FAILED", Message: err.Error(), Hint: "修复系统安全凭据存储后生成新的连接码", ExitCode: 3}
	}
	if err := localconfig.SaveWorkspaceToken(result.WorkspaceID, result.WorkspaceToken); err != nil {
		return cfg, result, &domain.Error{Type: "credential", Subtype: "secure_store", Code: "WORKSPACE_CREDENTIAL_STORE_FAILED", Message: err.Error(), Hint: "工作区凭据未能安全保存；请撤销当前连接后重新生成连接码", ExitCode: 3}
	}
	cfg.ServerURL = server
	cfg.DeviceID = result.Device.ID
	cfg.WorkspaceID = result.WorkspaceID
	cfg.ProjectID = result.ProjectID
	if err := localconfig.Save(cfg); err != nil {
		return cfg, result, err
	}
	return cfg, result, nil
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
		return r.writeOK("update", map[string]any{"current_version": Version, "installer": "npx --yes @goodvision/contentcloud@latest update", "automatic_update": false, "reason": "release manifest and checksum endpoint are required before in-process replacement is enabled"})
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
		return r.writeOK("status", map[string]any{"server_url": cfg.ServerURL, "device_id": cfg.DeviceID, "project_id": cfg.ProjectID, "device_credential": credential, "version": Version})
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
	var once, fixture bool
	var adapterKind string
	run := &cobra.Command{Use: "run", Short: "Poll for leased work and execute a local capability", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := localconfig.Load()
		if err != nil {
			return err
		}
		token, err := localconfig.DeviceToken(cfg.DeviceID)
		if err != nil {
			return &domain.Error{Type: "credential", Subtype: "device", Code: "DEVICE_CREDENTIAL_MISSING", Message: err.Error(), ExitCode: 3}
		}
		client := apiclient.New(r.resolveServer(cfg), token)
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
		execute := func() error {
			opened, openResult, err := handlePendingArtifactOpen(cmd.Context(), client)
			if err != nil {
				return err
			}
			if opened {
				return r.writeOK("daemon.artifact_open", openResult)
			}
			var lease app.Lease
			err = client.Dispatch(cmd.Context(), "daemon.poll", map[string]any{"capabilities": builtinCapabilities()}, &lease)
			if err != nil {
				var de *domain.Error
				if errors.As(err, &de) && de.Code == "NO_TASK" {
					return r.writeOK("daemon.poll", map[string]any{"leased": false})
				}
				return err
			}
			var output json.RawMessage
			if fixture {
				switch lease.Run.TaskType {
				case "script_generate", "script_revise":
					output, _ = json.Marshal(GenerateFixtureScript(lease.Contract))
				case "knowledge_extract":
					output, _ = json.Marshal(GenerateFixtureKnowledge(lease.Contract, lease.Run.OutputCount))
				default:
					runErr := domain.Invalid("TASK_TYPE_UNSUPPORTED", "fixture 不支持该任务类型")
					return finishAttemptError(client, lease, "runtime_resources", "本地开发 Fixture 不支持该任务类型", runErr)
				}
			} else {
				var heartbeatResult struct {
					CancelRequested bool `json:"cancel_requested"`
				}
				if err := client.Dispatch(cmd.Context(), "run.heartbeat", map[string]any{"run_id": lease.Run.ID, "attempt_id": lease.Attempt.ID, "run_token": lease.RunToken, "heartbeat": domain.RunHeartbeat{Sequence: 1, Phase: "contract_ready", Step: 1, Label: "上下文校验完成"}}, &heartbeatResult); err != nil {
					return finishAttemptError(client, lease, "heartbeat_failed", "首次心跳未完成", err)
				}
				if heartbeatResult.CancelRequested {
					cancelErr := domain.Conflict("RUN_CANCEL_REQUESTED", "任务已被用户取消")
					if err := finishAttempt(client, lease, "canceled", "user_canceled", "服务端取消请求已由本地客户端确认", nil); err != nil {
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
								CancelRequested bool `json:"cancel_requested"`
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
							sequence++
						}
					}
				}()
				schema, skillName, resourceErr := taskRuntimeResources(lease.Run)
				if resourceErr != nil {
					cancel()
					<-heartbeatDone
					return finishAttemptError(client, lease, "runtime_resources", "本地任务资源选择失败", resourceErr)
				}
				skillBody, resourceErr := builtinskills.Read(skillName, "SKILL.md")
				if resourceErr != nil {
					cancel()
					<-heartbeatDone
					return finishAttemptError(client, lease, "skill_load", "本地 Skill 加载失败", resourceErr)
				}
				output, err = adapter.Run(agentCtx, lease.Contract, schema, skillBody)
				cancel()
				<-heartbeatDone
				select {
				case heartbeatErr := <-heartbeatErrors:
					var de *domain.Error
					if errors.As(heartbeatErr, &de) && de.Code == "RUN_CANCEL_REQUESTED" {
						if finishErr := finishAttempt(client, lease, "canceled", "user_canceled", "服务端取消请求已由本地客户端确认", nil); finishErr != nil {
							return errors.Join(heartbeatErr, finishErr)
						}
						return heartbeatErr
					}
					return finishAttemptError(client, lease, "heartbeat_failed", "后台心跳中断", heartbeatErr)
				default:
				}
				if err != nil {
					failureClass, summary, exitCode := classifyAttemptFailure(err)
					return finishAttemptErrorWithExitCode(client, lease, failureClass, summary, exitCode, err)
				}
			}
			var report any
			if err := client.Dispatch(cmd.Context(), "run.report", map[string]any{"run_id": lease.Run.ID, "attempt_id": lease.Attempt.ID, "run_token": lease.RunToken, "package": output}, &report); err != nil {
				return err
			}
			return r.writeOK("daemon.run", map[string]any{"leased": true, "run_id": lease.Run.ID, "task_type": lease.Run.TaskType, "result": report})
		}
		if once {
			return execute()
		}
		for {
			if err := execute(); err != nil {
				fmt.Fprintln(r.stderr, err)
			}
			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(10 * time.Second):
			}
		}
	}}
	run.Flags().BoolVar(&once, "once", false, "poll at most once")
	run.Flags().BoolVar(&fixture, "fixture", false, "use deterministic local fixture adapter for development")
	run.Flags().StringVar(&adapterKind, "adapter", "auto", "local Agent adapter: auto, codex, or claude")
	cmd.AddCommand(run)
	return cmd
}

func finishAttempt(client *apiclient.Client, lease app.Lease, outcome, failureClass, summary string, exitCode *int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	params := map[string]any{
		"run_id":             lease.Run.ID,
		"attempt_id":         lease.Attempt.ID,
		"run_token":          lease.RunToken,
		"outcome":            outcome,
		"failure_class":      failureClass,
		"transcript_summary": summary,
	}
	if exitCode != nil {
		params["exit_code"] = *exitCode
	}
	return client.Dispatch(ctx, "run.finish", params, nil)
}

func finishAttemptError(client *apiclient.Client, lease app.Lease, failureClass, summary string, runErr error) error {
	return finishAttemptErrorWithExitCode(client, lease, failureClass, summary, nil, runErr)
}

func finishAttemptErrorWithExitCode(client *apiclient.Client, lease app.Lease, failureClass, summary string, exitCode *int, runErr error) error {
	if finishErr := finishAttempt(client, lease, "failed", failureClass, summary, exitCode); finishErr != nil {
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
	return []domain.Capability{
		{ID: domain.KnowledgeExtractCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.KnowledgeCandidatesSchema, PresentationProfiles: []string{"cloud_native"}, LocalOnly: true, Digest: "contentcloud-knowledge-extraction@" + Version},
		{ID: domain.ScriptCapability, Version: "1.1.0", Kind: "business_capability", InputSchema: domain.TaskContractSchema, OutputSchema: domain.ScriptPackageSchema, PresentationProfiles: []string{"review_projection/1.0", "text"}, LocalOnly: true, Digest: "contentcloud-marketing-video-script@" + Version},
		{ID: domain.ArtifactExportCapability, Version: "1.0.0", Kind: "business_capability", InputSchema: domain.ScriptPackageSchema, OutputSchema: "extension-artifact-envelope/1.0", PresentationProfiles: []string{"local_open"}, LocalOnly: true, Digest: "contentcloud-artifact-export@" + Version},
	}
}
func detectCapabilities() map[string]any {
	return map[string]any{"knowledge.extract": map[string]any{"ok": true, "version": "1.0.0"}, "script.generate": map[string]any{"ok": true, "version": "1.1.0"}, "artifact.local_open": map[string]any{"ok": true, "version": "1.0.0"}, "codex": binaryStatus("codex"), "claude": binaryStatus("claude")}
}

func taskRuntimeResources(run domain.TaskRun) ([]byte, string, error) {
	switch run.TaskType {
	case "script_generate", "script_revise":
		if run.OutputSchema != domain.ScriptPackageSchema {
			return nil, "", domain.Conflict("OUTPUT_SCHEMA_MISMATCH", "剧本任务输出 Schema 与本机能力不匹配")
		}
		return contracts.ScriptPackageSchema, builtinskills.MarketingVideoScript, nil
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

func installUserDaemon() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("user daemon registration is not implemented for %s", runtime.GOOS)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configDir := filepath.Join(home, "Library", "Application Support", "ContentCloud")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.goodvision.contentcloud.plist")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.goodvision.contentcloud</string>
<key>ProgramArguments</key><array><string>` + html.EscapeString(executable) + `</string><string>daemon</string><string>run</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ProcessType</key><string>Background</string>
<key>StandardOutPath</key><string>` + html.EscapeString(filepath.Join(configDir, "daemon.log")) + `</string>
<key>StandardErrorPath</key><string>` + html.EscapeString(filepath.Join(configDir, "daemon-error.log")) + `</string>
</dict></plist>`
	temporary := plistPath + ".tmp"
	if err := os.WriteFile(temporary, []byte(plist), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, plistPath); err != nil {
		return err
	}
	domainName := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domainName+"/com.goodvision.contentcloud").Run()
	if output, err := exec.Command("launchctl", "bootstrap", domainName, plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func uninstallUserDaemon() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("user daemon registration is not implemented for %s", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	domainName := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domainName+"/com.goodvision.contentcloud").Run()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.goodvision.contentcloud.plist")
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func commandSchemas() map[string]any {
	read := func(args []string, output string) map[string]any { return schemaEntry("read", "none", args, output) }
	userRead := func(args []string, output string) map[string]any { return schemaEntry("read", "user", args, output) }
	write := func(auth string, args []string, output string) map[string]any {
		return schemaEntry("write", auth, args, output)
	}
	high := func(args []string, output string) map[string]any {
		return schemaEntry("high-risk-write", "user", append(args, "--yes", "--dry-run"), output)
	}
	return map[string]any{
		"doctor": read([]string{"--offline"}, "diagnostic checks"), "status": read(nil, "local runtime status"), "update": read(nil, "verified installer guidance"),
		"init":             write("connect-key", []string{"directory", "--connect", "--target", "--accept-project-config", "--dry-run"}, "initialized local-first workspace"),
		"workspace.status": read([]string{"directory"}, "local workspace binding, template, and synchronization state"), "workspace.doctor": read([]string{"directory", "--offline"}, "workspace, Skill, MCP, and cloud checks"),
		"mcp.status": read([]string{"directory"}, "project-local MCP installation"), "mcp.serve": read(nil, "stdio MCP server"),
		"publish.knowledge":   write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable knowledge SubmissionRevision"),
		"publish.research":    write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable research SubmissionRevision"),
		"publish.strategy":    write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable strategy SubmissionRevision"),
		"publish.brief":       write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable brief SubmissionRevision"),
		"publish.script":      write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable script SubmissionRevision"),
		"publish.delivery":    write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable delivery SubmissionRevision"),
		"publish.performance": write("workspace", []string{"--file", "--disclosures", "--message", "--idempotency-key", "--review", "--yes", "--dry-run"}, "immutable performance SubmissionRevision"),
		"pull.feedback":       read([]string{"--dry-run"}, "review feedback bundles in local inbox"), "pull.decisions": read([]string{"--dry-run"}, "decision delta in local inbox"), "pull.approved": read([]string{"--type", "--id", "--dry-run"}, "read-only ApprovedSnapshot cache"),
		"submission.list": read(nil, "workspace submission list"), "submission.show": read([]string{"submission-id"}, "submission with immutable revisions"), "submission.status": read([]string{"submission-id"}, "submission governance status"), "submission.approve": high([]string{"revision-id", "--reason"}, "immutable ApprovedSnapshot"),
		"submission.request_changes": high([]string{"revision-id", "--reason", "--json-pointer"}, "immutable change request and review feedback"),
		"up":                         write("connect-key", []string{"--server-url", "--connect-key", "--name"}, "connected device summary"), "down": high(nil, "revoked device and cleared local binding"),
		"auth.login": write("none", []string{"--no-wait", "--device-code"}, "device login state"), "auth.status": read(nil, "user session state"), "auth.logout": write("user", nil, "revoked user session"),
		"context.show": read(nil, "resolved project ID"), "context.use": write("none", []string{"project-id"}, "local context path"), "context.clear": write("none", nil, "cleared local context"),
		"tenant.list": userRead(nil, "tenant list"), "tenant.switch": write("user", []string{"tenant-id", "--dry-run"}, "rotated tenant credential"),
		"membership.list": userRead(nil, "tenant member list"), "membership.invite.list": userRead(nil, "tenant invitation list"), "membership.invite.create": write("user", []string{"email", "--role", "--dry-run"}, "one-time tenant invitation"), "membership.invite.accept": write("user", []string{"invite-token", "--dry-run"}, "accepted membership"), "membership.invite.revoke": high([]string{"invite-id"}, "revoked tenant invitation"), "membership.update": write("user", []string{"user-id", "role", "--dry-run"}, "updated fixed membership role"), "membership.revoke": high([]string{"user-id"}, "revoked membership and tenant sessions"),
		"project.list": userRead(nil, "project list"), "project.show": userRead([]string{"project-id"}, "project"), "project.resolve": userRead([]string{"name-or-slug"}, "stable project ID"), "project.create": write("user", []string{"--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--template", "--dry-run"}, "single-product project"), "project.update": write("user", []string{"project-id", "--row-version", "--brand", "--product", "--channel", "--objective", "--owner", "--reviewer", "--client-approver", "--dry-run"}, "optimistically updated project"), "project.archive": high([]string{"project-id", "--row-version"}, "archived read-only project"), "project.restore": high([]string{"project-id", "--row-version"}, "restored active project"), "project_template.list": userRead(nil, "sanitized project template list"), "project_template.create": write("user", []string{"--name", "--channel", "--objective", "--dry-run"}, "sanitized project template"),
		"device.connect_session.create": write("user", []string{"project-id", "--project", "--dry-run"}, "one-time project connection session"), "device.connect_session.show": userRead([]string{"session-id"}, "project connection session"), "device.connect_session.cancel": high([]string{"session-id"}, "canceled project connection session"),
		"device.list": userRead([]string{"--project"}, "device list"), "device.show": userRead([]string{"device-id"}, "device"), "device.attach": write("user", []string{"device-id", "--project", "--dry-run"}, "project device grant"), "device.detach": high([]string{"device-id", "--project"}, "revoked project device grant"), "device.revoke": high([]string{"device-id"}, "revoked device"),
		"source.list": userRead([]string{"--project"}, "source list"), "source.upload": write("user", []string{"file", "--project", "--name", "--type", "--mime", "--dry-run"}, "source revision"), "source.status": userRead([]string{"revision-id"}, "source revision status"),
		"source.revisions": userRead([]string{"source-id"}, "immutable source revision list"), "source.revise": write("user", []string{"source-id", "file", "--mime", "--dry-run"}, "new immutable source revision"), "source.impact": userRead([]string{"source-id"}, "affected object list"), "evidence.review": write("user", []string{"evidence-id", "decision", "--dry-run"}, "reviewed evidence span"),
		"asset.list": userRead([]string{"--project"}, "governed asset list"), "asset.create": write("user", []string{"--project", "--name", "--type", "--source-revision", "--usage", "--dry-run"}, "governed asset"), "rights.list": userRead([]string{"asset-id"}, "asset rights records"), "rights.create": write("user", []string{"asset-id", "--holder", "--type", "--territory", "--channel", "--proof-source-revision", "--valid-from", "--valid-until", "--restriction", "--dry-run"}, "rights record"), "rights.review": write("user", []string{"rights-id", "decision", "--dry-run"}, "reviewed rights record"),
		"knowledge.list": userRead([]string{"--project"}, "knowledge list"), "knowledge.show": userRead([]string{"knowledge-id"}, "knowledge with evidence"), "knowledge.extract": write("user", []string{"--project", "--source-revision", "--count", "--idempotency-key", "--dry-run"}, "queued local knowledge extraction run"), "knowledge.review": write("user", []string{"id", "decision", "--dry-run"}, "reviewed knowledge"),
		"knowledge.conflicts": userRead([]string{"--project"}, "knowledge conflict list"), "knowledge.decisions": userRead([]string{"--project"}, "decision request list"), "knowledge.decision.resolve": write("user", []string{"decision-request-id", "--select", "--notes", "--dry-run"}, "resolved decision request"),
		"brief.list": userRead([]string{"--project"}, "brief list"), "brief.show": userRead([]string{"brief-id"}, "brief version"), "brief.create": write("user", []string{"--project", "--file", "--dry-run"}, "new immutable brief version"), "brief.revise": write("user", []string{"brief-id", "--project", "--file", "--reason", "--dry-run"}, "replacement immutable brief version"), "brief.submit": write("user", []string{"brief-id", "--dry-run"}, "brief in internal review"), "brief.return": write("user", []string{"brief-id", "--reason", "--dry-run"}, "brief returned for revision"), "brief.approve": write("user", []string{"brief-id", "--dry-run"}, "approved brief"),
		"run.create": write("user", []string{"brief-id", "--idempotency-key"}, "task run"), "run.list": userRead([]string{"--project"}, "run list"), "run.show": userRead([]string{"run-id"}, "task run"), "run.attempts": userRead([]string{"run-id"}, "immutable execution attempt list"), "run.log": userRead([]string{"run-id"}, "sanitized persisted progress"), "run.cancel": high([]string{"run-id"}, "canceled task run"),
		"script.list": userRead([]string{"--project"}, "script list"), "script.show": userRead([]string{"script-id"}, "canonical Script Package"), "script.revise": write("user", []string{"baseline-version-id", "--reason", "--brief", "--invariant", "--changed-field", "--idempotency-key", "--dry-run"}, "script revision run"), "script.variant": write("user", []string{"baseline-version-id", "--reason", "--hypothesis", "--changed-field", "--invariant", "--brief", "--idempotency-key", "--dry-run"}, "single-variable variant run"), "script.review": write("user", []string{"script-id", "decision", "--conclusion", "--assignee", "--dry-run"}, "internal review transition"), "review_cycle.list": userRead([]string{"script-id"}, "immutable review cycles"),
		"artifact.list": userRead([]string{"--script"}, "server-computed artifact presentations"), "artifact.presentation": userRead([]string{"artifact-id"}, "presentation tier and allowed actions"), "artifact.export": write("user", []string{"script-id", "--format"}, "versioned artifact"), "artifact.download": userRead([]string{"artifact-id", "--out"}, "local artifact path"), "artifact.register": write("device", []string{"file", "--script", "--schema", "--media-type", "--metadata", "--dry-run"}, "local-only extension artifact envelope"), "artifact.open": write("user", []string{"artifact-id", "--device", "--dry-run"}, "60-second declarative open request"), "artifact.open.status": userRead([]string{"open-request-id"}, "local-open request state"),
		"review.create": write("user", []string{"script-id", "--email", "--dry-run"}, "one-time customer review link"), "review.list": userRead([]string{"script-id"}, "customer review grants"), "review.revoke": high([]string{"grant-id", "--dry-run"}, "revoked customer review grant"), "review.status": userRead([]string{"script-id"}, "customer review state"),
		"result.list": userRead([]string{"--project"}, "observation list"), "result.import": write("user", []string{"json-or-csv-or-xlsx-file", "--project", "--dry-run"}, "atomic performance import batch"), "result.batches": userRead([]string{"--project"}, "immutable import batch list"), "result.batch-show": userRead([]string{"batch-id"}, "import batch and observations"), "result.rate": write("user", []string{"subject-type", "subject-id", "--project", "--observation", "--rating", "--reason", "--next-action", "--dry-run"}, "manual rating decision"), "result.ratings": userRead([]string{"--project"}, "manual rating decision list"),
		"lineage.show": userRead([]string{"--project", "--type", "--id", "--direction"}, "bidirectional project lineage graph"), "lineage.impact": userRead([]string{"--project", "--type", "--id"}, "affected objects with reasons and actions"), "audit.list": userRead([]string{"--project", "--limit"}, "immutable audit event list"),
		"daemon.run": write("device", []string{"--once", "--fixture", "--adapter"}, "leased run result"), "skills.list": read(nil, "embedded skills"), "skills.read": read([]string{"name", "--path"}, "skill content"), "skills.status": read(nil, "skill version state"), "skills.install": write("none", []string{"name", "--target"}, "local install path"), "schema": read([]string{"command"}, "CLI contract"), "request.get": userRead([]string{"projects|tenants|runs"}, "allowlisted resource"),
	}
}

func schemaEntry(risk, auth string, arguments []string, output string) map[string]any {
	if arguments == nil {
		arguments = []string{}
	}
	return map[string]any{"risk": risk, "auth": auth, "arguments": arguments, "output": output, "supports_json": true}
}
