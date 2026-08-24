package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/persistence/blob"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

// RuntimeCleanupDiagnosticSummary is the operator-facing result for a cleanup
// retry. It contains the durable diagnostic and whether this request attempted
// a Blob deletion.
type RuntimeCleanupDiagnosticSummary struct {
	Diagnostic      contentruntime.RuntimeCleanupDiagnostic `json:"diagnostic"`
	DeleteAttempted bool                                    `json:"delete_attempted"`
}

// RuntimeCleanupReconciliationResult is the durable-cleanup worker projection.
// It reports only counts; the diagnostic itself remains the Runtime fact.
type RuntimeCleanupReconciliationResult struct {
	TenantID  string `json:"tenant_id"`
	WorkerID  string `json:"worker_id"`
	Scanned   int    `json:"scanned"`
	Reclaimed int    `json:"reclaimed"`
	Claimed   int    `json:"claimed"`
	Cleaned   int    `json:"cleaned"`
	NotFound  int    `json:"not_found"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
}

const runtimeCleanupClaimLease = 5 * time.Minute

func requireRuntimeCleanupOperator(actor Actor) error {
	if actor.PlatformAdmin {
		return nil
	}
	if actor.Type != "" && actor.Type != "user" {
		return fault.Policy("RUNTIME_CLEANUP_OPERATOR_REQUIRED", "清理诊断人工运维入口只允许用户会话", "使用租户管理员或项目经理会话")
	}
	if actor.Role != "tenant_admin" && actor.Role != "project_manager" {
		return fault.Policy("RUNTIME_CLEANUP_OPERATOR_REQUIRED", "清理诊断查询和重试需要租户管理员或项目经理权限", "切换到租户管理员或项目经理账号")
	}
	return nil
}

func (s *OperationsService) RuntimeCleanupDiagnostics(ctx context.Context, actor Actor, status string, limit int) ([]contentruntime.RuntimeCleanupDiagnostic, error) {
	if err := requireRuntimeCleanupOperator(actor); err != nil {
		return nil, err
	}
	if strings.TrimSpace(status) != "" {
		switch status {
		case contentruntime.RuntimeCleanupPending, contentruntime.RuntimeCleanupRetrying, contentruntime.RuntimeCleanupCleaned, contentruntime.RuntimeCleanupNotFound, contentruntime.RuntimeCleanupFailed:
		default:
			return nil, fault.Invalid("RUNTIME_CLEANUP_STATUS_INVALID", "清理诊断状态无效")
		}
	}
	return s.runtimeRepo.RuntimeCleanupDiagnostics(ctx, actor.TenantID, strings.TrimSpace(status), limit)
}

func (s *OperationsService) RuntimeCleanupDiagnostic(ctx context.Context, actor Actor, id string) (contentruntime.RuntimeCleanupDiagnostic, error) {
	if err := requireRuntimeCleanupOperator(actor); err != nil {
		return contentruntime.RuntimeCleanupDiagnostic{}, err
	}
	if strings.TrimSpace(id) == "" {
		return contentruntime.RuntimeCleanupDiagnostic{}, fault.Invalid("RUNTIME_CLEANUP_ID_REQUIRED", "清理诊断 ID 不能为空")
	}
	return s.runtimeRepo.RuntimeCleanupDiagnostic(ctx, actor.TenantID, strings.TrimSpace(id))
}

func (s *OperationsService) RetryRuntimeCleanupDiagnostic(ctx context.Context, actor Actor, id, requestID string) (RuntimeCleanupDiagnosticSummary, error) {
	if err := requireRuntimeCleanupOperator(actor); err != nil {
		return RuntimeCleanupDiagnosticSummary{}, err
	}
	return s.retryRuntimeCleanupDiagnostic(ctx, actor, id, requestID)
}

// ReconcileRuntimeCleanupDiagnostics is the machine-only recovery path. It
// reads the durable Runtime diagnostic, reclaims expired retrying claims and
// reuses the same CAS-protected delete transition as the human operation.
// There is deliberately no second queue or business task model here.
func (s *OperationsService) ReconcileRuntimeCleanupDiagnostics(ctx context.Context, tenantID, workerID string, limit int) (RuntimeCleanupReconciliationResult, error) {
	tenantID = strings.TrimSpace(tenantID)
	workerID = strings.TrimSpace(workerID)
	if tenantID == "" {
		return RuntimeCleanupReconciliationResult{}, fault.Invalid("RUNTIME_CLEANUP_TENANT_REQUIRED", "清理恢复缺少租户范围")
	}
	if workerID == "" {
		return RuntimeCleanupReconciliationResult{}, fault.Invalid("RUNTIME_CLEANUP_WORKER_REQUIRED", "清理恢复缺少工作器标识")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	items, err := s.runtimeRepo.RuntimeCleanupDiagnostics(ctx, tenantID, "", limit)
	if err != nil {
		return RuntimeCleanupReconciliationResult{TenantID: tenantID, WorkerID: workerID}, err
	}
	result := RuntimeCleanupReconciliationResult{TenantID: tenantID, WorkerID: workerID, Scanned: len(items)}
	for _, item := range items {
		current := item
		now := s.now().UTC()
		if current.Status == contentruntime.RuntimeCleanupRetrying {
			if now.Sub(current.UpdatedAt) < runtimeCleanupClaimLease {
				result.Skipped++
				continue
			}
			recovered := current
			recovered.Status = contentruntime.RuntimeCleanupFailed
			recovered.CleanupError = "RUNTIME_CLEANUP_CLAIM_EXPIRED"
			retryAt := now
			recovered.NextRetryAt = &retryAt
			recovered.Version++
			recovered.UpdatedAt = now
			if err := s.runtimeRepo.UpdateRuntimeCleanupDiagnostic(ctx, recovered, current.Version); err != nil {
				if isRuntimeCleanupRace(err) {
					result.Skipped++
					continue
				}
				return result, err
			}
			result.Reclaimed++
			current = recovered
		}
		if current.Status == contentruntime.RuntimeCleanupCleaned || current.Status == contentruntime.RuntimeCleanupNotFound {
			result.Skipped++
			continue
		}
		if current.Status == contentruntime.RuntimeCleanupFailed && current.NextRetryAt != nil && current.NextRetryAt.After(now) {
			result.Skipped++
			continue
		}
		requestID := fmt.Sprintf("runtime-cleanup:%s:%s", workerID, current.ID)
		summary, retryErr := s.retryRuntimeCleanupDiagnostic(ctx, Actor{UserID: "runtime-cleanup:" + workerID, TenantID: tenantID, Type: "worker"}, current.ID, requestID)
		if retryErr != nil {
			if summary.Diagnostic.Status == contentruntime.RuntimeCleanupFailed && isRuntimeCleanupDeleteFailure(retryErr) {
				result.Claimed++
				result.Failed++
				continue
			}
			if isRuntimeCleanupRace(retryErr) {
				result.Skipped++
				continue
			}
			return result, retryErr
		}
		result.Claimed++
		switch summary.Diagnostic.Status {
		case contentruntime.RuntimeCleanupCleaned:
			result.Cleaned++
		case contentruntime.RuntimeCleanupNotFound:
			result.NotFound++
		case contentruntime.RuntimeCleanupFailed:
			result.Failed++
		}
	}
	return result, nil
}

func (s *OperationsService) retryRuntimeCleanupDiagnostic(ctx context.Context, actor Actor, id, requestID string) (RuntimeCleanupDiagnosticSummary, error) {
	if strings.TrimSpace(id) == "" {
		return RuntimeCleanupDiagnosticSummary{}, fault.Invalid("RUNTIME_CLEANUP_ID_REQUIRED", "清理诊断 ID 不能为空")
	}
	diagnostic, err := s.runtimeRepo.RuntimeCleanupDiagnostic(ctx, actor.TenantID, strings.TrimSpace(id))
	if err != nil {
		return RuntimeCleanupDiagnosticSummary{}, err
	}
	if diagnostic.Status != contentruntime.RuntimeCleanupPending && diagnostic.Status != contentruntime.RuntimeCleanupFailed {
		return RuntimeCleanupDiagnosticSummary{Diagnostic: diagnostic}, fault.Conflict("RUNTIME_CLEANUP_NOT_RETRYABLE", "当前清理诊断不是可重试状态")
	}
	now := s.now().UTC()
	claim := diagnostic
	claim.Status = contentruntime.RuntimeCleanupRetrying
	claim.CleanupError = ""
	claim.NextRetryAt = nil
	claim.AttemptCount++
	claim.Version++
	claim.UpdatedAt = now
	if err := s.runtimeRepo.UpdateRuntimeCleanupDiagnostic(ctx, claim, diagnostic.Version); err != nil {
		return RuntimeCleanupDiagnosticSummary{Diagnostic: diagnostic}, err
	}

	result := RuntimeCleanupDiagnosticSummary{Diagnostic: claim, DeleteAttempted: true}
	deleteErr := s.deleteRuntimeCleanupBlob(ctx, claim.ObjectKey)
	final := claim
	final.Version++
	final.UpdatedAt = s.now().UTC()
	switch {
	case deleteErr == nil:
		final.Status = contentruntime.RuntimeCleanupCleaned
		final.CleanupError = ""
		final.NextRetryAt = nil
	case errors.Is(deleteErr, blob.ErrNotFound):
		final.Status = contentruntime.RuntimeCleanupNotFound
		final.CleanupError = ""
		final.NextRetryAt = nil
	default:
		final.Status = contentruntime.RuntimeCleanupFailed
		final.CleanupError = safeRuntimeCleanupErrorCode(deleteErr)
		next := final.UpdatedAt.Add(runtimeCleanupRetryDelay(final.AttemptCount))
		final.NextRetryAt = &next
	}
	if err := s.runtimeRepo.UpdateRuntimeCleanupDiagnostic(ctx, final, claim.Version); err != nil {
		return RuntimeCleanupDiagnosticSummary{Diagnostic: claim, DeleteAttempted: true}, err
	}
	result.Diagnostic = final
	s.audit(ctx, actor, final.ProjectID, "runtime.cleanup_retried", "runtime_cleanup_diagnostic", final.ID, requestID, map[string]any{"status": final.Status, "attempt_count": final.AttemptCount})
	if deleteErr != nil && !errors.Is(deleteErr, blob.ErrNotFound) {
		return result, &fault.Error{Type: "storage", Subtype: "consistency", Code: "RUNTIME_CLEANUP_DELETE_FAILED", Message: "临时对象清理仍未完成", Retryable: true, Hint: "稍后再次重试清理诊断", Details: map[string]any{"diagnostic_id": final.ID, "status": final.Status}}
	}
	return result, nil
}

func isRuntimeCleanupRace(err error) bool {
	var value *fault.Error
	return errors.As(err, &value) && (value.Type == "conflict" || value.Type == "not_found")
}

func isRuntimeCleanupDeleteFailure(err error) bool {
	var value *fault.Error
	return errors.As(err, &value) && value.Code == "RUNTIME_CLEANUP_DELETE_FAILED"
}

func (s *OperationsService) deleteRuntimeCleanupBlob(ctx context.Context, objectKey string) error {
	deleter, ok := s.blobs.(blob.DeleteStore)
	if !ok {
		return errors.New("对象存储不支持删除操作")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return deleter.Delete(cleanupCtx, objectKey)
}

func runtimeCleanupRetryDelay(attemptCount int) time.Duration {
	if attemptCount < 1 {
		attemptCount = 1
	}
	delay := time.Minute
	for i := 1; i < attemptCount && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func safeRuntimeCleanupErrorCode(err error) string {
	var domainErr *fault.Error
	if errors.As(err, &domainErr) && strings.TrimSpace(domainErr.Code) != "" {
		return domainErr.Code
	}
	return "BLOB_DELETE_FAILED"
}
