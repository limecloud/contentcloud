-- +goose Up

ALTER TABLE knowledge_items ADD COLUMN subject text NOT NULL DEFAULT '';
ALTER TABLE knowledge_items ADD COLUMN predicate text NOT NULL DEFAULT '';
ALTER TABLE knowledge_items ADD COLUMN typed_value jsonb NOT NULL DEFAULT '{"type":"text"}';
ALTER TABLE knowledge_items ADD COLUMN scope jsonb NOT NULL DEFAULT '{}';
ALTER TABLE knowledge_items ADD COLUMN forbidden_extensions jsonb NOT NULL DEFAULT '[]';
ALTER TABLE knowledge_items ADD COLUMN depends_on_fact_ids jsonb NOT NULL DEFAULT '[]';
ALTER TABLE knowledge_items ADD COLUMN valid_from timestamptz;
ALTER TABLE knowledge_items ADD COLUMN valid_until timestamptz;
ALTER TABLE knowledge_items ADD COLUMN expires_at timestamptz;
ALTER TABLE knowledge_items ADD COLUMN approved_by uuid REFERENCES users(id);
ALTER TABLE knowledge_items ADD COLUMN approved_at timestamptz;
UPDATE knowledge_items SET subject=title,predicate=kind,typed_value=jsonb_build_object('type','text','text',statement);
CREATE INDEX knowledge_semantic_key_idx ON knowledge_items(tenant_id,project_id,subject,predicate);

CREATE TABLE knowledge_conflicts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject text NOT NULL,
  predicate text NOT NULL,
  knowledge_item_ids jsonb NOT NULL,
  reason text NOT NULL,
  status text NOT NULL CHECK (status IN ('open','resolved','dismissed')),
  resolved_by uuid REFERENCES users(id),
  resolved_at timestamptz,
  resolution text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE decision_requests (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  conflict_id uuid NOT NULL REFERENCES knowledge_conflicts(id),
  question text NOT NULL,
  knowledge_item_ids jsonb NOT NULL,
  status text NOT NULL CHECK (status IN ('open','resolved','canceled')),
  requested_by uuid NOT NULL REFERENCES users(id),
  resolved_by uuid REFERENCES users(id),
  resolved_at timestamptz,
  selected_knowledge_id uuid REFERENCES knowledge_items(id),
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE knowledge_conflicts ENABLE ROW LEVEL SECURITY;
ALTER TABLE decision_requests ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_knowledge_conflicts ON knowledge_conflicts USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_decision_requests ON decision_requests USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS decision_requests, knowledge_conflicts CASCADE;
DROP INDEX IF EXISTS knowledge_semantic_key_idx;
ALTER TABLE knowledge_items DROP COLUMN IF EXISTS approved_at, DROP COLUMN IF EXISTS approved_by, DROP COLUMN IF EXISTS expires_at, DROP COLUMN IF EXISTS valid_until, DROP COLUMN IF EXISTS valid_from, DROP COLUMN IF EXISTS depends_on_fact_ids, DROP COLUMN IF EXISTS forbidden_extensions, DROP COLUMN IF EXISTS scope, DROP COLUMN IF EXISTS typed_value, DROP COLUMN IF EXISTS predicate, DROP COLUMN IF EXISTS subject;
