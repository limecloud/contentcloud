-- +goose Up

CREATE TABLE channel_callback_receipts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  publication_id text NOT NULL,
  adapter_id text NOT NULL,
  event_id text NOT NULL,
  payload_digest text NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  state text NOT NULL CHECK (state IN ('submitted','published','failed','unknown','withdrawn')),
  safe_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_summary) = 'object'),
  observed_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,adapter_id,event_id),
  FOREIGN KEY (tenant_id,publication_id) REFERENCES channel_publications(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX channel_callback_publication_idx ON channel_callback_receipts(tenant_id,publication_id,received_at DESC);

ALTER TABLE channel_callback_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE channel_callback_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON channel_callback_receipts USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT ON channel_callback_receipts TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS channel_callback_receipts;
