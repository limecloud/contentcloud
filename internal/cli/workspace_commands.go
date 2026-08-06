package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/capabilityrouting"
	"github.com/limecloud/contentcloud/internal/codexplugin"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	"github.com/limecloud/contentcloud/internal/fixturev3"
	"github.com/limecloud/contentcloud/internal/localworkspace"
	"github.com/limecloud/contentcloud/internal/projectview"
)

func (r *Root) workspaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "workspace", Short: "查看本地优先的 Content Work OS 工作区"}
	conversationContext := &cobra.Command{Use: "conversation-context [directory]", Args: cobra.MaximumNArgs(1), Short: "离线读取跨对话的工作区上下文", RunE: func(cmd *cobra.Command, args []string) error {
		context, err := r.workspaceConversationContext(optionalDirectory(args))
		if err != nil {
			return err
		}
		return r.writeOK("workspace.conversation-context", context)
	}}
	conversationContext.Flags().Bool("offline", true, "只读取已保存的本地状态，不访问云端")
	projectBrief := &cobra.Command{Use: "project-brief", Short: "建立并保存项目简报"}
	var briefDirectory, briefClient, briefBrand, briefProduct, briefObjective, briefAudience, briefNotes string
	var briefChannels, briefMaterialRefs []string
	saveBrief := &cobra.Command{Use: "save", Args: cobra.NoArgs, Short: "确认并保存项目简报到本地工作区", RunE: func(cmd *cobra.Command, args []string) error {
		brief, err := localworkspace.SaveProjectBrief(localworkspace.SaveProjectBriefOptions{
			Root: briefDirectory, Client: briefClient, Brand: briefBrand, ProductOrService: briefProduct,
			Objective: briefObjective, Channels: briefChannels, Audience: briefAudience,
			MaterialRefs: briefMaterialRefs, Notes: briefNotes, Confirm: true, Now: r.currentTime(),
		})
		if err != nil {
			return err
		}
		context, err := r.workspaceConversationContext(briefDirectory)
		if err != nil {
			return err
		}
		return r.writeOK("workspace.project-brief.save", map[string]any{"brief": brief, "onboarding": context.Onboarding, "business_files_modified": true, "offline": true})
	}}
	saveBrief.Flags().StringVar(&briefDirectory, "directory", "", "Content Work OS 工作区路径")
	saveBrief.Flags().StringVar(&briefClient, "client", "", "客户或组织名称")
	saveBrief.Flags().StringVar(&briefBrand, "brand", "", "品牌名称")
	saveBrief.Flags().StringVar(&briefProduct, "product-or-service", "", "产品或服务")
	saveBrief.Flags().StringVar(&briefObjective, "objective", "", "本次内容目标")
	saveBrief.Flags().StringSliceVar(&briefChannels, "channel", nil, "目标渠道，可重复传入")
	saveBrief.Flags().StringVar(&briefAudience, "audience", "", "目标受众")
	saveBrief.Flags().StringSliceVar(&briefMaterialRefs, "material-ref", nil, "已有素材位置，可重复传入")
	saveBrief.Flags().StringVar(&briefNotes, "notes", "", "补充说明")
	for _, flag := range []string{"client", "brand", "product-or-service", "objective", "channel", "audience"} {
		_ = saveBrief.MarkFlagRequired(flag)
	}
	projectBrief.AddCommand(saveBrief)
	cmd.AddCommand(
		&cobra.Command{Use: "status [directory]", Args: cobra.MaximumNArgs(1), Short: "显示绑定、模板、本地改动和拉取状态", RunE: func(cmd *cobra.Command, args []string) error {
			status, err := localworkspace.LoadStatus(optionalDirectory(args))
			if err != nil {
				return err
			}
			return r.writeOK("workspace.status", enrichWorkspaceStatus(status))
		}},
		r.workspaceDoctorCommand(),
		r.workspaceExecutionPlanCommand(),
		r.workspaceEnvironmentPrepareCommand(),
		r.workspaceFixtureCommand(),
		conversationContext,
		projectBrief,
		r.workspaceApprovedCommand(),
	)
	return cmd
}

func (r *Root) workspaceFixtureCommand() *cobra.Command {
	command := &cobra.Command{Use: "fixture", Short: "在新工作区中生成带版本的 V3 验收数据"}
	var directory, projectID, workspaceID, deviceID, serverURL, target string
	apply := &cobra.Command{
		Use:   "apply <fixture.json>",
		Args:  cobra.ExactArgs(1),
		Short: "根据外部测试数据包创建完整的 V3 工作区",
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			fixture, err := fixturev3.Decode(file)
			if err != nil {
				return domain.Invalid("FIXTURE_V3_INVALID", err.Error())
			}
			result, err := localworkspace.MaterializeFixture(fixture, localworkspace.MaterializeFixtureOptions{
				Root: directory, ProjectID: projectID, WorkspaceID: workspaceID, DeviceID: deviceID, ServerURL: serverURL, CLIVersion: Version, Target: target,
			})
			if err != nil {
				return err
			}
			return r.writeOK("workspace.fixture.apply", result)
		},
	}
	apply.Flags().StringVar(&directory, "directory", "", "空的目标目录")
	apply.Flags().StringVar(&projectID, "project-id", "", "要绑定到本地工作区的项目 ID")
	apply.Flags().StringVar(&workspaceID, "workspace-id", "", "要绑定到本地工作区的工作区 ID")
	apply.Flags().StringVar(&deviceID, "device-id", "", "可选的设备 ID")
	apply.Flags().StringVar(&serverURL, "server-url", "", "可选的 Content Work OS 服务端地址")
	apply.Flags().StringVar(&target, "target", "codex", "工作区目标：codex、codex-plugin 或 none")
	_ = apply.MarkFlagRequired("directory")
	_ = apply.MarkFlagRequired("project-id")
	_ = apply.MarkFlagRequired("workspace-id")
	command.AddCommand(apply)
	return command
}

type environmentPreparationInput struct {
	Directory    string
	RunID        string
	Intent       string
	Capabilities []string
	InputRefs    []string
}

func (input *environmentPreparationInput) bindFlags(command *cobra.Command) {
	command.Flags().StringVar(&input.Directory, "directory", "", "Content Work OS 工作区路径")
	command.Flags().StringVar(&input.RunID, "run", "", "本地任务 ID")
	command.Flags().StringVar(&input.Intent, "intent", "", "稳定的执行意图")
	command.Flags().StringSliceVar(&input.Capabilities, "capability", nil, "必需的能力 ID；需要多个能力时可重复传入")
	command.Flags().StringSliceVar(&input.InputRefs, "input", nil, "工作区输入引用；需要多个输入时可重复传入")
	_ = command.MarkFlagRequired("run")
	_ = command.MarkFlagRequired("intent")
	_ = command.MarkFlagRequired("capability")
}

func (r *Root) workspaceEnvironmentPrepareCommand() *cobra.Command {
	command := &cobra.Command{Use: "prepare", Short: "为已验证的本地执行计划准备任务所需能力包"}
	var planInput environmentPreparationInput
	plan := &cobra.Command{Use: "plan", Short: "展示能力包的确切权限、数据流和成本，但不安装", RunE: func(command *cobra.Command, args []string) error {
		_, _, preparation, err := r.resolveEnvironmentPreparation(planInput)
		if err != nil {
			return err
		}
		return r.writeOK("workspace.prepare.plan", preparation)
	}}
	planInput.bindFlags(plan)

	var applyInput environmentPreparationInput
	var preparationID string
	var accept bool
	apply := &cobra.Command{Use: "apply", Short: "只安装已确认的任务能力包，并更新已验证的环境锁", RunE: func(command *cobra.Command, args []string) error {
		result, err := r.applyEnvironmentPreparation(command.Context(), applyInput, preparationID, accept)
		if err != nil {
			return err
		}
		return r.writeOK("workspace.prepare.apply", result)
	}}
	applyInput.bindFlags(apply)
	apply.Flags().StringVar(&preparationID, "preparation-id", "", "workspace prepare plan 返回的确切 epp_ 计划 ID")
	apply.Flags().BoolVar(&accept, "accept", false, "确认所展示的能力包安装、本地锁更新和新对话交接")
	_ = apply.MarkFlagRequired("preparation-id")
	command.AddCommand(plan, apply)
	return command
}

func (r *Root) workspaceExecutionPlanCommand() *cobra.Command {
	var directory, runID, intent string
	var capabilities, inputRefs []string
	command := &cobra.Command{
		Use:   "execution-plan",
		Short: "为一次任务解析已验证的离线本地执行计划（LocalExecutionPlan）",
		RunE: func(command *cobra.Command, args []string) error {
			plan, err := r.resolveLocalExecutionPlan(directory, runID, intent, capabilities, inputRefs)
			if err != nil {
				return err
			}
			return r.writeOK("workspace.execution-plan", plan)
		},
	}
	command.Flags().StringVar(&directory, "directory", "", "Content Work OS 工作区路径")
	command.Flags().StringVar(&runID, "run", "", "本地任务 ID")
	command.Flags().StringVar(&intent, "intent", "", "稳定的执行意图")
	command.Flags().StringSliceVar(&capabilities, "capability", nil, "必需的能力 ID；需要多个能力时可重复传入")
	command.Flags().StringSliceVar(&inputRefs, "input", nil, "工作区输入引用；需要多个输入时可重复传入")
	_ = command.MarkFlagRequired("run")
	_ = command.MarkFlagRequired("intent")
	_ = command.MarkFlagRequired("capability")
	return command
}

func (r *Root) workspaceApprovedCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "approved", Short: "从本地不可变缓存读取已验证的批准快照（ApprovedSnapshot）"}
	var listDirectory, submissionType string
	list := &cobra.Command{Use: "list", Short: "列出本地缓存的批准快照，不访问云端", RunE: func(cmd *cobra.Command, args []string) error {
		items, err := localworkspace.ApprovedSnapshotInbox(listDirectory, submissionType)
		if err != nil {
			return err
		}
		return r.writeOK("workspace.approved.list", map[string]any{"count": len(items), "snapshots": items, "offline": true})
	}}
	list.Flags().StringVar(&listDirectory, "directory", "", "Content Work OS 工作区路径")
	list.Flags().StringVar(&submissionType, "type", "", "可选的提交类型筛选条件")
	var showDirectory string
	show := &cobra.Command{Use: "show <snapshot-id>", Args: cobra.ExactArgs(1), Short: "读取一个已验证的本地批准快照，不访问云端", RunE: func(cmd *cobra.Command, args []string) error {
		record, err := localworkspace.ShowApprovedSnapshot(showDirectory, args[0])
		if err != nil {
			return err
		}
		return r.writeOK("workspace.approved.show", map[string]any{"record": record, "offline": true})
	}}
	show.Flags().StringVar(&showDirectory, "directory", "", "Content Work OS 工作区路径")
	cmd.AddCommand(list, show)
	return cmd
}

func (r *Root) workspaceDoctorCommand() *cobra.Command {
	var offline bool
	cmd := &cobra.Command{Use: "doctor [directory]", Args: cobra.MaximumNArgs(1), Short: "检查工作区结构、技能、MCP 和云端连接", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := r.workspaceDoctor(optionalDirectory(args))
		if err != nil {
			return err
		}
		binding, err := localworkspace.ProjectBinding(report.Root)
		if err != nil {
			return err
		}
		serverCheck := localworkspace.Check{OK: true, Required: false, Message: "云端连接检查已跳过"}
		if !offline {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			err := apiclient.New(binding.ServerURL, "").Health(ctx)
			serverCheck = localworkspace.Check{OK: err == nil, Required: false, Message: defaultValue(errorStringValue(err), "云端可访问")}
		}
		report.Checks["cloud"] = serverCheck
		return r.writeOK("workspace.doctor", report)
	}}
	cmd.Flags().BoolVar(&offline, "offline", false, "跳过云端连接检查")
	return cmd
}

func (r *Root) mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "运行和查看项目本地的 Content Work OS MCP 服务"}
	cmd.AddCommand(
		&cobra.Command{Use: "status [directory]", Args: cobra.MaximumNArgs(1), Short: "显示项目 MCP 的安装状态", RunE: func(cmd *cobra.Command, args []string) error {
			root, err := localworkspace.FindRoot(optionalDirectory(args))
			if err != nil {
				return err
			}
			paths := map[string]string{
				"descriptor": filepath.Join(root, ".contentcloud", "mcp", "contentcloud-local.json"),
				"codex":      filepath.Join(root, ".codex", "config.toml"),
				"claude":     filepath.Join(root, ".mcp.json"),
			}
			installed := map[string]bool{}
			for name, path := range paths {
				info, statErr := os.Stat(path)
				installed[name] = statErr == nil && !info.IsDir()
			}
			return r.writeOK("mcp.status", map[string]any{"name": "contentcloud-local", "root": root, "installed": installed, "transport": "stdio"})
		}},
		&cobra.Command{Use: "serve", Args: cobra.NoArgs, Short: "通过标准输入输出提供项目本地 MCP 工具", RunE: func(cmd *cobra.Command, args []string) error {
			return r.serveMCP(cmd.Context(), cmd.InOrStdin())
		}},
	)
	return cmd
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpProjectViewFocus struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

type mcpProjectViewArguments struct {
	Directory string               `json:"directory,omitempty"`
	View      string               `json:"view"`
	Focus     *mcpProjectViewFocus `json:"focus,omitempty"`
}

type mcpBrowserHandoff struct {
	Required      bool   `json:"required"`
	URL           string `json:"url"`
	PreferredMode string `json:"preferredMode"`
	BrowserAction string `json:"browserAction"`
}

type mcpProjectViewResult struct {
	ProjectID      string             `json:"project_id"`
	View           string             `json:"view"`
	Focus          *projectview.Focus `json:"focus,omitempty"`
	BrowserHandoff mcpBrowserHandoff  `json:"browserHandoff"`
}

func (r *Root) serveMCP(ctx context.Context, input io.Reader) error {
	if strings.TrimSpace(r.mcpCWD) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		r.mcpCWD = cwd
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	encoder := json.NewEncoder(r.stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var request mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if encodeErr := encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "JSON-RPC 请求无效"}}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		if request.Method == "notifications/initialized" {
			continue
		}
		response := r.handleMCPRequest(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (r *Root) handleMCPRequest(ctx context.Context, request mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": requestedMCPProtocolVersion(request.Params),
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
			},
			"serverInfo":   map[string]string{"name": "contentcloud-local", "version": Version},
			"instructions": capabilityrouting.MCPInstructions(),
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, err := r.callLocalMCPTool(ctx, request.Params)
		if err != nil {
			response.Result = mcpToolError(err)
		} else {
			response.Result = result
		}
	case "resources/list":
		response.Result = map[string]any{"resources": contentCloudMCPResources()}
	case "resources/read":
		result, err := r.readContentCloudMCPResource(request.Params)
		if err != nil {
			response.Error = &mcpError{Code: -32001, Message: err.Error()}
		} else {
			response.Result = result
		}
	default:
		response.Error = &mcpError{Code: -32601, Message: "未找到对应方法"}
	}
	return response
}

func mcpTools() []map[string]any {
	directory := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"}},
		"additionalProperties": false,
	}
	readOnlyAnnotations := map[string]any{
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   false,
	}
	workspaceWriteAnnotations := map[string]any{
		"readOnlyHint":    false,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   false,
	}
	projectBrief := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":          map[string]any{"type": "string", "description": "工作区路径；默认使用当前 MCP 工作目录"},
			"client":             map[string]any{"type": "string", "description": "客户或组织名称"},
			"brand":              map[string]any{"type": "string", "description": "品牌名称"},
			"product_or_service": map[string]any{"type": "string", "description": "产品或服务"},
			"objective":          map[string]any{"type": "string", "description": "本次内容目标"},
			"channels":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "description": "目标内容渠道"},
			"audience":           map[string]any{"type": "string", "description": "目标受众"},
			"material_refs":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "已有素材的本地位置，可选"},
			"notes":              map[string]any{"type": "string", "description": "补充说明，可选"},
			"confirm":            map[string]any{"type": "boolean", "const": true, "description": "确认已经核对以上项目简报"},
		},
		"required":             []string{"client", "brand", "product_or_service", "objective", "channels", "audience", "confirm"},
		"additionalProperties": false,
	}
	cloudReadAnnotations := map[string]any{
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   true,
	}
	cloudWriteAnnotations := map[string]any{
		"readOnlyHint":    false,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   true,
	}
	openProjectView := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"view":      map[string]any{"type": "string", "enum": projectview.IDs()},
			"focus": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":   map[string]any{"type": "string", "pattern": "^[a-z][a-z0-9_]{0,63}$"},
					"id":     map[string]any{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$"},
					"digest": map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"},
				},
				"required":             []string{"kind", "id"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"view"},
		"additionalProperties": false,
	}
	environmentWriteAnnotations := map[string]any{
		"readOnlyHint":    false,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   true,
	}
	preflight := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":        map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"submission_type":  map[string]any{"type": "string", "enum": []string{"context", "knowledge", "brief", "content_batch", "asset_batch", "delivery", "result"}},
			"files":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "相对于工作区的 JSON 检查点文件；默认使用该类型的输出目录"},
			"disclosures_file": map[string]any{"type": "string", "description": "相对于工作区的来源披露 JSON 文件"},
			"message":          map[string]any{"type": "string", "description": "会纳入计算摘要的审核说明"},
			"idempotency_key":  map[string]any{"type": "string", "description": "会纳入发布计划的稳定重试键"},
		},
		"required":             []string{"submission_type"},
		"additionalProperties": false,
	}
	publishApply := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":        map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"submission_type":  map[string]any{"type": "string", "enum": []string{"context", "knowledge", "brief", "content_batch", "asset_batch", "delivery", "result"}},
			"files":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"disclosures_file": map[string]any{"type": "string"},
			"message":          map[string]any{"type": "string"},
			"idempotency_key":  map[string]any{"type": "string"},
			"plan_id":          map[string]any{"type": "string", "pattern": "^pp_[a-f0-9]{64}$"},
			"accept":           map[string]any{"type": "boolean", "const": true},
		},
		"required":             []string{"submission_type", "plan_id", "accept"},
		"additionalProperties": false,
	}
	submissionStatus := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":     map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"submission_id": map[string]any{"type": "string"},
		},
		"required":             []string{"submission_id"},
		"additionalProperties": false,
	}
	snapshots := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":       map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"submission_type": map[string]any{"type": "string", "enum": []string{"context", "knowledge", "brief", "content_batch", "asset_batch", "delivery", "result"}},
		},
		"additionalProperties": false,
	}
	snapshotPull := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":       map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"submission_type": map[string]any{"type": "string", "enum": []string{"context", "knowledge", "brief", "content_batch", "asset_batch", "delivery", "result"}},
			"snapshot_id":     map[string]any{"type": "string", "description": "拉取一个指定快照；省略时拉取筛选后的工作区列表"},
		},
		"additionalProperties": false,
	}
	snapshotShow := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":   map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"snapshot_id": map[string]any{"type": "string"},
		},
		"required":             []string{"snapshot_id"},
		"additionalProperties": false,
	}
	localExecutionPlan := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":             map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":                map[string]any{"type": "string"},
			"intent":                map[string]any{"type": "string"},
			"required_capabilities": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"input_refs":            map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"run_id", "intent", "required_capabilities"},
		"additionalProperties": false,
	}
	environmentPreparationApply := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":             map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":                map[string]any{"type": "string"},
			"intent":                map[string]any{"type": "string"},
			"required_capabilities": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"input_refs":            map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"preparation_id":        map[string]any{"type": "string", "pattern": "^epp_[a-f0-9]{64}$"},
			"accept":                map[string]any{"type": "boolean", "const": true},
		},
		"required":             []string{"run_id", "intent", "required_capabilities", "preparation_id", "accept"},
		"additionalProperties": false,
	}
	sourceRegister := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":    map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"file":         map[string]any{"type": "string", "description": "来源文件路径"},
			"id":           map[string]any{"type": "string", "description": "稳定的来源 ID"},
			"title":        map[string]any{"type": "string"},
			"source_kind":  map[string]any{"type": "string"},
			"storage_mode": map[string]any{"type": "string", "enum": []string{"copy", "reference"}},
		},
		"required":             []string{"file"},
		"additionalProperties": false,
	}
	sourceID := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"source_id": map[string]any{"type": "string"},
		},
		"required":             []string{"source_id"},
		"additionalProperties": false,
	}
	localRunInit := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":   map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"run_id":      map[string]any{"type": "string"},
			"intent":      map[string]any{"type": "string", "pattern": "^intent:[A-Za-z0-9._-]+$"},
			"input_ids":   map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"with_ingest": map[string]any{"type": "boolean"},
		},
		"required":             []string{"intent"},
		"additionalProperties": false,
	}
	localRunShow := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"run_id":    map[string]any{"type": "string", "description": "默认使用当前任务"},
		},
		"additionalProperties": false,
	}
	localRunClaim := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"owner":             map[string]any{"type": "string", "description": "稳定的对话或工作进程持有者 ID"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"ttl_seconds":       map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
			"takeover_expired":  map[string]any{"type": "boolean"},
		},
		"required":             []string{"run_id", "owner", "expected_revision"},
		"additionalProperties": false,
	}
	localRunClaimControl := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":   map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":      map[string]any{"type": "string"},
			"claim_token": map[string]any{"type": "string"},
			"ttl_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
		},
		"required":             []string{"run_id", "claim_token"},
		"additionalProperties": false,
	}
	handoffCreate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":          map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"handoff_id":         map[string]any{"type": "string"},
			"run_id":             map[string]any{"type": "string"},
			"claim_token":        map[string]any{"type": "string"},
			"expected_revision":  map[string]any{"type": "integer", "minimum": 1},
			"next_capability_id": map[string]any{"type": "string"},
			"next_action":        map[string]any{"type": "string"},
			"input_paths":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1},
			"blockers":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"pending_decisions":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision", "next_capability_id", "next_action", "input_paths"},
		"additionalProperties": false,
	}
	handoffAccept := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":        map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"handoff_id":       map[string]any{"type": "string"},
			"owner":            map[string]any{"type": "string"},
			"ttl_seconds":      map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
			"takeover_expired": map[string]any{"type": "boolean"},
		},
		"required":             []string{"handoff_id", "owner"},
		"additionalProperties": false,
	}
	handoffControl := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":   map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"handoff_id":  map[string]any{"type": "string"},
			"claim_token": map[string]any{"type": "string"},
		},
		"required":             []string{"handoff_id"},
		"additionalProperties": false,
	}
	knowledgeImport := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":  map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"file":       map[string]any{"type": "string", "description": "相对于工作区的知识候选文件"},
			"origin_run": map[string]any{"type": "string"},
		},
		"required":             []string{"file"},
		"additionalProperties": false,
	}
	knowledgeQuery := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"channel":   map[string]any{"type": "string"},
			"at":        map[string]any{"type": "string", "format": "date-time"},
		},
		"additionalProperties": false,
	}
	knowledgePack := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"pack_id":   map[string]any{"type": "string"},
			"name":      map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	}
	localFile := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"file":      map[string]any{"type": "string", "description": "相对于工作区的 JSON 文件"},
		},
		"required":             []string{"file"},
		"additionalProperties": false,
	}
	creativeBatchInit := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":             map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"brief_id":              map[string]any{"type": "string"},
			"directions_file":       map[string]any{"type": "string"},
			"requested_count":       map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"variant_dimension":     map[string]any{"type": "string", "enum": []string{"hook", "audience", "scenario", "visualization", "cta", "duration"}},
			"controlled_dimensions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"batch_id":              map[string]any{"type": "string"},
		},
		"required":             []string{"directions_file", "requested_count", "variant_dimension"},
		"additionalProperties": false,
	}
	articleBatchCreate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":       map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"brief_id":        map[string]any{"type": "string"},
			"requested_count": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"batch_id":        map[string]any{"type": "string"},
		},
		"required":             []string{"requested_count"},
		"additionalProperties": false,
	}
	contentItemLint := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":  map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"file":       map[string]any{"type": "string", "description": "相对于工作区的内容候选文件"},
			"batch_file": map[string]any{"type": "string", "description": "相对于工作区的内容批次清单"},
		},
		"required":             []string{"file"},
		"additionalProperties": false,
	}
	contentBatchFiles := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":     map[string]any{"type": "string", "description": "工作区路径；默认为当前目录"},
			"batch_file":    map[string]any{"type": "string"},
			"content_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"batch_file", "content_files"},
		"additionalProperties": false,
	}
	contentItemDiff := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":      map[string]any{"type": "string", "description": "工作区路径；默认为当前目录"},
			"baseline_file":  map[string]any{"type": "string"},
			"candidate_file": map[string]any{"type": "string"},
			"allowed_paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"baseline_file", "candidate_file", "allowed_paths"},
		"additionalProperties": false,
	}
	contentDeliveryExport := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":        map[string]any{"type": "string", "description": "工作区路径；默认为当前目录"},
			"content_item_id":  map[string]any{"type": "string"},
			"output_directory": map[string]any{"type": "string"},
		},
		"required":             []string{"content_item_id"},
		"additionalProperties": false,
	}
	return []map[string]any{
		{
			"name":        "contentcloud_open_project_view",
			"description": "为当前项目生成可信的 Content Work OS 工作台链接，不打开浏览器，也不修改本地或云端状态",
			"inputSchema": openProjectView,
			"annotations": cloudReadAnnotations,
			"_meta": map[string]any{"contentcloud/effect": map[string]any{
				"effect_scope": "cloud_read_navigation", "requires_confirmation": false, "writes_workspace": false, "writes_cloud": false,
			}},
		},
		{"name": "workspace_context", "description": "读取跨对话保存的 Content Work OS 工作区状态，不访问云端，也不声明任何写入", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "workspace_project_brief", "description": "确认并保存项目简报，建立素材、知识和内容生产共用的本地业务上下文", "inputSchema": projectBrief, "annotations": workspaceWriteAnnotations},
		{"name": "environment_execution_plan", "description": "离线解析已签名的本地执行计划并准确报告缺少的任务能力包，不执行安装", "inputSchema": localExecutionPlan, "annotations": readOnlyAnnotations},
		{"name": "environment_prepare_plan", "description": "披露缺少能力包的准确权限、数据流、费用和会话影响，不执行安装", "inputSchema": localExecutionPlan, "annotations": readOnlyAnnotations},
		{"name": "environment_prepare_apply", "description": "只安装用户准确确认的能力包计划，原子更新环境锁，重新执行诊断并返回新对话交接", "inputSchema": environmentPreparationApply, "annotations": environmentWriteAnnotations},
		{"name": "local_run_claim", "description": "取得指定本地运行上下文版本的单写入者锁", "inputSchema": localRunClaim},
		{"name": "local_run_renew", "description": "续期有效的本地运行锁", "inputSchema": localRunClaimControl},
		{"name": "local_run_release", "description": "释放有效的本地运行锁", "inputSchema": localRunClaimControl},
		{"name": "handoff_create_ready", "description": "根据已锁定运行创建经过摘要校验的待接手交接，并释放运行锁", "inputSchema": handoffCreate},
		{"name": "handoff_list_ready", "description": "列出待接手的跨对话交接记录，但不锁定", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "handoff_accept", "description": "原子校验待接手交接并取得对应运行锁", "inputSchema": handoffAccept},
		{"name": "handoff_complete", "description": "将已接手交接记录标记为完成", "inputSchema": handoffControl},
		{"name": "handoff_supersede", "description": "取代一条待接手交接记录", "inputSchema": handoffControl},
		{"name": "workspace_status", "description": "读取本地工作区绑定、模板和同步状态，不访问云端", "inputSchema": directory},
		{"name": "workspace_doctor", "description": "校验本地工作区结构、受管文件、技能和 MCP 配置", "inputSchema": directory},
		{"name": "source_register", "description": "登记不可变的本地客户来源，不上传文件", "inputSchema": sourceRegister},
		{"name": "source_list", "description": "列出本地来源目录记录，不访问云端", "inputSchema": directory},
		{"name": "source_ingest", "description": "把已登记来源解析为可精确定位的本地证据片段", "inputSchema": sourceID},
		{"name": "source_verify", "description": "校验本地来源摘要和 MIME 类型", "inputSchema": directory},
		{"name": "local_run_init", "description": "初始化可恢复的本地导入、查询或内容工作流", "inputSchema": localRunInit},
		{"name": "local_run_show", "description": "读取本地运行上下文，不访问云端", "inputSchema": localRunShow},
		{"name": "knowledge_import", "description": "把有证据依据的候选导入作为事实源的 Markdown 知识页", "inputSchema": knowledgeImport},
		{"name": "knowledge_lint", "description": "运行确定性的本地知识治理检查", "inputSchema": directory},
		{"name": "knowledge_query", "description": "把知识查询结果分为可用、已阻断和仅供参考", "inputSchema": knowledgeQuery},
		{"name": "knowledge_diagnose", "description": "生成 15 个维度的客户素材诊断", "inputSchema": knowledgeQuery},
		{"name": "knowledge_pack", "description": "构建七层知识审核包和证据披露", "inputSchema": knowledgePack},
		{"name": "brief_lint", "description": "根据当前可用知识校验本地创作简报", "inputSchema": localFile},
		{"name": "content_batch_init", "description": "把已批准的创作简报和知识快照冻结到本地内容批次中", "inputSchema": creativeBatchInit},
		{"name": "content_item_lint", "description": "根据已冻结的内容批次上下文校验一个内容候选", "inputSchema": contentItemLint},
		{"name": "content_batch_lint", "description": "校验内容批次中的每个候选项", "inputSchema": contentBatchFiles},
		{"name": "content_batch_finalize", "description": "定稿已经通过校验的本地内容批次，不创建云端任务运行", "inputSchema": contentBatchFiles},
		{"name": "content_item_diff", "description": "检查内容版本或变量中未声明的 JSON Pointer 变化", "inputSchema": contentItemDiff},
		{"name": "delivery_export", "description": "把已经拉取的批准内容项导出为 JSON、Markdown 和 XLSX", "inputSchema": contentDeliveryExport},
		{"name": "article_brief_lint", "description": "根据可用知识和租户能力校验微信公众号文章简报", "inputSchema": localFile},
		{"name": "article_batch_create", "description": "把已批准的文章简报和知识快照冻结到微信公众号内容批次中", "inputSchema": articleBatchCreate},
		{"name": "article_item_lint", "description": "校验文章内容块、陈述、证据、权利和渠道限制", "inputSchema": contentItemLint},
		{"name": "article_batch_lint", "description": "校验微信公众号内容批次中的每个文章内容项", "inputSchema": contentBatchFiles},
		{"name": "article_batch_finalize", "description": "定稿已经通过校验的微信公众号文章批次", "inputSchema": contentBatchFiles},
		{"name": "article_item_diff", "description": "检查文章内容项版本中未声明的 JSON Pointer 变化", "inputSchema": contentItemDiff},
		{"name": "wechat_package_export", "description": "把已批准文章内容项导出为可直接交给运营人员使用的微信公众号交付包", "inputSchema": contentDeliveryExport},
		{"name": "wechat_package_lint", "description": "校验微信公众号交付包文件和摘要，不向外部平台发布", "inputSchema": localFile},
		{"name": "publish_preflight", "description": "校验本地不可变检查点，返回准确的 plan_id、环境摘要、披露范围和云端影响，但不发布", "inputSchema": preflight, "annotations": readOnlyAnnotations},
		{"name": "publish_apply", "description": "仅把用户准确确认的预检计划发布为不可变提交版本", "inputSchema": publishApply, "annotations": cloudWriteAnnotations},
		{"name": "submission_status", "description": "读取工作区提交记录的当前云端治理状态", "inputSchema": submissionStatus, "annotations": cloudReadAnnotations},
		{"name": "review_feedback_list", "description": "读取当前云端审核反馈，不修改本地文件", "inputSchema": directory, "annotations": cloudReadAnnotations},
		{"name": "review_feedback_pull", "description": "把当前云端审核反馈拉取到不可变本地收件箱", "inputSchema": directory, "annotations": cloudWriteAnnotations},
		{"name": "review_feedback_inbox", "description": "读取本地收件箱中已保存的审核反馈，不访问云端", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "approved_snapshot_list", "description": "读取当前云端批准快照列表，不修改本地文件", "inputSchema": snapshots, "annotations": cloudReadAnnotations},
		{"name": "approved_snapshot_pull", "description": "把云端批准快照拉取到经过校验的不可变本地缓存", "inputSchema": snapshotPull, "annotations": cloudWriteAnnotations},
		{"name": "approved_snapshot_inbox", "description": "列出经过校验的本地缓存批准快照，不访问云端", "inputSchema": snapshots, "annotations": readOnlyAnnotations},
		{"name": "approved_snapshot_show", "description": "读取一份经过校验的本地批准快照，不访问云端", "inputSchema": snapshotShow, "annotations": readOnlyAnnotations},
	}
}

func (r *Root) callLocalMCPTool(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var params struct {
		Name      string `json:"name"`
		Arguments struct {
			Directory            string               `json:"directory"`
			Client               string               `json:"client"`
			Brand                string               `json:"brand"`
			ProductOrService     string               `json:"product_or_service"`
			Objective            string               `json:"objective"`
			Channels             []string             `json:"channels"`
			Audience             string               `json:"audience"`
			MaterialRefs         []string             `json:"material_refs"`
			Notes                string               `json:"notes"`
			Confirm              bool                 `json:"confirm"`
			File                 string               `json:"file"`
			ID                   string               `json:"id"`
			Title                string               `json:"title"`
			SourceKind           string               `json:"source_kind"`
			StorageMode          string               `json:"storage_mode"`
			SourceID             string               `json:"source_id"`
			RunID                string               `json:"run_id"`
			Owner                string               `json:"owner"`
			ClaimToken           string               `json:"claim_token"`
			ExpectedRevision     uint64               `json:"expected_revision"`
			TTLSeconds           int64                `json:"ttl_seconds"`
			TakeoverExpired      bool                 `json:"takeover_expired"`
			HandoffID            string               `json:"handoff_id"`
			NextCapabilityID     string               `json:"next_capability_id"`
			NextAction           string               `json:"next_action"`
			InputPaths           []string             `json:"input_paths"`
			RequiredCapabilities []string             `json:"required_capabilities"`
			InputRefs            []string             `json:"input_refs"`
			Blockers             []string             `json:"blockers"`
			PendingDecisions     []string             `json:"pending_decisions"`
			Intent               string               `json:"intent"`
			InputIDs             []string             `json:"input_ids"`
			WithIngest           bool                 `json:"with_ingest"`
			OriginRun            string               `json:"origin_run"`
			Channel              string               `json:"channel"`
			At                   string               `json:"at"`
			PackID               string               `json:"pack_id"`
			Name                 string               `json:"name"`
			BriefID              string               `json:"brief_id"`
			DirectionsFile       string               `json:"directions_file"`
			RequestedCount       int                  `json:"requested_count"`
			VariantDimension     string               `json:"variant_dimension"`
			ControlledDimensions []string             `json:"controlled_dimensions"`
			BatchID              string               `json:"batch_id"`
			BatchFile            string               `json:"batch_file"`
			ContentFiles         []string             `json:"content_files"`
			BaselineFile         string               `json:"baseline_file"`
			CandidateFile        string               `json:"candidate_file"`
			AllowedPaths         []string             `json:"allowed_paths"`
			ContentItemID        string               `json:"content_item_id"`
			OutputDirectory      string               `json:"output_directory"`
			SubmissionType       string               `json:"submission_type"`
			SubmissionID         string               `json:"submission_id"`
			SnapshotID           string               `json:"snapshot_id"`
			Files                []string             `json:"files"`
			DisclosuresFile      string               `json:"disclosures_file"`
			Message              string               `json:"message"`
			IdempotencyKey       string               `json:"idempotency_key"`
			PlanID               string               `json:"plan_id"`
			PreparationID        string               `json:"preparation_id"`
			Accept               bool                 `json:"accept"`
			View                 string               `json:"view"`
			Focus                *mcpProjectViewFocus `json:"focus"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, domain.Invalid("MCP_PARAMS_INVALID", "MCP 工具参数无效")
	}
	var value any
	var err error
	var navigationRoot string
	var navigationTarget *projectview.Target
	switch params.Name {
	case "contentcloud_open_project_view":
		arguments, decodeErr := decodeOpenProjectViewArguments(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			workspaceErr := domain.Conflict("WORKSPACE_NOT_BOUND", "无法根据 directory 或 MCP 当前工作目录解析 Content Work OS 工作区绑定")
			workspaceErr.Hint = "在 Content Work OS 工作区中打开新对话，或显式传入工作区路径"
			return nil, workspaceErr
		}
		status, loadErr := localworkspace.LoadStatus(root)
		if loadErr != nil {
			return nil, loadErr
		}
		var focus *projectview.Focus
		if arguments.Focus != nil {
			focus = &projectview.Focus{Kind: arguments.Focus.Kind, ID: arguments.Focus.ID, Digest: arguments.Focus.Digest}
		}
		link, buildErr := projectview.Build(status.Binding.ServerURL, status.Binding.ProjectID, projectview.Target{View: arguments.View, Focus: focus})
		if buildErr != nil {
			return nil, buildErr
		}
		return openProjectViewEnvelope(link), nil
	case "workspace_context":
		value, err = r.workspaceConversationContext(params.Arguments.Directory)
	case "workspace_project_brief":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		brief, saveErr := localworkspace.SaveProjectBrief(localworkspace.SaveProjectBriefOptions{
			Root: root, Client: params.Arguments.Client, Brand: params.Arguments.Brand,
			ProductOrService: params.Arguments.ProductOrService, Objective: params.Arguments.Objective,
			Channels: params.Arguments.Channels, Audience: params.Arguments.Audience,
			MaterialRefs: params.Arguments.MaterialRefs, Notes: params.Arguments.Notes,
			Confirm: params.Arguments.Confirm, Now: r.currentTime(),
		})
		err = saveErr
		if err == nil {
			context, contextErr := r.workspaceConversationContext(root)
			if contextErr != nil {
				err = contextErr
			} else {
				value = map[string]any{"brief": brief, "onboarding": context.Onboarding, "business_files_modified": true, "offline": true}
			}
		}
	case "environment_execution_plan":
		value, err = r.resolveLocalExecutionPlan(params.Arguments.Directory, params.Arguments.RunID, params.Arguments.Intent, params.Arguments.RequiredCapabilities, params.Arguments.InputRefs)
	case "environment_prepare_plan":
		var preparation environment.PreparationPlan
		_, _, preparation, err = r.resolveEnvironmentPreparation(environmentPreparationInput{Directory: params.Arguments.Directory, RunID: params.Arguments.RunID, Intent: params.Arguments.Intent, Capabilities: params.Arguments.RequiredCapabilities, InputRefs: params.Arguments.InputRefs})
		value = preparation
	case "environment_prepare_apply":
		value, err = r.applyEnvironmentPreparation(ctx, environmentPreparationInput{Directory: params.Arguments.Directory, RunID: params.Arguments.RunID, Intent: params.Arguments.Intent, Capabilities: params.Arguments.RequiredCapabilities, InputRefs: params.Arguments.InputRefs}, params.Arguments.PreparationID, params.Arguments.Accept)
	case "workspace_status":
		var root string
		root, err = r.resolveMCPWorkspace(params.Arguments.Directory)
		if err == nil {
			var status localworkspace.Status
			status, err = localworkspace.LoadStatus(root)
			value = status
			navigationRoot = root
			navigationTarget = &projectview.Target{View: "overview"}
		}
	case "workspace_doctor":
		var root string
		root, err = r.resolveMCPWorkspace(params.Arguments.Directory)
		if err == nil {
			var report localworkspace.DoctorReport
			report, err = r.workspaceDoctor(root)
			value = report
			navigationRoot = root
			navigationTarget = &projectview.Target{View: "setup"}
		}
	case "source_register":
		if strings.TrimSpace(params.Arguments.File) == "" {
			return nil, domain.Invalid("LOCAL_SOURCE_FILE_REQUIRED", "file 参数必填")
		}
		value, err = localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: params.Arguments.Directory, File: params.Arguments.File, ID: params.Arguments.ID, Title: params.Arguments.Title, SourceKind: params.Arguments.SourceKind, StorageMode: params.Arguments.StorageMode, Now: time.Now()})
	case "source_list":
		var sources []localworkspace.LocalSource
		sources, err = localworkspace.LocalSources(params.Arguments.Directory)
		value = map[string]any{"count": len(sources), "sources": sources}
	case "source_ingest":
		if strings.TrimSpace(params.Arguments.SourceID) == "" {
			return nil, domain.Invalid("LOCAL_SOURCE_ID_REQUIRED", "source_id 参数必填")
		}
		value, err = localworkspace.IngestLocalSource(params.Arguments.Directory, params.Arguments.SourceID, time.Now())
	case "source_verify":
		var report localworkspace.SourceVerification
		report, err = localworkspace.VerifyLocalSources(params.Arguments.Directory)
		value = report
		if err == nil && !report.Valid {
			err = domain.Invalid("LOCAL_SOURCE_VERIFY_FAILED", "本地来源完整性校验失败")
		}
	case "local_run_init":
		value, err = localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: params.Arguments.Directory, RunID: params.Arguments.RunID, Intent: params.Arguments.Intent, InputIDs: params.Arguments.InputIDs, WithIngest: params.Arguments.WithIngest, Now: time.Now()})
	case "local_run_show":
		value, err = localworkspace.ShowLocalRun(params.Arguments.Directory, params.Arguments.RunID)
	case "local_run_claim":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ClaimRun(localworkspace.ClaimRunOptions{Root: root, RunID: params.Arguments.RunID, Owner: params.Arguments.Owner, ExpectedRevision: params.Arguments.ExpectedRevision, TTL: secondsDuration(params.Arguments.TTLSeconds), TakeoverExpired: params.Arguments.TakeoverExpired, Now: r.currentTime()})
	case "local_run_renew":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.RenewRunClaim(root, params.Arguments.RunID, params.Arguments.ClaimToken, secondsDuration(params.Arguments.TTLSeconds), r.currentTime())
	case "local_run_release":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		err = localworkspace.ReleaseRunClaim(root, params.Arguments.RunID, params.Arguments.ClaimToken, r.currentTime())
		value = map[string]any{"run_id": params.Arguments.RunID, "released": err == nil}
	case "handoff_create_ready":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.CreateReadyHandoff(localworkspace.CreateReadyHandoffOptions{Root: root, HandoffID: params.Arguments.HandoffID, RunID: params.Arguments.RunID, ClaimToken: params.Arguments.ClaimToken, ExpectedRevision: params.Arguments.ExpectedRevision, NextCapabilityID: params.Arguments.NextCapabilityID, NextAction: params.Arguments.NextAction, InputPaths: params.Arguments.InputPaths, Blockers: params.Arguments.Blockers, PendingDecisions: params.Arguments.PendingDecisions, Now: r.currentTime()})
	case "handoff_list_ready":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var handoffs []localworkspace.HandoffRecord
		handoffs, err = localworkspace.ListReadyHandoffs(root)
		value = map[string]any{"count": len(handoffs), "handoffs": handoffs}
	case "handoff_accept":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var handoff localworkspace.HandoffRecord
		var claim localworkspace.RunClaim
		handoff, claim, err = localworkspace.AcceptHandoff(localworkspace.AcceptHandoffOptions{Root: root, HandoffID: params.Arguments.HandoffID, Owner: params.Arguments.Owner, TTL: secondsDuration(params.Arguments.TTLSeconds), TakeoverExpired: params.Arguments.TakeoverExpired, Now: r.currentTime()})
		value = map[string]any{"handoff": handoff, "claim": claim}
	case "handoff_complete":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.CompleteHandoff(root, params.Arguments.HandoffID, params.Arguments.ClaimToken, r.currentTime())
	case "handoff_supersede":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.SupersedeReadyHandoff(root, params.Arguments.HandoffID, r.currentTime())
	case "knowledge_import":
		if strings.TrimSpace(params.Arguments.File) == "" {
			return nil, domain.Invalid("LOCAL_FILE_REQUIRED", "file 参数必填")
		}
		value, err = localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: params.Arguments.Directory, PackageFile: params.Arguments.File, OriginRunID: params.Arguments.OriginRun, Now: time.Now()})
	case "knowledge_lint":
		var report localworkspace.KnowledgeLintReport
		report, err = localworkspace.LintKnowledge(params.Arguments.Directory)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("KNOWLEDGE_LINT_FAILED", "知识库确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "knowledge_query", "knowledge_diagnose":
		var at time.Time
		at, err = parseLocalQueryTime(params.Arguments.At)
		if err != nil {
			break
		}
		if params.Name == "knowledge_query" {
			value, err = localworkspace.QueryKnowledge(localworkspace.QueryKnowledgeOptions{Root: params.Arguments.Directory, Channel: params.Arguments.Channel, At: at})
		} else {
			value, err = localworkspace.DiagnoseKnowledge(params.Arguments.Directory, params.Arguments.Channel, at)
		}
	case "knowledge_pack":
		value, err = localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: params.Arguments.Directory, PackID: params.Arguments.PackID, Name: params.Arguments.Name, Now: time.Now()})
	case "brief_lint":
		var report localworkspace.KnowledgeLintReport
		var brief localworkspace.LocalBrief
		report, brief, err = localworkspace.LintBrief(params.Arguments.Directory, params.Arguments.File)
		value = map[string]any{"brief": brief, "report": report}
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("BRIEF_LINT_FAILED", "V3 创作简报确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_init":
		value, err = localworkspace.CreateContentBatch(localworkspace.CreateContentBatchOptions{Root: params.Arguments.Directory, BriefID: params.Arguments.BriefID, DirectionsFile: params.Arguments.DirectionsFile, RequestedCount: params.Arguments.RequestedCount, VariantDimension: params.Arguments.VariantDimension, ControlledDimensions: params.Arguments.ControlledDimensions, BatchID: params.Arguments.BatchID, Now: time.Now()})
	case "content_item_lint":
		var report localworkspace.ContentItemLintReport
		report, _, err = localworkspace.LintContentItem(params.Arguments.Directory, params.Arguments.File, params.Arguments.BatchFile)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("CONTENT_ITEM_LINT_FAILED", "内容项确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_lint":
		var report localworkspace.ContentBatchLintReport
		report, err = localworkspace.LintContentBatch(params.Arguments.Directory, params.Arguments.BatchFile, params.Arguments.ContentFiles)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("CONTENT_BATCH_LINT_FAILED", "内容批次确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_finalize":
		value, err = localworkspace.FinalizeContentBatch(params.Arguments.Directory, params.Arguments.BatchFile, params.Arguments.ContentFiles, time.Now())
	case "content_item_diff":
		var diff localworkspace.ContentItemDiff
		diff, err = localworkspace.DiffContentItems(params.Arguments.Directory, params.Arguments.BaselineFile, params.Arguments.CandidateFile, params.Arguments.AllowedPaths)
		value = diff
		if err == nil && !diff.Valid {
			diffErr := domain.Invalid("CONTENT_ITEM_REVISION_DRIFT", "内容项修订包含未声明字段变化")
			diffErr.Details = diff
			err = diffErr
		}
	case "delivery_export":
		value, err = localworkspace.ExportApprovedContentItem(params.Arguments.Directory, params.Arguments.ContentItemID, params.Arguments.OutputDirectory, time.Now())
	case "article_brief_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleBriefLintReport
		var brief localworkspace.ArticleBrief
		report, brief, err = localworkspace.LintArticleBrief(root, params.Arguments.File)
		value = map[string]any{"brief": brief, "report": report}
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("ARTICLE_BRIEF_LINT_FAILED", "文章简报确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_create":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err == nil {
			value, err = localworkspace.CreateArticleBatch(localworkspace.CreateArticleBatchOptions{Root: root, BriefID: params.Arguments.BriefID, RequestedCount: params.Arguments.RequestedCount, BatchID: params.Arguments.BatchID, Now: r.currentTime()})
		}
	case "article_item_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleItemLintReport
		report, _, err = localworkspace.LintArticleItem(root, params.Arguments.File, params.Arguments.BatchFile)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("ARTICLE_ITEM_LINT_FAILED", "文章内容项确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleBatchLintReport
		report, err = localworkspace.LintArticleBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("ARTICLE_BATCH_LINT_FAILED", "公众号文章批次校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_finalize":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var batch localworkspace.ContentBatch
		var report localworkspace.ArticleBatchLintReport
		batch, report, err = localworkspace.FinalizeArticleBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles, r.currentTime())
		value = map[string]any{"batch": batch, "report": report}
	case "article_item_diff":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var diff localworkspace.ArticleItemDiff
		diff, err = localworkspace.DiffArticleItems(root, params.Arguments.BaselineFile, params.Arguments.CandidateFile, params.Arguments.AllowedPaths)
		value = diff
		if err == nil && !diff.Valid {
			diffErr := domain.Invalid("ARTICLE_ITEM_REVISION_DRIFT", "文章内容项修订包含未声明字段变化")
			diffErr.Details = diff
			err = diffErr
		}
	case "wechat_package_export":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err == nil {
			value, err = localworkspace.ExportWeChatPackage(root, params.Arguments.ContentItemID, params.Arguments.OutputDirectory, r.currentTime())
		}
	case "wechat_package_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, domain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.WeChatPackageLintReport
		report, err = localworkspace.LintWeChatPackage(root, params.Arguments.File)
		value = report
		if err == nil && !report.Valid {
			lintErr := domain.Invalid("WECHAT_PACKAGE_LINT_FAILED", "公众号交付包校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "publish_preflight":
		if !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, domain.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, preflight, buildErr := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: params.Arguments.SubmissionType, Files: params.Arguments.Files, DisclosuresFile: params.Arguments.DisclosuresFile, Message: params.Arguments.Message, IdempotencyKey: params.Arguments.IdempotencyKey})
		value, err = map[string]any{"preflight": preflight, "cloud_write": false, "business_files_modified": false}, buildErr
	case "publish_apply":
		if !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, domain.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var bundle domain.SubmissionBundle
		var preflight publishPreflight
		bundle, preflight, err = buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: params.Arguments.SubmissionType, Files: params.Arguments.Files, DisclosuresFile: params.Arguments.DisclosuresFile, Message: params.Arguments.Message, IdempotencyKey: params.Arguments.IdempotencyKey})
		if err == nil {
			var revision domain.SubmissionRevision
			revision, err = r.applyPublishCheckpoint(ctx, root, bundle, preflight, params.Arguments.PlanID, params.Arguments.Accept)
			value = map[string]any{"submission_revision": revision, "preflight": preflight, "cloud_write": err == nil, "business_files_modified": false}
			if err == nil {
				navigationRoot = root
				navigationTarget = &projectview.Target{View: "review", Focus: &projectview.Focus{Kind: "submission_revision", ID: revision.ID, Digest: revision.ContentHash}}
			}
		}
	case "submission_status":
		if strings.TrimSpace(params.Arguments.SubmissionID) == "" {
			return nil, domain.Invalid("SUBMISSION_ID_REQUIRED", "submission_id 必填")
		}
		root, client, _, clientErr := r.workspaceClient(params.Arguments.Directory)
		if clientErr != nil {
			return nil, clientErr
		}
		var details app.SubmissionDetails
		err = client.Dispatch(ctx, "submission.workspace-show", map[string]any{"id": params.Arguments.SubmissionID}, &details)
		value = map[string]any{"submission_id": details.Submission.ID, "status": details.Submission.Status, "current_revision_id": details.Submission.CurrentRevisionID, "revision_count": len(details.Revisions)}
		navigationRoot = root
		target := submissionStatusProjectView(details)
		navigationTarget = &target
	case "review_feedback_list":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var feedback []domain.ReviewFeedbackBundle
		err = client.Dispatch(ctx, "feedback.workspace-list", map[string]any{}, &feedback)
		value = map[string]any{"count": len(feedback), "feedback": feedback, "business_files_modified": false}
	case "review_feedback_pull":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		root, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var feedback []domain.ReviewFeedbackBundle
		err = client.Dispatch(ctx, "feedback.workspace-list", map[string]any{}, &feedback)
		items := make([]localworkspace.ReviewFeedbackInboxItem, 0, len(feedback))
		if err == nil {
			for _, bundle := range feedback {
				var item localworkspace.ReviewFeedbackInboxItem
				item, err = localworkspace.StoreReviewFeedback(root, bundle, r.currentTime())
				if err != nil {
					break
				}
				items = append(items, item)
			}
		}
		value = map[string]any{"count": len(feedback), "downloaded": items, "cloud_read": true, "business_files_modified": false}
		navigationRoot = root
		target := reviewFeedbackProjectView(feedback)
		navigationTarget = &target
	case "review_feedback_inbox":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var items []localworkspace.ReviewFeedbackInboxItem
		items, err = localworkspace.ReviewFeedbackInbox(root)
		value = map[string]any{"count": len(items), "feedback": items, "offline": true, "business_files_modified": false}
	case "approved_snapshot_list":
		if params.Arguments.SubmissionType != "" && !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, domain.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var snapshots []domain.ApprovedSnapshot
		err = client.Dispatch(ctx, "snapshot.workspace-list", map[string]any{"submission_type": params.Arguments.SubmissionType}, &snapshots)
		value = map[string]any{"count": len(snapshots), "snapshots": snapshots, "cloud_read": true, "business_files_modified": false}
	case "approved_snapshot_pull":
		if params.Arguments.SubmissionType != "" && !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, domain.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var snapshots []domain.ApprovedSnapshot
		if strings.TrimSpace(params.Arguments.SnapshotID) != "" {
			var snapshot domain.ApprovedSnapshot
			err = client.Dispatch(ctx, "snapshot.workspace-show", map[string]any{"id": params.Arguments.SnapshotID}, &snapshot)
			snapshots = []domain.ApprovedSnapshot{snapshot}
		} else {
			err = client.Dispatch(ctx, "snapshot.workspace-list", map[string]any{"submission_type": params.Arguments.SubmissionType}, &snapshots)
		}
		var downloaded []localworkspace.ApprovedSnapshotCacheRecord
		if err == nil {
			downloaded, err = localworkspace.StoreApprovedSnapshots(root, snapshots, r.currentTime())
		}
		value = map[string]any{"count": len(snapshots), "downloaded": downloaded, "cloud_read": true, "business_files_modified": false}
		navigationRoot = root
		target := approvedSnapshotsProjectView(snapshots)
		navigationTarget = &target
	case "approved_snapshot_inbox":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var snapshots []localworkspace.ApprovedSnapshotCacheSummary
		snapshots, err = localworkspace.ApprovedSnapshotInbox(root, params.Arguments.SubmissionType)
		value = map[string]any{"count": len(snapshots), "snapshots": snapshots, "offline": true, "business_files_modified": false}
	case "approved_snapshot_show":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var record localworkspace.ApprovedSnapshotCacheRecord
		record, err = localworkspace.ShowApprovedSnapshot(root, params.Arguments.SnapshotID)
		value = map[string]any{"record": record, "offline": true, "business_files_modified": false}
	default:
		return nil, domain.NotFound("MCP 工具")
	}
	if err != nil {
		return nil, err
	}
	var link *projectview.Link
	if navigationTarget != nil {
		built, buildErr := projectViewLinkForWorkspace(navigationRoot, *navigationTarget)
		if buildErr == nil {
			link = &built
		}
	}
	return mcpToolSuccessEnvelope(value, link)
}

func submissionStatusProjectView(details app.SubmissionDetails) projectview.Target {
	target := projectview.Target{View: "review"}
	for _, revision := range details.Revisions {
		if revision.ID != details.Submission.CurrentRevisionID {
			continue
		}
		focused := projectview.Target{View: "review", Focus: &projectview.Focus{Kind: "submission_revision", ID: revision.ID, Digest: revision.ContentHash}}
		if projectview.Validate(focused) == nil {
			return focused
		}
		break
	}
	return target
}

func reviewFeedbackProjectView(feedback []domain.ReviewFeedbackBundle) projectview.Target {
	target := projectview.Target{View: "review"}
	if len(feedback) == 0 {
		return target
	}
	revisionID := feedback[0].SubmissionRevisionID
	digest := feedback[0].SubjectHash
	for _, bundle := range feedback[1:] {
		if bundle.SubmissionRevisionID != revisionID || bundle.SubjectHash != digest {
			return target
		}
	}
	focused := projectview.Target{View: "review", Focus: &projectview.Focus{Kind: "submission_revision", ID: revisionID, Digest: digest}}
	if projectview.Validate(focused) != nil {
		return target
	}
	return focused
}

func approvedSnapshotsProjectView(snapshots []domain.ApprovedSnapshot) projectview.Target {
	target := projectview.Target{View: "delivery"}
	if len(snapshots) != 1 {
		return target
	}
	snapshot := snapshots[0]
	focused := projectview.Target{View: "delivery", Focus: &projectview.Focus{Kind: "snapshot", ID: snapshot.ID, Digest: snapshot.ContentHash}}
	if projectview.Validate(focused) != nil {
		return target
	}
	return focused
}

func projectViewLinkForWorkspace(root string, target projectview.Target) (projectview.Link, error) {
	status, err := localworkspace.LoadStatus(root)
	if err != nil {
		return projectview.Link{}, err
	}
	return projectview.Build(status.Binding.ServerURL, status.Binding.ProjectID, target)
}

func mcpToolSuccessEnvelope(value any, link *projectview.Link) (map[string]any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(body)}}, "structuredContent": value, "isError": false}, nil
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": string(body)},
			mcpProjectViewResourceLink(*link),
		},
		"structuredContent": value,
		"isError":           false,
	}, nil
}

func validSubmissionType(value string) bool {
	switch value {
	case "context", "knowledge", "brief", "content_batch", "asset_batch", "delivery", "result":
		return true
	default:
		return false
	}
}

func mcpToolError(err error) map[string]any {
	result := map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		result["structuredContent"] = map[string]any{"error": domainError}
	}
	return result
}

func decodeOpenProjectViewArguments(raw json.RawMessage) (mcpProjectViewArguments, error) {
	var params struct {
		Name      string                  `json:"name"`
		Arguments mcpProjectViewArguments `json:"arguments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&params); err != nil {
		return mcpProjectViewArguments{}, domain.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_project_view 参数无效或包含未知字段")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return mcpProjectViewArguments{}, domain.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_project_view 参数必须是单个 JSON 对象")
	}
	if params.Name != "contentcloud_open_project_view" || strings.TrimSpace(params.Arguments.View) == "" {
		return mcpProjectViewArguments{}, domain.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_project_view 需要 view")
	}
	return params.Arguments, nil
}

func openProjectViewEnvelope(link projectview.Link) map[string]any {
	result := mcpProjectViewResult{
		ProjectID: link.ProjectID,
		View:      link.View,
		Focus:     link.Focus,
		BrowserHandoff: mcpBrowserHandoff{
			Required: true, URL: link.URL, PreferredMode: "codex-internal-browser", BrowserAction: "navigate",
		},
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": "Content Work OS 项目页面链接已准备。请在浏览器中打开并核对项目和焦点对象后再报告完成。"},
			mcpProjectViewResourceLink(link),
		},
		"structuredContent": result,
		"isError":           false,
	}
}

func mcpProjectViewResourceLink(link projectview.Link) map[string]any {
	return map[string]any{"type": "resource_link", "uri": link.URL, "name": link.Name, "description": link.Description, "mimeType": "text/html"}
}

func requestedMCPProtocolVersion(raw json.RawMessage) string {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(raw, &params); err == nil && strings.TrimSpace(params.ProtocolVersion) != "" {
		return params.ProtocolVersion
	}
	return "2025-03-26"
}

func contentCloudMCPResources() []map[string]any {
	return []map[string]any{
		{"uri": "contentcloud://workspace/conversation-context", "name": "Content Work OS 工作区对话上下文", "description": "用于开始或继续对话的离线持久化状态", "mimeType": "application/json"},
		{"uri": "contentcloud://workspace/status", "name": "Content Work OS 工作区状态", "description": "离线工作区绑定、环境和同步状态", "mimeType": "application/json"},
	}
}

func (r *Root) readContentCloudMCPResource(raw json.RawMessage) (map[string]any, error) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, domain.Invalid("MCP_RESOURCE_PARAMS_INVALID", "MCP 资源参数无效")
	}
	var value any
	var err error
	switch params.URI {
	case "contentcloud://workspace/conversation-context":
		value, err = r.workspaceConversationContext("")
	case "contentcloud://workspace/status":
		value, err = r.contentCloudWorkspaceStatus("")
	default:
		return nil, domain.NotFound("MCP 资源")
	}
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return map[string]any{"contents": []map[string]string{{"uri": params.URI, "mimeType": "application/json", "text": string(body)}}}, nil
}

func (r *Root) contentCloudWorkspaceStatus(directory string) (map[string]any, error) {
	resolution, err := localworkspace.ResolveWorkspaceRoot(directory, r.mcpCWD)
	if err != nil {
		return nil, err
	}
	status, err := localworkspace.LoadStatus(resolution.Root)
	if err != nil {
		return nil, err
	}
	doctor, err := r.workspaceDoctor(resolution.Root)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema_version": "1.0", "resolution": resolution, "workspace": status, "doctor": doctor, "offline": true}, nil
}

func (r *Root) workspaceConversationContext(directory string) (localworkspace.WorkspaceConversationContext, error) {
	context, err := localworkspace.ConversationContext(directory, r.mcpCWD, r.currentTime())
	if err != nil {
		return localworkspace.WorkspaceConversationContext{}, err
	}
	report, err := r.workspaceDoctor(context.Root)
	if err != nil {
		return localworkspace.WorkspaceConversationContext{}, err
	}
	if report.OK {
		context.EnvironmentHealth = "ready"
		status, loadErr := localworkspace.LoadStatus(context.Root)
		if loadErr != nil {
			return localworkspace.WorkspaceConversationContext{}, loadErr
		}
		if hasBootstrapTarget(status.Template.Targets) {
			verified, verifyErr := r.loadVerifiedLocalEnvironment(context.Root)
			if verifyErr != nil {
				return localworkspace.WorkspaceConversationContext{}, verifyErr
			}
			context.ContentTypes = append([]string(nil), verified.State.Manifest.ContentTypes...)
		}
	} else {
		context.EnvironmentHealth = "repair_required"
	}
	return context, nil
}

func (r *Root) workspaceDoctor(directory string) (localworkspace.DoctorReport, error) {
	report, err := localworkspace.Doctor(directory)
	if err != nil {
		return localworkspace.DoctorReport{}, err
	}
	status, err := localworkspace.LoadStatus(report.Root)
	if err != nil {
		return localworkspace.DoctorReport{}, err
	}
	if !hasBootstrapTarget(status.Template.Targets) {
		return report, nil
	}
	manifestVerifier, manifestErr := r.environmentManifestVerifier()
	registryVerifier, registryErr := r.environmentRegistryVerifier()
	if manifestErr != nil || registryErr != nil {
		message := "环境信任库不可用"
		if manifestErr != nil {
			message = manifestErr.Error()
		} else if registryErr != nil {
			message = registryErr.Error()
		}
		report.Checks["environment"] = localworkspace.Check{OK: false, Required: true, Message: message}
		report.OK = false
		return report, nil
	}
	return localworkspace.DoctorWithEnvironment(report.Root, manifestVerifier, registryVerifier, r.currentTime())
}

type verifiedLocalEnvironment struct {
	Root     string
	State    localworkspace.EnvironmentState
	Registry environment.VerifiedRegistry
}

type environmentPackInstall struct {
	Plugin     environment.PluginRef `json:"plugin"`
	Applied    bool                  `json:"applied"`
	Idempotent bool                  `json:"idempotent"`
}

type environmentNewChatHandoff struct {
	RequiresNewChat bool   `json:"requires_new_chat"`
	WorkspacePath   string `json:"workspace_path"`
	DeepLink        string `json:"deep_link"`
	RecoveryPrompt  string `json:"recovery_prompt"`
}

type environmentPreparationApplyResult struct {
	PreparationID         string                         `json:"preparation_id"`
	ExecutionPlan         environment.LocalExecutionPlan `json:"execution_plan"`
	InstalledPacks        []environmentPackInstall       `json:"installed_packs"`
	Lock                  environment.EnvironmentLock    `json:"lock"`
	Doctor                localworkspace.DoctorReport    `json:"doctor"`
	Handoff               environmentNewChatHandoff      `json:"handoff"`
	BusinessFilesModified bool                           `json:"business_files_modified"`
}

type installedPackTransaction struct {
	adapter *codexplugin.Adapter
	receipt codexplugin.Receipt
}

func (r *Root) loadVerifiedLocalEnvironment(directory string) (verifiedLocalEnvironment, error) {
	resolution, err := localworkspace.ResolveWorkspaceRoot(directory, r.mcpCWD)
	if err != nil {
		return verifiedLocalEnvironment{}, err
	}
	manifestVerifier, err := r.environmentManifestVerifier()
	if err != nil {
		return verifiedLocalEnvironment{}, err
	}
	registryVerifier, err := r.environmentRegistryVerifier()
	if err != nil {
		return verifiedLocalEnvironment{}, err
	}
	state, err := localworkspace.LoadEnvironment(resolution.Root, manifestVerifier, r.currentTime())
	if err != nil {
		return verifiedLocalEnvironment{}, err
	}
	registry, err := localworkspace.LoadEnvironmentRegistry(resolution.Root, registryVerifier)
	if err != nil {
		return verifiedLocalEnvironment{}, err
	}
	if err := environment.ValidateManifestRegistry(state.Manifest, registry, environment.PurposeNewRun); err != nil {
		return verifiedLocalEnvironment{}, err
	}
	return verifiedLocalEnvironment{Root: resolution.Root, State: state, Registry: registry}, nil
}

func (r *Root) resolveLocalExecutionPlan(directory, runID, intent string, capabilities, inputRefs []string) (environment.LocalExecutionPlan, error) {
	verified, err := r.loadVerifiedLocalEnvironment(directory)
	if err != nil {
		return environment.LocalExecutionPlan{}, err
	}
	return r.resolveLocalExecutionPlanForEnvironment(verified, runID, intent, capabilities, inputRefs)
}

func (r *Root) resolveLocalExecutionPlanForEnvironment(verified verifiedLocalEnvironment, runID, intent string, capabilities, inputRefs []string) (environment.LocalExecutionPlan, error) {
	manifestVerifier, err := r.environmentManifestVerifier()
	if err != nil {
		return environment.LocalExecutionPlan{}, err
	}
	resolver, err := environment.NewResolver(manifestVerifier)
	if err != nil {
		return environment.LocalExecutionPlan{}, err
	}
	return resolver.ResolveLocal(verified.State.Manifest, verified.Registry, verified.State.Lock, environment.LocalPlanRequest{
		ProjectID: verified.State.Manifest.ProjectID, RunID: strings.TrimSpace(runID), Intent: strings.TrimSpace(intent),
		RequiredCapabilities: append([]string(nil), capabilities...), InputRefs: append([]string(nil), inputRefs...),
	}, r.currentTime())
}

func (r *Root) resolveEnvironmentPreparation(input environmentPreparationInput) (verifiedLocalEnvironment, environment.LocalExecutionPlan, environment.PreparationPlan, error) {
	verified, err := r.loadVerifiedLocalEnvironment(input.Directory)
	if err != nil {
		return verifiedLocalEnvironment{}, environment.LocalExecutionPlan{}, environment.PreparationPlan{}, err
	}
	execution, err := r.resolveLocalExecutionPlanForEnvironment(verified, input.RunID, input.Intent, input.Capabilities, input.InputRefs)
	if err != nil {
		return verifiedLocalEnvironment{}, environment.LocalExecutionPlan{}, environment.PreparationPlan{}, err
	}
	preparation, err := environment.BuildPreparationPlan(verified.State.Manifest.ProjectID, execution, verified.Registry)
	if err != nil {
		return verifiedLocalEnvironment{}, environment.LocalExecutionPlan{}, environment.PreparationPlan{}, err
	}
	return verified, execution, preparation, nil
}

func (r *Root) applyEnvironmentPreparation(ctx context.Context, input environmentPreparationInput, expectedPreparationID string, accepted bool) (environmentPreparationApplyResult, error) {
	verified, _, preparation, err := r.resolveEnvironmentPreparation(input)
	if err != nil {
		return environmentPreparationApplyResult{}, err
	}
	if strings.TrimSpace(expectedPreparationID) == "" || expectedPreparationID != preparation.PreparationID {
		stale := domain.Conflict("ENVIRONMENT_PREPARATION_PLAN_STALE", "环境准备计划与当前环境或执行输入不一致，请重新生成计划")
		stale.Details = map[string]any{"expected_preparation_id": expectedPreparationID, "current_preparation_id": preparation.PreparationID}
		return environmentPreparationApplyResult{}, stale
	}
	if preparation.State == "repair_required" {
		return environmentPreparationApplyResult{}, domain.Policy("ENVIRONMENT_PREPARATION_REPAIR_REQUIRED", "当前能力包存在版本、类型或摘要漂移，禁止原地覆盖", "运行独立环境修复流程后重新生成执行计划")
	}
	if preparation.State != "ready" || len(preparation.Actions) == 0 {
		return environmentPreparationApplyResult{}, domain.Conflict("ENVIRONMENT_PREPARATION_NOT_REQUIRED", "当前执行计划不需要安装任务能力包")
	}
	if !accepted {
		confirmation := domain.Policy("ENVIRONMENT_PREPARATION_CONFIRMATION_REQUIRED", "安装任务能力包、更新环境锁并切换新会话需要明确确认", "展示准备计划的权限、数据流和费用后，使用相同 preparation_id 并传入 accept=true")
		confirmation.ExitCode = 2
		return environmentPreparationApplyResult{}, confirmation
	}
	beforeDoctor, err := r.workspaceDoctor(verified.Root)
	if err != nil {
		return environmentPreparationApplyResult{}, err
	}
	if err := requireHealthyWorkspace(beforeDoctor); err != nil {
		return environmentPreparationApplyResult{}, err
	}
	lease, err := localworkspace.BeginEnvironmentPreparation(verified.Root, preparation.PreparationID, r.currentTime())
	if err != nil {
		return environmentPreparationApplyResult{}, err
	}

	transactions := []installedPackTransaction{}
	installs := []environmentPackInstall{}
	var committedLock environment.EnvironmentLock
	lockCommitted := false
	rollback := func(cause error) error {
		rollbackErrors := []string{}
		canRemovePacks := true
		if lockCommitted {
			if restoreErr := localworkspace.CompareAndSwapEnvironmentLock(verified.Root, verified.State.Manifest, committedLock, verified.State.Lock); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, "restore environment.lock: "+restoreErr.Error())
				canRemovePacks = false
			}
		}
		if canRemovePacks {
			rollbackContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			for index := len(transactions) - 1; index >= 0; index-- {
				for _, rollbackError := range transactions[index].adapter.Rollback(rollbackContext, transactions[index].receipt) {
					rollbackErrors = append(rollbackErrors, rollbackError)
				}
			}
		}
		if finishErr := localworkspace.FinishEnvironmentPreparation(verified.Root, lease.Token); finishErr != nil {
			rollbackErrors = append(rollbackErrors, "release preparation lease: "+finishErr.Error())
		}
		if len(rollbackErrors) == 0 {
			return cause
		}
		rollbackFailure := domain.E("runtime", "environment_preparation", "ENVIRONMENT_PREPARATION_ROLLBACK_FAILED", "环境准备失败且局部回滚不完整", 5)
		rollbackFailure.Details = map[string]any{"cause": cause.Error(), "rollback_errors": rollbackErrors}
		return rollbackFailure
	}

	rawRegistry := verified.Registry.Registry()
	for _, action := range preparation.Actions {
		entry, exactErr := rawRegistry.Exact(action.Plugin.ID, action.Plugin.Version, action.Plugin.Digest)
		if exactErr != nil {
			return environmentPreparationApplyResult{}, rollback(exactErr)
		}
		spec := codexplugin.Spec{
			CodexBinary: "codex", MarketplaceName: verified.State.Manifest.Distribution.Marketplace,
			MarketplaceSource: entry.Source.Repository, MarketplaceRef: entry.Source.Ref,
			PluginName: action.Plugin.ID, PluginID: action.Plugin.ID + "@" + verified.State.Manifest.Distribution.Marketplace, PluginVersion: action.Plugin.Version,
		}
		adapter, adapterErr := codexplugin.New(spec, r.codexRunner)
		if adapterErr != nil {
			return environmentPreparationApplyResult{}, rollback(adapterErr)
		}
		adapterPlan, planErr := adapter.Plan(ctx)
		if planErr != nil {
			return environmentPreparationApplyResult{}, rollback(planErr)
		}
		if adapterPlan.State == "blocked" {
			blocked := domain.Conflict("ENVIRONMENT_PREPARATION_CODEX_STATE_BLOCKED", "Codex 中现有插件市场或能力包与签名计划冲突")
			blocked.Details = adapterPlan.BlockingReasons
			return environmentPreparationApplyResult{}, rollback(blocked)
		}
		for _, adapterAction := range adapterPlan.Actions {
			if adapterAction.Kind != "plugin.add" {
				return environmentPreparationApplyResult{}, rollback(domain.Conflict("ENVIRONMENT_PREPARATION_MARKETPLACE_NOT_READY", "任务能力包安装要求已存在且版本正确的 Content Work OS 插件市场"))
			}
		}
		applyResult, applyErr := adapter.Apply(ctx, adapterPlan, true)
		if applyErr != nil {
			return environmentPreparationApplyResult{}, rollback(applyErr)
		}
		transactions = append(transactions, installedPackTransaction{adapter: adapter, receipt: applyResult.Receipt})
		installs = append(installs, environmentPackInstall{Plugin: action.Plugin, Applied: applyResult.Applied, Idempotent: applyResult.Idempotent})
	}

	committedLock, err = environment.PreparedLock(verified.State.Manifest, verified.State.Lock, preparation, verified.Registry, r.currentTime())
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	if err := localworkspace.CompareAndSwapEnvironmentLock(verified.Root, verified.State.Manifest, verified.State.Lock, committedLock); err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	lockCommitted = true
	verified.State.Lock = committedLock
	readyExecution, err := r.resolveLocalExecutionPlanForEnvironment(verified, input.RunID, input.Intent, input.Capabilities, input.InputRefs)
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	if readyExecution.State != "ready" || len(readyExecution.Preparation) != 0 {
		return environmentPreparationApplyResult{}, rollback(domain.Conflict("ENVIRONMENT_PREPARATION_NOT_READY", "任务能力包安装后，本地执行计划仍未 ready"))
	}
	afterDoctor, err := r.workspaceDoctor(verified.Root)
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	if err := requireHealthyWorkspace(afterDoctor); err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	handoff, err := environmentPreparationHandoff(verified, rawRegistry)
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	if err := localworkspace.FinishEnvironmentPreparation(verified.Root, lease.Token); err != nil {
		return environmentPreparationApplyResult{}, err
	}
	return environmentPreparationApplyResult{
		PreparationID: preparation.PreparationID, ExecutionPlan: readyExecution, InstalledPacks: installs,
		Lock: committedLock, Doctor: afterDoctor, Handoff: handoff, BusinessFilesModified: false,
	}, nil
}

func environmentPreparationHandoff(verified verifiedLocalEnvironment, registry environment.Registry) (environmentNewChatHandoff, error) {
	for _, plugin := range verified.State.Manifest.Distribution.Plugins {
		if plugin.Kind != "scene_plugin" || plugin.Scope != "environment" || !plugin.Required {
			continue
		}
		entry, err := registry.Exact(plugin.ID, plugin.Version, plugin.Digest)
		if err != nil {
			return environmentNewChatHandoff{}, err
		}
		spec := codexplugin.Spec{
			CodexBinary: "codex", MarketplaceName: verified.State.Manifest.Distribution.Marketplace,
			MarketplaceSource: entry.Source.Repository, MarketplaceRef: entry.Source.Ref,
			PluginName: plugin.ID, PluginID: plugin.ID + "@" + verified.State.Manifest.Distribution.Marketplace, PluginVersion: plugin.Version,
		}
		prompt := codexplugin.RecoveryPrompt(spec)
		deepLink, err := codexplugin.NewChatDeepLink(spec, verified.Root, prompt)
		if err != nil {
			return environmentNewChatHandoff{}, err
		}
		return environmentNewChatHandoff{RequiresNewChat: true, WorkspacePath: verified.Root, DeepLink: deepLink, RecoveryPrompt: prompt}, nil
	}
	return environmentNewChatHandoff{}, domain.Conflict("ENVIRONMENT_SCENE_PLUGIN_REQUIRED", "当前环境缺少恢复新会话所需的必装场景插件")
}

func (r *Root) resolveMCPWorkspace(directory string) (string, error) {
	resolution, err := localworkspace.ResolveWorkspaceRoot(directory, r.mcpCWD)
	if err != nil {
		return "", err
	}
	return resolution.Root, nil
}

func secondsDuration(value int64) time.Duration {
	if value == 0 {
		return 0
	}
	return time.Duration(value) * time.Second
}

func (r *Root) currentTime() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func optionalDirectory(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return ""
}

func workspaceConflict(paths []string) error {
	shown := paths
	if len(shown) > 8 {
		shown = shown[:8]
	}
	err := domain.Conflict("WORKSPACE_DIRECTORY_NOT_EMPTY", "目标目录非空且不是 Content Work OS 工作区")
	err.Hint = "请选择空目录，或先确认并整理冲突文件：" + strings.Join(shown, ", ")
	err.Details = map[string]any{"conflicts": paths}
	return err
}

func enrichWorkspaceStatus(status localworkspace.Status) map[string]any {
	return map[string]any{
		"workspace": status,
		"agents": map[string]any{
			"codex":  executableAvailable("codex"),
			"claude": executableAvailable("claude"),
		},
		"disclosure_policy": map[string]any{"default": "evidence_pack", "raw_upload": false},
		"automation":        map[string]any{"enabled": status.AutomationEnabled, "required_for_local_work": false},
	}
}

func executableAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func errorStringValue(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
