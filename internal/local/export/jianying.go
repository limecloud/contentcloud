package exportfmt

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/stablehash"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

// JianyingReaders contains read-only ports used to build a derived archive.
// Export never writes business facts or changes delivery state.
type JianyingReaders struct {
	Snapshots interface {
		ApprovedSnapshot(context.Context, string, string) (reviewdomain.ApprovedSnapshot, error)
	}
	Reviews interface {
		MediaReview(context.Context, string, string) (deliverydomain.MediaReview, error)
	}
	Packages interface {
		DeliveryPackage(context.Context, string, string) (deliverydomain.DeliveryPackage, error)
	}
	Artifacts interface {
		Artifact(context.Context, string, string) (deliverydomain.Artifact, error)
	}
	Blobs blob.Store
}

type JianyingExportInput struct {
	TenantID           string
	ProjectID          string
	ApprovedSnapshotID string
	FinalReviewID      string
	DeliveryPackageID  string
	Manifest           deliverydomain.CompositionManifest
}

type JianyingExportResult struct {
	Body           []byte
	ManifestDigest string
	ArchiveDigest  string
}

type JianyingLintInput struct {
	ManifestDigest string
	Artifacts      []deliverydomain.Artifact
}

type draftInfo struct {
	Schema                 string   `json:"schema"`
	Version                string   `json:"version"`
	TenantID               string   `json:"tenant_id"`
	ProjectID              string   `json:"project_id"`
	ApprovedSnapshotID     string   `json:"approved_snapshot_id"`
	ApprovedSnapshotDigest string   `json:"approved_snapshot_digest"`
	FinalReviewID          string   `json:"final_review_id"`
	DeliveryPackageID      string   `json:"delivery_package_id"`
	ManifestDigest         string   `json:"manifest_digest"`
	ArtifactIDs            []string `json:"artifact_ids"`
}

type manifestFile struct {
	Manifest           deliverydomain.CompositionManifest `json:"manifest"`
	Digest             string                             `json:"digest"`
	ApprovedSnapshotID string                             `json:"approved_snapshot_id"`
	FinalReviewID      string                             `json:"final_review_id"`
	DeliveryPackageID  string                             `json:"delivery_package_id"`
}

// ExportJianying creates a byte-stable ZIP suitable for a local Jianying import.
func ExportJianying(ctx context.Context, readers JianyingReaders, input JianyingExportInput) (JianyingExportResult, error) {
	if readers.Snapshots == nil || readers.Reviews == nil || readers.Packages == nil || readers.Artifacts == nil || readers.Blobs == nil {
		return JianyingExportResult{}, fault.Invalid("JIANYING_EXPORT_READERS_REQUIRED", "剪映导出必须配置完整的只读数据源")
	}
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.ApprovedSnapshotID) == "" || strings.TrimSpace(input.FinalReviewID) == "" || strings.TrimSpace(input.DeliveryPackageID) == "" {
		return JianyingExportResult{}, fault.Invalid("JIANYING_EXPORT_INPUT_REQUIRED", "剪映导出必须绑定租户、项目、批准快照、最终审核和交付包")
	}
	if err := input.Manifest.Validate(); err != nil {
		return JianyingExportResult{}, err
	}
	manifestDigest, err := input.Manifest.Digest()
	if err != nil {
		return JianyingExportResult{}, err
	}

	snapshot, err := readers.Snapshots.ApprovedSnapshot(ctx, input.TenantID, input.ApprovedSnapshotID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if snapshot.TenantID != input.TenantID || snapshot.ProjectID != input.ProjectID || snapshot.ID != input.Manifest.ApprovedSnapshot.ID || input.Manifest.ApprovedSnapshot.Digest != snapshotDigest(snapshot) {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_SNAPSHOT_MISMATCH", "剪映导出绑定的批准快照与服务端事实不一致")
	}

	finalReview, err := readers.Reviews.MediaReview(ctx, input.TenantID, input.FinalReviewID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if finalReview.TenantID != input.TenantID || finalReview.ProjectID != input.ProjectID || finalReview.ReviewKind != deliverydomain.MediaReviewFinal || finalReview.Status != deliverydomain.MediaReviewApproved || !finalReview.Selected {
		return JianyingExportResult{}, fault.Policy("JIANYING_EXPORT_FINAL_REVIEW_REQUIRED", "剪映导出必须绑定已批准且选中的最终成片审核", "先完成最终成片审核")
	}
	if input.Manifest.SelectedVideo.MediaReviewID == "" || input.Manifest.SelectedVideo.MediaReviewID == input.FinalReviewID {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_SOURCE_REVIEW_INVALID", "剪映导出必须保留选中视频审核与最终成片审核的独立血缘")
	}
	selectedReview, err := readers.Reviews.MediaReview(ctx, input.TenantID, input.Manifest.SelectedVideo.MediaReviewID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if selectedReview.TenantID != input.TenantID || selectedReview.ProjectID != input.ProjectID || selectedReview.ReviewKind != deliverydomain.MediaReviewContent || selectedReview.Status != deliverydomain.MediaReviewApproved || !selectedReview.Selected || selectedReview.SubjectArtifactID != input.Manifest.SelectedVideo.ArtifactID || input.Manifest.SelectedVideo.ReviewDigest != mediaReviewDigest(selectedReview) {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_SOURCE_REVIEW_MISMATCH", "剪映导出的选中视频审核与合成清单不一致")
	}
	selectedArtifact, err := readers.Artifacts.Artifact(ctx, input.TenantID, input.Manifest.SelectedVideo.ArtifactID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if selectedArtifact.TenantID != input.TenantID || selectedArtifact.ProjectID != input.ProjectID || selectedArtifact.ApprovedSnapshotID != snapshot.ID || selectedArtifact.ID != selectedReview.SubjectArtifactID || normalizedDigest(selectedArtifact.SHA256) != normalizedDigest(selectedReview.SubjectDigest) || normalizedDigest(selectedArtifact.SHA256) != normalizedDigest(input.Manifest.SelectedVideo.Digest) {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_SOURCE_ARTIFACT_MISMATCH", "剪映导出的选中视频 Artifact 与审核或合成清单不一致")
	}
	selectedBody, err := readers.Blobs.Get(ctx, selectedArtifact.ObjectKey)
	if err != nil || int64(len(selectedBody)) != selectedArtifact.ByteSize || digestBytes(selectedBody) != normalizedDigest(selectedArtifact.SHA256) {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_SOURCE_BLOB_INVALID", "选中视频 Artifact 的 Blob 不存在或摘要不一致")
	}

	pkg, err := readers.Packages.DeliveryPackage(ctx, input.TenantID, input.DeliveryPackageID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if pkg.TenantID != input.TenantID || pkg.ProjectID != input.ProjectID || pkg.ID != input.DeliveryPackageID || pkg.Status != "ready" || !contains(pkg.ApprovedSnapshotIDs, snapshot.ID) || len(pkg.Manifest) == 0 {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_DELIVERY_PACKAGE_INVALID", "交付包不属于当前项目、尚未就绪或缺少批准快照")
	}

	manifestArtifacts := make(map[string]deliverydomain.Artifact, len(pkg.Manifest))
	for _, listed := range pkg.Manifest {
		if listed.ID == "" || listed.TenantID != input.TenantID || listed.ProjectID != input.ProjectID || listed.ApprovedSnapshotID != snapshot.ID || !stablehash.Matches(listed.SHA256) || !safeBaseName(listed.FileName) {
			return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_ARTIFACT_INVALID", "交付包包含越权、摘要无效或文件名不安全的成果文件")
		}
		if _, exists := manifestArtifacts[listed.ID]; exists {
			return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_ARTIFACT_DUPLICATE", "交付包成果文件 ID 不能重复")
		}
		manifestArtifacts[listed.ID] = listed
	}
	finalArtifact, err := readers.Artifacts.Artifact(ctx, input.TenantID, finalReview.SubjectArtifactID)
	if err != nil {
		return JianyingExportResult{}, err
	}
	if finalArtifact.TenantID != input.TenantID || finalArtifact.ProjectID != input.ProjectID || finalArtifact.ApprovedSnapshotID != snapshot.ID || finalArtifact.ID != finalReview.SubjectArtifactID || normalizedDigest(finalArtifact.SHA256) != normalizedDigest(finalReview.SubjectDigest) || finalArtifact.Kind != "final_render" || finalArtifact.Purpose != "final_video" {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_FINAL_ARTIFACT_INVALID", "最终审核对应的成果文件不符合交付血缘")
	}
	if _, exists := manifestArtifacts[finalArtifact.ID]; !exists {
		return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_FINAL_ARTIFACT_NOT_DELIVERED", "最终成果文件不在交付包清单中")
	}

	files := make([]archiveFile, 0, len(manifestArtifacts)+2)
	artifactIDs := make([]string, 0, len(manifestArtifacts))
	for id := range manifestArtifacts {
		artifactIDs = append(artifactIDs, id)
	}
	sort.Slice(artifactIDs, func(i, j int) bool {
		a, b := manifestArtifacts[artifactIDs[i]], manifestArtifacts[artifactIDs[j]]
		if a.FileName == b.FileName {
			return a.ID < b.ID
		}
		return a.FileName < b.FileName
	})
	usedPaths := map[string]bool{"draft_info.json": true, "manifest.json": true}
	for _, id := range artifactIDs {
		artifact := manifestArtifacts[id]
		name := "assets/" + artifact.FileName
		if usedPaths[name] {
			return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_PATH_DUPLICATE", "交付包文件名映射后存在重复目标路径")
		}
		usedPaths[name] = true
		body, getErr := readers.Blobs.Get(ctx, artifact.ObjectKey)
		if getErr != nil {
			return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_BLOB_MISSING", "交付包中的成果文件 Blob 不存在")
		}
		if int64(len(body)) != artifact.ByteSize || digestBytes(body) != normalizedDigest(artifact.SHA256) {
			return JianyingExportResult{}, fault.Conflict("JIANYING_EXPORT_BLOB_DIGEST_MISMATCH", "成果文件 Blob 与 Artifact 摘要或大小不一致")
		}
		files = append(files, archiveFile{Name: name, Body: body})
	}

	info, err := json.Marshal(draftInfo{Schema: "contentcloud.jianying-draft/1.0", Version: "1", TenantID: input.TenantID, ProjectID: input.ProjectID, ApprovedSnapshotID: snapshot.ID, ApprovedSnapshotDigest: snapshotDigest(snapshot), FinalReviewID: finalReview.ID, DeliveryPackageID: pkg.ID, ManifestDigest: manifestDigest, ArtifactIDs: artifactIDs})
	if err != nil {
		return JianyingExportResult{}, err
	}
	mf, err := json.Marshal(manifestFile{Manifest: input.Manifest, Digest: manifestDigest, ApprovedSnapshotID: snapshot.ID, FinalReviewID: finalReview.ID, DeliveryPackageID: pkg.ID})
	if err != nil {
		return JianyingExportResult{}, err
	}
	files = append([]archiveFile{{Name: "draft_info.json", Body: info}, {Name: "manifest.json", Body: mf}}, files...)
	body, err := deterministicZIP(files)
	if err != nil {
		return JianyingExportResult{}, err
	}
	return JianyingExportResult{Body: body, ManifestDigest: manifestDigest, ArchiveDigest: digestBytes(body)}, nil
}

// LintJianyingArchive validates the deterministic archive independently from
// the repositories used to create it.
func LintJianyingArchive(body []byte, input JianyingLintInput) error {
	if !stablehash.Valid(input.ManifestDigest) {
		return fault.Invalid("JIANYING_LINT_MANIFEST_DIGEST_INVALID", "导出 lint 必须提供有效的 Manifest 摘要")
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fault.Invalid("JIANYING_LINT_ZIP_INVALID", "剪映导出不是有效 ZIP")
	}
	if len(reader.File) != len(input.Artifacts)+2 {
		return fault.Conflict("JIANYING_LINT_ENTRY_COUNT_INVALID", "剪映 ZIP 条目数量与成果清单不一致")
	}
	entries := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		if file.Name != "draft_info.json" && file.Name != "manifest.json" && (!strings.HasPrefix(file.Name, "assets/") || !safeBaseName(strings.TrimPrefix(file.Name, "assets/"))) {
			return fault.Invalid("JIANYING_LINT_PATH_INVALID", "剪映 ZIP 包含不安全或未知路径")
		}
		if _, exists := entries[file.Name]; exists {
			return fault.Conflict("JIANYING_LINT_DUPLICATE_ENTRY", "剪映 ZIP 包含重复条目")
		}
		entries[file.Name] = file
	}
	readEntry := func(name string) ([]byte, error) {
		file := entries[name]
		if file == nil {
			return nil, fault.Conflict("JIANYING_LINT_ENTRY_MISSING", "剪映 ZIP 缺少必需条目")
		}
		stream, openErr := file.Open()
		if openErr != nil {
			return nil, openErr
		}
		defer stream.Close()
		return io.ReadAll(stream)
	}
	infoBody, err := readEntry("draft_info.json")
	if err != nil {
		return err
	}
	var info draftInfo
	if err := json.Unmarshal(infoBody, &info); err != nil || info.ManifestDigest != input.ManifestDigest {
		return fault.Conflict("JIANYING_LINT_DRAFT_INFO_INVALID", "draft_info.json 的 Manifest 摘要无效")
	}
	manifestBody, err := readEntry("manifest.json")
	if err != nil {
		return err
	}
	var manifest manifestFile
	if err := json.Unmarshal(manifestBody, &manifest); err != nil || manifest.Digest != input.ManifestDigest {
		return fault.Conflict("JIANYING_LINT_MANIFEST_INVALID", "manifest.json 的摘要无效")
	}
	computedManifestDigest, err := manifest.Manifest.Digest()
	if err != nil {
		return err
	}
	if computedManifestDigest != input.ManifestDigest {
		return fault.Conflict("JIANYING_LINT_MANIFEST_DIGEST_MISMATCH", "manifest.json 内容与声明的 Manifest 摘要不一致")
	}
	for _, artifact := range input.Artifacts {
		if !safeBaseName(artifact.FileName) || !stablehash.Matches(artifact.SHA256) || artifact.ByteSize < 0 {
			return fault.Invalid("JIANYING_LINT_ARTIFACT_INVALID", "lint 成果文件元数据无效")
		}
		file := entries["assets/"+artifact.FileName]
		if file == nil {
			return fault.Conflict("JIANYING_LINT_ARTIFACT_MISSING", "lint 缺少成果文件")
		}
		stream, openErr := file.Open()
		if openErr != nil {
			return openErr
		}
		data, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			return readErr
		}
		if int64(len(data)) != artifact.ByteSize || digestBytes(data) != normalizedDigest(artifact.SHA256) {
			return fault.Conflict("JIANYING_LINT_ARTIFACT_DIGEST_MISMATCH", "lint 成果文件摘要或大小不一致")
		}
		if !mimeExtensionCompatible(artifact.MediaType, artifact.FileName) {
			return fault.Invalid("JIANYING_LINT_MEDIA_TYPE_INVALID", "成果文件 MIME 与扩展名不匹配")
		}
	}
	return nil
}

type archiveFile struct {
	Name string
	Body []byte
}

func deterministicZIP(files []archiveFile) ([]byte, error) {
	var output bytes.Buffer
	w := zip.NewWriter(&output)
	for _, file := range files {
		h := &zip.FileHeader{Name: file.Name, Method: zip.Store}
		h.SetModTime(time.Unix(0, 0).UTC())
		h.SetMode(0o644)
		entry, err := w.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(file.Body); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func safeBaseName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && path.Base(value) == value && !strings.ContainsAny(value, `/\\`) && !strings.ContainsRune(value, 0)
}

func snapshotDigest(value reviewdomain.ApprovedSnapshot) string {
	if strings.TrimSpace(value.SubjectHash) != "" {
		return normalizedDigest(value.SubjectHash)
	}
	return normalizedDigest(value.ContentHash)
}
func normalizedDigest(value string) string {
	return "sha256:" + strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}
func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mediaReviewDigest(value deliverydomain.MediaReview) string {
	hash, _ := stablehash.Sum(struct {
		ID            string         `json:"id"`
		SubjectDigest string         `json:"subject_digest"`
		ReviewKind    string         `json:"review_kind"`
		Status        string         `json:"status"`
		Checks        map[string]any `json:"checks"`
		Selected      bool           `json:"selected"`
		RowVersion    int            `json:"row_version"`
	}{value.ID, value.SubjectDigest, value.ReviewKind, value.Status, value.Checks, value.Selected, value.RowVersion})
	return "sha256:" + hash
}

func mimeExtensionCompatible(mediaType, fileName string) bool {
	ext := strings.ToLower(path.Ext(fileName))
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "video/mp4":
		return ext == ".mp4"
	case "video/quicktime":
		return ext == ".mov"
	case "audio/mpeg":
		return ext == ".mp3"
	case "audio/wav", "audio/x-wav":
		return ext == ".wav"
	case "application/json":
		return ext == ".json"
	case "text/plain", "text/markdown":
		return ext == ".txt" || ext == ".md"
	default:
		return true
	}
}
