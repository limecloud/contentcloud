-- +goose Up

CREATE TABLE workspace_folders (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  parent_id uuid,
  name text NOT NULL CHECK (char_length(trim(name)) BETWEEN 1 AND 120),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,id),
  FOREIGN KEY (tenant_id,parent_id) REFERENCES workspace_folders(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX workspace_folders_project_idx ON workspace_folders(tenant_id,project_id,parent_id,name);

CREATE TABLE workspace_materials (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  folder_id uuid,
  source_id uuid NOT NULL REFERENCES sources(id),
  source_revision_id uuid NOT NULL REFERENCES source_revisions(id),
  material_kind text NOT NULL CHECK (material_kind IN ('document','image','video','audio','table','other')),
  origin text NOT NULL CHECK (origin IN ('uploaded','imported','linked')),
  usage text NOT NULL CHECK (usage IN ('project_material','project_reference')),
  title text NOT NULL CHECK (char_length(trim(title)) BETWEEN 1 AND 240),
  created_by uuid NOT NULL REFERENCES users(id),
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id,folder_id) REFERENCES workspace_folders(tenant_id,id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX workspace_materials_revision_unique ON workspace_materials(tenant_id,source_revision_id);
CREATE INDEX workspace_materials_project_idx ON workspace_materials(tenant_id,project_id,updated_at DESC);
CREATE INDEX workspace_materials_recent_idx ON workspace_materials(tenant_id,last_used_at DESC NULLS LAST);

ALTER TABLE workspace_folders ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_folders FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_folders
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON workspace_folders TO contentcloud_runtime;

ALTER TABLE workspace_materials ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_materials FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_materials
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT,INSERT,UPDATE,DELETE ON workspace_materials TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS workspace_materials;
DROP TABLE IF EXISTS workspace_folders;
