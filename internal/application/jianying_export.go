package application

import (
	"context"
	"strings"

	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	exportfmt "github.com/limecloud/contentcloud/internal/local/export"
	"github.com/limecloud/contentcloud/internal/platform/fault"
)

// JianyingExportInput references existing governed facts. Export does not
// create a second delivery, publication, or media project fact.
type JianyingExportInput struct {
	ProjectID          string                             `json:"project_id"`
	ApprovedSnapshotID string                             `json:"approved_snapshot_id"`
	FinalReviewID      string                             `json:"final_review_id"`
	DeliveryPackageID  string                             `json:"delivery_package_id"`
	Manifest           deliverydomain.CompositionManifest `json:"manifest"`
}

func (s *DeliveryService) ExportJianying(ctx context.Context, actor Actor, input JianyingExportInput) (exportfmt.JianyingExportResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "editor", "reviewer"); err != nil {
		return exportfmt.JianyingExportResult{}, err
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return exportfmt.JianyingExportResult{}, fault.Invalid("JIANYING_EXPORT_PROJECT_REQUIRED", "剪映导出必须指定项目")
	}
	project, err := s.workspace.Project(ctx, actor.TenantID, projectID)
	if err != nil {
		return exportfmt.JianyingExportResult{}, err
	}
	if project.TenantID != actor.TenantID {
		return exportfmt.JianyingExportResult{}, fault.NotFound("项目")
	}
	return exportfmt.ExportJianying(ctx, exportfmt.JianyingReaders{
		Snapshots: s.review,
		Reviews:   s.delivery,
		Packages:  s.artifacts,
		Artifacts: s.artifacts,
		Blobs:     s.blobs,
	}, exportfmt.JianyingExportInput{
		TenantID: actor.TenantID, ProjectID: projectID,
		ApprovedSnapshotID: strings.TrimSpace(input.ApprovedSnapshotID),
		FinalReviewID:      strings.TrimSpace(input.FinalReviewID), DeliveryPackageID: strings.TrimSpace(input.DeliveryPackageID),
		Manifest: input.Manifest,
	})
}
