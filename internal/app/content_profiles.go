package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/limecloud/contentcloud/internal/contentprofile"
	"github.com/limecloud/contentcloud/internal/domain"
)

type ContentProfileInstallResult struct {
	Profile contentprofile.Profile `json:"profile"`
	SOP     domain.SOPSummary      `json:"sop"`
}

func (s *Service) ContentProfiles() []contentprofile.Profile {
	return contentprofile.List()
}

func (s *Service) InstallContentProfile(ctx context.Context, actor Actor, profileID, requestID string) (ContentProfileInstallResult, error) {
	if err := requireRole(actor, "tenant_admin", "project_manager"); err != nil && !actor.PlatformAdmin {
		return ContentProfileInstallResult{}, err
	}
	profile, ok := contentprofile.Get(profileID)
	if !ok {
		return ContentProfileInstallResult{}, domain.NotFound("内容 Profile")
	}
	if err := profile.Validate(); err != nil {
		return ContentProfileInstallResult{}, err
	}

	sourceRef := contentProfileSourceRef(profile)
	summaries, err := s.store.SOPs(ctx, actor.TenantID)
	if err != nil {
		return ContentProfileInstallResult{}, err
	}
	for _, summary := range summaries {
		if summary.Definition.SourceRef == sourceRef && hasPublishedSOPVersion(summary) {
			return ContentProfileInstallResult{Profile: profile, SOP: summary}, nil
		}
	}

	sopID := "content-profile-" + profile.ID
	var existing *domain.SOPSummary
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
	definition := domain.SOPDefinition{
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
		if err := s.store.CreateSOP(ctx, definition, version); err != nil {
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
		if err := s.store.SaveSOPDefinition(ctx, definition); err != nil {
			return ContentProfileInstallResult{}, err
		}
		if err := s.store.CreateSOPVersion(ctx, version); err != nil {
			return ContentProfileInstallResult{}, err
		}
	}

	if _, err := s.PublishSOP(ctx, actor, sopID, versionNumber, requestID); err != nil {
		return ContentProfileInstallResult{}, err
	}
	summary, err := s.store.SOP(ctx, actor.TenantID, sopID)
	if err != nil {
		return ContentProfileInstallResult{}, err
	}
	s.audit(ctx, actor, "", "content_profile.installed", "sop", sopID, requestID, map[string]any{"profile_id": profile.ID, "profile_version": profile.Version, "profile_digest": profile.Digest, "sop_version": versionNumber})
	return ContentProfileInstallResult{Profile: profile, SOP: summary}, nil
}

func contentProfileSourceRef(profile contentprofile.Profile) string {
	return fmt.Sprintf("content-profile:%s@%s#%s", profile.ID, profile.Version, profile.Digest)
}

func hasPublishedSOPVersion(summary domain.SOPSummary) bool {
	for _, version := range summary.Versions {
		if version.Status == "published" {
			return true
		}
	}
	return false
}

func compileContentProfile(profile contentprofile.Profile, actor Actor, sopID string, versionNumber int, now time.Time) domain.SOPVersion {
	stages := make([]domain.StageDefinition, 0, len(profile.Stages))
	for index, stage := range profile.Stages {
		executionModes := []string{"local", "agent", "runtime"}
		executorPolicy := "capability_routed"
		if stage.Deterministic {
			executionModes = []string{"local", "runtime"}
			executorPolicy = "deterministic_worker"
		}
		stages = append(stages, domain.StageDefinition{
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
			CompletionPolicy:     domain.StageCompletionAllRequired,
			ExecutorPolicy:       executorPolicy,
			RetryPolicy:          domain.StageRetryPolicy{MaxAttempts: stage.RetryMaxAttempts},
		})
	}
	gates := make([]domain.GateDefinition, 0, len(profile.RequiredGates))
	for _, gateID := range profile.RequiredGates {
		mode := domain.GateModeInternalReview
		roles := []string{"reviewer", "project_manager"}
		if gateID == "offer_valid" || gateID == "canon_consistency" {
			mode = domain.GateModeRequiredCheck
			roles = []string{}
		} else if strings.Contains(gateID, "preview") {
			mode = domain.GateModeClientDecision
			roles = []string{"client_approver", "tenant_admin"}
		}
		gates = append(gates, domain.GateDefinition{ID: gateID, Name: contentProfileGateName(gateID), Mode: mode, Blocking: true, AssigneeRoles: roles, Checks: []string{gateID}, OnReject: "changes_requested"})
	}
	return domain.SOPVersion{
		ID:                   domain.NewID(),
		TenantID:             actor.TenantID,
		SOPID:                sopID,
		Version:              versionNumber,
		SchemaVersion:        domain.SOPSchemaVersion,
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
