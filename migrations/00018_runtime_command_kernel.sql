-- +goose Up

CREATE TABLE runtime_outbox (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  event_id text NOT NULL,
  schema_version text NOT NULL CHECK (schema_version = 'contentcloud.runtime-event/1.0'),
  topic text NOT NULL,
  aggregate_id text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL,
  locked_until timestamptz,
  delivered_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,event_id),
  FOREIGN KEY (tenant_id,event_id) REFERENCES runtime_job_events(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_outbox_ready_idx ON runtime_outbox(tenant_id,next_attempt_at,id) WHERE delivered_at IS NULL;

ALTER TABLE runtime_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_outbox
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON runtime_outbox TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS runtime_outbox;
