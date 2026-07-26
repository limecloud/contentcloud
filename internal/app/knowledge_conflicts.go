package app

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

func validateKnowledgeValue(value domain.TypedValue) error {
	if value.Type == "" {
		return nil
	}
	switch value.Type {
	case "text", "date", "enum":
		if strings.TrimSpace(value.Text) == "" {
			return domain.Invalid("KNOWLEDGE_VALUE_INVALID", "文本、日期或枚举类型必须提供 text")
		}
	case "number":
		if value.Number == nil {
			return domain.Invalid("KNOWLEDGE_VALUE_INVALID", "数值类型必须提供 number")
		}
	case "boolean":
		if value.Boolean == nil {
			return domain.Invalid("KNOWLEDGE_VALUE_INVALID", "布尔类型必须提供 boolean")
		}
	default:
		return domain.Invalid("KNOWLEDGE_VALUE_TYPE_INVALID", "知识值类型只允许 text、number、boolean、date 或 enum")
	}
	return nil
}

func knowledgeWithinTime(item domain.KnowledgeItem, now time.Time) bool {
	if item.ValidFrom != nil && now.Before(*item.ValidFrom) {
		return false
	}
	if item.ValidUntil != nil && !now.Before(*item.ValidUntil) {
		return false
	}
	return item.ExpiresAt == nil || now.Before(*item.ExpiresAt)
}

func knowledgeActive(item domain.KnowledgeItem, channel string, now time.Time) bool {
	if item.Status != "approved" || !knowledgeWithinTime(item, now) {
		return false
	}
	allowed := item.AllowedChannels
	if len(item.Scope.Channels) > 0 {
		allowed = item.Scope.Channels
	}
	return channel == "" || len(allowed) == 0 || containsString(allowed, channel) || containsString(allowed, "*")
}

func (s *Service) KnowledgeConflicts(ctx context.Context, actor Actor, projectID string) ([]domain.KnowledgeConflict, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.KnowledgeConflicts(ctx, actor.TenantID, projectID)
}

func (s *Service) DecisionRequests(ctx context.Context, actor Actor, projectID string) ([]domain.DecisionRequest, error) {
	if _, err := s.store.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.store.DecisionRequests(ctx, actor.TenantID, projectID)
}

func (s *Service) ResolveDecisionRequest(ctx context.Context, actor Actor, requestID, selectedKnowledgeID, notes, auditRequestID string) (domain.DecisionRequest, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return domain.DecisionRequest{}, err
	}
	request, err := s.store.DecisionRequest(ctx, actor.TenantID, requestID)
	if err != nil {
		return request, err
	}
	if _, err := s.projectForWrite(ctx, actor, request.ProjectID); err != nil {
		return request, err
	}
	if request.Status != "open" || !containsString(request.KnowledgeItemIDs, selectedKnowledgeID) {
		return request, domain.Conflict("DECISION_REQUEST_STATE_INVALID", "决策请求已结束或选中值不属于该冲突")
	}
	now := s.now().UTC()
	for _, itemID := range request.KnowledgeItemIDs {
		item, err := s.store.KnowledgeItem(ctx, actor.TenantID, itemID)
		if err != nil {
			return request, err
		}
		item.Status = "rejected"
		if itemID == selectedKnowledgeID {
			item.Status = "needs_review"
		}
		item.ApprovedBy, item.ApprovedAt = "", nil
		item.RowVersion++
		item.UpdatedAt = now
		if err := s.store.SaveKnowledge(ctx, item); err != nil {
			return request, err
		}
	}
	conflict, err := s.store.KnowledgeConflict(ctx, actor.TenantID, request.ConflictID)
	if err != nil {
		return request, err
	}
	conflict.Status = "resolved"
	conflict.ResolvedBy = actor.UserID
	conflict.ResolvedAt = &now
	conflict.Resolution = selectedKnowledgeID
	conflict.UpdatedAt = now
	if err := s.store.SaveKnowledgeConflict(ctx, conflict); err != nil {
		return request, err
	}
	request.Status = "resolved"
	request.ResolvedBy = actor.UserID
	request.ResolvedAt = &now
	request.SelectedKnowledgeID = selectedKnowledgeID
	request.Notes = strings.TrimSpace(notes)
	if err := s.store.SaveDecisionRequest(ctx, request); err != nil {
		return request, err
	}
	s.audit(ctx, actor, request.ProjectID, "decision_request.resolved", "decision_request", request.ID, auditRequestID, map[string]any{"conflict_id": request.ConflictID, "selected_knowledge_id": selectedKnowledgeID})
	return request, nil
}
