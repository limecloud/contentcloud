package application

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	"github.com/google/uuid"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	"github.com/limecloud/contentcloud/internal/catalog/environment"
	"github.com/limecloud/contentcloud/internal/experience/projection"
	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

var bootstrapChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
var machineIDPattern = regexp.MustCompile(`^mach_[A-Za-z0-9_-]{32}$`)

type StartBootstrapAuthorizationInput struct {
	SessionID     string `json:"session_id"`
	CodeChallenge string `json:"code_challenge"`
	Platform      string `json:"platform"`
	Arch          string `json:"arch"`
	CLIVersion    string `json:"cli_version"`
}

type StartBootstrapAuthorizationResult struct {
	AttemptID       string    `json:"attempt_id"`
	AttemptToken    string    `json:"attempt_token"`
	UserCode        string    `json:"user_code"`
	SupportCode     string    `json:"support_code"`
	VerificationURL string    `json:"verification_url"`
	ExpiresAt       time.Time `json:"expires_at"`
	IntervalSeconds int       `json:"interval_seconds"`
}

type CompleteBootstrapAuthorizationInput struct {
	AttemptToken string             `json:"attempt_token"`
	CodeVerifier string             `json:"code_verifier"`
	Device       ConnectDeviceInput `json:"device"`
}

type BootstrapAuthorizationView struct {
	Attempt workspacedomain.BootstrapAttempt `json:"attempt"`
	Session workspacedomain.ConnectSession   `json:"session"`
}

func (s *WorkspaceService) BootstrapAuthorizationView(ctx context.Context, actor Actor, projectID, attemptID string) (BootstrapAuthorizationView, error) {
	if !canManage(actor.Role) {
		return BootstrapAuthorizationView{}, fault.Policy("ROLE_DENIED", "当前角色不能查看初始化授权", "联系项目负责人")
	}
	attempt, err := s.workspace.BootstrapAttempt(ctx, actor.TenantID, attemptID)
	if err != nil || attempt.ProjectID != projectID {
		if err == nil {
			err = fault.NotFound("初始化授权")
		}
		return BootstrapAuthorizationView{}, err
	}
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return BootstrapAuthorizationView{}, err
	}
	session, err := s.ConnectSession(ctx, actor, attempt.ConnectSessionID)
	if err != nil {
		return BootstrapAuthorizationView{}, err
	}
	return BootstrapAuthorizationView{Attempt: attempt, Session: session}, nil
}

func (s *WorkspaceService) StartBootstrapAuthorization(ctx context.Context, baseURL string, in StartBootstrapAuthorizationInput) (StartBootstrapAuthorizationResult, error) {
	if _, err := uuid.Parse(strings.TrimSpace(in.SessionID)); err != nil || !bootstrapChallengePattern.MatchString(in.CodeChallenge) {
		return StartBootstrapAuthorizationResult{}, fault.Invalid("BOOTSTRAP_AUTHORIZATION_INPUT_INVALID", "初始化授权缺少会话标识（session_id）或有效的校验值（code_challenge）")
	}
	if err := projection.ValidateServerBase(baseURL); err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	attemptToken, attemptTokenHash, err := idgen.NewOpaqueToken("cbt_", 32)
	if err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	codeSeed, _, err := idgen.NewOpaqueToken("", 8)
	if err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	userCode := compactBootstrapCode(codeSeed, 8)
	now := s.now().UTC()
	attempt := workspacedomain.BootstrapAttempt{
		ID: idgen.New(), AttemptTokenHash: attemptTokenHash, CodeChallenge: in.CodeChallenge,
		UserCode: userCode, State: "pending", CreatedAt: now, UpdatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute),
	}
	attempt.SupportCode = bootstrapSupportCode(attempt.ID)
	attempt, err = s.workspace.CreateBootstrapAttemptForSession(ctx, in.SessionID, attempt, now)
	if err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	initial := workspacedomain.BootstrapProgressEvent{
		SchemaVersion: workspacedomain.BootstrapSchemaVersion, Sequence: 1, OccurredAt: now,
		Stage: "authorizing", Status: "needs_action", ActionID: "open.browser.authorization",
		Facts: map[string]any{"platform": defaultString(in.Platform, runtime.GOOS), "arch": defaultString(in.Arch, runtime.GOARCH), "cli_version": in.CLIVersion},
	}
	if err := workspacedomain.ValidateBootstrapEvent(initial); err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	if _, err := s.workspace.AppendBootstrapProgress(ctx, attemptTokenHash, initial, now); err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	verificationURL, err := projection.BuildStudioConnect(baseURL, attempt.ConnectSessionID)
	if err != nil {
		return StartBootstrapAuthorizationResult{}, err
	}
	return StartBootstrapAuthorizationResult{AttemptID: attempt.ID, AttemptToken: attemptToken, UserCode: attempt.UserCode, SupportCode: attempt.SupportCode, VerificationURL: verificationURL, ExpiresAt: attempt.ExpiresAt, IntervalSeconds: 3}, nil
}

func (s *WorkspaceService) ApproveBootstrapAuthorization(ctx context.Context, actor Actor, sessionID, attemptID, requestID string) (workspacedomain.BootstrapAttempt, error) {
	return s.approveBootstrapAuthorization(ctx, actor, sessionID, attemptID, requestID, false)
}

func (s *WorkspaceService) approveBootstrapAuthorization(ctx context.Context, actor Actor, sessionID, attemptID, requestID string, allowCustomerRoles bool) (workspacedomain.BootstrapAttempt, error) {
	if !canManage(actor.Role) && !(allowCustomerRoles && canConnectStudioClient(actor.Role)) {
		return workspacedomain.BootstrapAttempt{}, fault.Policy("ROLE_DENIED", "当前角色不能批准初始化授权", "联系项目负责人")
	}
	attempt, err := s.workspace.BootstrapAttempt(ctx, actor.TenantID, attemptID)
	if err != nil {
		return attempt, err
	}
	if attempt.ConnectSessionID != sessionID {
		return attempt, fault.NotFound("初始化授权")
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, attempt.ProjectID); err != nil {
		return attempt, err
	}
	attempt, err = s.workspace.ApproveBootstrapAttempt(ctx, actor.TenantID, sessionID, attemptID, actor.UserID, s.now().UTC())
	if err == nil {
		s.audit(ctx, actor, attempt.ProjectID, "bootstrap.authorization.approved", "bootstrap_attempt", attempt.ID, requestID, map[string]any{"support_code": attempt.SupportCode})
	}
	return attempt, err
}

func (s *WorkspaceService) DenyBootstrapAuthorization(ctx context.Context, actor Actor, sessionID, attemptID, requestID string) (workspacedomain.BootstrapAttempt, error) {
	return s.denyBootstrapAuthorization(ctx, actor, sessionID, attemptID, requestID, false)
}

func (s *WorkspaceService) denyBootstrapAuthorization(ctx context.Context, actor Actor, sessionID, attemptID, requestID string, allowCustomerRoles bool) (workspacedomain.BootstrapAttempt, error) {
	if !canManage(actor.Role) && !(allowCustomerRoles && canConnectStudioClient(actor.Role)) {
		return workspacedomain.BootstrapAttempt{}, fault.Policy("ROLE_DENIED", "当前角色不能拒绝初始化授权", "联系项目负责人")
	}
	attempt, err := s.workspace.BootstrapAttempt(ctx, actor.TenantID, attemptID)
	if err != nil || attempt.ConnectSessionID != sessionID {
		if err == nil {
			err = fault.NotFound("初始化授权")
		}
		return attempt, err
	}
	attempt, err = s.workspace.DenyBootstrapAttempt(ctx, actor.TenantID, sessionID, attemptID, actor.UserID, s.now().UTC())
	if err == nil {
		s.audit(ctx, actor, attempt.ProjectID, "bootstrap.authorization.denied", "bootstrap_attempt", attempt.ID, requestID, map[string]any{"support_code": attempt.SupportCode})
	}
	return attempt, err
}

func (s *WorkspaceService) CompleteBootstrapAuthorization(ctx context.Context, in CompleteBootstrapAuthorizationInput) (ConnectDeviceResult, error) {
	if !strings.HasPrefix(in.AttemptToken, "cbt_") || len(in.CodeVerifier) < 43 || len(in.CodeVerifier) > 128 {
		return ConnectDeviceResult{}, fault.Invalid("BOOTSTRAP_AUTHORIZATION_INVALID", "初始化授权凭据格式错误")
	}
	tokenHash := idgen.TokenHash(in.AttemptToken)
	attempt, err := s.workspace.BootstrapAttemptByTokenHash(ctx, tokenHash)
	if err != nil {
		return ConnectDeviceResult{}, fault.E("authentication", "bootstrap", "BOOTSTRAP_AUTHORIZATION_INVALID", "初始化授权无效", 3)
	}
	now := s.now().UTC()
	if now.After(attempt.ExpiresAt) {
		return ConnectDeviceResult{}, fault.E("authentication", "bootstrap", "BOOTSTRAP_AUTHORIZATION_EXPIRED", "初始化授权已过期", 3)
	}
	if attempt.State == "pending" {
		pending := fault.Conflict("BOOTSTRAP_AUTHORIZATION_PENDING", "等待用户在浏览器中确认这台电脑")
		pending.Retryable = true
		return ConnectDeviceResult{}, pending
	}
	if attempt.State == "denied" {
		return ConnectDeviceResult{}, fault.Policy("BOOTSTRAP_AUTHORIZATION_DENIED", "用户拒绝了这次初始化授权", "重新发起初始化后再确认")
	}
	challenge := bootstrapCodeChallenge(in.CodeVerifier)
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(attempt.CodeChallenge)) != 1 {
		return ConnectDeviceResult{}, fault.E("authentication", "bootstrap", "BOOTSTRAP_VERIFIER_INVALID", "初始化授权校验值不匹配", 3)
	}
	in.Device.MachineID = strings.TrimSpace(in.Device.MachineID)
	if !machineIDPattern.MatchString(in.Device.MachineID) {
		return ConnectDeviceResult{}, fault.Invalid("DEVICE_MACHINE_ID_INVALID", "设备缺少本地生成的稳定 machine_id")
	}
	deviceToken, deviceTokenHash, err := idgen.NewOpaqueToken("dt_", 32)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	workspaceToken, workspaceTokenHash, err := idgen.NewOpaqueToken("wt_", 32)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	var issuedManifest *environment.Manifest
	if s.environmentControl != nil {
		contentTypes, capabilityErr := s.app.Identity.TenantContentTypes(ctx, attempt.TenantID)
		if capabilityErr != nil {
			return ConnectDeviceResult{}, capabilityErr
		}
		manifest, issueErr := s.environmentControl.Issue(attempt.ProjectID, contentTypes, now)
		if issueErr != nil {
			return ConnectDeviceResult{}, issueErr
		}
		issuedManifest = &manifest
	}
	deviceInput := in.Device
	device := workspacedomain.Device{ID: idgen.New(), MachineID: deviceInput.MachineID, DisplayName: defaultString(deviceInput.DisplayName, deviceInput.Hostname), Hostname: deviceInput.Hostname, Platform: defaultString(deviceInput.Platform, runtime.GOOS), Arch: defaultString(deviceInput.Arch, runtime.GOARCH), Version: deviceInput.Version, TokenHash: deviceTokenHash, CredentialVersion: 1, CredentialRotatedAt: now, Capabilities: append([]catalogdomain.Capability{}, deviceInput.Capabilities...), LastSeenAt: now}
	workspace := workspacedomain.WorkspaceBinding{ID: idgen.New(), TemplateID: localworkspace.TemplateID, TemplateVersion: localworkspace.TemplateVersion, Targets: []string{}, CredentialHash: workspaceTokenHash, Status: "active", InitializedAt: now, LastSeenAt: now}
	session, consumed, device, workspace, err := s.workspace.ConsumeBootstrapAttempt(ctx, tokenHash, device, workspace, now)
	if err != nil {
		return ConnectDeviceResult{}, err
	}
	device.TokenHash = ""
	s.audit(ctx, Actor{UserID: device.OwnerUserID, TenantID: device.TenantID, Type: "device", DeviceID: device.ID}, session.ProjectID, "device.connected", "device", device.ID, "", map[string]any{"platform": device.Platform, "bootstrap_attempt_id": consumed.ID})
	result := ConnectDeviceResult{Device: device, DeviceToken: deviceToken, WorkspaceID: workspace.ID, WorkspaceToken: workspaceToken, ProjectID: session.ProjectID, BootstrapAttemptID: consumed.ID, EnvironmentManifest: issuedManifest}
	return result, nil
}

func (s *WorkspaceService) AppendBootstrapProgress(ctx context.Context, attemptToken string, event workspacedomain.BootstrapProgressEvent) (workspacedomain.BootstrapProgressEvent, error) {
	if !strings.HasPrefix(attemptToken, "cbt_") {
		return event, fault.Invalid("BOOTSTRAP_ATTEMPT_TOKEN_INVALID", "初始化尝试凭据格式错误")
	}
	now := s.now().UTC()
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.OccurredAt.Before(now.Add(-30*time.Minute)) || event.OccurredAt.After(now.Add(5*time.Minute)) {
		return event, fault.Invalid("BOOTSTRAP_PROGRESS_TIME_INVALID", "初始化进度的发生时间（occurred_at）超出允许范围")
	}
	if err := workspacedomain.ValidateBootstrapEvent(event); err != nil {
		return event, err
	}
	return s.workspace.AppendBootstrapProgress(ctx, idgen.TokenHash(attemptToken), event, now)
}

func (s *WorkspaceService) CompleteBootstrapAttempt(ctx context.Context, attemptToken, state string) (workspacedomain.BootstrapAttempt, error) {
	if !strings.HasPrefix(attemptToken, "cbt_") {
		return workspacedomain.BootstrapAttempt{}, fault.Invalid("BOOTSTRAP_ATTEMPT_TOKEN_INVALID", "初始化尝试凭据格式错误")
	}
	return s.workspace.CompleteBootstrapAttempt(ctx, idgen.TokenHash(attemptToken), state, s.now().UTC())
}

func (s *WorkspaceService) UploadBootstrapDiagnostic(ctx context.Context, actor Actor, binding workspacedomain.WorkspaceBinding, summary workspacedomain.BootstrapDiagnosticSummary) (workspacedomain.BootstrapDiagnostic, error) {
	if actor.Type != "workspace" || binding.ProjectID == "" {
		return workspacedomain.BootstrapDiagnostic{}, fault.Policy("WORKSPACE_AUTH_REQUIRED", "上传诊断摘要需要本地工作区凭据", "在已初始化的本地工作区中重试")
	}
	if err := workspacedomain.ValidateBootstrapDiagnostic(summary); err != nil {
		return workspacedomain.BootstrapDiagnostic{}, err
	}
	attempt, err := s.workspace.BootstrapAttempt(ctx, actor.TenantID, summary.AttemptID)
	if err != nil || attempt.ProjectID != binding.ProjectID {
		if err == nil {
			err = fault.NotFound("初始化尝试")
		}
		return workspacedomain.BootstrapDiagnostic{}, err
	}
	body, err := json.Marshal(summary)
	if err != nil {
		return workspacedomain.BootstrapDiagnostic{}, err
	}
	if len(body) > 256<<10 {
		return workspacedomain.BootstrapDiagnostic{}, fault.Invalid("BOOTSTRAP_DIAGNOSTIC_TOO_LARGE", "诊断摘要超过 256 KiB")
	}
	sum := sha256.Sum256(body)
	diagnostic := workspacedomain.BootstrapDiagnostic{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: binding.ProjectID, AttemptID: attempt.ID, SupportCode: attempt.SupportCode, Digest: "sha256:" + hex.EncodeToString(sum[:]), ByteSize: int64(len(body)), Summary: summary, CreatedAt: s.now().UTC()}
	diagnostic, err = s.workspace.CreateBootstrapDiagnostic(ctx, diagnostic)
	if err != nil {
		return diagnostic, err
	}
	s.audit(ctx, actor, binding.ProjectID, "bootstrap.diagnostic.uploaded", "bootstrap_diagnostic", diagnostic.ID, "", map[string]any{"attempt_id": attempt.ID, "support_code": attempt.SupportCode, "digest": diagnostic.Digest, "byte_size": diagnostic.ByteSize})
	return diagnostic, nil
}

func (s *WorkspaceService) BootstrapActionCatalog() (workspacedomain.BootstrapActionCatalog, error) {
	catalog := workspacedomain.BootstrapActions()
	if err := workspacedomain.ValidateBootstrapActionCatalog(catalog); err != nil {
		return catalog, err
	}
	return catalog, nil
}

func compactBootstrapCode(seed string, length int) string {
	value := strings.ToUpper(strings.NewReplacer("-", "", "_", "").Replace(seed))
	if len(value) < length {
		value += strings.Repeat("X", length-len(value))
	}
	value = value[:length]
	return value[:4] + "-" + value[4:]
}

func bootstrapSupportCode(attemptID string) string {
	sum := sha256.Sum256([]byte(attemptID))
	value := strings.ToUpper(hex.EncodeToString(sum[:4]))
	return fmt.Sprintf("CC-%s-%s", value[:4], value[4:])
}

func bootstrapCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
