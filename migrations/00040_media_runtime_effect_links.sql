-- +goose Up

-- Runtime-managed media jobs carry explicit identity into the media projection.
-- Historical V7 rows remain unlinked and are never guessed into a JobRun.
ALTER TABLE media_generation_jobs
  ADD COLUMN runtime_job_run_id text,
  ADD COLUMN runtime_node_run_id text,
  ADD COLUMN runtime_attempt_id text,
  ADD COLUMN runtime_effect_id text,
  ADD CONSTRAINT media_generation_jobs_runtime_scope_ck
    CHECK ((runtime_job_run_id IS NULL) = (runtime_node_run_id IS NULL));

ALTER TABLE media_generation_jobs
  ADD CONSTRAINT media_generation_jobs_runtime_job_fk
    FOREIGN KEY (tenant_id, runtime_job_run_id) REFERENCES runtime_job_runs(tenant_id, id),
  ADD CONSTRAINT media_generation_jobs_runtime_node_fk
    FOREIGN KEY (tenant_id, runtime_node_run_id) REFERENCES runtime_node_runs(tenant_id, id),
  ADD CONSTRAINT media_generation_jobs_runtime_attempt_fk
    FOREIGN KEY (tenant_id, runtime_attempt_id) REFERENCES runtime_attempts(tenant_id, id),
  ADD CONSTRAINT media_generation_jobs_runtime_effect_fk
    FOREIGN KEY (tenant_id, runtime_effect_id) REFERENCES runtime_effects(tenant_id, id);

ALTER TABLE provider_attempts
  ADD COLUMN runtime_job_run_id text,
  ADD COLUMN runtime_node_run_id text,
  ADD COLUMN runtime_attempt_id text,
  ADD COLUMN runtime_effect_id text,
  ADD CONSTRAINT provider_attempts_runtime_scope_ck
    CHECK ((runtime_job_run_id IS NULL) = (runtime_node_run_id IS NULL));

ALTER TABLE provider_attempts
  ADD CONSTRAINT provider_attempts_runtime_job_fk
    FOREIGN KEY (tenant_id, runtime_job_run_id) REFERENCES runtime_job_runs(tenant_id, id),
  ADD CONSTRAINT provider_attempts_runtime_node_fk
    FOREIGN KEY (tenant_id, runtime_node_run_id) REFERENCES runtime_node_runs(tenant_id, id),
  ADD CONSTRAINT provider_attempts_runtime_attempt_fk
    FOREIGN KEY (tenant_id, runtime_attempt_id) REFERENCES runtime_attempts(tenant_id, id),
  ADD CONSTRAINT provider_attempts_runtime_effect_fk
    FOREIGN KEY (tenant_id, runtime_effect_id) REFERENCES runtime_effects(tenant_id, id);

CREATE INDEX media_generation_jobs_runtime_effect_idx ON media_generation_jobs(tenant_id, runtime_effect_id) WHERE runtime_effect_id IS NOT NULL;
CREATE INDEX provider_attempts_runtime_effect_idx ON provider_attempts(tenant_id, runtime_effect_id) WHERE runtime_effect_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS provider_attempts_runtime_effect_idx;
DROP INDEX IF EXISTS media_generation_jobs_runtime_effect_idx;
ALTER TABLE provider_attempts
  DROP CONSTRAINT IF EXISTS provider_attempts_runtime_effect_fk,
  DROP CONSTRAINT IF EXISTS provider_attempts_runtime_attempt_fk,
  DROP CONSTRAINT IF EXISTS provider_attempts_runtime_node_fk,
  DROP CONSTRAINT IF EXISTS provider_attempts_runtime_job_fk,
  DROP CONSTRAINT IF EXISTS provider_attempts_runtime_scope_ck,
  DROP COLUMN IF EXISTS runtime_effect_id,
  DROP COLUMN IF EXISTS runtime_attempt_id,
  DROP COLUMN IF EXISTS runtime_node_run_id,
  DROP COLUMN IF EXISTS runtime_job_run_id;
ALTER TABLE media_generation_jobs
  DROP CONSTRAINT IF EXISTS media_generation_jobs_runtime_effect_fk,
  DROP CONSTRAINT IF EXISTS media_generation_jobs_runtime_attempt_fk,
  DROP CONSTRAINT IF EXISTS media_generation_jobs_runtime_node_fk,
  DROP CONSTRAINT IF EXISTS media_generation_jobs_runtime_job_fk,
  DROP CONSTRAINT IF EXISTS media_generation_jobs_runtime_scope_ck,
  DROP COLUMN IF EXISTS runtime_effect_id,
  DROP COLUMN IF EXISTS runtime_attempt_id,
  DROP COLUMN IF EXISTS runtime_node_run_id,
  DROP COLUMN IF EXISTS runtime_job_run_id;
