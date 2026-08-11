-- +goose Up

CREATE TABLE model_generation_receipts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  task_revision_id text NOT NULL,
  provider_id text NOT NULL,
  provider text NOT NULL,
  model text NOT NULL,
  request_id text NOT NULL DEFAULT '',
  request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
  response_digest text NOT NULL CHECK (response_digest ~ '^sha256:[0-9a-f]{64}$'),
  input_tokens bigint NOT NULL CHECK (input_tokens >= 0),
  output_tokens bigint NOT NULL CHECK (output_tokens >= 0),
  total_tokens bigint NOT NULL CHECK (total_tokens >= 0),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,task_revision_id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_revision_id) REFERENCES task_revisions(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX model_generation_receipts_task_idx ON model_generation_receipts(tenant_id,task_id,created_at DESC);

ALTER TABLE model_generation_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_generation_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON model_generation_receipts USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT ON model_generation_receipts TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS model_generation_receipts;
