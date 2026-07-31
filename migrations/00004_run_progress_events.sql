-- +goose Up

CREATE TABLE run_progress_events (
  cursor bigserial PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  run_id uuid NOT NULL REFERENCES task_runs(id) ON DELETE CASCADE,
  attempt_id uuid NOT NULL REFERENCES run_attempts(id) ON DELETE CASCADE,
  device_id uuid NOT NULL REFERENCES devices(id),
  sequence integer NOT NULL CHECK (sequence > 0),
  phase text NOT NULL,
  step integer NOT NULL CHECK (step >= 0),
  label text NOT NULL,
  occurred_at timestamptz NOT NULL,
  UNIQUE (attempt_id,sequence)
);

CREATE INDEX run_progress_events_run_cursor_idx ON run_progress_events(tenant_id,run_id,cursor);

ALTER TABLE run_progress_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_progress_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON run_progress_events
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS run_progress_events;
