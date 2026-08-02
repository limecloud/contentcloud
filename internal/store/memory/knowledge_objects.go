package memory

import (
	"context"
	"sort"
	"strconv"

	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateKnowledgeObject(_ context.Context, value domain.KnowledgeObject) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Digest == "" {
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = digest
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := knowledgeObjectKey(value.TenantID, value.ID, value.Version)
	if _, exists := s.knowledgeObjects[key]; exists {
		return domain.Conflict("KNOWLEDGE_OBJECT_EXISTS", "同一知识对象版本已存在")
	}
	s.knowledgeObjects[key] = cloneKnowledgeObject(value)
	return nil
}

func (s *Store) CreateKnowledgeObjectDecision(_ context.Context, object domain.KnowledgeObject, decision domain.KnowledgeDecision) error {
	if err := object.Validate(); err != nil {
		return err
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	if decision.TenantID != object.TenantID || decision.ProjectID != object.ProjectID || decision.ObjectID != object.ID || decision.ResultVersion != object.Version {
		return domain.Invalid("KNOWLEDGE_DECISION_INVALID", "知识决策与结果对象版本不一致")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	objectKey := knowledgeObjectKey(object.TenantID, object.ID, object.Version)
	if _, exists := s.knowledgeObjects[objectKey]; exists {
		return domain.Conflict("KNOWLEDGE_OBJECT_EXISTS", "知识决策结果版本已存在")
	}
	if _, exists := s.knowledgeDecisions[decision.ID]; exists {
		return domain.Conflict("KNOWLEDGE_DECISION_EXISTS", "知识决策已存在")
	}
	previous, exists := s.knowledgeObjects[knowledgeObjectKey(object.TenantID, object.ID, decision.PreviousVersion)]
	if !exists || previous.Digest != decision.SubjectDigest {
		return domain.Conflict("KNOWLEDGE_DECISION_SUBJECT_CHANGED", "知识对象版本或 digest 已变化")
	}
	s.knowledgeObjects[objectKey] = cloneKnowledgeObject(object)
	s.knowledgeDecisions[decision.ID] = decision
	return nil
}

func (s *Store) KnowledgeObjects(_ context.Context, tenantID, projectID string) ([]domain.KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.KnowledgeObject{}
	for _, value := range s.knowledgeObjects {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			result = append(result, cloneKnowledgeObject(value))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == result[j].ID {
			return result[i].Version < result[j].Version
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (s *Store) KnowledgeObject(_ context.Context, tenantID, objectID string, version int) (domain.KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result domain.KnowledgeObject
	for _, value := range s.knowledgeObjects {
		if value.TenantID != tenantID || value.ID != objectID {
			continue
		}
		if version > 0 && value.Version != version {
			continue
		}
		if result.ID == "" || value.Version > result.Version {
			result = value
		}
	}
	if result.ID == "" {
		return result, domain.NotFound("知识对象")
	}
	return cloneKnowledgeObject(result), nil
}

func (s *Store) CreateKnowledgePack(_ context.Context, value domain.KnowledgePack) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Digest == "" {
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = digest
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.knowledgePacks[value.ID]; exists {
		return domain.Conflict("KNOWLEDGE_PACK_EXISTS", "知识包已存在")
	}
	s.knowledgePacks[value.ID] = cloneKnowledgePack(value)
	return nil
}

func (s *Store) KnowledgePacks(_ context.Context, tenantID, projectID string) ([]domain.KnowledgePack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.KnowledgePack{}
	for _, value := range s.knowledgePacks {
		if value.TenantID == tenantID && value.ProjectID == projectID {
			result = append(result, cloneKnowledgePack(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) KnowledgePack(_ context.Context, tenantID, id string) (domain.KnowledgePack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.knowledgePacks[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("知识包")
	}
	return cloneKnowledgePack(value), nil
}

func (s *Store) SaveKnowledgePack(_ context.Context, value domain.KnowledgePack) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.knowledgePacks[value.ID]
	if !ok || current.TenantID != value.TenantID {
		return domain.NotFound("知识包")
	}
	if current.Status != "draft" {
		return domain.Conflict("KNOWLEDGE_PACK_IMMUTABLE", "已发布或已退休知识包不可修改")
	}
	if value.Digest == "" {
		digest, err := value.ContentDigest()
		if err != nil {
			return err
		}
		value.Digest = digest
	}
	s.knowledgePacks[value.ID] = cloneKnowledgePack(value)
	return nil
}

func (s *Store) CreateKnowledgeSnapshot(_ context.Context, value domain.KnowledgeSnapshot) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.knowledgeSnapshots[value.ID]; exists {
		return domain.Conflict("KNOWLEDGE_SNAPSHOT_EXISTS", "知识快照已存在")
	}
	s.knowledgeSnapshots[value.ID] = cloneKnowledgeSnapshot(value)
	return nil
}

func (s *Store) KnowledgeSnapshots(_ context.Context, tenantID, projectID, packID string) ([]domain.KnowledgeSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.KnowledgeSnapshot{}
	for _, value := range s.knowledgeSnapshots {
		if value.TenantID == tenantID && value.ProjectID == projectID && (packID == "" || value.PackID == packID) {
			result = append(result, cloneKnowledgeSnapshot(value))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) KnowledgeSnapshot(_ context.Context, tenantID, id string) (domain.KnowledgeSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.knowledgeSnapshots[id]
	if !ok || value.TenantID != tenantID {
		return value, domain.NotFound("知识快照")
	}
	return cloneKnowledgeSnapshot(value), nil
}

func (s *Store) KnowledgeDecisions(_ context.Context, tenantID, objectID string) ([]domain.KnowledgeDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.KnowledgeDecision{}
	for _, value := range s.knowledgeDecisions {
		if value.TenantID == tenantID && value.ObjectID == objectID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func knowledgeObjectKey(tenantID, objectID string, version int) string {
	return tenantID + ":" + objectID + "@" + strconv.Itoa(version)
}

func cloneKnowledgeObject(value domain.KnowledgeObject) domain.KnowledgeObject {
	value.Dimensions = append([]string(nil), value.Dimensions...)
	value.AllowedChannels = append([]string(nil), value.AllowedChannels...)
	value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	value.RelationRefs = append([]string(nil), value.RelationRefs...)
	value.RightsRefs = append([]string(nil), value.RightsRefs...)
	value.ConflictRefs = append([]string(nil), value.ConflictRefs...)
	if value.Payload != nil {
		value.Payload = cloneMap(value.Payload)
	}
	return value
}

func cloneKnowledgePack(value domain.KnowledgePack) domain.KnowledgePack {
	value.ObjectRefs = append([]domain.KnowledgePackObjectRef(nil), value.ObjectRefs...)
	value.QueryPolicy.EligibleStatuses = append([]string(nil), value.QueryPolicy.EligibleStatuses...)
	value.QueryPolicy.AllowedObjectTypes = append([]string(nil), value.QueryPolicy.AllowedObjectTypes...)
	return value
}

func cloneKnowledgeSnapshot(value domain.KnowledgeSnapshot) domain.KnowledgeSnapshot {
	objects := value.Objects
	value.Objects = make([]domain.KnowledgeObject, len(objects))
	for index, object := range objects {
		value.Objects[index] = cloneKnowledgeObject(object)
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
