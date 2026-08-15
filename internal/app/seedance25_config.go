package app

import (
	"os"
	"strings"

	"github.com/limecloud/contentcloud/internal/blob"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/mediapipeline"
	"github.com/limecloud/contentcloud/internal/store"
)

const Seedance25ProviderID = "modelark-seedance25"

// Seedance25ProviderFromEnv is intentionally opt-in. A worker without the
// deployment SecretRef keeps the provider unregistered and cannot send an
// accidental external request.
func Seedance25ProviderFromEnv(st store.Store, blobs blob.Store) (*mediapipeline.Seedance25Provider, error) {
	apiKey := strings.TrimSpace(os.Getenv("CONTENTCLOUD_SEEDANCE25_API_KEY"))
	if apiKey == "" {
		return nil, nil
	}
	baseURL := strings.TrimSpace(os.Getenv("CONTENTCLOUD_SEEDANCE25_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://ark.ap-southeast.bytepluses.com/api/v3"
	}
	allowedHosts := splitSeedanceHosts(os.Getenv("CONTENTCLOUD_SEEDANCE25_ALLOWED_HOSTS"))
	if len(allowedHosts) == 0 {
		return nil, domain.Policy("SEEDANCE_ALLOWED_HOSTS_REQUIRED", "Seedance 2.5 生产配置必须声明 API 和输出下载域名白名单", "设置 CONTENTCLOUD_SEEDANCE25_ALLOWED_HOSTS")
	}
	return mediapipeline.NewSeedance25Provider(mediapipeline.Seedance25ProviderConfig{
		HTTPProviderConfig: mediapipeline.HTTPProviderConfig{BaseURL: baseURL, AuthToken: apiKey, AllowedHosts: allowedHosts, MaxDownloadBytes: 100 << 20, UserAgent: "contentcloud-seedance25-worker"},
		Model:              strings.TrimSpace(os.Getenv("CONTENTCLOUD_SEEDANCE25_MODEL")),
		Resolution:         strings.TrimSpace(os.Getenv("CONTENTCLOUD_SEEDANCE25_RESOLUTION")),
		Resolver:           NewSeedance25ArtifactResolver(st, blobs),
	})
}

func splitSeedanceHosts(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.ToLower(strings.TrimSpace(part)); value != "" {
			values = append(values, value)
		}
	}
	return values
}
