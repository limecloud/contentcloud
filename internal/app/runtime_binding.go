package app

import (
	"context"
	"strings"

	"github.com/limecloud/contentcloud/internal/domain"
	"github.com/limecloud/contentcloud/internal/environment"
	contentruntime "github.com/limecloud/contentcloud/internal/runtime"
)

type runtimeExecutionBindingInput struct {
	TenantID        string
	ProjectID       string
	EnvironmentID   string
	ContentTypes    []string
	RuntimePolicyID string
}

func (s *Service) buildRuntimeExecutionBinding(ctx context.Context, input runtimeExecutionBindingInput) (domain.ExecutionBindingSnapshot, error) {
	policyID := strings.TrimSpace(input.RuntimePolicyID)
	binding := domain.ExecutionBindingSnapshot{
		TenantID: input.TenantID, SchemaVersion: domain.ExecutionBindingSnapshotSchema,
		ProfileID: policyID, ProfileVersion: "1", RuntimePolicyID: policyID,
		HarnessKinds: []string{}, AllowedTools: []string{
			contentruntime.ToolChildList, contentruntime.ToolEffectStatus,
			contentruntime.ToolStateGet, contentruntime.ToolStateQuery,
		},
		SandboxProfile: "any", IsolationProfile: "workspace", EgressPolicy: "declared",
		DataClassification: "internal", MaxTokens: 8192, MaxDurationSeconds: 3600,
		MaxCostMinor: 0, MaxDynamicDescendants: domain.DefaultRuntimeLimits().MaxDynamicDescendants,
		FallbackPolicy: "none", CreatedAt: s.now().UTC(),
	}
	if binding.ProfileID == "" {
		binding.ProfileID = contentruntime.DefaultRuntimePolicyID
		binding.RuntimePolicyID = contentruntime.DefaultRuntimePolicyID
	}

	if environmentID := strings.TrimSpace(input.EnvironmentID); environmentID != "" {
		value, err := s.store.Environment(ctx, input.TenantID, environmentID)
		if err != nil {
			return domain.ExecutionBindingSnapshot{}, err
		}
		binding.EnvironmentID = value.ID
		binding.EnvironmentDigest = value.ManifestDigest
	}
	if s.environmentControl == nil {
		return binding, nil
	}

	manifest, err := s.environmentControl.Issue(input.ProjectID, input.ContentTypes, s.now().UTC())
	if err != nil {
		return domain.ExecutionBindingSnapshot{}, err
	}
	binding.ProfileID = manifest.ProfileID
	binding.ProfileVersion = manifest.ProfileVersion
	binding.HarnessKinds = []string{manifest.Harness}
	binding.WorkspaceTemplateID = manifest.WorkspaceTemplate.ID
	binding.WorkspaceDigest = manifest.WorkspaceTemplate.Digest
	declarations, err := environment.DigestsForManifest(manifest)
	if err != nil {
		return domain.ExecutionBindingSnapshot{}, err
	}
	binding.EnvironmentDigest = declarations.Environment
	binding.PluginDigest = declarations.Plugin
	binding.SkillDigest = declarations.Skill
	binding.MCPDigest = declarations.MCP
	binding.ProfileDigest, err = runtimeBindingDigest(struct {
		ProfileID         string                           `json:"profile_id"`
		ProfileVersion    string                           `json:"profile_version"`
		Harness           string                           `json:"harness"`
		Distribution      environment.Distribution         `json:"distribution"`
		WorkspaceTemplate environment.WorkspaceTemplateRef `json:"workspace_template"`
		Capabilities      []string                         `json:"capabilities"`
		Policies          environment.Policies             `json:"policies"`
	}{manifest.ProfileID, manifest.ProfileVersion, manifest.Harness, manifest.Distribution, manifest.WorkspaceTemplate, manifest.Capabilities, manifest.Policies})
	if err != nil {
		return domain.ExecutionBindingSnapshot{}, err
	}
	return binding, nil
}

func runtimeBindingDigest(value any) (string, error) {
	hash, err := domain.CanonicalHash(value)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (s *Service) projectRuntimeEnvironmentID(ctx context.Context, tenantID, projectID string) (string, error) {
	binding, err := s.store.ProjectSOPBinding(ctx, tenantID, projectID)
	if domain.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return binding.EnvironmentID, nil
}
