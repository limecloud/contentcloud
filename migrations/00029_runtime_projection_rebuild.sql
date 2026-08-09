-- +goose Up

CREATE TABLE runtime_projection_rebuild_runs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  mode text NOT NULL CHECK (mode IN ('rebuild','dry_run')),
  status text NOT NULL CHECK (status IN ('running','completed','failed')),
  event_count integer NOT NULL DEFAULT 0 CHECK (event_count >= 0),
  last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
  external_calls integer NOT NULL DEFAULT 0 CHECK (external_calls = 0),
  integrity_status text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  started_at timestamptz NOT NULL,
  finished_at timestamptz,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  CHECK ((status = 'running' AND finished_at IS NULL) OR (status IN ('completed','failed') AND finished_at IS NOT NULL))
);
CREATE INDEX runtime_projection_rebuild_runs_job_idx ON runtime_projection_rebuild_runs(tenant_id,job_run_id,started_at DESC);
ALTER TABLE runtime_projection_rebuild_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_projection_rebuild_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_projection_rebuild_runs USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS runtime_projection_rebuild_runs;
