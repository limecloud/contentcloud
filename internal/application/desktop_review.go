package application

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
)

// DesktopReviewInboxItem is the read model used by the Desktop approval work
// surface. It deliberately contains only the current Revision and review
// counters; the full object payload is loaded by DesktopReviewRevision.
type DesktopReviewInboxItem struct {
	Submission      reviewdomain.Submission         `json:"submission"`
	Revision        reviewdomain.SubmissionRevision `json:"revision"`
	PendingComments int                             `json:"pending_comments"`
	AllowedActions  []string                        `json:"allowed_actions"`
}

type DesktopReviewInbox struct {
	ProjectID string                   `json:"project_id"`
	Items     []DesktopReviewInboxItem `json:"items"`
}

type DesktopReviewObjectDiff struct {
	ObjectID       string `json:"object_id"`
	ObjectType     string `json:"object_type"`
	Path           string `json:"path"`
	Change         string `json:"change"`
	BaseDigest     string `json:"base_digest,omitempty"`
	CurrentDigest  string `json:"current_digest,omitempty"`
	BaseContent    string `json:"base_content,omitempty"`
	CurrentContent string `json:"current_content,omitempty"`
}

type DesktopReviewRevisionDetail struct {
	Submission       reviewdomain.Submission          `json:"submission"`
	Revision         reviewdomain.SubmissionRevision  `json:"revision"`
	PreviousRevision *reviewdomain.SubmissionRevision `json:"previous_revision,omitempty"`
	Comments         []reviewdomain.ReviewComment     `json:"comments"`
	Diffs            []DesktopReviewObjectDiff        `json:"diffs"`
	AllowedActions   []string                         `json:"allowed_actions"`
}

type DesktopProjectProjection struct {
	ProjectID       string `json:"project_id"`
	ReviewState     string `json:"review_state"`
	RuntimeState    string `json:"runtime_state"`
	LifecycleState  string `json:"lifecycle_state"`
	PendingFeedback int    `json:"pending_feedback"`
	PendingDecision int    `json:"pending_decision"`
}

func (s *ReviewService) DesktopProjectProjection(ctx context.Context, actor Actor, projectID string) (DesktopProjectProjection, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, projectID, false); err != nil {
		return DesktopProjectProjection{}, err
	}
	projection := DesktopProjectProjection{ProjectID: projectID, ReviewState: "unsubmitted", RuntimeState: "succeeded", LifecycleState: "draft"}
	submissions, err := s.review.Submissions(ctx, actor.TenantID, projectID)
	if err != nil {
		return DesktopProjectProjection{}, err
	}
	for _, submission := range submissions {
		switch submission.Status {
		case "submitted", "in_review", "internally_approved", "client_review":
			projection.ReviewState = "pending"
		case "changes_requested":
			projection.ReviewState = "changes_requested"
		case "rejected":
			projection.ReviewState = "rejected"
		case "approved":
			if projection.ReviewState == "unsubmitted" || projection.ReviewState == "approved" {
				projection.ReviewState = "approved"
			}
		}
	}
	if s.app.Runtime != nil {
		runs, runErr := s.app.Runtime.Runs(ctx, actor, projectID)
		if runErr != nil {
			return DesktopProjectProjection{}, runErr
		}
		if len(runs) > 0 {
			projection.RuntimeState = runs[0].State
		}
	}
	packages, err := s.DeliveryPackages(ctx, actor, projectID)
	if err != nil {
		return DesktopProjectProjection{}, err
	}
	if len(packages) > 0 {
		projection.LifecycleState = "delivered"
	}
	return projection, nil
}

type DesktopReviewCommentInput struct {
	ProjectID   string `json:"project_id"`
	RevisionID  string `json:"revision_id"`
	Body        string `json:"body"`
	JSONPointer string `json:"json_pointer,omitempty"`
}

type DesktopReviewDecisionInput struct {
	ProjectID   string `json:"project_id"`
	RevisionID  string `json:"revision_id"`
	Reason      string `json:"reason"`
	JSONPointer string `json:"json_pointer,omitempty"`
}

func (s *ReviewService) DesktopReviewInbox(ctx context.Context, actor Actor, projectID string) (DesktopReviewInbox, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, projectID, false); err != nil {
		return DesktopReviewInbox{}, err
	}
	submissions, err := s.review.Submissions(ctx, actor.TenantID, projectID)
	if err != nil {
		return DesktopReviewInbox{}, err
	}
	items := make([]DesktopReviewInboxItem, 0, len(submissions))
	effectiveActor, _ := s.reviewActorWithRole(ctx, actor)
	for _, submission := range submissions {
		if submission.CurrentRevisionID == "" {
			continue
		}
		revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, submission.CurrentRevisionID)
		if err != nil {
			return DesktopReviewInbox{}, err
		}
		comments, err := s.review.ReviewComments(ctx, actor.TenantID, revision.ID)
		if err != nil {
			return DesktopReviewInbox{}, err
		}
		pending := 0
		for _, comment := range comments {
			if comment.ResolvedAt == nil {
				pending++
			}
		}
		items = append(items, DesktopReviewInboxItem{
			Submission: submission, Revision: revision, PendingComments: pending,
			AllowedActions: desktopReviewActions(effectiveActor, submission.Status, effectiveActor.Role),
		})
	}
	return DesktopReviewInbox{ProjectID: projectID, Items: items}, nil
}

func (s *ReviewService) DesktopReviewRevision(ctx context.Context, actor Actor, projectID, revisionID string) (DesktopReviewRevisionDetail, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, projectID, false); err != nil {
		return DesktopReviewRevisionDetail{}, err
	}
	view, err := s.ProjectSubmissionRevision(ctx, actor, projectID, revisionID)
	if err != nil {
		return DesktopReviewRevisionDetail{}, err
	}
	revisions, err := s.review.SubmissionRevisions(ctx, actor.TenantID, view.Submission.ID)
	if err != nil {
		return DesktopReviewRevisionDetail{}, err
	}
	var previous *reviewdomain.SubmissionRevision
	for index := range revisions {
		candidate := revisions[index]
		if candidate.RevisionNo < view.Revision.RevisionNo && (previous == nil || candidate.RevisionNo > previous.RevisionNo) {
			copy := candidate
			previous = &copy
		}
	}
	effectiveActor, _ := s.reviewActorWithRole(ctx, actor)
	return DesktopReviewRevisionDetail{
		Submission: view.Submission, Revision: view.Revision, PreviousRevision: previous,
		Comments: view.Comments, Diffs: desktopReviewDiff(previous, view.Revision),
		AllowedActions: desktopReviewActions(effectiveActor, view.Submission.Status, effectiveActor.Role),
	}, nil
}

func (s *ReviewService) AddDesktopReviewComment(ctx context.Context, actor Actor, input DesktopReviewCommentInput, requestID string) (reviewdomain.ReviewComment, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, input.ProjectID, true); err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		return reviewdomain.ReviewComment{}, fault.Invalid("COMMENT_BODY_REQUIRED", "批注内容不能为空")
	}
	if input.JSONPointer != "" && !reviewdomain.ValidJSONPointer(input.JSONPointer) {
		return reviewdomain.ReviewComment{}, fault.Invalid("COMMENT_POINTER_INVALID", "批注位置必须使用合法的 JSON 指针")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, input.RevisionID)
	if err != nil || revision.ProjectID != input.ProjectID {
		return reviewdomain.ReviewComment{}, fault.NotFound("提交内容版本")
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil || submission.ProjectID != input.ProjectID || submission.CurrentRevisionID != revision.ID {
		return reviewdomain.ReviewComment{}, fault.Conflict("SUBMISSION_REVISION_STALE", "批注只能添加到当前提交内容版本")
	}
	cycle, err := s.openReviewCycle(ctx, actor, revision)
	if err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	comment := reviewdomain.ReviewComment{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, ReviewCycleID: cycle.ID, SubjectType: "submission_revision", SubjectID: revision.ID, JSONPointer: input.JSONPointer, Body: body, Visibility: "internal", AuthorID: actor.UserID, CreatedAt: s.now().UTC()}
	if err := s.review.CreateReviewComment(ctx, comment); err != nil {
		return reviewdomain.ReviewComment{}, err
	}
	s.audit(ctx, actor, input.ProjectID, "review_comment.created", "review_comment", comment.ID, requestID, map[string]any{"submission_revision_id": revision.ID, "json_pointer": input.JSONPointer})
	return comment, nil
}

func (s *ReviewService) DesktopApprove(ctx context.Context, actor Actor, input DesktopReviewDecisionInput, requestID string) (SubmissionApprovalResult, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, input.ProjectID, true); err != nil {
		return SubmissionApprovalResult{}, err
	}
	return s.ApproveSubmission(ctx, actor, input.RevisionID, input.Reason, requestID)
}

func (s *ReviewService) DesktopRequestChanges(ctx context.Context, actor Actor, input DesktopReviewDecisionInput, requestID string) (reviewdomain.Submission, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, input.ProjectID, true); err != nil {
		return reviewdomain.Submission{}, err
	}
	return s.RequestSubmissionChanges(ctx, actor, input.RevisionID, input.Reason, input.JSONPointer, requestID)
}

func (s *ReviewService) DesktopReject(ctx context.Context, actor Actor, input DesktopReviewDecisionInput, requestID string) (reviewdomain.Submission, error) {
	if err := s.requireDesktopReviewActor(ctx, actor, input.ProjectID, true); err != nil {
		return reviewdomain.Submission{}, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return reviewdomain.Submission{}, fault.Invalid("REJECT_REASON_REQUIRED", "拒绝必须填写原因")
	}
	if input.JSONPointer != "" && !reviewdomain.ValidJSONPointer(input.JSONPointer) {
		return reviewdomain.Submission{}, fault.Invalid("COMMENT_POINTER_INVALID", "批注位置必须使用合法的 JSON 指针")
	}
	revision, err := s.review.SubmissionRevision(ctx, actor.TenantID, input.RevisionID)
	if err != nil {
		return reviewdomain.Submission{}, err
	}
	submission, err := s.review.Submission(ctx, actor.TenantID, revision.SubmissionID)
	if err != nil {
		return reviewdomain.Submission{}, err
	}
	if submission.ProjectID != input.ProjectID || submission.CurrentRevisionID != revision.ID || (submission.Status != "submitted" && submission.Status != "in_review" && submission.Status != "internally_approved" && submission.Status != "client_review") {
		return submission, fault.Conflict("SUBMISSION_STATE_INVALID", "只能拒绝当前待审核的提交内容版本")
	}
	cycle, err := s.openReviewCycle(ctx, actor, revision)
	if err != nil {
		return submission, err
	}
	now := s.now().UTC()
	comment := reviewdomain.ReviewComment{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, ReviewCycleID: cycle.ID, SubjectType: "submission_revision", SubjectID: revision.ID, JSONPointer: input.JSONPointer, Body: reason, Visibility: "internal", AuthorID: actor.UserID, CreatedAt: now}
	decision := reviewdomain.ApprovalDecision{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: input.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, SubjectHash: revision.ContentHash, DecisionStage: "internal", ActorID: actor.UserID, Decision: "reject", Reason: reason, PreviousState: submission.Status, ResultingState: "rejected", CreatedAt: now}
	submission.Status = "rejected"
	submission.UpdatedAt = now
	if err := s.review.RejectSubmission(ctx, submission, decision, comment); err != nil {
		return submission, err
	}
	s.audit(ctx, actor, input.ProjectID, "submission.rejected", "submission_revision", revision.ID, requestID, map[string]any{"json_pointer": input.JSONPointer})
	return submission, nil
}

func (s *ReviewService) requireDesktopReviewActor(ctx context.Context, actor Actor, projectID string, write bool) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || actor.TenantID == "" {
		return fault.Policy("DESKTOP_REVIEW_SCOPE_INVALID", "审批请求缺少项目范围", "重新加载 Desktop 项目")
	}
	var err error
	actor, err = s.reviewActorWithRole(ctx, actor)
	if err != nil {
		return err
	}
	if actor.Type == "device" {
		if !containsString(actor.ProjectIDs, projectID) {
			return fault.Policy("DEVICE_PROJECT_ACCESS_DENIED", "当前设备未绑定此项目", "在项目设置中重新绑定设备")
		}
	}
	if actor.Type != "user" && actor.Type != "device" {
		return fault.Policy("DESKTOP_REVIEW_ACTOR_INVALID", "只有已授权用户设备可以访问审批工作面", "使用已登录用户绑定的 Desktop")
	}
	if write {
		if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
			return err
		}
	}
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return err
	}
	return nil
}

func (s *ReviewService) reviewActorWithRole(ctx context.Context, actor Actor) (Actor, error) {
	if actor.Type == "device" {
		membership, err := s.identity.Membership(ctx, actor.TenantID, actor.UserID)
		if err != nil {
			return Actor{}, err
		}
		actor.Role = membership.Role
	}
	if actor.Type != "user" && actor.Type != "device" {
		return Actor{}, fault.Policy("DESKTOP_REVIEW_ACTOR_INVALID", "只有已授权用户设备可以访问审批工作面", "使用已登录用户绑定的 Desktop")
	}
	return actor, nil
}

func desktopReviewActions(actor Actor, status, role string) []string {
	if actor.Type == "device" && role == "" {
		return nil
	}
	actions := []string{"comment"}
	if status == "submitted" || status == "in_review" || status == "internally_approved" || status == "client_review" {
		if role == "tenant_admin" || role == "project_manager" || role == "reviewer" {
			actions = append(actions, "approve", "reject", "request_changes")
		}
	}
	return actions
}

func (s *ReviewService) openReviewCycle(ctx context.Context, actor Actor, revision reviewdomain.SubmissionRevision) (reviewdomain.ReviewCycle, error) {
	cycles, err := s.review.ReviewCycles(ctx, actor.TenantID, revision.ID)
	if err != nil {
		return reviewdomain.ReviewCycle{}, err
	}
	if len(cycles) > 0 && cycles[0].Status == "open" {
		return cycles[0], nil
	}
	now := s.now().UTC()
	return s.review.CreateReviewCycle(ctx, reviewdomain.ReviewCycle{ID: idgen.New(), TenantID: actor.TenantID, ProjectID: revision.ProjectID, SubjectType: "submission_revision", SubjectID: revision.ID, Status: "open", OpenedBy: actor.UserID, OpenedAt: now, CreatedAt: now})
}

func desktopReviewDiff(previous *reviewdomain.SubmissionRevision, current reviewdomain.SubmissionRevision) []DesktopReviewObjectDiff {
	base := map[string]reviewdomain.SubmissionObjectRef{}
	if previous != nil {
		for _, object := range previous.Objects {
			base[object.ID] = object
		}
	}
	seen := map[string]bool{}
	diffs := make([]DesktopReviewObjectDiff, 0, len(current.Objects)+len(base))
	for _, object := range current.Objects {
		seen[object.ID] = true
		old, ok := base[object.ID]
		change := "added"
		if ok {
			change = "unchanged"
			if old.Digest != object.Digest {
				change = "modified"
			}
		}
		diff := DesktopReviewObjectDiff{ObjectID: object.ID, ObjectType: object.Type, Path: object.Path, Change: change, CurrentDigest: object.Digest, CurrentContent: string(object.Content)}
		if ok {
			diff.BaseDigest, diff.BaseContent = old.Digest, string(old.Content)
		}
		diffs = append(diffs, diff)
	}
	for id, object := range base {
		if !seen[id] {
			diffs = append(diffs, DesktopReviewObjectDiff{ObjectID: object.ID, ObjectType: object.Type, Path: object.Path, Change: "removed", BaseDigest: object.Digest, BaseContent: string(object.Content)})
		}
	}
	return diffs
}
