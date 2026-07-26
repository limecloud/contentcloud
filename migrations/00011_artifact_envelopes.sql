-- +goose Up
ALTER TABLE artifacts
  ADD COLUMN capability_id text NOT NULL DEFAULT 'contentcloud.artifact.export',
  ADD COLUMN capability_version text NOT NULL DEFAULT '1.0.0',
  ADD COLUMN capability_digest text NOT NULL DEFAULT 'legacy-server-export',
  ADD COLUMN schema_id text NOT NULL DEFAULT 'contentcloud.script-export.legacy/1.0',
  ADD COLUMN visibility text NOT NULL DEFAULT 'internal' CHECK (visibility IN ('internal','client','restricted')),
  ADD COLUMN retention_class text NOT NULL DEFAULT 'project' CHECK (retention_class IN ('attempt','project','audit')),
  ADD COLUMN derived_from_artifact_id uuid REFERENCES artifacts(id),
  ADD COLUMN purpose text NOT NULL DEFAULT 'download',
  ADD COLUMN source_device_id uuid REFERENCES devices(id),
  ADD COLUMN validation_status text NOT NULL DEFAULT 'valid' CHECK (validation_status IN ('pending','valid','rejected')),
  ADD COLUMN validation_error text NOT NULL DEFAULT '',
  ADD COLUMN artifact_envelope jsonb;

CREATE INDEX artifacts_derivation_idx ON artifacts(tenant_id,derived_from_artifact_id,created_at DESC);
CREATE INDEX artifacts_source_device_idx ON artifacts(tenant_id,source_device_id,created_at DESC);

CREATE TABLE artifact_open_requests (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  artifact_id uuid NOT NULL REFERENCES artifacts(id),
  device_id uuid NOT NULL REFERENCES devices(id),
  requested_by text NOT NULL,
  state text NOT NULL CHECK (state IN ('pending','accepted','opened','not_available','failed','expired')),
  reason text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX artifact_open_pending_idx ON artifact_open_requests(tenant_id,device_id,state,expires_at,created_at);
ALTER TABLE artifact_open_requests ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_artifact_open_requests ON artifact_open_requests
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS artifact_open_requests;
DROP INDEX IF EXISTS artifacts_source_device_idx;
DROP INDEX IF EXISTS artifacts_derivation_idx;
ALTER TABLE artifacts
  DROP COLUMN IF EXISTS artifact_envelope,
  DROP COLUMN IF EXISTS validation_error,
  DROP COLUMN IF EXISTS validation_status,
  DROP COLUMN IF EXISTS source_device_id,
  DROP COLUMN IF EXISTS purpose,
  DROP COLUMN IF EXISTS derived_from_artifact_id,
  DROP COLUMN IF EXISTS retention_class,
  DROP COLUMN IF EXISTS visibility,
  DROP COLUMN IF EXISTS schema_id,
  DROP COLUMN IF EXISTS capability_digest,
  DROP COLUMN IF EXISTS capability_version,
  DROP COLUMN IF EXISTS capability_id;
