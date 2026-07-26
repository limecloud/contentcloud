-- +goose Up

ALTER TABLE task_runs ADD COLUMN active_attempt_id uuid;

CREATE TABLE run_attempts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  run_id uuid NOT NULL REFERENCES task_runs(id),
  device_id uuid NOT NULL REFERENCES devices(id),
  state text NOT NULL CHECK (state IN ('leased','running','succeeded','failed','canceled','expired')),
  capability_id text NOT NULL,
  capability_version text NOT NULL,
  capability_digest text NOT NULL,
  input_schema text NOT NULL,
  output_schema text NOT NULL,
  token_hash text NOT NULL,
  lease_expires_at timestamptz NOT NULL,
  heartbeat_at timestamptz,
  started_at timestamptz,
  finished_at timestamptz,
  exit_code integer,
  failure_class text NOT NULL DEFAULT '',
  usage jsonb NOT NULL DEFAULT '{}',
  transcript_summary text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX run_attempt_active_unique ON run_attempts(run_id) WHERE state IN ('leased','running');
CREATE INDEX run_attempt_tenant_run_idx ON run_attempts(tenant_id,run_id,created_at DESC);

ALTER TABLE run_attempts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_run_attempts ON run_attempts USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS run_attempts CASCADE;
ALTER TABLE task_runs DROP COLUMN IF EXISTS active_attempt_id;
