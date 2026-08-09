package mediapipeline

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const (
	defaultProviderTimeout   = 30 * time.Second
	defaultProviderMaxBody   = 256 << 10
	defaultProviderMaxOutput = 100 << 20
)

// HTTPProviderConfig describes a provider-neutral JSON HTTP contract. The
// provider owns the implementation behind these endpoints; Runtime owns
// idempotency, attempt state, callbacks and reconciliation.
type HTTPProviderConfig struct {
	BaseURL              string
	HTTPClient           *http.Client
	AuthToken            string
	SigningSecret        []byte
	Timeout              time.Duration
	MaxResponseBytes     int64
	MaxDownloadBytes     int64
	AllowedHosts         []string
	AllowPrivateNetworks bool
	UserAgent            string
	Now                  func() time.Time
}

// HTTPProvider is intentionally stateless. A submitted external job must be
// persisted by the caller before polling or retrying, so process restarts do
// not change the provider's idempotency boundary.
type HTTPProvider struct {
	baseURL              *url.URL
	client               *http.Client
	authToken            string
	signingSecret        []byte
	timeout              time.Duration
	maxResponseBytes     int64
	maxDownloadBytes     int64
	allowedHosts         map[string]struct{}
	allowPrivateNetworks bool
	userAgent            string
	now                  func() time.Time
}

// NewHTTPProvider validates the endpoint once at startup. In production,
// private and loopback destinations are rejected unless an explicit network
// policy allows them; tests can opt into a httptest.Server through the same
// explicit flag.
func NewHTTPProvider(config HTTPProviderConfig) (*HTTPProvider, error) {
	base, err := parseProviderURL(config.BaseURL)
	if err != nil {
		return nil, err
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultProviderTimeout
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultProviderMaxBody
	}
	if config.MaxDownloadBytes <= 0 {
		config.MaxDownloadBytes = defaultProviderMaxOutput
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	allowed := map[string]struct{}{}
	for _, host := range config.AllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			allowed[host] = struct{}{}
		}
	}
	if len(allowed) > 0 {
		if _, ok := allowed[strings.ToLower(base.Hostname())]; !ok {
			return nil, domain.Policy("PROVIDER_HOST_NOT_ALLOWED", "服务商基础地址不在出站允许列表中", "把服务商主机加入显式允许列表")
		}
	}
	if err := validateProviderURL(base, allowed, config.AllowPrivateNetworks); err != nil {
		return nil, err
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	} else {
		clone := *client
		client = &clone
	}
	if !config.AllowPrivateNetworks {
		transport, ok := client.Transport.(*http.Transport)
		if client.Transport == nil {
			transport, ok = http.DefaultTransport.(*http.Transport)
		}
		if ok {
			clone := transport.Clone()
			dialer := &net.Dialer{Timeout: config.Timeout}
			clone.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, splitErr := net.SplitHostPort(address)
				if splitErr != nil {
					return nil, splitErr
				}
				ips, lookupErr := net.LookupIP(host)
				if lookupErr != nil {
					return nil, lookupErr
				}
				for _, ip := range ips {
					if isPrivateProviderIP(ip) {
						continue
					}
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				}
				return nil, domain.Policy("PROVIDER_SSRF_BLOCKED", "服务商主机名在连接时解析到了私有或本机网络", "使用公开 HTTPS 服务商地址并配置 DNS 防护")
			}
			client.Transport = clone
		}
	}
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		redirectHost := strings.ToLower(req.URL.Hostname())
		if redirectHost != strings.ToLower(base.Hostname()) {
			if _, allowedRedirect := allowed[redirectHost]; !allowedRedirect {
				return domain.Policy("PROVIDER_REDIRECT_BLOCKED", "服务商响应重定向到未授权主机", "配置同一服务商的显式下载域名")
			}
		}
		if req.URL.Hostname() == "" {
			return domain.Policy("PROVIDER_REDIRECT_BLOCKED", "服务商响应重定向到未授权主机", "配置同一服务商的显式下载域名")
		}
		return validateProviderURL(req.URL, allowed, config.AllowPrivateNetworks)
	}
	return &HTTPProvider{baseURL: base, client: client, authToken: strings.TrimSpace(config.AuthToken), signingSecret: append([]byte(nil), config.SigningSecret...), timeout: config.Timeout, maxResponseBytes: config.MaxResponseBytes, maxDownloadBytes: config.MaxDownloadBytes, allowedHosts: allowed, allowPrivateNetworks: config.AllowPrivateNetworks, userAgent: strings.TrimSpace(config.UserAgent), now: config.Now}, nil
}

func (p *HTTPProvider) Validate(request Request, profile domain.ProviderProfile) error {
	if p == nil || p.baseURL == nil {
		return domain.Policy("PROVIDER_ADAPTER_UNAVAILABLE", "服务商 HTTP 适配器尚未配置", "配置服务商的 HTTPS 基础地址")
	}
	if strings.TrimSpace(request.JobID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.StoryboardSnapshotID) == "" || request.DurationSeconds < 1 {
		return domain.Invalid("PROVIDER_REQUEST_INVALID", "服务商请求缺少任务、幂等键、分镜或时长")
	}
	if !contains(profile.Modes, request.Mode) {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "服务商配置不支持当前生成模式")
	}
	if maxDuration := integerLimit(profile.Limits, "max_duration_seconds"); maxDuration > 0 && request.DurationSeconds > maxDuration {
		return domain.Invalid("PROVIDER_DURATION_LIMIT_EXCEEDED", "请求时长超过服务商配置上限")
	}
	return nil
}

func (p *HTTPProvider) Estimate(_ Request, profile domain.ProviderProfile) (Estimate, error) {
	return Estimate{CostMinor: int64(integerLimit(profile.Pricing, "per_job_minor")), Currency: providerCurrency(profile.Pricing)}, nil
}

func (p *HTTPProvider) Submit(ctx context.Context, request Request, _ domain.ProviderProfile) (Submission, error) {
	var result struct {
		ExternalJobID     string `json:"external_job_id"`
		ID                string `json:"id"`
		JobID             string `json:"job_id"`
		ProviderRequestID string `json:"provider_request_id"`
		RequestID         string `json:"request_id"`
		State             string `json:"state"`
	}
	body := map[string]any{"job_id": request.JobID, "idempotency_key": request.IdempotencyKey, "storyboard_snapshot_id": request.StoryboardSnapshotID, "mode": request.Mode, "aspect_ratio": request.AspectRatio, "duration_seconds": request.DurationSeconds, "input_artifact_refs": request.InputArtifactRefs}
	statusCode, err := p.doJSON(ctx, http.MethodPost, "/v1/generations", request.IdempotencyKey, body, &result)
	if err != nil {
		return Submission{}, err
	}
	externalID := firstNonEmpty(result.ExternalJobID, result.ID, result.JobID)
	requestID := firstNonEmpty(result.ProviderRequestID, result.RequestID)
	if externalID == "" {
		return Submission{}, domain.Invalid("PROVIDER_RESPONSE_INVALID", "服务商提交响应缺少外部任务标识")
	}
	return Submission{ExternalJobID: externalID, ProviderRequestID: requestID, HTTPStatus: statusCode}, nil
}

func (p *HTTPProvider) Status(ctx context.Context, externalJobID string, _ domain.ProviderProfile) (Status, error) {
	externalJobID = strings.TrimSpace(externalJobID)
	if externalJobID == "" || strings.ContainsAny(externalJobID, "/?#") {
		return Status{}, domain.Invalid("PROVIDER_EXTERNAL_ID_INVALID", "服务商外部任务标识无效")
	}
	var result struct {
		State            string `json:"state"`
		Status           string `json:"status"`
		Progress         int    `json:"progress"`
		OutputRef        string `json:"output_ref"`
		OutputURL        string `json:"output_url"`
		ResultURL        string `json:"result_url"`
		ActualMinor      int64  `json:"actual_cost_minor"`
		Currency         string `json:"currency"`
		RetryAfterSecond int    `json:"retry_after_seconds"`
	}
	statusCode, err := p.doJSON(ctx, http.MethodGet, "/v1/generations/"+url.PathEscape(externalJobID), "", nil, &result)
	if err != nil {
		return Status{}, err
	}
	state := strings.ToLower(strings.TrimSpace(firstNonEmpty(result.State, result.Status)))
	if state == "" {
		return Status{}, domain.Invalid("PROVIDER_RESPONSE_INVALID", "服务商状态响应缺少任务状态")
	}
	if result.Progress < 0 || result.Progress > 100 {
		return Status{}, domain.Invalid("PROVIDER_RESPONSE_INVALID", "服务商状态响应的进度无效")
	}
	return Status{State: state, Progress: result.Progress, OutputRef: firstNonEmpty(result.OutputRef, result.OutputURL, result.ResultURL), ActualMinor: result.ActualMinor, RetryAfterSeconds: result.RetryAfterSecond, HTTPStatus: statusCode}, nil
}

func (p *HTTPProvider) Cancel(ctx context.Context, externalJobID string, _ domain.ProviderProfile) error {
	externalJobID = strings.TrimSpace(externalJobID)
	if externalJobID == "" || strings.ContainsAny(externalJobID, "/?#") {
		return domain.Invalid("PROVIDER_EXTERNAL_ID_INVALID", "服务商外部任务标识无效")
	}
	_, err := p.doJSON(ctx, http.MethodPost, "/v1/generations/"+url.PathEscape(externalJobID)+"/cancel", "", map[string]any{}, nil)
	return err
}

// StreamedDownload keeps the response body bounded by the caller. It is the
// preferred API for production object stores; Download remains as a bounded
// compatibility method for the current Adapter interface.
type StreamedDownload struct {
	Body          io.ReadCloser
	MediaType     string
	FileName      string
	ContentLength int64
}

type StreamingDownloader interface {
	OpenDownload(context.Context, string, domain.ProviderProfile) (StreamedDownload, error)
}

func (p *HTTPProvider) OpenDownload(ctx context.Context, outputRef string, _ domain.ProviderProfile) (StreamedDownload, error) {
	target, err := p.resolveOutputURL(outputRef)
	if err != nil {
		return StreamedDownload{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return StreamedDownload{}, err
	}
	resp, err := p.do(ctx, req, "")
	if err != nil {
		return StreamedDownload{}, err
	}
	if resp.ContentLength > p.maxDownloadBytes {
		_ = resp.Body.Close()
		return StreamedDownload{}, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出超过配置的媒体大小上限")
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "video/mp4" {
		_ = resp.Body.Close()
		return StreamedDownload{}, domain.Invalid("MEDIA_OUTPUT_MIME_INVALID", "服务商输出 MIME 不是受支持的 MP4")
	}
	return StreamedDownload{Body: &limitedReadCloser{Reader: io.LimitReader(resp.Body, p.maxDownloadBytes+1), closer: resp.Body, limit: p.maxDownloadBytes}, MediaType: mediaType, FileName: safeFileName(target, resp.Header.Get("Content-Disposition")), ContentLength: resp.ContentLength}, nil
}

func (p *HTTPProvider) Download(ctx context.Context, outputRef string, profile domain.ProviderProfile) (Download, error) {
	stream, err := p.OpenDownload(ctx, outputRef, profile)
	if err != nil {
		return Download{}, err
	}
	defer stream.Body.Close()
	body, err := io.ReadAll(stream.Body)
	if err != nil {
		return Download{}, err
	}
	if int64(len(body)) > p.maxDownloadBytes {
		return Download{}, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出超过配置的媒体大小上限")
	}
	return Download{Body: body, MediaType: stream.MediaType, FileName: stream.FileName}, nil
}

func (p *HTTPProvider) doJSON(ctx context.Context, method, endpoint, idempotency string, payload any, target any) (int, error) {
	var body io.Reader
	var encoded []byte
	var err error
	if payload != nil {
		encoded, err = json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(encoded)
	}
	targetURL, err := p.resolveEndpoint(endpoint)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	resp, err := p.do(ctx, req, string(encoded))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if target == nil {
		return resp.StatusCode, nil
	}
	limited := io.LimitReader(resp.Body, p.maxResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return resp.StatusCode, err
	}
	if int64(len(responseBody)) > p.maxResponseBytes {
		return resp.StatusCode, domain.Invalid("PROVIDER_RESPONSE_TOO_LARGE", "服务商 JSON 响应超过大小上限")
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return resp.StatusCode, domain.Invalid("PROVIDER_RESPONSE_INVALID", "服务商响应不是有效 JSON")
	}
	return resp.StatusCode, nil
}

func (p *HTTPProvider) do(ctx context.Context, req *http.Request, body string) (*http.Response, error) {
	if p == nil || p.client == nil {
		return nil, domain.Policy("PROVIDER_ADAPTER_UNAVAILABLE", "服务商 HTTP 适配器尚未配置", "检查 Provider 运行时配置")
	}
	if err := validateProviderURL(req.URL, p.allowedHosts, p.allowPrivateNetworks); err != nil {
		return nil, err
	}
	if p.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.authToken)
	}
	if p.userAgent != "" {
		req.Header.Set("User-Agent", p.userAgent)
	}
	digest := sha256.Sum256([]byte(body))
	digestHex := hex.EncodeToString(digest[:])
	req.Header.Set("X-ContentCloud-Request-Digest", "sha256:"+digestHex)
	if len(p.signingSecret) > 0 {
		timestamp := strconv.FormatInt(p.now().UTC().Unix(), 10)
		signature := hmac.New(sha256.New, p.signingSecret)
		_, _ = signature.Write([]byte(timestamp + "\n" + digestHex + "\n" + req.URL.Path))
		req.Header.Set("X-ContentCloud-Timestamp", timestamp)
		req.Header.Set("X-ContentCloud-Signature", "sha256="+hex.EncodeToString(signature.Sum(nil)))
	}
	callCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	resp, err := p.client.Do(req.WithContext(callCtx))
	if err != nil {
		if callCtx.Err() != nil {
			return nil, &domain.Error{Type: "provider", Subtype: "timeout", Code: "PROVIDER_TIMEOUT", Message: "服务商请求超时", Retryable: true, Details: map[string]any{"endpoint": req.URL.Path}}
		}
		return nil, &domain.Error{Type: "provider", Subtype: "network", Code: "PROVIDER_NETWORK_ERROR", Message: "服务商网络请求失败", Retryable: true}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		retryable := resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusTooEarly || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		errorDigest := sha256.Sum256(bodyBytes)
		return nil, &domain.Error{Type: "provider", Subtype: "http", Code: "PROVIDER_HTTP_ERROR", Message: "服务商返回非成功状态", Retryable: retryable, Details: map[string]any{"status": resp.StatusCode, "retry_after_seconds": parseRetryAfter(resp.Header.Get("Retry-After")), "response_digest": "sha256:" + hex.EncodeToString(errorDigest[:])}}
	}
	return resp, nil
}

func (p *HTTPProvider) resolveEndpoint(endpoint string) (*url.URL, error) {
	joined, err := url.Parse(endpoint)
	if err != nil || joined.IsAbs() || joined.Host != "" {
		return nil, domain.Invalid("PROVIDER_ENDPOINT_INVALID", "服务商接口路径无效")
	}
	base := *p.baseURL
	base.Path = path.Join(strings.TrimSuffix(base.Path, "/"), "/", strings.TrimPrefix(joined.Path, "/"))
	base.RawQuery = joined.RawQuery
	return &base, validateProviderURL(&base, p.allowedHosts, p.allowPrivateNetworks)
}

func (p *HTTPProvider) resolveOutputURL(outputRef string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(outputRef))
	if err != nil || strings.TrimSpace(outputRef) == "" {
		return nil, domain.Invalid("PROVIDER_OUTPUT_REF_INVALID", "服务商输出引用无效")
	}
	if !parsed.IsAbs() {
		return p.resolveEndpoint(parsed.RequestURI())
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, domain.Policy("PROVIDER_OUTPUT_SCHEME_BLOCKED", "服务商输出引用使用了不允许的协议", "仅允许 HTTPS 或测试环境显式允许的 HTTP")
	}
	return parsed, validateProviderURL(parsed, p.allowedHosts, p.allowPrivateNetworks)
}

func parseProviderURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, domain.Invalid("PROVIDER_BASE_URL_INVALID", "服务商基础地址必须是无用户信息的 HTTP(S) URL")
	}
	return parsed, nil
}

func validateProviderURL(target *url.URL, allowed map[string]struct{}, allowPrivate bool) error {
	if target == nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return domain.Invalid("PROVIDER_URL_INVALID", "服务商请求地址无效")
	}
	host := strings.ToLower(target.Hostname())
	if len(allowed) > 0 {
		if _, ok := allowed[host]; !ok {
			return domain.Policy("PROVIDER_HOST_NOT_ALLOWED", "服务商请求目标不在出站允许列表中", "只允许访问已核验的服务商或下载域名")
		}
	}
	if allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateProviderIP(ip) {
			return domain.Policy("PROVIDER_SSRF_BLOCKED", "服务商请求目标属于私有或本机网络", "使用公开 HTTPS 服务商地址")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return domain.Invalid("PROVIDER_DNS_INVALID", "服务商主机名无法解析")
	}
	for _, ip := range ips {
		if isPrivateProviderIP(ip) {
			return domain.Policy("PROVIDER_SSRF_BLOCKED", "服务商主机名解析到了私有或本机网络", "使用公开 HTTPS 服务商地址并配置 DNS 防护")
		}
	}
	return nil
}

func isPrivateProviderIP(ip net.IP) bool {
	parsed, err := netip.ParseAddr(ip.String())
	if err != nil {
		return true
	}
	return parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() || parsed.IsUnspecified() || parsed.IsMulticast()
}

type limitedReadCloser struct {
	Reader io.Reader
	closer io.Closer
	limit  int64
	read   int64
}

func (r *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += int64(n)
	if r.read > r.limit {
		return n, domain.Invalid("MEDIA_OUTPUT_SIZE_INVALID", "服务商输出超过配置的媒体大小上限")
	}
	return n, err
}

func (r *limitedReadCloser) Close() error { return r.closer.Close() }

func safeFileName(target *url.URL, disposition string) string {
	for _, part := range strings.Split(disposition, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(key, "filename") {
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if value != "" && !strings.ContainsAny(value, `/\\`) {
				return value
			}
		}
	}
	name := path.Base(target.Path)
	if name == "." || name == "/" || name == "" {
		return "generated-take.mp4"
	}
	return name
}

func integerLimit(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func providerCurrency(values map[string]any) string {
	currency, _ := values["currency"].(string)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return "CNY"
	}
	return currency
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseRetryAfter(value string) int {
	seconds, _ := strconv.Atoi(strings.TrimSpace(value))
	if seconds < 0 {
		return 0
	}
	return seconds
}

var _ Adapter = (*HTTPProvider)(nil)
var _ StreamingDownloader = (*HTTPProvider)(nil)
