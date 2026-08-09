-- +goose Up

CREATE TABLE runtime_fanout_sets (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  map_node_key text NOT NULL,
  join_node_key text NOT NULL,
  source_collection text NOT NULL DEFAULT '',
  source_revision integer NOT NULL DEFAULT 0 CHECK (source_revision >= 0),
  source_watermark bigint NOT NULL DEFAULT 0 CHECK (source_watermark >= 0),
  generation integer NOT NULL CHECK (generation > 0),
  idempotency_key text NOT NULL,
  membership_digest text NOT NULL CHECK (membership_digest ~ '^sha256:[0-9a-f]{64}$'),
  request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
  member_count integer NOT NULL CHECK (member_count >= 0),
  join_strategy text NOT NULL CHECK (join_strategy IN ('all','min_success','quorum','best_effort','fail_fast')),
  min_success integer NOT NULL DEFAULT 0 CHECK (min_success >= 0),
  quorum_percent integer NOT NULL DEFAULT 0 CHECK (quorum_percent >= 0 AND quorum_percent <= 100),
  zero_member_policy text NOT NULL CHECK (zero_member_policy IN ('fail','succeed_empty')),
  quorum_stop_policy text NOT NULL DEFAULT '' CHECK (quorum_stop_policy IN ('','wait_all_terminal','cancel_pending')),
  status text NOT NULL CHECK (status IN ('open','closed','succeeded','failed')),
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  closed_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,job_run_id,idempotency_key),
  UNIQUE (tenant_id,job_run_id,map_node_key,generation),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  CHECK ((status = 'open' AND closed_at IS NULL) OR (status <> 'open' AND closed_at IS NOT NULL)),
  CHECK ((join_strategy = 'min_success' AND min_success > 0) OR (join_strategy <> 'min_success' AND min_success = 0)),
  CHECK ((join_strategy = 'quorum' AND quorum_percent > 0 AND quorum_stop_policy <> '') OR (join_strategy <> 'quorum' AND quorum_percent = 0 AND quorum_stop_policy = ''))
);

CREATE TABLE runtime_fanout_members (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  fanout_set_id text NOT NULL,
  member_key text NOT NULL,
  item_key text NOT NULL,
  item_digest text NOT NULL,
  generation integer NOT NULL CHECK (generation > 0),
  node_run_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','running','succeeded','failed','cancelled','skipped')),
  output_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(output_refs) = 'array'),
  output_digest text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,fanout_set_id,member_key),
  FOREIGN KEY (tenant_id,fanout_set_id) REFERENCES runtime_fanout_sets(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX runtime_fanout_sets_job_idx ON runtime_fanout_sets(tenant_id,job_run_id,created_at);
CREATE INDEX runtime_fanout_members_node_idx ON runtime_fanout_members(tenant_id,node_run_id);

ALTER TABLE runtime_fanout_sets ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_fanout_sets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_fanout_sets USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE runtime_fanout_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_fanout_members FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_fanout_members USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
REVOKE UPDATE,DELETE ON runtime_fanout_sets,runtime_fanout_members FROM contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS runtime_fanout_members;
DROP TABLE IF EXISTS runtime_fanout_sets;
