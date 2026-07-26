package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type StartDeviceLoginResult struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

type CompleteDeviceLoginResult struct {
	AccessToken string        `json:"access_token"`
	ExpiresAt   time.Time     `json:"expires_at"`
	Tenant      domain.Tenant `json:"tenant"`
}

type SwitchCLITenantResult struct {
	AccessToken string        `json:"access_token"`
	ExpiresAt   time.Time     `json:"expires_at"`
	Tenant      domain.Tenant `json:"tenant"`
}

func (s *Service) StartUserDeviceLogin(ctx context.Context, baseURL string) (StartDeviceLoginResult, error) {
	deviceCode, deviceHash, err := domain.NewOpaqueToken("cdc_", 32)
	if err != nil {
		return StartDeviceLoginResult{}, err
	}
	codeSeed, _, err := domain.NewOpaqueToken("", 6)
	if err != nil {
		return StartDeviceLoginResult{}, err
	}
	userCode := strings.ToUpper(strings.ReplaceAll(codeSeed, "-", ""))
	if len(userCode) > 8 {
		userCode = userCode[:8]
	}
	userCode = userCode[:4] + "-" + userCode[4:]
	now := s.now().UTC()
	flow := domain.UserDeviceFlow{ID: domain.NewID(), DeviceCodeHash: deviceHash, UserCode: userCode, State: "pending", ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.store.CreateUserDeviceFlow(ctx, flow); err != nil {
		return StartDeviceLoginResult{}, err
	}
	verificationURL := strings.TrimRight(baseURL, "/") + "/device-auth?code=" + userCode
	return StartDeviceLoginResult{DeviceCode: deviceCode, UserCode: userCode, VerificationURL: verificationURL, ExpiresAt: flow.ExpiresAt, IntervalSeconds: 3}, nil
}

func (s *Service) ApproveUserDeviceLogin(ctx context.Context, actor Actor, userCode string) (domain.UserDeviceFlow, error) {
	flow, err := s.store.UserDeviceFlowByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)))
	if err != nil {
		return flow, err
	}
	now := s.now().UTC()
	if flow.State != "pending" || now.After(flow.ExpiresAt) {
		return flow, domain.Conflict("DEVICE_CODE_INVALID", "设备授权码已过期或已使用")
	}
	flow.UserID = actor.UserID
	flow.TenantID = actor.TenantID
	flow.State = "approved"
	flow.ApprovedAt = &now
	if err := s.store.SaveUserDeviceFlow(ctx, flow); err != nil {
		return flow, err
	}
	s.audit(ctx, actor, "", "cli_login.approved", "user_device_flow", flow.ID, "", map[string]any{})
	return flow, nil
}

func (s *Service) CompleteUserDeviceLogin(ctx context.Context, deviceCode string) (CompleteDeviceLoginResult, error) {
	if !strings.HasPrefix(deviceCode, "cdc_") {
		return CompleteDeviceLoginResult{}, domain.Invalid("DEVICE_CODE_INVALID", "设备授权码格式错误")
	}
	flow, err := s.store.UserDeviceFlowByCodeHash(ctx, domain.TokenHash(deviceCode))
	if err != nil {
		return CompleteDeviceLoginResult{}, domain.E("authentication", "device_flow", "DEVICE_CODE_INVALID", "设备授权码无效", 3)
	}
	now := s.now().UTC()
	if now.After(flow.ExpiresAt) {
		return CompleteDeviceLoginResult{}, domain.E("authentication", "device_flow", "DEVICE_CODE_EXPIRED", "设备授权码已过期", 3)
	}
	if flow.State == "pending" {
		pending := domain.Conflict("AUTHORIZATION_PENDING", "等待用户在浏览器中确认登录")
		pending.Retryable = true
		pending.Hint = "完成网页确认后再次运行原命令"
		return CompleteDeviceLoginResult{}, pending
	}
	if flow.State != "approved" || flow.ConsumedAt != nil {
		return CompleteDeviceLoginResult{}, domain.Conflict("DEVICE_CODE_CONSUMED", "设备授权码已使用")
	}
	plain, tokenHash, err := domain.NewOpaqueToken("ct_", 32)
	if err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	expires := now.Add(30 * 24 * time.Hour)
	token := domain.CLIToken{ID: domain.NewID(), UserID: flow.UserID, TenantID: flow.TenantID, TokenHash: tokenHash, ExpiresAt: expires}
	if err := s.store.CreateCLIToken(ctx, token); err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	flow.State = "consumed"
	flow.ConsumedAt = &now
	if err := s.store.SaveUserDeviceFlow(ctx, flow); err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	tenant, err := s.Tenant(ctx, Actor{UserID: flow.UserID, TenantID: flow.TenantID, Type: "user"})
	if err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	return CompleteDeviceLoginResult{AccessToken: plain, ExpiresAt: expires, Tenant: tenant}, nil
}

func (s *Service) CLITokenActor(ctx context.Context, token string) (Actor, domain.User, error) {
	if !strings.HasPrefix(token, "ct_") {
		return Actor{}, domain.User{}, domain.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效", 3)
	}
	credential, err := s.store.CLITokenByHash(ctx, domain.TokenHash(token))
	if err != nil {
		return Actor{}, domain.User{}, domain.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效或已过期", 3)
	}
	user, err := s.store.UserByID(ctx, credential.UserID)
	if err != nil {
		return Actor{}, domain.User{}, err
	}
	membership, err := s.store.Membership(ctx, credential.TenantID, credential.UserID)
	if err != nil {
		return Actor{}, domain.User{}, err
	}
	return Actor{UserID: user.ID, TenantID: credential.TenantID, Role: membership.Role, Type: "user"}, user, nil
}

func (s *Service) LogoutCLI(ctx context.Context, token string) error {
	if !strings.HasPrefix(token, "ct_") {
		return domain.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效", 3)
	}
	return s.store.RevokeCLIToken(ctx, domain.TokenHash(token), s.now().UTC())
}

func (s *Service) SwitchCLITenant(ctx context.Context, token, tenantID string) (SwitchCLITenantResult, error) {
	actor, _, err := s.CLITokenActor(ctx, token)
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	membership, err := s.store.Membership(ctx, tenantID, actor.UserID)
	if err != nil {
		return SwitchCLITenantResult{}, domain.NotFound("租户")
	}
	plain, tokenHash, err := domain.NewOpaqueToken("ct_", 32)
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)
	credential := domain.CLIToken{ID: domain.NewID(), UserID: actor.UserID, TenantID: tenantID, TokenHash: tokenHash, ExpiresAt: expiresAt}
	if err := s.store.CreateCLIToken(ctx, credential); err != nil {
		return SwitchCLITenantResult{}, err
	}
	if err := s.store.RevokeCLIToken(ctx, domain.TokenHash(token), now); err != nil {
		return SwitchCLITenantResult{}, err
	}
	tenant, err := s.Tenant(ctx, Actor{UserID: actor.UserID, TenantID: tenantID, Role: membership.Role, Type: "user"})
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	s.audit(ctx, Actor{UserID: actor.UserID, TenantID: tenantID, Role: membership.Role, Type: "user"}, "", "cli_token.tenant_switched", "cli_token", credential.ID, "", map[string]any{"from_tenant_id": actor.TenantID})
	return SwitchCLITenantResult{AccessToken: plain, ExpiresAt: expiresAt, Tenant: tenant}, nil
}
