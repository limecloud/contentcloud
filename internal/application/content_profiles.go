package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/platform/fault"
	"github.com/limecloud/contentcloud/internal/platform/idgen"

	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	contentprofile "github.com/limecloud/contentcloud/internal/catalog/profile"
)

type ContentProfileInstallResult struct {
	Profile contentprofile.Profile   `json:"profile"`
	SOP     catalogdomain.SOPSummary `json:"sop"`
}

func (s *CatalogService) ContentProfiles() []contentprofile.Profile {
	return contentprofile.List()
}

func (s *CatalogService) InstallContentProfile(ctx context.Context, actor Actor, profileID, requestID string) (ContentProfileInstallResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return ContentProfileInstallResult{}, err
	}
	profile, ok := contentprofile.Get(profileID)
	if !ok {
		return ContentProfileInstallResult{}, fault.NotFound("内容 Profile")
	}
	if err := profile.Validate(); err != nil {
		return ContentProfileInstallResult{}, err
	}

	sourceRef := contentProfileSourceRef(profile)
	summaries, err := s.catalog.SOPs(ctx, actor.TenantID)
	if err != nil {
		return ContentProfileInstallResult{}, err
	}
	for _, summary := range summaries {
		if summary.Definition.SourceRef == sourceRef && hasPublishedSOPVersion(summary) {
			return ContentProfileInstallResult{Profile: profile, SOP: summary}, nil
		}
	}

	sopID := "content-profile-" + profile.ID
	var existing *catalogdomain.SOPSummary
	for index := range summaries {
		if summaries[index].Definition.ID == sopID {
			existing = &summaries[index]
			break
		}
	}
	now := s.now().UTC()
	versionNumber := 1
	if existing != nil {
		for _, version := range existing.Versions {
			if version.Version >= versionNumber {
				versionNumber = version.Version + 1
			}
		}
	}
	version := compileContentProfile(profile, actor, sopID, versionNumber, now)
	definition := catalogdomain.SOPDefinition{
		ID:           sopID,
		TenantID:     actor.TenantID,
		Name:         profile.Name,
		Description:  profile.Description,
		ContentTypes: append([]string{}, profile.ContentTypes...),
		SourceRef:    sourceRef,
		CreatedBy:    actor.UserID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	definition.NormalizeCollections()
	version.NormalizeCollections()
	if err := version.Validate(); err != nil {
		return ContentProfileInstallResult{}, err
	}

	if existing == nil {
		if err := s.catalog.CreateSOP(ctx, definition, version); err != nil {
			return ContentProfileInstallResult{}, err
		}
	} else {
		definition = existing.Definition
		definition.Name = profile.Name
		definition.Description = profile.Description
		definition.ContentTypes = append([]string{}, profile.ContentTypes...)
		definition.SourceRef = sourceRef
		definition.UpdatedAt = now
		definition.NormalizeCollections()
		if err := s.catalog.SaveSOPDefinition(ctx, definition); err != nil {
			return ContentProfileInstallResult{}, err
		}
		if err := s.catalog.CreateSOPVersion(ctx, version); err != nil {
			return ContentProfileInstallResult{}, err
		}
	}

	if _, err := s.app.Work.PublishSOP(ctx, actor, sopID, versionNumber, requestID); err != nil {
		return ContentProfileInstallResult{}, err
	}
	summary, err := s.catalog.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return ContentProfileInstallResult{}, err
	}
	s.audit(ctx, actor, "", "content_profile.installed", "sop", sopID, requestID, map[string]any{"profile_id": profile.ID, "profile_version": profile.Version, "profile_digest": profile.Digest, "sop_version": versionNumber})
	return ContentProfileInstallResult{Profile: profile, SOP: summary}, nil
}

func contentProfileSourceRef(profile contentprofile.Profile) string {
	return fmt.Sprintf("content-profile:%s@%s#%s", profile.ID, profile.Version, profile.Digest)
}

func hasPublishedSOPVersion(summary catalogdomain.SOPSummary) bool {
	for _, version := range summary.Versions {
		if version.Status == "published" {
			return true
		}
	}
	return false
}

func compileContentProfile(profile contentprofile.Profile, actor Actor, sopID string, versionNumber int, now time.Time) catalogdomain.SOPVersion {
	stages := make([]catalogdomain.StageDefinition, 0, len(profile.Stages))
	for index, stage := range profile.Stages {
		executionModes := []string{"local", "agent", "runtime"}
		executorPolicy := "capability_routed"
		if stage.Deterministic {
			executionModes = []string{"local", "runtime"}
			executorPolicy = "deterministic_worker"
		}
		stages = append(stages, catalogdomain.StageDefinition{
			ID:                   stage.ID,
			Name:                 stage.Name,
			Order:                (index + 1) * 10,
			OwnerRoles:           []string{"editor", "project_manager"},
			InputRefs:            append([]string{}, stage.InputRefs...),
			OutputSchema:         stage.OutputSchema,
			OutputSchemaRefs:     []string{stage.OutputSchema},
			RequiredCapabilities: []string{stage.CapabilityID},
			AllowedExecutorKinds: append([]string{}, stage.ExecutorKinds...),
			ExecutionModes:       executionModes,
			Checks:               append([]string{}, stage.Checks...),
			GateIDs:              append([]string{}, stage.GateIDs...),
			RetryMaxAttempts:     stage.RetryMaxAttempts,
			CompletionPolicy:     catalogdomain.StageCompletionAllRequired,
			ExecutorPolicy:       executorPolicy,
			RetryPolicy:          catalogdomain.StageRetryPolicy{MaxAttempts: stage.RetryMaxAttempts},
		})
	}
	gates := make([]catalogdomain.GateDefinition, 0, len(profile.RequiredGates))
	for _, gateID := range profile.RequiredGates {
		mode := catalogdomain.GateModeInternalReview
		roles := []string{"reviewer", "project_manager"}
		if gateID == "offer_valid" || gateID == "canon_consistency" {
			mode = catalogdomain.GateModeRequiredCheck
			roles = []string{}
		} else if strings.Contains(gateID, "preview") {
			mode = catalogdomain.GateModeClientDecision
			roles = []string{"client_approver", "tenant_admin"}
		}
		gates = append(gates, catalogdomain.GateDefinition{ID: gateID, Name: contentProfileGateName(gateID), Mode: mode, Blocking: true, AssigneeRoles: roles, Checks: []string{gateID}, OnReject: "changes_requested"})
	}
	return catalogdomain.SOPVersion{
		ID:                   idgen.New(),
		TenantID:             actor.TenantID,
		SOPID:                sopID,
		Version:              versionNumber,
		SchemaVersion:        catalogdomain.SOPSchemaVersion,
		Name:                 profile.Name,
		Description:          profile.Description,
		ContentTypes:         append([]string{}, profile.ContentTypes...),
		Stages:               stages,
		Gates:                gates,
		DefaultExecutionMode: "agent",
		Status:               "draft",
		CreatedBy:            actor.UserID,
		CreatedAt:            now,
	}
}

func contentProfileGateName(id string) string {
	return strings.ReplaceAll(id, "_", " ")
}
