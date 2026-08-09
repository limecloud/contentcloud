package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) ApplyStateRecordCommand(_ context.Context, record domain.StateRecord, expectedVersion int, event domain.JobEvent) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return record, err
	}
	result, err := applyStateRecordLocked(s, record, expectedVersion)
	if err != nil {
		return record, err
	}
	appendRuntimeEventLocked(s, event)
	return result, nil
}

func (s *Store) ApplyFencedStateRecordCommand(_ context.Context, record domain.StateRecord, expectedVersion int, attemptID, fenceToken string, now time.Time, event domain.JobEvent) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateAttemptFenceLocked(s, record.TenantID, attemptID, fenceToken, now); err != nil {
		return record, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return record, err
	}
	result, err := applyStateRecordLocked(s, record, expectedVersion)
	if err != nil {
		return record, err
	}
	appendRuntimeEventLocked(s, event)
	return result, nil
}

func (s *Store) RegisterToolCallCommand(_ context.Context, call domain.ToolCall, event domain.JobEvent) (domain.ToolCall, error) {
	if err := call.Validate(); err != nil {
		return call, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateToolCallLocked(s, call); err != nil {
		return call, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return call, err
	}
	key := toolCallKey(call.TenantID, call.ID)
	if _, ok := s.runtimeToolCalls[key]; ok {
		return call, domain.Conflict("TOOL_CALL_EXISTS", "ToolCall 已存在")
	}
	if toolCallIdempotencyExistsLocked(s, call) {
		return call, domain.Conflict("TOOL_CALL_IDEMPOTENCY_EXISTS", "ToolCall 幂等键已存在")
	}
	s.runtimeToolCalls[key] = call
	appendRuntimeEventLocked(s, event)
	return call, nil
}

func (s *Store) RegisterFencedToolCallCommand(_ context.Context, call domain.ToolCall, fenceToken string, now time.Time, event domain.JobEvent) (domain.ToolCall, error) {
	if err := call.Validate(); err != nil {
		return call, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateAttemptFenceLocked(s, call.TenantID, call.AttemptID, fenceToken, now); err != nil {
		return call, err
	}
	if err := validateToolCallLocked(s, call); err != nil {
		return call, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return call, err
	}
	key := toolCallKey(call.TenantID, call.ID)
	if _, ok := s.runtimeToolCalls[key]; ok {
		return call, domain.Conflict("TOOL_CALL_EXISTS", "ToolCall 已存在")
	}
	if toolCallIdempotencyExistsLocked(s, call) {
		return call, domain.Conflict("TOOL_CALL_IDEMPOTENCY_EXISTS", "ToolCall 幂等键已存在")
	}
	s.runtimeToolCalls[key] = call
	appendRuntimeEventLocked(s, event)
	return call, nil
}

func (s *Store) ApplyToolCallTransitionCommand(_ context.Context, next domain.ToolCall, expectedVersion int, event domain.JobEvent) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return next, err
	}
	key := toolCallKey(next.TenantID, next.ID)
	current, ok := s.runtimeToolCalls[key]
	if !ok {
		return next, domain.NotFound("ToolCall")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return current, domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
	}
	if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
		return current, domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
	}
	s.runtimeToolCalls[key] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func (s *Store) ApplyFencedToolCallTransitionCommand(_ context.Context, next domain.ToolCall, expectedVersion int, fenceToken string, now time.Time, event domain.JobEvent) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateAttemptFenceLocked(s, next.TenantID, next.AttemptID, fenceToken, now); err != nil {
		return next, err
	}
	if err := validateRuntimeEventLocked(s, event); err != nil {
		return next, err
	}
	key := toolCallKey(next.TenantID, next.ID)
	current, ok := s.runtimeToolCalls[key]
	if !ok {
		return next, domain.NotFound("ToolCall")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return current, domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
	}
	if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
		return current, domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
	}
	s.runtimeToolCalls[key] = next
	appendRuntimeEventLocked(s, event)
	return next, nil
}

func stateCollectionKey(tenantID, id string) string { return tenantID + ":" + id }
func stateRecordKey(tenantID, collectionID, key string) string {
	return tenantID + ":" + collectionID + ":" + key
}
func toolCallKey(tenantID, id string) string { return tenantID + ":" + id }

func (s *Store) CreateStateCollection(_ context.Context, collection domain.StateCollection) error {
	if err := collection.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runtimeStateCollections[stateCollectionKey(collection.TenantID, collection.ID)]; ok {
		return domain.Conflict("STATE_COLLECTION_EXISTS", "状态集合已存在")
	}
	for _, existing := range s.runtimeStateCollections {
		if existing.TenantID == collection.TenantID && existing.JobRunID == collection.JobRunID && existing.CollectionKey == collection.CollectionKey {
			return domain.Conflict("STATE_COLLECTION_KEY_EXISTS", "状态集合名称已存在")
		}
	}
	s.runtimeStateCollections[stateCollectionKey(collection.TenantID, collection.ID)] = collection
	return nil
}

func (s *Store) StateCollection(_ context.Context, tenantID, id string) (domain.StateCollection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeStateCollections[stateCollectionKey(tenantID, id)]
	if !ok {
		return domain.StateCollection{}, domain.NotFound("状态集合")
	}
	return value, nil
}

func (s *Store) StateCollections(_ context.Context, tenantID, jobID string) ([]domain.StateCollection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.StateCollection{}
	for _, value := range s.runtimeStateCollections {
		if value.TenantID == tenantID && (jobID == "" || value.JobRunID == jobID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CollectionKey < result[j].CollectionKey })
	return result, nil
}

func (s *Store) StateRecord(_ context.Context, tenantID, id string) (domain.StateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeStateRecords {
		if value.TenantID == tenantID && value.ID == id {
			return value, nil
		}
	}
	return domain.StateRecord{}, domain.NotFound("状态记录")
}

func (s *Store) StateRecords(_ context.Context, tenantID, collectionID string) ([]domain.StateRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.StateRecord{}
	for _, value := range s.runtimeStateRecords {
		if value.TenantID == tenantID && value.CollectionID == collectionID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

func (s *Store) ApplyStateRecordCAS(_ context.Context, record domain.StateRecord, expectedVersion int) (domain.StateRecord, error) {
	if err := record.Validate(); err != nil {
		return record, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return applyStateRecordLocked(s, record, expectedVersion)
}

func applyStateRecordLocked(s *Store, record domain.StateRecord, expectedVersion int) (domain.StateRecord, error) {
	collection, ok := s.runtimeStateCollections[stateCollectionKey(record.TenantID, record.CollectionID)]
	if !ok {
		return record, domain.NotFound("状态集合")
	}
	if record.TenantID != collection.TenantID {
		return record, domain.Invalid("STATE_RECORD_SCOPE_INVALID", "状态记录与集合不属于同一租户")
	}
	if record.SchemaRevision != collection.SchemaRevision {
		return record, domain.Conflict("STATE_SCHEMA_REVISION_CONFLICT", "状态记录 SchemaRevision 与集合已发布版本不一致")
	}
	key := stateRecordKey(record.TenantID, record.CollectionID, record.Key)
	current, exists := s.runtimeStateRecords[key]
	if expectedVersion == 0 && exists {
		return current, domain.Conflict("STATE_RECORD_VERSION_CONFLICT", "状态记录已经存在")
	}
	if exists && current.Version != expectedVersion {
		return current, domain.Conflict("STATE_RECORD_VERSION_CONFLICT", "状态记录版本已变化")
	}
	if !exists && expectedVersion != 0 {
		return record, domain.Conflict("STATE_RECORD_VERSION_CONFLICT", "状态记录不存在")
	}
	if collection.Consistency == "append_only" && exists {
		return current, domain.Policy("STATE_APPEND_ONLY_UPDATE_FORBIDDEN", "append_only 集合不允许覆盖既有记录", "使用新的记录键追加一条记录")
	}
	if !exists {
		count := 0
		for _, existingRecord := range s.runtimeStateRecords {
			if existingRecord.TenantID == record.TenantID && existingRecord.CollectionID == record.CollectionID {
				count++
			}
		}
		if count >= collection.MaxRecords {
			return record, domain.Policy("STATE_COLLECTION_RECORD_LIMIT", "状态集合已达到最大记录数", "清理过期记录或提高已发布集合上限")
		}
	}
	if record.Value != nil {
		body, _ := json.Marshal(record.Value)
		if len(body) > collection.MaxRecordBytes {
			return record, domain.Policy("STATE_RECORD_TOO_LARGE", "状态记录超过集合大小限制", "减少记录内容或改用受控 Artifact 引用")
		}
	}
	if !exists {
		record.Version = 1
		collection.Revision++
	} else {
		record.Version = current.Version + 1
		collection.Revision++
	}
	collection.Watermark++
	collection.UpdatedAt = record.UpdatedAt
	s.runtimeStateRecords[key] = record
	s.runtimeStateCollections[stateCollectionKey(record.TenantID, record.CollectionID)] = collection
	return record, nil
}

func (s *Store) CreateToolCall(_ context.Context, call domain.ToolCall) error {
	if err := call.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateToolCallLocked(s, call); err != nil {
		return err
	}
	key := toolCallKey(call.TenantID, call.ID)
	if _, ok := s.runtimeToolCalls[key]; ok {
		return domain.Conflict("TOOL_CALL_EXISTS", "ToolCall 已存在")
	}
	if toolCallIdempotencyExistsLocked(s, call) {
		return domain.Conflict("TOOL_CALL_IDEMPOTENCY_EXISTS", "ToolCall 幂等键已存在")
	}
	s.runtimeToolCalls[key] = call
	return nil
}

func (s *Store) ToolCall(_ context.Context, tenantID, id string) (domain.ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.runtimeToolCalls[toolCallKey(tenantID, id)]
	if !ok {
		return domain.ToolCall{}, domain.NotFound("ToolCall")
	}
	return value, nil
}

func (s *Store) ToolCallByIdempotencyKey(_ context.Context, tenantID, attemptID, toolName, idempotencyKey string) (domain.ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, value := range s.runtimeToolCalls {
		if value.TenantID == tenantID && value.AttemptID == attemptID && value.ToolName == toolName && toolCallIdempotencyKey(value) == idempotencyKey {
			return value, nil
		}
	}
	return domain.ToolCall{}, domain.NotFound("ToolCall")
}

func (s *Store) ToolCalls(_ context.Context, tenantID, attemptID string) ([]domain.ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.ToolCall{}
	for _, value := range s.runtimeToolCalls {
		if value.TenantID == tenantID && (attemptID == "" || value.AttemptID == attemptID) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) ApplyToolCallTransition(_ context.Context, next domain.ToolCall, expectedVersion int) (domain.ToolCall, error) {
	if err := next.Validate(); err != nil {
		return next, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := toolCallKey(next.TenantID, next.ID)
	current, ok := s.runtimeToolCalls[key]
	if !ok {
		return next, domain.NotFound("ToolCall")
	}
	if current.Version != expectedVersion || next.Version != expectedVersion+1 {
		return current, domain.Conflict("TOOL_CALL_VERSION_CONFLICT", "ToolCall 已被更新")
	}
	if current.State == domain.ToolCallSucceeded || current.State == domain.ToolCallFailed || current.State == domain.ToolCallUnknown {
		return current, domain.Conflict("TOOL_CALL_TERMINAL", "终态 ToolCall 不能原地修改")
	}
	if err := validateToolCallLocked(s, current); err != nil {
		return current, err
	}
	current.State = next.State
	current.SafeResult, current.ResultDigest, current.ErrorCode = next.SafeResult, next.ResultDigest, next.ErrorCode
	current.StartedAt, current.FinishedAt = next.StartedAt, next.FinishedAt
	current.Version, current.UpdatedAt = next.Version, next.UpdatedAt
	s.runtimeToolCalls[key] = current
	return current, nil
}

func validateToolCallLocked(s *Store, call domain.ToolCall) error {
	attempt, ok := s.runtimeAttempts[runtimePlanKey(call.TenantID, call.AttemptID)]
	if !ok {
		return domain.NotFound("RuntimeAttempt")
	}
	agent, ok := s.runtimeAgents[runtimePlanKey(call.TenantID, call.AgentInstanceID)]
	if !ok {
		return domain.NotFound("AgentInstance")
	}
	node, ok := s.runtimeNodes[runtimeNodeKey(call.TenantID, call.NodeRunID)]
	if !ok {
		return domain.NotFound("NodeRun")
	}
	view, ok := s.runtimeContextViews[runtimePlanKey(call.TenantID, attempt.ContextViewID)]
	if !ok {
		return domain.NotFound("ContextView")
	}
	if attempt.JobRunID != call.JobRunID || attempt.NodeRunID != call.NodeRunID || attempt.AgentInstanceID != call.AgentInstanceID || agent.JobRunID != call.JobRunID || agent.NodeRunID != call.NodeRunID || agent.ContextViewID != view.ID || node.JobRunID != call.JobRunID || view.JobRunID != call.JobRunID || view.NodeRunID != call.NodeRunID || view.AttemptID != call.AttemptID {
		return domain.Invalid("TOOL_CALL_SCOPE_INVALID", "ToolCall 必须绑定同一 JobRun、NodeRun、Attempt、Agent 和 ContextView")
	}
	if attempt.State != domain.RuntimeAttemptPrepared && attempt.State != domain.RuntimeAttemptRunning {
		return domain.Conflict("TOOL_CALL_ATTEMPT_NOT_ACTIVE", "只有准备中或运行中的 Attempt 可以创建或推进 ToolCall")
	}
	if agent.State != domain.AgentRunnable && agent.State != domain.AgentActive {
		return domain.Conflict("TOOL_CALL_AGENT_NOT_ACTIVE", "只有可运行或活动中的 AgentInstance 可以创建或推进 ToolCall")
	}
	if !view.AllowsTool(call.ToolName) {
		return domain.Policy("TOOL_CALL_NOT_ALLOWED", "当前 ContextView 未授权该工具", "仅调用 AllowedTools 中的工具")
	}
	return nil
}

func toolCallIdempotencyExistsLocked(s *Store, call domain.ToolCall) bool {
	key := toolCallIdempotencyKey(call)
	if key == "" {
		return false
	}
	for _, existing := range s.runtimeToolCalls {
		if existing.TenantID == call.TenantID && existing.AttemptID == call.AttemptID && existing.ToolName == call.ToolName && toolCallIdempotencyKey(existing) == key {
			return true
		}
	}
	return false
}

func toolCallIdempotencyKey(call domain.ToolCall) string {
	value, _ := call.SafeRequest["idempotency_key"].(string)
	return strings.TrimSpace(value)
}
