-- +goose Up

CREATE UNIQUE INDEX runtime_agent_instances_scope_id_idx ON runtime_agent_instances(tenant_id,job_run_id,node_run_id,id);

CREATE TABLE runtime_attempts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  agent_instance_id text NOT NULL,
  context_view_id text NOT NULL,
  attempt_no integer NOT NULL CHECK (attempt_no > 0),
  harness_kind text NOT NULL,
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities) = 'object'),
  session_ref text NOT NULL DEFAULT '',
  state text NOT NULL CHECK (state IN ('prepared','running','succeeded','retryable_failed','failed','cancelled','expired')),
  lease_owner text NOT NULL DEFAULT '',
  lease_expires_at timestamptz,
  output_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(output_refs) = 'array'),
  result_digest text NOT NULL DEFAULT '',
  safe_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_summary) = 'object'),
  error_code text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  started_at timestamptz,
  finished_at timestamptz,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,node_run_id,attempt_no),
  UNIQUE (tenant_id,node_run_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id,context_view_id) REFERENCES runtime_context_views(tenant_id,job_run_id,node_run_id,id),
  FOREIGN KEY (tenant_id,job_run_id,node_run_id,agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,node_run_id,id),
  CHECK (
    (state IN ('prepared','running') AND lease_owner <> '' AND lease_expires_at IS NOT NULL AND finished_at IS NULL)
    OR
    (state IN ('succeeded','retryable_failed','failed','cancelled','expired') AND lease_owner = '' AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
  ),
  CHECK (state <> 'running' OR (session_ref <> '' AND started_at IS NOT NULL)),
  CHECK (state <> 'succeeded' OR result_digest <> '')
);
CREATE INDEX runtime_attempts_job_idx ON runtime_attempts(tenant_id,job_run_id,created_at,id);
CREATE INDEX runtime_attempts_active_lease_idx ON runtime_attempts(tenant_id,lease_expires_at) WHERE state IN ('prepared','running');

ALTER TABLE runtime_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_attempts USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS runtime_attempts;
DROP INDEX IF EXISTS runtime_agent_instances_scope_id_idx;
