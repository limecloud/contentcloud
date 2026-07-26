package app

import (
	"context"
	"crypto/subtle"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type FinishRunAttemptInput struct {
	Outcome           string         `json:"outcome"`
	FailureClass      string         `json:"failure_class"`
	ExitCode          *int           `json:"exit_code"`
	Usage             map[string]any `json:"usage"`
	TranscriptSummary string         `json:"transcript_summary"`
}

func (s *Service) RunAttempts(ctx context.Context, actor Actor, runID string) ([]domain.RunAttempt, error) {
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Project(ctx, actor.TenantID, run.ProjectID); err != nil {
		return nil, err
	}
	return s.store.RunAttempts(ctx, actor.TenantID, runID)
}

func (s *Service) activeRunAttempt(ctx context.Context, actor Actor, device domain.Device, run domain.TaskRun, attemptID, runToken string, now time.Time) (domain.RunAttempt, error) {
	if attemptID == "" {
		attemptID = run.ActiveAttemptID
	}
	if run.ActiveAttemptID == "" || attemptID != run.ActiveAttemptID || run.LeaseDeviceID != device.ID || (run.State != "leased" && run.State != "running") {
		return domain.RunAttempt{}, domain.Conflict("RUN_ATTEMPT_STALE", "Attempt 已失效或不再是任务的活动租约")
	}
	attempt, err := s.store.RunAttempt(ctx, actor.TenantID, attemptID)
	if err != nil {
		return attempt, err
	}
	if attempt.RunID != run.ID || attempt.DeviceID != device.ID || (attempt.State != "leased" && attempt.State != "running") {
		return attempt, domain.Conflict("RUN_ATTEMPT_STALE", "Attempt 已失效或不属于当前设备")
	}
	if attempt.TokenHash == "" || subtle.ConstantTimeCompare([]byte(attempt.TokenHash), []byte(domain.TokenHash(runToken))) != 1 {
		return attempt, domain.E("authentication", "run_token", "RUN_TOKEN_INVALID", "运行凭据无效", 3)
	}
	if now.After(attempt.LeaseExpiresAt.Add(2 * time.Minute)) {
		return attempt, domain.Conflict("RUN_LEASE_EXPIRED", "任务租约已失效")
	}
	return attempt, nil
}

func (s *Service) FinishRunAttempt(ctx context.Context, actor Actor, device domain.Device, runID, attemptID, runToken string, input FinishRunAttemptInput, requestID string) (domain.TaskRun, error) {
	run, err := s.store.Run(ctx, actor.TenantID, runID)
	if err != nil {
		return run, err
	}
	attempt, err := s.activeRunAttempt(ctx, actor, device, run, attemptID, runToken, s.now().UTC())
	if err != nil {
		return run, err
	}
	switch input.Outcome {
	case "failed":
		if strings.TrimSpace(input.FailureClass) == "" {
			return run, domain.Invalid("FAILURE_CLASS_REQUIRED", "失败 Attempt 必须提供 failure_class")
		}
		return s.failRunAttempt(ctx, run, attempt, input.FailureClass, input.ExitCode, input.Usage, input.TranscriptSummary)
	case "canceled":
		if run.CancelRequestedAt == nil {
			return run, domain.Conflict("RUN_CANCEL_NOT_REQUESTED", "任务尚未收到取消请求")
		}
		now := s.now().UTC()
		attempt.State = "canceled"
		attempt.FinishedAt = &now
		attempt.ExitCode = input.ExitCode
		attempt.Usage = input.Usage
		attempt.TranscriptSummary = strings.TrimSpace(input.TranscriptSummary)
		run.State = "canceled"
		run.ProgressLabel = "已取消"
		run.ActiveAttemptID = ""
		run.RunTokenHash = ""
		run.LeaseDeviceID = ""
		run.LeaseExpiresAt = nil
		run.UpdatedAt = now
		if err := s.store.SaveRunAttempt(ctx, attempt); err != nil {
			return run, err
		}
		if err := s.store.SaveRun(ctx, run); err != nil {
			return run, err
		}
		s.audit(ctx, actor, run.ProjectID, "run.attempt_canceled", "run_attempt", attempt.ID, requestID, map[string]any{"run_id": run.ID})
		return run, nil
	default:
		return run, domain.Invalid("ATTEMPT_OUTCOME_INVALID", "Attempt outcome 只允许 failed 或 canceled")
	}
}

func (s *Service) failRunAttempt(ctx context.Context, run domain.TaskRun, attempt domain.RunAttempt, failureClass string, exitCode *int, usage map[string]any, summary string) (domain.TaskRun, error) {
	now := s.now().UTC()
	attempt.State = "failed"
	attempt.FailureClass = strings.TrimSpace(failureClass)
	attempt.ExitCode = exitCode
	attempt.Usage = usage
	attempt.TranscriptSummary = strings.TrimSpace(summary)
	attempt.FinishedAt = &now
	if run.AttemptCount >= 3 {
		run.State = "failed"
		run.ErrorCode = "RUN_ATTEMPTS_EXHAUSTED"
		run.ProgressLabel = "本地执行失败，已达到重试上限"
	} else {
		run.State = "queued"
		run.ErrorCode = ""
		run.ProgressLabel = "等待本地设备重试"
	}
	run.ActiveAttemptID = ""
	run.RunTokenHash = ""
	run.LeaseDeviceID = ""
	run.LeaseExpiresAt = nil
	run.HeartbeatSequence = 0
	run.UpdatedAt = now
	if err := s.store.SaveRunAttempt(ctx, attempt); err != nil {
		return run, err
	}
	if err := s.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) succeedRunAttempt(ctx context.Context, run *domain.TaskRun, attempt domain.RunAttempt) error {
	now := s.now().UTC()
	attempt.State = "succeeded"
	attempt.FinishedAt = &now
	if err := s.store.SaveRunAttempt(ctx, attempt); err != nil {
		return err
	}
	run.ActiveAttemptID = ""
	run.RunTokenHash = ""
	run.LeaseDeviceID = ""
	run.LeaseExpiresAt = nil
	return nil
}
