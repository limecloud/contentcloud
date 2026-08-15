package mediapipeline

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/limecloud/contentcloud/internal/domain"
)

// Seedance25Media is an already-resolved provider input. The resolver is the
// only component allowed to turn ContentCloud Artifact IDs into provider-safe
// URLs or data URLs.
type Seedance25Media struct {
	URL       string
	MediaType string
	Role      string
}

type Seedance25Input struct {
	Prompt        string
	Images        []Seedance25Media
	Videos        []Seedance25Media
	Audios        []Seedance25Media
	Resolution    string
	GenerateAudio *bool
	Watermark     *bool
}

// Seedance25InputResolver resolves a locked ContentCloud input package into
// bounded, provider-safe input. It must never return local filesystem paths.
type Seedance25InputResolver interface {
	Resolve(context.Context, Request, domain.ProviderProfile) (Seedance25Input, error)
}

type Seedance25InputResolverFunc func(context.Context, Request, domain.ProviderProfile) (Seedance25Input, error)

func (f Seedance25InputResolverFunc) Resolve(ctx context.Context, request Request, profile domain.ProviderProfile) (Seedance25Input, error) {
	if f == nil {
		return Seedance25Input{}, domain.Policy("SEEDANCE_INPUT_RESOLVER_REQUIRED", "Seedance 2.5 缺少受控输入解析器", "配置由 ContentCloud 控制面的 Artifact 解析器")
	}
	return f(ctx, request, profile)
}

type Seedance25ProviderConfig struct {
	HTTPProviderConfig
	Model      string
	Resolution string
	Resolver   Seedance25InputResolver
}

// Seedance25Provider implements the ModelArk asynchronous Seedance 2.5 task
// API while reusing HTTPProvider's outbound policy, authentication, limits,
// SSRF checks and streamed download validation.
type Seedance25Provider struct {
	http       *HTTPProvider
	model      string
	resolution string
	resolver   Seedance25InputResolver
}

const seedance25MaxDataImageBytes = 8 << 20

func NewSeedance25Provider(config Seedance25ProviderConfig) (*Seedance25Provider, error) {
	httpProvider, err := NewHTTPProvider(config.HTTPProviderConfig)
	if err != nil {
		return nil, err
	}
	if config.Resolver == nil {
		return nil, domain.Policy("SEEDANCE_INPUT_RESOLVER_REQUIRED", "Seedance 2.5 缺少受控输入解析器", "配置由 ContentCloud 控制面的 Artifact 解析器")
	}
	return &Seedance25Provider{http: httpProvider, model: strings.TrimSpace(config.Model), resolution: strings.TrimSpace(config.Resolution), resolver: config.Resolver}, nil
}

func (p *Seedance25Provider) Validate(request Request, profile domain.ProviderProfile) error {
	if p == nil || p.http == nil || p.resolver == nil {
		return domain.Policy("PROVIDER_ADAPTER_UNAVAILABLE", "Seedance 2.5 Provider 尚未配置完成", "配置 HTTPS 地址、密钥和 Artifact 输入解析器")
	}
	if strings.TrimSpace(request.JobID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.StoryboardSnapshotID) == "" || strings.TrimSpace(request.PromptPackageArtifactID) == "" || request.DurationSeconds < 1 {
		return domain.Invalid("PROVIDER_REQUEST_INVALID", "Seedance 2.5 请求缺少任务、幂等键、快照、提示包或时长")
	}
	if request.DurationSeconds > 30 {
		return domain.Invalid("PROVIDER_DURATION_LIMIT_EXCEEDED", "Seedance 2.5 第一阶段最大时长为 30 秒")
	}
	if request.Mode != "text_to_video" && request.Mode != "image_to_video" {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "Seedance 2.5 第一阶段只支持 text_to_video 和 image_to_video")
	}
	if !contains(profile.Modes, request.Mode) {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "Provider Profile 不支持当前 Seedance 2.5 模式")
	}
	if maxDuration := integerLimit(profile.Limits, "max_duration_seconds"); maxDuration > 0 && request.DurationSeconds > maxDuration {
		return domain.Invalid("PROVIDER_DURATION_LIMIT_EXCEEDED", "请求时长超过 Seedance 2.5 Profile 上限")
	}
	return nil
}

func (p *Seedance25Provider) Estimate(request Request, profile domain.ProviderProfile) (Estimate, error) {
	if err := p.Validate(request, profile); err != nil {
		return Estimate{}, err
	}
	perJob := pricingInt64(profile.Pricing, "per_job_minor")
	perSecond := pricingInt64(profile.Pricing, "per_second_minor")
	minimum := pricingInt64(profile.Pricing, "minimum_minor")
	if perJob < 0 || perSecond < 0 || minimum < 0 || (perJob == 0 && perSecond == 0 && minimum == 0) {
		return Estimate{}, domain.Policy("PROVIDER_PRICING_UNAVAILABLE", "Seedance 2.5 Profile 没有经核验的费用估算", "补充按任务或按秒计费的 Profile 价格")
	}
	cost := perJob + perSecond*int64(request.DurationSeconds)
	if cost < minimum {
		cost = minimum
	}
	if maximum := pricingInt64(profile.Pricing, "max_estimated_minor"); maximum > 0 && cost > maximum {
		return Estimate{}, domain.Policy("PROVIDER_PRICING_INVALID", "Seedance 2.5 估算费用超过 Profile 声明的保守上限", "重新核验服务商价格或缩短视频时长")
	}
	return Estimate{CostMinor: cost, Currency: providerCurrency(profile.Pricing)}, nil
}

func (p *Seedance25Provider) Submit(ctx context.Context, request Request, profile domain.ProviderProfile) (Submission, error) {
	if err := p.Validate(request, profile); err != nil {
		return Submission{}, err
	}
	input, err := p.resolver.Resolve(ctx, request, profile)
	if err != nil {
		return Submission{}, err
	}
	if err := validateSeedanceInput(request, input, profile); err != nil {
		return Submission{}, err
	}
	if err := p.validateSeedanceInputHosts(input); err != nil {
		return Submission{}, err
	}
	content := make([]map[string]any, 0, 1+len(input.Images)+len(input.Videos)+len(input.Audios))
	if prompt := strings.TrimSpace(input.Prompt); prompt != "" {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	for _, media := range input.Images {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": media.URL}, "role": defaultString(media.Role, "reference_image")})
	}
	if len(content) == 0 {
		return Submission{}, domain.Invalid("SEEDANCE_INPUT_EMPTY", "Seedance 2.5 请求没有可提交的提示词或图片")
	}
	model := firstNonEmpty(profile.Model, p.model)
	if model == "" {
		return Submission{}, domain.Invalid("PROVIDER_MODEL_REQUIRED", "Seedance 2.5 请求缺少模型 ID")
	}
	body := map[string]any{
		"model":    model,
		"content":  content,
		"ratio":    request.AspectRatio,
		"duration": request.DurationSeconds,
	}
	if strings.TrimSpace(request.AspectRatio) == "" {
		delete(body, "ratio")
	}
	if resolution := firstNonEmpty(input.Resolution, p.resolution, stringLimit(profile.Limits, "resolution")); resolution != "" {
		body["resolution"] = resolution
	}
	if input.GenerateAudio != nil {
		body["generate_audio"] = *input.GenerateAudio
	}
	if input.Watermark != nil {
		body["watermark"] = *input.Watermark
	}
	var result struct {
		ID                string          `json:"id"`
		ProviderRequestID string          `json:"provider_request_id"`
		RequestID         string          `json:"request_id"`
		Data              json.RawMessage `json:"data"`
	}
	statusCode, requestID, err := p.http.doJSONWithMetadata(ctx, "POST", "/contents/generations/tasks", request.IdempotencyKey, body, &result)
	if err != nil {
		return Submission{}, err
	}
	externalID := strings.TrimSpace(result.ID)
	providerRequestID := firstNonEmpty(result.ProviderRequestID, result.RequestID, requestID)
	if externalID == "" && len(result.Data) > 0 {
		var nestedRequestID string
		externalID, nestedRequestID = seedanceSubmissionMetadata(result.Data)
		providerRequestID = firstNonEmpty(providerRequestID, nestedRequestID)
	}
	if externalID == "" {
		return Submission{}, domain.Invalid("PROVIDER_RESPONSE_INVALID", "ModelArk Seedance 响应缺少任务 ID")
	}
	return Submission{ExternalJobID: externalID, ProviderRequestID: providerRequestID, HTTPStatus: statusCode}, nil
}

func (p *Seedance25Provider) Status(ctx context.Context, externalJobID string, _ domain.ProviderProfile) (Status, error) {
	if err := validateExternalJobID(externalJobID); err != nil {
		return Status{}, err
	}
	var result struct {
		Status        string          `json:"status"`
		State         string          `json:"state"`
		VideoURL      string          `json:"video_url"`
		OutputURL     string          `json:"output_url"`
		Content       json.RawMessage `json:"content"`
		Data          json.RawMessage `json:"data"`
		ActualMinor   int64           `json:"actual_cost_minor"`
		RetryAfterSec int             `json:"retry_after_seconds"`
	}
	statusCode, _, err := p.http.doJSONWithMetadata(ctx, "GET", "/contents/generations/tasks/"+url.PathEscape(strings.TrimSpace(externalJobID)), "", nil, &result)
	if err != nil {
		return Status{}, err
	}
	state := strings.ToLower(strings.TrimSpace(firstNonEmpty(result.Status, result.State)))
	if state == "" && len(result.Data) > 0 {
		var nested struct {
			Status        string          `json:"status"`
			State         string          `json:"state"`
			VideoURL      string          `json:"video_url"`
			OutputURL     string          `json:"output_url"`
			Content       json.RawMessage `json:"content"`
			ActualMinor   int64           `json:"actual_cost_minor"`
			RetryAfterSec int             `json:"retry_after_seconds"`
		}
		if json.Unmarshal(result.Data, &nested) == nil {
			result.Status = nested.Status
			result.State = nested.State
			result.VideoURL = nested.VideoURL
			result.OutputURL = nested.OutputURL
			result.Content = nested.Content
			result.ActualMinor = nested.ActualMinor
			result.RetryAfterSec = nested.RetryAfterSec
			state = strings.ToLower(strings.TrimSpace(firstNonEmpty(result.Status, result.State)))
		}
	}
	if state == "" {
		return Status{}, domain.Invalid("PROVIDER_RESPONSE_INVALID", "ModelArk Seedance 状态响应缺少任务状态")
	}
	switch state {
	case "processing", "in_progress":
		state = "running"
	case "queued", "running", "succeeded", "completed", "failed", "cancelled", "canceled":
	case "expired":
		state = "failed"
	default:
		return Status{}, &domain.Error{Type: "provider", Subtype: "status", Code: "PROVIDER_STATUS_UNKNOWN", Message: "ModelArk Seedance 返回了未知任务状态", Retryable: true}
	}
	outputRef := firstNonEmpty(result.VideoURL, result.OutputURL, seedanceVideoURL(result.Content))
	if outputRef == "" {
		outputRef = seedanceVideoURL(result.Data)
	}
	if (state == "succeeded" || state == "completed") && outputRef == "" {
		return Status{}, &domain.Error{Type: "provider", Subtype: "status", Code: "PROVIDER_OUTPUT_PENDING", Message: "ModelArk Seedance 已成功但输出 URL 尚不可用", Retryable: true}
	}
	progress := 0
	if state == "succeeded" || state == "completed" {
		progress = 100
	}
	if state == "failed" || state == "cancelled" || state == "canceled" {
		progress = 100
	}
	return Status{State: state, Progress: progress, OutputRef: outputRef, ActualMinor: result.ActualMinor, RetryAfterSeconds: result.RetryAfterSec, HTTPStatus: statusCode}, nil
}

func (p *Seedance25Provider) Cancel(ctx context.Context, externalJobID string, _ domain.ProviderProfile) error {
	if err := validateExternalJobID(externalJobID); err != nil {
		return err
	}
	_, _, err := p.http.doJSONWithMetadata(ctx, "DELETE", "/contents/generations/tasks/"+url.PathEscape(strings.TrimSpace(externalJobID)), "", nil, nil)
	return err
}

func (p *Seedance25Provider) Download(ctx context.Context, outputRef string, profile domain.ProviderProfile) (Download, error) {
	return p.http.Download(ctx, outputRef, profile)
}

func (p *Seedance25Provider) OpenDownload(ctx context.Context, outputRef string, profile domain.ProviderProfile) (StreamedDownload, error) {
	return p.http.OpenDownload(ctx, outputRef, profile)
}

func validateSeedanceInput(request Request, input Seedance25Input, profile domain.ProviderProfile) error {
	if utf8.RuneCountInString(input.Prompt) > 32000 {
		return domain.Invalid("SEEDANCE_PROMPT_TOO_LARGE", "Seedance 单镜头提示词超过 32000 字符限制")
	}
	if request.Mode == "text_to_video" && strings.TrimSpace(input.Prompt) == "" {
		return domain.Invalid("SEEDANCE_PROMPT_REQUIRED", "text_to_video 必须提供非空提示词")
	}
	if request.Mode == "image_to_video" && len(input.Images) == 0 {
		return domain.Invalid("SEEDANCE_IMAGE_REQUIRED", "image_to_video 必须提供至少一张图片")
	}
	if max := integerLimit(profile.Limits, "max_reference_images"); max > 0 && len(input.Images) > max {
		return domain.Invalid("PROVIDER_REFERENCE_LIMIT_EXCEEDED", "图片引用数量超过 Seedance 2.5 Profile 上限")
	}
	if len(input.Images) > 30 {
		return domain.Invalid("PROVIDER_REFERENCE_LIMIT_EXCEEDED", "Seedance 2.5 第一阶段最多接受 30 张图片")
	}
	if len(input.Videos) > 0 || len(input.Audios) > 0 {
		return domain.Invalid("PROVIDER_MODE_UNSUPPORTED", "Seedance 2.5 第一阶段暂不接受视频或音频引用")
	}
	for _, media := range input.Images {
		if media.MediaType != "image/jpeg" && media.MediaType != "image/png" && media.MediaType != "image/webp" {
			return domain.Invalid("SEEDANCE_IMAGE_MIME_INVALID", "Seedance 图片引用必须是 JPEG、PNG 或 WebP")
		}
		if err := validateSeedanceURL(media.URL, true); err != nil {
			return err
		}
		if strings.HasPrefix(strings.TrimSpace(media.URL), "data:") {
			metadata, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(media.URL), "data:"), ",")
			mediaType := strings.ToLower(strings.Split(metadata, ";")[0])
			if mediaType != strings.ToLower(strings.TrimSpace(media.MediaType)) {
				return domain.Invalid("SEEDANCE_IMAGE_MIME_INVALID", "Seedance 图片 data URL 媒体类型与 Artifact 不一致")
			}
		}
	}
	return nil
}

func validateSeedanceURL(raw string, image bool) error {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || raw == "" || strings.ContainsAny(raw, "\r\n") {
		return domain.Invalid("SEEDANCE_INPUT_URL_INVALID", "Seedance 输入引用不是有效 URL")
	}
	if parsed.Scheme == "data" {
		if !image || !strings.Contains(raw, ";base64,") {
			return domain.Policy("SEEDANCE_INPUT_URL_BLOCKED", "Seedance 只允许图片 data URL，且必须使用 base64", "使用受控 HTTPS 图片 URL")
		}
		metadata, encoded, _ := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
		parts := strings.Split(strings.ToLower(metadata), ";")
		if len(parts) == 0 || (parts[0] != "image/jpeg" && parts[0] != "image/png" && parts[0] != "image/webp") || encoded == "" {
			return domain.Invalid("SEEDANCE_INPUT_URL_INVALID", "Seedance 图片 data URL 的媒体类型或内容无效")
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return domain.Invalid("SEEDANCE_INPUT_URL_INVALID", "Seedance 图片 data URL 不是有效 Base64")
		}
		if len(decoded) == 0 || len(decoded) > seedance25MaxDataImageBytes {
			return domain.Invalid("SEEDANCE_INPUT_SIZE_INVALID", "Seedance 图片 data URL 为空或超过 8 MB 限制")
		}
		return nil
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return domain.Policy("SEEDANCE_INPUT_URL_BLOCKED", "Seedance 输入只允许无凭据 HTTPS URL", "使用受控 HTTPS 图片 URL")
	}
	return nil
}

func (p *Seedance25Provider) validateSeedanceInputHosts(input Seedance25Input) error {
	if p == nil || p.http == nil {
		return domain.Policy("PROVIDER_ADAPTER_UNAVAILABLE", "Seedance 2.5 Provider 尚未配置完成", "配置服务商 HTTP 适配器")
	}
	for _, media := range input.Images {
		parsed, err := url.Parse(strings.TrimSpace(media.URL))
		if err != nil || parsed.Scheme != "https" {
			continue
		}
		if len(p.http.allowedHosts) == 0 {
			return domain.Policy("SEEDANCE_INPUT_HOST_NOT_ALLOWED", "Seedance 图片 HTTPS 地址未配置域名白名单", "使用 data:image Base64 或配置受控图片域名")
		}
		if _, ok := p.http.allowedHosts[strings.ToLower(parsed.Hostname())]; !ok {
			return domain.Policy("SEEDANCE_INPUT_HOST_NOT_ALLOWED", "Seedance 图片 HTTPS 地址不在域名白名单中", "使用已核验的图片下载域名")
		}
	}
	return nil
}

func validateExternalJobID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "/?#") {
		return domain.Invalid("PROVIDER_EXTERNAL_ID_INVALID", "服务商外部任务标识无效")
	}
	return nil
}

func seedanceSubmissionMetadata(raw json.RawMessage) (string, string) {
	var value struct {
		ID                string `json:"id"`
		TaskID            string `json:"task_id"`
		ExternalJobID     string `json:"external_job_id"`
		ProviderRequestID string `json:"provider_request_id"`
		RequestID         string `json:"request_id"`
	}
	if json.Unmarshal(raw, &value) != nil {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) == nil && len(values) > 0 {
			return seedanceSubmissionMetadata(values[0])
		}
		return "", ""
	}
	return firstNonEmpty(value.ID, value.TaskID, value.ExternalJobID), firstNonEmpty(value.ProviderRequestID, value.RequestID)
}

func seedanceVideoURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) == nil {
		for _, key := range []string{"video_url", "output_url", "url"} {
			if candidate, ok := value[key].(string); ok {
				return strings.TrimSpace(candidate)
			}
			if candidate, ok := value[key].(map[string]any); ok {
				if nested, ok := candidate["url"].(string); ok {
					return strings.TrimSpace(nested)
				}
			}
		}
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) == nil {
		for _, candidate := range values {
			if value := seedanceVideoURL(candidate); value != "" {
				return value
			}
		}
	}
	return ""
}

func pricingInt64(values map[string]any, key string) int64 {
	switch value := values[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		return 0
	}
}

func stringLimit(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

var _ Adapter = (*Seedance25Provider)(nil)
var _ StreamingDownloader = (*Seedance25Provider)(nil)
