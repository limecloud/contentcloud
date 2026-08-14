package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/limecloud/contentcloud/internal/domain"
)

func (s *Store) CreateExecutionBindingSnapshot(ctx context.Context, value domain.ExecutionBindingSnapshot) error {
	value.NormalizeCollections()
	if err := value.Validate(); err != nil {
		return err
	}
	return s.withTenant(ctx, value.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO runtime_execution_binding_snapshots(tenant_id,digest,schema_version,profile_id,profile_version,profile_digest,runtime_policy_id,harness_kinds,provider_ref,model_ref,environment_id,environment_digest,plugin_digest,skill_digest,mcp_digest,allowed_tools,sandbox_profile,isolation_profile,egress_policy,region,data_classification,max_tokens,max_duration_seconds,max_cost_minor,max_dynamic_descendants,fallback_policy,workspace_template_id,workspace_digest,legacy,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`, value.TenantID, value.Digest, value.SchemaVersion, value.ProfileID, value.ProfileVersion, value.ProfileDigest, value.RuntimePolicyID, jsonArrayValue(value.HarnessKinds), value.ProviderRef, value.ModelRef, value.EnvironmentID, value.EnvironmentDigest, value.PluginDigest, value.SkillDigest, value.MCPDigest, jsonArrayValue(value.AllowedTools), value.SandboxProfile, value.IsolationProfile, value.EgressPolicy, value.Region, value.DataClassification, value.MaxTokens, value.MaxDurationSeconds, value.MaxCostMinor, value.MaxDynamicDescendants, value.FallbackPolicy, value.WorkspaceTemplateID, value.WorkspaceDigest, value.Legacy, value.CreatedAt)
		return dbError(err)
	})
}

func (s *Store) ExecutionBindingSnapshot(ctx context.Context, tenantID, digest string) (domain.ExecutionBindingSnapshot, error) {
	var value domain.ExecutionBindingSnapshot
	err := s.withTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var harnessKinds, allowedTools []byte
		err := tx.QueryRow(ctx, `SELECT tenant_id,digest,schema_version,profile_id,profile_version,profile_digest,runtime_policy_id,harness_kinds,provider_ref,model_ref,environment_id,environment_digest,plugin_digest,skill_digest,mcp_digest,allowed_tools,sandbox_profile,isolation_profile,egress_policy,region,data_classification,max_tokens,max_duration_seconds,max_cost_minor,max_dynamic_descendants,fallback_policy,workspace_template_id,workspace_digest,legacy,created_at FROM runtime_execution_binding_snapshots WHERE tenant_id=$1 AND digest=$2`, tenantID, digest).Scan(&value.TenantID, &value.Digest, &value.SchemaVersion, &value.ProfileID, &value.ProfileVersion, &value.ProfileDigest, &value.RuntimePolicyID, &harnessKinds, &value.ProviderRef, &value.ModelRef, &value.EnvironmentID, &value.EnvironmentDigest, &value.PluginDigest, &value.SkillDigest, &value.MCPDigest, &allowedTools, &value.SandboxProfile, &value.IsolationProfile, &value.EgressPolicy, &value.Region, &value.DataClassification, &value.MaxTokens, &value.MaxDurationSeconds, &value.MaxCostMinor, &value.MaxDynamicDescendants, &value.FallbackPolicy, &value.WorkspaceTemplateID, &value.WorkspaceDigest, &value.Legacy, &value.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NotFound("ExecutionBindingSnapshot")
		}
		if err != nil {
			return err
		}
		value.HarnessKinds, err = decodeJSON[[]string](harnessKinds)
		if err == nil {
			value.AllowedTools, err = decodeJSON[[]string](allowedTools)
		}
		return err
	})
	return value, err
}
