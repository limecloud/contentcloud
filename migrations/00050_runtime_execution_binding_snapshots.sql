-- +goose Up

CREATE TABLE runtime_execution_binding_snapshots (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  schema_version text NOT NULL,
  profile_id text NOT NULL,
  profile_version text NOT NULL,
  profile_digest text NOT NULL DEFAULT '',
  runtime_policy_id text NOT NULL,
  harness_kinds jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(harness_kinds) = 'array'),
  provider_ref text NOT NULL DEFAULT '',
  model_ref text NOT NULL DEFAULT '',
  environment_id text NOT NULL DEFAULT '',
  environment_digest text NOT NULL DEFAULT '',
  plugin_digest text NOT NULL DEFAULT '',
  skill_digest text NOT NULL DEFAULT '',
  mcp_digest text NOT NULL DEFAULT '',
  allowed_tools jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allowed_tools) = 'array'),
  sandbox_profile text NOT NULL,
  isolation_profile text NOT NULL,
  egress_policy text NOT NULL,
  region text NOT NULL DEFAULT '',
  data_classification text NOT NULL,
  max_tokens integer NOT NULL CHECK (max_tokens > 0),
  max_duration_seconds integer NOT NULL CHECK (max_duration_seconds > 0),
  max_cost_minor bigint NOT NULL CHECK (max_cost_minor >= 0),
  max_dynamic_descendants integer NOT NULL CHECK (max_dynamic_descendants >= 0),
  fallback_policy text NOT NULL,
  workspace_template_id text NOT NULL DEFAULT '',
  workspace_digest text NOT NULL DEFAULT '',
  legacy boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,digest)
);

ALTER TABLE runtime_execution_binding_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_execution_binding_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_execution_binding_snapshots
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

GRANT SELECT,INSERT ON runtime_execution_binding_snapshots TO contentcloud_runtime;

-- Existing JobRuns used an opaque binding digest before the structured
-- snapshot existed. Preserve those identities explicitly as legacy rows so
-- the foreign key can be added without rewriting historical execution facts.
INSERT INTO runtime_execution_binding_snapshots(
  tenant_id,digest,schema_version,profile_id,profile_version,runtime_policy_id,
  harness_kinds,allowed_tools,sandbox_profile,isolation_profile,egress_policy,
  data_classification,max_tokens,max_duration_seconds,max_cost_minor,
  max_dynamic_descendants,fallback_policy,legacy,created_at
)
SELECT
  jobs.tenant_id,
  jobs.binding_digest,
  'contentcloud.execution-binding/1.0',
  MIN(jobs.runtime_policy_id),
  'legacy',
  MIN(jobs.runtime_policy_id),
  '[]'::jsonb,
  '["child.list","effect.status","state.get","state.query"]'::jsonb,
  'legacy',
  'legacy',
  'legacy',
  'internal',
  8192,
  3600,
  COALESCE(MAX((plans.limits->>'max_cost_minor')::bigint),0),
  COALESCE(MAX((plans.limits->>'max_dynamic_descendants')::integer),100),
  'none',
  true,
  MIN(jobs.created_at)
FROM runtime_job_runs jobs
JOIN runtime_plan_revisions plans
  ON plans.tenant_id=jobs.tenant_id AND plans.id=jobs.plan_revision_id
GROUP BY jobs.tenant_id,jobs.binding_digest;

ALTER TABLE runtime_job_runs
  ADD CONSTRAINT runtime_job_runs_execution_binding_fk
  FOREIGN KEY (tenant_id,binding_digest)
  REFERENCES runtime_execution_binding_snapshots(tenant_id,digest);

-- +goose Down

ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_execution_binding_fk;
DROP TABLE IF EXISTS runtime_execution_binding_snapshots;
