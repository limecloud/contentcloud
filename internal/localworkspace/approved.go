package localworkspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type ApprovedSnapshotCacheSummary struct {
	ID                   string    `json:"id"`
	Path                 string    `json:"path"`
	SHA256               string    `json:"sha256"`
	SubmissionID         string    `json:"submission_id,omitempty"`
	SubmissionRevisionID string    `json:"submission_revision_id,omitempty"`
	SubmissionType       string    `json:"submission_type"`
	SchemaVersion        string    `json:"schema_version,omitempty"`
	ContentHash          string    `json:"content_hash,omitempty"`
	SubjectHash          string    `json:"subject_hash,omitempty"`
	EligibleIDs          []string  `json:"eligible_ids"`
	CreatedAt            time.Time `json:"created_at"`
}

type ApprovedSnapshotCacheRecord struct {
	Summary  ApprovedSnapshotCacheSummary `json:"summary"`
	Snapshot domain.ApprovedSnapshot      `json:"snapshot"`
}

func StoreApprovedSnapshot(root string, snapshot domain.ApprovedSnapshot, now time.Time) (ApprovedSnapshotCacheRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	if err := validateApprovedSnapshot(resolved, snapshot); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	return storeApprovedSnapshot(resolved, snapshot, now)
}

func StoreApprovedSnapshots(root string, snapshots []domain.ApprovedSnapshot, now time.Time) ([]ApprovedSnapshotCacheRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if err := validateApprovedSnapshot(resolved, snapshot); err != nil {
			return nil, err
		}
	}
	ordered := append([]domain.ApprovedSnapshot(nil), snapshots...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})
	records := make([]ApprovedSnapshotCacheRecord, 0, len(ordered))
	for _, snapshot := range ordered {
		record, err := storeApprovedSnapshot(resolved, snapshot, now)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func storeApprovedSnapshot(root string, snapshot domain.ApprovedSnapshot, now time.Time) (ApprovedSnapshotCacheRecord, error) {
	if _, err := StorePulledBundle(root, "approved", snapshot.ID, snapshot, now); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	hash, err := domain.CanonicalHash(snapshot)
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	digestPath := approvedSnapshotDigestPath(root, snapshot.ID)
	if err := storeImmutableDigest(digestPath, "sha256:"+hash+"\n"); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	return loadApprovedSnapshot(root, snapshot.ID)
}

func ApprovedSnapshotInbox(root, submissionType string) ([]ApprovedSnapshotCacheSummary, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return nil, err
	}
	if submissionType != "" && !approvedSubmissionType(submissionType) {
		return nil, domain.Invalid("APPROVED_SNAPSHOT_TYPE_INVALID", "ApprovedSnapshot submission_type 无效")
	}
	paths, err := filepath.Glob(filepath.Join(resolved, ".contentcloud", "cache", "approved", "*", "snapshot.json"))
	if err != nil {
		return nil, err
	}
	items := make([]ApprovedSnapshotCacheSummary, 0, len(paths))
	for _, path := range paths {
		id := filepath.Base(filepath.Dir(path))
		record, err := loadApprovedSnapshot(resolved, id)
		if err != nil {
			return nil, err
		}
		if submissionType == "" || record.Summary.SubmissionType == submissionType {
			items = append(items, record.Summary)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func ShowApprovedSnapshot(root, snapshotID string) (ApprovedSnapshotCacheRecord, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	if !safePulledBundleID(snapshotID) {
		return ApprovedSnapshotCacheRecord{}, domain.Invalid("APPROVED_SNAPSHOT_ID_INVALID", "ApprovedSnapshot ID 无效")
	}
	return loadApprovedSnapshot(resolved, snapshotID)
}

func ApprovedSnapshotForObject(root, submissionType, objectID string) (domain.ApprovedSnapshot, error) {
	_, snapshot, err := latestApprovedObject(root, submissionType, objectID)
	return snapshot, err
}

func loadApprovedSnapshot(root, snapshotID string) (ApprovedSnapshotCacheRecord, error) {
	path := approvedSnapshotPath(root, snapshotID)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ApprovedSnapshotCacheRecord{}, domain.NotFound("本地 ApprovedSnapshot")
	}
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	if err := requireImmutableCacheFile(path); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	var snapshot domain.ApprovedSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return ApprovedSnapshotCacheRecord{}, domain.Invalid("APPROVED_SNAPSHOT_INVALID", "本地 ApprovedSnapshot 不是有效 JSON")
	}
	if snapshot.ID != snapshotID {
		return ApprovedSnapshotCacheRecord{}, domain.Conflict("APPROVED_SNAPSHOT_ID_MISMATCH", "本地 ApprovedSnapshot 目录与内容 ID 不一致")
	}
	if err := validateApprovedSnapshot(root, snapshot); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	hash, err := domain.CanonicalHash(snapshot)
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	digestPath := approvedSnapshotDigestPath(root, snapshotID)
	expected, err := os.ReadFile(digestPath)
	if errors.Is(err, os.ErrNotExist) {
		missing := domain.Conflict("APPROVED_SNAPSHOT_DIGEST_MISSING", "本地 ApprovedSnapshot 缺少可信 digest")
		missing.Hint = "显式执行 contentcloud pull approved 重新验证并升级本地缓存"
		return ApprovedSnapshotCacheRecord{}, missing
	}
	if err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	if err := requireImmutableCacheFile(digestPath); err != nil {
		return ApprovedSnapshotCacheRecord{}, err
	}
	actualDigest := "sha256:" + hash
	if strings.TrimSpace(string(expected)) != actualDigest {
		return ApprovedSnapshotCacheRecord{}, domain.Conflict("APPROVED_SNAPSHOT_DIGEST_MISMATCH", "本地 ApprovedSnapshot 内容与 pull 时记录的 digest 不一致")
	}
	summary := ApprovedSnapshotCacheSummary{
		ID: snapshot.ID, Path: relativeWorkspacePath(root, path), SHA256: actualDigest,
		SubmissionID: snapshot.SubmissionID, SubmissionRevisionID: snapshot.SubmissionRevisionID, SubmissionType: snapshot.SubmissionType,
		SchemaVersion: snapshot.SchemaVersion, ContentHash: snapshot.ContentHash, SubjectHash: snapshot.SubjectHash,
		EligibleIDs: append([]string(nil), snapshot.EligibleIDs...), CreatedAt: snapshot.CreatedAt,
	}
	return ApprovedSnapshotCacheRecord{Summary: summary, Snapshot: snapshot}, nil
}

func validateApprovedSnapshot(root string, snapshot domain.ApprovedSnapshot) error {
	if !safePulledBundleID(snapshot.ID) || !approvedSubmissionType(snapshot.SubmissionType) || snapshot.CreatedAt.IsZero() || len(snapshot.CanonicalContent) == 0 || !json.Valid(snapshot.CanonicalContent) {
		return domain.Invalid("APPROVED_SNAPSHOT_INVALID", "ApprovedSnapshot 缺少有效 ID、类型、canonical content 或创建时间")
	}
	status, err := LoadStatus(root)
	if err != nil {
		return err
	}
	if snapshot.ProjectID != "" && snapshot.ProjectID != status.Binding.ProjectID {
		return domain.Conflict("APPROVED_SNAPSHOT_PROJECT_MISMATCH", "ApprovedSnapshot 不属于当前项目")
	}
	if snapshot.WorkspaceID != "" && snapshot.WorkspaceID != status.Binding.WorkspaceID {
		return domain.Conflict("APPROVED_SNAPSHOT_WORKSPACE_MISMATCH", "ApprovedSnapshot 不属于当前工作区")
	}
	var canonical struct {
		SchemaVersion  string            `json:"schema_version"`
		SubmissionType string            `json:"submission_type"`
		Objects        []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(snapshot.CanonicalContent, &canonical); err != nil || canonical.SubmissionType != snapshot.SubmissionType || canonical.Objects == nil {
		return domain.Invalid("APPROVED_SNAPSHOT_CANONICAL_INVALID", "ApprovedSnapshot canonical content 与提交类型不一致或缺少 objects 数组")
	}
	if snapshot.SchemaVersion != "" && canonical.SchemaVersion != snapshot.SchemaVersion {
		return domain.Conflict("APPROVED_SNAPSHOT_SCHEMA_MISMATCH", "ApprovedSnapshot schema_version 与 canonical content 不一致")
	}
	objectIDs := map[string]bool{}
	for _, raw := range canonical.Objects {
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) != nil || strings.TrimSpace(identity.ID) == "" {
			return domain.Invalid("APPROVED_SNAPSHOT_OBJECT_INVALID", "ApprovedSnapshot canonical content 包含无 ID 对象")
		}
		objectIDs[identity.ID] = true
	}
	eligible := map[string]bool{}
	for _, id := range snapshot.EligibleIDs {
		if strings.TrimSpace(id) == "" || eligible[id] || !objectIDs[id] {
			return domain.Invalid("APPROVED_SNAPSHOT_ELIGIBLE_INVALID", "ApprovedSnapshot eligible_ids 包含空值、重复值或非 canonical 对象")
		}
		eligible[id] = true
	}
	if snapshot.ContentHash != "" && snapshot.SubjectHash != "" && normalizeApprovedHash(snapshot.ContentHash) != normalizeApprovedHash(snapshot.SubjectHash) {
		return domain.Conflict("APPROVED_SNAPSHOT_SUBJECT_MISMATCH", "ApprovedSnapshot content_hash 与 subject_hash 不一致")
	}
	return nil
}

func storeImmutableDigest(path, value string) error {
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) != value {
			return domain.Conflict("APPROVED_SNAPSHOT_DIGEST_CONFLICT", "本地 ApprovedSnapshot 已有不同 digest")
		}
		return requireImmutableCacheFile(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0o400)
}

func requireImmutableCacheFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return domain.Conflict("APPROVED_SNAPSHOT_CACHE_MUTABLE", "ApprovedSnapshot 缓存文件不是只读普通文件")
	}
	return nil
}

func approvedSnapshotPath(root, snapshotID string) string {
	return filepath.Join(root, ".contentcloud", "cache", "approved", snapshotID, "snapshot.json")
}

func approvedSnapshotDigestPath(root, snapshotID string) string {
	return filepath.Join(root, ".contentcloud", "cache", "approved", snapshotID, "snapshot.sha256")
}

func safePulledBundleID(value string) bool {
	return value != "" && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

func approvedSubmissionType(value string) bool {
	return domain.ValidSubmissionType(value)
}

func normalizeApprovedHash(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "sha256:")
}
