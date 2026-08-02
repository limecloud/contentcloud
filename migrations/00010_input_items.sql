-- +goose Up

CREATE TABLE input_items (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid,
  source_type text NOT NULL,
  title text NOT NULL,
  summary text NOT NULL DEFAULT '',
  body text NOT NULL DEFAULT '',
  source_ref text NOT NULL DEFAULT '',
  source_digest text NOT NULL DEFAULT '',
  disclosure text NOT NULL CHECK (disclosure IN ('project','tenant','restricted')),
  status text NOT NULL CHECK (status IN ('untriaged','needs_info','routed','task_created','task_merged','project_material','archived')),
  target_task_id text NOT NULL DEFAULT '',
  assignee_user_id text NOT NULL DEFAULT '',
  missing_fields jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(missing_fields) = 'array'),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  idempotency_key text NOT NULL DEFAULT '' CHECK (char_length(idempotency_key) <= 128),
  row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX input_items_idempotency_idx
  ON input_items(tenant_id,idempotency_key)
  WHERE idempotency_key <> '';
CREATE INDEX input_items_queue_idx ON input_items(tenant_id,status,updated_at DESC);
CREATE INDEX input_items_project_idx ON input_items(tenant_id,project_id,updated_at DESC);

ALTER TABLE input_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE input_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON input_items
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON input_items TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS input_items;
