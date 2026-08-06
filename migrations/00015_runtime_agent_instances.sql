-- +goose Up

CREATE UNIQUE INDEX runtime_node_runs_scope_id_idx ON runtime_node_runs(tenant_id,job_run_id,id);

CREATE TABLE runtime_context_views (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  attempt_id text NOT NULL,
  schema_version text NOT NULL CHECK (schema_version = 'contentcloud.context-view/1.0'),
  input_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_refs) = 'array'),
  state_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(state_refs) = 'array'),
  event_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(event_refs) = 'array'),
  allowed_tools jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allowed_tools) = 'array'),
  max_tokens integer NOT NULL CHECK (max_tokens > 0),
  budget_minor bigint NOT NULL DEFAULT 0 CHECK (budget_minor >= 0),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL CHECK (expires_at > created_at),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,digest),
  UNIQUE (tenant_id,node_run_id,attempt_id),
  UNIQUE (tenant_id,job_run_id,node_run_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_context_views_job_idx ON runtime_context_views(tenant_id,job_run_id,created_at);

CREATE TABLE runtime_agent_instances (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  parent_agent_instance_id text,
  role text NOT NULL,
  harness_kind text NOT NULL,
  session_ref text NOT NULL DEFAULT '',
  execution_profile_id text NOT NULL,
  context_view_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('created','runnable','active','waiting_children','waiting_gate','waiting_effect','completed','failed','canceling','cancelled')),
  depth integer NOT NULL DEFAULT 0 CHECK (depth >= 0),
  remaining_descendants integer NOT NULL DEFAULT 0 CHECK (remaining_descendants >= 0),
  budget_minor bigint NOT NULL DEFAULT 0 CHECK (budget_minor >= 0),
  used_cost_minor bigint NOT NULL DEFAULT 0 CHECK (used_cost_minor >= 0 AND used_cost_minor <= budget_minor),
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,context_view_id),
  UNIQUE (tenant_id,job_run_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id,context_view_id) REFERENCES runtime_context_views(tenant_id,job_run_id,node_run_id,id),
  FOREIGN KEY (tenant_id,job_run_id,parent_agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,id)
);
CREATE INDEX runtime_agent_instances_job_idx ON runtime_agent_instances(tenant_id,job_run_id,created_at);
CREATE INDEX runtime_agent_instances_parent_idx ON runtime_agent_instances(tenant_id,parent_agent_instance_id) WHERE parent_agent_instance_id IS NOT NULL;

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_context_views','runtime_agent_instances'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

-- +goose Down

DROP TABLE IF EXISTS runtime_agent_instances;
DROP TABLE IF EXISTS runtime_context_views;
DROP INDEX IF EXISTS runtime_node_runs_scope_id_idx;
