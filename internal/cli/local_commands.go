package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

func (r *Root) localCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "local", Short: "Run client-first source, knowledge, and LocalRun workflows"}
	cmd.AddCommand(r.localSourceCommand(), r.localRunCommand(), r.localHandoffCommand(), r.localKnowledgeCommand(), r.localAudienceCommand(), r.localOfferCommand(), r.localBriefCommand(), r.localContentCommand(), r.localArticleCommand(), r.localWeChatCommand(), r.localStoryboardCommand(), r.localSeedanceCommand())
	return cmd
}

func (r *Root) localArticleCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "article", Short: "Create and govern WeChat ArticleBrief, ContentBatch, and ArticleItem objects"}

	brief := &cobra.Command{Use: "brief", Short: "Validate an ArticleBrief against eligible knowledge"}
	var briefDirectory string
	briefLint := &cobra.Command{Use: "lint <article-brief.json>", Args: cobra.ExactArgs(1), Short: "Validate one ArticleBrief", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(briefDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, value, err := localworkspace.LintArticleBrief(briefDirectory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("ARTICLE_BRIEF_LINT_FAILED", "ArticleBrief 确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.article.brief.lint", map[string]any{"brief": value, "report": report})
	}}
	briefLint.Flags().StringVar(&briefDirectory, "directory", "", "workspace path; defaults to current directory")
	brief.AddCommand(briefLint)

	batch := &cobra.Command{Use: "batch", Short: "Create, lint, and finalize WeChat article batches"}
	var createDirectory, briefID, batchID string
	var requestedCount int
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, Short: "Freeze an approved ArticleBrief and Knowledge snapshot", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(createDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.CreateArticleBatch(localworkspace.CreateArticleBatchOptions{Root: createDirectory, BriefID: briefID, RequestedCount: requestedCount, BatchID: batchID, Now: r.currentTime()})
		if err != nil {
			return err
		}
		return r.writeOK("local.article.batch.create", value)
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "workspace path; defaults to current directory")
	create.Flags().StringVar(&briefID, "brief", "", "approved ArticleBrief object ID; defaults to newest eligible ArticleBrief")
	create.Flags().IntVar(&requestedCount, "count", 1, "number of ArticleItem candidates")
	create.Flags().StringVar(&batchID, "id", "", "optional stable ContentBatch ID")

	var batchLintDirectory, batchLintFile string
	var batchLintFiles []string
	batchLint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "Validate every ArticleItem in a batch", RunE: func(cmd *cobra.Command, args []string) error {
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
	batchLint.Flags().StringVar(&batchLintDirectory, "directory", "", "workspace path; defaults to current directory")
	batchLint.Flags().StringVar(&batchLintFile, "batch", "", "workspace-relative ContentBatch manifest")
	batchLint.Flags().StringSliceVar(&batchLintFiles, "file", nil, "ArticleItem JSON file; repeat for every candidate")

	var finalizeDirectory, finalizeBatch string
	var finalizeFiles []string
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, Short: "Finalize a validated article batch", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(finalizeDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, report, err := localworkspace.FinalizeArticleBatch(finalizeDirectory, finalizeBatch, finalizeFiles, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("local.article.batch.finalize", map[string]any{"batch": value, "report": report})
	}}
	finalize.Flags().StringVar(&finalizeDirectory, "directory", "", "workspace path; defaults to current directory")
	finalize.Flags().StringVar(&finalizeBatch, "batch", "", "workspace-relative ContentBatch manifest")
	finalize.Flags().StringSliceVar(&finalizeFiles, "file", nil, "ArticleItem JSON file; repeat for every candidate")
	batch.AddCommand(create, batchLint, finalize)

	item := &cobra.Command{Use: "item", Short: "Validate and compare ArticleItem revisions"}
	var itemDirectory, itemBatch string
	itemLint := &cobra.Command{Use: "lint <article-item.json>", Args: cobra.ExactArgs(1), Short: "Validate one ArticleItem", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(itemDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		report, _, err := localworkspace.LintArticleItem(itemDirectory, args[0], itemBatch)
		if err != nil {
			return err
		}
		if !report.Valid {
			lintErr := domain.Invalid("ARTICLE_ITEM_LINT_FAILED", "ArticleItem 确定性校验失败")
			lintErr.Details = report
			return lintErr
		}
		return r.writeOK("local.article.item.lint", report)
	}}
	itemLint.Flags().StringVar(&itemDirectory, "directory", "", "workspace path; defaults to current directory")
	itemLint.Flags().StringVar(&itemBatch, "batch", "", "workspace-relative ContentBatch manifest")

	var diffDirectory, baselineFile, candidateFile string
	var allowedPaths []string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "Detect undeclared ArticleItem revision drift", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(diffDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.DiffArticleItems(diffDirectory, baselineFile, candidateFile, allowedPaths)
		if err != nil {
			return err
		}
		if !value.Valid {
			diffErr := domain.Invalid("ARTICLE_ITEM_REVISION_DRIFT", "ArticleItem 修订包含未声明字段变化")
			diffErr.Details = value
			return diffErr
		}
		return r.writeOK("local.article.item.diff", value)
	}}
	diff.Flags().StringVar(&diffDirectory, "directory", "", "workspace path; defaults to current directory")
	diff.Flags().StringVar(&baselineFile, "baseline", "", "workspace-relative immutable baseline ArticleItem")
	diff.Flags().StringVar(&candidateFile, "candidate", "", "workspace-relative revised ArticleItem")
	diff.Flags().StringSliceVar(&allowedPaths, "allow", nil, "allowed JSON Pointer prefix; repeat as needed")
	item.AddCommand(itemLint, diff)
	cmd.AddCommand(brief, batch, item)
	return cmd
}

func (r *Root) localWeChatCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "wechat", Short: "Build and validate local WeChat Official Account delivery packages"}
	packageCommand := &cobra.Command{Use: "package", Short: "Export or validate a WeChat delivery package"}
	var exportDirectory, outputDirectory string
	export := &cobra.Command{Use: "export <approved-article-item-id>", Args: cobra.ExactArgs(1), Short: "Export a local operator-ready WeChat package", RunE: func(cmd *cobra.Command, args []string) error {
		if err := r.requireLocalContentType(exportDirectory, domain.ContentTypeWeChatArticle); err != nil {
			return err
		}
		value, err := localworkspace.ExportWeChatPackage(exportDirectory, args[0], outputDirectory, r.currentTime())
		if err != nil {
			return err
		}
		return r.writeOK("local.wechat.package.export", value)
	}}
	export.Flags().StringVar(&exportDirectory, "directory", "", "workspace path; defaults to current directory")
	export.Flags().StringVar(&outputDirectory, "out", "", "workspace-relative output directory")
	var lintDirectory string
	lint := &cobra.Command{Use: "lint <package.json>", Args: cobra.ExactArgs(1), Short: "Verify package files and digests", RunE: func(cmd *cobra.Command, args []string) error {
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
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")
	packageCommand.AddCommand(export, lint)
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
	cmd := &cobra.Command{Use: "brief", Short: "Validate local V3 Brief inputs"}
	var directory string
	lint := &cobra.Command{Use: "lint <brief.json>", Args: cobra.ExactArgs(1), Short: "Validate a V3 Brief against current eligible knowledge", RunE: func(cmd *cobra.Command, args []string) error {
		report, brief, err := localworkspace.LintBrief(directory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("BRIEF_LINT_FAILED", "Brief V3 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.brief.lint", map[string]any{"brief": brief, "report": report})
	}}
	lint.Flags().StringVar(&directory, "directory", "", "workspace path; defaults to current directory")
	cmd.AddCommand(lint)
	return cmd
}

func (r *Root) localContentCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "content", Short: "Create and govern V3 ContentBatch and ContentItem objects"}
	batch := &cobra.Command{Use: "batch", Short: "Create, lint, and finalize local ContentBatch manifests"}

	var initDirectory, briefID, directionsFile, variant, batchID string
	var requestedCount int
	var controlled []string
	init := &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Freeze approved Brief and Knowledge snapshots into a ContentBatch", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.CreateContentBatch(localworkspace.CreateContentBatchOptions{Root: initDirectory, BriefID: briefID, DirectionsFile: directionsFile, RequestedCount: requestedCount, VariantDimension: variant, ControlledDimensions: controlled, BatchID: batchID, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.content.batch.init", result)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "workspace path; defaults to current directory")
	init.Flags().StringVar(&briefID, "brief", "", "approved Brief object ID; defaults to the newest eligible Brief")
	init.Flags().StringVar(&directionsFile, "directions", "", "workspace-relative CreativeDirection JSON array")
	init.Flags().IntVar(&requestedCount, "count", 0, "number of ContentItem candidates; defaults to selected direction count")
	init.Flags().StringVar(&variant, "variant", "hook", "hook, audience, scenario, visualization, cta, or duration")
	init.Flags().StringSliceVar(&controlled, "control", nil, "controlled experiment dimension; repeat as needed")
	init.Flags().StringVar(&batchID, "id", "", "optional stable ContentBatch ID")

	var batchLintDirectory, batchLintFile string
	var batchLintContent []string
	batchLint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "Validate all ContentItem candidates in a batch", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.LintContentBatch(batchLintDirectory, batchLintFile, batchLintContent)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("CONTENT_BATCH_LINT_FAILED", "ContentBatch 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.content.batch.lint", report)
	}}
	batchLint.Flags().StringVar(&batchLintDirectory, "directory", "", "workspace path; defaults to current directory")
	batchLint.Flags().StringVar(&batchLintFile, "batch", "", "workspace-relative ContentBatch manifest.yaml")
	batchLint.Flags().StringSliceVar(&batchLintContent, "file", nil, "ContentItem JSON file; repeat for every candidate")

	var finalizeDirectory, finalizeBatch string
	var finalizeContent []string
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, Short: "Finalize a fully validated local ContentBatch", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.FinalizeContentBatch(finalizeDirectory, finalizeBatch, finalizeContent, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.content.batch.finalize", result)
	}}
	finalize.Flags().StringVar(&finalizeDirectory, "directory", "", "workspace path; defaults to current directory")
	finalize.Flags().StringVar(&finalizeBatch, "batch", "", "workspace-relative ContentBatch manifest.yaml")
	finalize.Flags().StringSliceVar(&finalizeContent, "file", nil, "ContentItem JSON file; repeat for every candidate")
	batch.AddCommand(init, batchLint, finalize)

	var lintDirectory, lintBatch string
	lint := &cobra.Command{Use: "lint <content-item.json>", Args: cobra.ExactArgs(1), Short: "Validate one ContentItem against its frozen ContentBatch context", RunE: func(cmd *cobra.Command, args []string) error {
		report, _, err := localworkspace.LintContentItem(lintDirectory, args[0], lintBatch)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("CONTENT_ITEM_LINT_FAILED", "ContentItem 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.content.item.lint", report)
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")
	lint.Flags().StringVar(&lintBatch, "batch", "", "workspace-relative ContentBatch manifest.yaml; inferred from content_batch_id when omitted")

	var diffDirectory, baselineFile, candidateFile string
	var allowedPaths []string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "Detect undeclared drift in a revision or single-variable variant", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.DiffContentItems(diffDirectory, baselineFile, candidateFile, allowedPaths)
		if err != nil {
			return err
		}
		if !result.Valid {
			err := domain.Invalid("CONTENT_ITEM_REVISION_DRIFT", "ContentItem 修订包含未声明字段变化")
			err.Details = result
			return err
		}
		return r.writeOK("local.content.item.diff", result)
	}}
	diff.Flags().StringVar(&diffDirectory, "directory", "", "workspace path; defaults to current directory")
	diff.Flags().StringVar(&baselineFile, "baseline", "", "workspace-relative immutable baseline ContentItem")
	diff.Flags().StringVar(&candidateFile, "candidate", "", "workspace-relative revised ContentItem")
	diff.Flags().StringSliceVar(&allowedPaths, "allow", nil, "allowed JSON Pointer prefix; repeat as needed")

	var exportDirectory, outputDirectory string
	export := &cobra.Command{Use: "export <approved-content-item-id>", Args: cobra.ExactArgs(1), Short: "Export an approved ContentItem as JSON, Markdown, and XLSX", RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := localworkspace.ExportApprovedContentItem(exportDirectory, args[0], outputDirectory, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.content.delivery.export", manifest)
	}}
	export.Flags().StringVar(&exportDirectory, "directory", "", "workspace path; defaults to current directory")
	export.Flags().StringVar(&outputDirectory, "out", "", "workspace-relative output directory")

	cmd.AddCommand(batch, lint, diff, export)
	return cmd
}

func (r *Root) localSourceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Register and ingest immutable source files in the local workspace"}
	var directory, id, title, sourceKind, storageMode string
	register := &cobra.Command{Use: "register <file>", Args: cobra.ExactArgs(1), Short: "Register a local source by immutable SHA-256", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RegisterLocalSource(localworkspace.RegisterLocalSourceOptions{Root: directory, File: args[0], ID: id, Title: title, SourceKind: sourceKind, StorageMode: storageMode, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.source.register", value)
	}}
	register.Flags().StringVar(&directory, "directory", "", "workspace path; defaults to current directory")
	register.Flags().StringVar(&id, "id", "", "stable local source ID")
	register.Flags().StringVar(&title, "title", "", "source title")
	register.Flags().StringVar(&sourceKind, "kind", "customer_material", "source kind")
	register.Flags().StringVar(&storageMode, "storage", "copy", "copy or reference")

	var listDirectory string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "List locally registered sources", RunE: func(cmd *cobra.Command, args []string) error {
		values, err := localworkspace.LocalSources(listDirectory)
		if err != nil {
			return err
		}
		return r.writeOK("local.source.list", map[string]any{"count": len(values), "sources": values})
	}}
	list.Flags().StringVar(&listDirectory, "directory", "", "workspace path; defaults to current directory")

	var showDirectory string
	show := &cobra.Command{Use: "show <source-id>", Args: cobra.ExactArgs(1), Short: "Show one local source", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.LocalSourceByID(showDirectory, args[0])
		if err != nil {
			return err
		}
		return r.writeOK("local.source.show", value)
	}}
	show.Flags().StringVar(&showDirectory, "directory", "", "workspace path; defaults to current directory")

	var ingestDirectory string
	ingest := &cobra.Command{Use: "ingest <source-id>", Args: cobra.ExactArgs(1), Short: "Parse one source into exact local evidence spans", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.IngestLocalSource(ingestDirectory, args[0], time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.source.ingest", value)
	}}
	ingest.Flags().StringVar(&ingestDirectory, "directory", "", "workspace path; defaults to current directory")

	var verifyDirectory string
	verify := &cobra.Command{Use: "verify", Args: cobra.NoArgs, Short: "Verify source existence, hashes, and detected MIME types", RunE: func(cmd *cobra.Command, args []string) error {
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
	verify.Flags().StringVar(&verifyDirectory, "directory", "", "workspace path; defaults to current directory")

	cmd.AddCommand(register, list, show, ingest, verify)
	return cmd
}

func (r *Root) localRunCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "run", Short: "Manage resumable LocalRunContext stage gates"}
	var initDirectory, runID, intent string
	var sourceRefs []string
	var withIngest bool
	init := &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Initialize a local ingest, query, or content run", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: initDirectory, RunID: runID, Intent: intent, InputIDs: sourceRefs, WithIngest: withIngest, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.init", value)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "workspace path; defaults to current directory")
	init.Flags().StringVar(&runID, "id", "", "optional stable run ID")
	init.Flags().StringVar(&intent, "intent", "intent:content", "stable intent ID, such as intent:content")
	init.Flags().StringSliceVar(&sourceRefs, "input", nil, "registered immutable source ID; repeat as needed")
	init.Flags().BoolVar(&withIngest, "with-ingest", false, "start at the ingest stage")

	var showDirectory string
	show := &cobra.Command{Use: "show [run-id]", Args: cobra.MaximumNArgs(1), Short: "Show a run, or the current run", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ShowLocalRun(showDirectory, optionalValue(args))
		if err != nil {
			return err
		}
		return r.writeOK("local.run.show", value)
	}}
	show.Flags().StringVar(&showDirectory, "directory", "", "workspace path; defaults to current directory")

	var recordDirectory, recordRunID, recordClaimToken string
	var recordRevision uint64
	var recordSourceRefs, changedIDs, eligibleIDs, blockedIDs, findings, outputPaths []string
	record := &cobra.Command{Use: "record", Args: cobra.NoArgs, Short: "Record immutable references and outputs in the current run", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RecordClaimedLocalRun(localworkspace.RecordLocalRunOptions{Root: recordDirectory, RunID: recordRunID, ClaimToken: recordClaimToken, ExpectedRevision: recordRevision, InputIDs: recordSourceRefs, ChangedIDs: changedIDs, EligibleIDs: eligibleIDs, BlockedIDs: blockedIDs, Findings: findings, OutputPaths: outputPaths, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.record", value)
	}}
	addLocalRunRecordFlags(record, &recordDirectory, &recordRunID, &recordSourceRefs, &changedIDs, &eligibleIDs, &blockedIDs, &findings, &outputPaths)
	record.Flags().StringVar(&recordClaimToken, "claim-token", "", "active local claim token")
	record.Flags().Uint64Var(&recordRevision, "revision", 0, "expected LocalRun context revision")

	var checkDirectory, checkRunID, checkName, checkStatus, checkCommand, checkDetail, checkClaimToken string
	var checkRevision uint64
	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "Record a deterministic stage check", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CheckClaimedLocalRun(localworkspace.CheckLocalRunOptions{Root: checkDirectory, RunID: checkRunID, ClaimToken: checkClaimToken, ExpectedRevision: checkRevision, Name: checkName, Status: checkStatus, Command: checkCommand, Detail: checkDetail, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.check", value)
	}}
	check.Flags().StringVar(&checkDirectory, "directory", "", "workspace path; defaults to current directory")
	check.Flags().StringVar(&checkRunID, "run", "", "run ID; defaults to current run")
	check.Flags().StringVar(&checkName, "name", "", "check name, such as kb-lint or content-lint")
	check.Flags().StringVar(&checkStatus, "status", "", "passed or failed")
	check.Flags().StringVar(&checkCommand, "command", "", "deterministic command that produced the result")
	check.Flags().StringVar(&checkDetail, "detail", "", "short check detail")
	check.Flags().StringVar(&checkClaimToken, "claim-token", "", "active local claim token")
	check.Flags().Uint64Var(&checkRevision, "revision", 0, "expected LocalRun context revision")

	var advanceDirectory, advanceRunID, advanceClaimToken string
	var advanceRevision uint64
	var advanceSourceRefs, advanceChanged, advanceEligible, advanceBlocked, advanceFindings, advanceOutputs []string
	advance := &cobra.Command{Use: "advance <stage>", Args: cobra.ExactArgs(1), Short: "Advance through a validated stage handoff", RunE: func(cmd *cobra.Command, args []string) error {
		additions := localworkspace.RecordLocalRunOptions{ClaimToken: advanceClaimToken, ExpectedRevision: advanceRevision, InputIDs: advanceSourceRefs, ChangedIDs: advanceChanged, EligibleIDs: advanceEligible, BlockedIDs: advanceBlocked, Findings: advanceFindings, OutputPaths: advanceOutputs}
		value, err := localworkspace.AdvanceClaimedLocalRun(advanceDirectory, advanceRunID, args[0], additions, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.advance", value)
	}}
	addLocalRunRecordFlags(advance, &advanceDirectory, &advanceRunID, &advanceSourceRefs, &advanceChanged, &advanceEligible, &advanceBlocked, &advanceFindings, &advanceOutputs)
	advance.Flags().StringVar(&advanceClaimToken, "claim-token", "", "active local claim token")
	advance.Flags().Uint64Var(&advanceRevision, "revision", 0, "expected LocalRun context revision")

	var resumeDirectory, resumeRunID, resumeClaimToken string
	var resumeRevision uint64
	resume := &cobra.Command{Use: "resume", Args: cobra.NoArgs, Short: "Resume a failed run at the same stage", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ResumeClaimedLocalRun(resumeDirectory, resumeRunID, resumeClaimToken, resumeRevision, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.resume", value)
	}}
	resume.Flags().StringVar(&resumeDirectory, "directory", "", "workspace path; defaults to current directory")
	resume.Flags().StringVar(&resumeRunID, "run", "", "run ID; defaults to current run")
	resume.Flags().StringVar(&resumeClaimToken, "claim-token", "", "active local claim token")
	resume.Flags().Uint64Var(&resumeRevision, "revision", 0, "expected LocalRun context revision")

	var failDirectory, failRunID, failClaimToken string
	var failRevision uint64
	var failFindings []string
	fail := &cobra.Command{Use: "fail", Args: cobra.NoArgs, Short: "Mark a run failed with actionable findings", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.FailClaimedLocalRun(failDirectory, failRunID, failFindings, failClaimToken, failRevision, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.fail", value)
	}}
	fail.Flags().StringVar(&failDirectory, "directory", "", "workspace path; defaults to current directory")
	fail.Flags().StringVar(&failRunID, "run", "", "run ID; defaults to current run")
	fail.Flags().StringSliceVar(&failFindings, "finding", nil, "failure finding; repeat as needed")
	fail.Flags().StringVar(&failClaimToken, "claim-token", "", "active local claim token")
	fail.Flags().Uint64Var(&failRevision, "revision", 0, "expected LocalRun context revision")

	var validateDirectory string
	validate := &cobra.Command{Use: "validate", Args: cobra.NoArgs, Short: "Validate every LocalRunContext and the current pointer", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.ValidateLocalRuns(validateDirectory)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("LOCAL_RUN_VALIDATE_FAILED", "LocalRunContext 校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.run.validate", report)
	}}
	validate.Flags().StringVar(&validateDirectory, "directory", "", "workspace path; defaults to current directory")

	var claimDirectory, claimRunID, claimOwner string
	var claimRevision uint64
	var claimTTL time.Duration
	var takeoverExpired bool
	claim := &cobra.Command{Use: "claim", Args: cobra.NoArgs, Short: "Acquire the single-writer claim for a LocalRun revision", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ClaimRun(localworkspace.ClaimRunOptions{Root: claimDirectory, RunID: claimRunID, Owner: claimOwner, ExpectedRevision: claimRevision, TTL: claimTTL, TakeoverExpired: takeoverExpired, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.claim", value)
	}}
	claim.Flags().StringVar(&claimDirectory, "directory", "", "workspace path; defaults to current directory")
	claim.Flags().StringVar(&claimRunID, "run", "", "run ID")
	claim.Flags().StringVar(&claimOwner, "owner", "", "conversation or worker owner ID")
	claim.Flags().Uint64Var(&claimRevision, "revision", 0, "expected LocalRun context revision")
	claim.Flags().DurationVar(&claimTTL, "ttl", 30*time.Minute, "claim TTL; maximum 4h")
	claim.Flags().BoolVar(&takeoverExpired, "takeover-expired", false, "explicitly take over an expired claim")

	var renewDirectory, renewRunID, renewToken string
	var renewTTL time.Duration
	renew := &cobra.Command{Use: "renew", Args: cobra.NoArgs, Short: "Renew an active LocalRun claim", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RenewRunClaim(renewDirectory, renewRunID, renewToken, renewTTL, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.renew", value)
	}}
	renew.Flags().StringVar(&renewDirectory, "directory", "", "workspace path; defaults to current directory")
	renew.Flags().StringVar(&renewRunID, "run", "", "run ID")
	renew.Flags().StringVar(&renewToken, "claim-token", "", "active local claim token")
	renew.Flags().DurationVar(&renewTTL, "ttl", 30*time.Minute, "renewed claim TTL; maximum 4h")

	var releaseDirectory, releaseRunID, releaseToken string
	release := &cobra.Command{Use: "release", Args: cobra.NoArgs, Short: "Release an active LocalRun claim", RunE: func(cmd *cobra.Command, args []string) error {
		if err := localworkspace.ReleaseRunClaim(releaseDirectory, releaseRunID, releaseToken, time.Now()); err != nil {
			return err
		}
		return r.writeOK("local.run.release", map[string]any{"run_id": releaseRunID, "released": true})
	}}
	release.Flags().StringVar(&releaseDirectory, "directory", "", "workspace path; defaults to current directory")
	release.Flags().StringVar(&releaseRunID, "run", "", "run ID")
	release.Flags().StringVar(&releaseToken, "claim-token", "", "active local claim token")

	var claimStatusDirectory, claimStatusRunID string
	claimStatus := &cobra.Command{Use: "claim-status", Args: cobra.NoArgs, Short: "Read non-secret LocalRun claim status", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RunClaimStatus(claimStatusDirectory, claimStatusRunID, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.claim-status", value)
	}}
	claimStatus.Flags().StringVar(&claimStatusDirectory, "directory", "", "workspace path; defaults to current directory")
	claimStatus.Flags().StringVar(&claimStatusRunID, "run", "", "run ID")

	cmd.AddCommand(init, show, record, check, advance, resume, fail, validate, claim, renew, release, claimStatus)
	return cmd
}

func (r *Root) localHandoffCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "handoff", Short: "Create and accept digest-verified cross-conversation handoffs"}

	var createDirectory, createID, createRunID, createToken, nextCapability, nextAction string
	var createRevision uint64
	var inputPaths, blockers, pendingDecisions []string
	create := &cobra.Command{Use: "create-ready", Args: cobra.NoArgs, Short: "Checkpoint a claimed Run into a ready Handoff and release the claim", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CreateReadyHandoff(localworkspace.CreateReadyHandoffOptions{Root: createDirectory, HandoffID: createID, RunID: createRunID, ClaimToken: createToken, ExpectedRevision: createRevision, NextCapabilityID: nextCapability, NextAction: nextAction, InputPaths: inputPaths, Blockers: blockers, PendingDecisions: pendingDecisions, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.create-ready", value)
	}}
	create.Flags().StringVar(&createDirectory, "directory", "", "workspace path; defaults to current directory")
	create.Flags().StringVar(&createID, "id", "", "optional stable handoff ID")
	create.Flags().StringVar(&createRunID, "run", "", "run ID")
	create.Flags().StringVar(&createToken, "claim-token", "", "active local claim token")
	create.Flags().Uint64Var(&createRevision, "revision", 0, "expected LocalRun context revision")
	create.Flags().StringVar(&nextCapability, "next-capability", "", "next stable capability ID")
	create.Flags().StringVar(&nextAction, "next-action", "", "short actionable continuation instruction")
	create.Flags().StringSliceVar(&inputPaths, "input", nil, "workspace-relative checkpoint input; repeat as needed")
	create.Flags().StringSliceVar(&blockers, "blocker", nil, "persisted blocker; repeat as needed")
	create.Flags().StringSliceVar(&pendingDecisions, "pending-decision", nil, "pending decision; repeat as needed")

	var listDirectory string
	list := &cobra.Command{Use: "list-ready", Args: cobra.NoArgs, Short: "List ready Handoffs without claiming them", RunE: func(cmd *cobra.Command, args []string) error {
		values, err := localworkspace.ListReadyHandoffs(listDirectory)
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.list-ready", map[string]any{"count": len(values), "handoffs": values})
	}}
	list.Flags().StringVar(&listDirectory, "directory", "", "workspace path; defaults to current directory")

	var acceptDirectory, acceptID, acceptOwner string
	var acceptTTL time.Duration
	var acceptTakeover bool
	accept := &cobra.Command{Use: "accept", Args: cobra.NoArgs, Short: "Atomically verify a ready Handoff and claim its Run", RunE: func(cmd *cobra.Command, args []string) error {
		handoff, claim, err := localworkspace.AcceptHandoff(localworkspace.AcceptHandoffOptions{Root: acceptDirectory, HandoffID: acceptID, Owner: acceptOwner, TTL: acceptTTL, TakeoverExpired: acceptTakeover, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.accept", map[string]any{"handoff": handoff, "claim": claim})
	}}
	accept.Flags().StringVar(&acceptDirectory, "directory", "", "workspace path; defaults to current directory")
	accept.Flags().StringVar(&acceptID, "id", "", "ready handoff ID")
	accept.Flags().StringVar(&acceptOwner, "owner", "", "accepting conversation or worker owner ID")
	accept.Flags().DurationVar(&acceptTTL, "ttl", 30*time.Minute, "claim TTL; maximum 4h")
	accept.Flags().BoolVar(&acceptTakeover, "takeover-expired", false, "explicitly take over an expired claim")

	var completeDirectory, completeID, completeToken string
	complete := &cobra.Command{Use: "complete", Args: cobra.NoArgs, Short: "Mark a claimed Handoff completed", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CompleteHandoff(completeDirectory, completeID, completeToken, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.complete", value)
	}}
	complete.Flags().StringVar(&completeDirectory, "directory", "", "workspace path; defaults to current directory")
	complete.Flags().StringVar(&completeID, "id", "", "claimed handoff ID")
	complete.Flags().StringVar(&completeToken, "claim-token", "", "active local claim token")

	var supersedeDirectory, supersedeID string
	supersede := &cobra.Command{Use: "supersede", Args: cobra.NoArgs, Short: "Supersede a ready Handoff", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.SupersedeReadyHandoff(supersedeDirectory, supersedeID, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.handoff.supersede", value)
	}}
	supersede.Flags().StringVar(&supersedeDirectory, "directory", "", "workspace path; defaults to current directory")
	supersede.Flags().StringVar(&supersedeID, "id", "", "ready handoff ID")

	cmd.AddCommand(create, list, accept, complete, supersede)
	return cmd
}

func (r *Root) localKnowledgeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "knowledge", Short: "Import, lint, query, diagnose, and pack local governed knowledge"}
	var importDirectory, originRun string
	importCandidates := &cobra.Command{Use: "import <knowledge-candidates.json>", Args: cobra.ExactArgs(1), Short: "Import evidence-grounded knowledge-candidates/1.0", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.ImportKnowledgeCandidates(localworkspace.ImportKnowledgeOptions{Root: importDirectory, PackageFile: args[0], OriginRunID: originRun, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.import", report)
	}}
	importCandidates.Flags().StringVar(&importDirectory, "directory", "", "workspace path; defaults to current directory")
	importCandidates.Flags().StringVar(&originRun, "run", "", "origin LocalRun ID")

	var lintDirectory string
	lint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "Validate IDs, evidence, state, decisions, dependencies, rights, and conflicts", RunE: func(cmd *cobra.Command, args []string) error {
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
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")

	var queryDirectory, queryChannel, queryAt string
	query := &cobra.Command{Use: "query", Args: cobra.NoArgs, Short: "Classify knowledge as eligible, blocked, or informational", RunE: func(cmd *cobra.Command, args []string) error {
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
	query.Flags().StringVar(&queryDirectory, "directory", "", "workspace path; defaults to current directory")
	query.Flags().StringVar(&queryChannel, "channel", "", "target content channel")
	query.Flags().StringVar(&queryAt, "at", "", "eligibility time in RFC3339; defaults to now")

	var diagnoseDirectory, diagnoseChannel, diagnoseAt string
	diagnose := &cobra.Command{Use: "diagnose", Args: cobra.NoArgs, Short: "Produce the 15-dimension material coverage diagnosis", RunE: func(cmd *cobra.Command, args []string) error {
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
	diagnose.Flags().StringVar(&diagnoseDirectory, "directory", "", "workspace path; defaults to current directory")
	diagnose.Flags().StringVar(&diagnoseChannel, "channel", "", "target content channel")
	diagnose.Flags().StringVar(&diagnoseAt, "at", "", "diagnosis time in RFC3339; defaults to now")

	var packDirectory, packID, packName string
	pack := &cobra.Command{Use: "pack", Args: cobra.NoArgs, Short: "Build a seven-layer review package and evidence disclosures", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.PackKnowledge(localworkspace.PackKnowledgeOptions{Root: packDirectory, PackID: packID, Name: packName, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.knowledge.pack", result)
	}}
	pack.Flags().StringVar(&packDirectory, "directory", "", "workspace path; defaults to current directory")
	pack.Flags().StringVar(&packID, "id", "", "stable pack ID; defaults to a content hash ID")
	pack.Flags().StringVar(&packName, "name", "", "human-readable pack name")

	cmd.AddCommand(importCandidates, lint, query, diagnose, pack)
	return cmd
}

func addLocalRunRecordFlags(command *cobra.Command, directory, runID *string, sourceRefs, changedIDs, eligibleIDs, blockedIDs, findings, outputPaths *[]string) {
	command.Flags().StringVar(directory, "directory", "", "workspace path; defaults to current directory")
	command.Flags().StringVar(runID, "run", "", "run ID; defaults to current run")
	command.Flags().StringSliceVar(sourceRefs, "source-ref", nil, "source ID; repeat as needed")
	command.Flags().StringSliceVar(changedIDs, "changed-id", nil, "changed object ID; repeat as needed")
	command.Flags().StringSliceVar(eligibleIDs, "eligible-id", nil, "eligible knowledge ID; repeat as needed")
	command.Flags().StringSliceVar(blockedIDs, "blocked-id", nil, "blocked knowledge ID; repeat as needed")
	command.Flags().StringSliceVar(findings, "finding", nil, "finding; repeat as needed")
	command.Flags().StringSliceVar(outputPaths, "output-path", nil, "workspace-relative output path; repeat as needed")
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
