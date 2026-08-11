package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/sourceinfra"
)

type SearchSourcesInput struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
}

type SearchSourcesResult struct {
	SchemaVersion string                     `json:"schema_version"`
	Kind          string                     `json:"kind"`
	ProjectID     string                     `json:"project_id"`
	Query         string                     `json:"query"`
	Results       []sourceinfra.SearchResult `json:"results"`
	Provider      string                     `json:"provider"`
	SearchKey     string                     `json:"search_key"`
}

type FetchSourceInput struct {
	ProjectID  string `json:"project_id"`
	URL        string `json:"url"`
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
}

type FetchSourceReceipt struct {
	SchemaVersion string                `json:"schema_version"`
	Kind          string                `json:"kind"`
	ProjectID     string                `json:"project_id"`
	RequestedURL  string                `json:"requested_url"`
	FinalURL      string                `json:"final_url"`
	Digest        string                `json:"digest"`
	MIME          string                `json:"mime"`
	Revision      domain.SourceRevision `json:"revision"`
	Reused        bool                  `json:"reused"`
}

func (s *Service) SearchSources(ctx context.Context, actor Actor, input SearchSourcesInput, requestID string) (SearchSourcesResult, error) {
	if strings.TrimSpace(input.ProjectID) == "" {
		return SearchSourcesResult{}, domain.Invalid("SOURCE_SEARCH_PROJECT_REQUIRED", "搜索必须指定项目")
	}
	if _, err := s.store.Project(ctx, actor.TenantID, input.ProjectID); err != nil {
		return SearchSourcesResult{}, err
	}
	query := strings.TrimSpace(input.Query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 500 {
		return SearchSourcesResult{}, domain.Invalid("SOURCE_SEARCH_QUERY_INVALID", "搜索词长度必须在 2 到 500 个字符之间")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		return SearchSourcesResult{}, domain.Invalid("SOURCE_SEARCH_LIMIT_INVALID", "搜索结果最多 20 条")
	}
	if s.sourceSearch == nil {
		return SearchSourcesResult{}, domain.Policy("SOURCE_SEARCH_UNAVAILABLE", "搜索 Provider 尚未配置", "配置 CONTENTCLOUD_SEARCH_ENDPOINT 或注入搜索适配器")
	}
	results, err := s.sourceSearch.Search(ctx, query, limit)
	if err != nil {
		return SearchSourcesResult{}, domain.Policy("SOURCE_SEARCH_FAILED", "搜索 Provider 暂时不可用", "检查 Provider 配置、网络和配额")
	}
	for index := range results {
		if results[index].Rank <= 0 {
			results[index].Rank = index + 1
		}
	}
	searchDigest, err := domain.CanonicalHash(struct {
		ProjectID string `json:"project_id"`
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
	}{input.ProjectID, query, limit})
	if err != nil {
		return SearchSourcesResult{}, err
	}
	searchKey := "sha256:" + searchDigest
	s.audit(ctx, actor, input.ProjectID, "source.search", "project", input.ProjectID, requestID, map[string]any{"query": query, "result_count": len(results), "search_key": searchKey})
	return SearchSourcesResult{SchemaVersion: "contentcloud.source-intake/1.0", Kind: "search_receipt", ProjectID: input.ProjectID, Query: query, Results: results, Provider: fmt.Sprintf("%T", s.sourceSearch), SearchKey: searchKey}, nil
}

func (s *Service) FetchSource(ctx context.Context, actor Actor, input FetchSourceInput, requestID string) (FetchSourceReceipt, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return FetchSourceReceipt{}, err
	}
	if _, err := s.projectForWrite(ctx, actor, input.ProjectID); err != nil {
		return FetchSourceReceipt{}, err
	}
	requested := strings.TrimSpace(input.URL)
	if requested == "" {
		return FetchSourceReceipt{}, domain.Invalid("SOURCE_FETCH_URL_REQUIRED", "采集必须指定 URL")
	}
	if s.sourceFetcher == nil {
		return FetchSourceReceipt{}, domain.Policy("SOURCE_FETCH_UNAVAILABLE", "采集器尚未配置", "检查 Source Provider 配置")
	}
	fetched, err := s.sourceFetcher.Fetch(ctx, requested)
	if err != nil {
		return FetchSourceReceipt{}, domain.Policy("SOURCE_FETCH_BLOCKED", "来源采集被安全策略或网络错误阻止", "检查 URL、域名白名单、重定向和响应大小")
	}
	// Store-level duplicate protection covers concurrent uploads. This lookup
	// gives callers a deterministic receipt for retries before hitting it.
	sources, err := s.store.Sources(ctx, actor.TenantID, input.ProjectID)
	if err != nil {
		return FetchSourceReceipt{}, err
	}
	for _, source := range sources {
		revisions, revisionErr := s.store.SourceRevisions(ctx, actor.TenantID, source.ID)
		if revisionErr != nil {
			return FetchSourceReceipt{}, revisionErr
		}
		for _, revision := range revisions {
			if revision.SHA256 == fetched.Digest && source.SourceType == "web_fetch" {
				return FetchSourceReceipt{SchemaVersion: "contentcloud.source-intake/1.0", Kind: "fetch_receipt", ProjectID: input.ProjectID, RequestedURL: fetched.RequestedURL, FinalURL: fetched.FinalURL, Digest: "sha256:" + fetched.Digest, MIME: fetched.MIME, Revision: revision, Reused: true}, nil
			}
		}
	}
	fileName := sourceinfra.FileName(fetched.FinalURL, fetched.MIME)
	if !strings.Contains(fileName, ".") {
		fileName += ".txt"
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		parsed, _ := url.Parse(fetched.FinalURL)
		name = parsed.Host + parsed.Path
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = "web_fetch"
	}
	revision, err := s.uploadSource(ctx, actor, input.ProjectID, name, sourceType, fileName, fetched.MIME, fetched.Body, requestID)
	if err != nil {
		return FetchSourceReceipt{}, err
	}
	s.audit(ctx, actor, input.ProjectID, "source.fetched", "source_revision", revision.ID, requestID, map[string]any{"requested_url": fetched.RequestedURL, "final_url": fetched.FinalURL, "digest": "sha256:" + fetched.Digest, "mime": fetched.MIME})
	return FetchSourceReceipt{SchemaVersion: "contentcloud.source-intake/1.0", Kind: "fetch_receipt", ProjectID: input.ProjectID, RequestedURL: fetched.RequestedURL, FinalURL: fetched.FinalURL, Digest: "sha256:" + fetched.Digest, MIME: fetched.MIME, Revision: revision}, nil
}
