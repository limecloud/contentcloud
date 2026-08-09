-- +goose Up

CREATE TABLE runtime_maintenance_heartbeats (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('runtime_reaper','runtime_delivery')),
  worker_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('running','succeeded','failed')),
  last_started_at timestamptz NOT NULL,
  last_success_at timestamptz,
  last_error_code text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,kind),
  CHECK (state <> 'succeeded' OR last_success_at IS NOT NULL)
);
CREATE INDEX runtime_maintenance_heartbeats_health_idx
  ON runtime_maintenance_heartbeats(kind,last_success_at,tenant_id);

ALTER TABLE runtime_maintenance_heartbeats ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_maintenance_heartbeats FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_maintenance_heartbeats
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE ON runtime_maintenance_heartbeats TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS runtime_maintenance_heartbeats;
