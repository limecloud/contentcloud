package localworkspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	memoryIndexRelativePath = ".contentcloud/cache/memory/index.sqlite3"
	memoryLockRelativePath  = ".contentcloud/locks/memory-rebuild.lock"
	memoryMaxFileBytes      = 1024 * 1024
	memoryMaxSourceFiles    = 10000
	memoryMaxEntries        = 20000
	memoryChunkRunes        = 1200
	memorySummaryRunes      = 360
	memoryExcerptRunes      = 480
)

var memorySourceRoots = []string{
	"10-context",
	"20-sources/extracts",
	"30-knowledge/pages",
	"30-knowledge/packs",
	"40-work",
	"50-production",
	"70-results",
}

var memoryTextExtensions = map[string]bool{
	".csv": true, ".json": true, ".jsonl": true, ".markdown": true, ".md": true,
	".srt": true, ".tsv": true, ".txt": true, ".yaml": true, ".yml": true,
}

type memorySource struct {
	Ref      string
	Kind     string
	Digest   string
	Mode     uint32
	ModTime  time.Time
	Contents string
}

type memoryCatalog struct {
	Sources        []memorySource
	Records        []memoryRecordFile
	RecordDigests  []memoryRecordFingerprint
	DuplicateCount int
	ConflictCount  int
	Digest         string
	Skipped        int
	Warnings       []string
}

type memoryRecordFingerprint struct {
	Ref  string
	Hash string
	Mode uint32
}

type memoryEntry struct {
	MemoryID      string
	SchemaVersion string
	Kind          string
	Scope         MemoryScope
	SourceRef     string
	SourceDigest  string
	Summary       string
	Content       string
	Trust         string
	Status        string
	FormedBy      string
	ObservedAt    time.Time
	ChunkNo       int
	Priority      int
}

func scanMemoryCatalog(root string, scope MemoryScope) (memoryCatalog, error) {
	catalog := memoryCatalog{Sources: []memorySource{}, Warnings: []string{}}
	for _, relativeRoot := range memorySourceRoots {
		path := filepath.Join(root, filepath.FromSlash(relativeRoot))
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			catalog.Skipped++
			catalog.Warnings = append(catalog.Warnings, "无法读取记忆来源目录："+relativeRoot)
			continue
		}
		err := filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				catalog.Skipped++
				catalog.Warnings = append(catalog.Warnings, "无法读取记忆来源："+memoryRelativePath(root, path))
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry == nil {
				return nil
			}
			if entry.IsDir() {
				if memoryRelativePath(root, path) == "40-work/memory" {
					return filepath.SkipDir
				}
				if strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if len(catalog.Sources) >= memoryMaxSourceFiles {
				catalog.Skipped++
				return filepath.SkipAll
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				catalog.Skipped++
				return nil
			}
			if !memoryTextExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				catalog.Skipped++
				catalog.Warnings = append(catalog.Warnings, "无法读取记忆来源元数据："+memoryRelativePath(root, path))
				return nil
			}
			if info.Size() <= 0 || info.Size() > memoryMaxFileBytes {
				catalog.Skipped++
				if info.Size() > memoryMaxFileBytes {
					catalog.Warnings = append(catalog.Warnings, "记忆来源超过 1 MiB，已跳过："+memoryRelativePath(root, path))
				}
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				catalog.Skipped++
				catalog.Warnings = append(catalog.Warnings, "无法读取记忆来源内容："+memoryRelativePath(root, path))
				return nil
			}
			contents, readErr := io.ReadAll(io.LimitReader(file, memoryMaxFileBytes+1))
			closeErr := file.Close()
			if readErr != nil || closeErr != nil || len(contents) > memoryMaxFileBytes || !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
				catalog.Skipped++
				catalog.Warnings = append(catalog.Warnings, "记忆来源不是可安全读取的 UTF-8 文本，已跳过："+memoryRelativePath(root, path))
				return nil
			}
			text := strings.TrimSpace(strings.ReplaceAll(string(contents), "\r\n", "\n"))
			if text == "" {
				return nil
			}
			catalog.Sources = append(catalog.Sources, memorySource{
				Ref:      memoryRelativePath(root, path),
				Kind:     memoryKindForRef(memoryRelativePath(root, path)),
				Digest:   memoryDigest(contents),
				Mode:     uint32(info.Mode().Perm()),
				ModTime:  info.ModTime().UTC(),
				Contents: text,
			})
			return nil
		})
		if errors.Is(err, filepath.SkipAll) {
			catalog.Warnings = append(catalog.Warnings, "记忆来源数量超过 10000 个，后续来源已跳过")
			break
		}
		if err != nil {
			return memoryCatalog{}, err
		}
	}
	records, recordDigests, skipped, recordWarnings, err := scanMemoryRecords(root, scope, catalog.Sources)
	if err != nil {
		return memoryCatalog{}, err
	}
	catalog.Records = records
	catalog.RecordDigests = recordDigests
	_, _, duplicateIDs, conflictIDs := consolidateMemoryRecords(catalog.Records)
	catalog.DuplicateCount = len(duplicateIDs)
	catalog.ConflictCount = len(conflictIDs)
	if catalog.DuplicateCount > 0 {
		catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("记忆候选存在 %d 条重复记录，重建时已保留确定性 canonical 记录", catalog.DuplicateCount))
	}
	if catalog.ConflictCount > 0 {
		catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("记忆候选存在 %d 条冲突记录，已阻断默认召回", catalog.ConflictCount))
	}
	for index := range catalog.Records {
		memoryID := catalog.Records[index].Record.MemoryID
		if conflictIDs[memoryID] {
			catalog.Records[index].Record.Status = "conflicted"
		}
		if duplicateIDs[memoryID] {
			catalog.Records[index].DuplicateOf = "duplicate"
		}
	}
	catalog.Skipped += skipped
	catalog.Warnings = append(catalog.Warnings, recordWarnings...)
	sort.Slice(catalog.Sources, func(i, j int) bool { return catalog.Sources[i].Ref < catalog.Sources[j].Ref })
	sort.Slice(catalog.RecordDigests, func(i, j int) bool { return catalog.RecordDigests[i].Ref < catalog.RecordDigests[j].Ref })
	var corpus strings.Builder
	corpus.WriteString(scope.WorkspaceID)
	corpus.WriteByte('\n')
	corpus.WriteString(scope.ProjectID)
	for _, source := range catalog.Sources {
		corpus.WriteByte('\n')
		corpus.WriteString(source.Ref)
		corpus.WriteByte('\t')
		corpus.WriteString(source.Digest)
		corpus.WriteByte('\t')
		corpus.WriteString(strconv.FormatUint(uint64(source.Mode), 8))
	}
	for _, record := range catalog.RecordDigests {
		corpus.WriteString("\nrecord\t")
		corpus.WriteString(record.Ref)
		corpus.WriteByte('\t')
		corpus.WriteString(record.Hash)
		corpus.WriteByte('\t')
		corpus.WriteString(strconv.FormatUint(uint64(record.Mode), 8))
	}
	catalog.Digest = memoryDigest([]byte(corpus.String()))
	catalog.Warnings = uniqueSortedStrings(catalog.Warnings)
	return catalog, nil
}

func memoryEntries(scope MemoryScope, catalog memoryCatalog) []memoryEntry {
	entries := make([]memoryEntry, 0, len(catalog.Sources)+len(catalog.Records))
	reservedIDs := map[string]bool{}
	usedIDs := map[string]bool{}
	for _, record := range catalog.Records {
		reservedIDs[record.Record.MemoryID] = true
	}
	for _, source := range catalog.Sources {
		for chunkNo, content := range chunkMemoryText(source.Contents) {
			if len(entries) >= memoryMaxEntries {
				return entries
			}
			entryID := memorySourceEntryID(scope, source.Ref, chunkNo, reservedIDs, usedIDs)
			usedIDs[entryID] = true
			entries = append(entries, memoryEntry{
				MemoryID:      entryID,
				SchemaVersion: MemoryEntrySchema,
				Kind:          source.Kind,
				Scope:         scope,
				SourceRef:     source.Ref,
				SourceDigest:  source.Digest,
				Summary:       truncateRunes(compactMemoryText(content), memorySummaryRunes),
				Content:       content,
				Trust:         "memory_candidate",
				Status:        "active",
				FormedBy:      MemoryFormedBy,
				ObservedAt:    source.ModTime,
				ChunkNo:       chunkNo,
				Priority:      memoryPriority(source.Ref),
			})
		}
	}
	for _, record := range catalog.Records {
		if record.Record.Status != "active" || record.DuplicateOf != "" {
			continue
		}
		if len(entries) >= memoryMaxEntries {
			return entries
		}
		if usedIDs[record.Record.MemoryID] {
			continue
		}
		usedIDs[record.Record.MemoryID] = true
		entries = append(entries, memoryEntry{
			MemoryID:      record.Record.MemoryID,
			SchemaVersion: MemoryEntrySchema,
			Kind:          record.Record.Kind,
			Scope:         scope,
			SourceRef:     record.Record.SourceRef,
			SourceDigest:  record.Record.SourceDigest,
			Summary:       truncateRunes(compactMemoryText(record.Record.Summary), memorySummaryRunes),
			Content:       record.Record.Summary,
			Trust:         record.Record.Trust,
			Status:        record.Record.Status,
			FormedBy:      record.Record.FormedBy,
			ObservedAt:    record.Record.ObservedAt,
			Priority:      0,
		})
	}
	return entries
}

func memorySourceEntryID(scope MemoryScope, sourceRef string, chunkNo int, reservedIDs, usedIDs map[string]bool) string {
	entryID := memoryID(scope, sourceRef, chunkNo)
	if !reservedIDs[entryID] && !usedIDs[entryID] {
		return entryID
	}
	for salt := 0; ; salt++ {
		candidate := "memsrc_" + strings.TrimPrefix(memoryDigest([]byte(scope.WorkspaceID+"\n"+scope.ProjectID+"\n"+sourceRef+"\n"+strconv.Itoa(chunkNo)+"\n"+strconv.Itoa(salt))), "sha256:")[:24]
		if !reservedIDs[candidate] && !usedIDs[candidate] {
			return candidate
		}
	}
}

func scanMemoryRecords(root string, scope MemoryScope, sources []memorySource) ([]memoryRecordFile, []memoryRecordFingerprint, int, []string, error) {
	directory := filepath.Join(root, filepath.FromSlash(memoryRecordRelativeDir))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []memoryRecordFile{}, []memoryRecordFingerprint{}, 0, []string{}, nil
	}
	if err != nil {
		return nil, nil, 0, nil, err
	}
	bySource := make(map[string]memorySource, len(sources))
	for _, source := range sources {
		bySource[source.Ref] = source
	}
	records := []memoryRecordFile{}
	digests := []memoryRecordFingerprint{}
	warnings := []string{}
	skipped := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			skipped++
			warnings = append(warnings, "无法读取记忆记录元数据："+memoryRelativePath(root, path))
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			skipped++
			warnings = append(warnings, "无法读取记忆记录："+memoryRelativePath(root, path))
			continue
		}
		ref := memoryRelativePath(root, path)
		digests = append(digests, memoryRecordFingerprint{Ref: ref, Hash: memoryDigest(body), Mode: uint32(info.Mode().Perm())})
		var record MemoryRecord
		if err := strictUnmarshal(body, &record); err != nil || record.SchemaVersion != MemoryRecordSchema || record.RecordDigest == "" || memoryRecordDigest(record) != record.RecordDigest {
			skipped++
			warnings = append(warnings, "记忆记录格式或摘要无效："+ref)
			continue
		}
		if record.MemoryID == "" || record.MemoryID != strings.TrimSpace(record.MemoryID) || record.MemoryID != localSafeName(record.MemoryID) || record.Kind == "" || !validMemoryKinds[record.Kind] || record.SourceRef == "" || record.Summary == "" || record.Trust != "memory_candidate" || !validMemoryStatuses[record.Status] || record.FormedBy == "" || record.ObservedAt.IsZero() || record.SourceMode == 0 || record.Scope != scope {
			skipped++
			warnings = append(warnings, "记忆记录字段无效："+ref)
			continue
		}
		source, ok := bySource[record.SourceRef]
		if !ok || source.Digest != record.SourceDigest || source.Mode != record.SourceMode {
			record.Status = memoryRecordSourceStatus(record.SourceMode, source, ok)
			warnings = append(warnings, "记忆记录来源已变化、删除或权限收窄，已标记为 "+record.Status+"："+ref)
		}
		records = append(records, memoryRecordFile{Record: record, FileRef: ref, FileDigest: memoryDigest(body), Mode: uint32(info.Mode().Perm())})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].FileRef < records[j].FileRef })
	sort.Slice(digests, func(i, j int) bool { return digests[i].Ref < digests[j].Ref })
	return records, digests, skipped, uniqueSortedStrings(warnings), nil
}

func memoryRecordSourceStatus(previousMode uint32, source memorySource, exists bool) string {
	if !exists || memoryPermissionNarrowed(previousMode, source.Mode) {
		return "tombstoned"
	}
	return "stale"
}

func memoryPermissionNarrowed(previousMode, currentMode uint32) bool {
	previous := previousMode & 0o777
	current := currentMode & 0o777
	return previous&^current != 0
}

func queryMemoryEntries(entries []memoryEntry, query string, kinds []string) []MemoryCandidate {
	allowed := memoryKindSet(kinds)
	terms := memorySearchTerms(query)
	type scoredEntry struct {
		entry memoryEntry
		score int
	}
	scored := make([]scoredEntry, 0, len(entries))
	for _, entry := range entries {
		if len(allowed) > 0 && !allowed[entry.Kind] {
			continue
		}
		lower := strings.ToLower(entry.Content)
		score := 0
		if len(terms) > 0 {
			for _, term := range terms {
				score += strings.Count(lower, strings.ToLower(term))
			}
			if score == 0 {
				continue
			}
			if strings.Contains(lower, strings.ToLower(strings.TrimSpace(query))) {
				score += 2
			}
		}
		scored = append(scored, scoredEntry{entry: entry, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].entry.Priority != scored[j].entry.Priority {
			return scored[i].entry.Priority < scored[j].entry.Priority
		}
		if !scored[i].entry.ObservedAt.Equal(scored[j].entry.ObservedAt) {
			return scored[i].entry.ObservedAt.After(scored[j].entry.ObservedAt)
		}
		if scored[i].entry.SourceRef != scored[j].entry.SourceRef {
			return scored[i].entry.SourceRef < scored[j].entry.SourceRef
		}
		return scored[i].entry.ChunkNo < scored[j].entry.ChunkNo
	})
	result := make([]MemoryCandidate, 0, len(scored))
	for _, value := range scored {
		candidate := memoryCandidate(value.entry, "")
		candidate.MatchExcerpt = memoryExcerpt(value.entry.Content, query)
		result = append(result, candidate)
	}
	return result
}

func memoryCandidate(entry memoryEntry, excerpt string) MemoryCandidate {
	return MemoryCandidate{
		SchemaVersion: entry.SchemaVersion,
		MemoryID:      entry.MemoryID,
		Kind:          entry.Kind,
		Scope:         entry.Scope,
		SourceRef:     entry.SourceRef,
		SourceDigest:  entry.SourceDigest,
		Summary:       entry.Summary,
		MatchExcerpt:  truncateRunes(compactMemoryText(excerpt), memoryExcerptRunes),
		Trust:         entry.Trust,
		Status:        entry.Status,
		FormedBy:      entry.FormedBy,
		ObservedAt:    entry.ObservedAt,
	}
}

func chunkMemoryText(content string) []string {
	paragraphs := strings.Split(strings.TrimSpace(content), "\n\n")
	chunks := make([]string, 0, len(paragraphs))
	current := make([]rune, 0, memoryChunkRunes)
	flush := func() {
		if text := strings.TrimSpace(string(current)); text != "" {
			chunks = append(chunks, text)
		}
		current = current[:0]
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > memoryChunkRunes {
			if len(current) > 0 {
				flush()
			}
			chunks = append(chunks, string(runes[:memoryChunkRunes]))
			runes = runes[memoryChunkRunes:]
		}
		if len(current)+len(runes)+1 > memoryChunkRunes {
			flush()
		}
		if len(current) > 0 {
			current = append(current, '\n', '\n')
		}
		current = append(current, runes...)
	}
	flush()
	return chunks
}

func memorySearchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return []string{query}
	}
	return parts
}

func supportsFTSQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	for _, term := range memorySearchTerms(query) {
		if utf8.RuneCountInString(term) < 3 {
			return false
		}
	}
	return true
}

func memoryExcerpt(content, query string) string {
	content = strings.TrimSpace(content)
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	lowerContent := strings.ToLower(content)
	position := -1
	for _, term := range memorySearchTerms(query) {
		if candidate := strings.Index(lowerContent, strings.ToLower(term)); candidate >= 0 && (position < 0 || candidate < position) {
			position = candidate
		}
	}
	if position < 0 {
		return truncateRunes(content, memoryExcerptRunes)
	}
	runes := []rune(content)
	runePosition := utf8.RuneCountInString(content[:position])
	start := max(0, runePosition-80)
	end := min(len(runes), runePosition+memoryExcerptRunes-80)
	return string(runes[start:end])
}

func memoryKindForRef(ref string) string {
	switch {
	case strings.HasPrefix(ref, "40-work/handoffs/"):
		return "interaction"
	case strings.HasPrefix(ref, "40-work/runs/"), strings.HasPrefix(ref, "70-results/"):
		return "execution"
	case strings.HasPrefix(ref, "10-context/"), strings.HasPrefix(ref, "20-sources/extracts/"), strings.HasPrefix(ref, "30-knowledge/"):
		return "knowledge"
	default:
		return "working"
	}
}

func memoryPriority(ref string) int {
	switch {
	case ref == "40-work/focus.md":
		return 0
	case strings.HasPrefix(ref, "40-work/handoffs/"):
		return 1
	case strings.HasPrefix(ref, "40-work/runs/"):
		return 2
	case ref == "10-context/project-brief.yaml":
		return 3
	case strings.HasPrefix(ref, "10-context/"):
		return 4
	default:
		return 5
	}
}

func memoryKindSet(kinds []string) map[string]bool {
	set := map[string]bool{}
	for _, kind := range kinds {
		set[kind] = true
	}
	return set
}

func memoryRelativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(filepath.Base(path))
	}
	return filepath.ToSlash(relative)
}

func memoryID(scope MemoryScope, sourceRef string, chunkNo int) string {
	return "mem_" + strings.TrimPrefix(memoryDigest([]byte(scope.WorkspaceID+"\n"+scope.ProjectID+"\n"+sourceRef+"\n"+strconv.Itoa(chunkNo))), "sha256:")[:24]
}

func memoryDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func compactMemoryText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:max(0, limit-1)]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
