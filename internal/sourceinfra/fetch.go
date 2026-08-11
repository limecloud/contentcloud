package sourceinfra

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultFetchLimit int64 = 10 << 20
	maxSearchResults        = 20
)

// SearchResult is provider-neutral. Provider response bodies never cross the
// application boundary; only these stable fields are persisted or shown.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
	Source  string `json:"source,omitempty"`
	Rank    int    `json:"rank"`
}

type SearchProvider interface {
	Search(context.Context, string, int) ([]SearchResult, error)
}

// HTTPJSONSearchProvider accepts the deliberately small provider contract:
// {"results":[{"title":"...","url":"...","snippet":"..."}]}. A
// data.results wrapper is also accepted for common SaaS gateways.
type HTTPJSONSearchProvider struct {
	Endpoint string
	Client   *http.Client
}

func NewDefaultSearchProvider() SearchProvider {
	endpoint := strings.TrimSpace(os.Getenv("CONTENTCLOUD_SEARCH_ENDPOINT"))
	if endpoint == "" {
		return urlSearchProvider{}
	}
	return &HTTPJSONSearchProvider{Endpoint: endpoint, Client: &http.Client{Timeout: 12 * time.Second}}
}

func (p *HTTPJSONSearchProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if p == nil || strings.TrimSpace(p.Endpoint) == "" {
		return nil, errors.New("search provider endpoint is not configured")
	}
	endpoint, err := url.Parse(p.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		return nil, errors.New("search provider endpoint must be an HTTPS URL")
	}
	if limit <= 0 || limit > maxSearchResults {
		limit = 10
	}
	values := endpoint.Query()
	values.Set("q", strings.TrimSpace(query))
	values.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = values.Encode()
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("search provider request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("search provider returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Results []SearchResult `json:"results"`
		Data    struct {
			Results []SearchResult `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("search provider response is not JSON: %w", err)
	}
	results := envelope.Results
	if len(results) == 0 {
		results = envelope.Data.Results
	}
	return normalizeResults(results, limit)
}

// URL queries are useful for controlled, credential-free intake and make the
// search contract usable before a paid search SaaS is configured.
type urlSearchProvider struct{}

func (urlSearchProvider) Search(_ context.Context, query string, limit int) ([]SearchResult, error) {
	value := strings.TrimSpace(query)
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("未配置搜索 Provider；查询必须直接是 http(s) URL，或配置 CONTENTCLOUD_SEARCH_ENDPOINT")
	}
	return []SearchResult{{Title: u.Host + u.Path, URL: u.String(), Source: "url-intake", Rank: 1}}, nil
}

func normalizeResults(results []SearchResult, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > maxSearchResults {
		limit = 10
	}
	out := make([]SearchResult, 0, len(results))
	seen := map[string]struct{}{}
	for _, result := range results {
		result.URL = strings.TrimSpace(result.URL)
		parsed, err := url.Parse(result.URL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		if _, ok := seen[result.URL]; ok {
			continue
		}
		seen[result.URL] = struct{}{}
		if strings.TrimSpace(result.Title) == "" {
			result.Title = parsed.Host + parsed.Path
		}
		if result.Rank <= 0 {
			result.Rank = len(out) + 1
		}
		out = append(out, result)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type FetchResult struct {
	RequestedURL string
	FinalURL     string
	Body         []byte
	MIME         string
	Digest       string
	FetchedAt    time.Time
}

type Fetcher struct {
	Client        *http.Client
	AllowedHosts  []string
	MaxBytes      int64
	Timeout       time.Duration
	AllowPrivate  bool
	RespectRobots bool
	MinInterval   time.Duration

	mu        sync.Mutex
	lastFetch map[string]time.Time
}

func NewDefaultFetcher() *Fetcher {
	allowed := strings.FieldsFunc(os.Getenv("CONTENTCLOUD_FETCH_ALLOWED_HOSTS"), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	return &Fetcher{AllowedHosts: allowed, MaxBytes: defaultFetchLimit, Timeout: 15 * time.Second, RespectRobots: true, MinInterval: time.Second, lastFetch: map[string]time.Time{}}
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (FetchResult, error) {
	requested, err := parseSafeURL(rawURL)
	if err != nil {
		return FetchResult{}, err
	}
	if err := f.validateURL(requested); err != nil {
		return FetchResult{}, err
	}
	if err := f.enforceRate(requested.Hostname(), time.Now().UTC()); err != nil {
		return FetchResult{}, err
	}
	maxBytes := f.MaxBytes
	if maxBytes <= 0 || maxBytes > 100<<20 {
		maxBytes = defaultFetchLimit
	}
	timeout := f.Timeout
	if timeout <= 0 || timeout > 60*time.Second {
		timeout = 15 * time.Second
	}
	client := f.Client
	if client == nil {
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}} // #nosec G402 -- minimum TLS is explicit.
		client = &http.Client{Transport: transport, Timeout: timeout}
	}
	client = cloneClientWithRedirectPolicy(client, f)
	if f.RespectRobots {
		allowed, robotsErr := f.robotsAllowed(ctx, client, requested)
		if robotsErr != nil {
			return FetchResult{}, robotsErr
		}
		if !allowed {
			return FetchResult{}, errors.New("robots.txt 禁止采集该路径")
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requested.String(), nil)
	if err != nil {
		return FetchResult{}, err
	}
	request.Header.Set("Accept", "text/html, text/plain, application/json;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "ContentCloudSourceFetcher/1.0")
	response, err := client.Do(request)
	if err != nil {
		return FetchResult{}, fmt.Errorf("source fetch failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FetchResult{}, fmt.Errorf("source fetch returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return FetchResult{}, errors.New("source response exceeds maximum size")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return FetchResult{}, err
	}
	if int64(len(body)) > maxBytes {
		return FetchResult{}, errors.New("source response exceeds maximum size")
	}
	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(response.Header.Get("Content-Type"), ";", 2)[0]))
	if mime == "" || mime == "application/octet-stream" {
		mime = sniffMIME(body)
	}
	return FetchResult{RequestedURL: requested.String(), FinalURL: response.Request.URL.String(), Body: body, MIME: mime, Digest: digest(body), FetchedAt: time.Now().UTC()}, nil
}

func (f *Fetcher) enforceRate(host string, now time.Time) error {
	if f.MinInterval <= 0 {
		return nil
	}
	host = strings.ToLower(strings.TrimSpace(host))
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastFetch == nil {
		f.lastFetch = map[string]time.Time{}
	}
	if previous := f.lastFetch[host]; !previous.IsZero() && now.Sub(previous) < f.MinInterval {
		return errors.New("source host 请求过于频繁")
	}
	f.lastFetch[host] = now
	return nil
}

func (f *Fetcher) robotsAllowed(ctx context.Context, client *http.Client, target *url.URL) (bool, error) {
	robotsURL := *target
	robotsURL.Path = "/robots.txt"
	robotsURL.RawPath = ""
	robotsURL.RawQuery = ""
	robotsURL.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", "ContentCloudSourceFetcher/1.0")
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("robots.txt request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		return true, nil
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return false, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("robots.txt returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
	if err != nil {
		return false, err
	}
	return pathAllowedByRobots(string(body), target.EscapedPath()), nil
}

func pathAllowedByRobots(body, escapedPath string) bool {
	if escapedPath == "" {
		escapedPath = "/"
	}
	active := false
	bestLength := -1
	allowed := true
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "#", 2)[0])
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			active = value == "*" || strings.EqualFold(value, "ContentCloudSourceFetcher") || strings.EqualFold(value, "ContentCloudSourceFetcher/1.0")
		case "allow", "disallow":
			if !active || value == "" || !strings.HasPrefix(escapedPath, value) || len(value) < bestLength {
				continue
			}
			bestLength = len(value)
			allowed = key == "allow"
		}
	}
	return allowed
}

func cloneClientWithRedirectPolicy(base *http.Client, fetcher *Fetcher) *http.Client {
	copy := *base
	copy.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if err := fetcher.validateURL(request.URL); err != nil {
			return err
		}
		return nil
	}
	return &copy
}

func parseSafeURL(raw string) (*url.URL, error) {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || value.Host == "" || (value.Scheme != "http" && value.Scheme != "https") || value.User != nil {
		return nil, errors.New("source URL 必须是无用户信息的 http(s) 地址")
	}
	return value, nil
}

func (f *Fetcher) validateURL(value *url.URL) error {
	if _, err := parseSafeURL(value.String()); err != nil {
		return err
	}
	host := strings.ToLower(value.Hostname())
	allowed := hostAllowed(host, value.Port(), f.AllowedHosts)
	if allowed || f.AllowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if privateIP(ip) {
			return errors.New("source URL 指向私有或本机地址")
		}
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("source URL 禁止访问 localhost")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("无法解析 source URL 主机: %w", err)
	}
	for _, ip := range ips {
		if privateIP(ip) {
			return errors.New("source URL 解析到私有或本机地址")
		}
	}
	return nil
}

func hostAllowed(host, port string, values []string) bool {
	for _, raw := range values {
		candidate := strings.ToLower(strings.TrimSpace(raw))
		candidate = strings.TrimPrefix(candidate, "*.")
		candidatePort := ""
		if parsedHost, parsedPort, err := net.SplitHostPort(candidate); err == nil {
			candidate = parsedHost
			candidatePort = parsedPort
		}
		if candidatePort != "" && candidatePort != port {
			continue
		}
		if candidate != "" && (host == candidate || strings.HasSuffix(host, "."+candidate)) {
			return true
		}
	}
	return false
}

func privateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func sniffMIME(body []byte) string {
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return "text/html"
	}
	if len(body) > 0 {
		return "text/plain"
	}
	return "application/octet-stream"
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}

func FileName(rawURL, mime string) string {
	u, _ := url.Parse(rawURL)
	name := filepath.Base(u.Path)
	if name == "." || name == "/" || name == "" || name == ".." {
		name = "source"
	}
	if filepath.Ext(name) == "" {
		ext := map[string]string{"text/html": ".html", "application/json": ".json", "text/plain": ".txt"}[mime]
		name += ext
	}
	return name
}
