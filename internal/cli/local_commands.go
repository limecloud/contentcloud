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
	cmd.AddCommand(r.localSourceCommand(), r.localRunCommand(), r.localKnowledgeCommand(), r.localBriefCommand(), r.localScriptCommand())
	return cmd
}

func (r *Root) localBriefCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "brief", Short: "Validate local ScriptPackage V2 Brief inputs"}
	var directory string
	lint := &cobra.Command{Use: "lint <brief.json>", Args: cobra.ExactArgs(1), Short: "Validate a Brief V2 against current eligible knowledge", RunE: func(cmd *cobra.Command, args []string) error {
		report, brief, err := localworkspace.LintBrief(directory, args[0])
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("BRIEF_LINT_FAILED", "Brief V2 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.brief.lint", map[string]any{"brief": brief, "report": report})
	}}
	lint.Flags().StringVar(&directory, "directory", "", "workspace path; defaults to current directory")
	cmd.AddCommand(lint)
	return cmd
}

func (r *Root) localScriptCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "script", Short: "Create CreativeBatch manifests and govern ScriptPackage V2"}
	batch := &cobra.Command{Use: "batch", Short: "Create, lint, and finalize local CreativeBatch manifests"}

	var initDirectory, briefID, directionsFile, variant, batchID string
	var requestedCount int
	var controlled []string
	init := &cobra.Command{Use: "init", Args: cobra.NoArgs, Short: "Freeze approved Brief and Knowledge snapshots into a CreativeBatch", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.CreateCreativeBatch(localworkspace.CreateCreativeBatchOptions{Root: initDirectory, BriefID: briefID, DirectionsFile: directionsFile, RequestedCount: requestedCount, VariantDimension: variant, ControlledDimensions: controlled, BatchID: batchID, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.script.batch.init", result)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "workspace path; defaults to current directory")
	init.Flags().StringVar(&briefID, "brief", "", "approved Brief object ID; defaults to the newest eligible Brief")
	init.Flags().StringVar(&directionsFile, "directions", "", "workspace-relative CreativeDirection JSON array")
	init.Flags().IntVar(&requestedCount, "count", 0, "number of ScriptPackage candidates; defaults to selected direction count")
	init.Flags().StringVar(&variant, "variant", "hook", "hook, audience, scenario, visualization, cta, or duration")
	init.Flags().StringSliceVar(&controlled, "control", nil, "controlled experiment dimension; repeat as needed")
	init.Flags().StringVar(&batchID, "id", "", "optional stable CreativeBatch ID")

	var batchLintDirectory, batchLintFile string
	var batchLintScripts []string
	batchLint := &cobra.Command{Use: "lint", Args: cobra.NoArgs, Short: "Validate all ScriptPackage candidates in a batch", RunE: func(cmd *cobra.Command, args []string) error {
		report, err := localworkspace.LintCreativeBatch(batchLintDirectory, batchLintFile, batchLintScripts)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("CREATIVE_BATCH_LINT_FAILED", "CreativeBatch 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.script.batch.lint", report)
	}}
	batchLint.Flags().StringVar(&batchLintDirectory, "directory", "", "workspace path; defaults to current directory")
	batchLint.Flags().StringVar(&batchLintFile, "batch", "", "workspace-relative batch.json")
	batchLint.Flags().StringSliceVar(&batchLintScripts, "file", nil, "ScriptPackage V2 file; repeat for every candidate")

	var finalizeDirectory, finalizeBatch string
	var finalizeScripts []string
	finalize := &cobra.Command{Use: "finalize", Args: cobra.NoArgs, Short: "Finalize a fully validated local CreativeBatch", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.FinalizeCreativeBatch(finalizeDirectory, finalizeBatch, finalizeScripts, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.script.batch.finalize", result)
	}}
	finalize.Flags().StringVar(&finalizeDirectory, "directory", "", "workspace path; defaults to current directory")
	finalize.Flags().StringVar(&finalizeBatch, "batch", "", "workspace-relative batch.json")
	finalize.Flags().StringSliceVar(&finalizeScripts, "file", nil, "ScriptPackage V2 file; repeat for every candidate")
	batch.AddCommand(init, batchLint, finalize)

	var lintDirectory, lintBatch string
	lint := &cobra.Command{Use: "lint <script-package.json>", Args: cobra.ExactArgs(1), Short: "Validate one ScriptPackage V2 against its frozen batch context", RunE: func(cmd *cobra.Command, args []string) error {
		report, _, err := localworkspace.LintScriptPackage(lintDirectory, args[0], lintBatch)
		if err != nil {
			return err
		}
		if !report.Valid {
			err := domain.Invalid("SCRIPT_PACKAGE_LINT_FAILED", "ScriptPackage V2 确定性校验失败")
			err.Details = report
			return err
		}
		return r.writeOK("local.script.lint", report)
	}}
	lint.Flags().StringVar(&lintDirectory, "directory", "", "workspace path; defaults to current directory")
	lint.Flags().StringVar(&lintBatch, "batch", "", "workspace-relative batch.json; inferred from creative_batch_id when omitted")

	var diffDirectory, baselineFile, candidateFile string
	var allowedPaths []string
	diff := &cobra.Command{Use: "diff", Args: cobra.NoArgs, Short: "Detect undeclared drift in a revision or single-variable variant", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := localworkspace.DiffScriptPackages(diffDirectory, baselineFile, candidateFile, allowedPaths)
		if err != nil {
			return err
		}
		if !result.Valid {
			err := domain.Invalid("SCRIPT_REVISION_DRIFT", "修订包含未声明字段变化")
			err.Details = result
			return err
		}
		return r.writeOK("local.script.diff", result)
	}}
	diff.Flags().StringVar(&diffDirectory, "directory", "", "workspace path; defaults to current directory")
	diff.Flags().StringVar(&baselineFile, "baseline", "", "workspace-relative immutable baseline ScriptPackage")
	diff.Flags().StringVar(&candidateFile, "candidate", "", "workspace-relative revision ScriptPackage")
	diff.Flags().StringSliceVar(&allowedPaths, "allow", nil, "allowed JSON Pointer prefix; repeat as needed")

	var exportDirectory, outputDirectory string
	export := &cobra.Command{Use: "export <approved-script-id>", Args: cobra.ExactArgs(1), Short: "Export an approved ScriptPackage V2 as JSON, Markdown, and XLSX", RunE: func(cmd *cobra.Command, args []string) error {
		manifest, err := localworkspace.ExportApprovedScript(exportDirectory, args[0], outputDirectory, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.script.export", manifest)
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
		value, err := localworkspace.InitLocalRun(localworkspace.InitLocalRunOptions{Root: initDirectory, RunID: runID, Intent: intent, SourceRefs: sourceRefs, WithIngest: withIngest, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.init", value)
	}}
	init.Flags().StringVar(&initDirectory, "directory", "", "workspace path; defaults to current directory")
	init.Flags().StringVar(&runID, "id", "", "optional stable run ID")
	init.Flags().StringVar(&intent, "intent", "content", "ingest, query, or content")
	init.Flags().StringSliceVar(&sourceRefs, "source-ref", nil, "registered source ID; repeat as needed")
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

	var recordDirectory, recordRunID string
	var recordSourceRefs, changedIDs, eligibleIDs, blockedIDs, findings, outputPaths []string
	record := &cobra.Command{Use: "record", Args: cobra.NoArgs, Short: "Record immutable references and outputs in the current run", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.RecordLocalRun(localworkspace.RecordLocalRunOptions{Root: recordDirectory, RunID: recordRunID, SourceRefs: recordSourceRefs, ChangedIDs: changedIDs, EligibleIDs: eligibleIDs, BlockedIDs: blockedIDs, Findings: findings, OutputPaths: outputPaths, Now: time.Now()})
		if err != nil {
			return err
		}
		return r.writeOK("local.run.record", value)
	}}
	addLocalRunRecordFlags(record, &recordDirectory, &recordRunID, &recordSourceRefs, &changedIDs, &eligibleIDs, &blockedIDs, &findings, &outputPaths)

	var checkDirectory, checkRunID, checkName, checkStatus, checkCommand, checkDetail string
	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "Record a deterministic stage check", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.CheckLocalRun(localworkspace.CheckLocalRunOptions{Root: checkDirectory, RunID: checkRunID, Name: checkName, Status: checkStatus, Command: checkCommand, Detail: checkDetail, Now: time.Now()})
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

	var advanceDirectory, advanceRunID string
	var advanceSourceRefs, advanceChanged, advanceEligible, advanceBlocked, advanceFindings, advanceOutputs []string
	advance := &cobra.Command{Use: "advance <stage>", Args: cobra.ExactArgs(1), Short: "Advance through a validated stage handoff", RunE: func(cmd *cobra.Command, args []string) error {
		additions := localworkspace.RecordLocalRunOptions{SourceRefs: advanceSourceRefs, ChangedIDs: advanceChanged, EligibleIDs: advanceEligible, BlockedIDs: advanceBlocked, Findings: advanceFindings, OutputPaths: advanceOutputs}
		value, err := localworkspace.AdvanceLocalRun(advanceDirectory, advanceRunID, args[0], additions, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.advance", value)
	}}
	addLocalRunRecordFlags(advance, &advanceDirectory, &advanceRunID, &advanceSourceRefs, &advanceChanged, &advanceEligible, &advanceBlocked, &advanceFindings, &advanceOutputs)

	var resumeDirectory, resumeRunID string
	resume := &cobra.Command{Use: "resume", Args: cobra.NoArgs, Short: "Resume a failed run at the same stage", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.ResumeLocalRun(resumeDirectory, resumeRunID, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.resume", value)
	}}
	resume.Flags().StringVar(&resumeDirectory, "directory", "", "workspace path; defaults to current directory")
	resume.Flags().StringVar(&resumeRunID, "run", "", "run ID; defaults to current run")

	var failDirectory, failRunID string
	var failFindings []string
	fail := &cobra.Command{Use: "fail", Args: cobra.NoArgs, Short: "Mark a run failed with actionable findings", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := localworkspace.FailLocalRun(failDirectory, failRunID, failFindings, time.Now())
		if err != nil {
			return err
		}
		return r.writeOK("local.run.fail", value)
	}}
	fail.Flags().StringVar(&failDirectory, "directory", "", "workspace path; defaults to current directory")
	fail.Flags().StringVar(&failRunID, "run", "", "run ID; defaults to current run")
	fail.Flags().StringSliceVar(&failFindings, "finding", nil, "failure finding; repeat as needed")

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

	cmd.AddCommand(init, show, record, check, advance, resume, fail, validate)
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
