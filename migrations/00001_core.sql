-- +goose Up
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  display_name text NOT NULL,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  slug text NOT NULL UNIQUE,
  name text NOT NULL,
  status text NOT NULL CHECK (status IN ('active','suspended','closed')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('tenant_admin','project_manager','strategist','editor','reviewer','viewer')),
  PRIMARY KEY (tenant_id,user_id)
);

CREATE TABLE sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE brand_projects (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  slug text NOT NULL,
  brand_name text NOT NULL,
  product_name text NOT NULL,
  channel text NOT NULL,
  stage_objective text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('draft','active','blocked','archived')),
  owner_name text NOT NULL DEFAULT '',
  reviewer_name text NOT NULL DEFAULT '',
  client_approver text NOT NULL DEFAULT '',
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,slug)
);

CREATE TABLE connect_sessions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  inviter_user_id uuid NOT NULL REFERENCES users(id),
  connect_key_hash text NOT NULL UNIQUE,
  state text NOT NULL CHECK (state IN ('waiting_for_computer','verifying','connected','expired','canceled','failed')),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  consumed_device_id uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE devices (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  owner_user_id uuid NOT NULL REFERENCES users(id),
  display_name text NOT NULL,
  hostname text NOT NULL,
  platform text NOT NULL,
  arch text NOT NULL,
  daemon_version text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  capability_manifests jsonb NOT NULL DEFAULT '[]',
  last_seen_at timestamptz NOT NULL,
  revoked_at timestamptz
);
ALTER TABLE connect_sessions ADD CONSTRAINT connect_session_device_fk FOREIGN KEY (consumed_device_id) REFERENCES devices(id);

CREATE TABLE project_device_grants (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  device_id uuid NOT NULL REFERENCES devices(id),
  granted_by uuid NOT NULL REFERENCES users(id),
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (tenant_id,project_id,device_id)
);

CREATE TABLE sources (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  name text NOT NULL,
  source_type text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE source_revisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  source_id uuid NOT NULL REFERENCES sources(id),
  object_key text NOT NULL,
  sha256 text NOT NULL,
  byte_size bigint NOT NULL,
  declared_mime text NOT NULL,
  detected_mime text,
  processing_status text NOT NULL,
  parser_version text,
  supersedes_id uuid REFERENCES source_revisions(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,project_id,sha256)
);
CREATE TABLE evidence_spans (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  revision_id uuid NOT NULL REFERENCES source_revisions(id),
  locator_kind text NOT NULL,
  locator jsonb NOT NULL,
  quote_text text NOT NULL,
  quote_hash text NOT NULL,
  ocr_confidence numeric,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE knowledge_items (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  kind text NOT NULL,
  title text NOT NULL,
  statement text NOT NULL,
  status text NOT NULL CHECK (status IN ('candidate','needs_review','approved','rejected','conflicted','review_required','expired')),
  risk_level text NOT NULL,
  allowed_channels jsonb NOT NULL DEFAULT '[]',
  evidence jsonb NOT NULL DEFAULT '[]',
  decision_required boolean NOT NULL DEFAULT true,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE brief_versions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  version integer NOT NULL,
  status text NOT NULL,
  payload jsonb NOT NULL,
  content_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id,version)
);

CREATE TABLE context_snapshots (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  brief_version_id uuid NOT NULL REFERENCES brief_versions(id),
  builder_version text NOT NULL,
  schema_version text NOT NULL,
  payload jsonb NOT NULL,
  manifest_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE task_runs (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  brief_version_id uuid NOT NULL REFERENCES brief_versions(id),
  input_snapshot_id uuid NOT NULL REFERENCES context_snapshots(id),
  idempotency_key text NOT NULL,
  task_type text NOT NULL,
  state text NOT NULL CHECK (state IN ('queued','leased','running','succeeded','failed','canceled')),
  priority integer NOT NULL DEFAULT 0,
  attempt_count integer NOT NULL DEFAULT 0,
  lease_device_id uuid REFERENCES devices(id),
  lease_expires_at timestamptz,
  progress_label text,
  error_code text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX task_runs_claim_idx ON task_runs (priority DESC,created_at) WHERE state='queued';

CREATE TABLE script_versions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  run_id uuid NOT NULL REFERENCES task_runs(id),
  version integer NOT NULL,
  status text NOT NULL,
  input_snapshot_id uuid NOT NULL REFERENCES context_snapshots(id),
  content_hash text NOT NULL,
  canonical_json jsonb NOT NULL,
  validation_report jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id,version)
);

CREATE TABLE approval_decisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  subject_hash text NOT NULL,
  actor_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL,
  reason text NOT NULL DEFAULT '',
  previous_state text NOT NULL,
  resulting_state text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid,
  actor_type text NOT NULL,
  actor_id text NOT NULL,
  action text NOT NULL,
  subject_type text NOT NULL,
  subject_id text NOT NULL,
  summary jsonb NOT NULL DEFAULT '{}',
  request_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX projects_tenant_idx ON brand_projects (tenant_id,updated_at DESC);
CREATE INDEX knowledge_project_idx ON knowledge_items (tenant_id,project_id,status,created_at);
CREATE INDEX scripts_project_idx ON script_versions (tenant_id,project_id,created_at DESC);
CREATE INDEX audit_project_idx ON audit_events (tenant_id,project_id,created_at DESC);

ALTER TABLE brand_projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE connect_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_device_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE source_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_spans ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE brief_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE context_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE script_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE approval_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_projects ON brand_projects USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_connect_sessions ON connect_sessions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_devices ON devices USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_device_grants ON project_device_grants USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_sources ON sources USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_source_revisions ON source_revisions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_evidence ON evidence_spans USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_knowledge ON knowledge_items USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_briefs ON brief_versions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_snapshots ON context_snapshots USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_runs ON task_runs USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_scripts ON script_versions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_approvals ON approval_decisions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_audit ON audit_events USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS audit_events, approval_decisions, script_versions, task_runs, context_snapshots, brief_versions, knowledge_items, evidence_spans, source_revisions, sources, project_device_grants, connect_sessions, devices, brand_projects, sessions, memberships, tenants, users CASCADE;
