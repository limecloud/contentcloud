-- +goose Up

CREATE TABLE runtime_outbox_receipts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  message_id text NOT NULL,
  subscriber text NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at timestamptz NOT NULL,
  locked_by text NOT NULL DEFAULT '',
  locked_until timestamptz,
  delivered_at timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,message_id,subscriber),
  FOREIGN KEY (tenant_id,message_id) REFERENCES runtime_outbox(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_outbox_receipts_ready_idx ON runtime_outbox_receipts(tenant_id,subscriber,next_attempt_at,message_id) WHERE delivered_at IS NULL;
CREATE INDEX runtime_outbox_receipts_lock_idx ON runtime_outbox_receipts(tenant_id,subscriber,locked_until,message_id) WHERE delivered_at IS NULL;

ALTER TABLE runtime_outbox_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_outbox_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_outbox_receipts
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

INSERT INTO runtime_outbox_receipts(tenant_id,message_id,subscriber,attempts,next_attempt_at,locked_by,locked_until,delivered_at,last_error,created_at)
SELECT tenant_id,id,'runtime_projection',attempts,next_attempt_at,locked_by,locked_until,delivered_at,last_error,created_at
FROM runtime_outbox;

-- Business results were synchronously applied before this migration. Replaying
-- every successful attempt is safe because business imports are digest-fenced
-- and ensures a crash during upgrade cannot leave a terminal result unapplied.
INSERT INTO runtime_outbox_receipts(tenant_id,message_id,subscriber,attempts,next_attempt_at,created_at)
SELECT tenant_id,id,'runtime_business_result',0,created_at,created_at
FROM runtime_outbox
WHERE payload->>'type' = 'attempt.succeeded';

DROP INDEX IF EXISTS runtime_outbox_ready_idx;
DROP INDEX IF EXISTS runtime_outbox_lock_idx;
ALTER TABLE runtime_outbox DROP COLUMN attempts;
ALTER TABLE runtime_outbox DROP COLUMN next_attempt_at;
ALTER TABLE runtime_outbox DROP COLUMN locked_by;
ALTER TABLE runtime_outbox DROP COLUMN locked_until;
ALTER TABLE runtime_outbox DROP COLUMN delivered_at;
ALTER TABLE runtime_outbox DROP COLUMN last_error;

REVOKE UPDATE,DELETE ON runtime_outbox FROM contentcloud_runtime;
GRANT SELECT,INSERT ON runtime_outbox TO contentcloud_runtime;
GRANT SELECT,INSERT,UPDATE ON runtime_outbox_receipts TO contentcloud_runtime;

-- +goose Down

ALTER TABLE runtime_outbox ADD COLUMN attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0);
ALTER TABLE runtime_outbox ADD COLUMN next_attempt_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE runtime_outbox ADD COLUMN locked_by text NOT NULL DEFAULT '';
ALTER TABLE runtime_outbox ADD COLUMN locked_until timestamptz;
ALTER TABLE runtime_outbox ADD COLUMN delivered_at timestamptz;
ALTER TABLE runtime_outbox ADD COLUMN last_error text NOT NULL DEFAULT '';

UPDATE runtime_outbox AS message
SET attempts=receipt.attempts,
    next_attempt_at=receipt.next_attempt_at,
    locked_by=receipt.locked_by,
    locked_until=receipt.locked_until,
    delivered_at=receipt.delivered_at,
    last_error=receipt.last_error
FROM runtime_outbox_receipts AS receipt
WHERE receipt.tenant_id=message.tenant_id
  AND receipt.message_id=message.id
  AND receipt.subscriber='runtime_projection';

CREATE INDEX runtime_outbox_ready_idx ON runtime_outbox(tenant_id,next_attempt_at,id) WHERE delivered_at IS NULL;
CREATE INDEX runtime_outbox_lock_idx ON runtime_outbox(tenant_id,locked_until,id) WHERE delivered_at IS NULL;
GRANT UPDATE ON runtime_outbox TO contentcloud_runtime;
DROP TABLE IF EXISTS runtime_outbox_receipts;
