package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func (r *Root) localCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "local", Short: "运行本地优先的来源、知识和任务工作流"}
	cmd.AddCommand(r.localSourceCommand(), r.localRunCommand(), r.localHandoffCommand(), r.localKnowledgeCommand(), r.localAudienceCommand(), r.localOfferCommand(), r.localBriefCommand(), r.localContentCommand(), r.localArticleCommand(), r.localWeChatCommand(), r.localStoryboardCommand(), r.localSeedanceCommand(), r.localDouyinCommerceCommand())
	return cmd
}

func (r *Root) localArticleCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "article", Short: "创建和治理公众号文章简报、内容批次与文章内容项"}

	brief := &cobra.Command{Use: "brief", Short: "根据可用知识校验文章简报"}
	var briefDirectory string
	briefLint := &cobra.Command{Use: "lint <article-brief.json>", Args: cobra.ExactArgs(1), Short: "校验一份文章简报", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(briefDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, value, err := localworkspace.LintArticleBrief(briefDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("ARTICLE_BRIEF_LINT_FAILED", "文章简报确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.article.brief.lint", map[string]any{"brief": value, "report": report})
	}}
	briefLint.Flags().StringVar(&briefDirectory, "directory", "", "工作区路径；默认为当前目录")
	brief.AddCommand(briefLint)

	batch := &cobra.Command{Use: "batch", Short: "创建、校验并定稿公众号文章批次"}
	var createDirectory, briefID, batchID string
	var requestedCount int
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, Short: "冻结已批准的文章简报和知识快照", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(createDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.CreateArticleBatch(localworkspace.CreateArticleBatchOptions{Root: createDirectory, BriefID: briefID, RequestedCount: requestedCount, BatchID: batchID, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("local.article.batch.create", value)
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "工作区路径；默认为当前目录")
	create.Flags().StringVar(&briefID, "brief", "", "已批准的文章简报对象 ID；默认使用最新的可用文章简报")
	create.Flags().IntVar(&requestedCount, "count", 1, "文章内容候选数量")
	create.Flags().StringVar(&batchID, "id", "", "可选的稳定内容批次 ID")

	var batchLintDirectory, batchLintFile string
	var batchLintFiles []string
	batchLint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "校验批次中的每个文章内容项", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(batchLintDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, err := localworkspace.LintArticleBatch(batchLintDirectory, batchLintFile, batchLintFiles)
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("ARTICLE_BATCH_LINT_FAILED", "公众号文章批次校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.article.batch.lint", report)
	}}
	batchLint.Flags().StringVar(&batchLintDirectory, "directory", "", "工作区路径；默认为当前目录")
	batchLint.Flags().StringVar(&batchLintFile, "batch", "", "相对于工作区的内容批次清单")
	batchLint.Flags().StringSliceVar(&batchLintFiles, "file", nil, "文章内容项 JSON 文件；每个候选项重复传入一次")

	var finalizeDirectory, finalizeBatch string
	var finalizeFiles []string
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, Short: "定稿已经通过校验的文章批次", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(finalizeDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, report, err := localworkspace.FinalizeArticleBatch(finalizeDirectory, finalizeBatch, finalizeFiles, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("local.article.batch.finalize", map[string]any{"batch": value, "report": report})
	}}
	finalize.Flags().StringVar(&finalizeDirectory, "directory", "", "工作区路径；默认为当前目录")
	finalize.Flags().StringVar(&finalizeBatch, "batch", "", "相对于工作区的内容批次清单")
	finalize.Flags().StringSliceVar(&finalizeFiles, "file", nil, "文章内容项 JSON 文件；每个候选项重复传入一次")
	batch.AddCommand(create, batchLint, finalize)

	item := &cobra.Command{Use: "item", Short: "校验并比较文章内容项版本"}
	var itemDirectory, itemBatch string
	itemLint := &cobra.Command{Use: "lint <article-item.json>", Args: cobra.ExactArgs(1), Short: "校验一个文章内容项", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(itemDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, _, err := localworkspace.LintArticleItem(itemDirectory, args[0], itemBatch)
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("ARTICLE_ITEM_LINT_FAILED", "文章内容项确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.article.item.lint", report)
	}}
	itemLint.Flags().StringVar(&itemDirectory, "directory", "", "工作区路径；默认为当前目录")
	itemLint.Flags().StringVar(&itemBatch, "batch", "", "相对于工作区的内容批次清单")

	var diffDirectory, baselineFile, candidateFile string
	var allowedPaths []string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "检查文章内容项版本中未声明的变化", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(diffDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.DiffArticleItems(diffDirectory, baselineFile, candidateFile, allowedPaths)
		if err != nil {
			return err
		}
		if !value.Valid {
			diffErr := domain.Invalid("ARTICLE_ITEM_REVISION_DRIFT", "文章内容项修订包含未声明的字段变化")
			diffErr.Details = value
			return diffErr
		}
		return r.writeOK("local.article.item.diff", value)
	}}
	diff.Flags().StringVar(&diffDirectory, "directory", "", "工作区路径；默认为当前目录")
	diff.Flags().StringVar(&baselineFile, "baseline", "", "相对于工作区的不可变文章内容基线")
	diff.Flags().StringVar(&candidateFile, "candidate", "", "相对于工作区的文章内容修订稿")
	diff.Flags().StringSliceVar(&allowedPaths, "allow", nil, "允许变化的 JSON Pointer 前缀；可重复传入")
	item.AddCommand(itemLint, diff)
	cmd.AddCommand(brief, batch, item)
	return cmd
}

func (r *Root) localWeChatCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "wechat", Short: "构建并校验本地微信公众号交付包"}
	packageCommand := &cobra.Command{Use: "package", Short: "导出或校验微信公众号交付包"}
	var exportDirectory, outputDirectory string
	export := &cobra.Command{Use: "export <approved-article-item-id>", Args: cobra.ExactArgs(1), Short: "导出可直接交给运营人员使用的公众号交付包", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(exportDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.ExportWeChatPackage(exportDirectory, args[0], outputDirectory, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("local.wechat.package.export", value)
	}}
	export.Flags().StringVar(&exportDirectory, "directory", "", "工作区路径；默认为当前目录")
	export.Flags().StringVar(&outputDirectory, "out", "", "相对于工作区的输出目录")
	var lintDirectory string
	lint := &cobra.Command{Use: "lint <package.json>", Args: cobra.ExactArgs(1), Short: "校验交付包文件和摘要", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(lintDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, err := localworkspace.LintWeChatPackage(lintDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("WECHAT_PACKAGE_LINT_FAILED", "公众号交付包校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.wechat.package.lint", report)
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认为当前目录")
	var inspectDirectory, observedFile string
	inspect := &cobra.Command{Use: "inspect-dom <package.json>", Args: cobra.ExactArgs(1), Short: "比较平台粘贴清洗后的 HTML 与交付包 DOM", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(inspectDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, err := localworkspace.InspectWeChatPlatformDOM(inspectDirectory, args[0], observedFile, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("local.wechat.package.inspect_dom", report)
	}}
	inspect.Flags().StringVar(&inspectDirectory, "directory", "", "工作区路径；默认为当前目录")
	inspect.Flags().StringVar(&observedFile, "observed", "", "从微信编辑器复制回来的 HTML 文件")
	_ = inspect.MarkFlagRequired("observed")
	packageCommand.AddCommand(export, lint, inspect)
	cmd.AddCommand(packageCommand)
	return cmd
}

func (r *Root) requireLocalContentType(directory, contentType string) error {
	verifier, err := r.environmentManifestVerifier()
	if err != nil {
		return err
	}
	_, err = localworkspace.RequireContentType(directory, contentType, verifier, r.currentTime())
	return err
}

func (r *Root) requireMCPContentType(directory, contentType string) (string, error) {
	root, err := r.resolveMCPWorkspace(directory)
	if err != nil {
		return "", err
	}
	if err := r.requireLocalContentType(root, contentType); err != nil {
		return "", err
	}
	return root, nil
}

func (r *Root) localBriefCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "brief", Short: "校验本地 V3 创作简报输入"}
	var directory string
	lint := &cobra.Command{Use: "lint <brief.json>", Args: cobra.ExactArgs(1), Short: "根据当前可用知识校验 V3 创作简报", RunE: func(cmd *cobra.Command, args []string) error {
		report, brief, err := localworkspace.LintBrief(directory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("BRIEF_LINT_FAILED", "V3 创作简报确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.brief.lint", map[string]any{"brief": brief, "report": report})
	}}
	lint.Flags().StringVar(&directory, "directory", "", "工作区路径；默认为当前目录")
	cmd.AddCommand(lint)
	return cmd
}

func (r *Root) localContentCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "content", Short: "创建和治理 V3 内容批次与内容项"}
	batch := &cobra.Command{Use: "batch", Short: "创建、校验并定稿本地内容批次清单"}

	var initDirectory, briefID, directionsFile, variant, batchID string
	var requestedCount int
	var controlled []string
	init := &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "把已批准的创作简报和知识快照冻结到内容批次中", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.CreateContentBatch(localworkspace.CreateContentBatchOptions{Root: initDirectory, BriefID: briefID, DirectionsFile: directionsFile, RequestedCount: requestedCount, VariantDimension: variant, ControlledDimensions: controlled, BatchID: batchID, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.content.batch.init", result)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "工作区路径；默认为当前目录")
	init.Flags().StringVar(&briefID, "brief", "", "已批准的创作简报对象 ID；默认使用最新的可用简报")
	init.Flags().StringVar(&directionsFile, "directions", "", "相对于工作区的创作方向 JSON 数组")
	init.Flags().IntVar(&requestedCount, "count", 0, "内容候选数量；默认等于选中的创作方向数量")
	init.Flags().StringVar(&variant, "variant", "hook", "变量维度：hook、audience、scenario、visualization、cta 或 duration")
	init.Flags().StringSliceVar(&controlled, "control", nil, "受控实验维度；可重复传入")
	init.Flags().StringVar(&batchID, "id", "", "可选的稳定内容批次 ID")

	var batchLintDirectory, batchLintFile string
	var batchLintContent []string
	batchLint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "校验批次中的所有内容候选", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.LintContentBatch(batchLintDirectory, batchLintFile, batchLintContent)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("CONTENT_BATCH_LINT_FAILED", "内容批次确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.content.batch.lint", report)
	}}
	batchLint.Flags().StringVar(&batchLintDirectory, "directory", "", "工作区路径；默认为当前目录")
	batchLint.Flags().StringVar(&batchLintFile, "batch", "", "相对于工作区的内容批次 manifest.yaml")
	batchLint.Flags().StringSliceVar(&batchLintContent, "file", nil, "内容项 JSON 文件；每个候选项重复传入一次")

	var finalizeDirectory, finalizeBatch string
	var finalizeContent []string
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, Short: "定稿已经完整通过校验的本地内容批次", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.FinalizeContentBatch(finalizeDirectory, finalizeBatch, finalizeContent, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.content.batch.finalize", result)
	}}
	finalize.Flags().StringVar(&finalizeDirectory, "directory", "", "工作区路径；默认为当前目录")
	finalize.Flags().StringVar(&finalizeBatch, "batch", "", "相对于工作区的内容批次 manifest.yaml")
	finalize.Flags().StringSliceVar(&finalizeContent, "file", nil, "内容项 JSON 文件；每个候选项重复传入一次")
	batch.AddCommand(init, batchLint, finalize)

	var lintDirectory, lintBatch string
	lint := &cobra.Command{Use: "lint <content-item.json>", Args: cobra.ExactArgs(1), Short: "根据已冻结的内容批次上下文校验一个内容项", RunE: func(cmd *cobra.Command, args []string) error {
		report, _, err := localworkspace.LintContentItem(lintDirectory, args[0], lintBatch)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("CONTENT_ITEM_LINT_FAILED", "内容项确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.content.item.lint", report)
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认为当前目录")
	lint.Flags().StringVar(&lintBatch, "batch", "", "相对于工作区的内容批次 manifest.yaml；省略时根据 content_batch_id 推断")

	var diffDirectory, baselineFile, candidateFile string
	var allowedPaths []string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "检查版本或单变量实验中未声明的变化", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.DiffContentItems(diffDirectory, baselineFile, candidateFile, allowedPaths)
		if err != nil {
			return err
		}
		if !result.Valid {
			err := domain.Invalid("CONTENT_ITEM_REVISION_DRIFT", "内容项修订包含未声明的字段变化")
			err.Details = result
			return err
		}
		return r.writeOK("local.content.item.diff", result)
	}}
	diff.Flags().StringVar(&diffDirectory, "directory", "", "工作区路径；默认为当前目录")
	diff.Flags().StringVar(&baselineFile, "baseline", "", "相对于工作区的不可变内容项基线")
	diff.Flags().StringVar(&candidateFile, "candidate", "", "相对于工作区的内容项修订稿")
	diff.Flags().StringSliceVar(&allowedPaths, "allow", nil, "允许变化的 JSON Pointer 前缀；可重复传入")

	var exportDirectory, outputDirectory string
	export := &cobra.Command{Use: "export <approved-content-item-id>", Args: cobra.ExactArgs(1), Short: "把已批准内容项导出为 JSON、Markdown 和 XLSX", RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := localworkspace.ExportApprovedContentItem(exportDirectory, args[0], outputDirectory, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.content.delivery.export", manifest)
	}}
	export.Flags().StringVar(&exportDirectory, "directory", "", "工作区路径；默认为当前目录")
	export.Flags().StringVar(&outputDirectory, "out", "", "相对于工作区的输出目录")

	cmd.AddCommand(batch, lint, diff, export)
	return cmd
}

func (r *Root) localSourceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "在本地工作区登记并导入不可变来源文件"}
	var directory, id, title, sourceKind, storageMode string
	register := &cobra.Command{Use: "register <file>", Args: cobra.ExactArgs(1), Short: "根据不可变 SHA-256 摘要登记本地来源", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: directory, File: args[0], ID: id, Title: title, SourceKind: sourceKind, StorageMode: storageMode, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.source.register", value)
	}}
	register.Flags().StringVar(&directory, "directory", "", "工作区路径；默认为当前目录")
	register.Flags().StringVar(&id, "id", "", "稳定的本地来源 ID")
	register.Flags().StringVar(&title, "title", "", "来源标题")
	register.Flags().StringVar(&sourceKind, "kind", "customer_material", "来源类型")
	register.Flags().StringVar(&storageMode, "storage", "copy", "保存方式：copy 或 reference")

	var listDirectory string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "列出本地已登记来源", RunE: func(cmd *cobra.Command, args []string) error {
		values, err := localworkspace.LocalSources(listDirectory)
		if err != nil {
			return err
		}
		return r.writeOK("local.source.list", map[string]any{"count": len(values), "sources": values})
	}}
	list.Flags().StringVar(&listDirectory, "directory", "", "工作区路径；默认为当前目录")

	var showDirectory string
	show := &cobra.Command{Use: "show <source-id>", Args: cobra.ExactArgs(1), Short: "显示一个本地来源", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.LocalSourceByID(showDirectory, args[0])
		if err != nil {
			return err
		}
		return r.writeOK("local.source.show", value)
	}}
	show.Flags().StringVar(&showDirectory, "directory", "", "工作区路径；默认为当前目录")

	var ingestDirectory string
	ingest := &cobra.Command{Use: "ingest <source-id>", Args: cobra.ExactArgs(1), Short: "把一个来源解析为可精确定位的本地证据片段", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.IngestLocalSource(ingestDirectory, args[0], time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.source.ingest", value)
	}}
	ingest.Flags().StringVar(&ingestDirectory, "directory", "", "工作区路径；默认为当前目录")

	var verifyDirectory string
	verify := &cobra.Command{Use: "verify", Args: cobra.NoArgs, Short: "校验来源文件、摘要和检测到的 MIME 类型", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.VerifyLocalSources(verifyDirectory)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("LOCAL_SOURCE_VERIFY_FAILED", "本地来源完整性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.source.verify", report)
	}}
	verify.Flags().StringVar(&verifyDirectory, "directory", "", "工作区路径；默认为当前目录")

	cmd.AddCommand(register, list, show, ingest, verify)
	return cmd
}

func (r *Root) localRunCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "管理可恢复的本地运行及其阶段检查"}
	var initDirectory, runID, intent string
	var sourceRefs []string
	var withIngest bool
	init := &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "初始化本地导入、查询或内容任务", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: initDirectory, RunID: runID, Intent: intent, InputIDs: sourceRefs, WithIngest: withIngest, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.init", value)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "工作区路径；默认为当前目录")
	init.Flags().StringVar(&runID, "id", "", "可选的稳定运行 ID")
	init.Flags().StringVar(&intent, "intent", "intent:content", "稳定的意图 ID，例如 intent:content")
	init.Flags().StringSliceVar(&sourceRefs, "input", nil, "已登记的不可变来源 ID；可重复传入")
	init.Flags().BoolVar(&withIngest, "with-ingest", false, "从导入阶段开始")

	var showDirectory string
	show := &cobra.Command{Use: "show [run-id]", Args: cobra.MaximumNArgs(1), Short: "显示指定运行，或当前运行", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ShowLocalRun(showDirectory, optionalValue(args))
		if err != nil {
			return err
		}
		return r.writeOK("local.run.show", value)
	}}
	show.Flags().StringVar(&showDirectory, "directory", "", "工作区路径；默认为当前目录")

	var recordDirectory, recordRunID, recordClaimToken string
	var recordRevision uint64
	var recordSourceRefs, changedIDs, eligibleIDs, blockedIDs, findings, outputPaths []string
	record := &cobra.Command{Use: "record", Args: cobra.NoArgs, Short: "在当前运行中记录不可变引用和输出", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RecordClaimedLocalRun(localworkspace.RecordLocalRunOptions{Root: recordDirectory, RunID: recordRunID, ClaimToken: recordClaimToken, ExpectedRevision: recordRevision, InputIDs: recordSourceRefs, ChangedIDs: changedIDs, EligibleIDs: eligibleIDs, BlockedIDs: blockedIDs, Findings: findings, OutputPaths: outputPaths, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.record", value)
	}}
	addLocalRunRecordFlags(record, &recordDirectory, &recordRunID, &recordSourceRefs, &changedIDs, &eligibleIDs, &blockedIDs, &findings, &outputPaths)
	record.Flags().StringVar(&recordClaimToken, "claim-token", "", "有效的本地运行锁凭据")
	record.Flags().Uint64Var(&recordRevision, "revision", 0, "预期的本地运行上下文版本")

	var checkDirectory, checkRunID, checkName, checkStatus, checkCommand, checkDetail, checkClaimToken string
	var checkRevision uint64
	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "记录一项确定性阶段检查", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CheckClaimedLocalRun(localworkspace.CheckLocalRunOptions{Root: checkDirectory, RunID: checkRunID, ClaimToken: checkClaimToken, ExpectedRevision: checkRevision, Name: checkName, Status: checkStatus, Command: checkCommand, Detail: checkDetail, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.check", value)
	}}
	check.Flags().StringVar(&checkDirectory, "directory", "", "工作区路径；默认为当前目录")
	check.Flags().StringVar(&checkRunID, "run", "", "运行 ID；默认为当前运行")
	check.Flags().StringVar(&checkName, "name", "", "检查名称，例如 kb-lint 或 content-lint")
	check.Flags().StringVar(&checkStatus, "status", "", "检查状态：passed 或 failed")
	check.Flags().StringVar(&checkCommand, "command", "", "生成结果的确定性命令")
	check.Flags().StringVar(&checkDetail, "detail", "", "简短的检查说明")
	check.Flags().StringVar(&checkClaimToken, "claim-token", "", "有效的本地运行锁凭据")
	check.Flags().Uint64Var(&checkRevision, "revision", 0, "预期的本地运行上下文版本")

	var advanceDirectory, advanceRunID, advanceClaimToken string
	var advanceRevision uint64
	var advanceSourceRefs, advanceChanged, advanceEligible, advanceBlocked, advanceFindings, advanceOutputs []string
	advance := &cobra.Command{Use: "advance <stage>", Args: cobra.ExactArgs(1), Short: "通过已经校验的阶段交接继续推进", RunE: func(cmd *cobra.Command, args []string) error {
		additions := localworkspace.RecordLocalRunOptions{ClaimToken: advanceClaimToken, ExpectedRevision: advanceRevision, InputIDs: advanceSourceRefs, ChangedIDs: advanceChanged, EligibleIDs: advanceEligible, BlockedIDs: advanceBlocked, Findings: advanceFindings, OutputPaths: advanceOutputs}
		value, err := localworkspace.AdvanceClaimedLocalRun(advanceDirectory, advanceRunID, args[0], additions, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.advance", value)
	}}
	addLocalRunRecordFlags(advance, &advanceDirectory, &advanceRunID, &advanceSourceRefs, &advanceChanged, &advanceEligible, &advanceBlocked, &advanceFindings, &advanceOutputs)
	advance.Flags().StringVar(&advanceClaimToken, "claim-token", "", "有效的本地运行锁凭据")
	advance.Flags().Uint64Var(&advanceRevision, "revision", 0, "预期的本地运行上下文版本")

	var resumeDirectory, resumeRunID, resumeClaimToken string
	var resumeRevision uint64
	resume := &cobra.Command{Use: "resume", Args: cobra.NoArgs, Short: "从原阶段恢复失败的运行", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ResumeClaimedLocalRun(resumeDirectory, resumeRunID, resumeClaimToken, resumeRevision, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.resume", value)
	}}
	resume.Flags().StringVar(&resumeDirectory, "directory", "", "工作区路径；默认为当前目录")
	resume.Flags().StringVar(&resumeRunID, "run", "", "运行 ID；默认为当前运行")
	resume.Flags().StringVar(&resumeClaimToken, "claim-token", "", "有效的本地运行锁凭据")
	resume.Flags().Uint64Var(&resumeRevision, "revision", 0, "预期的本地运行上下文版本")

	var failDirectory, failRunID, failClaimToken string
	var failRevision uint64
	var failFindings []string
	fail := &cobra.Command{Use: "fail", Args: cobra.NoArgs, Short: "将运行标记为失败，并记录可执行问题", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.FailClaimedLocalRun(failDirectory, failRunID, failFindings, failClaimToken, failRevision, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.fail", value)
	}}
	fail.Flags().StringVar(&failDirectory, "directory", "", "工作区路径；默认为当前目录")
	fail.Flags().StringVar(&failRunID, "run", "", "运行 ID；默认为当前运行")
	fail.Flags().StringSliceVar(&failFindings, "finding", nil, "失败问题；可重复传入")
	fail.Flags().StringVar(&failClaimToken, "claim-token", "", "有效的本地运行锁凭据")
	fail.Flags().Uint64Var(&failRevision, "revision", 0, "预期的本地运行上下文版本")

	var validateDirectory string
	validate := &cobra.Command{Use: "validate", Args: cobra.NoArgs, Short: "校验所有本地运行上下文和当前运行指针", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.ValidateLocalRuns(validateDirectory)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("LOCAL_RUN_VALIDATE_FAILED", "本地运行上下文校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.run.validate", report)
	}}
	validate.Flags().StringVar(&validateDirectory, "directory", "", "工作区路径；默认为当前目录")

	var claimDirectory, claimRunID, claimOwnerKind, claimOwnerID string
	var claimRevision uint64
	var claimTTL time.Duration
	var takeoverExpired bool
	claim := &cobra.Command{Use: "claim", Args: cobra.NoArgs, Short: "取得本地运行版本的单写入者锁", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ClaimRun(localworkspace.ClaimRunOptions{Root: claimDirectory, RunID: claimRunID, OwnerKind: claimOwnerKind, OwnerID: claimOwnerID, ExpectedRevision: claimRevision, TTL: claimTTL, TakeoverExpired: takeoverExpired, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.claim", value)
	}}
	claim.Flags().StringVar(&claimDirectory, "directory", "", "工作区路径；默认为当前目录")
	claim.Flags().StringVar(&claimRunID, "run", "", "运行 ID")
	claim.Flags().StringVar(&claimOwnerKind, "owner-kind", "agent", "持有者类型：agent 或 browser")
	claim.Flags().StringVar(&claimOwnerID, "owner-id", "", "对话、工作进程或 Workbench 的稳定持有者 ID")
	claim.Flags().Uint64Var(&claimRevision, "revision", 0, "预期的本地运行上下文版本")
	claim.Flags().DurationVar(&claimTTL, "ttl", 30*time.Minute, "运行锁有效期；最长 4 小时")
	claim.Flags().BoolVar(&takeoverExpired, "takeover-expired", false, "明确接管已经过期的运行锁")

	var takeoverDirectory, takeoverRunID, takeoverOwnerKind, takeoverOwnerID, expectedOwnerKind, expectedOwnerID string
	var takeoverRevision, expectedEpoch uint64
	var takeoverTTL time.Duration
	takeover := &cobra.Command{Use: "takeover", Args: cobra.NoArgs, Short: "按 owner 和 epoch 明确接管仍有效的运行锁", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.TakeoverRunClaim(localworkspace.TakeoverRunClaimOptions{
			Root: takeoverDirectory, RunID: takeoverRunID, OwnerKind: takeoverOwnerKind, OwnerID: takeoverOwnerID,
			ExpectedOwnerKind: expectedOwnerKind, ExpectedOwnerID: expectedOwnerID, ExpectedEpoch: expectedEpoch,
			ExpectedRevision: takeoverRevision, TTL: takeoverTTL, Now: time.Now(),
		})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.takeover", value)
	}}
	takeover.Flags().StringVar(&takeoverDirectory, "directory", "", "工作区路径；默认为当前目录")
	takeover.Flags().StringVar(&takeoverRunID, "run", "", "运行 ID")
	takeover.Flags().StringVar(&takeoverOwnerKind, "owner-kind", "agent", "新持有者类型：agent 或 browser")
	takeover.Flags().StringVar(&takeoverOwnerID, "owner-id", "", "新持有者 ID")
	takeover.Flags().StringVar(&expectedOwnerKind, "expected-owner-kind", "", "当前持有者类型")
	takeover.Flags().StringVar(&expectedOwnerID, "expected-owner-id", "", "当前持有者 ID")
	takeover.Flags().Uint64Var(&expectedEpoch, "expected-epoch", 0, "当前运行锁 epoch")
	takeover.Flags().Uint64Var(&takeoverRevision, "revision", 0, "预期的本地运行上下文版本")
	takeover.Flags().DurationVar(&takeoverTTL, "ttl", 30*time.Minute, "新运行锁有效期；最长 4 小时")

	var renewDirectory, renewRunID, renewToken string
	var renewTTL time.Duration
	renew := &cobra.Command{Use: "renew", Args: cobra.NoArgs, Short: "续期有效的本地运行锁", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RenewRunClaim(renewDirectory, renewRunID, renewToken, renewTTL, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.renew", value)
	}}
	renew.Flags().StringVar(&renewDirectory, "directory", "", "工作区路径；默认为当前目录")
	renew.Flags().StringVar(&renewRunID, "run", "", "运行 ID")
	renew.Flags().StringVar(&renewToken, "claim-token", "", "有效的本地运行锁凭据")
	renew.Flags().DurationVar(&renewTTL, "ttl", 30*time.Minute, "续期后的运行锁有效期；最长 4 小时")

	var releaseDirectory, releaseRunID, releaseToken string
	release := &cobra.Command{Use: "release", Args: cobra.NoArgs, Short: "释放有效的本地运行锁", RunE: func(cmd *cobra.Command, args []string) error {
		if err := localworkspace.ReleaseRunClaim(releaseDirectory, releaseRunID, releaseToken, time.Now()); err != nil {
			return err
		}
		return r.writeOK("local.run.release", map[string]any{"run_id": releaseRunID, "released": true})
	}}
	release.Flags().StringVar(&releaseDirectory, "directory", "", "工作区路径；默认为当前目录")
	release.Flags().StringVar(&releaseRunID, "run", "", "运行 ID")
	release.Flags().StringVar(&releaseToken, "claim-token", "", "有效的本地运行锁凭据")

	var claimStatusDirectory, claimStatusRunID string
	claimStatus := &cobra.Command{Use: "claim-status", Args: cobra.NoArgs, Short: "读取不含凭据的本地运行锁状态", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RunClaimStatus(claimStatusDirectory, claimStatusRunID, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.claim-status", value)
	}}
	claimStatus.Flags().StringVar(&claimStatusDirectory, "directory", "", "工作区路径；默认为当前目录")
	claimStatus.Flags().StringVar(&claimStatusRunID, "run", "", "运行 ID")

	cmd.AddCommand(init, show, record, check, advance, resume, fail, validate, claim, takeover, renew, release, claimStatus)
	return cmd
}

func (r *Root) localHandoffCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "handoff", Short: "创建和接收经过摘要校验的跨对话交接"}

	var createDirectory, createID, createRunID, createToken, nextCapability, nextAction string
	var createRevision uint64
	var inputPaths, blockers, pendingDecisions []string
	create := &cobra.Command{Use: "create-ready", Args: cobra.NoArgs, Short: "把已锁定运行保存为待接手交接，并释放运行锁", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CreateReadyHandoff(localworkspace.CreateReadyHandoffOptions{Root: createDirectory, HandoffID: createID, RunID: createRunID, ClaimToken: createToken, ExpectedRevision: createRevision, NextCapabilityID: nextCapability, NextAction: nextAction, InputPaths: inputPaths, Blockers: blockers, PendingDecisions: pendingDecisions, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.create-ready", value)
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "工作区路径；默认为当前目录")
	create.Flags().StringVar(&createID, "id", "", "可选的稳定交接 ID")
	create.Flags().StringVar(&createRunID, "run", "", "运行 ID")
	create.Flags().StringVar(&createToken, "claim-token", "", "有效的本地运行锁凭据")
	create.Flags().Uint64Var(&createRevision, "revision", 0, "预期的本地运行上下文版本")
	create.Flags().StringVar(&nextCapability, "next-capability", "", "下一项稳定能力 ID")
	create.Flags().StringVar(&nextAction, "next-action", "", "简短且可执行的后续操作说明")
	create.Flags().StringSliceVar(&inputPaths, "input", nil, "相对于工作区的检查点输入；可重复传入")
	create.Flags().StringSliceVar(&blockers, "blocker", nil, "需要保留的阻断项；可重复传入")
	create.Flags().StringSliceVar(&pendingDecisions, "pending-decision", nil, "待处理决定；可重复传入")

	var listDirectory string
	list := &cobra.Command{Use: "list-ready", Args: cobra.NoArgs, Short: "列出待接手交接记录，但不锁定", RunE: func(cmd *cobra.Command, args []string) error {
		values, err := localworkspace.ListReadyHandoffs(listDirectory)
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.list-ready", map[string]any{"count": len(values), "handoffs": values})
	}}
	list.Flags().StringVar(&listDirectory, "directory", "", "工作区路径；默认为当前目录")

	var acceptDirectory, acceptID, acceptOwnerKind, acceptOwnerID string
	var acceptTTL time.Duration
	var acceptTakeover bool
	accept := &cobra.Command{Use: "accept", Args: cobra.NoArgs, Short: "原子校验待接手交接记录并锁定对应运行", RunE: func(cmd *cobra.Command, args []string) error {
		handoff, claim, err := localworkspace.AcceptHandoff(localworkspace.AcceptHandoffOptions{Root: acceptDirectory, HandoffID: acceptID, OwnerKind: acceptOwnerKind, OwnerID: acceptOwnerID, TTL: acceptTTL, TakeoverExpired: acceptTakeover, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.accept", map[string]any{"handoff": handoff, "claim": claim})
	}}
	accept.Flags().StringVar(&acceptDirectory, "directory", "", "工作区路径；默认为当前目录")
	accept.Flags().StringVar(&acceptID, "id", "", "待接手交接 ID")
	accept.Flags().StringVar(&acceptOwnerKind, "owner-kind", "agent", "接手者类型：agent 或 browser")
	accept.Flags().StringVar(&acceptOwnerID, "owner-id", "", "接手对话、工作进程或 Workbench 的持有者 ID")
	accept.Flags().DurationVar(&acceptTTL, "ttl", 30*time.Minute, "运行锁有效期；最长 4 小时")
	accept.Flags().BoolVar(&acceptTakeover, "takeover-expired", false, "明确接管已经过期的运行锁")

	var completeDirectory, completeID, completeToken string
	complete := &cobra.Command{Use: "complete", Args: cobra.NoArgs, Short: "将已接手交接记录标记为完成", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CompleteHandoff(completeDirectory, completeID, completeToken, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.complete", value)
	}}
	complete.Flags().StringVar(&completeDirectory, "directory", "", "工作区路径；默认为当前目录")
	complete.Flags().StringVar(&completeID, "id", "", "已接手交接 ID")
	complete.Flags().StringVar(&completeToken, "claim-token", "", "有效的本地运行锁凭据")

	var supersedeDirectory, supersedeID string
	supersede := &cobra.Command{Use: "supersede", Args: cobra.NoArgs, Short: "取代一条待接手交接记录", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.SupersedeReadyHandoff(supersedeDirectory, supersedeID, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.supersede", value)
	}}
	supersede.Flags().StringVar(&supersedeDirectory, "directory", "", "工作区路径；默认为当前目录")
	supersede.Flags().StringVar(&supersedeID, "id", "", "待接手交接 ID")

	cmd.AddCommand(create, list, accept, complete, supersede)
	return cmd
}

func (r *Root) localKnowledgeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "knowledge", Short: "导入、校验、查询、诊断并打包本地受治理知识"}
	var importDirectory, originRun string
	importCandidates := &cobra.Command{Use: "import <knowledge-candidates.json>", Args: cobra.ExactArgs(1), Short: "导入有证据依据的 knowledge-candidates/1.0 数据", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: importDirectory, PackageFile: args[0], OriginRunID: originRun, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.import", report)
	}}
	importCandidates.Flags().StringVar(&importDirectory, "directory", "", "工作区路径；默认为当前目录")
	importCandidates.Flags().StringVar(&originRun, "run", "", "来源本地运行 ID")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "校验 ID、证据、状态、决定、依赖、权利和冲突", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.LintKnowledge(lintDirectory)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("KNOWLEDGE_LINT_FAILED", "知识库确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.knowledge.lint", report)
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "工作区路径；默认为当前目录")

	var queryDirectory, queryChannel, queryAt string
	query := &cobra.Command{Use: "query", Args: cobra.NoArgs, Short: "把知识查询结果分为可用、已阻断和仅供参考", RunE: func(cmd *cobra.Command, args []string) error {
		at, err := parseLocalQueryTime(queryAt)
		if err != nil {
			return err
		}
		result, err := localworkspace.QueryKnowledge(localworkspace.QueryKnowledgeOptions{Root: queryDirectory, Channel: queryChannel, At: at})
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.query", result)
	}}
	query.Flags().StringVar(&queryDirectory, "directory", "", "工作区路径；默认为当前目录")
	query.Flags().StringVar(&queryChannel, "channel", "", "目标内容渠道")
	query.Flags().StringVar(&queryAt, "at", "", "按 RFC3339 格式指定可用性判断时间；默认为当前时间")

	var diagnoseDirectory, diagnoseChannel, diagnoseAt string
	diagnose := &cobra.Command{Use: "diagnose", Args: cobra.NoArgs, Short: "生成 15 个维度的素材覆盖诊断", RunE: func(cmd *cobra.Command, args []string) error {
		at, err := parseLocalQueryTime(diagnoseAt)
		if err != nil {
			return err
		}
		result, err := localworkspace.DiagnoseKnowledge(diagnoseDirectory, diagnoseChannel, at)
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.diagnose", result)
	}}
	diagnose.Flags().StringVar(&diagnoseDirectory, "directory", "", "工作区路径；默认为当前目录")
	diagnose.Flags().StringVar(&diagnoseChannel, "channel", "", "目标内容渠道")
	diagnose.Flags().StringVar(&diagnoseAt, "at", "", "按 RFC3339 格式指定诊断时间；默认为当前时间")

	var packDirectory, packID, packName string
	pack := &cobra.Command{Use: "pack", Args: cobra.NoArgs, Short: "构建七层知识审核包和证据披露", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: packDirectory, PackID: packID, Name: packName, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.pack", result)
	}}
	pack.Flags().StringVar(&packDirectory, "directory", "", "工作区路径；默认为当前目录")
	pack.Flags().StringVar(&packID, "id", "", "稳定的知识包 ID；默认使用内容摘要 ID")
	pack.Flags().StringVar(&packName, "name", "", "便于阅读的知识包名称")

	cmd.AddCommand(importCandidates, lint, query, diagnose, pack)
	return cmd
}

func addLocalRunRecordFlags(command *cobra.Command, directory, runID *string, sourceRefs, changedIDs, eligibleIDs, blockedIDs, findings, outputPaths *[]string) {
	command.Flags().StringVar(directory, "directory", "", "工作区路径；默认为当前目录")
	command.Flags().StringVar(runID, "run", "", "运行 ID；默认为当前运行")
	command.Flags().StringSliceVar(sourceRefs, "source-ref", nil, "来源 ID；可重复传入")
	command.Flags().StringSliceVar(changedIDs, "changed-id", nil, "已变化对象 ID；可重复传入")
	command.Flags().StringSliceVar(eligibleIDs, "eligible-id", nil, "可用知识 ID；可重复传入")
	command.Flags().StringSliceVar(blockedIDs, "blocked-id", nil, "已阻断知识 ID；可重复传入")
	command.Flags().StringSliceVar(findings, "finding", nil, "问题记录；可重复传入")
	command.Flags().StringSliceVar(outputPaths, "output-path", nil, "相对于工作区的输出路径；可重复传入")
}

func parseLocalQueryTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, domain.Invalid("TIME_INVALID", "--at 必须是 RFC3339 时间")
	}
	return parsed, nil
}

func optionalValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
