package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

// CreateProviderProfileInput is the platform-owned description of a provider
// adapter. Credentials are deliberately absent: provider profiles are public
// capability facts, while secrets belong to tenant bindings.
type CreateProviderProfileInput struct {
	ProviderID      string         `json:"provider_id"`
	Version         string         `json:"version"`
	Digest          string         `json:"digest"`
	AdapterVersion  string         `json:"adapter_version"`
	Model           string         `json:"model"`
	Region          string         `json:"region"`
	Modes           []string       `json:"modes"`
	InputMediaTypes []string       `json:"input_media_types"`
	OutputMediaType string         `json:"output_media_type"`
	Limits          map[string]any `json:"limits"`
	DataRetention   string         `json:"data_retention"`
	Pricing         map[string]any `json:"pricing"`
	VerifiedAt      time.Time      `json:"verified_at"`
	ExpiresAt       time.Time      `json:"expires_at"`
}

type ConfigureProviderBindingInput struct {
	ProfileVersion     string `json:"profile_version"`
	State              string `json:"state"`
	CredentialRef      string `json:"credential_ref"`
	EgressPolicy       string `json:"egress_policy"`
	MonthlyBudgetMinor int64  `json:"monthly_budget_minor"`
	MaxJobCostMinor    int64  `json:"max_job_cost_minor"`
	MaxConcurrency     int    `json:"max_concurrency"`
	MaxRetries         int    `json:"max_retries"`
}

type providerProfileAdminStore interface {
	SaveProviderProfile(context.Context, domain.ProviderProfile) error
	ProviderProfiles(context.Context, string) ([]domain.ProviderProfile, error)
}

func (s *Service) providerProfileAdminRepository() (providerProfileAdminStore, error) {
	repository, ok := s.store.(providerProfileAdminStore)
	if !ok {
		return nil, domain.Policy("PROVIDER_PROFILE_STORE_UNAVAILABLE", "当前存储未启用 Provider Profile 管理能力", "使用支持 Provider Profile 管理的服务端存储")
	}
	return repository, nil
}

func (s *Service) CreateProviderProfile(ctx context.Context, actor Actor, input CreateProviderProfileInput, requestID string) (domain.ProviderProfile, error) {
	if !actor.PlatformAdmin {
		return domain.ProviderProfile{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以创建 Provider Profile", "联系系统管理员配置平台权限")
	}
	now := s.now().UTC()
	value := domain.ProviderProfile{
		ProviderID: strings.ToLower(strings.TrimSpace(input.ProviderID)), Version: strings.TrimSpace(input.Version),
		Digest: strings.ToLower(strings.TrimSpace(input.Digest)), AdapterVersion: strings.TrimSpace(input.AdapterVersion),
		Model: strings.TrimSpace(input.Model), Region: strings.TrimSpace(input.Region), Modes: append([]string{}, input.Modes...),
		InputMediaTypes: append([]string{}, input.InputMediaTypes...), OutputMediaType: strings.TrimSpace(input.OutputMediaType),
		Limits: input.Limits, DataRetention: strings.TrimSpace(input.DataRetention), Pricing: input.Pricing,
		Status: "draft", VerifiedAt: input.VerifiedAt.UTC(), ExpiresAt: input.ExpiresAt.UTC(),
	}
	if value.VerifiedAt.IsZero() || value.ExpiresAt.IsZero() || value.VerifiedAt.After(now) {
		return domain.ProviderProfile{}, domain.Invalid("PROVIDER_PROFILE_VERIFICATION_INVALID", "Provider Profile 的核验时间必须存在且不能晚于当前时间")
	}
	if !value.ExpiresAt.After(now) {
		return domain.ProviderProfile{}, domain.Invalid("PROVIDER_PROFILE_EXPIRED", "Provider Profile 有效期必须晚于当前时间")
	}
	if err := value.Validate(); err != nil {
		return domain.ProviderProfile{}, err
	}
	if err := s.store.CreateProviderProfile(ctx, value); err != nil {
		return domain.ProviderProfile{}, err
	}
	s.audit(ctx, actor, "", "provider.profile_created", "provider_profile", value.ProviderID+":"+value.Version, requestID, map[string]any{"provider_id": value.ProviderID, "version": value.Version, "digest": value.Digest, "status": value.Status})
	return value, nil
}

func (s *Service) PublishProviderProfile(ctx context.Context, actor Actor, providerID, version, requestID string) (domain.ProviderProfile, error) {
	if !actor.PlatformAdmin {
		return domain.ProviderProfile{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以发布 Provider Profile", "联系系统管理员配置平台权限")
	}
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	version = strings.TrimSpace(version)
	value, err := s.store.ProviderProfile(ctx, providerID, version)
	if err != nil {
		return domain.ProviderProfile{}, err
	}
	now := s.now().UTC()
	if value.Status == "published" {
		if value.VerifiedAt.After(now) || !value.ExpiresAt.After(now) {
			return domain.ProviderProfile{}, domain.Invalid("PROVIDER_PROFILE_EXPIRED", "Provider Profile 尚未核验或已过期，不能继续使用")
		}
		return value, nil
	}
	if value.Status != "draft" {
		return domain.ProviderProfile{}, domain.Policy("PROVIDER_PROFILE_NOT_PUBLISHABLE", "只有 draft Provider Profile 可以发布", "创建新的 Profile 版本")
	}
	if value.VerifiedAt.After(now) || !value.ExpiresAt.After(now) {
		return domain.ProviderProfile{}, domain.Invalid("PROVIDER_PROFILE_EXPIRED", "Provider Profile 尚未核验或已过期，不能发布")
	}
	value.Status = "published"
	repository, repositoryErr := s.providerProfileAdminRepository()
	if repositoryErr != nil {
		return domain.ProviderProfile{}, repositoryErr
	}
	if err := repository.SaveProviderProfile(ctx, value); err != nil {
		return domain.ProviderProfile{}, err
	}
	s.audit(ctx, actor, "", "provider.profile_published", "provider_profile", value.ProviderID+":"+value.Version, requestID, map[string]any{"provider_id": value.ProviderID, "version": value.Version, "digest": value.Digest})
	return value, nil
}

func (s *Service) ProviderProfiles(ctx context.Context, actor Actor, providerID string) ([]domain.ProviderProfile, error) {
	if !actor.PlatformAdmin {
		return nil, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以查看 Provider Profile 管理列表", "联系系统管理员配置平台权限")
	}
	repository, err := s.providerProfileAdminRepository()
	if err != nil {
		return nil, err
	}
	return repository.ProviderProfiles(ctx, strings.ToLower(strings.TrimSpace(providerID)))
}

// AvailableProviderProfiles is the tenant-facing, credential-free view used
// to select a binding version. Draft and expired platform records stay hidden.
func (s *Service) AvailableProviderProfiles(ctx context.Context, actor Actor, providerID string) ([]domain.ProviderProfile, error) {
	if strings.TrimSpace(actor.TenantID) == "" {
		return nil, domain.Policy("TENANT_REQUIRED", "当前会话没有可用租户", "切换到有效租户后重试")
	}
	repository, err := s.providerProfileAdminRepository()
	if err != nil {
		return nil, err
	}
	values, err := repository.ProviderProfiles(ctx, strings.ToLower(strings.TrimSpace(providerID)))
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := make([]domain.ProviderProfile, 0, len(values))
	for _, value := range values {
		if value.Status == "published" && !value.VerifiedAt.After(now) && value.ExpiresAt.After(now) {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Service) ProviderProfile(ctx context.Context, actor Actor, providerID, version string) (domain.ProviderProfile, error) {
	if !actor.PlatformAdmin {
		return domain.ProviderProfile{}, domain.Policy("PLATFORM_ADMIN_REQUIRED", "只有平台管理员可以查看 Provider Profile", "联系系统管理员配置平台权限")
	}
	return s.store.ProviderProfile(ctx, strings.ToLower(strings.TrimSpace(providerID)), strings.TrimSpace(version))
}

func (s *Service) ConfigureProviderBinding(ctx context.Context, actor Actor, tenantID, providerID string, input ConfigureProviderBindingInput, requestID string) (domain.ProviderBinding, error) {
	tenantID = strings.TrimSpace(tenantID)
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if tenantID == "" || providerID == "" {
		return domain.ProviderBinding{}, domain.Invalid("PROVIDER_BINDING_SCOPE_INVALID", "Provider Binding 缺少租户或服务商标识")
	}
	if !actor.PlatformAdmin && (actor.Role != "tenant_admin" || actor.TenantID != tenantID) {
		return domain.ProviderBinding{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以配置当前租户的 Provider Binding", "联系租户管理员")
	}
	profileVersion := strings.TrimSpace(input.ProfileVersion)
	profile, err := s.store.ProviderProfile(ctx, providerID, profileVersion)
	if err != nil {
		return domain.ProviderBinding{}, err
	}
	now := s.now().UTC()
	if profile.Status != "published" || !profile.ExpiresAt.After(now) || profile.VerifiedAt.After(now) {
		return domain.ProviderBinding{}, domain.Policy("PROVIDER_PROFILE_NOT_ACTIVE", "只能绑定已发布且仍在有效期内的 Provider Profile", "先核验并发布对应 Profile 版本")
	}
	state := strings.ToLower(strings.TrimSpace(input.State))
	if state == "" {
		state = "active"
	}
	egressPolicy := strings.TrimSpace(input.EgressPolicy)
	credentialRef := strings.TrimSpace(input.CredentialRef)
	if egressPolicy == "" {
		return domain.ProviderBinding{}, domain.Invalid("PROVIDER_EGRESS_POLICY_INVALID", "Provider Binding 必须声明出口策略")
	}
	if state == "active" && providerID != "fake" && !validProviderCredentialRef(credentialRef) {
		return domain.ProviderBinding{}, domain.Invalid("PROVIDER_CREDENTIAL_REF_INVALID", "启用 Provider 必须保存 SecretRef、VaultRef 或 EnvRef，不能保存明文 API Key")
	}
	if credentialRef != "" && !validProviderCredentialRef(credentialRef) {
		return domain.ProviderBinding{}, domain.Invalid("PROVIDER_CREDENTIAL_REF_INVALID", "Provider 凭据只能保存 SecretRef、VaultRef 或 EnvRef，不能保存明文 API Key")
	}
	maxConcurrency := input.MaxConcurrency
	if maxConcurrency == 0 {
		maxConcurrency = 1
	}
	value := domain.ProviderBinding{TenantID: tenantID, ProviderID: providerID, ProfileVersion: profileVersion, State: state, CredentialRef: credentialRef, EgressPolicy: egressPolicy, MonthlyBudgetMinor: input.MonthlyBudgetMinor, MaxJobCostMinor: input.MaxJobCostMinor, MaxConcurrency: maxConcurrency, MaxRetries: input.MaxRetries, UpdatedBy: actor.UserID, UpdatedAt: now}
	if err := value.Validate(); err != nil {
		return domain.ProviderBinding{}, err
	}
	if err := s.store.SaveProviderBinding(ctx, value); err != nil {
		return domain.ProviderBinding{}, err
	}
	s.audit(ctx, actor, "", "provider.binding_configured", "provider_binding", tenantID+":"+providerID, requestID, map[string]any{"tenant_id": tenantID, "provider_id": providerID, "profile_version": profileVersion, "state": state, "egress_policy": egressPolicy, "monthly_budget_minor": value.MonthlyBudgetMinor, "max_job_cost_minor": value.MaxJobCostMinor, "max_concurrency": value.MaxConcurrency, "max_retries": value.MaxRetries})
	return value, nil
}

func (s *Service) ProviderBindingForActor(ctx context.Context, actor Actor, tenantID, providerID string) (domain.ProviderBinding, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !actor.PlatformAdmin && (actor.Role != "tenant_admin" || actor.TenantID != tenantID) {
		return domain.ProviderBinding{}, domain.Policy("ROLE_DENIED", "只有租户管理员可以查看当前租户的 Provider Binding", "联系租户管理员")
	}
	return s.store.ProviderBinding(ctx, tenantID, strings.ToLower(strings.TrimSpace(providerID)))
}

func validProviderCredentialRef(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return false
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"secret://", "vault://", "env://"} {
		if strings.HasPrefix(lower, prefix) {
			return len(value) > len(prefix)
		}
	}
	return false
}
