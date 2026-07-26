package domain

import "time"

// PlatformTenant is the platform operator projection for one tenant.
type PlatformTenant struct {
	Tenant
	MemberCount    int        `json:"member_count"`
	ProjectCount   int        `json:"project_count"`
	DeviceCount    int        `json:"device_count"`
	ActiveRunCount int        `json:"active_run_count"`
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
