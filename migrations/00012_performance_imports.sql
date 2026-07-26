-- +goose Up
CREATE TABLE performance_import_batches (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  source_name text NOT NULL CHECK (char_length(source_name) BETWEEN 1 AND 255),
  source_format text NOT NULL CHECK (source_format IN ('manual','json','csv','xlsx')),
  source_sha256 text NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  currency text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
  row_count integer NOT NULL CHECK (row_count > 0 AND row_count <= 1000),
  imported_count integer NOT NULL CHECK (imported_count = row_count),
  status text NOT NULL CHECK (status = 'imported'),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL
);

ALTER TABLE performance_observations
  ADD COLUMN import_batch_id uuid REFERENCES performance_import_batches(id),
  ADD COLUMN row_number integer,
  ADD COLUMN currency text NOT NULL DEFAULT '' CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),
  ADD COLUMN spend double precision NOT NULL DEFAULT 0 CHECK (spend >= 0 AND spend <> 'Infinity'::double precision AND spend <> 'NaN'::double precision),
  ADD COLUMN gmv double precision NOT NULL DEFAULT 0 CHECK (gmv >= 0 AND gmv <> 'Infinity'::double precision AND gmv <> 'NaN'::double precision),
  ADD COLUMN roi double precision CHECK (roi IS NULL OR (roi >= 0 AND roi <> 'Infinity'::double precision AND roi <> 'NaN'::double precision)),
  ADD COLUMN dedup_key text;

CREATE UNIQUE INDEX performance_observations_dedup_unique
  ON performance_observations(tenant_id, project_id, dedup_key)
  WHERE dedup_key IS NOT NULL;
CREATE INDEX performance_observations_import_batch_idx
  ON performance_observations(tenant_id, import_batch_id, row_number);

CREATE TABLE rating_decisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  subject_type text NOT NULL CHECK (subject_type IN ('script_version','content_framework','shot_pattern')),
  subject_id uuid NOT NULL,
  observation_ids jsonb NOT NULL CHECK (jsonb_typeof(observation_ids) = 'array' AND jsonb_array_length(observation_ids) BETWEEN 1 AND 100),
  rating text NOT NULL CHECK (rating IN ('seed_candidate','repairable','discarded','insufficient_sample')),
  reason text NOT NULL CHECK (char_length(btrim(reason)) > 0),
  next_action text NOT NULL CHECK (char_length(btrim(next_action)) > 0),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL
);

ALTER TABLE performance_import_batches ENABLE ROW LEVEL SECURITY;
ALTER TABLE rating_decisions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_performance_import_batches ON performance_import_batches
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_rating_decisions ON rating_decisions
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

CREATE OR REPLACE FUNCTION contentcloud_reject_performance_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'performance records are immutable' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER performance_import_batches_immutable
  BEFORE UPDATE OR DELETE ON performance_import_batches
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_performance_mutation();
CREATE TRIGGER performance_observations_immutable
  BEFORE UPDATE OR DELETE ON performance_observations
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_performance_mutation();
CREATE TRIGGER rating_decisions_immutable
  BEFORE UPDATE OR DELETE ON rating_decisions
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_performance_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS rating_decisions_immutable ON rating_decisions;
DROP TRIGGER IF EXISTS performance_observations_immutable ON performance_observations;
DROP TRIGGER IF EXISTS performance_import_batches_immutable ON performance_import_batches;
DROP FUNCTION IF EXISTS contentcloud_reject_performance_mutation();
DROP TABLE IF EXISTS rating_decisions;
DROP INDEX IF EXISTS performance_observations_import_batch_idx;
DROP INDEX IF EXISTS performance_observations_dedup_unique;
ALTER TABLE performance_observations
  DROP COLUMN IF EXISTS dedup_key,
  DROP COLUMN IF EXISTS roi,
  DROP COLUMN IF EXISTS gmv,
  DROP COLUMN IF EXISTS spend,
  DROP COLUMN IF EXISTS currency,
  DROP COLUMN IF EXISTS row_number,
  DROP COLUMN IF EXISTS import_batch_id;
DROP TABLE IF EXISTS performance_import_batches;
