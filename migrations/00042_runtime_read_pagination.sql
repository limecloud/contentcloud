-- +goose Up

CREATE UNIQUE INDEX runtime_tool_calls_gateway_idempotency_idx
  ON runtime_tool_calls(tenant_id,attempt_id,tool_name,(safe_request->>'idempotency_key'))
  WHERE safe_request ? 'idempotency_key' AND length(btrim(safe_request->>'idempotency_key')) > 0;

CREATE INDEX runtime_job_runs_explorer_idx
  ON runtime_job_runs(tenant_id,updated_at DESC,id DESC);
CREATE INDEX runtime_job_runs_explorer_project_idx
  ON runtime_job_runs(tenant_id,project_id,state,updated_at DESC,id DESC);
CREATE INDEX runtime_node_runs_explorer_idx
  ON runtime_node_runs(tenant_id,job_run_id,created_at,id);
CREATE INDEX runtime_effects_explorer_idx
  ON runtime_effects(tenant_id,job_run_id,created_at,id);
CREATE INDEX runtime_checkpoints_explorer_idx
  ON runtime_checkpoints(tenant_id,job_run_id,created_at DESC,id DESC);

-- +goose Down

DROP INDEX IF EXISTS runtime_checkpoints_explorer_idx;
DROP INDEX IF EXISTS runtime_effects_explorer_idx;
DROP INDEX IF EXISTS runtime_node_runs_explorer_idx;
DROP INDEX IF EXISTS runtime_job_runs_explorer_project_idx;
DROP INDEX IF EXISTS runtime_job_runs_explorer_idx;
DROP INDEX IF EXISTS runtime_tool_calls_gateway_idempotency_idx;
