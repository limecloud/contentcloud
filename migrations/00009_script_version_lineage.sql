-- +goose Up

CREATE TABLE scripts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  title text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE scripts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scripts_logical ON scripts
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE INDEX scripts_tenant_project_idx ON scripts(tenant_id,project_id,created_at DESC);

ALTER TABLE script_versions
  ADD COLUMN script_id uuid,
  ADD COLUMN supersedes_id uuid REFERENCES script_versions(id),
  ADD COLUMN baseline_script_version_id uuid REFERENCES script_versions(id),
  ADD COLUMN change_type text NOT NULL DEFAULT 'initial' CHECK (change_type IN ('initial','revision','variant')),
  ADD COLUMN invariant_fields jsonb NOT NULL DEFAULT '[]',
  ADD COLUMN changed_fields jsonb NOT NULL DEFAULT '[]',
  ADD COLUMN hypothesis text NOT NULL DEFAULT '',
  ADD COLUMN revision_reason text NOT NULL DEFAULT '';

INSERT INTO scripts(id,tenant_id,project_id,title,created_at)
SELECT id,tenant_id,project_id,COALESCE(NULLIF(canonical_json->>'title',''),'Untitled script'),created_at
FROM script_versions;
UPDATE script_versions SET script_id=id WHERE script_id IS NULL;
ALTER TABLE script_versions ALTER COLUMN script_id SET NOT NULL;
ALTER TABLE script_versions ADD CONSTRAINT script_versions_script_id_fkey FOREIGN KEY(script_id) REFERENCES scripts(id);
ALTER TABLE script_versions DROP CONSTRAINT IF EXISTS script_versions_project_id_version_key;
CREATE UNIQUE INDEX script_versions_script_version_unique ON script_versions(script_id,version);
CREATE INDEX script_versions_lineage_idx ON script_versions(tenant_id,script_id,created_at DESC);

ALTER TABLE task_runs
  ADD COLUMN script_id uuid REFERENCES scripts(id),
  ADD COLUMN baseline_script_version_id uuid REFERENCES script_versions(id),
  ADD COLUMN change_type text NOT NULL DEFAULT '',
  ADD COLUMN invariant_fields jsonb NOT NULL DEFAULT '[]',
  ADD COLUMN expected_changed_fields jsonb NOT NULL DEFAULT '[]',
  ADD COLUMN hypothesis text NOT NULL DEFAULT '',
  ADD COLUMN revision_reason text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE task_runs
  DROP COLUMN IF EXISTS revision_reason,
  DROP COLUMN IF EXISTS hypothesis,
  DROP COLUMN IF EXISTS expected_changed_fields,
  DROP COLUMN IF EXISTS invariant_fields,
  DROP COLUMN IF EXISTS change_type,
  DROP COLUMN IF EXISTS baseline_script_version_id,
  DROP COLUMN IF EXISTS script_id;
DROP INDEX IF EXISTS script_versions_lineage_idx;
DROP INDEX IF EXISTS script_versions_script_version_unique;
ALTER TABLE script_versions DROP CONSTRAINT IF EXISTS script_versions_script_id_fkey;
ALTER TABLE script_versions
  DROP COLUMN IF EXISTS revision_reason,
  DROP COLUMN IF EXISTS hypothesis,
  DROP COLUMN IF EXISTS changed_fields,
  DROP COLUMN IF EXISTS invariant_fields,
  DROP COLUMN IF EXISTS change_type,
  DROP COLUMN IF EXISTS baseline_script_version_id,
  DROP COLUMN IF EXISTS supersedes_id,
  DROP COLUMN IF EXISTS script_id;
ALTER TABLE script_versions ADD CONSTRAINT script_versions_project_id_version_key UNIQUE(project_id,version);
DROP TABLE IF EXISTS scripts;
