package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/projectview"
)

var projectionSectionNames = []string{
	"onboarding",
	"methodology",
	"knowledge",
	"intelligence",
	"strategy",
	"planning",
	"creative",
	"review",
	"delivery",
	"learning",
	"automation",
}

func (s *Service) ProjectProjection(ctx context.Context, actor Actor, projectID string) (domain.ProjectProjection, error) {
	project, err := s.store.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	submissions, err := s.store.Submissions(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	snapshots, err := s.store.ApprovedSnapshots(ctx, actor.TenantID, projectID, "")
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	runs, err := s.runtimeRunsForProject(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	deliveries, err := s.store.DeliveryPackages(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	results, err := s.store.PerformanceObservations(ctx, actor.TenantID, projectID)
	if err != nil {
		return domain.ProjectProjection{}, err
	}

	sections := make(map[string]domain.ProjectionSection, len(projectionSectionNames))
	for _, name := range projectionSectionNames {
		sections[name] = domain.ProjectionSection{Status: "empty"}
	}
	sections["onboarding"] = domain.ProjectionSection{Status: projectionStatus(project.ConnectedDevices, 0, 0), Count: project.ConnectedDevices}

	projectionSubmissions := make([]domain.ProjectionSubmission, 0, len(submissions))
	governance := domain.ProjectionGovernance{}
	for _, submission := range submissions {
		projectionSubmissions = append(projectionSubmissions, domain.ProjectionSubmission{ID: submission.ID, Type: submission.SubmissionType, Status: submission.Status, CurrentRevisionID: submission.CurrentRevisionID, UpdatedAt: submission.UpdatedAt})
		sectionName := projectionSectionForSubmission(submission.SubmissionType)
		section := sections[sectionName]
		section.Count++
		if section.UpdatedAt == nil || submission.UpdatedAt.After(*section.UpdatedAt) {
			updatedAt := submission.UpdatedAt
			section.UpdatedAt = &updatedAt
		}
		switch submission.Status {
		case "submitted", "in_review", "internally_approved", "client_review":
			section.Pending++
			governance.PendingReviews++
		case "changes_requested", "blocked":
			section.Blocked++
			if submission.Status == "changes_requested" {
				governance.ChangesRequested++
			}
			if submission.SubmissionType == "content_batch" {
				governance.BlockedContentBatches++
			}
		}
		section.Status = projectionStatus(section.Count, section.Pending, section.Blocked)
		sections[sectionName] = section
	}

	projectionSnapshots := make([]domain.ProjectionSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		projectionSnapshots = append(projectionSnapshots, domain.ProjectionSnapshot{ID: snapshot.ID, Type: snapshot.SubmissionType, SubmissionRevisionID: snapshot.SubmissionRevisionID, EligibleCount: len(snapshot.EligibleIDs), CreatedAt: snapshot.CreatedAt})
	}
	if len(deliveries) > 0 {
		sections["delivery"] = domain.ProjectionSection{Status: "ready", Count: len(deliveries), UpdatedAt: latestDeliveryTime(deliveries)}
	}
	if len(results) > 0 {
		sections["learning"] = domain.ProjectionSection{Status: "ready", Count: len(results), UpdatedAt: latestResultTime(results)}
	}
	activeRuns, failedRuns := 0, 0
	var latestRun *time.Time
	for _, run := range runs {
		if run.State == "queued" || run.State == "running" || run.State == "leased" {
			activeRuns++
		}
		if run.State == "failed" {
			failedRuns++
		}
		if latestRun == nil || run.UpdatedAt.After(*latestRun) {
			updatedAt := run.UpdatedAt
			latestRun = &updatedAt
		}
	}
	sections["automation"] = domain.ProjectionSection{Status: projectionStatus(len(runs), activeRuns, failedRuns), Count: len(runs), Pending: activeRuns, Blocked: failedRuns, UpdatedAt: latestRun}
	governance.ActiveAutomationRuns = activeRuns

	sort.Slice(projectionSubmissions, func(i, j int) bool {
		return projectionSubmissions[i].UpdatedAt.After(projectionSubmissions[j].UpdatedAt)
	})
	sort.Slice(projectionSnapshots, func(i, j int) bool { return projectionSnapshots[i].CreatedAt.After(projectionSnapshots[j].CreatedAt) })

	actions, err := s.projectionNextActions(ctx, actor.TenantID, project, sections, governance, projectionSubmissions)
	if err != nil {
		return domain.ProjectProjection{}, err
	}
	return domain.ProjectProjection{
		SchemaVersion: domain.ProjectProjectionSchemaVersion,
		Project:       domain.ProjectProjectionHeader{ID: project.ID, BrandName: project.BrandName, ProductName: project.ProductName, Channel: project.Channel, StageObjective: project.StageObjective, Status: project.Status},
		Sections:      sections, Governance: governance, Submissions: projectionSubmissions, Snapshots: projectionSnapshots, NextActions: actions, GeneratedAt: s.now().UTC(),
	}, nil
}

func projectionSectionForSubmission(submissionType string) string {
	switch submissionType {
	case "context":
		return "methodology"
	case "knowledge":
		return "knowledge"
	case "brief":
		return "planning"
	case "content_batch", "asset_batch":
		return "creative"
	case "delivery":
		return "delivery"
	case "result":
		return "learning"
	default:
		return "review"
	}
}

func projectionStatus(count, pending, blocked int) string {
	if blocked > 0 {
		return "blocked"
	}
	if pending > 0 {
		return "pending"
	}
	if count > 0 {
		return "ready"
	}
	return "empty"
}

func (s *Service) projectionNextActions(ctx context.Context, tenantID string, project domain.Project, sections map[string]domain.ProjectionSection, governance domain.ProjectionGovernance, submissions []domain.ProjectionSubmission) ([]domain.ProjectionAction, error) {
	if project.Status == "archived" {
		return projectionActions(domain.ProjectionAction{ID: "project-archived", Kind: "project", Label: "项目已归档", Enabled: false, Reason: "恢复项目后才能创建新的任务或内容提交", Navigation: navigation("home", "next_action", "project-archived", "")})
	}
	if sections["onboarding"].Count == 0 {
		return projectionActions(domain.ProjectionAction{ID: "initialize-workspace", Kind: "onboarding", Label: "连接执行客户端", Enabled: true, Navigation: navigation("connect", "", "", "")})
	}
	if sections["knowledge"].Count == 0 {
		return projectionActions(domain.ProjectionAction{ID: "collect-knowledge", Kind: "assignment", Label: "创建资料与知识任务", Enabled: true, Navigation: navigation("knowledge", "", "", "")})
	}
	if governance.PendingReviews > 0 {
		for _, submission := range submissions {
			if !pendingReviewStatus(submission.Status) {
				continue
			}
			revision, err := s.store.SubmissionRevision(ctx, tenantID, submission.CurrentRevisionID)
			if err != nil {
				return nil, err
			}
			if revision.ProjectID != project.ID || revision.SubmissionID != submission.ID {
				return nil, domain.Conflict("PROJECTION_REVISION_MISMATCH", "项目投影中的当前内容版本不属于目标项目或内容提交")
			}
			return projectionActions(domain.ProjectionAction{ID: "review-submission-" + revision.ID, Kind: "review", Label: "处理待审核内容版本", Enabled: true, Navigation: navigation("tasks", "submission_revision", revision.ID, projectNavigationDigest(revision.ContentHash))})
		}
		return nil, domain.Conflict("PROJECTION_REVIEW_TARGET_MISSING", "项目投影存在待审核计数，但没有可定位的当前内容版本")
	}
	return projectionActions(domain.ProjectionAction{ID: "create-assignment", Kind: "assignment", Label: "创建下一项创作任务", Enabled: true, Navigation: navigation("tasks", "", "", "")})
}

func pendingReviewStatus(status string) bool {
	switch status {
	case "submitted", "in_review", "internally_approved", "client_review":
		return true
	default:
		return false
	}
}

func navigation(view, focusKind, focusID, digest string) domain.ProjectNavigation {
	target := domain.ProjectNavigation{View: view}
	if focusKind != "" {
		target.Focus = &domain.ProjectNavigationFocus{Kind: focusKind, ID: focusID, Digest: digest}
	}
	return target
}

func projectNavigationDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		value = "sha256:" + value
	}
	return value
}

func projectionActions(action domain.ProjectionAction) ([]domain.ProjectionAction, error) {
	if err := projectview.Validate(action.Navigation); err != nil {
		return nil, err
	}
	return []domain.ProjectionAction{action}, nil
}

func latestDeliveryTime(values []domain.DeliveryPackage) *time.Time {
	var latest time.Time
	for _, value := range values {
		if value.CreatedAt.After(latest) {
			latest = value.CreatedAt
		}
	}
	return &latest
}

func latestResultTime(values []domain.PerformanceObservation) *time.Time {
	var latest time.Time
	for _, value := range values {
		if value.CreatedAt.After(latest) {
			latest = value.CreatedAt
		}
	}
	return &latest
}
