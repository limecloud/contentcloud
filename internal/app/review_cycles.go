package app

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
)

type ReviewScriptInput struct {
	Decision       string `json:"decision"`
	Conclusion     string `json:"conclusion"`
	AssigneeUserID string `json:"assignee_user_id"`
}

func (s *Service) ReviewCycles(ctx context.Context, actor Actor, scriptVersionID string) ([]domain.ReviewCycle, error) {
	if _, err := s.store.Script(ctx, actor.TenantID, scriptVersionID); err != nil {
		return nil, err
	}
	return s.store.ReviewCycles(ctx, actor.TenantID, scriptVersionID)
}

func (s *Service) ensureReviewCycle(ctx context.Context, actor Actor, script domain.ScriptVersion) (domain.ReviewCycle, bool, error) {
	cycles, err := s.store.ReviewCycles(ctx, actor.TenantID, script.ID)
	if err != nil {
		return domain.ReviewCycle{}, false, err
	}
	if len(cycles) > 0 && cycles[0].Status == "open" {
		return cycles[0], false, nil
	}
	now := s.now().UTC()
	openedBy := actor.UserID
	if openedBy == "" {
		openedBy = actor.DeviceID
	}
	cycle := domain.ReviewCycle{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: script.ProjectID, SubjectType: "script_version", SubjectID: script.ID, Status: "open", OpenedBy: openedBy, OpenedAt: now, CreatedAt: now}
	cycle, err = s.store.CreateReviewCycle(ctx, cycle)
	if err != nil {
		return cycle, false, err
	}
	if script.BaselineID != "" {
		comments, _ := s.store.ReviewComments(ctx, actor.TenantID, script.BaselineID)
		for _, comment := range comments {
			if comment.ResolvedAt != nil {
				continue
			}
			carried := comment
			carried.ID = domain.NewID()
			carried.ReviewCycleID = cycle.ID
			carried.SubjectID = script.ID
			carried.CarriedFromID = comment.ID
			carried.CreatedAt = now
			if err := s.store.CreateReviewComment(ctx, carried); err != nil {
				return cycle, true, err
			}
		}
	}
	return cycle, true, nil
}

func (s *Service) ReviewScriptWithInput(ctx context.Context, actor Actor, id string, input ReviewScriptInput, requestID string) (domain.ScriptVersion, error) {
	v, err := s.store.Script(ctx, actor.TenantID, id)
	if err != nil {
		return v, err
	}
	if _, err := s.projectForWrite(ctx, actor, v.ProjectID); err != nil {
		return v, err
	}
	input.Decision = strings.TrimSpace(input.Decision)
	input.Conclusion = strings.TrimSpace(input.Conclusion)
	input.AssigneeUserID = strings.TrimSpace(input.AssigneeUserID)
	cycle, _, err := s.ensureReviewCycle(ctx, actor, v)
	if err != nil {
		return v, err
	}
	prev := v.Status
	next := ""
	switch input.Decision {
	case "submit":
		if err := requireRole(actor, "tenant_admin", "project_manager", "editor"); err != nil {
			return v, err
		}
		if v.Status != "review_ready" {
			return v, domain.Conflict("SCRIPT_STATE_INVALID", "只有 review_ready 剧本可提交内审")
		}
		next = "internal_review"
	case "approve_internal":
		if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
			return v, err
		}
		if v.Status != "internal_review" {
			return v, domain.Conflict("SCRIPT_STATE_INVALID", "只有内审中剧本可批准")
		}
		if input.Conclusion == "" {
			return v, domain.Invalid("REVIEW_CONCLUSION_REQUIRED", "内审批准必须填写整版结论")
		}
		if err := s.requireResolvedComments(ctx, actor.TenantID, v.ID, ""); err != nil {
			return v, err
		}
		next = "internally_approved"
		cycle.Status = "approved"
	case "return":
		if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
			return v, err
		}
		if v.Status != "internal_review" {
			return v, domain.Conflict("SCRIPT_STATE_INVALID", "只有内审中剧本可退回")
		}
		if input.Conclusion == "" || input.AssigneeUserID == "" {
			return v, domain.Invalid("REVIEW_RETURN_FIELDS_REQUIRED", "内审退回必须填写整版结论和修改责任人")
		}
		membership, err := s.store.Membership(ctx, actor.TenantID, input.AssigneeUserID)
		if err != nil || membership.Status != "active" {
			return v, domain.Policy("REVIEW_ASSIGNEE_INVALID", "修改责任人不是当前租户有效成员", "选择有效团队成员")
		}
		next = "revision_requested"
		cycle.Status = "changes_requested"
		cycle.AssigneeUserID = input.AssigneeUserID
	default:
		return v, domain.Invalid("DECISION_INVALID", "内部审核只允许 submit、approve_internal 或 return")
	}
	v.Status = next
	if err := s.store.SaveScript(ctx, v); err != nil {
		return v, err
	}
	if input.Decision != "submit" {
		now := s.now().UTC()
		cycle.Conclusion = input.Conclusion
		cycle.DecidedBy = actor.UserID
		cycle.DecidedAt = &now
		if err := s.store.SaveReviewCycle(ctx, cycle); err != nil {
			return v, err
		}
	}
	approval := domain.ApprovalDecision{ID: domain.NewID(), TenantID: actor.TenantID, ProjectID: v.ProjectID, SubjectType: "script_version", SubjectID: v.ID, SubjectHash: v.ContentHash, ActorID: actor.UserID, Decision: input.Decision, Reason: input.Conclusion, PreviousState: prev, ResultingState: next, CreatedAt: s.now().UTC()}
	_ = s.store.CreateApproval(ctx, approval)
	s.audit(ctx, actor, v.ProjectID, "script.reviewed", "script_version", v.ID, requestID, map[string]any{"from": prev, "to": next, "decision": input.Decision, "review_cycle_id": cycle.ID})
	return v, nil
}

func (s *Service) requireResolvedComments(ctx context.Context, tenantID, scriptVersionID, visibility string) error {
	comments, err := s.store.ReviewComments(ctx, tenantID, scriptVersionID)
	if err != nil {
		return err
	}
	for _, comment := range comments {
		if comment.ResolvedAt == nil && (visibility == "" || comment.Visibility == visibility) {
			return domain.Policy("REVIEW_COMMENTS_UNRESOLVED", "仍有未解决审核批注，不能批准", "先解决所有适用批注")
		}
	}
	return nil
}
