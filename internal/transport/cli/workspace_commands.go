package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/spf13/cobra"

	fixturev3 "github.com/limecloud/contentcloud/internal/bootstrap/fixture"
	"github.com/limecloud/contentcloud/internal/catalog/environment"
	capabilityrouting "github.com/limecloud/contentcloud/internal/catalog/routing"
	"github.com/limecloud/contentcloud/internal/experience/projection"
	"github.com/limecloud/contentcloud/internal/integration/pluginhost"
	"github.com/limecloud/contentcloud/internal/local/workbench"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	apiclient "github.com/limecloud/contentcloud/internal/transport/client"

	"github.com/limecloud/contentcloud/internal/application"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
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
		r.workspaceMemoryCommand(),
		r.workspaceApprovedCommand(),
	)
	return cmd
}

func (r *Root) workspaceMemoryCommand() *cobra.Command {
	command := &cobra.Command{Use: "memory", Short: "管理本地可重建的记忆检索投影"}
	var statusDirectory string
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, Short: "检查本地记忆索引是否存在、陈旧或损坏", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.MemoryStatus(statusDirectory, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.status", result)
	}}
	status.Flags().StringVar(&statusDirectory, "directory", "", "Content Work OS 工作区路径")

	var rebuildDirectory string
	rebuild := &cobra.Command{Use: "rebuild", Args: cobra.NoArgs, Short: "从当前工作区文件重建本地记忆索引", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.RebuildMemory(rebuildDirectory, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.rebuild", result)
	}}
	rebuild.Flags().StringVar(&rebuildDirectory, "directory", "", "Content Work OS 工作区路径")

	var rememberDirectory, rememberID, rememberKind, rememberClaimKey, rememberSourceRef, rememberSummary, rememberFormedBy string
	remember := &cobra.Command{Use: "remember", Args: cobra.NoArgs, Short: "保存一条绑定当前来源的本地记忆候选", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.RememberMemory(localworkspace.MemoryRememberOptions{Root: rememberDirectory, MemoryID: rememberID, Kind: rememberKind, ClaimKey: rememberClaimKey, SourceRef: rememberSourceRef, Summary: rememberSummary, FormedBy: rememberFormedBy, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.remember", result)
	}}
	remember.Flags().StringVar(&rememberDirectory, "directory", "", "Content Work OS 工作区路径")
	remember.Flags().StringVar(&rememberID, "id", "", "可选的稳定记忆候选 ID")
	remember.Flags().StringVar(&rememberKind, "kind", "", "记忆类型：working、execution、knowledge 或 interaction")
	remember.Flags().StringVar(&rememberClaimKey, "claim-key", "", "可选的主张稳定键；相同键的不同摘要会进入冲突报告")
	remember.Flags().StringVar(&rememberSourceRef, "source-ref", "", "当前工作区内的来源文件相对路径")
	remember.Flags().StringVar(&rememberSummary, "summary", "", "带来源的候选摘要")
	remember.Flags().StringVar(&rememberFormedBy, "formed-by", "", "形成者或模型版本标识")
	_ = remember.MarkFlagRequired("kind")
	_ = remember.MarkFlagRequired("source-ref")
	_ = remember.MarkFlagRequired("summary")

	var consolidateDirectory string
	consolidate := &cobra.Command{Use: "consolidate", Args: cobra.NoArgs, Short: "检测本地记忆候选的重复和冲突，不覆盖任何记录", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.ConsolidateMemory(localworkspace.MemoryConsolidationOptions{Root: consolidateDirectory, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.consolidate", result)
	}}
	consolidate.Flags().StringVar(&consolidateDirectory, "directory", "", "Content Work OS 工作区路径")

	var promoteDirectory, promoteID, promoteKind, promoteTitle, promoteSubject, promotePredicate, promoteRisk, promoteOriginRun string
	var promoteChannels, promoteEvidenceIDs []string
	promote := &cobra.Command{Use: "promote", Args: cobra.NoArgs, Short: "将一个无冲突记忆候选导入为待审核知识候选", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.PromoteMemory(localworkspace.MemoryPromoteOptions{Root: promoteDirectory, MemoryID: promoteID, KnowledgeKind: promoteKind, Title: promoteTitle, Subject: promoteSubject, Predicate: promotePredicate, RiskLevel: promoteRisk, AllowedChannels: promoteChannels, EvidenceIDs: promoteEvidenceIDs, OriginRunID: promoteOriginRun, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.promote", result)
	}}
	promote.Flags().StringVar(&promoteDirectory, "directory", "", "Content Work OS 工作区路径")
	promote.Flags().StringVar(&promoteID, "memory-id", "", "待晋升的记忆候选 ID")
	promote.Flags().StringVar(&promoteKind, "knowledge-kind", "fact", "知识类型：fact、claim、visual_rule 或 methodology")
	promote.Flags().StringVar(&promoteTitle, "title", "", "知识标题；默认使用记忆摘要")
	promote.Flags().StringVar(&promoteSubject, "subject", "", "知识主语")
	promote.Flags().StringVar(&promotePredicate, "predicate", "", "知识谓语")
	promote.Flags().StringVar(&promoteRisk, "risk-level", "low", "风险级别：low、medium 或 high")
	promote.Flags().StringSliceVar(&promoteChannels, "allowed-channel", nil, "允许使用的渠道，可重复传入")
	promote.Flags().StringSliceVar(&promoteEvidenceIDs, "evidence-id", nil, "已接受的本地证据 ID，可重复传入")
	promote.Flags().StringVar(&promoteOriginRun, "origin-run", "", "形成来源的本地运行 ID")
	_ = promote.MarkFlagRequired("memory-id")
	_ = promote.MarkFlagRequired("subject")
	_ = promote.MarkFlagRequired("predicate")
	_ = promote.MarkFlagRequired("evidence-id")

	var extractDirectory, extractEndpoint, extractProvider, extractTokenEnv, extractFormedBy string
	var extractSources []string
	var extractAllowPrivate, extractAllowHTTP bool
	extract := &cobra.Command{Use: "extract", Args: cobra.NoArgs, Short: "显式调用受控远程抽取器形成本地记忆候选", RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv(strings.TrimSpace(extractTokenEnv))
		adapter, err := localworkspace.NewMemoryRemoteAdapter(localworkspace.MemoryRemoteAdapterConfig{Provider: extractProvider, BaseURL: extractEndpoint, AuthToken: token, AllowPrivateNetworks: extractAllowPrivate, AllowInsecureHTTP: extractAllowHTTP})
		if err != nil {
			return err
		}
		result, err := localworkspace.ExtractMemory(cmd.Context(), localworkspace.MemoryExtractOptions{Root: extractDirectory, SourceRefs: extractSources, Adapter: adapter, FormedBy: extractFormedBy, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.extract", result)
	}}
	extract.Flags().StringVar(&extractDirectory, "directory", "", "Content Work OS 工作区路径")
	extract.Flags().StringVar(&extractEndpoint, "endpoint", "", "远程记忆抽取服务 HTTPS 地址")
	extract.Flags().StringVar(&extractProvider, "provider", "custom", "远程适配器标识，例如 mem0 或 tencentdb")
	extract.Flags().StringVar(&extractTokenEnv, "token-env", "CONTENTCLOUD_MEMORY_REMOTE_TOKEN", "从哪个环境变量读取远程服务 Token")
	extract.Flags().StringSliceVar(&extractSources, "source-ref", nil, "要发送给抽取器的来源文件，可重复传入；为空表示全部允许来源")
	extract.Flags().StringVar(&extractFormedBy, "formed-by", "", "候选形成者标识")
	extract.Flags().BoolVar(&extractAllowPrivate, "allow-private-networks", false, "显式允许连接私有网络服务")
	extract.Flags().BoolVar(&extractAllowHTTP, "allow-http", false, "显式允许 HTTP；仅建议测试环境使用")
	_ = extract.MarkFlagRequired("endpoint")

	var remoteDirectory, remoteEndpoint, remoteProvider, remoteTokenEnv, remoteQueryText string
	var remoteKinds []string
	var remoteLimit, remoteMaxChars int
	var remoteAllowPrivate, remoteAllowHTTP bool
	remoteQuery := &cobra.Command{Use: "remote-query", Args: cobra.NoArgs, Short: "显式通过受控远程适配器查询并回验本地记忆来源", RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv(strings.TrimSpace(remoteTokenEnv))
		adapter, err := localworkspace.NewMemoryRemoteAdapter(localworkspace.MemoryRemoteAdapterConfig{Provider: remoteProvider, BaseURL: remoteEndpoint, AuthToken: token, AllowPrivateNetworks: remoteAllowPrivate, AllowInsecureHTTP: remoteAllowHTTP})
		if err != nil {
			return err
		}
		result, err := localworkspace.QueryRemoteMemory(cmd.Context(), remoteDirectory, adapter, localworkspace.MemoryQueryOptions{Root: remoteDirectory, Query: remoteQueryText, Kinds: remoteKinds, Limit: remoteLimit, MaxChars: remoteMaxChars, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.remote-query", result)
	}}
	remoteQuery.Flags().StringVar(&remoteDirectory, "directory", "", "Content Work OS 工作区路径")
	remoteQuery.Flags().StringVar(&remoteEndpoint, "endpoint", "", "远程记忆查询服务 HTTPS 地址")
	remoteQuery.Flags().StringVar(&remoteProvider, "provider", "custom", "远程适配器标识")
	remoteQuery.Flags().StringVar(&remoteTokenEnv, "token-env", "CONTENTCLOUD_MEMORY_REMOTE_TOKEN", "从哪个环境变量读取远程服务 Token")
	remoteQuery.Flags().StringVar(&remoteQueryText, "query", "", "查询文本")
	remoteQuery.Flags().StringSliceVar(&remoteKinds, "kind", nil, "记忆类型，可重复传入")
	remoteQuery.Flags().IntVar(&remoteLimit, "limit", 0, "最多返回条数")
	remoteQuery.Flags().IntVar(&remoteMaxChars, "max-chars", 0, "摘要字符预算")
	remoteQuery.Flags().BoolVar(&remoteAllowPrivate, "allow-private-networks", false, "显式允许连接私有网络服务")
	remoteQuery.Flags().BoolVar(&remoteAllowHTTP, "allow-http", false, "显式允许 HTTP；仅建议测试环境使用")
	_ = remoteQuery.MarkFlagRequired("endpoint")

	var queryDirectory, queryText string
	var queryKinds []string
	var queryLimit, queryMaxChars int
	query := &cobra.Command{Use: "query", Args: cobra.NoArgs, Short: "按当前工作区范围查询带来源引用的记忆候选", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.QueryMemory(localworkspace.MemoryQueryOptions{Root: queryDirectory, Query: queryText, Kinds: queryKinds, Limit: queryLimit, MaxChars: queryMaxChars, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.query", result)
	}}
	query.Flags().StringVar(&queryDirectory, "directory", "", "Content Work OS 工作区路径")
	query.Flags().StringVar(&queryText, "query", "", "查询文本；为空时按工作区焦点和最近修改排序")
	query.Flags().StringSliceVar(&queryKinds, "kind", nil, "记忆类型：working、execution、knowledge、interaction，可重复传入")
	query.Flags().IntVar(&queryLimit, "limit", 0, "最多返回条数，默认 6，最大 20")
	query.Flags().IntVar(&queryMaxChars, "max-chars", 0, "摘要字符预算，默认 2400，最大 12000")

	var clearDirectory string
	var clearYes, clearDryRun bool
	clear := &cobra.Command{Use: "clear", Args: cobra.NoArgs, Short: "清除本地记忆索引缓存，不删除工作区文件", RunE: func(cmd *cobra.Command, args []string) error {
		status, err := localworkspace.MemoryStatus(clearDirectory, r.currentTime())
		if err != nil {
			return err
		}
		if clearDryRun {
			return r.writeOK("workspace.memory.clear", map[string]any{"dry_run": true, "projection_ref": status.ProjectionRef, "state": status.State, "would_clear": status.State != localworkspace.MemoryStateMissing})
		}
		if !clearYes {
			return confirmationRequired("清除本地记忆索引缓存；不会删除工作区文件，但会丢弃可重建的检索投影")
		}
		result, err := localworkspace.ClearMemory(clearDirectory, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("workspace.memory.clear", result)
	}}
	clear.Flags().StringVar(&clearDirectory, "directory", "", "Content Work OS 工作区路径")
	clear.Flags().BoolVar(&clearYes, "yes", false, "确认清除本地记忆索引缓存")
	clear.Flags().BoolVar(&clearDryRun, "dry-run", false, "只检查，不修改本地缓存")

	command.AddCommand(status, rebuild, remember, consolidate, promote, extract, remoteQuery, query, clear)
	return command
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
				return fault.Invalid("FIXTURE_V3_INVALID", err.Error())
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
		&cobra.Command{Use: "runtime-serve", Args: cobra.NoArgs, Short: "通过标准输入输出提供 Attempt 级 Runtime MCP 工具", RunE: func(cmd *cobra.Command, args []string) error {
			return r.serveRuntimeMCP(cmd.Context(), cmd.InOrStdin())
		}},
	)
	return cmd
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpServerRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  map[string]any  `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

const (
	mcpAppsExtensionID = "io.modelcontextprotocol/ui"
	mcpAppsResourceURI = "ui://contentcloud/workbench"
	mcpAppsMIMEType    = "text/html;profile=mcp-app"
)

const contentCloudWorkspaceRootEnvironment = "CONTENTCLOUD_WORKSPACE_ROOT"

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpRoot struct {
	URI  string
	Name string
	Root string
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
	ProjectID      string            `json:"project_id"`
	View           string            `json:"view"`
	Focus          *projection.Focus `json:"focus,omitempty"`
	BrowserHandoff mcpBrowserHandoff `json:"browserHandoff"`
}

type mcpWorkspaceViewArguments struct {
	Directory               string `json:"directory,omitempty"`
	View                    string `json:"view"`
	Ref                     string `json:"ref,omitempty"`
	RunID                   string `json:"run_id,omitempty"`
	ExpectedContextRevision uint64 `json:"expected_context_revision,omitempty"`
	ExpectedDigest          string `json:"expected_digest,omitempty"`
}

type mcpWorkbenchArguments struct {
	Directory               string `json:"directory,omitempty"`
	View                    string `json:"view,omitempty"`
	Ref                     string `json:"ref,omitempty"`
	RunID                   string `json:"run_id,omitempty"`
	ExpectedContextRevision uint64 `json:"expected_context_revision,omitempty"`
	ExpectedDigest          string `json:"expected_digest,omitempty"`
}

type mcpWorkspaceProposalPrepareArguments struct {
	Directory               string `json:"directory,omitempty"`
	RunID                   string `json:"run_id"`
	ClaimToken              string `json:"claim_token"`
	OwnerKind               string `json:"owner_kind"`
	OwnerID                 string `json:"owner_id"`
	OwnerEpoch              uint64 `json:"owner_epoch"`
	ExpectedContextRevision uint64 `json:"expected_context_revision"`
	TypedAction             string `json:"typed_action"`
	Ref                     string `json:"ref"`
	ExpectedDigest          string `json:"expected_digest"`
	Content                 string `json:"content"`
	IdempotencyKey          string `json:"idempotency_key"`
}

type mcpWorkspaceProposalApplyArguments struct {
	Directory               string `json:"directory,omitempty"`
	ProposalID              string `json:"proposal_id"`
	ClaimToken              string `json:"claim_token"`
	OwnerKind               string `json:"owner_kind"`
	OwnerID                 string `json:"owner_id"`
	OwnerEpoch              uint64 `json:"owner_epoch"`
	ExpectedContextRevision uint64 `json:"expected_context_revision"`
	IdempotencyKey          string `json:"idempotency_key"`
	Confirm                 bool   `json:"confirm"`
}

func (r *Root) serveMCP(ctx context.Context, input io.Reader) error {
	r.setMCPAppsSupported(false)
	r.resetMCPRoots()
	if strings.TrimSpace(r.mcpCWD) == "" {
		r.mcpCWD = strings.TrimSpace(os.Getenv(contentCloudWorkspaceRootEnvironment))
		if r.mcpCWD == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			r.mcpCWD = cwd
		}
	}
	manager := r.localWorkbenchManager()
	defer manager.Close()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	encoder := json.NewEncoder(r.stdout)
	rootRequests := map[string]bool{}
	clientSupportsRoots := false
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
		if request.Method == "" {
			if len(request.ID) > 0 && rootRequests[mcpID(request.ID)] && len(request.Result) > 0 {
				r.applyMCPRoots(request.Result)
				delete(rootRequests, mcpID(request.ID))
			}
			continue
		}
		if request.Method == "notifications/initialized" {
			if clientSupportsRoots {
				if rootsRequestID := r.requestMCPRoots(encoder); rootsRequestID != "" {
					rootRequests[rootsRequestID] = true
				}
			}
			continue
		}
		if request.Method == "notifications/roots/list_changed" {
			if clientSupportsRoots {
				if rootsRequestID := r.requestMCPRoots(encoder); rootsRequestID != "" {
					rootRequests[rootsRequestID] = true
				}
			}
			continue
		}
		response := r.handleMCPRequest(ctx, request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
		if request.Method == "initialize" {
			clientSupportsRoots = mcpRootsCapabilitySupported(request.Params)
		}
	}
	return scanner.Err()
}

func mcpID(raw json.RawMessage) string {
	return string(bytes.TrimSpace(raw))
}

func mcpRootsCapabilitySupported(raw json.RawMessage) bool {
	var params struct {
		Capabilities struct {
			Roots json.RawMessage `json:"roots"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || len(params.Capabilities.Roots) == 0 || string(params.Capabilities.Roots) == "null" {
		return false
	}
	var roots map[string]json.RawMessage
	return json.Unmarshal(params.Capabilities.Roots, &roots) == nil
}

func (r *Root) resetMCPRoots() {
	r.mcpRootsMu.Lock()
	r.mcpRoots = nil
	r.mcpRootsError = ""
	r.mcpRootsMu.Unlock()
}

func (r *Root) applyMCPRoots(raw json.RawMessage) {
	var payload struct {
		Roots []struct {
			URI  string `json:"uri"`
			Name string `json:"name,omitempty"`
		} `json:"roots"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		r.mcpRootsMu.Lock()
		r.mcpRoots = nil
		r.mcpRootsError = "roots/list 响应无效"
		r.mcpRootsMu.Unlock()
		return
	}
	valid := make([]mcpRoot, 0, len(payload.Roots))
	for _, candidate := range payload.Roots {
		parsed, err := url.Parse(strings.TrimSpace(candidate.URI))
		if err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") {
			continue
		}
		path := parsed.Path
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		root, err := localworkspace.FindRoot(filepath.FromSlash(path))
		if err != nil {
			continue
		}
		if canonical, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
			root = canonical
		}
		duplicate := false
		for _, existing := range valid {
			if existing.Root == root {
				duplicate = true
				break
			}
		}
		if !duplicate {
			valid = append(valid, mcpRoot{URI: candidate.URI, Name: candidate.Name, Root: root})
		}
	}
	r.mcpRootsMu.Lock()
	r.mcpRoots = valid
	r.mcpRootsError = ""
	r.mcpRootsMu.Unlock()
}

func (r *Root) mcpRootsSnapshot() ([]mcpRoot, string) {
	r.mcpRootsMu.RLock()
	defer r.mcpRootsMu.RUnlock()
	return append([]mcpRoot(nil), r.mcpRoots...), r.mcpRootsError
}

func (r *Root) requestMCPRoots(encoder *json.Encoder) string {
	id := "contentcloud-roots-" + idgen.New()
	if err := encoder.Encode(mcpServerRequest{JSONRPC: "2.0", ID: json.RawMessage(strconv.Quote(id)), Method: "roots/list"}); err != nil {
		return ""
	}
	return strconv.Quote(id)
}

func (r *Root) handleMCPRequest(ctx context.Context, request mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		r.setMCPAppsSupported(mcpAppsCapabilitySupported(request.Params))
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
		response.Result = map[string]any{"tools": mcpToolsWithApps(r.mcpAppsEnabled())}
	case "tools/call":
		result, err := r.callLocalMCPTool(ctx, request.Params)
		if err != nil {
			response.Result = mcpToolError(err)
		} else {
			response.Result = result
		}
	case "resources/list":
		response.Result = map[string]any{"resources": contentCloudMCPResourcesWithApps(r.mcpAppsEnabled())}
	case "resources/templates/list":
		response.Result = map[string]any{"resourceTemplates": contentCloudMCPResourceTemplates()}
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

func (r *Root) setMCPAppsSupported(supported bool) {
	r.mcpCapabilityMu.Lock()
	r.mcpAppsSupported = supported
	r.mcpCapabilityMu.Unlock()
}

func (r *Root) mcpAppsEnabled() bool {
	r.mcpCapabilityMu.RLock()
	defer r.mcpCapabilityMu.RUnlock()
	return r.mcpAppsSupported
}

func mcpAppsCapabilitySupported(raw json.RawMessage) bool {
	var params struct {
		Capabilities struct {
			Extensions map[string]json.RawMessage `json:"extensions"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return false
	}
	capability, ok := params.Capabilities.Extensions[mcpAppsExtensionID]
	if !ok || len(capability) == 0 || string(capability) == "null" {
		return false
	}
	var settings struct {
		MIMETypes []string `json:"mimeTypes"`
	}
	if err := json.Unmarshal(capability, &settings); err != nil {
		return false
	}
	for _, mimeType := range settings.MIMETypes {
		if mimeType == mcpAppsMIMEType {
			return true
		}
	}
	return false
}

func mcpTools() []map[string]any {
	return mcpToolsWithApps(false)
}

func mcpToolsWithApps(appsSupported bool) []map[string]any {
	directory := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"}},
		"additionalProperties": false,
	}
	memoryQuery := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"query":     map[string]any{"type": "string", "description": "查询文本；为空时按工作区焦点和最近修改排序"},
			"kinds":     map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"working", "execution", "knowledge", "interaction"}}},
			"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
			"max_chars": map[string]any{"type": "integer", "minimum": 1, "maximum": 12000},
		},
		"additionalProperties": false,
	}
	memoryRemember := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":  map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"memory_id":  map[string]any{"type": "string"},
			"kind":       map[string]any{"type": "string", "enum": []string{"working", "execution", "knowledge", "interaction"}},
			"claim_key":  map[string]any{"type": "string", "description": "可选的主张稳定键；同键不同摘要会进入冲突报告"},
			"source_ref": map[string]any{"type": "string", "description": "当前工作区内的来源文件相对路径"},
			"summary":    map[string]any{"type": "string"},
			"formed_by":  map[string]any{"type": "string"},
		},
		"required":             []string{"kind", "source_ref", "summary"},
		"additionalProperties": false,
	}
	memoryConsolidate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory": map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
		},
		"additionalProperties": false,
	}
	memoryPromote := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":        map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"memory_id":        map[string]any{"type": "string"},
			"knowledge_kind":   map[string]any{"type": "string", "enum": []string{"fact", "claim", "visual_rule", "methodology"}},
			"title":            map[string]any{"type": "string"},
			"subject":          map[string]any{"type": "string"},
			"predicate":        map[string]any{"type": "string"},
			"risk_level":       map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
			"allowed_channels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"evidence_ids":     map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
			"origin_run":       map[string]any{"type": "string"},
		},
		"required":             []string{"memory_id", "knowledge_kind", "subject", "predicate", "evidence_ids"},
		"additionalProperties": false,
	}
	memoryExtract := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":              map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"endpoint":               map[string]any{"type": "string", "description": "远程抽取服务 HTTPS 地址"},
			"provider":               map[string]any{"type": "string"},
			"token_env":              map[string]any{"type": "string", "description": "服务 Token 所在环境变量名"},
			"source_refs":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"formed_by":              map[string]any{"type": "string"},
			"allow_private_networks": map[string]any{"type": "boolean"},
			"allow_http":             map[string]any{"type": "boolean"},
		},
		"required":             []string{"endpoint"},
		"additionalProperties": false,
	}
	memoryRemoteQuery := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":              map[string]any{"type": "string", "description": "工作区路径；默认使用当前目录"},
			"endpoint":               map[string]any{"type": "string", "description": "远程查询服务 HTTPS 地址"},
			"provider":               map[string]any{"type": "string"},
			"token_env":              map[string]any{"type": "string"},
			"query":                  map[string]any{"type": "string"},
			"kinds":                  map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"working", "execution", "knowledge", "interaction"}}},
			"limit":                  map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
			"max_chars":              map[string]any{"type": "integer", "minimum": 1, "maximum": 12000},
			"allow_private_networks": map[string]any{"type": "boolean"},
			"allow_http":             map[string]any{"type": "boolean"},
		},
		"required":             []string{"endpoint"},
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
			"view":      map[string]any{"type": "string", "enum": projection.IDs()},
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
	localRunRecord := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"claim_token":       map[string]any{"type": "string"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"input_ids":         map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"changed_ids":       map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"eligible_ids":      map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"blocked_ids":       map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"findings":          map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"output_paths":      map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision"},
		"additionalProperties": false,
	}
	localRunCheck := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"claim_token":       map[string]any{"type": "string"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"name":              map[string]any{"type": "string"},
			"status":            map[string]any{"type": "string", "enum": []string{"passed", "failed"}},
			"command":           map[string]any{"type": "string"},
			"detail":            map[string]any{"type": "string"},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision", "name", "status"},
		"additionalProperties": false,
	}
	localRunAdvance := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"claim_token":       map[string]any{"type": "string"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"stage":             map[string]any{"type": "string", "enum": []string{"knowledge-lint", "query", "compile", "output-lint", "done"}},
			"input_ids":         map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"changed_ids":       map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"eligible_ids":      map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"blocked_ids":       map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"findings":          map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
			"output_paths":      map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision", "stage"},
		"additionalProperties": false,
	}
	localRunFail := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"claim_token":       map[string]any{"type": "string"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"findings":          map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision", "findings"},
		"additionalProperties": false,
	}
	localRunResume := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"claim_token":       map[string]any{"type": "string"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
		},
		"required":             []string{"run_id", "claim_token", "expected_revision"},
		"additionalProperties": false,
	}
	localRunClaim := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":         map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":            map[string]any{"type": "string"},
			"owner_kind":        map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"owner_id":          map[string]any{"type": "string", "description": "稳定的对话、工作进程或 Workbench 持有者 ID"},
			"expected_revision": map[string]any{"type": "integer", "minimum": 1},
			"ttl_seconds":       map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
			"takeover_expired":  map[string]any{"type": "boolean"},
		},
		"required":             []string{"run_id", "owner_kind", "owner_id", "expected_revision"},
		"additionalProperties": false,
	}
	localRunTakeover := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":           map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":              map[string]any{"type": "string"},
			"owner_kind":          map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"owner_id":            map[string]any{"type": "string"},
			"expected_owner_kind": map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"expected_owner_id":   map[string]any{"type": "string"},
			"expected_epoch":      map[string]any{"type": "integer", "minimum": 1},
			"expected_revision":   map[string]any{"type": "integer", "minimum": 1},
			"ttl_seconds":         map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
		},
		"required":             []string{"run_id", "owner_kind", "owner_id", "expected_owner_kind", "expected_owner_id", "expected_epoch", "expected_revision"},
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
			"owner_kind":       map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"owner_id":         map[string]any{"type": "string"},
			"ttl_seconds":      map[string]any{"type": "integer", "minimum": 1, "maximum": 14400},
			"takeover_expired": map[string]any{"type": "boolean"},
		},
		"required":             []string{"handoff_id", "owner_kind", "owner_id"},
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
	workspaceView := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":                 map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"view":                      map[string]any{"type": "string", "enum": []string{"workspace_summary", "file", "run", "handoff", "content_item", "render", "diff", "delivery"}},
			"ref":                       map[string]any{"type": "string", "description": "允许目录内的 Workspace-relative 文件路径"},
			"run_id":                    map[string]any{"type": "string"},
			"expected_context_revision": map[string]any{"type": "integer", "minimum": 1},
			"expected_digest":           map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"},
		},
		"required":             []string{"view"},
		"additionalProperties": false,
	}
	workbenchOpen := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":                 map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"view":                      map[string]any{"type": "string", "enum": []string{"workspace_summary", "file", "run", "handoff", "content_item", "render", "diff", "delivery"}},
			"ref":                       map[string]any{"type": "string", "description": "允许目录内的 Workspace-relative 文件或目录"},
			"run_id":                    map[string]any{"type": "string"},
			"expected_context_revision": map[string]any{"type": "integer", "minimum": 1},
			"expected_digest":           map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"},
		},
		"additionalProperties": false,
	}
	workspaceProposalPrepare := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":                 map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"run_id":                    map[string]any{"type": "string"},
			"claim_token":               map[string]any{"type": "string"},
			"owner_kind":                map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"owner_id":                  map[string]any{"type": "string"},
			"owner_epoch":               map[string]any{"type": "integer", "minimum": 1},
			"expected_context_revision": map[string]any{"type": "integer", "minimum": 1},
			"typed_action":              map[string]any{"type": "string", "enum": []string{"workspace_file.replace"}},
			"ref":                       map[string]any{"type": "string"},
			"expected_digest":           map[string]any{"type": "string", "pattern": "^sha256:[a-f0-9]{64}$"},
			"content":                   map[string]any{"type": "string", "maxLength": 2097152},
			"idempotency_key":           map[string]any{"type": "string", "minLength": 8, "maxLength": 128},
		},
		"required":             []string{"run_id", "claim_token", "owner_kind", "owner_id", "owner_epoch", "expected_context_revision", "typed_action", "ref", "expected_digest", "content", "idempotency_key"},
		"additionalProperties": false,
	}
	workspaceProposalApply := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"directory":                 map[string]any{"type": "string", "description": "工作区路径；默认使用 MCP 进程当前目录"},
			"proposal_id":               map[string]any{"type": "string"},
			"claim_token":               map[string]any{"type": "string"},
			"owner_kind":                map[string]any{"type": "string", "enum": []string{"agent", "browser"}},
			"owner_id":                  map[string]any{"type": "string"},
			"owner_epoch":               map[string]any{"type": "integer", "minimum": 1},
			"expected_context_revision": map[string]any{"type": "integer", "minimum": 1},
			"idempotency_key":           map[string]any{"type": "string", "minLength": 8, "maxLength": 128},
			"confirm":                   map[string]any{"type": "boolean", "const": true},
		},
		"required":             []string{"proposal_id", "claim_token", "owner_kind", "owner_id", "owner_epoch", "expected_context_revision", "idempotency_key", "confirm"},
		"additionalProperties": false,
	}
	tools := []map[string]any{
		{
			"name":        "contentcloud_open_studio_view",
			"description": "为当前项目生成可信的 Content Work OS Studio 页面链接，不打开浏览器，也不修改本地或云端状态",
			"inputSchema": openProjectView,
			"annotations": cloudReadAnnotations,
			"_meta": map[string]any{"contentcloud/effect": map[string]any{
				"effect_scope": "cloud_read_navigation", "requires_confirmation": false, "writes_workspace": false, "writes_cloud": false,
			}},
		},
		{"name": "workspace_context", "description": "读取跨对话保存的 Content Work OS 工作区状态，不访问云端，也不声明任何写入", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "workspace_view", "description": "把允许目录内的 Workspace 文件、Run 或 Handoff 读取为带 digest 的类型化本地视图，不访问云端", "inputSchema": workspaceView, "annotations": readOnlyAnnotations},
		{"name": "workspace_open_workbench", "description": "在当前 stdio MCP 进程内启动或复用受限 loopback Workbench，并返回私有浏览器交接元数据；不访问云端", "inputSchema": workbenchOpen, "annotations": readOnlyAnnotations},
		{"name": "workspace_workbench_status", "description": "读取当前 MCP 进程内的本地 Workbench 状态，不返回 URL 或 capability", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "workspace_close_workbench", "description": "关闭当前 MCP 进程内的本地 Workbench listener 和所有浏览器 capability", "inputSchema": directory, "annotations": workspaceWriteAnnotations},
		{"name": "workspace_proposal_prepare", "description": "在有效 owner/epoch 下校验本地草稿并生成绑定 revision 与文件 digest 的一次性 Proposal，不写正式文件", "inputSchema": workspaceProposalPrepare, "annotations": workspaceWriteAnnotations},
		{"name": "workspace_proposal_apply", "description": "只应用用户准确确认且 owner、epoch、revision、digest 均未变化的 Proposal，并推进 LocalRun revision", "inputSchema": workspaceProposalApply, "annotations": workspaceWriteAnnotations},
		{"name": "memory_status", "description": "检查本地记忆投影是否存在、陈旧或损坏，不读取云端", "inputSchema": directory, "annotations": readOnlyAnnotations},
		{"name": "memory_rebuild", "description": "从当前工作区文本文件重建可删除的本地记忆检索投影，不上传文件", "inputSchema": directory, "annotations": workspaceWriteAnnotations},
		{"name": "memory_remember", "description": "保存一条绑定当前来源文件的本地记忆候选，不晋升为正式知识", "inputSchema": memoryRemember, "annotations": workspaceWriteAnnotations},
		{"name": "memory_consolidate", "description": "检测本地记忆候选的重复和冲突，不覆盖任何记录", "inputSchema": memoryConsolidate, "annotations": readOnlyAnnotations},
		{"name": "memory_promote", "description": "把无冲突记忆候选导入为待审核知识候选，不创建批准快照", "inputSchema": memoryPromote, "annotations": workspaceWriteAnnotations},
		{"name": "memory_extract", "description": "只向明确指定的远程抽取器发送工作区来源，结果仍由本地记忆契约校验", "inputSchema": memoryExtract, "annotations": workspaceWriteAnnotations},
		{"name": "memory_remote_query", "description": "通过远程适配器查询并回验 scope、来源 digest 和状态", "inputSchema": memoryRemoteQuery, "annotations": readOnlyAnnotations},
		{"name": "memory_query", "description": "按当前工作区绑定范围召回带来源引用的记忆候选", "inputSchema": memoryQuery, "annotations": readOnlyAnnotations},
		{"name": "workspace_project_brief", "description": "确认并保存项目简报，建立素材、知识和内容生产共用的本地业务上下文", "inputSchema": projectBrief, "annotations": workspaceWriteAnnotations},
		{"name": "environment_execution_plan", "description": "离线解析已签名的本地执行计划并准确报告缺少的任务能力包，不执行安装", "inputSchema": localExecutionPlan, "annotations": readOnlyAnnotations},
		{"name": "environment_prepare_plan", "description": "披露缺少能力包的准确权限、数据流、费用和会话影响，不执行安装", "inputSchema": localExecutionPlan, "annotations": readOnlyAnnotations},
		{"name": "environment_prepare_apply", "description": "只安装用户准确确认的能力包计划，原子更新环境锁，重新执行诊断并返回新对话交接", "inputSchema": environmentPreparationApply, "annotations": environmentWriteAnnotations},
		{"name": "local_run_claim", "description": "取得指定本地运行上下文版本的单写入者租约，明文 token 只在本次结果中返回", "inputSchema": localRunClaim, "annotations": workspaceWriteAnnotations},
		{"name": "local_run_takeover", "description": "在用户确认后按当前 owner、epoch 和 revision 接管仍有效的单写入者租约", "inputSchema": localRunTakeover, "annotations": workspaceWriteAnnotations},
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
		{"name": "local_run_record", "description": "在当前运行中记录不可变输入、变更、结果和阻断引用", "inputSchema": localRunRecord, "annotations": workspaceWriteAnnotations},
		{"name": "local_run_check", "description": "在当前运行中记录一项确定性阶段检查", "inputSchema": localRunCheck, "annotations": workspaceWriteAnnotations},
		{"name": "local_run_advance", "description": "在检查通过后按受治理状态机推进当前运行", "inputSchema": localRunAdvance, "annotations": workspaceWriteAnnotations},
		{"name": "local_run_fail", "description": "保留 finding 并将当前运行标记为失败", "inputSchema": localRunFail, "annotations": workspaceWriteAnnotations},
		{"name": "local_run_resume", "description": "在修复失败原因后恢复原运行，不创建第二个状态源", "inputSchema": localRunResume, "annotations": workspaceWriteAnnotations},
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
	if appsSupported {
		for _, tool := range tools {
			if tool["name"] == "workspace_open_workbench" {
				tool["_meta"] = map[string]any{"ui": map[string]any{"resourceUri": mcpAppsResourceURI}}
				break
			}
		}
	}
	return tools
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
			Stage                string               `json:"stage"`
			Status               string               `json:"status"`
			Command              string               `json:"command"`
			Detail               string               `json:"detail"`
			OwnerKind            string               `json:"owner_kind"`
			OwnerID              string               `json:"owner_id"`
			OwnerEpoch           uint64               `json:"owner_epoch"`
			ExpectedOwnerKind    string               `json:"expected_owner_kind"`
			ExpectedOwnerID      string               `json:"expected_owner_id"`
			ExpectedEpoch        uint64               `json:"expected_epoch"`
			ClaimToken           string               `json:"claim_token"`
			ExpectedRevision     uint64               `json:"expected_revision"`
			ContextRevision      uint64               `json:"expected_context_revision"`
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
			ChangedIDs           []string             `json:"changed_ids"`
			EligibleIDs          []string             `json:"eligible_ids"`
			BlockedIDs           []string             `json:"blocked_ids"`
			Findings             []string             `json:"findings"`
			OutputPaths          []string             `json:"output_paths"`
			WithIngest           bool                 `json:"with_ingest"`
			OriginRun            string               `json:"origin_run"`
			Channel              string               `json:"channel"`
			At                   string               `json:"at"`
			Query                string               `json:"query"`
			Kinds                []string             `json:"kinds"`
			Limit                int                  `json:"limit"`
			MaxChars             int                  `json:"max_chars"`
			MemoryID             string               `json:"memory_id"`
			Kind                 string               `json:"kind"`
			ClaimKey             string               `json:"claim_key"`
			SourceRef            string               `json:"source_ref"`
			SourceRefs           []string             `json:"source_refs"`
			Summary              string               `json:"summary"`
			FormedBy             string               `json:"formed_by"`
			Endpoint             string               `json:"endpoint"`
			Provider             string               `json:"provider"`
			TokenEnv             string               `json:"token_env"`
			AllowPrivateNetworks bool                 `json:"allow_private_networks"`
			AllowHTTP            bool                 `json:"allow_http"`
			KnowledgeKind        string               `json:"knowledge_kind"`
			Subject              string               `json:"subject"`
			Predicate            string               `json:"predicate"`
			RiskLevel            string               `json:"risk_level"`
			AllowedChannels      []string             `json:"allowed_channels"`
			EvidenceIDs          []string             `json:"evidence_ids"`
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
			Content              string               `json:"content"`
			TypedAction          string               `json:"typed_action"`
			ProposalID           string               `json:"proposal_id"`
			IdempotencyKey       string               `json:"idempotency_key"`
			PlanID               string               `json:"plan_id"`
			PreparationID        string               `json:"preparation_id"`
			Accept               bool                 `json:"accept"`
			View                 string               `json:"view"`
			Ref                  string               `json:"ref"`
			ExpectedDigest       string               `json:"expected_digest"`
			Focus                *mcpProjectViewFocus `json:"focus"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fault.Invalid("MCP_PARAMS_INVALID", "MCP 工具参数无效")
	}
	var value any
	var err error
	var navigationRoot string
	var navigationTarget *projection.Target
	switch params.Name {
	case "contentcloud_open_studio_view":
		arguments, decodeErr := decodeOpenProjectViewArguments(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			workspaceErr := fault.Conflict("WORKSPACE_NOT_BOUND", "无法根据 directory 或 MCP 当前工作目录解析 Content Work OS 工作区绑定")
			workspaceErr.Hint = "在 Content Work OS 工作区中打开新对话，或显式传入工作区路径"
			return nil, workspaceErr
		}
		status, loadErr := localworkspace.LoadStatus(root)
		if loadErr != nil {
			return nil, loadErr
		}
		var focus *projection.Focus
		if arguments.Focus != nil {
			focus = &projection.Focus{Kind: arguments.Focus.Kind, ID: arguments.Focus.ID, Digest: arguments.Focus.Digest}
		}
		link, buildErr := projection.Build(status.Binding.ServerURL, status.Binding.ProjectID, projection.Target{View: arguments.View, Focus: focus})
		if buildErr != nil {
			return nil, buildErr
		}
		return openProjectViewEnvelope(link), nil
	case "workspace_context":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.workspaceConversationContext(root)
	case "workspace_view":
		arguments, decodeErr := decodeWorkspaceViewArguments(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.BuildWorkspaceView(localworkspace.WorkspaceViewOptions{
			Root: root, View: arguments.View, Ref: arguments.Ref, RunID: arguments.RunID,
			ExpectedContextRevision: arguments.ExpectedContextRevision, ExpectedDigest: arguments.ExpectedDigest,
			Now: r.currentTime(),
		})
	case "workspace_open_workbench":
		arguments, decodeErr := decodeWorkbenchArguments(raw, "workspace_open_workbench")
		if decodeErr != nil {
			return nil, decodeErr
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		opened, openErr := r.localWorkbenchManager().Open(ctx, workbench.OpenOptions{Root: root, View: arguments.View, Ref: arguments.Ref, RunID: arguments.RunID, ExpectedContextRevision: arguments.ExpectedContextRevision, ExpectedDigest: arguments.ExpectedDigest})
		if openErr != nil {
			return nil, openErr
		}
		return workbenchOpenEnvelope(opened), nil
	case "workspace_workbench_status":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.localWorkbenchManager().Status(root)
	case "workspace_close_workbench":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		err = r.localWorkbenchManager().CloseWorkspace(root)
		value = map[string]any{"closed": err == nil}
	case "workspace_proposal_prepare":
		arguments, decodeErr := decodeWorkspaceProposalPrepareArguments(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.localProposalStore().PrepareIdempotent(arguments.IdempotencyKey, localworkspace.PrepareWorkspaceProposalOptions{
			Root: root, RunID: arguments.RunID, ClaimToken: arguments.ClaimToken,
			OwnerKind: arguments.OwnerKind, OwnerID: arguments.OwnerID, OwnerEpoch: arguments.OwnerEpoch,
			ExpectedContextRevision: arguments.ExpectedContextRevision, TypedAction: arguments.TypedAction,
			Ref: arguments.Ref, ExpectedDigest: arguments.ExpectedDigest, Content: arguments.Content, Now: r.currentTime(),
		})
	case "workspace_proposal_apply":
		arguments, decodeErr := decodeWorkspaceProposalApplyArguments(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if !arguments.Confirm {
			return nil, fault.Invalid("WORKSPACE_PROPOSAL_CONFIRMATION_REQUIRED", "workspace_proposal_apply 需要 confirm=true 准确确认同一 Proposal")
		}
		root, resolveErr := r.resolveMCPWorkspace(arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.localProposalStore().ApplyIdempotent(arguments.IdempotencyKey, arguments.ProposalID, localworkspace.ApplyWorkspaceProposalOptions{
			Root: root, ClaimToken: arguments.ClaimToken, OwnerKind: arguments.OwnerKind, OwnerID: arguments.OwnerID,
			OwnerEpoch: arguments.OwnerEpoch, ExpectedContextRevision: arguments.ExpectedContextRevision, Now: r.currentTime(),
		})
	case "memory_status":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.MemoryStatus(root, r.currentTime())
	case "memory_rebuild":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.RebuildMemory(root, r.currentTime())
	case "memory_remember":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.RememberMemory(localworkspace.MemoryRememberOptions{Root: root, MemoryID: params.Arguments.MemoryID, Kind: params.Arguments.Kind, ClaimKey: params.Arguments.ClaimKey, SourceRef: params.Arguments.SourceRef, Summary: params.Arguments.Summary, FormedBy: params.Arguments.FormedBy, Now: r.currentTime()})
	case "memory_consolidate":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ConsolidateMemory(localworkspace.MemoryConsolidationOptions{Root: root, Now: r.currentTime()})
	case "memory_promote":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.PromoteMemory(localworkspace.MemoryPromoteOptions{Root: root, MemoryID: params.Arguments.MemoryID, KnowledgeKind: params.Arguments.KnowledgeKind, Title: params.Arguments.Title, Subject: params.Arguments.Subject, Predicate: params.Arguments.Predicate, RiskLevel: params.Arguments.RiskLevel, AllowedChannels: params.Arguments.AllowedChannels, EvidenceIDs: params.Arguments.EvidenceIDs, OriginRunID: params.Arguments.OriginRun, Now: r.currentTime()})
	case "memory_extract":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		token := os.Getenv(strings.TrimSpace(params.Arguments.TokenEnv))
		adapter, adapterErr := localworkspace.NewMemoryRemoteAdapter(localworkspace.MemoryRemoteAdapterConfig{
			Provider:             params.Arguments.Provider,
			BaseURL:              params.Arguments.Endpoint,
			AuthToken:            token,
			AllowPrivateNetworks: params.Arguments.AllowPrivateNetworks,
			AllowInsecureHTTP:    params.Arguments.AllowHTTP,
		})
		if adapterErr != nil {
			return nil, adapterErr
		}
		value, err = localworkspace.ExtractMemory(ctx, localworkspace.MemoryExtractOptions{
			Root:       root,
			SourceRefs: params.Arguments.SourceRefs,
			Adapter:    adapter,
			FormedBy:   params.Arguments.FormedBy,
			Now:        r.currentTime(),
		})
	case "memory_remote_query":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		token := os.Getenv(strings.TrimSpace(params.Arguments.TokenEnv))
		adapter, adapterErr := localworkspace.NewMemoryRemoteAdapter(localworkspace.MemoryRemoteAdapterConfig{
			Provider:             params.Arguments.Provider,
			BaseURL:              params.Arguments.Endpoint,
			AuthToken:            token,
			AllowPrivateNetworks: params.Arguments.AllowPrivateNetworks,
			AllowInsecureHTTP:    params.Arguments.AllowHTTP,
		})
		if adapterErr != nil {
			return nil, adapterErr
		}
		value, err = localworkspace.QueryRemoteMemory(ctx, root, adapter, localworkspace.MemoryQueryOptions{
			Root:     root,
			Query:    params.Arguments.Query,
			Kinds:    params.Arguments.Kinds,
			Limit:    params.Arguments.Limit,
			MaxChars: params.Arguments.MaxChars,
			Now:      r.currentTime(),
		})
	case "memory_query":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.QueryMemory(localworkspace.MemoryQueryOptions{Root: root, Query: params.Arguments.Query, Kinds: params.Arguments.Kinds, Limit: params.Arguments.Limit, MaxChars: params.Arguments.MaxChars, Now: r.currentTime()})
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
			navigationTarget = &projection.Target{View: "home"}
		}
	case "workspace_doctor":
		var root string
		root, err = r.resolveMCPWorkspace(params.Arguments.Directory)
		if err == nil {
			var report localworkspace.DoctorReport
			report, err = r.workspaceDoctor(root)
			value = report
			navigationRoot = root
			navigationTarget = &projection.Target{View: "connect"}
		}
	case "source_register":
		if strings.TrimSpace(params.Arguments.File) == "" {
			return nil, fault.Invalid("LOCAL_SOURCE_FILE_REQUIRED", "file 参数必填")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: root, File: params.Arguments.File, ID: params.Arguments.ID, Title: params.Arguments.Title, SourceKind: params.Arguments.SourceKind, StorageMode: params.Arguments.StorageMode, Now: r.currentTime()})
	case "source_list":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var sources []localworkspace.LocalSource
		sources, err = localworkspace.LocalSources(root)
		value = map[string]any{"count": len(sources), "sources": sources}
	case "source_ingest":
		if strings.TrimSpace(params.Arguments.SourceID) == "" {
			return nil, fault.Invalid("LOCAL_SOURCE_ID_REQUIRED", "source_id 参数必填")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.IngestLocalSource(root, params.Arguments.SourceID, r.currentTime())
	case "source_verify":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var report localworkspace.SourceVerification
		report, err = localworkspace.VerifyLocalSources(root)
		value = report
		if err == nil && !report.Valid {
			err = fault.Invalid("LOCAL_SOURCE_VERIFY_FAILED", "本地来源完整性校验失败")
		}
	case "local_run_init":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: root, RunID: params.Arguments.RunID, Intent: params.Arguments.Intent, InputIDs: params.Arguments.InputIDs, WithIngest: params.Arguments.WithIngest, Now: r.currentTime()})
	case "local_run_show":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ShowLocalRun(root, params.Arguments.RunID)
	case "local_run_record":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.RecordClaimedLocalRun(localworkspace.RecordLocalRunOptions{
			Root: root, RunID: params.Arguments.RunID, ClaimToken: params.Arguments.ClaimToken, ExpectedRevision: params.Arguments.ExpectedRevision,
			InputIDs: params.Arguments.InputIDs, ChangedIDs: params.Arguments.ChangedIDs, EligibleIDs: params.Arguments.EligibleIDs,
			BlockedIDs: params.Arguments.BlockedIDs, Findings: params.Arguments.Findings, OutputPaths: params.Arguments.OutputPaths, Now: r.currentTime(),
		})
	case "local_run_check":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.CheckClaimedLocalRun(localworkspace.CheckLocalRunOptions{
			Root: root, RunID: params.Arguments.RunID, ClaimToken: params.Arguments.ClaimToken, ExpectedRevision: params.Arguments.ExpectedRevision,
			Name: params.Arguments.Name, Status: params.Arguments.Status, Command: params.Arguments.Command, Detail: params.Arguments.Detail, Now: r.currentTime(),
		})
	case "local_run_advance":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.AdvanceClaimedLocalRun(root, params.Arguments.RunID, params.Arguments.Stage, localworkspace.RecordLocalRunOptions{
			ClaimToken: params.Arguments.ClaimToken, ExpectedRevision: params.Arguments.ExpectedRevision,
			InputIDs: params.Arguments.InputIDs, ChangedIDs: params.Arguments.ChangedIDs, EligibleIDs: params.Arguments.EligibleIDs,
			BlockedIDs: params.Arguments.BlockedIDs, Findings: params.Arguments.Findings, OutputPaths: params.Arguments.OutputPaths,
		}, r.currentTime())
	case "local_run_fail":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.FailClaimedLocalRun(root, params.Arguments.RunID, params.Arguments.Findings, params.Arguments.ClaimToken, params.Arguments.ExpectedRevision, r.currentTime())
	case "local_run_resume":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ResumeClaimedLocalRun(root, params.Arguments.RunID, params.Arguments.ClaimToken, params.Arguments.ExpectedRevision, r.currentTime())
	case "local_run_claim":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ClaimRun(localworkspace.ClaimRunOptions{Root: root, RunID: params.Arguments.RunID, OwnerKind: params.Arguments.OwnerKind, OwnerID: params.Arguments.OwnerID, ExpectedRevision: params.Arguments.ExpectedRevision, TTL: secondsDuration(params.Arguments.TTLSeconds), TakeoverExpired: params.Arguments.TakeoverExpired, Now: r.currentTime()})
	case "local_run_takeover":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.TakeoverRunClaim(localworkspace.TakeoverRunClaimOptions{
			Root: root, RunID: params.Arguments.RunID, OwnerKind: params.Arguments.OwnerKind, OwnerID: params.Arguments.OwnerID,
			ExpectedOwnerKind: params.Arguments.ExpectedOwnerKind, ExpectedOwnerID: params.Arguments.ExpectedOwnerID,
			ExpectedEpoch: params.Arguments.ExpectedEpoch, ExpectedRevision: params.Arguments.ExpectedRevision,
			TTL: secondsDuration(params.Arguments.TTLSeconds), Now: r.currentTime(),
		})
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
		handoff, claim, err = localworkspace.AcceptHandoff(localworkspace.AcceptHandoffOptions{Root: root, HandoffID: params.Arguments.HandoffID, OwnerKind: params.Arguments.OwnerKind, OwnerID: params.Arguments.OwnerID, TTL: secondsDuration(params.Arguments.TTLSeconds), TakeoverExpired: params.Arguments.TakeoverExpired, Now: r.currentTime()})
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
			return nil, fault.Invalid("LOCAL_FILE_REQUIRED", "file 参数必填")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: root, PackageFile: params.Arguments.File, OriginRunID: params.Arguments.OriginRun, Now: r.currentTime()})
	case "knowledge_lint":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var report localworkspace.KnowledgeLintReport
		report, err = localworkspace.LintKnowledge(root)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("KNOWLEDGE_LINT_FAILED", "知识库确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "knowledge_query", "knowledge_diagnose":
		var at time.Time
		at, err = parseLocalQueryTime(params.Arguments.At)
		if err != nil {
			break
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if params.Name == "knowledge_query" {
			value, err = localworkspace.QueryKnowledge(localworkspace.QueryKnowledgeOptions{Root: root, Channel: params.Arguments.Channel, At: at})
		} else {
			value, err = localworkspace.DiagnoseKnowledge(root, params.Arguments.Channel, at)
		}
	case "knowledge_pack":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: root, PackID: params.Arguments.PackID, Name: params.Arguments.Name, Now: r.currentTime()})
	case "brief_lint":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var report localworkspace.KnowledgeLintReport
		var brief localworkspace.LocalBrief
		report, brief, err = localworkspace.LintBrief(root, params.Arguments.File)
		value = map[string]any{"brief": brief, "report": report}
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("BRIEF_LINT_FAILED", "V3 创作简报确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_init":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.CreateContentBatch(localworkspace.CreateContentBatchOptions{Root: root, BriefID: params.Arguments.BriefID, DirectionsFile: params.Arguments.DirectionsFile, RequestedCount: params.Arguments.RequestedCount, VariantDimension: params.Arguments.VariantDimension, ControlledDimensions: params.Arguments.ControlledDimensions, BatchID: params.Arguments.BatchID, Now: r.currentTime()})
	case "content_item_lint":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var report localworkspace.ContentItemLintReport
		report, _, err = localworkspace.LintContentItem(root, params.Arguments.File, params.Arguments.BatchFile)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("CONTENT_ITEM_LINT_FAILED", "内容项确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_lint":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var report localworkspace.ContentBatchLintReport
		report, err = localworkspace.LintContentBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("CONTENT_BATCH_LINT_FAILED", "内容批次确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "content_batch_finalize":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.FinalizeContentBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles, r.currentTime())
	case "content_item_diff":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var diff localworkspace.ContentItemDiff
		diff, err = localworkspace.DiffContentItems(root, params.Arguments.BaselineFile, params.Arguments.CandidateFile, params.Arguments.AllowedPaths)
		value = diff
		if err == nil && !diff.Valid {
			diffErr := fault.Invalid("CONTENT_ITEM_REVISION_DRIFT", "内容项修订包含未声明字段变化")
			diffErr.Details = diff
			err = diffErr
		}
	case "delivery_export":
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = localworkspace.ExportApprovedContentItem(root, params.Arguments.ContentItemID, params.Arguments.OutputDirectory, r.currentTime())
	case "article_brief_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleBriefLintReport
		var brief localworkspace.ArticleBrief
		report, brief, err = localworkspace.LintArticleBrief(root, params.Arguments.File)
		value = map[string]any{"brief": brief, "report": report}
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("ARTICLE_BRIEF_LINT_FAILED", "文章简报确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_create":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err == nil {
			value, err = localworkspace.CreateArticleBatch(localworkspace.CreateArticleBatchOptions{Root: root, BriefID: params.Arguments.BriefID, RequestedCount: params.Arguments.RequestedCount, BatchID: params.Arguments.BatchID, Now: r.currentTime()})
		}
	case "article_item_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleItemLintReport
		report, _, err = localworkspace.LintArticleItem(root, params.Arguments.File, params.Arguments.BatchFile)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("ARTICLE_ITEM_LINT_FAILED", "文章内容项确定性校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.ArticleBatchLintReport
		report, err = localworkspace.LintArticleBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("ARTICLE_BATCH_LINT_FAILED", "公众号文章批次校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "article_batch_finalize":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var batch localworkspace.ContentBatch
		var report localworkspace.ArticleBatchLintReport
		batch, report, err = localworkspace.FinalizeArticleBatch(root, params.Arguments.BatchFile, params.Arguments.ContentFiles, r.currentTime())
		value = map[string]any{"batch": batch, "report": report}
	case "article_item_diff":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var diff localworkspace.ArticleItemDiff
		diff, err = localworkspace.DiffArticleItems(root, params.Arguments.BaselineFile, params.Arguments.CandidateFile, params.Arguments.AllowedPaths)
		value = diff
		if err == nil && !diff.Valid {
			diffErr := fault.Invalid("ARTICLE_ITEM_REVISION_DRIFT", "文章内容项修订包含未声明字段变化")
			diffErr.Details = diff
			err = diffErr
		}
	case "wechat_package_export":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err == nil {
			value, err = localworkspace.ExportWeChatPackage(root, params.Arguments.ContentItemID, params.Arguments.OutputDirectory, r.currentTime())
		}
	case "wechat_package_lint":
		var root string
		root, err = r.requireMCPContentType(params.Arguments.Directory, identitydomain.ContentTypeWeChatArticle)
		if err != nil {
			break
		}
		var report localworkspace.WeChatPackageLintReport
		report, err = localworkspace.LintWeChatPackage(root, params.Arguments.File)
		value = report
		if err == nil && !report.Valid {
			lintErr := fault.Invalid("WECHAT_PACKAGE_LINT_FAILED", "公众号交付包校验失败")
			lintErr.Details = report
			err = lintErr
		}
	case "publish_preflight":
		if !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, fault.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, preflight, buildErr := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: params.Arguments.SubmissionType, Files: params.Arguments.Files, DisclosuresFile: params.Arguments.DisclosuresFile, Message: params.Arguments.Message, IdempotencyKey: params.Arguments.IdempotencyKey})
		value, err = map[string]any{"preflight": preflight, "cloud_write": false, "business_files_modified": false}, buildErr
	case "publish_apply":
		if !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, fault.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		var bundle reviewdomain.SubmissionBundle
		var preflight publishPreflight
		bundle, preflight, err = buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: params.Arguments.SubmissionType, Files: params.Arguments.Files, DisclosuresFile: params.Arguments.DisclosuresFile, Message: params.Arguments.Message, IdempotencyKey: params.Arguments.IdempotencyKey})
		if err == nil {
			var revision reviewdomain.SubmissionRevision
			revision, err = r.applyPublishCheckpoint(ctx, root, bundle, preflight, params.Arguments.PlanID, params.Arguments.Accept)
			value = map[string]any{"submission_revision": revision, "preflight": preflight, "cloud_write": err == nil, "business_files_modified": false}
			if err == nil {
				navigationRoot = root
				navigationTarget = &projection.Target{View: "tasks", Focus: &projection.Focus{Kind: "submission_revision", ID: revision.ID, Digest: revision.ContentHash}}
			}
		}
	case "submission_status":
		if strings.TrimSpace(params.Arguments.SubmissionID) == "" {
			return nil, fault.Invalid("SUBMISSION_ID_REQUIRED", "submission_id 必填")
		}
		root, client, _, clientErr := r.workspaceClient(params.Arguments.Directory)
		if clientErr != nil {
			return nil, clientErr
		}
		var details application.SubmissionDetails
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
		var feedback []reviewdomain.ReviewFeedbackBundle
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
		var feedback []reviewdomain.ReviewFeedbackBundle
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
			return nil, fault.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var snapshots []reviewdomain.ApprovedSnapshot
		err = client.Dispatch(ctx, "snapshot.workspace-list", map[string]any{"submission_type": params.Arguments.SubmissionType}, &snapshots)
		value = map[string]any{"count": len(snapshots), "snapshots": snapshots, "cloud_read": true, "business_files_modified": false}
	case "approved_snapshot_pull":
		if params.Arguments.SubmissionType != "" && !validSubmissionType(params.Arguments.SubmissionType) {
			return nil, fault.Invalid("SUBMISSION_TYPE_INVALID", "submission_type 无效")
		}
		root, resolveErr := r.resolveMCPWorkspace(params.Arguments.Directory)
		if resolveErr != nil {
			return nil, resolveErr
		}
		_, client, _, clientErr := r.workspaceClient(root)
		if clientErr != nil {
			return nil, clientErr
		}
		var snapshots []reviewdomain.ApprovedSnapshot
		if strings.TrimSpace(params.Arguments.SnapshotID) != "" {
			var snapshot reviewdomain.ApprovedSnapshot
			err = client.Dispatch(ctx, "snapshot.workspace-show", map[string]any{"id": params.Arguments.SnapshotID}, &snapshot)
			snapshots = []reviewdomain.ApprovedSnapshot{snapshot}
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
		return nil, fault.NotFound("MCP 工具")
	}
	if err != nil {
		return nil, err
	}
	var link *projection.Link
	if navigationTarget != nil {
		built, buildErr := projectViewLinkForWorkspace(navigationRoot, *navigationTarget)
		if buildErr == nil {
			link = &built
		}
	}
	return mcpToolSuccessEnvelope(value, link)
}

func submissionStatusProjectView(details application.SubmissionDetails) projection.Target {
	target := projection.Target{View: "tasks"}
	for _, revision := range details.Revisions {
		if revision.ID != details.Submission.CurrentRevisionID {
			continue
		}
		focused := projection.Target{View: "tasks", Focus: &projection.Focus{Kind: "submission_revision", ID: revision.ID, Digest: revision.ContentHash}}
		if projection.Validate(focused) == nil {
			return focused
		}
		break
	}
	return target
}

func reviewFeedbackProjectView(feedback []reviewdomain.ReviewFeedbackBundle) projection.Target {
	target := projection.Target{View: "tasks"}
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
	focused := projection.Target{View: "tasks", Focus: &projection.Focus{Kind: "submission_revision", ID: revisionID, Digest: digest}}
	if projection.Validate(focused) != nil {
		return target
	}
	return focused
}

func approvedSnapshotsProjectView(snapshots []reviewdomain.ApprovedSnapshot) projection.Target {
	view := "deliveries"
	allKnowledge := len(snapshots) > 0
	for _, snapshot := range snapshots {
		if snapshot.SubmissionType != "knowledge" {
			allKnowledge = false
			break
		}
	}
	if allKnowledge {
		view = "knowledge"
	}
	target := projection.Target{View: view}
	if len(snapshots) != 1 {
		return target
	}
	snapshot := snapshots[0]
	focused := projection.Target{View: view, Focus: &projection.Focus{Kind: "snapshot", ID: snapshot.ID, Digest: snapshot.ContentHash}}
	if projection.Validate(focused) != nil {
		return target
	}
	return focused
}

func projectViewLinkForWorkspace(root string, target projection.Target) (projection.Link, error) {
	status, err := localworkspace.LoadStatus(root)
	if err != nil {
		return projection.Link{}, err
	}
	return projection.Build(status.Binding.ServerURL, status.Binding.ProjectID, target)
}

func mcpToolSuccessEnvelope(value any, link *projection.Link) (map[string]any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if link == nil {
		resources := localWorkspaceResourceLinks(value)
		if len(resources) == 0 {
			return map[string]any{"content": []map[string]string{{"type": "text", "text": string(body)}}, "structuredContent": value, "isError": false}, nil
		}
		content := []map[string]any{{"type": "text", "text": string(body)}}
		for _, resource := range resources {
			content = append(content, resource)
		}
		return map[string]any{"content": content, "structuredContent": value, "isError": false}, nil
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

func localWorkspaceResourceLinks(value any) []map[string]any {
	refs := []localworkspace.WorkspaceResourceRef{}
	switch typed := value.(type) {
	case localworkspace.WorkspaceView:
		refs = typed.Resources
	default:
		return nil
	}
	links := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		links = append(links, map[string]any{
			"type": "resource_link", "uri": ref.URI, "name": ref.Name,
			"description": ref.Digest, "mimeType": ref.MIMEType,
		})
	}
	return links
}

func (r *Root) localWorkbenchManager() *workbench.Manager {
	r.mcpWorkspaceMu.Lock()
	defer r.mcpWorkspaceMu.Unlock()
	if r.proposalStore == nil {
		r.proposalStore = localworkspace.NewProposalStore()
	}
	if r.workbenchManager == nil {
		r.workbenchManager = workbench.NewManagerWithProposalStore(r.currentTime, r.proposalStore)
	}
	return r.workbenchManager
}

func (r *Root) localProposalStore() *localworkspace.ProposalStore {
	r.mcpWorkspaceMu.Lock()
	defer r.mcpWorkspaceMu.Unlock()
	if r.proposalStore == nil {
		r.proposalStore = localworkspace.NewProposalStore()
	}
	return r.proposalStore
}

func workbenchOpenEnvelope(opened workbench.OpenResult) map[string]any {
	body, _ := json.Marshal(opened.Descriptor)
	content := []map[string]any{{"type": "text", "text": string(body)}}
	for _, resource := range localWorkspaceResourceLinks(opened.Descriptor.Fallback) {
		content = append(content, resource)
	}
	return map[string]any{
		"content":           content,
		"structuredContent": opened.Descriptor,
		"isError":           false,
		"_meta": map[string]any{
			"run.zhongcao.contentcloud/browserHandoff": opened.Private,
		},
	}
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
	var domainError *fault.Error
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
		return mcpProjectViewArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_studio_view 参数无效或包含未知字段")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return mcpProjectViewArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_studio_view 参数必须是单个 JSON 对象")
	}
	if params.Name != "contentcloud_open_studio_view" || strings.TrimSpace(params.Arguments.View) == "" {
		return mcpProjectViewArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "contentcloud_open_studio_view 需要 view")
	}
	return params.Arguments, nil
}

func decodeWorkspaceViewArguments(raw json.RawMessage) (mcpWorkspaceViewArguments, error) {
	var params struct {
		Name      string                    `json:"name"`
		Arguments mcpWorkspaceViewArguments `json:"arguments"`
	}
	if err := decodeStrictMCPParams(raw, &params); err != nil {
		return mcpWorkspaceViewArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "workspace_view 参数无效或包含未知字段")
	}
	if params.Name != "workspace_view" || strings.TrimSpace(params.Arguments.View) == "" {
		return mcpWorkspaceViewArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "workspace_view 需要 view")
	}
	return params.Arguments, nil
}

func decodeWorkbenchArguments(raw json.RawMessage, expectedName string) (mcpWorkbenchArguments, error) {
	var params struct {
		Name      string                `json:"name"`
		Arguments mcpWorkbenchArguments `json:"arguments"`
	}
	if err := decodeStrictMCPParams(raw, &params); err != nil {
		return mcpWorkbenchArguments{}, fault.Invalid("MCP_PARAMS_INVALID", expectedName+" 参数无效或包含未知字段")
	}
	if params.Name != expectedName {
		return mcpWorkbenchArguments{}, fault.Invalid("MCP_PARAMS_INVALID", expectedName+" 工具名称无效")
	}
	return params.Arguments, nil
}

func decodeWorkspaceProposalPrepareArguments(raw json.RawMessage) (mcpWorkspaceProposalPrepareArguments, error) {
	var params struct {
		Name      string                               `json:"name"`
		Arguments mcpWorkspaceProposalPrepareArguments `json:"arguments"`
	}
	if err := decodeStrictMCPParams(raw, &params); err != nil || params.Name != "workspace_proposal_prepare" {
		return mcpWorkspaceProposalPrepareArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "workspace_proposal_prepare 参数无效或包含未知字段")
	}
	return params.Arguments, nil
}

func decodeWorkspaceProposalApplyArguments(raw json.RawMessage) (mcpWorkspaceProposalApplyArguments, error) {
	var params struct {
		Name      string                             `json:"name"`
		Arguments mcpWorkspaceProposalApplyArguments `json:"arguments"`
	}
	if err := decodeStrictMCPParams(raw, &params); err != nil || params.Name != "workspace_proposal_apply" {
		return mcpWorkspaceProposalApplyArguments{}, fault.Invalid("MCP_PARAMS_INVALID", "workspace_proposal_apply 参数无效或包含未知字段")
	}
	return params.Arguments, nil
}

func decodeStrictMCPParams(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("MCP 参数必须是单个 JSON 对象")
	}
	return nil
}

func openProjectViewEnvelope(link projection.Link) map[string]any {
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
			{"type": "text", "text": "Content Work OS Studio 页面链接已准备。请在浏览器中打开并核对项目和焦点对象后再报告完成。"},
			mcpProjectViewResourceLink(link),
		},
		"structuredContent": result,
		"isError":           false,
	}
}

func mcpProjectViewResourceLink(link projection.Link) map[string]any {
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
	return contentCloudMCPResourcesWithApps(false)
}

func contentCloudMCPResourcesWithApps(appsSupported bool) []map[string]any {
	resources := []map[string]any{
		{"uri": "contentcloud://workspace/conversation-context", "name": "Content Work OS 工作区对话上下文", "description": "用于开始或继续对话的离线持久化状态", "mimeType": "application/json"},
		{"uri": "contentcloud://workspace/status", "name": "Content Work OS 工作区状态", "description": "离线工作区绑定、环境和同步状态", "mimeType": "application/json"},
	}
	if appsSupported {
		resources = append(resources, map[string]any{
			"uri":         mcpAppsResourceURI,
			"name":        "Content Work OS 本地工作台",
			"description": "在支持 MCP Apps 的宿主沙箱中呈现本地工作区工具结果",
			"mimeType":    mcpAppsMIMEType,
			"_meta":       map[string]any{"ui": map[string]any{"prefersBorder": true}},
		})
	}
	return resources
}

func contentCloudMCPResourceTemplates() []map[string]any {
	return []map[string]any{
		{"uriTemplate": "contentcloud://workspace/files/{path}?digest={sha256}", "name": "Content Work OS 本地文件", "description": "由 workspace_view 返回的摘要固定本地文件；必须包含观察到的 SHA-256", "mimeType": "application/octet-stream"},
	}
}

func (r *Root) readContentCloudMCPResource(raw json.RawMessage) (map[string]any, error) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fault.Invalid("MCP_RESOURCE_PARAMS_INVALID", "MCP 资源参数无效")
	}
	if params.URI == mcpAppsResourceURI {
		if !r.mcpAppsEnabled() {
			return nil, fault.E("not_found", "mcp_app", "MCP_APP_RESOURCE_UNAVAILABLE", "当前 MCP 会话未协商 MCP Apps，不能读取本地工作台 App Resource", 4)
		}
		html, err := workbench.MCPAppHTML()
		if err != nil {
			return nil, err
		}
		return map[string]any{"contents": []map[string]any{{
			"uri":      params.URI,
			"mimeType": mcpAppsMIMEType,
			"text":     html,
			"_meta":    map[string]any{"ui": map[string]any{"prefersBorder": true}},
		}}}, nil
	}
	var value any
	var err error
	switch params.URI {
	case "contentcloud://workspace/conversation-context":
		root, resolveErr := r.resolveMCPWorkspace("")
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.workspaceConversationContext(root)
	case "contentcloud://workspace/status":
		root, resolveErr := r.resolveMCPWorkspace("")
		if resolveErr != nil {
			return nil, resolveErr
		}
		value, err = r.contentCloudWorkspaceStatus(root)
	default:
		root, resolveErr := r.resolveMCPWorkspace("")
		if resolveErr != nil {
			return nil, resolveErr
		}
		resource, readErr := localworkspace.ReadWorkspaceResource(root, params.URI)
		if readErr != nil {
			return nil, readErr
		}
		content := map[string]string{"uri": resource.URI, "mimeType": resource.MIMEType}
		if len(resource.Blob) > 0 {
			content["blob"] = base64.StdEncoding.EncodeToString(resource.Blob)
		} else {
			content["text"] = resource.Text
		}
		return map[string]any{"contents": []map[string]string{content}}, nil
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
	adapter *pluginhost.Adapter
	receipt pluginhost.Receipt
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
		stale := fault.Conflict("ENVIRONMENT_PREPARATION_PLAN_STALE", "环境准备计划与当前环境或执行输入不一致，请重新生成计划")
		stale.Details = map[string]any{"expected_preparation_id": expectedPreparationID, "current_preparation_id": preparation.PreparationID}
		return environmentPreparationApplyResult{}, stale
	}
	if preparation.State == "repair_required" {
		return environmentPreparationApplyResult{}, fault.Policy("ENVIRONMENT_PREPARATION_REPAIR_REQUIRED", "当前能力包存在版本、类型或摘要漂移，禁止原地覆盖", "运行独立环境修复流程后重新生成执行计划")
	}
	if preparation.State != "ready" || len(preparation.Actions) == 0 {
		return environmentPreparationApplyResult{}, fault.Conflict("ENVIRONMENT_PREPARATION_NOT_REQUIRED", "当前执行计划不需要安装任务能力包")
	}
	if !accepted {
		confirmation := fault.Policy("ENVIRONMENT_PREPARATION_CONFIRMATION_REQUIRED", "安装任务能力包、更新环境锁并切换新会话需要明确确认", "展示准备计划的权限、数据流和费用后，使用相同 preparation_id 并传入 accept=true")
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
				if rollbackError := transactions[index].adapter.Rollback(rollbackContext, transactions[index].receipt); rollbackError != nil {
					rollbackErrors = append(rollbackErrors, rollbackError.Error())
				}
			}
		}
		if finishErr := localworkspace.FinishEnvironmentPreparation(verified.Root, lease.Token); finishErr != nil {
			rollbackErrors = append(rollbackErrors, "release preparation lease: "+finishErr.Error())
		}
		if len(rollbackErrors) == 0 {
			return cause
		}
		rollbackFailure := fault.E("runtime", "environment_preparation", "ENVIRONMENT_PREPARATION_ROLLBACK_FAILED", "环境准备失败且局部回滚不完整", 5)
		rollbackFailure.Details = map[string]any{"cause": cause.Error(), "rollback_errors": rollbackErrors}
		return rollbackFailure
	}

	hostID, err := pluginHostForWorkspace(verified.Root)
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	for _, action := range preparation.Actions {
		pluginRuntime, adapterErr := r.bundledPluginRuntime(string(hostID), action.Plugin.ID, action.Plugin.Version)
		if adapterErr != nil {
			return environmentPreparationApplyResult{}, rollback(adapterErr)
		}
		if action.Plugin.ID != pluginRuntime.Package.Manifest.Name || action.Plugin.Version != pluginRuntime.Package.Manifest.Version || action.Plugin.Digest != pluginRuntime.Package.Digest {
			return environmentPreparationApplyResult{}, rollback(fault.Policy("ENVIRONMENT_PLUGIN_ARTIFACT_UNAVAILABLE", "环境准备引用的标准插件包未随当前 CLI 发布", "安装包含该标准包的 ContentCloud CLI 版本后重试"))
		}
		adapterPlan, planErr := pluginRuntime.Adapter.Plan(ctx, pluginRuntime.Package, "install")
		if planErr != nil {
			return environmentPreparationApplyResult{}, rollback(planErr)
		}
		if adapterPlan.State == pluginhost.StatusBlocked {
			blocked := fault.Conflict("ENVIRONMENT_PREPARATION_HOST_STATE_BLOCKED", "宿主中的现有插件状态与签名计划冲突")
			blocked.Details = adapterPlan.BlockingReasons
			return environmentPreparationApplyResult{}, rollback(blocked)
		}
		idempotent := adapterPlan.State == pluginhost.StatusReady
		applyResult, applyErr := pluginRuntime.Adapter.Apply(ctx, pluginRuntime.Package, adapterPlan, true)
		if applyErr != nil {
			return environmentPreparationApplyResult{}, rollback(applyErr)
		}
		if !idempotent {
			transactions = append(transactions, installedPackTransaction{adapter: pluginRuntime.Adapter, receipt: applyResult})
		}
		installs = append(installs, environmentPackInstall{Plugin: action.Plugin, Applied: !idempotent, Idempotent: idempotent})
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
		return environmentPreparationApplyResult{}, rollback(fault.Conflict("ENVIRONMENT_PREPARATION_NOT_READY", "任务能力包安装后，本地执行计划仍未 ready"))
	}
	afterDoctor, err := r.workspaceDoctor(verified.Root)
	if err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	if err := requireHealthyWorkspace(afterDoctor); err != nil {
		return environmentPreparationApplyResult{}, rollback(err)
	}
	handoff, err := environmentPreparationHandoff(verified, hostID)
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

func environmentPreparationHandoff(verified verifiedLocalEnvironment, hostID pluginhost.HostID) (environmentNewChatHandoff, error) {
	for _, plugin := range verified.State.Manifest.Distribution.Plugins {
		if plugin.Kind != "scene_plugin" || plugin.Scope != "environment" || !plugin.Required {
			continue
		}
		prompt := recoveryPrompt(plugin.ID)
		deepLink := ""
		if hostID == pluginhost.HostCodex {
			deepLink = codexNewChatDeepLink(verified.Root, prompt)
		}
		return environmentNewChatHandoff{RequiresNewChat: true, WorkspacePath: verified.Root, DeepLink: deepLink, RecoveryPrompt: prompt}, nil
	}
	return environmentNewChatHandoff{}, fault.Conflict("ENVIRONMENT_SCENE_PLUGIN_REQUIRED", "当前环境缺少恢复新会话所需的必装场景插件")
}

func pluginHostForWorkspace(root string) (pluginhost.HostID, error) {
	status, err := localworkspace.LoadStatus(root)
	if err != nil {
		return "", err
	}
	for _, target := range status.Template.Targets {
		switch target {
		case "codex-plugin":
			return pluginhost.HostCodex, nil
		case "claude-plugin":
			return pluginhost.HostClaude, nil
		}
	}
	return "", fault.Conflict("PLUGIN_HOST_TARGET_MISSING", "工作区未声明 codex-plugin 或 claude-plugin 宿主")
}

func (r *Root) resolveMCPWorkspace(directory string) (string, error) {
	r.mcpWorkspaceMu.Lock()
	defer r.mcpWorkspaceMu.Unlock()

	requested := directory
	if strings.TrimSpace(requested) == "" && r.mcpWorkspaceRoot != "" {
		requested = r.mcpWorkspaceRoot
	}
	roots, _ := r.mcpRootsSnapshot()
	if strings.TrimSpace(requested) == "" {
		switch len(roots) {
		case 1:
			requested = roots[0].Root
		default:
			if len(roots) < 2 {
				break
			}
			conflict := fault.Conflict("MCP_ROOT_SELECTION_REQUIRED", "MCP 客户端提供了多个工作区根，请通过 directory 明确选择一个")
			conflict.Hint = "不能根据多个 roots 猜测客户工作区；请在工具参数中传入对应目录"
			return "", conflict
		}
	}
	resolution, err := localworkspace.ResolveWorkspaceRoot(requested, r.mcpCWD)
	if err != nil {
		return "", err
	}
	if len(roots) > 0 {
		selected := false
		for _, root := range roots {
			if root.Root == resolution.Root {
				selected = true
				break
			}
		}
		if !selected {
			conflict := fault.Conflict("MCP_ROOT_OUTSIDE_DECLARED_ROOTS", "工作区不在 MCP 客户端声明的 roots 内")
			conflict.Hint = "请使用 roots/list 返回的工作区目录，或在宿主中重新打开正确项目"
			return "", conflict
		}
	}
	if r.mcpWorkspaceRoot == "" {
		r.mcpWorkspaceRoot = resolution.Root
		return resolution.Root, nil
	}
	if resolution.Root != r.mcpWorkspaceRoot {
		conflict := fault.Conflict("MCP_WORKSPACE_SESSION_CONFLICT", "当前 MCP 会话已绑定另一个 Content Work OS 工作区")
		conflict.Hint = "为另一个工作区启动独立的 Agent 会话；不要在同一 MCP 子进程中混用客户工作区"
		return "", conflict
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
	err := fault.Conflict("WORKSPACE_DIRECTORY_NOT_EMPTY", "目标目录非空且不是 Content Work OS 工作区")
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
