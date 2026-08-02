-- +goose Up

CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  display_name text NOT NULL,
  password_hash text NOT NULL,
  verified_at timestamptz,
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
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
  created_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (tenant_id,user_id)
);

CREATE TABLE sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

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

CREATE TABLE project_templates (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  name text NOT NULL,
  channel text NOT NULL,
  stage_objective text NOT NULL DEFAULT '',
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,name)
);

CREATE TABLE connect_sessions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  inviter_user_id uuid NOT NULL REFERENCES users(id),
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
  capability_manifests jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(capability_manifests) = 'array'),
  last_seen_at timestamptz NOT NULL,
  revoked_at timestamptz
);

ALTER TABLE connect_sessions
  ADD CONSTRAINT connect_sessions_consumed_device_fkey FOREIGN KEY (consumed_device_id) REFERENCES devices(id);

CREATE TABLE project_device_grants (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  device_id uuid NOT NULL REFERENCES devices(id),
  granted_by uuid NOT NULL REFERENCES users(id),
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (tenant_id,project_id,device_id)
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

CREATE TABLE bootstrap_attempts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  connect_session_id uuid NOT NULL REFERENCES connect_sessions(id),
  attempt_token_hash text NOT NULL UNIQUE,
  code_challenge text NOT NULL,
  user_code text NOT NULL UNIQUE,
  state text NOT NULL CHECK (state IN ('pending','approved','denied','consumed','completed','failed','expired')),
  support_code text NOT NULL UNIQUE,
  last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
  decided_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  decided_at timestamptz,
  consumed_at timestamptz,
  completed_at timestamptz
);

CREATE TABLE bootstrap_progress_events (
  attempt_id uuid NOT NULL REFERENCES bootstrap_attempts(id),
  sequence bigint NOT NULL CHECK (sequence > 0),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  schema_version text NOT NULL CHECK (schema_version = '1.0'),
  occurred_at timestamptz NOT NULL,
  stage text NOT NULL,
  status text NOT NULL CHECK (status IN ('started','passed','needs_action','failed','skipped')),
  check_id text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  action_id text NOT NULL DEFAULT '',
  facts jsonb NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (attempt_id,sequence)
);

CREATE TABLE bootstrap_diagnostics (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  attempt_id uuid NOT NULL REFERENCES bootstrap_attempts(id),
  support_code text NOT NULL,
  digest text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size BETWEEN 0 AND 262144),
  summary jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (attempt_id,digest)
);

CREATE TABLE workspace_bindings (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  device_id uuid REFERENCES devices(id),
  owner_user_id uuid NOT NULL REFERENCES users(id),
  template_id text NOT NULL,
  template_version text NOT NULL DEFAULT '',
  targets jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(targets) = 'array'),
  credential_hash text NOT NULL UNIQUE CHECK (credential_hash ~ '^[0-9a-f]{64}$'),
  status text NOT NULL CHECK (status IN ('active','revoked')),
  initialized_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  revoked_at timestamptz
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
  file_name text NOT NULL DEFAULT 'source',
  object_key text NOT NULL,
  sha256 text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size >= 0),
  declared_mime text NOT NULL,
  detected_mime text,
  processing_status text NOT NULL,
  parser_version text,
  error_code text NOT NULL DEFAULT '',
  supersedes_id uuid REFERENCES source_revisions(id),
  uploaded_by uuid REFERENCES users(id),
  effective_from timestamptz,
  effective_to timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,project_id,sha256),
  CHECK (effective_to IS NULL OR effective_from IS NULL OR effective_to > effective_from)
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
  review_status text NOT NULL DEFAULT 'accepted' CHECK (review_status IN ('needs_review','accepted','rejected')),
  reviewed_by uuid REFERENCES users(id),
  reviewed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE assets (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  name text NOT NULL,
  asset_type text NOT NULL CHECK (asset_type IN ('product_image','brand_mark','packaging','person','location','audio','other')),
  source_revision_id uuid NOT NULL REFERENCES source_revisions(id),
  usage_mode text NOT NULL CHECK (usage_mode IN ('analysis_only','generation_reference','owned')),
  status text NOT NULL CHECK (status IN ('needs_review','approved','rejected','review_required','expired')),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rights_records (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  asset_id uuid NOT NULL REFERENCES assets(id),
  rights_holder text NOT NULL,
  rights_type text NOT NULL CHECK (rights_type IN ('owned','licensed_generation','licensed_edit','public_domain')),
  territories jsonb NOT NULL CHECK (jsonb_typeof(territories) = 'array'),
  channels jsonb NOT NULL CHECK (jsonb_typeof(channels) = 'array'),
  valid_from timestamptz,
  valid_until timestamptz,
  proof_source_revision_id uuid NOT NULL REFERENCES source_revisions(id),
  restrictions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(restrictions) = 'array'),
  status text NOT NULL CHECK (status IN ('needs_review','approved','rejected','review_required','expired')),
  reviewed_by uuid REFERENCES users(id),
  reviewed_at timestamptz,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from)
);

CREATE TABLE context_snapshots (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
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
  input_snapshot_id uuid NOT NULL REFERENCES context_snapshots(id),
  idempotency_key text NOT NULL,
  task_type text NOT NULL,
  capability_id text NOT NULL,
  capability_version text NOT NULL,
  input_schema text NOT NULL,
  output_schema text NOT NULL,
  output_count integer NOT NULL DEFAULT 1 CHECK (output_count BETWEEN 1 AND 20),
  delivery_profiles jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(delivery_profiles) = 'array'),
  state text NOT NULL CHECK (state IN ('queued','leased','running','succeeded','failed','canceled')),
  priority integer NOT NULL DEFAULT 0,
  attempt_count integer NOT NULL DEFAULT 0,
  active_attempt_id uuid,
  lease_device_id uuid REFERENCES devices(id),
  lease_expires_at timestamptz,
  run_token_hash text NOT NULL DEFAULT '',
  progress_label text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  cancel_requested_at timestamptz,
  report_hash text NOT NULL DEFAULT '',
  heartbeat_sequence integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);

CREATE TABLE run_attempts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  run_id uuid NOT NULL REFERENCES task_runs(id),
  device_id uuid NOT NULL REFERENCES devices(id),
  state text NOT NULL CHECK (state IN ('leased','running','succeeded','failed','canceled','expired')),
  capability_id text NOT NULL,
  capability_version text NOT NULL,
  capability_digest text NOT NULL,
  input_schema text NOT NULL,
  output_schema text NOT NULL,
  token_hash text NOT NULL,
  lease_expires_at timestamptz NOT NULL,
  heartbeat_at timestamptz,
  started_at timestamptz,
  finished_at timestamptz,
  exit_code integer,
  failure_class text NOT NULL DEFAULT '',
  usage jsonb NOT NULL DEFAULT '{}'::jsonb,
  transcript_summary text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE creative_execution_bundles (
  run_id uuid PRIMARY KEY REFERENCES task_runs(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  bundle_id text NOT NULL,
  digest text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id,bundle_id)
);

CREATE TABLE approval_decisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL CHECK (subject_type = 'submission_revision'),
  subject_id uuid NOT NULL,
  subject_hash text NOT NULL,
  decision_stage text NOT NULL DEFAULT 'internal' CHECK (decision_stage IN ('internal','client')),
  actor_id text NOT NULL,
  decision text NOT NULL,
  reason text NOT NULL DEFAULT '',
  previous_state text NOT NULL,
  resulting_state text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE review_cycles (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL CHECK (subject_type = 'submission_revision'),
  subject_id uuid NOT NULL,
  cycle_number integer NOT NULL CHECK (cycle_number > 0),
  status text NOT NULL CHECK (status IN ('open','approved','changes_requested','superseded')),
  conclusion text NOT NULL DEFAULT '',
  assignee_user_id text NOT NULL DEFAULT '',
  opened_by text NOT NULL,
  decided_by text NOT NULL DEFAULT '',
  opened_at timestamptz NOT NULL,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (subject_id,cycle_number)
);

CREATE TABLE review_comments (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  review_cycle_id uuid REFERENCES review_cycles(id),
  subject_type text NOT NULL CHECK (subject_type = 'submission_revision'),
  subject_id uuid NOT NULL,
  carried_from_comment_id uuid REFERENCES review_comments(id),
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
  subject_type text NOT NULL CHECK (subject_type = 'submission_revision'),
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

CREATE TABLE submissions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  workspace_id uuid NOT NULL REFERENCES workspace_bindings(id),
  submission_type text NOT NULL CHECK (submission_type IN ('context','knowledge','brief','content_batch','asset_batch','delivery','result')),
  status text NOT NULL CHECK (status IN ('preparing','submitted','in_review','internally_approved','client_review','changes_requested','approved','rejected','withdrawn','superseded')),
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
  content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
  base_snapshot_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(base_snapshot_ids) = 'array'),
  environment_digest text NOT NULL,
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
  ADD CONSTRAINT submissions_current_revision_fkey FOREIGN KEY (current_revision_id) REFERENCES submission_revisions(id);

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
  submission_type text NOT NULL CHECK (submission_type IN ('context','knowledge','brief','content_batch','asset_batch','delivery','result')),
  schema_version text NOT NULL,
  content_hash text NOT NULL CHECK (content_hash ~ '^sha256:[0-9a-f]{64}$'),
  subject_hash text NOT NULL CHECK (subject_hash ~ '^sha256:[0-9a-f]{64}$'),
  canonical_content jsonb NOT NULL,
  eligible_ids jsonb NOT NULL CHECK (jsonb_typeof(eligible_ids) = 'array'),
  artifacts jsonb NOT NULL CHECK (jsonb_typeof(artifacts) = 'array'),
  decision_id uuid NOT NULL REFERENCES approval_decisions(id),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE artifacts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  approved_snapshot_id uuid NOT NULL REFERENCES approved_snapshots(id),
  kind text NOT NULL CHECK (kind = 'delivery'),
  capability_id text NOT NULL,
  capability_version text NOT NULL,
  capability_digest text NOT NULL,
  schema_id text NOT NULL,
  media_type text NOT NULL,
  file_name text NOT NULL,
  sha256 text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size >= 0),
  object_key text NOT NULL,
  visibility text NOT NULL,
  retention_class text NOT NULL,
  purpose text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE delivery_packages (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  content_item_id text NOT NULL CHECK (char_length(btrim(content_item_id)) > 0),
  status text NOT NULL CHECK (status = 'ready'),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE delivery_package_snapshots (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  delivery_package_id uuid NOT NULL REFERENCES delivery_packages(id),
  approved_snapshot_id uuid NOT NULL REFERENCES approved_snapshots(id),
  position integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (delivery_package_id,approved_snapshot_id)
);

CREATE TABLE delivery_package_artifacts (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  delivery_package_id uuid NOT NULL REFERENCES delivery_packages(id),
  artifact_id uuid NOT NULL REFERENCES artifacts(id),
  position integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (delivery_package_id,artifact_id)
);

CREATE TABLE performance_import_batches (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  source_name text NOT NULL CHECK (char_length(source_name) BETWEEN 1 AND 255),
  source_format text NOT NULL CHECK (source_format IN ('manual','json','csv','xlsx')),
  source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  currency text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
  row_count integer NOT NULL CHECK (row_count BETWEEN 1 AND 1000),
  imported_count integer NOT NULL CHECK (imported_count = row_count),
  status text NOT NULL CHECK (status = 'imported'),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL
);

CREATE TABLE performance_observations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  import_batch_id uuid NOT NULL REFERENCES performance_import_batches(id),
  row_number integer NOT NULL CHECK (row_number > 0),
  approved_snapshot_id uuid NOT NULL REFERENCES approved_snapshots(id),
  platform text NOT NULL,
  account_alias text NOT NULL,
  published_at timestamptz NOT NULL,
  window_hours integer NOT NULL CHECK (window_hours > 0),
  sample_status text NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
  currency text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
  spend double precision NOT NULL DEFAULT 0 CHECK (spend >= 0 AND spend <> 'Infinity'::double precision AND spend <> 'NaN'::double precision),
  gmv double precision NOT NULL DEFAULT 0 CHECK (gmv >= 0 AND gmv <> 'Infinity'::double precision AND gmv <> 'NaN'::double precision),
  roi double precision CHECK (roi IS NULL OR (roi >= 0 AND roi <> 'Infinity'::double precision AND roi <> 'NaN'::double precision)),
  dedup_key text NOT NULL,
  issue_category text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,project_id,dedup_key)
);

CREATE TABLE rating_decisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL CHECK (subject_type = 'approved_snapshot'),
  subject_id uuid NOT NULL REFERENCES approved_snapshots(id),
  observation_ids jsonb NOT NULL CHECK (jsonb_typeof(observation_ids) = 'array' AND jsonb_array_length(observation_ids) BETWEEN 1 AND 100),
  rating text NOT NULL CHECK (rating IN ('seed_candidate','repairable','discarded','insufficient_sample')),
  reason text NOT NULL CHECK (char_length(btrim(reason)) > 0),
  next_action text NOT NULL CHECK (char_length(btrim(next_action)) > 0),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL
);

CREATE TABLE audit_events (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid REFERENCES brand_projects(id),
  actor_type text NOT NULL,
  actor_id text NOT NULL,
  action text NOT NULL,
  subject_type text NOT NULL,
  subject_id text NOT NULL,
  summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX membership_invites_pending_email_unique ON membership_invites(tenant_id,lower(email)) WHERE status = 'pending';
CREATE INDEX projects_tenant_idx ON brand_projects(tenant_id,updated_at DESC);
CREATE INDEX bootstrap_attempts_session_created_idx ON bootstrap_attempts(tenant_id,connect_session_id,created_at DESC);
CREATE INDEX workspace_bindings_project_idx ON workspace_bindings(tenant_id,project_id,initialized_at DESC);
CREATE INDEX assets_project_idx ON assets(tenant_id,project_id,status,created_at DESC);
CREATE INDEX rights_asset_idx ON rights_records(tenant_id,asset_id,status,created_at DESC);
CREATE INDEX task_runs_claim_idx ON task_runs(priority DESC,created_at) WHERE state = 'queued';
CREATE UNIQUE INDEX run_attempt_active_unique ON run_attempts(run_id) WHERE state IN ('leased','running');
CREATE INDEX run_attempt_tenant_run_idx ON run_attempts(tenant_id,run_id,created_at DESC);
CREATE INDEX creative_execution_bundles_project_idx ON creative_execution_bundles(tenant_id,project_id,created_at DESC);
CREATE INDEX review_cycles_subject_idx ON review_cycles(tenant_id,subject_id,cycle_number DESC);
CREATE INDEX review_comments_cycle_idx ON review_comments(tenant_id,review_cycle_id,created_at);
CREATE INDEX submission_revisions_project_idx ON submission_revisions(tenant_id,project_id,created_at DESC);
CREATE INDEX approved_snapshots_project_idx ON approved_snapshots(tenant_id,project_id,submission_type,created_at DESC);
CREATE INDEX artifacts_approved_snapshot_idx ON artifacts(tenant_id,approved_snapshot_id,created_at DESC);
CREATE INDEX delivery_packages_project_idx ON delivery_packages(tenant_id,project_id,created_at DESC);
CREATE INDEX delivery_package_snapshots_snapshot_idx ON delivery_package_snapshots(tenant_id,approved_snapshot_id);
CREATE INDEX performance_observations_import_batch_idx ON performance_observations(tenant_id,import_batch_id,row_number);
CREATE INDEX performance_observations_snapshot_idx ON performance_observations(tenant_id,approved_snapshot_id,published_at DESC);
CREATE INDEX audit_project_idx ON audit_events(tenant_id,project_id,created_at DESC);

DO $$
DECLARE
  table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'brand_projects','membership_invites','project_templates','connect_sessions','devices','project_device_grants','cli_tokens',
    'bootstrap_attempts','bootstrap_progress_events','bootstrap_diagnostics','workspace_bindings','sources','source_revisions',
    'evidence_spans','assets','rights_records','context_snapshots','task_runs','run_attempts','creative_execution_bundles',
    'approval_decisions','review_cycles','review_comments',
    'review_grants','submissions','submission_revisions','source_disclosures','approved_snapshots','artifacts','delivery_packages',
    'delivery_package_snapshots','delivery_package_artifacts','performance_import_batches','performance_observations',
    'rating_decisions','audit_events'
  ]
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'',true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'',true)::uuid)',
      table_name
    );
  END LOOP;
END
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_device_token(p_hash text)
RETURNS TABLE(tenant_id uuid, device_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT d.tenant_id,d.id FROM devices d WHERE d.token_hash = p_hash AND d.revoked_at IS NULL LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_cli_token(p_hash text)
RETURNS TABLE(tenant_id uuid, token_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id,c.id FROM cli_tokens c WHERE c.token_hash = p_hash AND c.revoked_at IS NULL AND c.expires_at > now() LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_membership_invite(p_hash text)
RETURNS TABLE(tenant_id uuid, invite_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT i.tenant_id,i.id FROM membership_invites i WHERE i.token_hash = p_hash LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_review_token(p_hash text)
RETURNS TABLE(tenant_id uuid, grant_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT g.tenant_id,g.id FROM review_grants g WHERE g.token_hash = p_hash LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_workspace_token(p_hash text)
RETURNS TABLE(tenant_id uuid, workspace_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT w.tenant_id,w.id FROM workspace_bindings w
  WHERE w.credential_hash = p_hash AND w.status = 'active' AND w.revoked_at IS NULL LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_connect_session(p_id uuid)
RETURNS TABLE(tenant_id uuid, session_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id,c.id FROM connect_sessions c
  WHERE c.id = p_id AND c.state = 'waiting_for_computer' AND c.expires_at > now() LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_bootstrap_attempt(p_hash text)
RETURNS TABLE(tenant_id uuid, attempt_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT a.tenant_id,a.id FROM bootstrap_attempts a WHERE a.attempt_token_hash = p_hash LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_pending_source_revisions(p_limit integer)
RETURNS TABLE(tenant_id uuid, revision_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT r.tenant_id,r.id FROM source_revisions r
  WHERE r.processing_status = 'pending' ORDER BY r.created_at LIMIT greatest(1,least(p_limit,100))
$$;

CREATE OR REPLACE FUNCTION contentcloud_reject_immutable_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'immutable ContentCloud record cannot be changed' USING ERRCODE = '55000';
END;
$$;

DO $$
DECLARE
  table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'context_snapshots','creative_execution_bundles','approval_decisions','submission_revisions','source_disclosures',
    'approved_snapshots','artifacts','delivery_packages','delivery_package_snapshots','delivery_package_artifacts',
    'performance_import_batches','performance_observations','rating_decisions','audit_events'
  ]
  LOOP
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_immutable_mutation()',
      table_name || '_immutable',
      table_name
    );
  END LOOP;
END
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'contentcloud_runtime') THEN
    CREATE ROLE contentcloud_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS;
  END IF;
  EXECUTE format('GRANT contentcloud_runtime TO %I', current_user);
END
$$;

GRANT USAGE ON SCHEMA public TO contentcloud_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO contentcloud_runtime;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO contentcloud_runtime;
REVOKE ALL ON users,tenants,memberships,sessions,user_device_flows,contentcloud_schema_migrations FROM contentcloud_runtime;
REVOKE UPDATE,DELETE ON context_snapshots,creative_execution_bundles,approval_decisions,submission_revisions,source_disclosures,approved_snapshots,artifacts,delivery_packages,delivery_package_snapshots,delivery_package_artifacts,performance_import_batches,performance_observations,rating_decisions,audit_events FROM contentcloud_runtime;

ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO contentcloud_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE,SELECT ON SEQUENCES TO contentcloud_runtime;
