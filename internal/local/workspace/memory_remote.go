package localworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
)

const MemoryRemoteBackendPrefix = "remote:"

type MemoryRemoteQueryRequest struct {
	Scope    MemoryScope `json:"scope"`
	Query    string      `json:"query,omitempty"`
	Kinds    []string    `json:"kinds,omitempty"`
	Limit    int         `json:"limit"`
	MaxChars int         `json:"max_chars"`
}

type MemoryRemoteExtractRequest struct {
	Scope        MemoryScope `json:"scope"`
	SourceRef    string      `json:"source_ref"`
	SourceDigest string      `json:"source_digest"`
	Content      string      `json:"content"`
}

type MemoryExtractedCandidate struct {
	MemoryID string `json:"memory_id,omitempty"`
	Kind     string `json:"kind"`
	ClaimKey string `json:"claim_key,omitempty"`
	Summary  string `json:"summary"`
	FormedBy string `json:"formed_by,omitempty"`
}

type MemoryRemoteAdapterConfig struct {
	Provider             string
	BaseURL              string
	AuthToken            string
	QueryPath            string
	RememberPath         string
	ExtractPath          string
	EmbeddingPath        string
	EmbeddingModel       string
	Timeout              time.Duration
	AllowPrivateNetworks bool
	AllowInsecureHTTP    bool
	Client               *http.Client
}

type MemoryRemoteAdapter struct {
	provider       string
	baseURL        *url.URL
	authToken      string
	queryPath      string
	rememberPath   string
	extractPath    string
	embeddingPath  string
	embeddingModel string
	timeout        time.Duration
	allowPrivate   bool
	client         *http.Client
}

type memoryRemoteQueryResponse struct {
	Candidates []MemoryCandidate `json:"candidates"`
}

type memoryRemoteExtractResponse struct {
	Candidates []MemoryExtractedCandidate `json:"candidates"`
}

// NewMemoryRemoteAdapter creates a provider-neutral HTTP adapter. Provider
// paths are configurable because TencentDB Agent Memory and Mem0 expose
// different API envelopes; the returned data is always ContentCloud-shaped.
func NewMemoryRemoteAdapter(config MemoryRemoteAdapterConfig) (*MemoryRemoteAdapter, error) {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	if provider == "" {
		provider = "custom"
	}
	base, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || base.User != nil || base.Host == "" || (base.Scheme != "https" && !(config.AllowInsecureHTTP && base.Scheme == "http")) {
		return nil, fault.Invalid("MEMORY_REMOTE_URL_INVALID", "远程记忆地址必须是无用户信息的 HTTPS URL；测试环境可显式允许 HTTP")
	}
	if !config.AllowPrivateNetworks {
		if err := rejectPrivateMemoryRemoteHost(base.Hostname()); err != nil {
			return nil, err
		}
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client, err := newMemoryRemoteHTTPClient(config, timeout)
	if err != nil {
		return nil, err
	}
	return &MemoryRemoteAdapter{provider: provider, baseURL: base, authToken: strings.TrimSpace(config.AuthToken), queryPath: normalizeRemotePath(config.QueryPath, "/v1/memory/query"), rememberPath: normalizeRemotePath(config.RememberPath, "/v1/memory/remember"), extractPath: normalizeRemotePath(config.ExtractPath, "/v1/memory/extract"), embeddingPath: normalizeRemotePath(config.EmbeddingPath, "/v1/embeddings"), embeddingModel: strings.TrimSpace(config.EmbeddingModel), timeout: timeout, allowPrivate: config.AllowPrivateNetworks, client: client}, nil
}

func newMemoryRemoteHTTPClient(config MemoryRemoteAdapterConfig, timeout time.Duration) (*http.Client, error) {
	var client http.Client
	if config.Client != nil {
		client = *config.Client
	} else {
		client.Timeout = timeout
	}
	if client.Timeout <= 0 || client.Timeout > timeout {
		client.Timeout = timeout
	}
	// Remote memory adapters are endpoint-bound. Redirects would otherwise let
	// a provider move an authenticated request to an unvalidated host.
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if config.AllowPrivateNetworks {
		return &client, nil
	}
	transport, ok := client.Transport.(*http.Transport)
	if client.Transport != nil && !ok {
		return nil, fault.Invalid("MEMORY_REMOTE_CLIENT_INVALID", "生产远程记忆适配器不允许使用无法校验目标地址的自定义 HTTP Transport")
	}
	if !ok {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	} else {
		transport = transport.Clone()
	}
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips := []net.IP{net.ParseIP(host)}
		if ips[0] == nil {
			ips, err = net.LookupIP(host)
			if err != nil {
				return nil, err
			}
		}
		var lastErr error
		for _, ip := range ips {
			if ip == nil || isPrivateMemoryIP(ip) {
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fault.Policy("MEMORY_REMOTE_PRIVATE_NETWORK", "远程记忆地址解析到本机或私有网络", "生产环境使用公开 HTTPS 服务地址；本地测试显式开启 AllowPrivateNetworks")
	}
	client.Transport = transport
	return &client, nil
}

func (a *MemoryRemoteAdapter) Provider() string {
	if a == nil {
		return ""
	}
	return a.provider
}

func (a *MemoryRemoteAdapter) Query(ctx context.Context, request MemoryRemoteQueryRequest) ([]MemoryCandidate, error) {
	if a == nil || a.baseURL == nil || a.client == nil {
		return nil, fault.Invalid("MEMORY_REMOTE_ADAPTER_INVALID", "远程记忆适配器未配置")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	responseBody, err := a.do(ctx, http.MethodPost, a.queryPath, body)
	if err != nil {
		return nil, err
	}
	var envelope memoryRemoteQueryResponse
	if err := decodeRemoteCandidates(responseBody, &envelope); err != nil {
		return nil, err
	}
	return envelope.Candidates, nil
}

func (a *MemoryRemoteAdapter) Remember(ctx context.Context, record MemoryRecord) error {
	if a == nil || a.baseURL == nil || a.client == nil {
		return fault.Invalid("MEMORY_REMOTE_ADAPTER_INVALID", "远程记忆适配器未配置")
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = a.do(ctx, http.MethodPost, a.rememberPath, body)
	return err
}

func (a *MemoryRemoteAdapter) Extract(ctx context.Context, request MemoryRemoteExtractRequest) ([]MemoryExtractedCandidate, error) {
	if a == nil || a.baseURL == nil || a.client == nil {
		return nil, fault.Invalid("MEMORY_REMOTE_ADAPTER_INVALID", "远程记忆适配器未配置")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	responseBody, err := a.do(ctx, http.MethodPost, a.extractPath, body)
	if err != nil {
		return nil, err
	}
	var direct memoryRemoteExtractResponse
	if json.Unmarshal(responseBody, &direct) == nil && direct.Candidates != nil {
		return direct.Candidates, nil
	}
	var wrapped struct {
		Data memoryRemoteExtractResponse `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &wrapped); err != nil || wrapped.Data.Candidates == nil {
		return nil, fault.Invalid("MEMORY_REMOTE_EXTRACT_INVALID", "远程记忆抽取响应不是受支持的候选集合")
	}
	return wrapped.Data.Candidates, nil
}

// QueryRemoteMemory is an explicit opt-in path. It validates every returned
// candidate against the bound local workspace before exposing it to callers.
func QueryRemoteMemory(ctx context.Context, root string, adapter *MemoryRemoteAdapter, options MemoryQueryOptions) (MemoryQueryResult, error) {
	if adapter == nil {
		return MemoryQueryResult{}, fault.Invalid("MEMORY_REMOTE_ADAPTER_REQUIRED", "远程记忆查询必须提供适配器")
	}
	now := normalizedMemoryTime(options.Now)
	resolved, scope, err := resolveMemoryScope(root)
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
	catalog, err := scanMemoryCatalog(resolved, scope)
	if err != nil {
		return MemoryQueryResult{}, err
	}
	remote, err := adapter.Query(ctx, MemoryRemoteQueryRequest{Scope: scope, Query: strings.TrimSpace(options.Query), Kinds: kinds, Limit: limit, MaxChars: maxChars})
	if err != nil {
		return MemoryQueryResult{}, err
	}
	validated := make([]MemoryCandidate, 0, len(remote))
	warnings := append([]string{}, catalog.Warnings...)
	for _, candidate := range remote {
		if err := validateRemoteMemoryCandidate(resolved, scope, candidate); err != nil {
			warnings = append(warnings, "远程记忆候选已拒绝："+err.Error())
			continue
		}
		validated = append(validated, candidate)
	}
	selected, usedChars, truncated := applyMemoryBudget(validated, limit, maxChars)
	return MemoryQueryResult{SchemaVersion: MemoryProjectionSchema, Scope: scope, Query: strings.TrimSpace(options.Query), Kinds: kinds, Backend: MemoryRemoteBackendPrefix + adapter.Provider(), IndexState: MemoryStateReady, SourceCount: len(catalog.Sources), EntryCount: len(validated), DuplicateCount: catalog.DuplicateCount, ConflictCount: catalog.ConflictCount, Limit: limit, MaxChars: maxChars, UsedChars: usedChars, Truncated: truncated, Candidates: selected, Warnings: uniqueSortedStrings(warnings), GeneratedAt: now}, nil
}

func validateRemoteMemoryCandidate(root string, scope MemoryScope, candidate MemoryCandidate) error {
	if candidate.Scope != scope {
		return fault.Policy("MEMORY_REMOTE_SCOPE_MISMATCH", "远程候选 scope 与当前绑定工作区不一致", "让适配器从 ContentCloud scope 生成查询")
	}
	if candidate.SchemaVersion != MemoryEntrySchema || candidate.MemoryID == "" || candidate.MemoryID != strings.TrimSpace(candidate.MemoryID) || candidate.MemoryID != localSafeName(candidate.MemoryID) || candidate.Kind == "" || !validMemoryKinds[candidate.Kind] || strings.TrimSpace(candidate.Summary) == "" || len([]rune(candidate.Summary)) > memoryRecordSummaryRunes || candidate.Trust != "memory_candidate" || candidate.Status != "active" || strings.TrimSpace(candidate.FormedBy) == "" || len([]rune(candidate.FormedBy)) > 160 || candidate.ObservedAt.IsZero() {
		return fault.Invalid("MEMORY_REMOTE_CANDIDATE_INVALID", "远程候选缺少合法的 ID、类型、摘要、信任或状态")
	}
	if _, digest, _, err := resolveMemoryRecordSource(root, candidate.SourceRef); err != nil || digest != candidate.SourceDigest {
		return fault.Conflict("MEMORY_REMOTE_SOURCE_STALE", "远程候选来源不存在或 digest 已变化")
	}
	return nil
}

func (a *MemoryRemoteAdapter) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if !a.allowPrivate {
		if err := rejectPrivateMemoryRemoteHost(a.baseURL.Hostname()); err != nil {
			return nil, err
		}
	}
	requestContext := ctx
	if a.timeout > 0 {
		var cancel context.CancelFunc
		requestContext, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	endpoint := *a.baseURL
	endpoint.Path = strings.TrimRight(a.baseURL.Path, "/") + normalizeRemotePath(path, "/")
	request, err := http.NewRequestWithContext(requestContext, method, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-ContentCloud-Memory-Provider", a.provider)
	if a.authToken != "" {
		request.Header.Set("Authorization", "Bearer "+a.authToken)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return nil, memoryRemoteRetryable("MEMORY_REMOTE_UNAVAILABLE", "远程记忆服务不可用", "继续使用本地 FTS 或稍后重试")
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if readErr != nil {
		return nil, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, memoryRemoteRetryable("MEMORY_REMOTE_HTTP_ERROR", fmt.Sprintf("远程记忆服务返回 HTTP %d", response.StatusCode), "检查适配器地址、凭据和服务状态")
	}
	return responseBody, nil
}

func decodeRemoteCandidates(body []byte, output *memoryRemoteQueryResponse) error {
	var direct memoryRemoteQueryResponse
	if err := json.Unmarshal(body, &direct); err == nil && direct.Candidates != nil {
		*output = direct
		return nil
	}
	var wrapped struct {
		Data memoryRemoteQueryResponse `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.Data.Candidates == nil {
		return fault.Invalid("MEMORY_REMOTE_RESPONSE_INVALID", "远程记忆查询响应不是受支持的候选集合")
	}
	*output = wrapped.Data
	return nil
}

func normalizeRemotePath(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return value
}

func rejectPrivateMemoryRemoteHost(host string) error {
	if host == "" {
		return fault.Invalid("MEMORY_REMOTE_HOST_INVALID", "远程记忆地址缺少主机名")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateMemoryIP(ip) {
			return fault.Policy("MEMORY_REMOTE_PRIVATE_NETWORK", "远程记忆地址指向本机或私有网络", "生产环境使用公开 HTTPS 服务地址；本地测试显式开启 AllowPrivateNetworks")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fault.Policy("MEMORY_REMOTE_DNS_UNRESOLVED", "无法解析远程记忆服务主机", "检查 DNS 或使用可解析的 HTTPS 地址")
	}
	for _, ip := range ips {
		if isPrivateMemoryIP(ip) {
			return fault.Policy("MEMORY_REMOTE_PRIVATE_NETWORK", "远程记忆地址解析到本机或私有网络", "生产环境使用公开 HTTPS 服务地址；本地测试显式开启 AllowPrivateNetworks")
		}
	}
	return nil
}

func isPrivateMemoryIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func memoryRemoteRetryable(code, message, hint string) *fault.Error {
	return &fault.Error{Type: "network", Subtype: "memory_remote", Code: code, Message: message, Retryable: true, Hint: hint, ExitCode: 5}
}
