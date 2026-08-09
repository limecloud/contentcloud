-- +goose Up

ALTER TABLE runtime_node_runs DROP CONSTRAINT runtime_node_runs_state_check;
ALTER TABLE runtime_node_runs ADD CONSTRAINT runtime_node_runs_state_check CHECK (state IN ('pending','ready','waiting_resource','leased','running','waiting_children','waiting_external','waiting_human','succeeded','retryable_failed','failed','blocked','skipped','cancelled','lease_expired'));

ALTER TABLE runtime_attempts DROP CONSTRAINT runtime_attempts_state_check;
ALTER TABLE runtime_attempts DROP CONSTRAINT runtime_attempts_check;
ALTER TABLE runtime_attempts ADD CONSTRAINT runtime_attempts_state_check CHECK (state IN ('prepared','running','yielded','succeeded','retryable_failed','failed','cancelled','expired'));
ALTER TABLE runtime_attempts ADD CONSTRAINT runtime_attempts_lifecycle_check CHECK (
  (state IN ('prepared','running') AND lease_owner <> '' AND lease_expires_at IS NOT NULL AND finished_at IS NULL)
  OR
  (state IN ('yielded','succeeded','retryable_failed','failed','cancelled','expired') AND lease_owner = '' AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
);

CREATE TABLE runtime_yields (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  attempt_id text NOT NULL,
  agent_instance_id text NOT NULL,
  reason text NOT NULL CHECK (reason IN ('wait_children','wait_human','wait_effect')),
  wait_refs jsonb NOT NULL CHECK (jsonb_typeof(wait_refs) = 'array' AND jsonb_array_length(wait_refs) > 0),
  state text NOT NULL CHECK (state IN ('open','resolved')),
  resume_key text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  yielded_at timestamptz NOT NULL,
  resolved_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,attempt_id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,attempt_id) REFERENCES runtime_attempts(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,id) ON DELETE CASCADE,
  CHECK ((state = 'open' AND resume_key = '' AND resolved_at IS NULL) OR (state = 'resolved' AND resume_key <> '' AND resolved_at IS NOT NULL))
);
CREATE UNIQUE INDEX runtime_yields_resume_key_idx ON runtime_yields(tenant_id,resume_key) WHERE resume_key <> '';
CREATE INDEX runtime_yields_open_idx ON runtime_yields(tenant_id,job_run_id,yielded_at) WHERE state = 'open';

ALTER TABLE runtime_yields ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_yields FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_yields USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS runtime_yields;
ALTER TABLE runtime_attempts DROP CONSTRAINT IF EXISTS runtime_attempts_lifecycle_check;
ALTER TABLE runtime_attempts DROP CONSTRAINT IF EXISTS runtime_attempts_state_check;
ALTER TABLE runtime_attempts ADD CONSTRAINT runtime_attempts_state_check CHECK (state IN ('prepared','running','succeeded','retryable_failed','failed','cancelled','expired'));
ALTER TABLE runtime_attempts ADD CONSTRAINT runtime_attempts_check CHECK (
  (state IN ('prepared','running') AND lease_owner <> '' AND lease_expires_at IS NOT NULL AND finished_at IS NULL)
  OR
  (state IN ('succeeded','retryable_failed','failed','cancelled','expired') AND lease_owner = '' AND lease_expires_at IS NULL AND finished_at IS NOT NULL)
);
ALTER TABLE runtime_node_runs DROP CONSTRAINT IF EXISTS runtime_node_runs_state_check;
ALTER TABLE runtime_node_runs ADD CONSTRAINT runtime_node_runs_state_check CHECK (state IN ('pending','ready','waiting_resource','leased','running','waiting_external','waiting_human','succeeded','retryable_failed','failed','blocked','skipped','cancelled','lease_expired'));
