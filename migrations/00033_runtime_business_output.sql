ALTER TABLE runtime_job_runs
  ADD COLUMN business_output_count integer NOT NULL DEFAULT 0;

ALTER TABLE runtime_job_runs
  ADD CONSTRAINT runtime_job_runs_business_output_count_ck CHECK (business_output_count >= 0);
