package localworkspace

import (
	"sort"
	"strings"
	"time"
)

const MemoryConsolidationSchema = "contentcloud.memory-consolidation/1.0"

type MemoryDuplicateGroup struct {
	ClaimKey     string   `json:"claim_key"`
	CanonicalID  string   `json:"canonical_id"`
	DuplicateIDs []string `json:"duplicate_ids"`
}

type MemoryConflictGroup struct {
	ClaimKey  string   `json:"claim_key"`
	RecordIDs []string `json:"record_ids"`
	Summaries []string `json:"summaries"`
}

type MemoryConsolidationOptions struct {
	Root string
	Now  time.Time
}

type MemoryConsolidationReport struct {
	SchemaVersion  string                 `json:"schema_version"`
	Scope          MemoryScope            `json:"scope"`
	RecordCount    int                    `json:"record_count"`
	DuplicateCount int                    `json:"duplicate_count"`
	ConflictCount  int                    `json:"conflict_count"`
	Duplicates     []MemoryDuplicateGroup `json:"duplicates"`
	Conflicts      []MemoryConflictGroup  `json:"conflicts"`
	Warnings       []string               `json:"warnings"`
	GeneratedAt    time.Time              `json:"generated_at"`
}

// ConsolidateMemory derives duplicate and conflict information from immutable
// memory records. It never rewrites a record or silently chooses a winner.
func ConsolidateMemory(options MemoryConsolidationOptions) (MemoryConsolidationReport, error) {
	root, scope, err := resolveMemoryScope(options.Root)
	if err != nil {
		return MemoryConsolidationReport{}, err
	}
	catalog, err := scanMemoryCatalog(root, scope)
	if err != nil {
		return MemoryConsolidationReport{}, err
	}
	duplicates, conflicts, duplicateIDs, conflictIDs := consolidateMemoryRecords(catalog.Records)
	return MemoryConsolidationReport{
		SchemaVersion:  MemoryConsolidationSchema,
		Scope:          scope,
		RecordCount:    len(catalog.Records),
		DuplicateCount: len(duplicateIDs),
		ConflictCount:  len(conflictIDs),
		Duplicates:     duplicates,
		Conflicts:      conflicts,
		Warnings:       catalog.Warnings,
		GeneratedAt:    normalizedMemoryTime(options.Now),
	}, nil
}

func normalizeMemoryClaimKey(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func memoryRecordClaimKey(record MemoryRecord) string {
	if key := normalizeMemoryClaimKey(record.ClaimKey); key != "" {
		return key
	}
	return "summary:" + memoryDigest([]byte(normalizeMemoryClaimKey(record.Summary)))
}

func memoryRecordValueKey(record MemoryRecord) string {
	return memoryRecordClaimKey(record) + "\x00" + normalizeMemoryClaimKey(record.Summary) + "\x00" + record.SourceDigest
}

func consolidateMemoryRecords(records []memoryRecordFile) ([]MemoryDuplicateGroup, []MemoryConflictGroup, map[string]bool, map[string]bool) {
	byClaim := map[string][]memoryRecordFile{}
	for _, record := range records {
		if record.Record.Status != "active" && record.Record.Status != "conflicted" {
			continue
		}
		byClaim[memoryRecordClaimKey(record.Record)] = append(byClaim[memoryRecordClaimKey(record.Record)], record)
	}
	keys := make([]string, 0, len(byClaim))
	for key := range byClaim {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	duplicates := []MemoryDuplicateGroup{}
	conflicts := []MemoryConflictGroup{}
	duplicateIDs := map[string]bool{}
	conflictIDs := map[string]bool{}
	for _, key := range keys {
		group := byClaim[key]
		sort.Slice(group, func(i, j int) bool { return group[i].Record.MemoryID < group[j].Record.MemoryID })
		byValue := map[string][]memoryRecordFile{}
		for _, record := range group {
			byValue[memoryRecordValueKey(record.Record)] = append(byValue[memoryRecordValueKey(record.Record)], record)
		}
		valueKeys := make([]string, 0, len(byValue))
		for valueKey := range byValue {
			valueKeys = append(valueKeys, valueKey)
		}
		sort.Strings(valueKeys)
		for _, valueKey := range valueKeys {
			valueGroup := byValue[valueKey]
			if len(valueGroup) < 2 {
				continue
			}
			ids := make([]string, 0, len(valueGroup)-1)
			for _, record := range valueGroup[1:] {
				ids = append(ids, record.Record.MemoryID)
				duplicateIDs[record.Record.MemoryID] = true
			}
			duplicates = append(duplicates, MemoryDuplicateGroup{ClaimKey: key, CanonicalID: valueGroup[0].Record.MemoryID, DuplicateIDs: ids})
		}
		if len(byValue) > 1 {
			ids := make([]string, 0, len(group))
			summaries := make([]string, 0, len(byValue))
			seenSummary := map[string]bool{}
			for _, record := range group {
				ids = append(ids, record.Record.MemoryID)
				summary := strings.TrimSpace(record.Record.Summary)
				if !seenSummary[summary] {
					seenSummary[summary] = true
					summaries = append(summaries, summary)
				}
				conflictIDs[record.Record.MemoryID] = true
			}
			sort.Strings(ids)
			sort.Strings(summaries)
			conflicts = append(conflicts, MemoryConflictGroup{ClaimKey: key, RecordIDs: ids, Summaries: summaries})
		}
	}
	return duplicates, conflicts, duplicateIDs, conflictIDs
}
