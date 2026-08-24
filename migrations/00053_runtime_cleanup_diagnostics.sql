-- +goose Up

CREATE TABLE runtime_cleanup_diagnostics (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id text NOT NULL,
  task_id text NOT NULL,
  request_id text NOT NULL,
  manifest_digest text NOT NULL CHECK (manifest_digest ~ '^sha256:[0-9a-f]{64}$'),
  object_key text NOT NULL,
  cause_code text NOT NULL,
  cause_summary text NOT NULL,
  cleanup_error text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('pending','retrying','cleaned','not_found','failed')),
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_retry_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,object_key)
);

CREATE INDEX runtime_cleanup_diagnostics_due_idx
  ON runtime_cleanup_diagnostics(tenant_id,status,next_retry_at,updated_at);

ALTER TABLE runtime_cleanup_diagnostics ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_cleanup_diagnostics FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_cleanup_diagnostics
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT ON runtime_cleanup_diagnostics TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS runtime_cleanup_diagnostics;
