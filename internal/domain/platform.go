package domain

import "time"

const (
	ContentTypeVideoScript   = "video_script"
	ContentTypeWeChatArticle = "wechat_article"
)

var optionalTenantContentTypes = map[string]struct{}{
	ContentTypeWeChatArticle: {},
}

// TenantContentCapability is the server-owned entitlement for an optional content type.
// Video scripts are always enabled and therefore are not persisted as a mutable row.
type TenantContentCapability struct {
	TenantID    string    `json:"tenant_id"`
	ContentType string    `json:"content_type"`
	Enabled     bool      `json:"enabled"`
	UpdatedBy   string    `json:"updated_by"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func ValidOptionalTenantContentType(value string) bool {
	_, ok := optionalTenantContentTypes[value]
	return ok
}

func ValidTenantContentType(value string) bool {
	return value == ContentTypeVideoScript || ValidOptionalTenantContentType(value)
}

func EnabledTenantContentTypes(values []TenantContentCapability) []string {
	result := []string{ContentTypeVideoScript}
	for _, value := range values {
		if value.Enabled && ValidOptionalTenantContentType(value.ContentType) {
			result = append(result, value.ContentType)
		}
	}
	return result
}

// PlatformTenant is the platform operator projection for one tenant.
type PlatformTenant struct {
	Tenant
	MemberCount    int        `json:"member_count"`
	ProjectCount   int        `json:"project_count"`
	DeviceCount    int        `json:"device_count"`
	ActiveRunCount int        `json:"active_run_count"`
	ContentTypes   []string   `json:"content_types"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
}

type PlatformUserMembership struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Role       string `json:"role"`
	Status     string `json:"status"`
}

// PlatformUser deliberately excludes credentials and other authentication material.
type PlatformUser struct {
	ID              string                   `json:"id"`
	Email           string                   `json:"email"`
	DisplayName     string                   `json:"display_name"`
	VerifiedAt      *time.Time               `json:"verified_at,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	IsPlatformAdmin bool                     `json:"is_platform_admin"`
	Memberships     []PlatformUserMembership `json:"memberships"`
}

type PlatformCounts struct {
	Tenants       int `json:"tenants"`
	ActiveTenants int `json:"active_tenants"`
	Users         int `json:"users"`
	Projects      int `json:"projects"`
	OnlineDevices int `json:"online_devices"`
	ActiveRuns    int `json:"active_runs"`
}

type PlatformOverview struct {
	Counts      PlatformCounts   `json:"counts"`
	Tenants     []PlatformTenant `json:"tenants"`
	Users       []PlatformUser   `json:"users"`
	GeneratedAt time.Time        `json:"generated_at"`
}
