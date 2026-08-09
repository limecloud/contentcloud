package localworkspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	MemoryProjectionSchema = "contentcloud.memory-projection/1.0"
	MemoryEntrySchema      = "contentcloud.memory-entry/1.0"
	MemoryFormedBy         = "contentcloud.local-memory/1.0"

	MemoryStateReady        = "ready"
	MemoryStateMissing      = "missing"
	MemoryStateStale        = "stale"
	MemoryStateCorrupt      = "corrupt"
	MemoryStateIncompatible = "incompatible"

	MemoryBackendSQLiteFTS5 = "sqlite_fts5"
	MemoryBackendScan       = "scan_fallback"

	defaultMemoryLimit    = 6
	maximumMemoryLimit    = 20
	defaultMemoryMaxChars = 2400
	maximumMemoryMaxChars = 12000
)

var validMemoryKinds = map[string]bool{
	"working":     true,
	"execution":   true,
	"knowledge":   true,
	"interaction": true,
}

var validMemoryStatuses = map[string]bool{
	"active": true, "stale": true, "conflicted": true, "tombstoned": true,
}

type MemoryScope struct {
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id"`
}

type MemoryCandidate struct {
	SchemaVersion string      `json:"schema_version"`
	MemoryID      string      `json:"memory_id"`
	Kind          string      `json:"kind"`
	Scope         MemoryScope `json:"scope"`
	SourceRef     string      `json:"source_ref"`
	SourceDigest  string      `json:"source_digest"`
	Summary       string      `json:"summary"`
	MatchExcerpt  string      `json:"match_excerpt,omitempty"`
	Trust         string      `json:"trust"`
	Status        string      `json:"status"`
	FormedBy      string      `json:"formed_by"`
	ObservedAt    time.Time   `json:"observed_at"`
	Rank          int         `json:"rank"`
}

type MemoryProjectionStatus struct {
	SchemaVersion  string      `json:"schema_version"`
	Scope          MemoryScope `json:"scope"`
	State          string      `json:"state"`
	Backend        string      `json:"backend"`
	ProjectionRef  string      `json:"projection_ref"`
	CorpusDigest   string      `json:"corpus_digest"`
	BuiltAt        *time.Time  `json:"built_at,omitempty"`
	SourceCount    int         `json:"source_count"`
	EntryCount     int         `json:"entry_count"`
	DuplicateCount int         `json:"duplicate_count"`
	ConflictCount  int         `json:"conflict_count"`
	SkippedCount   int         `json:"skipped_count"`
	Warnings       []string    `json:"warnings"`
	CheckedAt      time.Time   `json:"checked_at"`
}

type MemoryQueryOptions struct {
	Root     string
	Query    string
	Kinds    []string
	Limit    int
	MaxChars int
	Now      time.Time
}

type MemoryQueryResult struct {
	SchemaVersion  string            `json:"schema_version"`
	Scope          MemoryScope       `json:"scope"`
	Query          string            `json:"query,omitempty"`
	Kinds          []string          `json:"kinds,omitempty"`
	Backend        string            `json:"backend"`
	IndexState     string            `json:"index_state"`
	SourceCount    int               `json:"source_count"`
	EntryCount     int               `json:"entry_count"`
	DuplicateCount int               `json:"duplicate_count"`
	ConflictCount  int               `json:"conflict_count"`
	Limit          int               `json:"limit"`
	MaxChars       int               `json:"max_chars"`
	UsedChars      int               `json:"used_chars"`
	Truncated      bool              `json:"truncated"`
	Candidates     []MemoryCandidate `json:"candidates"`
	Warnings       []string          `json:"warnings"`
	GeneratedAt    time.Time         `json:"generated_at"`
}

type MemoryRebuildReport struct {
	SchemaVersion  string      `json:"schema_version"`
	Scope          MemoryScope `json:"scope"`
	State          string      `json:"state"`
	Backend        string      `json:"backend"`
	ProjectionRef  string      `json:"projection_ref"`
	CorpusDigest   string      `json:"corpus_digest"`
	SourceCount    int         `json:"source_count"`
	EntryCount     int         `json:"entry_count"`
	DuplicateCount int         `json:"duplicate_count"`
	ConflictCount  int         `json:"conflict_count"`
	SkippedCount   int         `json:"skipped_count"`
	Warnings       []string    `json:"warnings"`
	BuiltAt        time.Time   `json:"built_at"`
}

type MemoryClearReport struct {
	SchemaVersion string      `json:"schema_version"`
	Scope         MemoryScope `json:"scope"`
	ProjectionRef string      `json:"projection_ref"`
	Cleared       bool        `json:"cleared"`
	AlreadyClear  bool        `json:"already_clear"`
}

type WorkspaceMemoryContext struct {
	State          string            `json:"state"`
	Backend        string            `json:"backend"`
	CandidateCount int               `json:"candidate_count"`
	Candidates     []MemoryCandidate `json:"candidates"`
	Warnings       []string          `json:"warnings"`
}

func QueryMemory(options MemoryQueryOptions) (MemoryQueryResult, error) {
	now := normalizedMemoryTime(options.Now)
	root, scope, err := resolveMemoryScope(options.Root)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	kinds, err := normalizeMemoryKinds(options.Kinds)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	limit, maxChars, err := normalizeMemoryBudget(options.Limit, options.MaxChars)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	catalog, err := scanMemoryCatalog(root, scope)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	entries := memoryEntries(scope, catalog)
	status := inspectMemoryIndex(memoryIndexPath(root), scope, catalog.Digest, now)
	warnings := append([]string{}, catalog.Warnings...)
	warnings = append(warnings, status.Warnings...)
	query := strings.TrimSpace(options.Query)

	var candidates []MemoryCandidate
	backend := MemoryBackendScan
	if status.State == MemoryStateReady && supportsFTSQuery(query) {
		candidates, err = queryMemoryIndex(memoryIndexPath(root), scope, query, kinds, limit)
		if err == nil {
			backend = MemoryBackendSQLiteFTS5
		} else {
			warnings = append(warnings, "SQLite FTS 查询失败，已改用当前工作区文件扫描")
		}
	}
	if backend == MemoryBackendScan {
		candidates = queryMemoryEntries(entries, query, kinds)
	}

	selected, usedChars, truncated := applyMemoryBudget(candidates, limit, maxChars)
	return MemoryQueryResult{
		SchemaVersion:  MemoryProjectionSchema,
		Scope:          scope,
		Query:          query,
		Kinds:          kinds,
		Backend:        backend,
		IndexState:     status.State,
		SourceCount:    len(catalog.Sources),
		EntryCount:     len(entries),
		DuplicateCount: catalog.DuplicateCount,
		ConflictCount:  catalog.ConflictCount,
		Limit:          limit,
		MaxChars:       maxChars,
		UsedChars:      usedChars,
		Truncated:      truncated,
		Candidates:     selected,
		Warnings:       uniqueSortedStrings(warnings),
		GeneratedAt:    now,
	}, nil
}

func MemoryStatus(root string, now time.Time) (MemoryProjectionStatus, error) {
	checkedAt := normalizedMemoryTime(now)
	resolved, scope, err := resolveMemoryScope(root)
	if err != nil {
		return MemoryProjectionStatus{}, err
	}
	catalog, err := scanMemoryCatalog(resolved, scope)
	if err != nil {
		return MemoryProjectionStatus{}, err
	}
	status := inspectMemoryIndex(memoryIndexPath(resolved), scope, catalog.Digest, checkedAt)
	status.SourceCount = len(catalog.Sources)
	status.DuplicateCount = catalog.DuplicateCount
	status.ConflictCount = catalog.ConflictCount
	status.SkippedCount = catalog.Skipped
	status.Warnings = uniqueSortedStrings(append(status.Warnings, catalog.Warnings...))
	return status, nil
}

func RebuildMemory(root string, now time.Time) (MemoryRebuildReport, error) {
	builtAt := normalizedMemoryTime(now)
	resolved, scope, err := resolveMemoryScope(root)
	if err != nil {
		return MemoryRebuildReport{}, err
	}
	catalog, err := scanMemoryCatalog(resolved, scope)
	if err != nil {
		return MemoryRebuildReport{}, err
	}
	entries := memoryEntries(scope, catalog)
	release, err := acquireMemoryLock(resolved, builtAt)
	if err != nil {
		return MemoryRebuildReport{}, err
	}
	defer release()
	if err := buildMemoryIndex(memoryIndexPath(resolved), scope, catalog, entries, builtAt); err != nil {
		return MemoryRebuildReport{}, err
	}
	return MemoryRebuildReport{
		SchemaVersion:  MemoryProjectionSchema,
		Scope:          scope,
		State:          MemoryStateReady,
		Backend:        MemoryBackendSQLiteFTS5,
		ProjectionRef:  memoryIndexRelativePath,
		CorpusDigest:   catalog.Digest,
		SourceCount:    len(catalog.Sources),
		EntryCount:     len(entries),
		DuplicateCount: catalog.DuplicateCount,
		ConflictCount:  catalog.ConflictCount,
		SkippedCount:   catalog.Skipped,
		Warnings:       uniqueSortedStrings(catalog.Warnings),
		BuiltAt:        builtAt,
	}, nil
}

func ClearMemory(root string, now time.Time) (MemoryClearReport, error) {
	resolved, scope, err := resolveMemoryScope(root)
	if err != nil {
		return MemoryClearReport{}, err
	}
	release, err := acquireMemoryLock(resolved, normalizedMemoryTime(now))
	if err != nil {
		return MemoryClearReport{}, err
	}
	defer release()
	directory := filepath.Dir(memoryIndexPath(resolved))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return MemoryClearReport{SchemaVersion: MemoryProjectionSchema, Scope: scope, ProjectionRef: memoryIndexRelativePath, AlreadyClear: true}, nil
	}
	if err != nil {
		return MemoryClearReport{}, err
	}
	removed := false
	baseName := filepath.Base(memoryIndexRelativePath)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isMemoryIndexArtifact(name, baseName) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return MemoryClearReport{}, err
		}
		removed = true
	}
	return MemoryClearReport{SchemaVersion: MemoryProjectionSchema, Scope: scope, ProjectionRef: memoryIndexRelativePath, Cleared: removed, AlreadyClear: !removed}, nil
}

func isMemoryIndexArtifact(name, baseName string) bool {
	if name == baseName || strings.HasPrefix(name, baseName+".") {
		return true
	}
	return isMemoryIndexSidecar(name, baseName)
}

func WorkspaceMemory(root string, now time.Time) WorkspaceMemoryContext {
	result, err := QueryMemory(MemoryQueryOptions{Root: root, Limit: 4, MaxChars: 1400, Now: now})
	if err != nil {
		return WorkspaceMemoryContext{State: "unavailable", Backend: MemoryBackendScan, Candidates: []MemoryCandidate{}, Warnings: []string{err.Error()}}
	}
	return WorkspaceMemoryContext{
		State:          result.IndexState,
		Backend:        result.Backend,
		CandidateCount: len(result.Candidates),
		Candidates:     result.Candidates,
		Warnings:       result.Warnings,
	}
}

func resolveMemoryScope(root string) (string, MemoryScope, error) {
	resolved, err := FindRoot(root)
	if err != nil {
		return "", MemoryScope{}, err
	}
	status, err := LoadStatus(resolved)
	if err != nil {
		return "", MemoryScope{}, err
	}
	scope := MemoryScope{WorkspaceID: strings.TrimSpace(status.Binding.WorkspaceID), ProjectID: strings.TrimSpace(status.Binding.ProjectID)}
	if scope.WorkspaceID == "" || scope.ProjectID == "" {
		return "", MemoryScope{}, domain.Invalid("MEMORY_SCOPE_INVALID", "记忆范围必须从已绑定工作区和项目推导")
	}
	return resolved, scope, nil
}

func normalizeMemoryKinds(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		kind := strings.ToLower(strings.TrimSpace(value))
		if kind == "" || seen[kind] {
			continue
		}
		if !validMemoryKinds[kind] {
			return nil, domain.Invalid("MEMORY_KIND_INVALID", "记忆类型必须为 working、execution、knowledge 或 interaction")
		}
		seen[kind] = true
		result = append(result, kind)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeMemoryBudget(limit, maxChars int) (int, int, error) {
	if limit < 0 || limit > maximumMemoryLimit {
		return 0, 0, domain.Invalid("MEMORY_LIMIT_INVALID", fmt.Sprintf("记忆召回条数必须在 1 到 %d 之间", maximumMemoryLimit))
	}
	if maxChars < 0 || maxChars > maximumMemoryMaxChars {
		return 0, 0, domain.Invalid("MEMORY_BUDGET_INVALID", fmt.Sprintf("记忆召回字符预算必须在 1 到 %d 之间", maximumMemoryMaxChars))
	}
	if limit == 0 {
		limit = defaultMemoryLimit
	}
	if maxChars == 0 {
		maxChars = defaultMemoryMaxChars
	}
	return limit, maxChars, nil
}

func normalizedMemoryTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func applyMemoryBudget(candidates []MemoryCandidate, limit, maxChars int) ([]MemoryCandidate, int, bool) {
	selected := make([]MemoryCandidate, 0, min(limit, len(candidates)))
	used := 0
	truncated := false
	for _, candidate := range candidates {
		if len(selected) >= limit {
			truncated = true
			break
		}
		cost := len([]rune(candidate.Summary)) + len([]rune(candidate.MatchExcerpt))
		if used+cost > maxChars {
			truncated = true
			continue
		}
		candidate.Rank = len(selected) + 1
		selected = append(selected, candidate)
		used += cost
	}
	return selected, used, truncated
}
