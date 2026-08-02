-- +goose Up

ALTER TABLE task_runs
  ADD COLUMN work_task_id text NOT NULL DEFAULT '',
  ADD COLUMN sop_id text NOT NULL DEFAULT '',
  ADD COLUMN sop_version integer NOT NULL DEFAULT 0,
  ADD COLUMN sop_digest text NOT NULL DEFAULT '',
  ADD COLUMN stage_id text NOT NULL DEFAULT '',
  ADD COLUMN execution_mode text NOT NULL DEFAULT '',
  ADD COLUMN executor_kind text NOT NULL DEFAULT '',
  ADD COLUMN output_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN task_revision_id text NOT NULL DEFAULT '',
  ADD COLUMN gate_evaluation_id text NOT NULL DEFAULT '';

CREATE INDEX task_runs_work_task_idx ON task_runs(tenant_id,work_task_id,created_at DESC) WHERE work_task_id <> '';

CREATE TABLE task_gate_evaluations (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  stage_run_id text NOT NULL,
  gate_id text NOT NULL,
  gate_mode text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending','approved','rejected','changes_requested','expired')),
  revision_id text NOT NULL DEFAULT '',
  input_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_refs) = 'array'),
  checks jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(checks) = 'object'),
  decision text NOT NULL DEFAULT '',
  reason text NOT NULL DEFAULT '',
  decided_by text NOT NULL DEFAULT '',
  decided_at timestamptz,
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,stage_run_id) REFERENCES stage_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE task_revisions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  revision_no integer NOT NULL CHECK (revision_no > 0),
  content_type text NOT NULL,
  schema_version text NOT NULL,
  content jsonb NOT NULL,
  content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
  sop_digest text NOT NULL CHECK (sop_digest ~ '^sha256:[0-9a-f]{64}$'),
  knowledge_snapshot_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(knowledge_snapshot_ids) = 'array'),
  evidence_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(evidence_summary) = 'object'),
  rights_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rights_summary) = 'object'),
  status text NOT NULL CHECK (status IN ('draft','submitted','accepted','rejected','superseded')),
  submitted_by text NOT NULL,
  submitted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,task_id,revision_no),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE task_deliveries (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  revision_id text NOT NULL,
  destination text NOT NULL,
  status text NOT NULL CHECK (status IN ('ready','delivered','failed','cancelled')),
  manifest jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(manifest) = 'array'),
  delivery_digest text NOT NULL CHECK (delivery_digest ~ '^sha256:[0-9a-f]{64}$'),
  delivered_by text NOT NULL DEFAULT '',
  delivered_at timestamptz,
  error_code text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,revision_id) REFERENCES task_revisions(tenant_id,id)
);

CREATE INDEX task_gate_evaluations_task_idx ON task_gate_evaluations(tenant_id,task_id,created_at);
CREATE INDEX task_revisions_task_idx ON task_revisions(tenant_id,task_id,revision_no DESC);
CREATE INDEX task_deliveries_task_idx ON task_deliveries(tenant_id,task_id,created_at DESC);

ALTER TABLE task_gate_evaluations ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_gate_evaluations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_gate_evaluations USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE task_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_revisions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_revisions USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE task_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_deliveries USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON task_gate_evaluations,task_revisions,task_deliveries TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS task_deliveries;
DROP TABLE IF EXISTS task_revisions;
DROP TABLE IF EXISTS task_gate_evaluations;
DROP INDEX IF EXISTS task_runs_work_task_idx;
ALTER TABLE task_runs
  DROP COLUMN IF EXISTS gate_evaluation_id,
  DROP COLUMN IF EXISTS task_revision_id,
  DROP COLUMN IF EXISTS output_refs,
  DROP COLUMN IF EXISTS executor_kind,
  DROP COLUMN IF EXISTS execution_mode,
  DROP COLUMN IF EXISTS stage_id,
  DROP COLUMN IF EXISTS sop_digest,
  DROP COLUMN IF EXISTS sop_version,
  DROP COLUMN IF EXISTS sop_id,
  DROP COLUMN IF EXISTS work_task_id;
