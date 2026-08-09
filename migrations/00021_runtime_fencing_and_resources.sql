-- +goose Up

ALTER TABLE runtime_node_runs ADD COLUMN fence_token text NOT NULL DEFAULT '';
ALTER TABLE runtime_attempts ADD COLUMN fence_token text NOT NULL DEFAULT '';
ALTER TABLE runtime_node_runs ADD CONSTRAINT runtime_node_runs_active_fence_ck CHECK (state NOT IN ('leased','running') OR fence_token <> '');
ALTER TABLE runtime_attempts ADD CONSTRAINT runtime_attempts_active_fence_ck CHECK (state NOT IN ('prepared','running') OR fence_token <> '');

CREATE TABLE runtime_resource_quotas (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  resource_key text NOT NULL,
  capacity bigint NOT NULL CHECK (capacity >= 0),
  unit text NOT NULL,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, resource_key)
);

CREATE TABLE runtime_resource_reservations (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  attempt_id text NOT NULL,
  resource_key text NOT NULL,
  quantity bigint NOT NULL CHECK (quantity > 0),
  unit text NOT NULL,
  state text NOT NULL CHECK (state IN ('held','consumed','released','expired')),
  fence_token text NOT NULL DEFAULT '',
  idempotency_key text NOT NULL,
  expires_at timestamptz,
  released_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, id),
  UNIQUE (tenant_id, idempotency_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,attempt_id) REFERENCES runtime_attempts(tenant_id,id) ON DELETE CASCADE,
  CHECK ((state = 'held' AND fence_token <> '' AND expires_at IS NOT NULL AND released_at IS NULL) OR (state <> 'held' AND fence_token = '' AND expires_at IS NULL AND released_at IS NOT NULL))
);
CREATE INDEX runtime_resource_reservations_active_idx ON runtime_resource_reservations(tenant_id,resource_key,state,expires_at) WHERE state = 'held';
CREATE INDEX runtime_resource_reservations_attempt_idx ON runtime_resource_reservations(tenant_id,attempt_id);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_resource_quotas','runtime_resource_reservations'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

-- +goose Down

DROP TABLE IF EXISTS runtime_resource_reservations;
DROP TABLE IF EXISTS runtime_resource_quotas;
ALTER TABLE runtime_attempts DROP CONSTRAINT IF EXISTS runtime_attempts_active_fence_ck;
ALTER TABLE runtime_node_runs DROP CONSTRAINT IF EXISTS runtime_node_runs_active_fence_ck;
ALTER TABLE runtime_attempts DROP COLUMN IF EXISTS fence_token;
ALTER TABLE runtime_node_runs DROP COLUMN IF EXISTS fence_token;
