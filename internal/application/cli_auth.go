package application

import (
	"context"
	"strings"
	"time"

	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type StartDeviceLoginResult struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

type CompleteDeviceLoginResult struct {
	AccessToken string                `json:"access_token"`
	ExpiresAt   time.Time             `json:"expires_at"`
	Tenant      identitydomain.Tenant `json:"tenant"`
}

type SwitchCLITenantResult struct {
	AccessToken string                `json:"access_token"`
	ExpiresAt   time.Time             `json:"expires_at"`
	Tenant      identitydomain.Tenant `json:"tenant"`
}

func (s *IdentityService) StartUserDeviceLogin(ctx context.Context, baseURL string) (StartDeviceLoginResult, error) {
	deviceCode, deviceHash, err := idgen.NewOpaqueToken("cdc_", 32)
	if err != nil {
		return StartDeviceLoginResult{}, err
	}
	codeSeed, _, err := idgen.NewOpaqueToken("", 6)
	if err != nil {
		return StartDeviceLoginResult{}, err
	}
	userCode := strings.ToUpper(strings.ReplaceAll(codeSeed, "-", ""))
	if len(userCode) > 8 {
		userCode = userCode[:8]
	}
	userCode = userCode[:4] + "-" + userCode[4:]
	now := s.now().UTC()
	flow := workspacedomain.UserDeviceFlow{ID: idgen.New(), DeviceCodeHash: deviceHash, UserCode: userCode, State: "pending", ExpiresAt: now.Add(10 * time.Minute)}
	if err := s.workspace.CreateUserDeviceFlow(ctx, flow); err != nil {
		return StartDeviceLoginResult{}, err
	}
	verificationURL := strings.TrimRight(baseURL, "/") + "/device-auth?code=" + userCode
	return StartDeviceLoginResult{DeviceCode: deviceCode, UserCode: userCode, VerificationURL: verificationURL, ExpiresAt: flow.ExpiresAt, IntervalSeconds: 3}, nil
}

func (s *IdentityService) ApproveUserDeviceLogin(ctx context.Context, actor Actor, userCode string) (workspacedomain.UserDeviceFlow, error) {
	flow, err := s.workspace.UserDeviceFlowByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)))
	if err != nil {
		return flow, err
	}
	now := s.now().UTC()
	if flow.State != "pending" || now.After(flow.ExpiresAt) {
		return flow, fault.Conflict("DEVICE_CODE_INVALID", "设备授权码已过期或已使用")
	}
	flow.UserID = actor.UserID
	flow.TenantID = actor.TenantID
	flow.State = "approved"
	flow.ApprovedAt = &now
	if err := s.workspace.SaveUserDeviceFlow(ctx, flow); err != nil {
		return flow, err
	}
	s.audit(ctx, actor, "", "cli_login.approved", "user_device_flow", flow.ID, "", map[string]any{})
	return flow, nil
}

func (s *IdentityService) CompleteUserDeviceLogin(ctx context.Context, deviceCode string) (CompleteDeviceLoginResult, error) {
	if !strings.HasPrefix(deviceCode, "cdc_") {
		return CompleteDeviceLoginResult{}, fault.Invalid("DEVICE_CODE_INVALID", "设备授权码格式错误")
	}
	flow, err := s.workspace.UserDeviceFlowByCodeHash(ctx, idgen.TokenHash(deviceCode))
	if err != nil {
		return CompleteDeviceLoginResult{}, fault.E("authentication", "device_flow", "DEVICE_CODE_INVALID", "设备授权码无效", 3)
	}
	now := s.now().UTC()
	if now.After(flow.ExpiresAt) {
		return CompleteDeviceLoginResult{}, fault.E("authentication", "device_flow", "DEVICE_CODE_EXPIRED", "设备授权码已过期", 3)
	}
	if flow.State == "pending" {
		pending := fault.Conflict("AUTHORIZATION_PENDING", "等待用户在浏览器中确认登录")
		pending.Retryable = true
		pending.Hint = "完成网页确认后再次运行原命令"
		return CompleteDeviceLoginResult{}, pending
	}
	if flow.State != "approved" || flow.ConsumedAt != nil {
		return CompleteDeviceLoginResult{}, fault.Conflict("DEVICE_CODE_CONSUMED", "设备授权码已使用")
	}
	plain, tokenHash, err := idgen.NewOpaqueToken("ct_", 32)
	if err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	expires := now.Add(30 * 24 * time.Hour)
	token := workspacedomain.CLIToken{ID: idgen.New(), UserID: flow.UserID, TenantID: flow.TenantID, TokenHash: tokenHash, ExpiresAt: expires}
	if err := s.workspace.CreateCLIToken(ctx, token); err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	flow.State = "consumed"
	flow.ConsumedAt = &now
	if err := s.workspace.SaveUserDeviceFlow(ctx, flow); err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	tenant, err := s.Tenant(ctx, Actor{UserID: flow.UserID, TenantID: flow.TenantID, Type: "user"})
	if err != nil {
		return CompleteDeviceLoginResult{}, err
	}
	return CompleteDeviceLoginResult{AccessToken: plain, ExpiresAt: expires, Tenant: tenant}, nil
}

func (s *IdentityService) CLITokenActor(ctx context.Context, token string) (Actor, identitydomain.User, error) {
	if !strings.HasPrefix(token, "ct_") {
		return Actor{}, identitydomain.User{}, fault.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效", 3)
	}
	credential, err := s.workspace.CLITokenByHash(ctx, idgen.TokenHash(token))
	if err != nil {
		return Actor{}, identitydomain.User{}, fault.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效或已过期", 3)
	}
	user, err := s.identity.UserByID(ctx, credential.UserID)
	if err != nil {
		return Actor{}, identitydomain.User{}, err
	}
	membership, err := s.identity.Membership(ctx, credential.TenantID, credential.UserID)
	if err != nil {
		return Actor{}, identitydomain.User{}, err
	}
	return Actor{UserID: user.ID, TenantID: credential.TenantID, Role: membership.Role, Type: "user"}, user, nil
}

func (s *IdentityService) LogoutCLI(ctx context.Context, token string) error {
	if !strings.HasPrefix(token, "ct_") {
		return fault.E("authentication", "cli_token", "CLI_TOKEN_INVALID", "CLI 用户凭据无效", 3)
	}
	return s.workspace.RevokeCLIToken(ctx, idgen.TokenHash(token), s.now().UTC())
}

func (s *IdentityService) SwitchCLITenant(ctx context.Context, token, tenantID string) (SwitchCLITenantResult, error) {
	actor, _, err := s.CLITokenActor(ctx, token)
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	membership, err := s.identity.Membership(ctx, tenantID, actor.UserID)
	if err != nil {
		return SwitchCLITenantResult{}, fault.NotFound("租户")
	}
	plain, tokenHash, err := idgen.NewOpaqueToken("ct_", 32)
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)
	credential := workspacedomain.CLIToken{ID: idgen.New(), UserID: actor.UserID, TenantID: tenantID, TokenHash: tokenHash, ExpiresAt: expiresAt}
	if err := s.workspace.CreateCLIToken(ctx, credential); err != nil {
		return SwitchCLITenantResult{}, err
	}
	if err := s.workspace.RevokeCLIToken(ctx, idgen.TokenHash(token), now); err != nil {
		return SwitchCLITenantResult{}, err
	}
	tenant, err := s.Tenant(ctx, Actor{UserID: actor.UserID, TenantID: tenantID, Role: membership.Role, Type: "user"})
	if err != nil {
		return SwitchCLITenantResult{}, err
	}
	s.audit(ctx, Actor{UserID: actor.UserID, TenantID: tenantID, Role: membership.Role, Type: "user"}, "", "cli_token.tenant_switched", "cli_token", credential.ID, "", map[string]any{"from_tenant_id": actor.TenantID})
	return SwitchCLITenantResult{AccessToken: plain, ExpiresAt: expiresAt, Tenant: tenant}, nil
}
