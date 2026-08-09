package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

type RuntimeBusinessResultRun struct {
	Claimed int `json:"claimed"`
	Applied int `json:"applied"`
	Ignored int `json:"ignored"`
	Retried int `json:"retried"`
}

func (s *Service) applyRuntimeBusinessResult(ctx context.Context, actor Actor, jobID string, payload json.RawMessage, requestID string) (bool, error) {
	run, pkg, handled, err := s.runtimeKnowledgePackage(ctx, actor, jobID, payload)
	if err != nil || !handled {
		return handled, err
	}
	_, _, err = s.importKnowledgePackage(ctx, actor, run, pkg, requestID)
	return true, err
}

// ConsumeRuntimeBusinessResults materializes business-owned objects from
// successful Runtime outputs. The outbox receipt is acknowledged only after
// the idempotent business write has completed.
func (s *Service) ConsumeRuntimeBusinessResults(ctx context.Context, tenantID, worker string, leaseFor time.Duration, limit int) (RuntimeBusinessResultRun, error) {
	result := RuntimeBusinessResultRun{}
	if s == nil || s.runtimeService == nil {
		return result, domain.Policy("RUNTIME_UNAVAILABLE", "业务结果消费者需要已配置的 Runtime", "配置 Runtime Repository 后重试")
	}
	commands, ok := s.store.(contentruntime.RuntimeCommandStore)
	if !ok {
		return result, domain.Policy("RUNTIME_COMMAND_STORE_REQUIRED", "业务结果消费者需要事务 outbox 存储", "升级 Runtime 存储实现后重试")
	}
	worker = strings.TrimSpace(worker)
	if worker == "" {
		return result, domain.Invalid("RUNTIME_BUSINESS_CONSUMER_REQUIRED", "业务结果消费者需要稳定的工作器身份")
	}
	if leaseFor <= 0 {
		leaseFor = time.Minute
	}
	if limit <= 0 {
		limit = 50
	}
	now := s.now().UTC()
	messages, err := commands.ClaimRuntimeOutbox(ctx, tenantID, domain.RuntimeOutboxSubscriberBusinessResult, worker, now, leaseFor, limit)
	if err != nil {
		return result, err
	}
	result.Claimed = len(messages)
	for _, message := range messages {
		applied, consumeErr := s.consumeRuntimeBusinessResult(ctx, tenantID, message)
		completedAt := s.now().UTC()
		if consumeErr != nil {
			result.Retried++
			retryAt := completedAt.Add(runtimeBusinessResultBackoff(message.Attempts))
			if retryErr := commands.RetryRuntimeOutbox(ctx, tenantID, message.ID, domain.RuntimeOutboxSubscriberBusinessResult, worker, completedAt, retryAt, consumeErr.Error()); retryErr != nil {
				return result, retryErr
			}
			continue
		}
		if err := commands.AckRuntimeOutbox(ctx, tenantID, message.ID, domain.RuntimeOutboxSubscriberBusinessResult, worker, completedAt); err != nil {
			return result, err
		}
		if applied {
			result.Applied++
		} else {
			result.Ignored++
		}
	}
	return result, nil
}

func (s *Service) consumeRuntimeBusinessResult(ctx context.Context, tenantID string, message domain.RuntimeOutboxMessage) (bool, error) {
	eventType, _ := message.Payload["type"].(string)
	if eventType != "attempt.succeeded" {
		return false, nil
	}
	eventPayload, _ := message.Payload["payload"].(map[string]any)
	attemptID, _ := eventPayload["attempt_id"].(string)
	if strings.TrimSpace(attemptID) == "" {
		return false, domain.Invalid("RUNTIME_BUSINESS_EVENT_INVALID", "Runtime 成功事件缺少 attempt_id")
	}
	handle, err := s.runtimeService.LoadDispatchHandle(ctx, tenantID, attemptID)
	if err != nil {
		return false, err
	}
	if handle.Attempt.State != domain.RuntimeAttemptSucceeded || handle.Attempt.JobRunID != message.AggregateID {
		return false, domain.Conflict("RUNTIME_BUSINESS_EVENT_CONFLICT", "Runtime 成功事件与权威 Attempt 状态不一致")
	}
	eventResultDigest, _ := eventPayload["result_digest"].(string)
	if eventResultDigest != "" && eventResultDigest != handle.Attempt.ResultDigest {
		return false, domain.Conflict("RUNTIME_BUSINESS_EVENT_CONFLICT", "Runtime 成功事件与权威 Attempt 结果摘要不一致")
	}
	resultRefs := []string{}
	for _, ref := range handle.Attempt.OutputRefs {
		if strings.HasPrefix(ref, "runtime-result:") {
			resultRefs = append(resultRefs, ref)
		}
	}
	if len(resultRefs) == 0 {
		return false, nil
	}
	if len(resultRefs) != 1 {
		return false, domain.Conflict("RUNTIME_BUSINESS_RESULT_AMBIGUOUS", "一个 RuntimeAttempt 只能交接一个结构化业务结果")
	}
	key, expectedDigest, err := parseRuntimeBusinessResultRef(resultRefs[0], tenantID, attemptID)
	if err != nil {
		return false, err
	}
	body, err := s.blobs.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if len(body) > 4*1024*1024 {
		return false, domain.Policy("RUNTIME_BUSINESS_RESULT_TOO_LARGE", "持久化业务结果超过 Runtime 单次交接上限", "修复损坏的结果对象后重试")
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return false, domain.Invalid("RUNTIME_BUSINESS_RESULT_INVALID", "持久化业务结果不是合法 JSON")
	}
	actualDigest, err := domain.CanonicalHash(value)
	if err != nil {
		return false, err
	}
	if actualDigest != expectedDigest {
		return false, domain.Conflict("RUNTIME_BUSINESS_RESULT_DIGEST_MISMATCH", "持久化业务结果摘要与 Runtime output ref 不一致")
	}
	job, err := s.runtimeService.Job(ctx, tenantID, handle.Attempt.JobRunID)
	if err != nil {
		return false, err
	}
	actor := Actor{UserID: job.CreatedBy, TenantID: tenantID, Type: "runtime"}
	handled, err := s.applyRuntimeBusinessResult(ctx, actor, job.ID, body, "runtime-business-result:"+message.EventID)
	if err != nil {
		return false, err
	}
	return handled, nil
}

func parseRuntimeBusinessResultRef(ref, tenantID, attemptID string) (string, string, error) {
	key := strings.TrimPrefix(strings.TrimSpace(ref), "runtime-result:")
	wantedDirectory := "runtime/results/" + strings.TrimSpace(tenantID) + "/" + strings.TrimSpace(attemptID)
	if key == "" || key == ref || path.Ext(key) != ".json" || path.Dir(key) != wantedDirectory {
		return "", "", domain.Invalid("RUNTIME_BUSINESS_RESULT_REF_INVALID", "Runtime 业务结果引用无效")
	}
	digest := strings.TrimSuffix(path.Base(key), ".json")
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 {
		return "", "", domain.Invalid("RUNTIME_BUSINESS_RESULT_REF_INVALID", "Runtime 业务结果引用缺少 SHA-256 摘要")
	}
	return key, digest, nil
}

func runtimeBusinessResultBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := time.Second * time.Duration(1<<(attempt-1))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
