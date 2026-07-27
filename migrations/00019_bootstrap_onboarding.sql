-- +goose Up

DROP FUNCTION IF EXISTS contentcloud_lookup_connect_key(text);
ALTER TABLE connect_sessions DROP COLUMN IF EXISTS connect_key_hash;

CREATE TABLE IF NOT EXISTS bootstrap_attempts (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  connect_session_id uuid NOT NULL REFERENCES connect_sessions(id),
  attempt_token_hash text NOT NULL UNIQUE,
  code_challenge text NOT NULL,
  user_code text NOT NULL UNIQUE,
  state text NOT NULL CHECK (state IN ('pending','approved','denied','consumed','completed','failed','expired')),
  support_code text NOT NULL UNIQUE,
  last_sequence bigint NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
  decided_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  decided_at timestamptz,
  consumed_at timestamptz,
  completed_at timestamptz
);

-- An unreleased bootstrap build reached production with the old approval column names.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'bootstrap_attempts' AND column_name = 'approved_by'
  ) AND NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'bootstrap_attempts' AND column_name = 'decided_by'
  ) THEN
    ALTER TABLE bootstrap_attempts RENAME COLUMN approved_by TO decided_by;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'bootstrap_attempts' AND column_name = 'approved_at'
  ) AND NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'bootstrap_attempts' AND column_name = 'decided_at'
  ) THEN
    ALTER TABLE bootstrap_attempts RENAME COLUMN approved_at TO decided_at;
  END IF;
END
$$;

ALTER TABLE bootstrap_attempts
  ADD COLUMN IF NOT EXISTS decided_by uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS decided_at timestamptz;

CREATE INDEX IF NOT EXISTS bootstrap_attempts_session_created_idx
  ON bootstrap_attempts(tenant_id, connect_session_id, created_at DESC);

CREATE TABLE IF NOT EXISTS bootstrap_progress_events (
  attempt_id uuid NOT NULL REFERENCES bootstrap_attempts(id),
  sequence bigint NOT NULL CHECK (sequence > 0),
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  schema_version text NOT NULL CHECK (schema_version = '1.0'),
  occurred_at timestamptz NOT NULL,
  stage text NOT NULL,
  status text NOT NULL CHECK (status IN ('started','passed','needs_action','failed','skipped')),
  check_id text NOT NULL DEFAULT '',
  error_code text NOT NULL DEFAULT '',
  action_id text NOT NULL DEFAULT '',
  facts jsonb NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (attempt_id, sequence)
);

CREATE TABLE IF NOT EXISTS bootstrap_diagnostics (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  attempt_id uuid NOT NULL REFERENCES bootstrap_attempts(id),
  support_code text NOT NULL,
  digest text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size >= 0 AND byte_size <= 262144),
  summary jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  UNIQUE (attempt_id, digest)
);

ALTER TABLE bootstrap_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE bootstrap_progress_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE bootstrap_diagnostics ENABLE ROW LEVEL SECURITY;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = 'public' AND tablename = 'bootstrap_attempts' AND policyname = 'tenant_bootstrap_attempts') THEN
    CREATE POLICY tenant_bootstrap_attempts ON bootstrap_attempts
      USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
      WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = 'public' AND tablename = 'bootstrap_progress_events' AND policyname = 'tenant_bootstrap_progress_events') THEN
    CREATE POLICY tenant_bootstrap_progress_events ON bootstrap_progress_events
      USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
      WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = 'public' AND tablename = 'bootstrap_diagnostics' AND policyname = 'tenant_bootstrap_diagnostics') THEN
    CREATE POLICY tenant_bootstrap_diagnostics ON bootstrap_diagnostics
      USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
      WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
  END IF;
END
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_connect_session(p_id uuid)
RETURNS TABLE(tenant_id uuid, session_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id, c.id
  FROM connect_sessions c
  WHERE c.id = p_id AND c.state = 'waiting_for_computer' AND c.expires_at > now()
  LIMIT 1
$$;

CREATE OR REPLACE FUNCTION contentcloud_lookup_bootstrap_attempt(p_hash text)
RETURNS TABLE(tenant_id uuid, attempt_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT a.tenant_id, a.id
  FROM bootstrap_attempts a
  WHERE a.attempt_token_hash = p_hash
  LIMIT 1
$$;

-- +goose Down

DROP FUNCTION IF EXISTS contentcloud_lookup_bootstrap_attempt(text);
DROP FUNCTION IF EXISTS contentcloud_lookup_connect_session(uuid);
DROP TABLE IF EXISTS bootstrap_diagnostics, bootstrap_progress_events, bootstrap_attempts;
ALTER TABLE connect_sessions ADD COLUMN IF NOT EXISTS connect_key_hash text;
CREATE UNIQUE INDEX IF NOT EXISTS connect_sessions_connect_key_hash_key ON connect_sessions(connect_key_hash);
CREATE OR REPLACE FUNCTION contentcloud_lookup_connect_key(p_hash text)
RETURNS TABLE(tenant_id uuid, session_id uuid)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT c.tenant_id, c.id FROM connect_sessions c WHERE c.connect_key_hash = p_hash LIMIT 1
$$;
