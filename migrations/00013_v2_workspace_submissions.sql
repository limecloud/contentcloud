-- +goose Up
CREATE TABLE workspace_bindings (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  device_id uuid REFERENCES devices(id),
  owner_user_id uuid NOT NULL REFERENCES users(id),
  template_id text NOT NULL,
  template_version text NOT NULL DEFAULT '',
  targets jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(targets) = 'array'),
  credential_hash text NOT NULL UNIQUE CHECK (credential_hash ~ '^[0-9a-f]{64}$'),
  status text NOT NULL CHECK (status IN ('active','revoked')),
  initialized_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  revoked_at timestamptz
);

CREATE INDEX workspace_bindings_project_idx ON workspace_bindings(tenant_id,project_id,initialized_at DESC);

CREATE TABLE submissions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  workspace_id uuid NOT NULL REFERENCES workspace_bindings(id),
  submission_type text NOT NULL CHECK (submission_type IN ('knowledge','research','strategy','brief','script','delivery','performance')),
  status text NOT NULL CHECK (status IN ('preparing','submitted','in_review','changes_requested','approved','rejected','withdrawn','superseded')),
  current_revision_id uuid,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (tenant_id,project_id,workspace_id,submission_type)
);

CREATE TABLE submission_revisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  workspace_id uuid NOT NULL REFERENCES workspace_bindings(id),
  submission_id uuid NOT NULL REFERENCES submissions(id),
  revision_no integer NOT NULL CHECK (revision_no > 0),
  schema_version text NOT NULL,
  content_hash text NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  base_approved_snapshot_id uuid,
  local_run_summary jsonb NOT NULL,
  objects jsonb NOT NULL CHECK (jsonb_typeof(objects) = 'array'),
  artifacts jsonb NOT NULL CHECK (jsonb_typeof(artifacts) = 'array'),
  message text NOT NULL DEFAULT '',
  idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 128),
  evidence_limited boolean NOT NULL DEFAULT false,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id,submission_id,revision_no),
  UNIQUE (tenant_id,workspace_id,idempotency_key)
);

ALTER TABLE submissions
  ADD CONSTRAINT submissions_current_revision_fk FOREIGN KEY (current_revision_id) REFERENCES submission_revisions(id);

CREATE TABLE source_disclosures (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  submission_revision_id uuid NOT NULL REFERENCES submission_revisions(id),
  source_ref text NOT NULL,
  disclosure_level text NOT NULL CHECK (disclosure_level IN ('metadata_only','evidence_pack','full_source')),
  sha256 text NOT NULL CHECK (sha256 ~ '^(sha256:)?[0-9a-f]{64}$'),
  byte_size bigint NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
  evidence_pack jsonb,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id,submission_revision_id,source_ref)
);

CREATE TABLE approved_snapshots (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  workspace_id uuid NOT NULL REFERENCES workspace_bindings(id),
  submission_id uuid NOT NULL REFERENCES submissions(id),
  submission_revision_id uuid NOT NULL UNIQUE REFERENCES submission_revisions(id),
  submission_type text NOT NULL CHECK (submission_type IN ('knowledge','research','strategy','brief','script','delivery','performance')),
  schema_version text NOT NULL,
  content_hash text NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  subject_hash text NOT NULL CHECK (subject_hash ~ '^[0-9a-f]{64}$'),
  canonical_content jsonb NOT NULL,
  eligible_ids jsonb NOT NULL CHECK (jsonb_typeof(eligible_ids) = 'array'),
  artifacts jsonb NOT NULL CHECK (jsonb_typeof(artifacts) = 'array'),
  decision_id uuid NOT NULL REFERENCES approval_decisions(id),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL
);

ALTER TABLE submission_revisions
  ADD CONSTRAINT submission_revisions_base_snapshot_fk FOREIGN KEY (base_approved_snapshot_id) REFERENCES approved_snapshots(id);

CREATE INDEX submission_revisions_project_idx ON submission_revisions(tenant_id,project_id,created_at DESC);
CREATE INDEX approved_snapshots_project_idx ON approved_snapshots(tenant_id,project_id,submission_type,created_at DESC);

ALTER TABLE workspace_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE submissions ENABLE ROW LEVEL SECURITY;
ALTER TABLE submission_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_disclosures ENABLE ROW LEVEL SECURITY;
ALTER TABLE approved_snapshots ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_workspace_bindings ON workspace_bindings USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_submissions ON submissions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_submission_revisions ON submission_revisions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_source_disclosures ON source_disclosures USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_approved_snapshots ON approved_snapshots USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

CREATE OR REPLACE FUNCTION contentcloud_lookup_workspace_token(p_hash text)
RETURNS TABLE(tenant_id uuid, workspace_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT w.tenant_id, w.id FROM workspace_bindings w
  WHERE w.credential_hash = p_hash AND w.status = 'active' AND w.revoked_at IS NULL LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_reject_v2_immutable_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'V2 revision and snapshot records are immutable' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER submission_revisions_immutable BEFORE UPDATE OR DELETE ON submission_revisions FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();
CREATE TRIGGER source_disclosures_immutable BEFORE UPDATE OR DELETE ON source_disclosures FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();
CREATE TRIGGER approved_snapshots_immutable BEFORE UPDATE OR DELETE ON approved_snapshots FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS approved_snapshots_immutable ON approved_snapshots;
DROP TRIGGER IF EXISTS source_disclosures_immutable ON source_disclosures;
DROP TRIGGER IF EXISTS submission_revisions_immutable ON submission_revisions;
DROP FUNCTION IF EXISTS contentcloud_reject_v2_immutable_mutation();
DROP FUNCTION IF EXISTS contentcloud_lookup_workspace_token(text);
ALTER TABLE submission_revisions DROP CONSTRAINT IF EXISTS submission_revisions_base_snapshot_fk;
DROP TABLE IF EXISTS approved_snapshots;
DROP TABLE IF EXISTS source_disclosures;
ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_current_revision_fk;
DROP TABLE IF EXISTS submission_revisions;
DROP TABLE IF EXISTS submissions;
DROP INDEX IF EXISTS workspace_bindings_project_idx;
DROP TABLE IF EXISTS workspace_bindings;
