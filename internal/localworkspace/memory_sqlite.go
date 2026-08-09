package localworkspace

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	_ "modernc.org/sqlite"
)

func memoryIndexPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(memoryIndexRelativePath))
}

func inspectMemoryIndex(path string, scope MemoryScope, corpusDigest string, checkedAt time.Time) MemoryProjectionStatus {
	status := MemoryProjectionStatus{
		SchemaVersion: MemoryProjectionSchema,
		Scope:         scope,
		State:         MemoryStateMissing,
		Backend:       MemoryBackendSQLiteFTS5,
		ProjectionRef: memoryIndexRelativePath,
		CorpusDigest:  corpusDigest,
		Warnings:      []string{},
		CheckedAt:     checkedAt,
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return status
	} else if err != nil {
		status.State = MemoryStateCorrupt
		status.Warnings = append(status.Warnings, "无法读取本地记忆索引状态")
		return status
	}
	db, err := openMemoryDB(path, true)
	if err != nil {
		status.State = MemoryStateCorrupt
		status.Warnings = append(status.Warnings, "本地记忆索引无法打开，已降级为文件扫描")
		return status
	}
	defer db.Close()
	meta, err := memoryMeta(db)
	if err != nil {
		status.State = MemoryStateCorrupt
		status.Warnings = append(status.Warnings, "本地记忆索引元数据损坏，已降级为文件扫描")
		return status
	}
	if meta["schema_version"] != MemoryProjectionSchema || meta["workspace_id"] != scope.WorkspaceID || meta["project_id"] != scope.ProjectID {
		status.State = MemoryStateIncompatible
		status.Warnings = append(status.Warnings, "本地记忆索引与当前工作区或项目不兼容，已降级为文件扫描")
		return status
	}
	if meta["root_digest"] == "" || meta["root_digest"] != memoryRootDigest(memoryRootFromIndexPath(path)) {
		status.State = MemoryStateIncompatible
		status.Warnings = append(status.Warnings, "本地记忆索引不是由当前工作区生成，已降级为文件扫描")
		return status
	}
	status.BuiltAt = parseMemoryTime(meta["built_at"])
	status.SourceCount = parseMemoryInt(meta["source_count"])
	status.EntryCount = parseMemoryInt(meta["entry_count"])
	status.DuplicateCount = parseMemoryInt(meta["duplicate_count"])
	status.ConflictCount = parseMemoryInt(meta["conflict_count"])
	if meta["corpus_digest"] != corpusDigest {
		status.State = MemoryStateStale
		status.Warnings = append(status.Warnings, "工作区来源已变化，本次召回未使用旧记忆索引")
		return status
	}
	status.State = MemoryStateReady
	return status
}

func queryMemoryIndex(path string, scope MemoryScope, query string, kinds []string, limit int) ([]MemoryCandidate, error) {
	db, err := openMemoryDB(path, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	where := []string{"e.workspace_id = ?", "e.project_id = ?", "e.status = 'active'"}
	args := []any{scope.WorkspaceID, scope.ProjectID}
	allowed := memoryKindSet(kinds)
	if len(allowed) > 0 {
		values := make([]string, 0, len(allowed))
		for kind := range allowed {
			values = append(values, kind)
		}
		// normalizeMemoryKinds sorted the public value; sorting again keeps SQL and
		// test output deterministic even when this helper is called internally.
		sortStrings(values)
		placeholders := make([]string, 0, len(values))
		for _, value := range values {
			placeholders = append(placeholders, "?")
			args = append(args, value)
		}
		where = append(where, "e.kind IN ("+strings.Join(placeholders, ",")+")")
	}
	var rows *sql.Rows
	if strings.TrimSpace(query) == "" {
		statement := `SELECT e.memory_id,e.kind,e.source_ref,e.source_digest,e.summary,e.trust,e.status,e.formed_by,e.observed_at
			FROM memory_entries e WHERE ` + strings.Join(where, " AND ") + `
			ORDER BY e.priority ASC,e.observed_at DESC,e.source_ref ASC,e.chunk_no ASC LIMIT ?`
		args = append(args, limit)
		rows, err = db.Query(statement, args...)
	} else {
		where = append(where, "memory_fts MATCH ?")
		args = append(args, memoryFTSQuery(query))
		statement := `SELECT e.memory_id,e.kind,e.source_ref,e.source_digest,e.summary,
			snippet(memory_fts,1,'','', ' … ',32),e.trust,e.status,e.formed_by,e.observed_at
			FROM memory_fts JOIN memory_entries e ON e.memory_id = memory_fts.memory_id
			WHERE ` + strings.Join(where, " AND ") + `
			ORDER BY bm25(memory_fts) ASC,e.priority ASC,e.source_ref ASC,e.chunk_no ASC LIMIT ?`
		args = append(args, limit)
		rows, err = db.Query(statement, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MemoryCandidate, 0, limit)
	for rows.Next() {
		var candidate MemoryCandidate
		var observedAt string
		if strings.TrimSpace(query) == "" {
			err = rows.Scan(&candidate.MemoryID, &candidate.Kind, &candidate.SourceRef, &candidate.SourceDigest, &candidate.Summary, &candidate.Trust, &candidate.Status, &candidate.FormedBy, &observedAt)
		} else {
			err = rows.Scan(&candidate.MemoryID, &candidate.Kind, &candidate.SourceRef, &candidate.SourceDigest, &candidate.Summary, &candidate.MatchExcerpt, &candidate.Trust, &candidate.Status, &candidate.FormedBy, &observedAt)
		}
		if err != nil {
			return nil, err
		}
		candidate.SchemaVersion = MemoryEntrySchema
		candidate.Scope = scope
		if parsed := parseMemoryTime(observedAt); parsed != nil {
			candidate.ObservedAt = *parsed
		}
		result = append(result, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func buildMemoryIndex(path string, scope MemoryScope, catalog memoryCatalog, entries []memoryEntry, builtAt time.Time) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	defer os.Remove(temporaryPath)
	_ = os.Chmod(temporaryPath, 0o600)
	db, err := openMemoryDB(temporaryPath, false)
	if err != nil {
		return err
	}
	closeDB := func() error { return db.Close() }
	defer func() { _ = closeDB() }()
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON;`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE TABLE projection_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE memory_entries (
			memory_id TEXT PRIMARY KEY,
			schema_version TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			source_ref TEXT NOT NULL,
			source_digest TEXT NOT NULL,
			summary TEXT NOT NULL,
			content TEXT NOT NULL,
			trust TEXT NOT NULL,
			status TEXT NOT NULL,
			formed_by TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			chunk_no INTEGER NOT NULL,
			priority INTEGER NOT NULL
		);
		CREATE INDEX memory_entries_scope ON memory_entries(workspace_id, project_id, kind, status);
		CREATE VIRTUAL TABLE memory_fts USING fts5(memory_id UNINDEXED, content, tokenize='trigram');
	`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	meta := map[string]string{
		"schema_version":  MemoryProjectionSchema,
		"workspace_id":    scope.WorkspaceID,
		"project_id":      scope.ProjectID,
		"root_digest":     memoryRootDigest(memoryRootFromIndexPath(path)),
		"corpus_digest":   catalog.Digest,
		"built_at":        builtAt.Format(time.RFC3339Nano),
		"source_count":    strconv.Itoa(len(catalog.Sources)),
		"entry_count":     strconv.Itoa(len(entries)),
		"duplicate_count": strconv.Itoa(catalog.DuplicateCount),
		"conflict_count":  strconv.Itoa(catalog.ConflictCount),
	}
	for key, value := range meta {
		if _, err := tx.Exec("INSERT INTO projection_meta(key,value) VALUES(?,?)", key, value); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if _, err := tx.Exec(`INSERT INTO memory_entries(memory_id,schema_version,workspace_id,project_id,kind,source_ref,source_digest,summary,content,trust,status,formed_by,observed_at,chunk_no,priority) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, entry.MemoryID, entry.SchemaVersion, entry.Scope.WorkspaceID, entry.Scope.ProjectID, entry.Kind, entry.SourceRef, entry.SourceDigest, entry.Summary, entry.Content, entry.Trust, entry.Status, entry.FormedBy, entry.ObservedAt.Format(time.RFC3339Nano), entry.ChunkNo, entry.Priority); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO memory_fts(memory_id,content) VALUES(?,?)", entry.MemoryID, entry.Content); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	if _, err := db.Exec("PRAGMA optimize"); err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	closeDB = func() error { return nil }
	return replaceMemoryIndex(temporaryPath, path)
}

func memoryRootFromIndexPath(path string) string {
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Clean(path)))))
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return filepath.Clean(root)
}

func memoryRootDigest(root string) string {
	return memoryDigest([]byte(root))
}

func replaceMemoryIndex(temporaryPath, path string) error {
	backupPath := path + ".backup"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := removeMemoryIndexSidecars(path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func removeMemoryIndexSidecars(path string) error {
	directory := filepath.Dir(path)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	baseName := filepath.Base(path)
	for _, entry := range entries {
		if entry.IsDir() || !isMemoryIndexSidecar(entry.Name(), baseName) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func isMemoryIndexSidecar(name, baseName string) bool {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if name == baseName+suffix {
			return true
		}
	}
	return false
}

func openMemoryDB(path string, readOnly bool) (*sql.DB, error) {
	dsn := path
	if readOnly {
		dsn += "?mode=ro"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA query_only=" + boolSQLite(readOnly)); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func memoryMeta(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key,value FROM projection_meta")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meta := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		meta[key] = value
	}
	return meta, rows.Err()
}

func memoryFTSQuery(query string) string {
	terms := memorySearchTerms(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

func parseMemoryTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func parseMemoryInt(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func boolSQLite(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func acquireMemoryLock(root string, now time.Time) (func(), error) {
	path := filepath.Join(root, filepath.FromSlash(memoryLockRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "pid=%d\ncreated_at=%s\n", os.Getpid(), now.Format(time.RFC3339Nano))
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, statErr := os.Stat(path)
		if statErr == nil && now.Sub(info.ModTime()) > 15*time.Minute {
			if removeErr := os.Remove(path); removeErr == nil {
				continue
			}
		}
		conflict := domain.Conflict("MEMORY_REBUILD_BUSY", "本地记忆索引正在重建，请稍后重试")
		conflict.Retryable = true
		return nil, conflict
	}
	conflict := domain.Conflict("MEMORY_REBUILD_BUSY", "本地记忆索引正在重建，请稍后重试")
	conflict.Retryable = true
	return nil, conflict
}
