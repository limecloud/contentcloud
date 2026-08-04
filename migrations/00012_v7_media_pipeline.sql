-- +goose Up

ALTER TABLE artifacts DROP CONSTRAINT artifacts_kind_check;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_kind_check CHECK (kind IN ('delivery','generated_video','video_preview','final_render','storyboard_asset','prompt_package'));
ALTER TABLE artifacts ADD CONSTRAINT artifacts_tenant_id_id_key UNIQUE (tenant_id,id);

CREATE TABLE task_stage_outputs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  stage_run_id text NOT NULL,
  stage_id text NOT NULL,
  output_type text NOT NULL CHECK (output_type IN ('source_revision','evidence_set','knowledge_object','knowledge_snapshot','submission_revision','approved_snapshot','storyboard_package','artifact','generation_job','media_review','delivery_package')),
  object_id text NOT NULL,
  object_version integer NOT NULL DEFAULT 0 CHECK (object_version >= 0),
  object_digest text NOT NULL CHECK (object_digest ~ '^sha256:[0-9a-f]{64}$'),
  role text NOT NULL CHECK (role IN ('primary','supporting','preview','selected_take','final')),
  status text NOT NULL CHECK (status IN ('candidate','validated','approved','blocked','failed')),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,stage_run_id,output_type,object_id,role),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,stage_run_id) REFERENCES stage_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE provider_profiles (
  provider_id text NOT NULL,
  version text NOT NULL,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  adapter_version text NOT NULL,
  model text NOT NULL,
  region text NOT NULL,
  modes jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(modes) = 'array'),
  input_media_types jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_media_types) = 'array'),
  output_media_type text NOT NULL,
  limits jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(limits) = 'object'),
  data_retention text NOT NULL,
  pricing jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(pricing) = 'object'),
  status text NOT NULL CHECK (status IN ('draft','published','withdrawn')),
  verified_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (provider_id,version)
);

INSERT INTO provider_profiles(provider_id,version,digest,adapter_version,model,region,modes,input_media_types,output_media_type,limits,data_retention,pricing,status,verified_at,expires_at)
VALUES(
  'fake','1.0.0','sha256:0000000000000000000000000000000000000000000000000000000000000000','fake/1.0.0','fixture-video','local',
  '["text_to_video","image_to_video"]'::jsonb,'["image/jpeg","image/png","application/json"]'::jsonb,'video/mp4',
  '{"max_duration_seconds":60,"max_bytes":10485760}'::jsonb,'ephemeral','{"currency":"CNY","per_job_minor":0}'::jsonb,
  'published','2026-08-01T00:00:00Z','2099-01-01T00:00:00Z'
);

CREATE TABLE provider_bindings (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  provider_id text NOT NULL,
  profile_version text NOT NULL,
  state text NOT NULL CHECK (state IN ('active','disabled','misconfigured','budget_blocked')),
  credential_ref text NOT NULL DEFAULT '',
  egress_policy text NOT NULL,
  monthly_budget_minor bigint NOT NULL DEFAULT 0 CHECK (monthly_budget_minor >= 0),
  max_job_cost_minor bigint NOT NULL DEFAULT 0 CHECK (max_job_cost_minor >= 0),
  max_concurrency integer NOT NULL DEFAULT 1 CHECK (max_concurrency > 0),
  max_retries integer NOT NULL DEFAULT 0 CHECK (max_retries >= 0),
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,provider_id),
  FOREIGN KEY (provider_id,profile_version) REFERENCES provider_profiles(provider_id,version)
);

CREATE TABLE media_generation_jobs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  stage_run_id text NOT NULL,
  storyboard_snapshot_id text NOT NULL,
  prompt_package_artifact_id text NOT NULL DEFAULT '',
  provider_id text NOT NULL,
  profile_version text NOT NULL,
  profile_digest text NOT NULL CHECK (profile_digest ~ '^sha256:[0-9a-f]{64}$'),
  model text NOT NULL,
  mode text NOT NULL,
  aspect_ratio text NOT NULL,
  duration_seconds integer NOT NULL CHECK (duration_seconds > 0),
  input_artifact_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(input_artifact_refs) = 'array'),
  state text NOT NULL CHECK (state IN ('draft','awaiting_cost_approval','queued','submitting','submitted','generating','downloading','validating','succeeded','retry_wait','retryable_failed','output_invalid','failed','cancelled','budget_blocked','awaiting_external_result')),
  idempotency_key text NOT NULL,
  estimated_cost_minor bigint NOT NULL DEFAULT 0 CHECK (estimated_cost_minor >= 0),
  actual_cost_minor bigint NOT NULL DEFAULT 0 CHECK (actual_cost_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  max_attempts integer NOT NULL CHECK (max_attempts > 0),
  lease_owner text NOT NULL DEFAULT '',
  lease_expires_at timestamptz,
  cancel_requested_at timestamptz,
  error_code text NOT NULL DEFAULT '',
  error_detail_safe text NOT NULL DEFAULT '',
  row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,stage_run_id) REFERENCES stage_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (provider_id,profile_version) REFERENCES provider_profiles(provider_id,version)
);

CREATE UNIQUE INDEX media_generation_jobs_active_idx
  ON media_generation_jobs(tenant_id,task_id,stage_run_id,storyboard_snapshot_id,provider_id,mode)
  WHERE state IN ('draft','awaiting_cost_approval','queued','submitting','submitted','generating','downloading','validating','retry_wait','awaiting_external_result');
CREATE INDEX media_generation_jobs_claim_idx ON media_generation_jobs(tenant_id,state,updated_at,created_at);

CREATE TABLE provider_attempts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  generation_job_id text NOT NULL,
  attempt_number integer NOT NULL CHECK (attempt_number > 0),
  provider_id text NOT NULL,
  request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
  external_job_id text NOT NULL DEFAULT '',
  provider_state text NOT NULL,
  safe_request_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_request_summary) = 'object'),
  safe_response_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_response_summary) = 'object'),
  disclosure_manifest jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(disclosure_manifest) = 'object'),
  http_status integer NOT NULL DEFAULT 0,
  provider_request_id text NOT NULL DEFAULT '',
  estimated_cost_minor bigint NOT NULL DEFAULT 0 CHECK (estimated_cost_minor >= 0),
  actual_cost_minor bigint NOT NULL DEFAULT 0 CHECK (actual_cost_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  last_polled_at timestamptz,
  next_poll_at timestamptz,
  submitted_at timestamptz,
  downloaded_at timestamptz,
  completed_at timestamptz,
  retry_after_seconds integer NOT NULL DEFAULT 0 CHECK (retry_after_seconds >= 0),
  error_code text NOT NULL DEFAULT '',
  error_detail_safe text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,generation_job_id,attempt_number),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,generation_job_id) REFERENCES media_generation_jobs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE media_reviews (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  task_id text NOT NULL,
  generation_job_id text NOT NULL DEFAULT '',
  subject_artifact_id uuid NOT NULL,
  subject_digest text NOT NULL CHECK (subject_digest ~ '^sha256:[0-9a-f]{64}$'),
  review_kind text NOT NULL CHECK (review_kind IN ('technical','content','final')),
  status text NOT NULL CHECK (status IN ('pending','approved','changes_requested','rejected')),
  checks jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(checks) = 'object'),
  selected boolean NOT NULL DEFAULT false,
  decision_reason text NOT NULL DEFAULT '',
  decided_by text NOT NULL DEFAULT '',
  decided_at timestamptz,
  row_version integer NOT NULL DEFAULT 1 CHECK (row_version > 0),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,task_id,review_kind,subject_artifact_id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,task_id) REFERENCES work_tasks(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,subject_artifact_id) REFERENCES artifacts(tenant_id,id)
);

CREATE UNIQUE INDEX media_reviews_selected_take_idx ON media_reviews(tenant_id,task_id,review_kind) WHERE selected;

ALTER TABLE task_deliveries
  ADD COLUMN delivery_package_id uuid REFERENCES delivery_packages(id),
  ADD COLUMN integrity_status text NOT NULL DEFAULT 'unclassified'
    CHECK (integrity_status IN ('complete','script_only','legacy_incomplete','missing_artifact','unclassified'));

UPDATE task_deliveries
SET integrity_status = CASE
  WHEN status = 'delivered' AND jsonb_array_length(manifest) = 0 THEN 'legacy_incomplete'
  WHEN status = 'delivered' THEN 'unclassified'
  ELSE 'unclassified'
END;

CREATE INDEX task_stage_outputs_task_idx ON task_stage_outputs(tenant_id,task_id,created_at);
CREATE INDEX provider_attempts_job_idx ON provider_attempts(tenant_id,generation_job_id,attempt_number);
CREATE INDEX media_reviews_task_idx ON media_reviews(tenant_id,task_id,created_at);

CREATE OR REPLACE FUNCTION contentcloud_pending_media_generation_jobs(p_limit integer)
RETURNS TABLE(tenant_id uuid, job_id text)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT j.tenant_id,j.id FROM media_generation_jobs j
  WHERE j.state IN ('queued','retry_wait')
  ORDER BY j.updated_at,j.created_at
  LIMIT greatest(1,least(p_limit,100))
$$;

ALTER TABLE task_stage_outputs ENABLE ROW LEVEL SECURITY;
ALTER TABLE task_stage_outputs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON task_stage_outputs USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE provider_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON provider_bindings USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE media_generation_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_generation_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON media_generation_jobs USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE provider_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE provider_attempts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON provider_attempts USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE media_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON media_reviews USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT ON provider_profiles TO contentcloud_runtime;
GRANT SELECT,INSERT,UPDATE,DELETE ON task_stage_outputs,provider_bindings,media_generation_jobs,provider_attempts,media_reviews TO contentcloud_runtime;

-- +goose Down

DROP FUNCTION IF EXISTS contentcloud_pending_media_generation_jobs(integer);
ALTER TABLE task_deliveries DROP COLUMN IF EXISTS integrity_status;
ALTER TABLE task_deliveries DROP COLUMN IF EXISTS delivery_package_id;
DROP TABLE IF EXISTS media_reviews;
DROP TABLE IF EXISTS provider_attempts;
DROP TABLE IF EXISTS media_generation_jobs;
DROP TABLE IF EXISTS provider_bindings;
DROP TABLE IF EXISTS provider_profiles;
DROP TABLE IF EXISTS task_stage_outputs;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_kind_check;
ALTER TABLE artifacts DROP CONSTRAINT artifacts_tenant_id_id_key;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_kind_check CHECK (kind = 'delivery');
