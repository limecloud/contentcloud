package localworkspace

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const (
	MemoryExtractionSchema    = "contentcloud.memory-extraction/1.0"
	memoryMaxExtractInputSize = 8 << 20
)

type MemoryExtractOptions struct {
	Root       string
	SourceRefs []string
	Adapter    *MemoryRemoteAdapter
	FormedBy   string
	Now        time.Time
}

type MemoryExtractionRejection struct {
	SourceRef string `json:"source_ref"`
	Summary   string `json:"summary,omitempty"`
	Reason    string `json:"reason"`
}

type MemoryExtractionReport struct {
	SchemaVersion  string                      `json:"schema_version"`
	Scope          MemoryScope                 `json:"scope"`
	Backend        string                      `json:"backend"`
	SourceCount    int                         `json:"source_count"`
	CandidateCount int                         `json:"candidate_count"`
	Remembered     []MemoryRememberReport      `json:"remembered"`
	Rejected       []MemoryExtractionRejection `json:"rejected"`
	Warnings       []string                    `json:"warnings"`
	GeneratedAt    time.Time                   `json:"generated_at"`
}

// ExtractMemory is an explicit, opt-in remote formation path. The provider
// receives only selected, already-scanned local source text. Every returned
// candidate is written through RememberMemory, so scope, digest, ID safety and
// immutable conflict rules remain local invariants.
func ExtractMemory(ctx context.Context, options MemoryExtractOptions) (MemoryExtractionReport, error) {
	if options.Adapter == nil {
		return MemoryExtractionReport{}, fault.Invalid("MEMORY_EXTRACT_ADAPTER_REQUIRED", "记忆抽取必须提供显式远程适配器")
	}
	root, scope, err := resolveMemoryScope(options.Root)
	if err != nil {
		return MemoryExtractionReport{}, err
	}
	catalog, err := scanMemoryCatalog(root, scope)
	if err != nil {
		return MemoryExtractionReport{}, err
	}
	sources := selectMemoryExtractionSources(catalog.Sources, options.SourceRefs)
	if len(options.SourceRefs) > 0 && len(sources) != len(uniqueStrings(options.SourceRefs)) {
		return MemoryExtractionReport{}, fault.Invalid("MEMORY_EXTRACT_SOURCE_INVALID", "source_refs 必须全部指向当前允许读取的来源文件")
	}
	var inputBytes int64
	for _, source := range sources {
		inputBytes += int64(len(source.Contents))
		if inputBytes > memoryMaxExtractInputSize {
			return MemoryExtractionReport{}, fault.Policy("MEMORY_EXTRACT_INPUT_TOO_LARGE", "远程记忆抽取的来源正文总量不能超过 8 MiB", "缩小 source_refs 范围后重试")
		}
	}
	formedBy := strings.TrimSpace(options.FormedBy)
	if formedBy == "" {
		formedBy = "memory-extractor:" + options.Adapter.Provider()
	}
	now := normalizedMemoryTime(options.Now)
	report := MemoryExtractionReport{SchemaVersion: MemoryExtractionSchema, Scope: scope, Backend: MemoryRemoteBackendPrefix + options.Adapter.Provider(), SourceCount: len(sources), Remembered: []MemoryRememberReport{}, Rejected: []MemoryExtractionRejection{}, Warnings: append([]string{}, catalog.Warnings...), GeneratedAt: now}
	for _, source := range sources {
		candidates, extractErr := options.Adapter.Extract(ctx, MemoryRemoteExtractRequest{Scope: scope, SourceRef: source.Ref, SourceDigest: source.Digest, Content: source.Contents})
		if extractErr != nil {
			return MemoryExtractionReport{}, extractErr
		}
		if len(candidates) > 100 {
			return MemoryExtractionReport{}, fault.Policy("MEMORY_EXTRACT_TOO_MANY_CANDIDATES", "单个来源的记忆抽取候选不能超过 100 条", "缩小来源范围或让抽取器先做摘要")
		}
		for _, candidate := range candidates {
			report.CandidateCount++
			if candidate.Kind == "" || strings.TrimSpace(candidate.Summary) == "" {
				report.Rejected = append(report.Rejected, MemoryExtractionRejection{SourceRef: source.Ref, Summary: candidate.Summary, Reason: "kind 和 summary 必填"})
				continue
			}
			candidateFormedBy := strings.TrimSpace(candidate.FormedBy)
			if candidateFormedBy == "" {
				candidateFormedBy = formedBy
			}
			remembered, rememberErr := RememberMemory(MemoryRememberOptions{Root: root, MemoryID: candidate.MemoryID, Kind: candidate.Kind, ClaimKey: candidate.ClaimKey, SourceRef: source.Ref, Summary: candidate.Summary, FormedBy: candidateFormedBy, Now: now})
			if rememberErr != nil {
				report.Rejected = append(report.Rejected, MemoryExtractionRejection{SourceRef: source.Ref, Summary: candidate.Summary, Reason: rememberErr.Error()})
				continue
			}
			report.Remembered = append(report.Remembered, remembered)
		}
	}
	return report, nil
}

func selectMemoryExtractionSources(sources []memorySource, refs []string) []memorySource {
	if len(refs) == 0 {
		result := append([]memorySource(nil), sources...)
		sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
		return result
	}
	wanted := map[string]bool{}
	for _, ref := range refs {
		wanted[filepath.ToSlash(strings.TrimSpace(ref))] = true
	}
	result := make([]memorySource, 0, len(wanted))
	for _, source := range sources {
		if wanted[source.Ref] {
			result = append(result, source)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Ref < result[j].Ref })
	return result
}
