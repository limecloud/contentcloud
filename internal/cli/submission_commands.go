package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
	"github.com/limecloud/contentcloud/internal/localworkspace"
)

type publishPreflight struct {
	PlanID             string         `json:"plan_id"`
	PreflightPlane     string         `json:"preflight_execution_plane"`
	ApplyPlane         string         `json:"apply_execution_plane"`
	SubmissionType     string         `json:"submission_type"`
	SchemaVersion      string         `json:"schema_version"`
	Files              []string       `json:"files"`
	ObjectCount        int            `json:"object_count"`
	BlockedCount       int            `json:"blocked_count"`
	DisclosureCount    map[string]int `json:"disclosure_count"`
	UploadBytes        int64          `json:"upload_bytes"`
	ContentHash        string         `json:"content_hash"`
	IdempotencyKey     string         `json:"idempotency_key"`
	EnvironmentHash    string         `json:"environment_digest"`
	WorkspaceStateHash string         `json:"workspace_state_digest"`
	BaseSnapshotIDs    []string       `json:"base_snapshot_ids"`
	ReviewVisible      []string       `json:"review_visible"`
	ExternalEffects    []string       `json:"external_side_effects"`
	RawFilesUpload     bool           `json:"raw_files_upload"`
	RequiresConfirm    bool           `json:"requires_confirmation"`
}

type publishBuildOptions struct {
	Root            string
	SubmissionType  string
	Files           []string
	DisclosuresFile string
	Message         string
	IdempotencyKey  string
}

func (r *Root) publishCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "publish", Short: "发布不可变的本地检查点，供云端审核"}
	for _, submissionType := range domain.SubmissionTypes() {
		typeName := submissionType
		var files []string
		var disclosuresFile, message, idempotencyKey, planID string
		var dryRun, review, yes bool
		publish := &cobra.Command{Use: typeName, Short: "发布 " + typeName + " 检查点", RunE: func(command *cobra.Command, args []string) error {
			root, err := localworkspace.FindRoot("")
			if err != nil {
				return err
			}
			bundle, preflight, err := buildPublishCheckpoint(publishBuildOptions{Root: root, SubmissionType: typeName, Files: files, DisclosuresFile: disclosuresFile, Message: message, IdempotencyKey: idempotencyKey})
			if err != nil {
				return err
			}
			if dryRun {
				return r.writeOK("publish."+typeName, map[string]any{"dry_run": true, "preflight": preflight})
			}
			revision, err := r.applyPublishCheckpoint(command.Context(), root, bundle, preflight, planID, review || yes)
			if err != nil {
				return err
			}
			return r.writeOK("publish."+typeName, map[string]any{"submission_revision": revision, "preflight": preflight, "next_command": "contentcloud submission status " + revision.SubmissionID})
		}}
		publish.Flags().StringSliceVar(&files, "file", nil, "JSON 检查点文件；需要多个文件时可重复传入")
		publish.Flags().StringVar(&disclosuresFile, "disclosures", "", "描述各来源披露级别的 JSON 数组")
		publish.Flags().StringVar(&message, "message", "", "审核说明")
		publish.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "稳定的重试键；默认由类型和内容摘要生成")
		publish.Flags().StringVar(&planID, "plan-id", "", "发布预检确认后返回的准确 plan_id")
		publish.Flags().BoolVar(&dryRun, "dry-run", false, "只执行本地预检，不读取凭据，也不写入网络")
		publish.Flags().BoolVar(&review, "review", false, "确认发布此检查点供审核")
		publish.Flags().BoolVar(&yes, "yes", false, "确认发布此检查点")
		cmd.AddCommand(publish)
	}
	return cmd
}

func buildPublishCheckpoint(options publishBuildOptions) (domain.SubmissionBundle, publishPreflight, error) {
	resolvedFiles, err := resolvePublishFiles(options.Root, options.SubmissionType, options.Files)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	if err := validatePublishDomainFiles(options.Root, options.SubmissionType, resolvedFiles); err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	objects, fileBytes, blocked, inputHash, err := readPublishObjects(options.Root, options.SubmissionType, resolvedFiles)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	disclosures, disclosureBytes, err := readDisclosures(options.Root, options.DisclosuresFile)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	status, err := localworkspace.LoadStatus(options.Root)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	environmentDigest := strings.TrimSpace(status.Binding.EnvironmentDigest)
	if environmentDigest == "" {
		return domain.SubmissionBundle{}, publishPreflight{}, domain.Invalid("ENVIRONMENT_DIGEST_REQUIRED", "工作区绑定缺少 environment_digest，请重新初始化创作环境")
	}
	workspaceStateHash, err := domain.CanonicalHash(status.Template)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	baseSnapshotIDs := []string{}
	if status.Sync.ApprovedSnapshotID != "" {
		baseSnapshotIDs = append(baseSnapshotIDs, status.Sync.ApprovedSnapshotID)
	}
	derivedBaseSnapshotIDs, err := requiredSubmissionBaseSnapshotIDs(options.Root, options.SubmissionType, objects)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	baseSnapshotIDs = uniqueSortedCLIStrings(append(baseSnapshotIDs, derivedBaseSnapshotIDs...))
	bundle := domain.SubmissionBundle{
		BundleVersion: "3.0", SubmissionType: options.SubmissionType, ProjectID: status.Binding.ProjectID, WorkspaceID: status.Binding.WorkspaceID, BaseSnapshotIDs: baseSnapshotIDs,
		LocalRunSummary: domain.LocalRunSummary{Stage: "publish_preflight", Checks: publishChecks(options.SubmissionType), InputHash: inputHash, OutputHash: inputHash, Versions: map[string]string{"cli": Version, "template": status.Template.TemplateVersion, "environment": environmentDigest}},
		Objects:         objects, SourceDisclosures: disclosures, EnvironmentDigest: environmentDigest, Artifacts: []domain.SubmissionArtifact{}, Message: strings.TrimSpace(options.Message), IdempotencyKey: options.IdempotencyKey,
	}
	if err := bundle.SetComputedHash(); err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	if bundle.IdempotencyKey == "" {
		bundle.IdempotencyKey = options.SubmissionType + ":" + strings.TrimPrefix(bundle.ContentHash, "sha256:")
	}
	if err := bundle.Validate(); err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	counts := map[string]int{"metadata_only": 0, "evidence_pack": 0, "full_source": 0}
	for _, disclosure := range disclosures {
		counts[disclosure.Level]++
	}
	preflight := publishPreflight{
		PreflightPlane: codexLocalExecutionPlane, ApplyPlane: "contentcloud_server",
		SubmissionType: options.SubmissionType, SchemaVersion: domain.SubmissionSchemaVersion(options.SubmissionType), Files: relativePaths(options.Root, resolvedFiles), ObjectCount: len(objects), BlockedCount: blocked,
		DisclosureCount: counts, UploadBytes: fileBytes + disclosureBytes, ContentHash: bundle.ContentHash, IdempotencyKey: bundle.IdempotencyKey, EnvironmentHash: environmentDigest, WorkspaceStateHash: "sha256:" + workspaceStateHash, BaseSnapshotIDs: append([]string(nil), bundle.BaseSnapshotIDs...),
		ReviewVisible: []string{"objects", "local_run_summary", "source_disclosures", "artifact_manifest"}, ExternalEffects: []string{"创建不可变的提交修订版本", "让审核人员查看结构化对象和已声明的来源披露"}, RawFilesUpload: false, RequiresConfirm: true,
	}
	planHash, err := domain.CanonicalHash(preflight)
	if err != nil {
		return domain.SubmissionBundle{}, publishPreflight{}, err
	}
	preflight.PlanID = "pp_" + planHash
	return bundle, preflight, nil
}

func (r *Root) applyPublishCheckpoint(ctx context.Context, root string, bundle domain.SubmissionBundle, preflight publishPreflight, approvedPlanID string, confirmed bool) (domain.SubmissionRevision, error) {
	approvedPlanID = strings.TrimSpace(approvedPlanID)
	if approvedPlanID == "" {
		return domain.SubmissionRevision{}, domain.Invalid("PUBLISH_PLAN_ID_REQUIRED", "--plan-id 必填；请先运行发布预检并确认返回的 plan_id")
	}
	if approvedPlanID != preflight.PlanID {
		err := domain.Conflict("PUBLISH_PLAN_STALE", "本地发布内容、披露或环境与已确认的 plan_id 不一致")
		err.Details = map[string]any{"approved_plan_id": approvedPlanID, "current_plan_id": preflight.PlanID}
		err.Hint = "重新运行发布预检，检查变化后再次确认"
		return domain.SubmissionRevision{}, err
	}
	if !confirmed {
		return domain.SubmissionRevision{}, confirmationRequired("发布会将预检中列出的结构化对象和来源披露发送到云端审核；原始文件不会上传")
	}
	_, client, _, err := r.workspaceClient(root)
	if err != nil {
		return domain.SubmissionRevision{}, err
	}
	var revision domain.SubmissionRevision
	if err := client.Dispatch(ctx, "submission.create", bundle, &revision); err != nil {
		return domain.SubmissionRevision{}, err
	}
	if err := localworkspace.RecordPublished(root, bundle.SubmissionType, revision.ID, revision.ContentHash, r.currentTime()); err != nil {
		return domain.SubmissionRevision{}, err
	}
	return revision, nil
}

func (r *Root) pullCommand() *cobra.Command {
	var submissionType, id string
	var dryRun bool
	cmd := &cobra.Command{Use: "pull <feedback|decisions|approved>", Args: cobra.ExactArgs(1), Short: "拉取不可变审核数据包，不修改业务文件", RunE: func(cmd *cobra.Command, args []string) error {
		kind := args[0]
		if kind != "feedback" && kind != "decisions" && kind != "approved" {
			return domain.Invalid("PULL_KIND_INVALID", "拉取类型只支持 feedback、decisions 或 approved")
		}
		root, client, _, err := r.workspaceClient("")
		if err != nil {
			return err
		}
		downloaded := []string{}
		count := 0
		switch kind {
		case "feedback":
			var bundles []domain.ReviewFeedbackBundle
			if err := client.Dispatch(cmd.Context(), "feedback.workspace-list", map[string]any{}, &bundles); err != nil {
				return err
			}
			count = len(bundles)
			if !dryRun {
				for _, bundle := range bundles {
					item, err := localworkspace.StoreReviewFeedback(root, bundle, r.currentTime())
					if err != nil {
						return err
					}
					downloaded = append(downloaded, item.Path)
				}
			}
		case "decisions":
			var delta domain.DecisionDelta
			if err := client.Dispatch(cmd.Context(), "decision.workspace-list", map[string]any{}, &delta); err != nil {
				return err
			}
			count = len(delta.Decisions)
			if !dryRun && len(delta.Decisions) > 0 {
				hash, _ := domain.CanonicalHash(delta.Decisions)
				path, err := localworkspace.StorePulledBundle(root, kind, "delta-"+hash[:16], delta, time.Now())
				if err != nil {
					return err
				}
				downloaded = append(downloaded, path)
			}
		case "approved":
			var snapshots []domain.ApprovedSnapshot
			if id != "" {
				var snapshot domain.ApprovedSnapshot
				if err := client.Dispatch(cmd.Context(), "snapshot.workspace-show", map[string]any{"id": id}, &snapshot); err != nil {
					return err
				}
				snapshots = []domain.ApprovedSnapshot{snapshot}
			} else if err := client.Dispatch(cmd.Context(), "snapshot.workspace-list", map[string]any{"submission_type": submissionType}, &snapshots); err != nil {
				return err
			}
			count = len(snapshots)
			if !dryRun {
				records, err := localworkspace.StoreApprovedSnapshots(root, snapshots, r.currentTime())
				if err != nil {
					return err
				}
				for _, record := range records {
					downloaded = append(downloaded, record.Summary.Path)
				}
			}
		}
		return r.writeOK("pull."+kind, map[string]any{"dry_run": dryRun, "count": count, "downloaded": downloaded, "business_files_modified": false})
	}}
	cmd.Flags().StringVar(&submissionType, "type", "", "按提交类型筛选批准快照")
	cmd.Flags().StringVar(&id, "id", "", "按 ID 拉取一份批准快照")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "只显示远端数据包数量，不写入本地文件")
	return cmd
}

func (r *Root) submissionCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "submission", Short: "查看工作区提交记录并执行人工治理操作"}
	list := &cobra.Command{Use: "list", Short: "列出当前工作区创建的提交记录", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.workspaceClient("")
		if err != nil {
			return err
		}
		var values []domain.Submission
		if err := client.Dispatch(cmd.Context(), "submission.workspace-list", map[string]any{}, &values); err != nil {
			return err
		}
		return r.writeOK("submission.list", values)
	}}
	show := &cobra.Command{Use: "show <submission-id>", Args: cobra.ExactArgs(1), Short: "显示一条提交记录及其不可变版本", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.workspaceClient("")
		if err != nil {
			return err
		}
		var value app.SubmissionDetails
		if err := client.Dispatch(cmd.Context(), "submission.workspace-show", map[string]any{"id": args[0]}, &value); err != nil {
			return err
		}
		return r.writeOK("submission.show", value)
	}}
	status := &cobra.Command{Use: "status <submission-id>", Args: cobra.ExactArgs(1), Short: "显示当前云端治理状态", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.workspaceClient("")
		if err != nil {
			return err
		}
		var value app.SubmissionDetails
		if err := client.Dispatch(cmd.Context(), "submission.workspace-show", map[string]any{"id": args[0]}, &value); err != nil {
			return err
		}
		return r.writeOK("submission.status", map[string]any{"submission_id": value.Submission.ID, "status": value.Submission.Status, "current_revision_id": value.Submission.CurrentRevisionID, "revision_count": len(value.Revisions)})
	}}
	var reason string
	var yes, dryRun bool
	approve := &cobra.Command{Use: "approve <revision-id>", Args: cobra.ExactArgs(1), Short: "记录当前提交版本的内部批准结果", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(reason) == "" {
			return domain.Invalid("APPROVAL_REASON_REQUIRED", "--reason 必填")
		}
		if dryRun {
			return r.writeOK("submission.approve", map[string]any{"dry_run": true, "revision_id": args[0], "reason": reason, "script_requires_client_approval": true})
		}
		if !yes {
			return confirmationRequired("批准后将锁定当前修订版本的内容摘要；视频脚本还需客户使用一次性验证码批准，之后才会创建批准快照")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result app.SubmissionApprovalResult
		if err := client.Dispatch(cmd.Context(), "submission.approve", map[string]any{"revision_id": args[0], "reason": reason}, &result); err != nil {
			return err
		}
		return r.writeOK("submission.approve", result)
	}}
	approve.Flags().StringVar(&reason, "reason", "", "人工批准结论")
	approve.Flags().BoolVar(&yes, "yes", false, "确认写入不可变批准结果")
	approve.Flags().BoolVar(&dryRun, "dry-run", false, "只验证命令，不执行批准")
	var changeReason, jsonPointer string
	var changeYes, changeDryRun bool
	requestChanges := &cobra.Command{Use: "request-changes <revision-id>", Args: cobra.ExactArgs(1), Short: "退回当前版本并提供可执行的修改意见", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(changeReason) == "" {
			return domain.Invalid("CHANGE_REASON_REQUIRED", "--reason 必填")
		}
		if changeDryRun {
			return r.writeOK("submission.request_changes", map[string]any{"dry_run": true, "revision_id": args[0], "reason": changeReason, "json_pointer": jsonPointer})
		}
		if !changeYes {
			return confirmationRequired("提出修改后将记录不可变的审核决定和批注，并把当前提交状态改为“已要求修改”")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var submission domain.Submission
		if err := client.Dispatch(cmd.Context(), "submission.request_changes", map[string]any{"revision_id": args[0], "reason": changeReason, "json_pointer": jsonPointer}, &submission); err != nil {
			return err
		}
		return r.writeOK("submission.request_changes", submission)
	}}
	requestChanges.Flags().StringVar(&changeReason, "reason", "", "可执行的修改要求")
	requestChanges.Flags().StringVar(&jsonPointer, "json-pointer", "", "可选的 RFC 6901 路径，用于定位已提交对象")
	requestChanges.Flags().BoolVar(&changeYes, "yes", false, "确认审核决定")
	requestChanges.Flags().BoolVar(&changeDryRun, "dry-run", false, "只验证命令，不修改云端状态")
	cmd.AddCommand(list, show, status, approve, requestChanges)
	return cmd
}

func (r *Root) workspaceClient(root string) (string, *apiclient.Client, localworkspace.Binding, error) {
	resolved, err := localworkspace.FindRoot(root)
	if err != nil {
		return "", nil, localworkspace.Binding{}, err
	}
	binding, err := localworkspace.ProjectBinding(resolved)
	if err != nil {
		return "", nil, binding, err
	}
	if r.projectID != "" && r.projectID != binding.ProjectID {
		return "", nil, binding, domain.Conflict("PROJECT_CONTEXT_MISMATCH", "--project 与本地工作区绑定不一致")
	}
	token, err := localconfig.WorkspaceToken(binding.WorkspaceID)
	if err != nil {
		return "", nil, binding, &domain.Error{Type: "credential", Subtype: "workspace", Code: "WORKSPACE_CREDENTIAL_MISSING", Message: err.Error(), Hint: "重新运行项目 init，或修复系统安全凭据存储", ExitCode: 3}
	}
	server := binding.ServerURL
	if r.serverURL != "" {
		server = strings.TrimRight(r.serverURL, "/")
	}
	return resolved, apiclient.New(server, token), binding, nil
}

func resolvePublishFiles(root, submissionType string, explicit []string) ([]string, error) {
	if submissionType == "content_batch" && len(explicit) == 1 && filepath.Base(explicit[0]) == "manifest.yaml" {
		return contentBatchPublishFiles(root, explicit[0])
	}
	if len(explicit) > 0 {
		values := make([]string, 0, len(explicit))
		for _, path := range explicit {
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			values = append(values, filepath.Clean(path))
		}
		return values, nil
	}
	if submissionType == "content_batch" {
		return discoverContentBatchPublishFiles(root)
	}
	if submissionType == "storyboard" {
		values, err := filepath.Glob(filepath.Join(root, "50-production", "media", "storyboards", "*", "manifest.json"))
		if err != nil {
			return nil, err
		}
		sort.Strings(values)
		if len(values) == 0 {
			return nil, domain.Invalid("PUBLISH_FILE_REQUIRED", "没有找到可发布的分镜包清单；请使用 --file 明确指定 manifest.json")
		}
		if len(values) > 1 {
			return nil, domain.Invalid("PUBLISH_FILE_AMBIGUOUS", "发现多个分镜包清单；请使用 --file 明确指定本次审核的 manifest.json")
		}
		return values, nil
	}
	directory := map[string]string{
		"context":     "10-context/submissions",
		"knowledge":   "30-knowledge/packs",
		"strategy":    "50-production/strategies",
		"offer":       "50-production/offers",
		"brief":       "50-production/briefs",
		"asset_batch": "50-production/assets",
		"storyboard":  "50-production/media/storyboards",
		"delivery":    "60-delivery/packages",
		"result":      "70-results/submissions",
	}[submissionType]
	if directory == "" {
		return nil, domain.Invalid("SUBMISSION_TYPE_INVALID", "不支持该提交类型（submission_type）")
	}
	values, err := filepath.Glob(filepath.Join(root, directory, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(values)
	if len(values) == 0 {
		return nil, domain.Invalid("PUBLISH_FILE_REQUIRED", "没有找到可发布 JSON；使用 --file 明确指定检查点文件")
	}
	return values, nil
}

func discoverContentBatchPublishFiles(root string) ([]string, error) {
	manifests, err := filepath.Glob(filepath.Join(root, "50-production", "batches", "*", "manifest.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(manifests)
	if len(manifests) == 0 {
		return nil, domain.Invalid("PUBLISH_FILE_REQUIRED", "没有找到可发布的内容批次清单；请使用 --file 明确指定 manifest.yaml")
	}
	if len(manifests) > 1 {
		return nil, domain.Invalid("PUBLISH_FILE_AMBIGUOUS", "发现多个内容批次清单；请使用 --file 明确指定本次审核的 manifest.yaml")
	}
	return contentBatchPublishFiles(root, manifests[0])
}

func contentBatchPublishFiles(root, manifest string) ([]string, error) {
	manifestPath, err := localworkspace.ResolveWorkspaceFile(root, manifest)
	if err != nil {
		return nil, err
	}
	batch, err := localworkspace.LoadContentBatch(root, manifestPath)
	if err != nil {
		return nil, err
	}
	if len(batch.ContentItemRefs) == 0 {
		return nil, domain.Invalid("CONTENT_BATCH_OBJECTS_REQUIRED", "内容批次清单尚未列出可审核的内容项")
	}
	values := make([]string, 0, len(batch.ContentItemRefs))
	for _, ref := range batch.ContentItemRefs {
		resolved, resolveErr := localworkspace.ResolveWorkspaceFile(root, ref)
		if resolveErr != nil {
			return nil, resolveErr
		}
		values = append(values, resolved)
	}
	return values, nil
}

func validatePublishDomainFiles(root, submissionType string, files []string) error {
	switch submissionType {
	case "strategy":
		for _, file := range files {
			body, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var identity struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(body, &identity); err != nil {
				return domain.Invalid("STRATEGY_JSON_INVALID", "策略发布文件不是有效的 JSON："+file)
			}
			var report localworkspace.V5LintReport
			switch identity.Type {
			case "audience_taxonomy_snapshot":
				report, _, err = localworkspace.LintAudienceTaxonomy(root, file, time.Now())
			case "audience_strategy_version":
				report, _, err = localworkspace.LintAudienceStrategy(root, file, time.Now())
			default:
				return domain.Invalid("STRATEGY_OBJECT_TYPE_INVALID", "策略提交只接受受众分类快照或受众策略版本："+file)
			}
			if err != nil {
				return err
			}
			if !report.Valid {
				lintErr := domain.Invalid("STRATEGY_LINT_FAILED", "策略发布前校验失败："+report.File)
				lintErr.Details = report
				return lintErr
			}
		}
	case "offer":
		for _, file := range files {
			report, _, err := localworkspace.LintCommerceOffer(root, file, time.Now())
			if err != nil {
				return err
			}
			if !report.Valid {
				lintErr := domain.Invalid("COMMERCE_OFFER_LINT_FAILED", "商品方案发布前校验失败："+report.File)
				lintErr.Details = report
				return lintErr
			}
		}
	case "brief":
		for _, file := range files {
			body, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var identity struct {
				SchemaVersion string `json:"schema_version"`
			}
			if err := json.Unmarshal(body, &identity); err != nil {
				return domain.Invalid("BRIEF_JSON_INVALID", "创作简报发布文件不是有效的 JSON："+file)
			}
			if identity.SchemaVersion == localworkspace.ArticleBriefSchema {
				report, _, lintErr := localworkspace.LintArticleBrief(root, file)
				if lintErr != nil {
					return lintErr
				}
				if !report.Valid {
					lintErr := domain.Invalid("ARTICLE_BRIEF_LINT_FAILED", "文章简报发布前校验失败："+file)
					lintErr.Details = report
					return lintErr
				}
				continue
			}
			report, _, lintErr := localworkspace.LintBrief(root, file)
			if lintErr != nil {
				return lintErr
			}
			if !report.Valid {
				lintErr := domain.Invalid("BRIEF_LINT_FAILED", "创作简报发布前校验失败："+file)
				lintErr.Details = report
				return lintErr
			}
		}
	case "content_batch":
		for _, file := range files {
			body, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var identity struct {
				SchemaVersion string `json:"schema_version"`
			}
			if err := json.Unmarshal(body, &identity); err != nil {
				return domain.Invalid("CONTENT_ITEM_JSON_INVALID", "内容对象不是有效 JSON："+file)
			}
			switch identity.SchemaVersion {
			case localworkspace.ContentItemSchema:
				report, _, lintErr := localworkspace.LintContentItem(root, file, "")
				if lintErr != nil {
					return lintErr
				}
				if !report.Valid {
					lintErr := domain.Invalid("CONTENT_ITEM_LINT_FAILED", "内容项发布前校验失败："+report.File)
					lintErr.Details = report
					return lintErr
				}
			case localworkspace.ArticleSchema:
				report, _, lintErr := localworkspace.LintArticleItem(root, file, "")
				if lintErr != nil {
					return lintErr
				}
				if !report.Valid {
					lintErr := domain.Invalid("ARTICLE_ITEM_LINT_FAILED", "文章内容项发布前校验失败："+report.File)
					lintErr.Details = report
					return lintErr
				}
			default:
				return domain.Invalid("CONTENT_SCHEMA_UNSUPPORTED", "内容批次包含不受支持的内容结构版本："+identity.SchemaVersion)
			}
		}
	case "storyboard":
		for _, file := range files {
			report, _, err := localworkspace.LintStoryboardPackage(root, file)
			if err != nil {
				return err
			}
			if !report.Valid {
				lintErr := domain.Invalid("STORYBOARD_LINT_FAILED", "分镜包发布前校验失败："+report.File)
				lintErr.Details = report
				return lintErr
			}
		}
	}
	return nil
}

func requiredSubmissionBaseSnapshotIDs(root, submissionType string, objects []domain.SubmissionObjectRef) ([]string, error) {
	values := []string{}
	for _, object := range objects {
		switch submissionType {
		case "strategy":
			if object.Type != "audience_strategy_version" {
				continue
			}
			var strategy domain.AudienceStrategyVersion
			if err := json.Unmarshal(object.Content, &strategy); err != nil {
				return nil, domain.Invalid("AUDIENCE_STRATEGY_JSON_INVALID", "受众策略版本不是有效的 JSON")
			}
			snapshot, err := localworkspace.ApprovedSnapshotForObject(root, "strategy", strategy.TaxonomySnapshotID)
			if err != nil {
				if domain.IsNotFound(err) {
					return nil, domain.Policy("AUDIENCE_TAXONOMY_BASE_SNAPSHOT_REQUIRED", "受众策略版本必须引用本机已拉取的受众分类批准快照", "先执行 contentcloud pull approved --type strategy")
				}
				return nil, err
			}
			values = append(values, snapshot.ID)
		case "storyboard":
			var storyboard domain.StoryboardPackage
			if err := json.Unmarshal(object.Content, &storyboard); err != nil {
				return nil, domain.Invalid("STORYBOARD_JSON_INVALID", "分镜包不是有效的 JSON")
			}
			if strings.TrimSpace(storyboard.ApprovedSnapshotID) == "" {
				return nil, domain.Invalid("STORYBOARD_BASE_SNAPSHOT_REQUIRED", "分镜包缺少 approved_snapshot_id")
			}
			values = append(values, storyboard.ApprovedSnapshotID)
		case "content_batch":
			var identity struct {
				SchemaVersion  string `json:"schema_version"`
				ContentBatchID string `json:"content_batch_id"`
			}
			if err := json.Unmarshal(object.Content, &identity); err != nil {
				return nil, domain.Invalid("CONTENT_ITEM_JSON_INVALID", "内容批次对象不是有效的 JSON")
			}
			if identity.SchemaVersion != localworkspace.ArticleSchema {
				continue
			}
			batch, err := localworkspace.LoadContentBatch(root, filepath.ToSlash(filepath.Join("50-production", "batches", identity.ContentBatchID, "manifest.yaml")))
			if err != nil {
				return nil, err
			}
			values = append(values, batch.BriefSnapshotID)
			values = append(values, batch.KnowledgeSnapshotRefs...)
		}
	}
	return uniqueSortedCLIStrings(values), nil
}

func uniqueSortedCLIStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func publishChecks(submissionType string) []domain.LocalRunCheck {
	checks := []domain.LocalRunCheck{{Name: submissionType + "-json", Status: "passed"}, {Name: submissionType + "-preflight", Status: "passed"}}
	if submissionType == "content_batch" {
		checks[1].Name = "content-batch-lint"
	} else if submissionType == "brief" {
		checks[1].Name = "brief-lint"
	} else if submissionType == "strategy" || submissionType == "offer" || submissionType == "storyboard" {
		checks[1].Name = submissionType + "-lint"
	}
	return checks
}

func readPublishObjects(root, submissionType string, files []string) ([]domain.SubmissionObjectRef, int64, int, string, error) {
	objects := []domain.SubmissionObjectRef{}
	var total int64
	blocked := 0
	for _, path := range files {
		absolute, relative, err := resolvePublishReadPath(root, path, "PUBLISH_PATH_OUTSIDE_WORKSPACE", "publish 文件必须位于当前工作区", "将结构化检查点写入 outputs 或 knowledge 后重试")
		if err != nil {
			return nil, 0, 0, "", err
		}
		body, err := os.ReadFile(absolute)
		if err != nil {
			return nil, 0, 0, "", err
		}
		if len(body) > 20<<20 || total+int64(len(body)) > 50<<20 {
			return nil, 0, 0, "", domain.Invalid("PUBLISH_SIZE_LIMIT", "单个文件不能超过 20 MB，单次发布不能超过 50 MB")
		}
		total += int64(len(body))
		trimmed := strings.TrimSpace(string(body))
		if !json.Valid(body) {
			return nil, 0, 0, "", domain.Invalid("PUBLISH_JSON_INVALID", "发布文件不是有效的 JSON："+relative)
		}
		contents := []json.RawMessage{}
		if strings.HasPrefix(trimmed, "[") {
			var values []json.RawMessage
			if err := json.Unmarshal(body, &values); err != nil {
				return nil, 0, 0, "", err
			}
			contents = values
		} else {
			contents = []json.RawMessage{append(json.RawMessage(nil), body...)}
		}
		for _, content := range contents {
			objectBlocked, err := validatePublishObject(submissionType, content)
			if err != nil {
				return nil, 0, 0, "", domain.Invalid("PUBLISH_OBJECT_INVALID", fmt.Sprintf("对象 %d: %s", len(objects)+1, err.Error()))
			}
			if objectBlocked {
				blocked++
			}
			var identity map[string]any
			_ = json.Unmarshal(content, &identity)
			id := stringField(identity, "id")
			if id == "" {
				return nil, 0, 0, "", domain.Invalid("PUBLISH_OBJECT_ID_REQUIRED", "每个发布对象都必须有稳定的 ID")
			}
			objectType := stringField(identity, "type")
			if objectType == "" {
				objectType = stringField(identity, "kind")
			}
			if objectType == "" {
				objectType = submissionType
			}
			version := integerField(identity, "version")
			if version < 1 {
				version = 1
			}
			object, err := domain.NewSubmissionObjectRef(id, objectType, version, filepath.ToSlash(relative), content)
			if err != nil {
				return nil, 0, 0, "", err
			}
			objects = append(objects, object)
		}
	}
	inputHash, err := domain.CanonicalHash(objects)
	if err != nil {
		return nil, 0, 0, "", err
	}
	return objects, total, blocked, inputHash, nil
}

func validatePublishObject(submissionType string, body json.RawMessage) (bool, error) {
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return false, fmt.Errorf("必须是 JSON 对象")
	}
	switch submissionType {
	case "knowledge":
		if stringField(object, "id") == "" || stringField(object, "kind") == "" {
			return false, fmt.Errorf("知识对象需要 id 和 kind")
		}
	case "brief":
		if stringField(object, "objective") == "" || stringField(object, "audience") == "" {
			return false, fmt.Errorf("简报需要 objective 和 audience")
		}
	case "content_batch":
		schemaVersion := stringField(object, "schema_version")
		blocked := stringField(object, "deliverability") == "blocked" || stringField(object, "status") == "blocked"
		switch schemaVersion {
		case localworkspace.ContentItemSchema:
			if stringField(object, "title") == "" {
				return false, fmt.Errorf("视频 ContentItem 需要 title")
			}
			shots, ok := object["shots"].([]any)
			if !ok || (!blocked && len(shots) == 0) {
				return false, fmt.Errorf("非 blocked 视频 ContentItem 需要至少一个 shot")
			}
		case localworkspace.ArticleSchema:
			if stringField(object, "type") != "article_item" || stringField(object, "selected_title_id") == "" {
				return false, fmt.Errorf("文章对象需要 type=article_item 和 selected_title_id")
			}
			blocks, ok := object["blocks"].([]any)
			if !ok || (!blocked && len(blocks) == 0) {
				return false, fmt.Errorf("未阻断的文章对象至少需要一个内容块")
			}
		default:
			return false, fmt.Errorf("不支持该内容 Schema：%s", schemaVersion)
		}
		if blocked {
			reasons, ok := object["blocked_reasons"].([]any)
			if !ok || len(reasons) == 0 {
				return false, fmt.Errorf("已阻断的内容对象需要 blocked_reasons")
			}
		}
	case "strategy":
		if objectType := stringField(object, "type"); objectType != "audience_taxonomy_snapshot" && objectType != "audience_strategy_version" {
			return false, fmt.Errorf("策略 type 必须是 audience_taxonomy_snapshot 或 audience_strategy_version")
		}
	case "offer":
		if stringField(object, "type") != "commerce_offer_snapshot" {
			return false, fmt.Errorf("报价 type 必须是 commerce_offer_snapshot")
		}
	case "storyboard":
		if stringField(object, "type") != "storyboard_package" || stringField(object, "status") != "review_ready" {
			return false, fmt.Errorf("分镜必须是 review_ready StoryboardPackage")
		}
	}
	deliverability := stringField(object, "deliverability")
	status := stringField(object, "status")
	return deliverability == "blocked" || status == "blocked", nil
}

func readDisclosures(root, path string) ([]domain.SourceDisclosure, int64, error) {
	if strings.TrimSpace(path) == "" {
		return []domain.SourceDisclosure{}, 0, nil
	}
	absolute, _, err := resolvePublishReadPath(root, path, "DISCLOSURE_PATH_OUTSIDE_WORKSPACE", "来源披露文件必须位于当前工作区", "将披露 manifest 放入工作区后重试")
	if err != nil {
		return nil, 0, err
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return nil, 0, err
	}
	var values []domain.SourceDisclosure
	if err := json.Unmarshal(body, &values); err != nil {
		return nil, 0, domain.Invalid("DISCLOSURE_JSON_INVALID", "来源披露文件必须是 JSON 数组")
	}
	return values, int64(len(body)), nil
}

func resolvePublishReadPath(root, path, code, message, hint string) (string, string, error) {
	resolved, err := localworkspace.ResolveWorkspaceFile(root, path)
	if err != nil {
		var domainError *domain.Error
		if errors.As(err, &domainError) && domainError.Code == "LOCAL_FILE_OUTSIDE_WORKSPACE" {
			return "", "", domain.Policy(code, message, hint)
		}
		return "", "", err
	}
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbsolute)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(rootResolved, resolved)
	if err != nil {
		return "", "", err
	}
	return resolved, relative, nil
}

func relativePaths(root string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, path := range values {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			relative = filepath.Base(path)
		}
		out = append(out, filepath.ToSlash(relative))
	}
	return out
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func integerField(object map[string]any, key string) int {
	value, _ := object[key].(float64)
	return int(value)
}
