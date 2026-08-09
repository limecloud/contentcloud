-- +goose Up

ALTER TABLE runtime_checkpoints ADD COLUMN event_cursor bigint NOT NULL DEFAULT 0 CHECK (event_cursor >= 0);
ALTER TABLE runtime_checkpoints ADD COLUMN state_watermarks jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(state_watermarks) = 'object');
ALTER TABLE runtime_checkpoints ADD COLUMN side_effect_watermark text NOT NULL DEFAULT '';
ALTER TABLE runtime_checkpoints ADD COLUMN parent_checkpoint_id text NOT NULL DEFAULT '';

ALTER TABLE runtime_effects ADD COLUMN attempt_id text;
ALTER TABLE runtime_effects ADD COLUMN resource_reservation_id text;
ALTER TABLE runtime_effects ADD CONSTRAINT runtime_effect_attempt_fk FOREIGN KEY (tenant_id,attempt_id) REFERENCES runtime_attempts(tenant_id,id);
ALTER TABLE runtime_effects ADD CONSTRAINT runtime_effect_reservation_fk FOREIGN KEY (tenant_id,resource_reservation_id) REFERENCES runtime_resource_reservations(tenant_id,id);

CREATE TABLE runtime_state_collections (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  collection_key text NOT NULL,
  scope text NOT NULL CHECK (scope IN ('job','branch','node_private')),
  schema_id text NOT NULL,
  schema_revision integer NOT NULL CHECK (schema_revision > 0),
  consistency text NOT NULL CHECK (consistency IN ('single_writer','append_only','cas_map','reducer_owned')),
  writer_node_key text NOT NULL DEFAULT '',
  max_record_bytes integer NOT NULL CHECK (max_record_bytes > 0),
  max_records integer NOT NULL CHECK (max_records > 0),
  retention_policy text NOT NULL DEFAULT 'job',
  read_policy jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(read_policy) = 'array'),
  write_policy jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(write_policy) = 'array'),
  revision integer NOT NULL DEFAULT 0 CHECK (revision >= 0),
  watermark bigint NOT NULL DEFAULT 0 CHECK (watermark >= 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,job_run_id,collection_key),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_state_collections_job_idx ON runtime_state_collections(tenant_id,job_run_id,collection_key);

CREATE TABLE runtime_state_records (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  collection_id text NOT NULL,
  key text NOT NULL,
  value jsonb,
  artifact_ref text NOT NULL DEFAULT '',
  schema_revision integer NOT NULL CHECK (schema_revision > 0),
  version integer NOT NULL CHECK (version > 0),
  digest text NOT NULL,
  created_by text NOT NULL,
  updated_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,collection_id,key),
  FOREIGN KEY (tenant_id,collection_id) REFERENCES runtime_state_collections(tenant_id,id) ON DELETE CASCADE,
  CHECK ((value IS NULL) <> (artifact_ref = ''))
);
CREATE INDEX runtime_state_records_collection_idx ON runtime_state_records(tenant_id,collection_id,updated_at,key);

CREATE TABLE runtime_tool_calls (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  job_run_id text NOT NULL,
  node_run_id text NOT NULL,
  attempt_id text NOT NULL,
  agent_instance_id text NOT NULL,
  tool_name text NOT NULL,
  schema_version text NOT NULL,
  request_digest text NOT NULL,
  safe_request jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_request) = 'object'),
  result_digest text NOT NULL DEFAULT '',
  state text NOT NULL CHECK (state IN ('proposed','authorized','running','succeeded','failed','unknown')),
  error_code text NOT NULL DEFAULT '',
  started_at timestamptz,
  finished_at timestamptz,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,job_run_id) REFERENCES runtime_job_runs(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id) REFERENCES runtime_node_runs(tenant_id,job_run_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,attempt_id) REFERENCES runtime_attempts(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,job_run_id,node_run_id,agent_instance_id) REFERENCES runtime_agent_instances(tenant_id,job_run_id,node_run_id,id) ON DELETE CASCADE
);
CREATE INDEX runtime_tool_calls_attempt_idx ON runtime_tool_calls(tenant_id,attempt_id,created_at);

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['runtime_state_collections','runtime_state_records','runtime_tool_calls'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_setting(''app.tenant_id'', true)::uuid) WITH CHECK (tenant_id = current_setting(''app.tenant_id'', true)::uuid)', table_name);
  END LOOP;
END $$;

-- +goose Down

DROP TABLE IF EXISTS runtime_tool_calls;
DROP TABLE IF EXISTS runtime_state_records;
DROP TABLE IF EXISTS runtime_state_collections;
ALTER TABLE runtime_effects DROP CONSTRAINT IF EXISTS runtime_effect_reservation_fk;
ALTER TABLE runtime_effects DROP CONSTRAINT IF EXISTS runtime_effect_attempt_fk;
ALTER TABLE runtime_effects DROP COLUMN IF EXISTS resource_reservation_id;
ALTER TABLE runtime_effects DROP COLUMN IF EXISTS attempt_id;
ALTER TABLE runtime_checkpoints DROP COLUMN IF EXISTS parent_checkpoint_id;
ALTER TABLE runtime_checkpoints DROP COLUMN IF EXISTS side_effect_watermark;
ALTER TABLE runtime_checkpoints DROP COLUMN IF EXISTS state_watermarks;
ALTER TABLE runtime_checkpoints DROP COLUMN IF EXISTS event_cursor;
