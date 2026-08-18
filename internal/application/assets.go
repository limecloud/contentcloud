package application

import (
	"context"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
)

var allowedAssetTypes = map[string]bool{
	"product_image": true,
	"brand_mark":    true,
	"packaging":     true,
	"person":        true,
	"location":      true,
	"audio":         true,
	"other":         true,
}

var allowedUsageModes = map[string]bool{
	"analysis_only":        true,
	"generation_reference": true,
	"owned":                true,
}

var allowedRightsTypes = map[string]bool{
	"owned":               true,
	"licensed_generation": true,
	"licensed_edit":       true,
	"public_domain":       true,
}

type CreateAssetInput struct {
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	AssetType        string `json:"asset_type"`
	SourceRevisionID string `json:"source_revision_id"`
	UsageMode        string `json:"usage_mode"`
}

func (s *ReviewService) CreateAsset(ctx context.Context, actor Actor, input CreateAssetInput, requestID string) (sourcedomain.Asset, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "strategist", "editor"); err != nil {
		return sourcedomain.Asset{}, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, input.ProjectID); err != nil {
		return sourcedomain.Asset{}, err
	}
	if strings.TrimSpace(input.Name) == "" || !allowedAssetTypes[input.AssetType] || !allowedUsageModes[input.UsageMode] {
		return sourcedomain.Asset{}, fault.Invalid("ASSET_FIELDS_INVALID", "素材名称、类型或使用模式无效")
	}
	revision, err := s.source.SourceRevision(ctx, actor.TenantID, input.SourceRevisionID)
	if err != nil {
		return sourcedomain.Asset{}, err
	}
	if revision.ProjectID != input.ProjectID || revision.ProcessingStatus != "ready" {
		return sourcedomain.Asset{}, fault.Policy("ASSET_SOURCE_BLOCKED", "素材原件必须来自当前项目的就绪来源版本", "完成来源解析和复核后重试")
	}
	now := s.now().UTC()
	asset := sourcedomain.Asset{
		ID:               idgen.New(),
		TenantID:         actor.TenantID,
		ProjectID:        input.ProjectID,
		Name:             strings.TrimSpace(input.Name),
		AssetType:        input.AssetType,
		SourceRevisionID: input.SourceRevisionID,
		UsageMode:        input.UsageMode,
		Status:           "needs_review",
		CreatedBy:        actor.UserID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.source.CreateAsset(ctx, asset); err != nil {
		return asset, err
	}
	s.audit(ctx, actor, asset.ProjectID, "asset.created", "asset", asset.ID, requestID, map[string]any{"asset_type": asset.AssetType, "usage_mode": asset.UsageMode, "source_revision_id": asset.SourceRevisionID})
	return asset, nil
}

func (s *ReviewService) Assets(ctx context.Context, actor Actor, projectID string) ([]sourcedomain.Asset, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.source.Assets(ctx, actor.TenantID, projectID)
}

func (s *ReviewService) Asset(ctx context.Context, actor Actor, id string) (sourcedomain.Asset, error) {
	return s.source.Asset(ctx, actor.TenantID, id)
}

type CreateRightsRecordInput struct {
	AssetID               string     `json:"asset_id"`
	RightsHolder          string     `json:"rights_holder"`
	RightsType            string     `json:"rights_type"`
	Territories           []string   `json:"territories"`
	Channels              []string   `json:"channels"`
	ValidFrom             *time.Time `json:"valid_from"`
	ValidUntil            *time.Time `json:"valid_until"`
	ProofSourceRevisionID string     `json:"proof_source_revision_id"`
	Restrictions          []string   `json:"restrictions"`
}

func (s *ReviewService) CreateRightsRecord(ctx context.Context, actor Actor, input CreateRightsRecordInput, requestID string) (sourcedomain.RightsRecord, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager", "reviewer"); err != nil {
		return sourcedomain.RightsRecord{}, err
	}
	asset, err := s.source.Asset(ctx, actor.TenantID, input.AssetID)
	if err != nil {
		return sourcedomain.RightsRecord{}, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, asset.ProjectID); err != nil {
		return sourcedomain.RightsRecord{}, err
	}
	if strings.TrimSpace(input.RightsHolder) == "" || !allowedRightsTypes[input.RightsType] || len(input.Territories) == 0 || len(input.Channels) == 0 || strings.TrimSpace(input.ProofSourceRevisionID) == "" {
		return sourcedomain.RightsRecord{}, fault.Invalid("RIGHTS_FIELDS_INVALID", "权利主体、类型、地域、渠道和证明来源必填")
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && !input.ValidUntil.After(*input.ValidFrom) {
		return sourcedomain.RightsRecord{}, fault.Invalid("RIGHTS_PERIOD_INVALID", "权利结束时间必须晚于开始时间")
	}
	proof, err := s.source.SourceRevision(ctx, actor.TenantID, input.ProofSourceRevisionID)
	if err != nil {
		return sourcedomain.RightsRecord{}, err
	}
	if proof.ProjectID != asset.ProjectID || proof.ProcessingStatus != "ready" {
		return sourcedomain.RightsRecord{}, fault.Policy("RIGHTS_PROOF_BLOCKED", "权利证明必须来自当前项目的就绪来源版本", "上传并解析权利证明后重试")
	}
	now := s.now().UTC()
	record := sourcedomain.RightsRecord{
		ID:                    idgen.New(),
		TenantID:              actor.TenantID,
		ProjectID:             asset.ProjectID,
		AssetID:               asset.ID,
		RightsHolder:          strings.TrimSpace(input.RightsHolder),
		RightsType:            input.RightsType,
		Territories:           uniqueNonEmpty(input.Territories),
		Channels:              uniqueNonEmpty(input.Channels),
		ValidFrom:             input.ValidFrom,
		ValidUntil:            input.ValidUntil,
		ProofSourceRevisionID: input.ProofSourceRevisionID,
		Restrictions:          uniqueNonEmpty(input.Restrictions),
		Status:                "needs_review",
		RowVersion:            1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if len(record.Territories) == 0 || len(record.Channels) == 0 {
		return record, fault.Invalid("RIGHTS_SCOPE_INVALID", "地域和渠道不能只包含空值")
	}
	if err := s.source.CreateRightsRecord(ctx, record); err != nil {
		return record, err
	}
	s.audit(ctx, actor, record.ProjectID, "rights.created", "rights_record", record.ID, requestID, map[string]any{"asset_id": record.AssetID, "rights_type": record.RightsType})
	return record, nil
}

func (s *ReviewService) RightsRecords(ctx context.Context, actor Actor, assetID string) ([]sourcedomain.RightsRecord, error) {
	if _, err := s.source.Asset(ctx, actor.TenantID, assetID); err != nil {
		return nil, err
	}
	return s.source.RightsRecords(ctx, actor.TenantID, assetID)
}

func (s *ReviewService) ReviewRightsRecord(ctx context.Context, actor Actor, id, decision, requestID string) (sourcedomain.RightsRecord, error) {
	if err := requireRole(actor, "tenant_admin", "reviewer"); err != nil {
		return sourcedomain.RightsRecord{}, err
	}
	record, err := s.source.RightsRecord(ctx, actor.TenantID, id)
	if err != nil {
		return record, err
	}
	if _, err := s.app.Identity.projectForWrite(ctx, actor, record.ProjectID); err != nil {
		return record, err
	}
	previous := record.Status
	switch decision {
	case "approve":
		record.Status = "approved"
	case "reject":
		record.Status = "rejected"
	default:
		return record, fault.Invalid("RIGHTS_DECISION_INVALID", "权利审核只允许“批准（approve）”或“拒绝（reject）”")
	}
	now := s.now().UTC()
	if record.Status == "approved" && record.ValidUntil != nil && !record.ValidUntil.After(now) {
		record.Status = "expired"
	}
	record.ReviewedBy = actor.UserID
	record.ReviewedAt = &now
	record.RowVersion++
	record.UpdatedAt = now
	if err := s.source.SaveRightsRecord(ctx, record); err != nil {
		return record, err
	}
	asset, err := s.source.Asset(ctx, actor.TenantID, record.AssetID)
	if err != nil {
		return record, err
	}
	records, listErr := s.source.RightsRecords(ctx, actor.TenantID, asset.ID)
	if listErr != nil {
		return record, listErr
	}
	asset.Status = aggregateRightsStatus(records)
	asset.UpdatedAt = now
	if err := s.source.SaveAsset(ctx, asset); err != nil {
		return record, err
	}
	s.audit(ctx, actor, record.ProjectID, "rights.reviewed", "rights_record", record.ID, requestID, map[string]any{"from": previous, "to": record.Status, "asset_id": record.AssetID})
	return record, nil
}

func aggregateRightsStatus(records []sourcedomain.RightsRecord) string {
	status := "rejected"
	for _, record := range records {
		if record.Status == "approved" {
			return "approved"
		}
		if record.Status == "needs_review" || record.Status == "review_required" {
			status = "needs_review"
		} else if record.Status == "expired" && status == "rejected" {
			status = "expired"
		}
	}
	return status
}

func (s *ReviewService) EligibleAssets(ctx context.Context, actor Actor, projectID, channel string) ([]sourcedomain.AssetBundle, error) {
	if _, err := s.workspace.Project(ctx, actor.TenantID, projectID); err != nil {
		return nil, err
	}
	return s.eligibleAssets(ctx, actor.TenantID, projectID, channel, s.now().UTC())
}

func (s *ReviewService) eligibleAssets(ctx context.Context, tenantID, projectID, channel string, now time.Time) ([]sourcedomain.AssetBundle, error) {
	assets, err := s.source.Assets(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	bundles := []sourcedomain.AssetBundle{}
	for _, asset := range assets {
		if asset.Status != "approved" || asset.UsageMode == "analysis_only" {
			continue
		}
		records, err := s.source.RightsRecords(ctx, tenantID, asset.ID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if rightsUsable(record, channel, now) {
				bundles = append(bundles, sourcedomain.AssetBundle{Asset: asset, Rights: record})
				break
			}
		}
	}
	return bundles, nil
}

func rightsUsable(record sourcedomain.RightsRecord, channel string, now time.Time) bool {
	if record.Status != "approved" || (!containsString(record.Territories, "CN") && !containsString(record.Territories, "*")) || (!containsString(record.Channels, channel) && !containsString(record.Channels, "*")) {
		return false
	}
	if record.ValidFrom != nil && now.Before(*record.ValidFrom) {
		return false
	}
	return record.ValidUntil == nil || now.Before(*record.ValidUntil)
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
