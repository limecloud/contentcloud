package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/limecloud/contentcloud/internal/apiclient"
	"github.com/limecloud/contentcloud/internal/app"
	"github.com/limecloud/contentcloud/internal/bootstrapcheck"
	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/localconfig"
)

type bootstrapProgressReporter struct {
	serverURL    string
	attemptID    string
	attemptToken string
	supportCode  string
	sequence     int64
}

func validateBootstrapSession(value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return domain.Invalid("BOOTSTRAP_SESSION_INVALID", "--session 必须是 Web 创建的有效 ConnectSession ID")
	}
	return nil
}

func (r *Root) withBootstrapPrerequisites(ctx context.Context, plan bootstrapPlan, sessionID string) (bootstrapPlan, error) {
	options := bootstrapcheck.Options{Directory: plan.Workspace.Root, ServerURL: plan.ServerURL}
	report := bootstrapcheck.Run(ctx, options)
	if r.bootstrapCheckHook != nil {
		report = r.bootstrapCheckHook(ctx, options)
	}
	if err := bootstrapcheck.ValidateReport(report); err != nil {
		return plan, domain.E("internal", "bootstrap_preflight", "BOOTSTRAP_PREFLIGHT_INVALID", err.Error(), 1)
	}
	plan.SessionID = sessionID
	plan.AuthorizationMode = "browser_device"
	plan.Prerequisites = &report
	if !report.OK {
		plan.State = "blocked"
		if report.FirstFailure != nil {
			plan.BlockingReasons = append(plan.BlockingReasons, report.FirstFailure.CheckID+": "+report.FirstFailure.ErrorCode)
		}
	}
	planID, err := bootstrapPlanID(plan)
	if err != nil {
		return plan, err
	}
	plan.PlanID = planID
	return plan, nil
}

func (r *Root) authorizeBootstrapDevice(ctx context.Context, sessionID, name string, _ *bootstrapcheck.Report) (localconfig.Config, app.ConnectDeviceResult, *bootstrapProgressReporter, error) {
	if r.bootstrapAuthorizeHook != nil {
		return r.bootstrapAuthorizeHook(ctx, sessionID, name)
	}
	cfg, err := localconfig.Load()
	if err != nil {
		return cfg, app.ConnectDeviceResult{}, nil, err
	}
	server := r.resolveServer(cfg)
	if err := validateBootstrapServer(server); err != nil {
		return cfg, app.ConnectDeviceResult{}, nil, err
	}
	verifier, err := newBootstrapVerifier()
	if err != nil {
		return cfg, app.ConnectDeviceResult{}, nil, err
	}
	challenge := bootstrapVerifierChallenge(verifier)
	var authorization app.StartBootstrapAuthorizationResult
	err = apiclient.New(server, "").Dispatch(ctx, "bootstrap.authorization.start", app.StartBootstrapAuthorizationInput{SessionID: sessionID, CodeChallenge: challenge, Platform: runtime.GOOS, Arch: runtime.GOARCH, CLIVersion: Version}, &authorization)
	if err != nil {
		return cfg, app.ConnectDeviceResult{}, nil, err
	}
	verificationURL, err := sameOriginBootstrapURL(server, authorization.VerificationURL)
	if err != nil {
		return cfg, app.ConnectDeviceResult{}, nil, err
	}
	progress := &bootstrapProgressReporter{serverURL: server, attemptID: authorization.AttemptID, attemptToken: authorization.AttemptToken, supportCode: authorization.SupportCode, sequence: 1}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", verificationURL).Start()
	}
	hostname, _ := os.Hostname()
	device := app.ConnectDeviceInput{DisplayName: name, Hostname: hostname, Platform: runtime.GOOS, Arch: runtime.GOARCH, Version: Version, Capabilities: builtinCapabilities()}
	interval := time.Duration(authorization.IntervalSeconds) * time.Second
	if interval < time.Second {
		interval = 3 * time.Second
	}
	for {
		var result app.ConnectDeviceResult
		err = apiclient.New(server, "").Dispatch(ctx, "bootstrap.authorization.complete", app.CompleteBootstrapAuthorizationInput{AttemptToken: authorization.AttemptToken, CodeVerifier: verifier, Device: device}, &result)
		if err == nil {
			if err := localconfig.SaveDeviceToken(result.Device.ID, result.DeviceToken); err != nil {
				return cfg, result, progress, domain.E("credential", "secure_store", "CREDENTIAL_STORE_FAILED", err.Error(), 3)
			}
			if err := localconfig.SaveWorkspaceToken(result.WorkspaceID, result.WorkspaceToken); err != nil {
				return cfg, result, progress, domain.E("credential", "secure_store", "WORKSPACE_CREDENTIAL_STORE_FAILED", err.Error(), 3)
			}
			cfg.ServerURL = server
			cfg.UpsertDaemonBinding(localconfig.DaemonBinding{ServerURL: server, DeviceID: result.Device.ID, Workspaces: []localconfig.DaemonWorkspace{{WorkspaceID: result.WorkspaceID, ProjectID: result.ProjectID}}})
			if err := localconfig.Save(cfg); err != nil {
				return cfg, result, progress, err
			}
			progress.append(ctx, "authorizing", "passed", "", "", "")
			return cfg, result, progress, nil
		}
		var domainError *domain.Error
		if !errors.As(err, &domainError) || domainError.Code != "BOOTSTRAP_AUTHORIZATION_PENDING" {
			return cfg, result, progress, err
		}
		if time.Now().After(authorization.ExpiresAt) {
			return cfg, result, progress, domain.E("authentication", "bootstrap", "BOOTSTRAP_AUTHORIZATION_EXPIRED", "初始化授权已过期", 3)
		}
		select {
		case <-ctx.Done():
			return cfg, result, progress, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func sameOriginBootstrapURL(serverURL, verificationURL string) (string, error) {
	server, serverErr := url.Parse(serverURL)
	target, targetErr := url.Parse(verificationURL)
	if serverErr != nil || targetErr != nil || target.User != nil || target.Scheme != server.Scheme || !strings.EqualFold(target.Host, server.Host) || target.Path == "" {
		return "", domain.Policy("BOOTSTRAP_VERIFICATION_URL_INVALID", "服务端返回了非同源的浏览器授权地址", "检查 Content Work OS 反向代理的 Host 配置后重试")
	}
	return target.String(), nil
}

func (p *bootstrapProgressReporter) append(ctx context.Context, stage, status, checkID, errorCode, actionID string) {
	if p == nil || p.attemptToken == "" {
		return
	}
	event := domain.BootstrapProgressEvent{SchemaVersion: domain.BootstrapSchemaVersion, Sequence: p.sequence + 1, OccurredAt: time.Now().UTC(), Stage: stage, Status: status, CheckID: checkID, ErrorCode: errorCode, ActionID: actionID, Facts: map[string]any{}}
	var stored domain.BootstrapProgressEvent
	if err := apiclient.New(p.serverURL, "").Dispatch(ctx, "bootstrap.progress.append", map[string]any{"attempt_token": p.attemptToken, "event": event}, &stored); err == nil {
		p.sequence = event.Sequence
	}
}

func (p *bootstrapProgressReporter) complete(ctx context.Context, state string) {
	if p == nil || p.attemptToken == "" {
		return
	}
	var attempt domain.BootstrapAttempt
	_ = apiclient.New(p.serverURL, "").Dispatch(ctx, "bootstrap.attempt.complete", map[string]any{"attempt_token": p.attemptToken, "state": state}, &attempt)
}

func (r *Root) bootstrapDiagnosticCommand() *cobra.Command {
	var attemptID string
	var upload, acceptUpload bool
	command := &cobra.Command{
		Use:   "diagnostics [directory]",
		Args:  cobra.MaximumNArgs(1),
		Short: "预览已脱敏的初始化诊断摘要",
		RunE: func(command *cobra.Command, args []string) error {
			if _, err := uuid.Parse(strings.TrimSpace(attemptID)); err != nil {
				return domain.Invalid("BOOTSTRAP_ATTEMPT_ID_INVALID", "--attempt 必须是有效的初始化尝试 ID")
			}
			cfg, err := localconfig.Load()
			if err != nil {
				return err
			}
			options := bootstrapcheck.Options{Directory: optionalDirectory(args), ServerURL: r.resolveServer(cfg), Offline: true}
			report := bootstrapcheck.Run(command.Context(), options)
			if r.bootstrapCheckHook != nil {
				report = r.bootstrapCheckHook(command.Context(), options)
			}
			summary := diagnosticSummary(attemptID, report)
			if err := domain.ValidateBootstrapDiagnostic(summary); err != nil {
				return err
			}
			result := map[string]any{"summary": summary, "redacted": true, "uploaded": false, "requires_upload_confirmation": true}
			if !upload {
				return r.writeOK("bootstrap.diagnostics", result)
			}
			if !acceptUpload {
				return domain.Policy("BOOTSTRAP_DIAGNOSTIC_CONFIRMATION_REQUIRED", "上传前必须确认当前脱敏摘要", "先预览输出，再同时传入 --upload --accept-upload")
			}
			_, workspace, ok := cfg.PrimaryWorkspace()
			if !ok || workspace.WorkspaceID == "" {
				return domain.Conflict("WORKSPACE_BINDING_MISSING", "当前配置没有可用于上传诊断摘要的工作区")
			}
			workspaceToken, err := localconfig.WorkspaceToken(workspace.WorkspaceID)
			if err != nil {
				return domain.E("credential", "secure_store", "WORKSPACE_CREDENTIAL_UNAVAILABLE", err.Error(), 3)
			}
			var uploaded domain.BootstrapDiagnostic
			if err := apiclient.New(r.resolveServer(cfg), workspaceToken).Dispatch(command.Context(), "bootstrap.diagnostic.upload", summary, &uploaded); err != nil {
				return err
			}
			result["uploaded"], result["diagnostic"] = true, uploaded
			return r.writeOK("bootstrap.diagnostics", result)
		},
	}
	command.Flags().StringVar(&attemptID, "attempt", "", "与支持码一同显示的初始化尝试 ID")
	command.Flags().BoolVar(&upload, "upload", false, "上传刚刚预览的脱敏摘要")
	command.Flags().BoolVar(&acceptUpload, "accept-upload", false, "确认只上传本次生成的摘要")
	return command
}

func diagnosticSummary(attemptID string, report bootstrapcheck.Report) domain.BootstrapDiagnosticSummary {
	versions := map[string]string{"contentcloud_cli": Version}
	checks := make([]domain.BootstrapDiagnosticCheck, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, domain.BootstrapDiagnosticCheck{CheckID: check.CheckID, Status: check.Status, ErrorCode: check.ErrorCode})
		if value, ok := check.Facts["node_version"].(string); ok && value != "" {
			versions["node"] = value
		}
		if value, ok := check.Facts["codex_version"].(string); ok && value != "" {
			versions["codex_cli"] = value
		}
	}
	return domain.BootstrapDiagnosticSummary{SchemaVersion: domain.BootstrapSchemaVersion, AttemptID: attemptID, Platform: report.Platform, Arch: report.Arch, Versions: versions, Checks: checks, ManagedDigests: map[string]string{}}
}

func newBootstrapVerifier() (string, error) {
	body := make([]byte, 32)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func bootstrapVerifierChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
