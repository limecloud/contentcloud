-- +goose Up

ALTER TABLE work_tasks
  ADD COLUMN IF NOT EXISTS idempotency_key text NOT NULL DEFAULT ''
    CHECK (char_length(idempotency_key) <= 128);

CREATE UNIQUE INDEX IF NOT EXISTS work_tasks_idempotency_idx
  ON work_tasks(tenant_id,idempotency_key)
  WHERE idempotency_key <> '';

-- +goose Down

DROP INDEX IF EXISTS work_tasks_idempotency_idx;
ALTER TABLE work_tasks DROP COLUMN IF EXISTS idempotency_key;
