-- +goose Up

ALTER TABLE users ADD COLUMN verified_at timestamptz;
UPDATE users SET verified_at = created_at WHERE verified_at IS NULL;

ALTER TABLE sessions ADD COLUMN revoked_at timestamptz;

ALTER TABLE source_revisions ADD COLUMN uploaded_by uuid REFERENCES users(id);
ALTER TABLE source_revisions ADD COLUMN effective_from timestamptz;
ALTER TABLE source_revisions ADD COLUMN effective_to timestamptz;
ALTER TABLE evidence_spans ADD COLUMN review_status text NOT NULL DEFAULT 'accepted'
  CHECK (review_status IN ('needs_review','accepted','rejected'));
ALTER TABLE evidence_spans ADD COLUMN reviewed_by uuid REFERENCES users(id);
ALTER TABLE evidence_spans ADD COLUMN reviewed_at timestamptz;
UPDATE evidence_spans SET review_status = 'needs_review'
WHERE ocr_confidence IS NOT NULL AND ocr_confidence < 0.85;

ALTER TABLE memberships ADD COLUMN status text NOT NULL DEFAULT 'active'
  CHECK (status IN ('active','revoked'));
ALTER TABLE memberships ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE memberships ADD COLUMN revoked_at timestamptz;

CREATE TABLE membership_invites (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  email text NOT NULL,
  role text NOT NULL CHECK (role IN ('tenant_admin','project_manager','strategist','editor','reviewer','viewer')),
  invited_by uuid NOT NULL REFERENCES users(id),
  token_hash text NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('pending','accepted','revoked','expired')),
  expires_at timestamptz NOT NULL,
  accepted_by uuid REFERENCES users(id),
  accepted_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX membership_invites_pending_email_unique
  ON membership_invites(tenant_id, lower(email)) WHERE status = 'pending';

CREATE TABLE project_templates (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  name text NOT NULL,
  channel text NOT NULL,
  stage_objective text NOT NULL DEFAULT '',
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

ALTER TABLE membership_invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_templates ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_membership_invites ON membership_invites
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_project_templates ON project_templates
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

CREATE OR REPLACE FUNCTION contentcloud_lookup_membership_invite(p_hash text)
RETURNS TABLE(tenant_id uuid, invite_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT i.tenant_id, i.id
  FROM membership_invites i
  WHERE i.token_hash = p_hash
  LIMIT 1
$$;

-- +goose Down

DROP FUNCTION IF EXISTS contentcloud_lookup_membership_invite(text);
DROP TABLE IF EXISTS project_templates, membership_invites;
ALTER TABLE memberships DROP COLUMN IF EXISTS revoked_at, DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS status;
ALTER TABLE sessions DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE evidence_spans DROP COLUMN IF EXISTS reviewed_at, DROP COLUMN IF EXISTS reviewed_by, DROP COLUMN IF EXISTS review_status;
ALTER TABLE source_revisions DROP COLUMN IF EXISTS effective_to, DROP COLUMN IF EXISTS effective_from, DROP COLUMN IF EXISTS uploaded_by;
ALTER TABLE users DROP COLUMN IF EXISTS verified_at;
