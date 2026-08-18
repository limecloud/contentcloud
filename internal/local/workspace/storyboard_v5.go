package localworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"

	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	"github.com/limecloud/contentcloud/internal/work"
)

type CreateStoryboardPackageOptions struct {
	Root               string
	ApprovedSnapshotID string
	ContentItemID      string
	Capability         work.CapabilityRef
	PackageID          string
}

type CreateStoryboardPackageResult struct {
	ManifestPath string                 `json:"manifest_path"`
	ShotPaths    []string               `json:"shot_paths"`
	Package      work.StoryboardPackage `json:"package"`
}

func CreateStoryboardPackage(options CreateStoryboardPackageOptions) (CreateStoryboardPackageResult, error) {
	root, err := FindRoot(options.Root)
	if err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	if err := options.Capability.Validate(); err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	record, err := ShowApprovedSnapshot(root, options.ApprovedSnapshotID)
	if err != nil {
		if fault.IsNotFound(err) {
			return CreateStoryboardPackageResult{}, fault.Policy("CONTENT_SNAPSHOT_PULL_REQUIRED", "本机没有指定的 ApprovedSnapshot", "先执行 contentcloud pull approved --id <snapshot-id>")
		}
		return CreateStoryboardPackageResult{}, err
	}
	if record.Snapshot.SubmissionType != "content_batch" {
		return CreateStoryboardPackageResult{}, fault.Invalid("CONTENT_SNAPSHOT_TYPE_INVALID", "分镜输入必须是 content_batch 类型的批准快照")
	}
	raw, err := approvedObjectContent(record.Snapshot, options.ContentItemID)
	if err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	var item ContentItem
	if err := json.Unmarshal(raw, &item); err != nil || item.Type != "content_item" || item.Deliverability != "review_ready" {
		return CreateStoryboardPackageResult{}, fault.Policy("CONTENT_ITEM_NOT_PRODUCTION_READY", "批准快照中的 ContentItem 无效或仍 blocked", "修订并重新批准 ContentItem")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	sourceHash, err := stablehash.Sum(item)
	if err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	if options.PackageID == "" {
		options.PackageID = idgen.New()
	}
	if localSafeName(options.PackageID) != options.PackageID {
		return CreateStoryboardPackageResult{}, fault.Invalid("STORYBOARD_PACKAGE_ID_INVALID", "分镜包 ID 只能包含字母、数字、点、下划线和连字符")
	}
	shots := make([]work.StoryboardShot, 0, len(item.Shots))
	rights := []string{}
	for _, shot := range item.Shots {
		shots = append(shots, work.StoryboardShot{
			ShotID: shot.ShotID, StartMS: shot.StartMS, EndMS: shot.EndMS, Role: shot.Role,
			ImagePromptZH: strings.TrimSpace(shot.FirstFrame.PromptZH), Subject: shot.Subject, Product: shot.ProductTruthStrategy,
			Scene: shot.FirstFrame.VisualState, Composition: shot.Composition, Lighting: shot.Continuity.LightingLock,
			Camera: strings.TrimSpace(shot.CameraMotion), Action: strings.TrimSpace(shot.MotionSpec),
			IncomingState: shot.Continuity.IncomingState, OutgoingState: shot.Continuity.OutgoingState, MovementAxis: shot.Continuity.MovementAxis,
			LightingLock: shot.Continuity.LightingLock, ProductLock: shot.Continuity.ProductLock, Anchors: append([]string(nil), shot.Continuity.Anchors...),
			AssetRefs: append([]string(nil), shot.AssetRefs...), RightsRefs: append([]string(nil), shot.RightsRefs...), KnowledgeRefs: append([]string(nil), shot.KnowledgeRefs...), ClaimRefs: append([]string(nil), shot.ClaimRefs...),
			NegativeConstraints: append([]string(nil), shot.NegativeConstraints...), AcceptanceCriteria: append([]string(nil), shot.AcceptanceCriteria...), PlanB: shot.PlanB,
		})
		rights = append(rights, shot.RightsRefs...)
	}
	value := work.StoryboardPackage{
		ID: options.PackageID, Type: "storyboard_package", SchemaVersion: work.StoryboardPackageSchema,
		ProjectID: status.Binding.ProjectID, ApprovedSnapshotID: record.Snapshot.ID, ContentItemID: item.ID,
		GeneratorCapability: options.Capability, Status: "candidate", Shots: shots, Assets: []work.StoryboardAsset{},
		RightsRefs: uniqueSortedStrings(rights), SourceDigest: "sha256:" + sourceHash,
	}
	value.LockedDigest, err = storyboardLockedDigest(value)
	if err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	if err := value.Validate(false); err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	directory := filepath.Join(root, "50-production", "media", "storyboards", value.ID)
	shotPaths := make([]string, 0, len(value.Shots))
	for _, shot := range value.Shots {
		shotPath := filepath.Join(directory, "shots", storyboardShotDirectoryName(shot.ShotID))
		if err := os.MkdirAll(shotPath, 0o700); err != nil {
			return CreateStoryboardPackageResult{}, err
		}
		shotPaths = append(shotPaths, relativeWorkspacePath(root, shotPath))
	}
	manifestPath := filepath.Join(directory, "manifest.json")
	if err := writeJSON(manifestPath, value); err != nil {
		return CreateStoryboardPackageResult{}, err
	}
	return CreateStoryboardPackageResult{ManifestPath: relativeWorkspacePath(root, manifestPath), ShotPaths: shotPaths, Package: value}, nil
}

func PrepareStoryboardReview(root, manifest string) (V5LintReport, work.StoryboardPackage, error) {
	resolved, path, err := resolveV5JSON(root, manifest)
	if err != nil {
		return V5LintReport{}, work.StoryboardPackage{}, err
	}
	var value work.StoryboardPackage
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, fault.Invalid("STORYBOARD_JSON_INVALID", err.Error())
	}
	if value.Status != "candidate" && value.Status != "review_ready" {
		return V5LintReport{}, value, fault.Conflict("STORYBOARD_STATE_INVALID", "只能准备 candidate 或 review_ready 分镜包")
	}
	assets, shots, reviewSheetID, err := discoverStoryboardAssets(resolved, filepath.Dir(path), value)
	if err != nil {
		return V5LintReport{}, value, err
	}
	value.Assets = assets
	value.Shots = shots
	value.ReviewSheetArtifactID = reviewSheetID
	value.Status = "review_ready"
	value.LockedDigest, err = storyboardLockedDigest(value)
	if err != nil {
		return V5LintReport{}, value, err
	}
	if err := value.Validate(true); err != nil {
		return V5LintReport{}, value, err
	}
	if err := replaceJSON(path, value, 0o600); err != nil {
		return V5LintReport{}, value, err
	}
	report, linted, err := LintStoryboardPackage(resolved, path)
	return report, linted, err
}

func LintStoryboardPackage(root, manifest string) (V5LintReport, work.StoryboardPackage, error) {
	resolved, path, err := resolveV5JSON(root, manifest)
	if err != nil {
		return V5LintReport{}, work.StoryboardPackage{}, err
	}
	var value work.StoryboardPackage
	if err := readStrictJSON(path, &value); err != nil {
		return V5LintReport{}, value, fault.Invalid("STORYBOARD_JSON_INVALID", err.Error())
	}
	report := v5Report(resolved, path, value.ID, value.Type)
	if err := value.Validate(true); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	if err := validateStoryboardSource(resolved, value); err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: domainErrorCode(err), Message: err.Error()})
	}
	for _, asset := range value.Assets {
		absolute, err := ResolveWorkspaceFile(resolved, asset.Path)
		if err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: "STORYBOARD_ASSET_PATH_INVALID", Message: err.Error()})
			continue
		}
		sha, size, err := fileDigest(absolute)
		if err != nil {
			report.Issues = append(report.Issues, V5LintIssue{Code: "STORYBOARD_ASSET_UNREADABLE", Message: asset.Path + ": " + err.Error()})
			continue
		}
		if sha != asset.SHA256 || size != asset.ByteSize {
			report.Issues = append(report.Issues, V5LintIssue{Code: "STORYBOARD_ASSET_DIGEST_MISMATCH", Message: "素材内容与 manifest 摘要不一致：" + asset.Path})
		}
	}
	computed, err := storyboardLockedDigest(value)
	if err != nil {
		report.Issues = append(report.Issues, V5LintIssue{Code: "STORYBOARD_DIGEST_FAILED", Message: err.Error()})
	} else if computed != value.LockedDigest {
		report.Issues = append(report.Issues, V5LintIssue{Code: "STORYBOARD_LOCKED_DIGEST_MISMATCH", Message: "分镜 manifest 或素材列表已在准备审核后变化"})
	}
	report.LockedDigest = value.LockedDigest
	report = finishV5Report(report, value)
	return report, value, nil
}

func validateStoryboardSource(root string, value work.StoryboardPackage) error {
	record, err := ShowApprovedSnapshot(root, value.ApprovedSnapshotID)
	if err != nil {
		if fault.IsNotFound(err) {
			return fault.Policy("STORYBOARD_CONTENT_SNAPSHOT_PULL_REQUIRED", "本机没有分镜包引用的内容批次批准快照", "先执行 contentcloud pull approved --id <snapshot-id>")
		}
		return err
	}
	if record.Snapshot.SubmissionType != "content_batch" {
		return fault.Conflict("STORYBOARD_CONTENT_SNAPSHOT_INVALID", "分镜包必须引用内容批次批准快照")
	}
	raw, err := approvedObjectContent(record.Snapshot, value.ContentItemID)
	if err != nil {
		return fault.Conflict("STORYBOARD_CONTENT_ITEM_BASE_INVALID", "分镜包的 content_item_id 不在所引用批准快照的可用对象中")
	}
	hash, err := stablehash.Sum(json.RawMessage(raw))
	if err != nil {
		return err
	}
	if value.SourceDigest != "sha256:"+hash {
		return fault.Conflict("STORYBOARD_SOURCE_DIGEST_MISMATCH", "分镜包的来源摘要与已批准内容项不一致")
	}
	return nil
}

func LoadLockedStoryboardSnapshot(root, snapshotID, packageID string) (work.StoryboardPackage, error) {
	record, err := ShowApprovedSnapshot(root, snapshotID)
	if err != nil {
		if fault.IsNotFound(err) {
			return work.StoryboardPackage{}, fault.Policy("STORYBOARD_SNAPSHOT_PULL_REQUIRED", "本机没有可信的分镜批准快照", "先执行 contentcloud pull approved --id <snapshot-id>")
		}
		return work.StoryboardPackage{}, err
	}
	if record.Snapshot.SubmissionType != "storyboard" {
		return work.StoryboardPackage{}, fault.Invalid("STORYBOARD_SNAPSHOT_TYPE_INVALID", "Seedance 导出只能使用分镜批准快照")
	}
	raw, err := approvedObjectContent(record.Snapshot, packageID)
	if err != nil {
		return work.StoryboardPackage{}, err
	}
	var value work.StoryboardPackage
	if err := json.Unmarshal(raw, &value); err != nil {
		return work.StoryboardPackage{}, fault.Invalid("STORYBOARD_SNAPSHOT_OBJECT_INVALID", "分镜批准快照中的对象无效")
	}
	if err := value.Validate(true); err != nil {
		return work.StoryboardPackage{}, err
	}
	computed, err := storyboardLockedDigest(value)
	if err != nil {
		return work.StoryboardPackage{}, err
	}
	if computed != value.LockedDigest {
		return work.StoryboardPackage{}, fault.Conflict("STORYBOARD_LOCKED_DIGEST_MISMATCH", "无法重新计算服务端已批准分镜包的锁定摘要（locked_digest）")
	}
	for _, asset := range value.Assets {
		absolute, err := ResolveWorkspaceFile(root, asset.Path)
		if err != nil {
			return work.StoryboardPackage{}, err
		}
		sha, size, err := fileDigest(absolute)
		if err != nil {
			return work.StoryboardPackage{}, err
		}
		if sha != asset.SHA256 || size != asset.ByteSize {
			return work.StoryboardPackage{}, fault.Conflict("STORYBOARD_LOCKED_MEDIA_DRIFT", "本地分镜素材与服务端锁定摘要不一致："+asset.Path)
		}
	}
	return value, nil
}

func discoverStoryboardAssets(root, manifestDirectory string, value work.StoryboardPackage) ([]work.StoryboardAsset, []work.StoryboardShot, string, error) {
	preserved := []work.StoryboardAsset{}
	for _, asset := range value.Assets {
		if asset.Role == "identity_anchor" || asset.Role == "reference_video" || asset.Role == "reference_audio" {
			refreshed, err := storyboardAssetFromFile(root, asset.Path, asset.Role, asset.ShotID, asset.RightsRefs)
			if err != nil {
				return nil, nil, "", err
			}
			preserved = append(preserved, refreshed)
		}
	}
	assets := append([]work.StoryboardAsset(nil), preserved...)
	shots := append([]work.StoryboardShot(nil), value.Shots...)
	for index := range shots {
		shot := &shots[index]
		shotDirectory := filepath.Join(manifestDirectory, "shots", storyboardShotDirectoryName(shot.ShotID))
		first, err := findUniqueMediaFile(shotDirectory, "first-frame", []string{".png", ".jpg", ".jpeg", ".webp"}, true)
		if err != nil {
			return nil, nil, "", err
		}
		firstAsset, err := storyboardAssetFromFile(root, first, "first_frame", shot.ShotID, shot.RightsRefs)
		if err != nil {
			return nil, nil, "", err
		}
		shot.FirstFrameArtifactID = firstAsset.ID
		assets = append(assets, firstAsset)
		end, err := findUniqueMediaFile(shotDirectory, "end-frame", []string{".png", ".jpg", ".jpeg", ".webp"}, false)
		if err != nil {
			return nil, nil, "", err
		}
		if end != "" {
			endAsset, err := storyboardAssetFromFile(root, end, "end_frame", shot.ShotID, shot.RightsRefs)
			if err != nil {
				return nil, nil, "", err
			}
			shot.EndFrameArtifactID = endAsset.ID
			assets = append(assets, endAsset)
		} else {
			shot.EndFrameArtifactID = ""
		}
	}
	review, err := findUniqueMediaFile(manifestDirectory, "review-sheet", []string{".png", ".jpg", ".jpeg", ".webp"}, true)
	if err != nil {
		return nil, nil, "", err
	}
	reviewAsset, err := storyboardAssetFromFile(root, review, "review_sheet", "", value.RightsRefs)
	if err != nil {
		return nil, nil, "", err
	}
	assets = append(assets, reviewAsset)
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].Role != assets[j].Role {
			return assets[i].Role < assets[j].Role
		}
		return assets[i].Path < assets[j].Path
	})
	return assets, shots, reviewAsset.ID, nil
}

func storyboardAssetFromFile(root, file, role, shotID string, rights []string) (work.StoryboardAsset, error) {
	absolute, err := ResolveWorkspaceFile(root, file)
	if err != nil {
		return work.StoryboardAsset{}, err
	}
	sha, size, err := fileDigest(absolute)
	if err != nil {
		return work.StoryboardAsset{}, err
	}
	relative := relativeWorkspacePath(root, absolute)
	idHash, err := stablehash.Sum(map[string]string{"path": relative, "role": role, "shot_id": shotID, "sha256": sha})
	if err != nil {
		return work.StoryboardAsset{}, err
	}
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(absolute)))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return work.StoryboardAsset{ID: "sba_" + idHash[:20], Role: role, ShotID: shotID, Path: relative, MediaType: mediaType, SHA256: sha, ByteSize: size, RightsRefs: uniqueSortedStrings(rights)}, nil
}

func findUniqueMediaFile(directory, base string, extensions []string, required bool) (string, error) {
	values := []string{}
	for _, extension := range extensions {
		matches, err := filepath.Glob(filepath.Join(directory, base+extension))
		if err != nil {
			return "", err
		}
		values = append(values, matches...)
	}
	if len(values) == 0 {
		if required {
			return "", fault.Invalid("STORYBOARD_MEDIA_REQUIRED", "缺少分镜媒体："+filepath.Join(directory, base+".png"))
		}
		return "", nil
	}
	if len(values) > 1 {
		return "", fault.Conflict("STORYBOARD_MEDIA_AMBIGUOUS", "同一分镜位置存在多个候选文件："+filepath.Join(directory, base))
	}
	return values[0], nil
}

func storyboardLockedDigest(value work.StoryboardPackage) (string, error) {
	return value.ComputedLockedDigest()
}

func storyboardShotDirectoryName(shotID string) string {
	name := localSafeName(shotID)
	if name == shotID {
		return name
	}
	sum := sha256.Sum256([]byte(shotID))
	return name + "-" + hex.EncodeToString(sum[:4])
}

func approvedObjectContent(snapshot reviewdomain.ApprovedSnapshot, objectID string) (json.RawMessage, error) {
	eligible := map[string]bool{}
	for _, id := range snapshot.EligibleIDs {
		eligible[id] = true
	}
	var canonical struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &canonical); err != nil {
		return nil, fault.Invalid("APPROVED_SNAPSHOT_CANONICAL_INVALID", "批准快照的规范内容无效")
	}
	for _, raw := range canonical.Objects {
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) == nil && identity.ID == objectID && eligible[identity.ID] {
			return raw, nil
		}
	}
	return nil, fault.NotFound("批准快照中的可用对象")
}

func fileDigest(path string) (string, int64, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("素材不是普通文件")
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), int64(len(body)), nil
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
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
