-- +goose Up

CREATE TABLE runtime_agent_sessions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  harness_kind text NOT NULL,
  session_id text NOT NULL,
  state text NOT NULL CHECK (state IN ('active','interrupted','completed','failed')),
  last_event_at timestamptz NOT NULL,
  error_code text NOT NULL DEFAULT '',
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,harness_kind,session_id)
);

CREATE TABLE runtime_agent_session_events (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  harness_kind text NOT NULL,
  session_id text NOT NULL,
  sequence bigint NOT NULL CHECK (sequence > 0),
  event_type text NOT NULL,
  event_data jsonb NOT NULL DEFAULT 'null'::jsonb,
  error_code text NOT NULL DEFAULT '',
  occurred_at timestamptz NOT NULL,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,harness_kind,session_id,sequence),
  UNIQUE (tenant_id,harness_kind,session_id,digest),
  FOREIGN KEY (tenant_id,harness_kind,session_id) REFERENCES runtime_agent_sessions(tenant_id,harness_kind,session_id) ON DELETE CASCADE
);
CREATE INDEX runtime_agent_sessions_state_idx ON runtime_agent_sessions(tenant_id,state,updated_at);
CREATE INDEX runtime_agent_session_events_time_idx ON runtime_agent_session_events(tenant_id,occurred_at);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_agent_sessions','runtime_agent_session_events'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

-- +goose Down

DROP TABLE IF EXISTS runtime_agent_session_events;
DROP TABLE IF EXISTS runtime_agent_sessions;
