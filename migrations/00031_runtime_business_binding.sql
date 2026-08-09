ALTER TABLE runtime_job_runs
  ADD COLUMN business_type text NOT NULL DEFAULT 'runtime.job';

ALTER TABLE runtime_job_runs
  ADD CONSTRAINT runtime_job_runs_business_type_ck CHECK (btrim(business_type) <> '');

CREATE INDEX runtime_job_runs_business_type_idx
  ON runtime_job_runs(tenant_id,business_type,created_at DESC);
