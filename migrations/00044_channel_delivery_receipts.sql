-- +goose Up

CREATE TABLE channel_bindings (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  channel text NOT NULL,
  adapter_id text NOT NULL,
  account_ref text NOT NULL,
  authorization_secret_ref text NOT NULL,
  region text NOT NULL DEFAULT 'global',
  status text NOT NULL CHECK (status IN ('active','disabled')),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,project_id,channel,account_ref),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE channel_publications (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  task_delivery_id text NOT NULL,
  delivery_package_id uuid NOT NULL REFERENCES delivery_packages(id),
  channel_binding_id text NOT NULL,
  channel text NOT NULL,
  account_ref text NOT NULL,
  state text NOT NULL CHECK (state IN ('prepared','manual_action_required','submitted','published','failed','unknown','withdrawn')),
  idempotency_key text NOT NULL,
  delivery_digest text NOT NULL CHECK (delivery_digest ~ '^sha256:[0-9a-f]{64}$'),
  request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
  response_digest text NOT NULL DEFAULT '' CHECK (response_digest = '' OR response_digest ~ '^sha256:[0-9a-f]{64}$'),
  external_id text NOT NULL DEFAULT '',
  external_url text NOT NULL DEFAULT '',
  checklist jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(checklist) = 'array'),
  preview jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(preview) = 'object'),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  safe_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_summary) = 'object'),
  cost_minor bigint NOT NULL DEFAULT 0 CHECK (cost_minor >= 0),
  currency text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  scheduled_at timestamptz,
  submitted_at timestamptz,
  published_at timestamptz,
  observed_at timestamptz NOT NULL,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_delivery_id) REFERENCES task_deliveries(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,channel_binding_id) REFERENCES channel_bindings(tenant_id,id)
);

CREATE INDEX channel_bindings_project_idx ON channel_bindings(tenant_id,project_id,channel);
CREATE INDEX channel_publications_task_idx ON channel_publications(tenant_id,task_id,created_at DESC);
CREATE INDEX channel_publications_reconcile_idx ON channel_publications(tenant_id,state,observed_at) WHERE state IN ('submitted','unknown');

ALTER TABLE channel_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON channel_bindings USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE channel_publications ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_publications FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON channel_publications USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON channel_bindings,channel_publications TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS channel_publications;
DROP TABLE IF EXISTS channel_bindings;
