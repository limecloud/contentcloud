package app_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/store/memory"
)

func TestProviderProfileLifecycleRequiresPlatformAdminAndExplicitPublish(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	now := time.Now().UTC().Truncate(time.Second)
	input := providerProfileInput(now)
	tenantAdmin := app.Actor{UserID: "tenant-admin", TenantID: "tenant-1", Role: "tenant_admin", Type: "user"}
	if _, err := service.CreateProviderProfile(ctx, tenantAdmin, input, "request-1"); !hasProviderCode(err, "PLATFORM_ADMIN_REQUIRED") {
		t.Fatalf("tenant profile create error = %v", err)
	}

	platform := app.Actor{UserID: "platform-admin", TenantID: "platform-tenant", Type: "user", PlatformAdmin: true}
	created, err := service.CreateProviderProfile(ctx, platform, input, "request-2")
	if err != nil || created.Status != "draft" {
		t.Fatalf("created profile = %#v, err=%v", created, err)
	}
	if _, err := service.ConfigureProviderBinding(ctx, tenantAdmin, tenantAdmin.TenantID, input.ProviderID, app.ConfigureProviderBindingInput{ProfileVersion: input.Version, CredentialRef: "secret://providers/modelark", EgressPolicy: "provider-only"}, "request-3"); !hasProviderCode(err, "PROVIDER_PROFILE_NOT_ACTIVE") {
		t.Fatalf("draft binding error = %v", err)
	}
	if _, err := service.PublishProviderProfile(ctx, tenantAdmin, input.ProviderID, input.Version, "request-4"); !hasProviderCode(err, "PLATFORM_ADMIN_REQUIRED") {
		t.Fatalf("tenant profile publish error = %v", err)
	}
	published, err := service.PublishProviderProfile(ctx, platform, input.ProviderID, input.Version, "request-5")
	if err != nil || published.Status != "published" {
		t.Fatalf("published profile = %#v, err=%v", published, err)
	}
	profiles, err := service.ProviderProfiles(ctx, platform, input.ProviderID)
	if err != nil || len(profiles) != 1 || profiles[0].Status != "published" {
		t.Fatalf("profiles = %#v, err=%v", profiles, err)
	}
	available, err := service.AvailableProviderProfiles(ctx, tenantAdmin, input.ProviderID)
	if err != nil || len(available) != 1 || available[0].Digest != input.Digest {
		t.Fatalf("available profiles = %#v, err=%v", available, err)
	}
}

func TestProviderBindingRequiresSecretRefAndDoesNotExposeCredential(t *testing.T) {
	ctx := t.Context()
	service := app.New(memory.New(), nil)
	now := time.Now().UTC().Truncate(time.Second)
	input := providerProfileInput(now)
	platform := app.Actor{UserID: "platform-admin", TenantID: "platform-tenant", Type: "user", PlatformAdmin: true}
	if _, err := service.CreateProviderProfile(ctx, platform, input, "create"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PublishProviderProfile(ctx, platform, input.ProviderID, input.Version, "publish"); err != nil {
		t.Fatal(err)
	}
	tenantAdmin := app.Actor{UserID: "tenant-admin", TenantID: "tenant-1", Role: "tenant_admin", Type: "user"}
	bad := app.ConfigureProviderBindingInput{ProfileVersion: input.Version, CredentialRef: "sk-live-secret", EgressPolicy: "provider-only"}
	if _, err := service.ConfigureProviderBinding(ctx, tenantAdmin, tenantAdmin.TenantID, input.ProviderID, bad, "bad"); !hasProviderCode(err, "PROVIDER_CREDENTIAL_REF_INVALID") {
		t.Fatalf("plain credential error = %v", err)
	}
	configured, err := service.ConfigureProviderBinding(ctx, tenantAdmin, tenantAdmin.TenantID, input.ProviderID, app.ConfigureProviderBindingInput{ProfileVersion: input.Version, CredentialRef: "secret://providers/modelark", EgressPolicy: "provider-only", MaxConcurrency: 2, MaxRetries: 1}, "good")
	if err != nil || configured.State != "active" || configured.MaxConcurrency != 2 {
		t.Fatalf("configured binding = %#v, err=%v", configured, err)
	}
	body, err := json.Marshal(configured)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret://providers/modelark") || strings.Contains(string(body), "credential_ref") {
		t.Fatalf("binding response leaked credential: %s", body)
	}
	viewer, err := service.ProviderBindingForActor(ctx, app.Actor{UserID: "viewer", TenantID: tenantAdmin.TenantID, Role: "viewer", Type: "user"}, tenantAdmin.TenantID, input.ProviderID)
	if err != nil || viewer.CredentialRef != "secret://providers/modelark" {
		t.Fatalf("same-tenant read = %#v, err=%v", viewer, err)
	}
	updated, err := service.ConfigureProviderBinding(ctx, tenantAdmin, tenantAdmin.TenantID, input.ProviderID, app.ConfigureProviderBindingInput{ProfileVersion: input.Version, EgressPolicy: "provider-only", MonthlyBudgetMinor: 2000}, "update")
	if err != nil || updated.CredentialRef != "secret://providers/modelark" || updated.MonthlyBudgetMinor != 2000 {
		t.Fatalf("credential-preserving update = %#v, err=%v", updated, err)
	}
	if _, err := service.ProviderBindingForActor(ctx, app.Actor{UserID: "other", TenantID: "tenant-2", Role: "tenant_admin", Type: "user"}, tenantAdmin.TenantID, input.ProviderID); !hasProviderCode(err, "ROLE_DENIED") {
		t.Fatalf("cross-tenant read error = %v", err)
	}
}

func providerProfileInput(now time.Time) app.CreateProviderProfileInput {
	return app.CreateProviderProfileInput{ProviderID: "modelark-seedance25", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), AdapterVersion: "modelark/1.0.0", Model: "dreamina-seedance-2-5-260628", Region: "cn-beijing", Modes: []string{"text_to_video", "image_to_video"}, InputMediaTypes: []string{"image/png", "application/json"}, OutputMediaType: "video/mp4", Limits: map[string]any{"max_duration_seconds": 30}, DataRetention: "provider_policy", Pricing: map[string]any{"currency": "CNY", "per_second_minor": 2}, VerifiedAt: now.Add(-time.Hour), ExpiresAt: now.Add(24 * time.Hour)}
}

func hasProviderCode(err error, code string) bool {
	var value *domain.Error
	return errors.As(err, &value) && value.Code == code
}
