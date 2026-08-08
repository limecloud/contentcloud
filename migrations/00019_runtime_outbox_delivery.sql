-- +goose Up

ALTER TABLE runtime_outbox ADD COLUMN locked_by text NOT NULL DEFAULT '';
CREATE INDEX runtime_outbox_lock_idx ON runtime_outbox(tenant_id,locked_until,id) WHERE delivered_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS runtime_outbox_lock_idx;
ALTER TABLE runtime_outbox DROP COLUMN IF EXISTS locked_by;
