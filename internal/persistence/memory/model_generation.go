package memory

import (
	"context"
	"sort"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	"github.com/limecloud/contentcloud/internal/platform/fault"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

func (s *Store) CreateModelGenerationReceipt(_ context.Context, value deliverydomain.ModelGenerationReceipt) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.modelGenerationReceipts[value.ID]; exists {
		return fault.Conflict("MODEL_GENERATION_RECEIPT_EXISTS", "模型生成回执已存在")
	}
	s.modelGenerationReceipts[value.ID] = value
	return nil
}

func (s *Store) CreateModelGeneratedRevision(_ context.Context, revision reviewdomain.TaskRevision, receipt deliverydomain.ModelGenerationReceipt) error {
	revision.NormalizeCollections()
	if err := revision.Validate(); err != nil {
		return err
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.TaskRevisionID != revision.ID || receipt.TaskID != revision.TaskID || receipt.TenantID != revision.TenantID {
		return fault.Invalid("MODEL_GENERATION_SCOPE_MISMATCH", "模型回执与候选修订作用域不一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.taskRevisions[revision.ID]; exists {
		return fault.Conflict("TASK_REVISION_EXISTS", "任务版本已存在")
	}
	for _, existing := range s.taskRevisions {
		if existing.TenantID == revision.TenantID && existing.TaskID == revision.TaskID && existing.RevisionNo == revision.RevisionNo {
			return fault.Conflict("TASK_REVISION_NUMBER_EXISTS", "任务版本编号已存在")
		}
	}
	if _, exists := s.modelGenerationReceipts[receipt.ID]; exists {
		return fault.Conflict("MODEL_GENERATION_RECEIPT_EXISTS", "模型生成回执已存在")
	}
	s.taskRevisions[revision.ID] = cloneTaskRevision(revision)
	s.modelGenerationReceipts[receipt.ID] = receipt
	return nil
}

func (s *Store) ModelGenerationReceipts(_ context.Context, tenantID, taskID string) ([]deliverydomain.ModelGenerationReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := []deliverydomain.ModelGenerationReceipt{}
	for _, value := range s.modelGenerationReceipts {
		if value.TenantID == tenantID && (taskID == "" || value.TaskID == taskID) {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].CreatedAt.Before(values[j].CreatedAt) })
	return values, nil
}
