-- +goose Up

CREATE TABLE runtime_provider_inbox (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  provider_id text NOT NULL,
  message_id text NOT NULL,
  received_digest text NOT NULL CHECK (received_digest ~ '^sha256:[0-9a-f]{64}$'),
  external_id text NOT NULL,
  effect_id text NOT NULL DEFAULT '',
  provider_state text NOT NULL,
  response_digest text NOT NULL CHECK (response_digest ~ '^sha256:[0-9a-f]{64}$'),
  cost_minor bigint NOT NULL DEFAULT 0 CHECK (cost_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  safe_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_payload) = 'object'),
  state text NOT NULL CHECK (state IN ('received','applied','pending_reconciliation','failed')),
  error_code text NOT NULL DEFAULT '',
  received_at timestamptz NOT NULL,
  processed_at timestamptz,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,provider_id,message_id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE runtime_provider_reconciliations (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  effect_id text NOT NULL DEFAULT '',
  provider_id text NOT NULL,
  external_id text NOT NULL,
  request_key text NOT NULL,
  observed_state text NOT NULL,
  response_digest text NOT NULL CHECK (response_digest ~ '^sha256:[0-9a-f]{64}$'),
  expected_minor bigint NOT NULL DEFAULT 0 CHECK (expected_minor >= 0),
  observed_minor bigint NOT NULL DEFAULT 0 CHECK (observed_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  reason text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('pending','matched','cost_mismatch','manual_action')),
  safe_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_summary) = 'object'),
  resolved_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,request_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE runtime_provider_bills (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  provider_id text NOT NULL,
  bill_id text NOT NULL,
  external_id text NOT NULL,
  effect_id text NOT NULL DEFAULT '',
  bill_digest text NOT NULL CHECK (bill_digest ~ '^sha256:[0-9a-f]{64}$'),
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  status text NOT NULL CHECK (status IN ('matched','disputed','unmatched')),
  observed_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,provider_id,bill_id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX runtime_provider_inbox_pending_idx ON runtime_provider_inbox(tenant_id,state,received_at);
CREATE INDEX runtime_provider_reconciliations_effect_idx ON runtime_provider_reconciliations(tenant_id,effect_id,created_at);
CREATE INDEX runtime_provider_bills_effect_idx ON runtime_provider_bills(tenant_id,effect_id,observed_at);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_provider_inbox','runtime_provider_reconciliations','runtime_provider_bills'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

REVOKE UPDATE,DELETE ON runtime_provider_inbox,runtime_provider_reconciliations,runtime_provider_bills FROM contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS runtime_provider_bills;
DROP TABLE IF EXISTS runtime_provider_reconciliations;
DROP TABLE IF EXISTS runtime_provider_inbox;
