package connector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

const maxHTTPResponseBytes = 32 << 20

type HTTPConfig struct {
	Endpoint   string
	Token      string
	HTTPClient *http.Client
	AllowHTTP  bool
}

// HTTPAdapter is the provider-neutral boundary for CMS, DAM, PIM, CRM,
// social-listening and Agent-SaaS connectors. AuthorizationRef remains an
// opaque reference; deployment credentials authenticate this adapter hop.
type HTTPAdapter struct {
	endpoint *url.URL
	token    string
	client   *http.Client
}

type httpRecord struct {
	ExternalID string         `json:"external_id"`
	Version    string         `json:"version"`
	Title      string         `json:"title"`
	SourceURL  string         `json:"source_url,omitempty"`
	MIME       string         `json:"mime,omitempty"`
	BodyBase64 string         `json:"body_base64,omitempty"`
	Deleted    bool           `json:"deleted"`
	DeletedAt  *time.Time     `json:"deleted_at,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Rights     map[string]any `json:"rights"`
	Metadata   map[string]any `json:"metadata"`
}

type httpPullResult struct {
	Records    []httpRecord `json:"records"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

func NewHTTPAdapter(config HTTPConfig) (*HTTPAdapter, error) {
	endpoint, err := url.Parse(strings.TrimSpace(config.Endpoint))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && !(config.AllowHTTP && endpoint.Scheme == "http")) {
		return nil, domain.Invalid("CONNECTOR_ENDPOINT_INVALID", "Connector Endpoint 必须是 HTTPS URL")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPAdapter{endpoint: endpoint, token: strings.TrimSpace(config.Token), client: client}, nil
}

func (a *HTTPAdapter) Pull(ctx context.Context, request PullRequest) (PullResult, error) {
	payload := map[string]any{
		"binding_id": request.Binding.ID, "tenant_id": request.Binding.TenantID,
		"project_id": request.Binding.ProjectID, "authorization_ref": request.Binding.AuthorizationRef,
		"region": request.Binding.Region, "cursor": request.Cursor, "limit": request.Limit,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PullResult{}, err
	}
	target := a.endpoint.ResolveReference(&url.URL{Path: strings.TrimRight(a.endpoint.Path, "/") + "/v1/pull"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return PullResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	response, err := a.client.Do(req)
	if err != nil {
		return PullResult{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBytes+1))
	if err != nil {
		return PullResult{}, err
	}
	if len(data) > maxHTTPResponseBytes {
		return PullResult{}, domain.Invalid("CONNECTOR_RESPONSE_TOO_LARGE", "Connector 响应超过 32 MB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PullResult{}, fmt.Errorf("connector HTTP %d", response.StatusCode)
	}
	var wire httpPullResult
	if err := json.Unmarshal(data, &wire); err != nil {
		return PullResult{}, domain.Invalid("CONNECTOR_RESPONSE_INVALID", "Connector 响应不是有效 JSON")
	}
	result := PullResult{NextCursor: wire.NextCursor, HasMore: wire.HasMore, Records: make([]Record, 0, len(wire.Records))}
	for _, value := range wire.Records {
		var body []byte
		if value.BodyBase64 != "" {
			body, err = base64.StdEncoding.DecodeString(value.BodyBase64)
			if err != nil {
				return PullResult{}, domain.Invalid("CONNECTOR_BODY_INVALID", "Connector 正文不是有效 Base64")
			}
		}
		result.Records = append(result.Records, Record{ExternalID: value.ExternalID, Version: value.Version, Title: value.Title, SourceURL: value.SourceURL, MIME: value.MIME, Body: body, Deleted: value.Deleted, DeletedAt: value.DeletedAt, UpdatedAt: value.UpdatedAt, Rights: value.Rights, Metadata: value.Metadata})
	}
	return result, nil
}
