package localworkspace

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type MemoryEmbeddingRequest struct {
	Model string   `json:"model,omitempty"`
	Input []string `json:"input"`
}

type MemoryEmbeddingAdapter interface {
	Provider() string
	Embed(context.Context, []string) ([][]float32, error)
}

type memoryEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (a *MemoryRemoteAdapter) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if a == nil || a.baseURL == nil || a.client == nil {
		return nil, domain.Invalid("MEMORY_EMBEDDING_ADAPTER_INVALID", "Embedding 适配器未配置")
	}
	clean := make([]string, 0, len(inputs))
	for _, input := range inputs {
		value := strings.TrimSpace(input)
		if value == "" {
			return nil, domain.Invalid("MEMORY_EMBEDDING_INPUT_INVALID", "Embedding 输入不能包含空文本")
		}
		clean = append(clean, value)
	}
	if len(clean) == 0 || len(clean) > 100 {
		return nil, domain.Invalid("MEMORY_EMBEDDING_INPUT_INVALID", "Embedding 一次只能处理 1 到 100 条文本")
	}
	body, err := json.Marshal(MemoryEmbeddingRequest{Model: a.embeddingModel, Input: clean})
	if err != nil {
		return nil, err
	}
	responseBody, err := a.do(ctx, "POST", a.embeddingPath, body)
	if err != nil {
		return nil, err
	}
	var direct memoryEmbeddingResponse
	if err := json.Unmarshal(responseBody, &direct); err != nil || len(direct.Data) == 0 {
		var wrapped struct {
			Data memoryEmbeddingResponse `json:"data"`
		}
		if json.Unmarshal(responseBody, &wrapped) != nil || len(wrapped.Data.Data) == 0 {
			return nil, domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 服务响应缺少 data")
		}
		direct = wrapped.Data
	}
	result := make([][]float32, len(clean))
	seen := map[int]bool{}
	for _, item := range direct.Data {
		if item.Index < 0 || item.Index >= len(result) || seen[item.Index] || len(item.Embedding) == 0 || len(item.Embedding) > 4096 {
			return nil, domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 响应的 index 或向量维度无效")
		}
		for _, value := range item.Embedding {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 向量包含非有限数值")
			}
		}
		result[item.Index] = append([]float32(nil), item.Embedding...)
		seen[item.Index] = true
	}
	for _, vector := range result {
		if len(vector) == 0 {
			return nil, domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 响应缺少输入对应向量")
		}
	}
	return result, nil
}

// QueryMemoryWithEmbedding is an explicit evaluation path. It retrieves a
// bounded local candidate set, reranks it with cosine similarity, and keeps
// the original source/scope/status fields. It is intentionally not used by
// workspace_context until a corpus benchmark proves a material gain.
func QueryMemoryWithEmbedding(ctx context.Context, root string, adapter MemoryEmbeddingAdapter, options MemoryQueryOptions) (MemoryQueryResult, error) {
	if adapter == nil {
		return MemoryQueryResult{}, domain.Invalid("MEMORY_EMBEDDING_ADAPTER_REQUIRED", "混合记忆查询必须提供 Embedding 适配器")
	}
	limit, maxChars, err := normalizeMemoryBudget(options.Limit, options.MaxChars)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	baseOptions := options
	baseOptions.Query = ""
	baseOptions.Limit = maximumMemoryLimit
	baseOptions.MaxChars = maximumMemoryMaxChars
	base, err := QueryMemory(baseOptions)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	if strings.TrimSpace(options.Query) == "" || len(base.Candidates) == 0 {
		base.Backend = MemoryRemoteBackendPrefix + adapter.Provider() + "+hybrid"
		selected, used, truncated := applyMemoryBudget(base.Candidates, limit, maxChars)
		base.Limit = limit
		base.MaxChars = maxChars
		base.Candidates, base.UsedChars, base.Truncated = selected, used, truncated
		return base, nil
	}
	texts := make([]string, 0, len(base.Candidates)+1)
	texts = append(texts, strings.TrimSpace(options.Query))
	for _, candidate := range base.Candidates {
		texts = append(texts, candidate.Summary)
	}
	vectors, err := adapter.Embed(ctx, texts)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	if err := validateMemoryEmbeddingVectors(vectors, len(texts)); err != nil {
		return MemoryQueryResult{}, err
	}
	type scored struct {
		candidate MemoryCandidate
		score     float64
	}
	scoredCandidates := make([]scored, 0, len(base.Candidates))
	for index, candidate := range base.Candidates {
		if len(vectors[0]) != len(vectors[index+1]) {
			return MemoryQueryResult{}, domain.Invalid("MEMORY_EMBEDDING_DIMENSION_MISMATCH", "查询和候选 Embedding 维度不一致")
		}
		scoredCandidates = append(scoredCandidates, scored{candidate: candidate, score: cosineMemorySimilarity(vectors[0], vectors[index+1])})
	}
	sort.SliceStable(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score != scoredCandidates[j].score {
			return scoredCandidates[i].score > scoredCandidates[j].score
		}
		return scoredCandidates[i].candidate.MemoryID < scoredCandidates[j].candidate.MemoryID
	})
	ranked := make([]MemoryCandidate, 0, len(scoredCandidates))
	for _, item := range scoredCandidates {
		ranked = append(ranked, item.candidate)
	}
	selected, used, truncated := applyMemoryBudget(ranked, limit, maxChars)
	base.Query = strings.TrimSpace(options.Query)
	base.Limit = limit
	base.MaxChars = maxChars
	base.Backend = MemoryRemoteBackendPrefix + adapter.Provider() + "+hybrid"
	base.Candidates, base.UsedChars, base.Truncated = selected, used, truncated
	return base, nil
}

func cosineMemorySimilarity(left, right []float32) float64 {
	var dot, leftNorm, rightNorm float64
	for index := range left {
		l, r := float64(left[index]), float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func validateMemoryEmbeddingVectors(vectors [][]float32, expected int) error {
	if expected <= 0 || len(vectors) != expected {
		return domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 向量数量与输入不一致")
	}
	dimension := 0
	for _, vector := range vectors {
		if len(vector) == 0 || len(vector) > 4096 {
			return domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 响应的向量维度无效")
		}
		if dimension == 0 {
			dimension = len(vector)
		} else if len(vector) != dimension {
			return domain.Invalid("MEMORY_EMBEDDING_DIMENSION_MISMATCH", "Embedding 向量维度不一致")
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return domain.Invalid("MEMORY_EMBEDDING_RESPONSE_INVALID", "Embedding 向量包含非有限数值")
			}
		}
	}
	return nil
}
