-- +goose Up

CREATE TABLE conversation_imports (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  stage_run_id text,
  node_id text NOT NULL DEFAULT '',
  client_id text NOT NULL,
  adapter_version text NOT NULL,
  adapter_id text NOT NULL,
  purpose text NOT NULL,
  requested_scope text NOT NULL,
  attach_as text NOT NULL,
  retention_days integer NOT NULL CHECK (retention_days BETWEEN 1 AND 90),
  status text NOT NULL CHECK (status IN ('awaiting_client_confirmation','uploaded','cancelled','expired','rejected')),
  idempotency_key text NOT NULL DEFAULT '' CHECK (char_length(idempotency_key) <= 128),
  bundle jsonb CHECK (bundle IS NULL OR jsonb_typeof(bundle) = 'object'),
  expires_at timestamptz NOT NULL,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  cancelled_at timestamptz,
  uploaded_at timestamptz,
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,stage_run_id) REFERENCES stage_runs(tenant_id,id) DEFERRABLE INITIALLY DEFERRED
);

CREATE UNIQUE INDEX conversation_imports_idempotency_idx
  ON conversation_imports(tenant_id,idempotency_key)
  WHERE idempotency_key <> '';
CREATE INDEX conversation_imports_task_idx ON conversation_imports(tenant_id,task_id,created_at DESC);
CREATE INDEX conversation_imports_expiry_idx ON conversation_imports(tenant_id,status,expires_at);

ALTER TABLE conversation_imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE conversation_imports FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON conversation_imports
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON conversation_imports TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS conversation_imports;
