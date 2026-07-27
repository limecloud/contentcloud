-- +goose Up

CREATE TABLE creative_execution_bundles (
  run_id uuid PRIMARY KEY REFERENCES task_runs(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  bundle_id text NOT NULL,
  digest text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id,bundle_id)
);

CREATE INDEX creative_execution_bundles_project_idx ON creative_execution_bundles(tenant_id,project_id,created_at DESC);

ALTER TABLE creative_execution_bundles ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_creative_execution_bundles ON creative_execution_bundles
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

GRANT SELECT, INSERT ON creative_execution_bundles TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS creative_execution_bundles CASCADE;
