-- +goose Up
CREATE TABLE user_device_flows (
  id uuid PRIMARY KEY,
  device_code_hash text NOT NULL UNIQUE,
  user_code text NOT NULL UNIQUE,
  user_id uuid REFERENCES users(id),
  tenant_id uuid REFERENCES tenants(id),
  state text NOT NULL CHECK (state IN ('pending','approved','consumed','expired')),
  expires_at timestamptz NOT NULL,
  approved_at timestamptz,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE cli_tokens (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE source_revisions ADD COLUMN file_name text NOT NULL DEFAULT 'source';
ALTER TABLE source_revisions ADD COLUMN error_code text NOT NULL DEFAULT '';

CREATE TABLE benchmark_contents (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  title text NOT NULL,
  platform text NOT NULL,
  original_url text NOT NULL DEFAULT '',
  rights_mode text NOT NULL CHECK (rights_mode IN ('analysis_only','generation_reference','owned')),
  validation_level text NOT NULL CHECK (validation_level IN ('observed','sales_indicated','internally_verified')),
  validation_note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_frameworks (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  benchmark_id uuid NOT NULL REFERENCES benchmark_contents(id),
  name text NOT NULL,
  visual_sequence jsonb NOT NULL,
  copy_sequence jsonb NOT NULL,
  status text NOT NULL CHECK (status IN ('draft','approved','review_required','retired')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shot_patterns (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  framework_id uuid NOT NULL REFERENCES content_frameworks(id),
  role text NOT NULL,
  purpose text NOT NULL,
  subject text NOT NULL DEFAULT '',
  action text NOT NULL,
  proof_type text NOT NULL DEFAULT '',
  failure_modes jsonb NOT NULL DEFAULT '[]',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE selling_points (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  title text NOT NULL,
  description text NOT NULL DEFAULT '',
  priority integer NOT NULL,
  knowledge_ids jsonb NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE visualization_plans (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  selling_point_id uuid NOT NULL REFERENCES selling_points(id),
  payload jsonb NOT NULL,
  status text NOT NULL CHECK (status IN ('needs_review','approved','rejected','review_required')),
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE task_runs ADD COLUMN run_token_hash text NOT NULL DEFAULT '';
ALTER TABLE task_runs ADD COLUMN cancel_requested_at timestamptz;
ALTER TABLE task_runs ADD COLUMN report_hash text NOT NULL DEFAULT '';
ALTER TABLE task_runs ADD COLUMN heartbeat_sequence integer NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX script_versions_run_unique ON script_versions(run_id);

CREATE TABLE review_comments (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  shot_id text NOT NULL DEFAULT '',
  json_pointer text NOT NULL DEFAULT '',
  body text NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('internal','client')),
  author_id text NOT NULL,
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE review_grants (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  subject_hash text NOT NULL,
  reviewer_email text NOT NULL,
  token_hash text NOT NULL UNIQUE,
  otp_hash text NOT NULL,
  expires_at timestamptz NOT NULL,
  verified_at timestamptz,
  revoked_at timestamptz,
  decision_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE artifacts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  script_version_id uuid NOT NULL REFERENCES script_versions(id),
  kind text NOT NULL,
  media_type text NOT NULL,
  file_name text NOT NULL,
  sha256 text NOT NULL,
  byte_size bigint NOT NULL,
  object_key text NOT NULL,
  presentation_tier text NOT NULL CHECK (presentation_tier IN ('cloud_native','safe_rendition','local_open','metadata_only')),
  metadata jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE performance_observations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  script_version_id uuid NOT NULL REFERENCES script_versions(id),
  platform text NOT NULL,
  account_alias text NOT NULL,
  published_at timestamptz NOT NULL,
  window_hours integer NOT NULL CHECK (window_hours > 0),
  sample_status text NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}',
  issue_category text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE approval_decisions DROP CONSTRAINT IF EXISTS approval_decisions_actor_id_fkey;
ALTER TABLE approval_decisions ALTER COLUMN actor_id TYPE text USING actor_id::text;

ALTER TABLE cli_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE benchmark_contents ENABLE ROW LEVEL SECURITY;
ALTER TABLE content_frameworks ENABLE ROW LEVEL SECURITY;
ALTER TABLE shot_patterns ENABLE ROW LEVEL SECURITY;
ALTER TABLE selling_points ENABLE ROW LEVEL SECURITY;
ALTER TABLE visualization_plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE performance_observations ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_cli_tokens ON cli_tokens USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_benchmarks ON benchmark_contents USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_frameworks ON content_frameworks USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_patterns ON shot_patterns USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_selling_points ON selling_points USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_visualization_plans ON visualization_plans USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_review_comments ON review_comments USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_review_grants ON review_grants USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_artifacts ON artifacts USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_observations ON performance_observations USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

CREATE OR REPLACE FUNCTION contentcloud_lookup_device_token(p_hash text)
RETURNS TABLE(tenant_id uuid, device_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT d.tenant_id, d.id FROM devices d WHERE d.token_hash = p_hash AND d.revoked_at IS NULL LIMIT 1
$$;
CREATE OR REPLACE FUNCTION contentcloud_lookup_connect_key(p_hash text)
RETURNS TABLE(tenant_id uuid, session_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id, c.id FROM connect_sessions c WHERE c.connect_key_hash = p_hash LIMIT 1
$$;
CREATE OR REPLACE FUNCTION contentcloud_lookup_cli_token(p_hash text)
RETURNS TABLE(tenant_id uuid, token_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id, c.id FROM cli_tokens c WHERE c.token_hash = p_hash AND c.revoked_at IS NULL AND c.expires_at > now() LIMIT 1
$$;
CREATE OR REPLACE FUNCTION contentcloud_lookup_review_token(p_hash text)
RETURNS TABLE(tenant_id uuid, grant_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT g.tenant_id, g.id FROM review_grants g WHERE g.token_hash = p_hash LIMIT 1
$$;
CREATE OR REPLACE FUNCTION contentcloud_pending_source_revisions(p_limit integer)
RETURNS TABLE(tenant_id uuid, revision_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT r.tenant_id, r.id FROM source_revisions r WHERE r.processing_status='pending' ORDER BY r.created_at LIMIT greatest(1,least(p_limit,100))
$$;

-- +goose Down
DROP FUNCTION IF EXISTS contentcloud_pending_source_revisions(integer), contentcloud_lookup_review_token(text), contentcloud_lookup_cli_token(text), contentcloud_lookup_connect_key(text), contentcloud_lookup_device_token(text);
DROP TABLE IF EXISTS performance_observations, artifacts, review_grants, review_comments, visualization_plans, selling_points, shot_patterns, content_frameworks, benchmark_contents, cli_tokens, user_device_flows CASCADE;
DROP INDEX IF EXISTS script_versions_run_unique;
ALTER TABLE task_runs DROP COLUMN IF EXISTS heartbeat_sequence, DROP COLUMN IF EXISTS report_hash, DROP COLUMN IF EXISTS cancel_requested_at, DROP COLUMN IF EXISTS run_token_hash;
ALTER TABLE source_revisions DROP COLUMN IF EXISTS error_code, DROP COLUMN IF EXISTS file_name;
