ALTER TABLE runtime_job_runs
  DROP CONSTRAINT IF EXISTS runtime_job_runs_tenant_id_work_task_id_fkey;

DROP TABLE IF EXISTS run_progress_events;
DROP TABLE IF EXISTS run_attempts;
DROP TABLE IF EXISTS creative_execution_bundles;
DROP TABLE IF EXISTS task_runs;
