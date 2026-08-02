-- +goose Up

ALTER TABLE brand_projects ADD CONSTRAINT brand_projects_tenant_id_id_key UNIQUE (tenant_id,id);

CREATE TABLE environments (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  name text NOT NULL,
  slug text NOT NULL,
  status text NOT NULL CHECK (status IN ('active','paused')),
  manifest_digest text NOT NULL DEFAULT '' CHECK (manifest_digest = '' OR manifest_digest ~ '^sha256:[0-9a-f]{64}$'),
  default_sop_id text NOT NULL DEFAULT '',
  default_sop_version integer NOT NULL DEFAULT 0 CHECK (default_sop_version >= 0),
  capabilities jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(capabilities) = 'array'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,slug)
);

CREATE TABLE sop_definitions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  content_types jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(content_types) = 'array'),
  current_version integer NOT NULL DEFAULT 0 CHECK (current_version >= 0),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id)
);

CREATE TABLE sop_versions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  sop_id text NOT NULL,
  version integer NOT NULL CHECK (version > 0),
  schema_version text NOT NULL,
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  content_types jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(content_types) = 'array'),
  stages jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(stages) = 'array'),
  gates jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(gates) = 'array'),
  default_execution_mode text NOT NULL,
  digest text NOT NULL DEFAULT '' CHECK (digest = '' OR digest ~ '^sha256:[0-9a-f]{64}$'),
  status text NOT NULL CHECK (status IN ('draft','published','retired')),
  created_by text NOT NULL,
  published_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,sop_id,version),
  FOREIGN KEY (tenant_id,sop_id) REFERENCES sop_definitions(tenant_id,id)
);

CREATE TABLE project_sop_bindings (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL,
  environment_id text NOT NULL,
  sop_id text NOT NULL,
  sop_version integer NOT NULL CHECK (sop_version > 0),
  sop_digest text NOT NULL CHECK (sop_digest ~ '^sha256:[0-9a-f]{64}$'),
  bound_by text NOT NULL,
  bound_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,project_id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,environment_id) REFERENCES environments(tenant_id,id),
  FOREIGN KEY (tenant_id,sop_id,sop_version) REFERENCES sop_versions(tenant_id,sop_id,version)
);

CREATE TABLE work_tasks (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  environment_id text NOT NULL,
  sop_id text NOT NULL,
  sop_version integer NOT NULL CHECK (sop_version > 0),
  sop_digest text NOT NULL CHECK (sop_digest ~ '^sha256:[0-9a-f]{64}$'),
  title text NOT NULL,
  intent text NOT NULL DEFAULT '',
  content_type text NOT NULL DEFAULT '',
  input_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_refs) = 'array'),
  requested_output jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(requested_output) = 'object'),
  assignee_user_id text NOT NULL DEFAULT '',
  priority text NOT NULL,
  due_at timestamptz,
  risk_profile text NOT NULL,
  status text NOT NULL CHECK (status IN ('needs_input','ready','running','paused','waiting_gate','blocked','accepted','delivered','cancelled')),
  current_stage_id text NOT NULL DEFAULT '',
  next_action text NOT NULL DEFAULT '',
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,environment_id) REFERENCES environments(tenant_id,id),
  FOREIGN KEY (tenant_id,sop_id,sop_version) REFERENCES sop_versions(tenant_id,sop_id,version)
);

CREATE TABLE stage_runs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  task_id text NOT NULL,
  stage_id text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending','running','waiting_gate','blocked','completed','cancelled')),
  execution_mode text NOT NULL,
  input_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_refs) = 'array'),
  output_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(output_refs) = 'array'),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id)
);

CREATE INDEX environments_status_idx ON environments(tenant_id,status,updated_at DESC);
CREATE INDEX sop_versions_registry_idx ON sop_versions(tenant_id,sop_id,status,version DESC);
CREATE INDEX work_tasks_project_idx ON work_tasks(tenant_id,project_id,status,updated_at DESC);
CREATE INDEX stage_runs_task_idx ON stage_runs(tenant_id,task_id,updated_at);

ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
ALTER TABLE environments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON environments USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE sop_definitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sop_definitions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sop_definitions USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE sop_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sop_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sop_versions USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE project_sop_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_sop_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON project_sop_bindings USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE work_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON work_tasks USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE stage_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE stage_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stage_runs USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON environments,sop_definitions,sop_versions,project_sop_bindings,work_tasks,stage_runs TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS stage_runs;
DROP TABLE IF EXISTS work_tasks;
DROP TABLE IF EXISTS project_sop_bindings;
DROP TABLE IF EXISTS sop_versions;
DROP TABLE IF EXISTS sop_definitions;
DROP TABLE IF EXISTS environments;
ALTER TABLE brand_projects DROP CONSTRAINT IF EXISTS brand_projects_tenant_id_id_key;
