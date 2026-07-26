package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	SubmissionType  string         `json:"submission_type"`
	SchemaVersion   string         `json:"schema_version"`
	Files           []string       `json:"files"`
	ObjectCount     int            `json:"object_count"`
	BlockedCount    int            `json:"blocked_count"`
	DisclosureCount map[string]int `json:"disclosure_count"`
	UploadBytes     int64          `json:"upload_bytes"`
	ContentHash     string         `json:"content_hash"`
	BaseSnapshotID  string         `json:"base_approved_snapshot_id,omitempty"`
	ReviewVisible   []string       `json:"review_visible"`
	RawFilesUpload  bool           `json:"raw_files_upload"`
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
	cmd := &cobra.Command{Use: "publish", Short: "Publish an immutable local checkpoint for cloud review"}
	for _, submissionType := range []string{"knowledge", "research", "strategy", "brief", "script", "delivery", "performance"} {
		typeName := submissionType
		var files []string
		var disclosuresFile, message, idempotencyKey string
		var dryRun, review, yes bool
		publish := &cobra.Command{Use: typeName, Short: "Publish a " + typeName + " checkpoint", RunE: func(command *cobra.Command, args []string) error {
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
			if !review && !yes {
				return confirmationRequired("发布会将 preflight 中列出的结构化对象和来源披露发送到云端审核；raw 原始文件不会上传")
			}
			_, client, _, err := r.workspaceClient(root)
			if err != nil {
				return err
			}
			var revision domain.SubmissionRevision
			if err := client.Dispatch(command.Context(), "submission.create", bundle, &revision); err != nil {
				return err
			}
			if err := localworkspace.RecordPublished(root, typeName, revision.ID, revision.ContentHash, time.Now()); err != nil {
				return err
			}
			return r.writeOK("publish."+typeName, map[string]any{"submission_revision": revision, "preflight": preflight, "next_command": "contentcloud submission status " + revision.SubmissionID})
		}}
		publish.Flags().StringSliceVar(&files, "file", nil, "JSON checkpoint file; repeat for multiple files")
		publish.Flags().StringVar(&disclosuresFile, "disclosures", "", "JSON array describing per-source disclosure levels")
		publish.Flags().StringVar(&message, "message", "", "review note")
		publish.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "stable retry key; defaults to type and content hash")
		publish.Flags().BoolVar(&dryRun, "dry-run", false, "run local preflight without credentials or network writes")
		publish.Flags().BoolVar(&review, "review", false, "confirm publishing this checkpoint for review")
		publish.Flags().BoolVar(&yes, "yes", false, "confirm publishing this checkpoint")
		cmd.AddCommand(publish)
	}
	return cmd
}

func buildPublishCheckpoint(options publishBuildOptions) (domain.SubmissionBundle, publishPreflight, error) {
	resolvedFiles, err := resolvePublishFiles(options.Root, options.SubmissionType, options.Files)
	if err != nil {
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
	bundle := domain.SubmissionBundle{
		BundleVersion: "1.0", SchemaVersion: publishSchemaVersion(options.SubmissionType), SubmissionType: options.SubmissionType,
		ProjectID: status.Binding.ProjectID, WorkspaceID: status.Binding.WorkspaceID, BaseApprovedSnapshotID: status.Sync.ApprovedSnapshotID,
		LocalRunSummary: domain.LocalRunSummary{Stage: "publish_preflight", Checks: []domain.LocalRunCheck{{Name: options.SubmissionType + "-json", Status: "passed"}, {Name: options.SubmissionType + "-lint", Status: "passed"}}, InputHash: inputHash, OutputHash: inputHash, Versions: map[string]string{"cli": Version, "template": status.Template.TemplateVersion}},
		Objects:         objects, SourceDisclosures: disclosures, Artifacts: []domain.SubmissionArtifact{}, Message: strings.TrimSpace(options.Message), IdempotencyKey: options.IdempotencyKey,
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
		SubmissionType: options.SubmissionType, SchemaVersion: bundle.SchemaVersion, Files: relativePaths(options.Root, resolvedFiles), ObjectCount: countJSONArray(objects), BlockedCount: blocked,
		DisclosureCount: counts, UploadBytes: fileBytes + disclosureBytes, ContentHash: bundle.ContentHash, BaseSnapshotID: bundle.BaseApprovedSnapshotID,
		ReviewVisible: []string{"objects", "local_run_summary", "source_disclosures", "artifact_manifest"}, RawFilesUpload: false,
	}
	return bundle, preflight, nil
}

func (r *Root) pullCommand() *cobra.Command {
	var submissionType, id string
	var dryRun bool
	cmd := &cobra.Command{Use: "pull <feedback|decisions|approved>", Args: cobra.ExactArgs(1), Short: "Pull immutable review bundles without changing business files", RunE: func(cmd *cobra.Command, args []string) error {
		kind := args[0]
		if kind != "feedback" && kind != "decisions" && kind != "approved" {
			return domain.Invalid("PULL_KIND_INVALID", "pull 只支持 feedback、decisions 或 approved")
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
					path, err := localworkspace.StorePulledBundle(root, kind, bundle.SubmissionRevisionID, bundle, time.Now())
					if err != nil {
						return err
					}
					downloaded = append(downloaded, path)
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
				for index := len(snapshots) - 1; index >= 0; index-- {
					snapshot := snapshots[index]
					path, err := localworkspace.StorePulledBundle(root, kind, snapshot.ID, snapshot, time.Now())
					if err != nil {
						return err
					}
					downloaded = append(downloaded, path)
				}
			}
		}
		return r.writeOK("pull."+kind, map[string]any{"dry_run": dryRun, "count": count, "downloaded": downloaded, "business_files_modified": false})
	}}
	cmd.Flags().StringVar(&submissionType, "type", "", "filter approved snapshots by submission type")
	cmd.Flags().StringVar(&id, "id", "", "pull one approved snapshot by ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show remote bundle count without writing local files")
	return cmd
}

func (r *Root) submissionCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "submission", Short: "Inspect workspace submissions and perform human governance actions"}
	list := &cobra.Command{Use: "list", Short: "List submissions created by this workspace", RunE: func(cmd *cobra.Command, args []string) error {
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
	show := &cobra.Command{Use: "show <submission-id>", Args: cobra.ExactArgs(1), Short: "Show one submission and immutable revisions", RunE: func(cmd *cobra.Command, args []string) error {
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
	status := &cobra.Command{Use: "status <submission-id>", Args: cobra.ExactArgs(1), Short: "Show the current cloud governance state", RunE: func(cmd *cobra.Command, args []string) error {
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
	approve := &cobra.Command{Use: "approve <revision-id>", Args: cobra.ExactArgs(1), Short: "Approve a current SubmissionRevision and create an ApprovedSnapshot", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(reason) == "" {
			return domain.Invalid("APPROVAL_REASON_REQUIRED", "--reason 必填")
		}
		if dryRun {
			return r.writeOK("submission.approve", map[string]any{"dry_run": true, "revision_id": args[0], "reason": reason, "would_create_approved_snapshot": true})
		}
		if !yes {
			return confirmationRequired("批准会锁定当前 revision hash 并创建不可变 ApprovedSnapshot")
		}
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var snapshot domain.ApprovedSnapshot
		if err := client.Dispatch(cmd.Context(), "submission.approve", map[string]any{"revision_id": args[0], "reason": reason}, &snapshot); err != nil {
			return err
		}
		return r.writeOK("submission.approve", snapshot)
	}}
	approve.Flags().StringVar(&reason, "reason", "", "human approval conclusion")
	approve.Flags().BoolVar(&yes, "yes", false, "confirm immutable approval")
	approve.Flags().BoolVar(&dryRun, "dry-run", false, "validate command without approving")
	var changeReason, jsonPointer string
	var changeYes, changeDryRun bool
	requestChanges := &cobra.Command{Use: "request-changes <revision-id>", Args: cobra.ExactArgs(1), Short: "Return the current revision with actionable feedback", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(changeReason) == "" {
			return domain.Invalid("CHANGE_REASON_REQUIRED", "--reason 必填")
		}
		if changeDryRun {
			return r.writeOK("submission.request_changes", map[string]any{"dry_run": true, "revision_id": args[0], "reason": changeReason, "json_pointer": jsonPointer})
		}
		if !changeYes {
			return confirmationRequired("提出修改会记录不可变决定和批注，并把当前 Submission 状态改为 changes_requested")
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
	requestChanges.Flags().StringVar(&changeReason, "reason", "", "actionable revision request")
	requestChanges.Flags().StringVar(&jsonPointer, "json-pointer", "", "optional RFC 6901 path into the submitted objects")
	requestChanges.Flags().BoolVar(&changeYes, "yes", false, "confirm the review decision")
	requestChanges.Flags().BoolVar(&changeDryRun, "dry-run", false, "validate command without changing cloud state")
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
	directory := map[string]string{"knowledge": "knowledge/packs", "brief": "outputs/briefs", "script": "outputs/scripts"}[submissionType]
	if directory == "" {
		directory = filepath.Join("outputs", submissionType)
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

func readPublishObjects(root, submissionType string, files []string) (json.RawMessage, int64, int, string, error) {
	objects := []json.RawMessage{}
	var total int64
	hasher := sha256.New()
	blocked := 0
	for _, path := range files {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, 0, 0, "", err
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, 0, 0, "", domain.Policy("PUBLISH_PATH_OUTSIDE_WORKSPACE", "publish 文件必须位于当前工作区", "将结构化检查点写入 outputs 或 knowledge 后重试")
		}
		body, err := os.ReadFile(absolute)
		if err != nil {
			return nil, 0, 0, "", err
		}
		if len(body) > 20<<20 || total+int64(len(body)) > 50<<20 {
			return nil, 0, 0, "", domain.Invalid("PUBLISH_SIZE_LIMIT", "单文件不能超过 20 MB，单次 publish 不能超过 50 MB")
		}
		total += int64(len(body))
		_, _ = hasher.Write(body)
		trimmed := strings.TrimSpace(string(body))
		if !json.Valid(body) {
			return nil, 0, 0, "", domain.Invalid("PUBLISH_JSON_INVALID", "publish 文件不是有效 JSON："+relative)
		}
		if strings.HasPrefix(trimmed, "[") {
			var values []json.RawMessage
			if err := json.Unmarshal(body, &values); err != nil {
				return nil, 0, 0, "", err
			}
			objects = append(objects, values...)
		} else {
			objects = append(objects, append(json.RawMessage(nil), body...))
		}
	}
	for index, object := range objects {
		objectBlocked, err := validatePublishObject(submissionType, object)
		if err != nil {
			return nil, 0, 0, "", domain.Invalid("PUBLISH_OBJECT_INVALID", fmt.Sprintf("对象 %d: %s", index+1, err.Error()))
		}
		if objectBlocked {
			blocked++
		}
	}
	body, err := json.Marshal(objects)
	if err != nil {
		return nil, 0, 0, "", err
	}
	return body, total, blocked, hex.EncodeToString(hasher.Sum(nil)), nil
}

func validatePublishObject(submissionType string, body json.RawMessage) (bool, error) {
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return false, fmt.Errorf("必须是 JSON object")
	}
	switch submissionType {
	case "knowledge":
		if stringField(object, "id") == "" || stringField(object, "kind") == "" {
			return false, fmt.Errorf("knowledge 对象需要 id 和 kind")
		}
	case "brief":
		if stringField(object, "objective") == "" || stringField(object, "audience") == "" {
			return false, fmt.Errorf("brief 需要 objective 和 audience")
		}
	case "script":
		if stringField(object, "schema_version") == "" || stringField(object, "title") == "" {
			return false, fmt.Errorf("script 需要 schema_version 和 title")
		}
		shots, ok := object["shots"].([]any)
		if !ok || len(shots) == 0 {
			return false, fmt.Errorf("script 需要至少一个 shot")
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
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, 0, domain.Policy("DISCLOSURE_PATH_OUTSIDE_WORKSPACE", "来源披露文件必须位于当前工作区", "将披露 manifest 放入工作区后重试")
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

func publishSchemaVersion(submissionType string) string {
	if submissionType == "script" {
		return "script-package/1.1"
	}
	return "contentcloud." + submissionType + "/2.0"
}

func countJSONArray(body json.RawMessage) int {
	var values []json.RawMessage
	_ = json.Unmarshal(body, &values)
	return len(values)
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
