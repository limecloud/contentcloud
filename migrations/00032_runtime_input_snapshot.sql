ALTER TABLE runtime_job_runs
  ADD COLUMN input_snapshot_id text NOT NULL DEFAULT '';

ALTER TABLE runtime_job_runs
  ADD CONSTRAINT runtime_job_runs_input_snapshot_scope_ck CHECK (btrim(input_snapshot_id) <> '' OR business_type = 'runtime.job');

CREATE INDEX runtime_job_runs_input_snapshot_idx
  ON runtime_job_runs(tenant_id,input_snapshot_id)
  WHERE input_snapshot_id <> '';
