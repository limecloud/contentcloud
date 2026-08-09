-- +goose Up

CREATE TABLE runtime_projection_snapshots (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  job_run_id text NOT NULL,
  job jsonb NOT NULL CHECK (jsonb_typeof(job) = 'object'),
  nodes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(nodes) = 'array'),
  last_event_sequence bigint NOT NULL DEFAULT 0 CHECK (last_event_sequence >= 0),
  source_event_id text NOT NULL,
  projected_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,job_run_id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_projection_snapshots_projected_idx ON runtime_projection_snapshots(tenant_id,projected_at);
ALTER TABLE runtime_projection_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_projection_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_projection_snapshots USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS runtime_projection_snapshots;
