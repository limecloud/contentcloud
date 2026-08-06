-- +goose Up

CREATE TABLE runtime_job_plans (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  sop_id text NOT NULL,
  sop_version integer NOT NULL CHECK (sop_version > 0),
  sop_digest text NOT NULL,
  schema_version text NOT NULL,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  nodes jsonb NOT NULL CHECK (jsonb_typeof(nodes) = 'array'),
  edges jsonb NOT NULL CHECK (jsonb_typeof(edges) = 'array'),
  customer_steps jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(customer_steps) = 'array'),
  limits jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(limits) = 'object'),
  compiled_at timestamptz NOT NULL,
  compiled_by text NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,digest)
);

CREATE TABLE runtime_job_runs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  work_task_id text NOT NULL,
  plan_revision_id text NOT NULL,
  plan_digest text NOT NULL CHECK (plan_digest ~ '^sha256:[0-9a-f]{64}$'),
  source_job_run_id text NOT NULL DEFAULT '',
  checkpoint_id text NOT NULL DEFAULT '',
  idempotency_key text NOT NULL DEFAULT '',
  state text NOT NULL CHECK (state IN ('created','admitted','running','waiting_human','paused','completed','failed','cancelled','rejected')),
  priority integer NOT NULL DEFAULT 0,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  error_code text NOT NULL DEFAULT '',
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,work_task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,plan_revision_id) REFERENCES runtime_job_plans(tenant_id,id)
);
CREATE UNIQUE INDEX runtime_job_runs_idempotency_idx ON runtime_job_runs(tenant_id,idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX runtime_job_runs_task_idx ON runtime_job_runs(tenant_id,work_task_id,created_at DESC);

CREATE TABLE runtime_node_runs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_key text NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','ready','waiting_resource','leased','running','waiting_external','waiting_human','succeeded','retryable_failed','failed','blocked','skipped','cancelled','lease_expired')),
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  output_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(output_refs) = 'array'),
  output_digest text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  lease_owner text NOT NULL DEFAULT '',
  lease_expires_at timestamptz,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,job_run_id,node_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_node_runs_ready_idx ON runtime_node_runs(tenant_id,state,updated_at);

CREATE TABLE runtime_job_events (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  sequence bigint NOT NULL CHECK (sequence > 0),
  type text NOT NULL,
  node_key text NOT NULL DEFAULT '',
  actor_type text NOT NULL,
  actor_id text NOT NULL DEFAULT '',
  correlation_id text NOT NULL DEFAULT '',
  idempotency_key text NOT NULL DEFAULT '',
  payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
  occurred_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,job_run_id,sequence),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX runtime_job_events_idempotency_idx ON runtime_job_events(tenant_id,job_run_id,idempotency_key) WHERE idempotency_key <> '';

CREATE TABLE runtime_states (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  collection text NOT NULL,
  schema_version text NOT NULL,
  revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
  values jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(values) = 'object'),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,job_run_id,collection),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE runtime_state_mutations (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  job_run_id text NOT NULL,
  collection text NOT NULL,
  idempotency_key text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,job_run_id,collection,idempotency_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE runtime_checkpoints (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_key text NOT NULL,
  plan_digest text NOT NULL CHECK (plan_digest ~ '^sha256:[0-9a-f]{64}$'),
  state_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(state_refs) = 'array'),
  output_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(output_refs) = 'array'),
  completed_nodes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(completed_nodes) = 'array'),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE runtime_effects (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  kind text NOT NULL,
  idempotency_key text NOT NULL,
  state text NOT NULL CHECK (state IN ('registered','submitted','acknowledged','succeeded','failed','unknown','reconciling','manual_action')),
  external_id text NOT NULL DEFAULT '',
  request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
  response_digest text NOT NULL DEFAULT '',
  cost_minor bigint NOT NULL DEFAULT 0 CHECK (cost_minor >= 0),
  currency text NOT NULL DEFAULT 'CNY' CHECK (currency ~ '^[A-Z]{3}$'),
  safe_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_summary) = 'object'),
  error_code text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_job_plans','runtime_job_runs','runtime_node_runs','runtime_job_events','runtime_states','runtime_state_mutations','runtime_checkpoints','runtime_effects'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

-- +goose Down

DROP TABLE IF EXISTS runtime_effects;
DROP TABLE IF EXISTS runtime_checkpoints;
DROP TABLE IF EXISTS runtime_state_mutations;
DROP TABLE IF EXISTS runtime_states;
DROP TABLE IF EXISTS runtime_job_events;
DROP TABLE IF EXISTS runtime_node_runs;
DROP TABLE IF EXISTS runtime_job_runs;
DROP TABLE IF EXISTS runtime_job_plans;
