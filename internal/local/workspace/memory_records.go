package localworkspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const (
	MemoryRecordSchema          = "contentcloud.memory-record/1.0"
	memoryRecordRelativeDir     = "40-work/memory/records"
	memoryRecordSummaryRunes    = 2000
	memoryRecordDefaultFormedBy = "contentcloud.agent-assisted-memory/1.0"
)

// MemoryRecord is an explicit, source-bound candidate produced by a host or a
// user. It remains a candidate and never becomes a formal knowledge object by
// itself.
type MemoryRecord struct {
	SchemaVersion string      `json:"schema_version"`
	MemoryID      string      `json:"memory_id"`
	Kind          string      `json:"kind"`
	ClaimKey      string      `json:"claim_key,omitempty"`
	Scope         MemoryScope `json:"scope"`
	SourceRef     string      `json:"source_ref"`
	SourceDigest  string      `json:"source_digest"`
	SourceMode    uint32      `json:"source_mode"`
	Summary       string      `json:"summary"`
	Trust         string      `json:"trust"`
	Status        string      `json:"status"`
	FormedBy      string      `json:"formed_by"`
	ObservedAt    time.Time   `json:"observed_at"`
	RecordDigest  string      `json:"record_digest"`
}

type MemoryRememberOptions struct {
	Root      string
	MemoryID  string
	Kind      string
	ClaimKey  string
	SourceRef string
	Summary   string
	FormedBy  string
	Now       time.Time
}

type MemoryRememberReport struct {
	SchemaVersion string       `json:"schema_version"`
	Record        MemoryRecord `json:"record"`
	RecordRef     string       `json:"record_ref"`
	AlreadyExists bool         `json:"already_exists"`
}

type memoryRecordFile struct {
	Record      MemoryRecord
	FileRef     string
	FileDigest  string
	Mode        uint32
	DuplicateOf string
}

func RememberMemory(options MemoryRememberOptions) (MemoryRememberReport, error) {
	root, scope, err := resolveMemoryScope(options.Root)
	if err != nil {
		return MemoryRememberReport{}, err
	}
	kinds, err := normalizeMemoryKinds([]string{options.Kind})
	if err != nil {
		return MemoryRememberReport{}, err
	}
	if len(kinds) != 1 {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_KIND_REQUIRED", "记忆候选必须指定 kind")
	}
	sourceRef, sourceDigest, sourceMode, err := resolveMemoryRecordSource(root, options.SourceRef)
	if err != nil {
		return MemoryRememberReport{}, err
	}
	summary := strings.TrimSpace(options.Summary)
	if summary == "" {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_SUMMARY_REQUIRED", "记忆候选必须包含非空 summary")
	}
	if len([]rune(summary)) > memoryRecordSummaryRunes {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_SUMMARY_TOO_LARGE", fmt.Sprintf("记忆候选 summary 不能超过 %d 个字符", memoryRecordSummaryRunes))
	}
	formedBy := strings.TrimSpace(options.FormedBy)
	if formedBy == "" {
		formedBy = memoryRecordDefaultFormedBy
	}
	if len([]rune(formedBy)) > 160 {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_FORMED_BY_INVALID", "formed_by 不能超过 160 个字符")
	}
	claimKey := normalizeMemoryClaimKey(options.ClaimKey)
	if len([]rune(claimKey)) > 256 {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_CLAIM_KEY_INVALID", "claim_key 不能超过 256 个字符")
	}
	now := normalizedMemoryTime(options.Now)
	memoryID := strings.TrimSpace(options.MemoryID)
	if memoryID == "" {
		memoryID = "memr_" + strings.TrimPrefix(memoryDigest([]byte(scope.WorkspaceID+"\n"+scope.ProjectID+"\n"+claimKey+"\n"+sourceRef+"\n"+summary)), "sha256:")[:24]
	}
	if memoryID != localSafeName(memoryID) || memoryID == "." || memoryID == ".." || strings.ContainsAny(memoryID, `/\\`) {
		return MemoryRememberReport{}, fault.Invalid("MEMORY_ID_INVALID", "记忆候选 ID 只能包含字母、数字、点、短横线和下划线")
	}
	record := MemoryRecord{
		SchemaVersion: MemoryRecordSchema,
		MemoryID:      memoryID,
		Kind:          kinds[0],
		ClaimKey:      claimKey,
		Scope:         scope,
		SourceRef:     sourceRef,
		SourceDigest:  sourceDigest,
		SourceMode:    sourceMode,
		Summary:       summary,
		Trust:         "memory_candidate",
		Status:        "active",
		FormedBy:      formedBy,
		ObservedAt:    now,
	}
	record.RecordDigest = memoryRecordDigest(record)
	path := filepath.Join(root, filepath.FromSlash(memoryRecordRelativeDir), memoryID+".json")
	if existing, readErr := readMemoryRecord(path); readErr == nil {
		if existing.RecordDigest == record.RecordDigest && existing.MemoryID == record.MemoryID {
			return MemoryRememberReport{SchemaVersion: MemoryRecordSchema, Record: existing, RecordRef: relativeWorkspacePath(root, path), AlreadyExists: true}, nil
		}
		return MemoryRememberReport{}, fault.Conflict("MEMORY_RECORD_IMMUTABLE_CONFLICT", "相同记忆 ID 已存在不同内容："+memoryID)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return MemoryRememberReport{}, readErr
	}
	if err := replaceJSON(path, record, 0o600); err != nil {
		return MemoryRememberReport{}, err
	}
	return MemoryRememberReport{SchemaVersion: MemoryRecordSchema, Record: record, RecordRef: relativeWorkspacePath(root, path)}, nil
}

func resolveMemoryRecordSource(root, value string) (string, string, uint32, error) {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(value))))
	if clean == "." || clean == "" || filepath.IsAbs(value) || strings.HasPrefix(clean, "../") || !isMemorySourceRefAllowed(clean) {
		return "", "", 0, fault.Invalid("MEMORY_SOURCE_REF_INVALID", "source_ref 必须是允许记忆来源目录内的相对路径")
	}
	path, err := resolveWorkspaceFile(root, clean)
	if err != nil {
		return "", "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", 0, err
	}
	if info.Size() <= 0 || info.Size() > memoryMaxFileBytes || !memoryTextExtensions[strings.ToLower(filepath.Ext(path))] {
		return "", "", 0, fault.Policy("MEMORY_SOURCE_NOT_INDEXED", "source_ref 不是可安全读取的记忆文本来源", "选择允许目录中的 UTF-8 文本文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", "", 0, err
	}
	body, readErr := io.ReadAll(io.LimitReader(file, memoryMaxFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", "", 0, readErr
	}
	if closeErr != nil {
		return "", "", 0, closeErr
	}
	if len(body) > memoryMaxFileBytes || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return "", "", 0, fault.Policy("MEMORY_SOURCE_NOT_INDEXED", "source_ref 不是可安全读取的 UTF-8 文本来源", "选择允许目录中的 UTF-8 文本文件")
	}
	return relativeWorkspacePath(root, path), memoryDigest(body), uint32(info.Mode().Perm()), nil
}

func isMemorySourceRefAllowed(ref string) bool {
	for _, root := range memorySourceRoots {
		prefix := strings.TrimSuffix(filepath.ToSlash(root), "/") + "/"
		if strings.HasPrefix(ref, prefix) && !strings.HasPrefix(ref, "40-work/memory/") {
			return true
		}
	}
	return false
}

func readMemoryRecord(path string) (MemoryRecord, error) {
	var record MemoryRecord
	if err := readStrictJSON(path, &record); err != nil {
		return MemoryRecord{}, err
	}
	return record, nil
}

func memoryRecordDigest(record MemoryRecord) string {
	withoutDigest := record
	withoutDigest.RecordDigest = ""
	body, _ := jsonMarshalMemoryRecord(withoutDigest)
	return memoryDigest(body)
}

func jsonMarshalMemoryRecord(record MemoryRecord) ([]byte, error) {
	return json.Marshal(record)
}
