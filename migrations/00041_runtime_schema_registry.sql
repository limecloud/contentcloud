-- +goose Up

CREATE TABLE runtime_schemas (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  schema_id text NOT NULL,
  revision integer NOT NULL CHECK (revision > 0),
  status text NOT NULL CHECK (status IN ('draft','published','retired')),
  compatibility text NOT NULL CHECK (compatibility IN ('backward','full','none')),
  definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  retention_policy text NOT NULL,
  retain_until timestamptz,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  published_at timestamptz,
  retired_at timestamptz,
  version integer NOT NULL DEFAULT 1 CHECK (version > 0),
  PRIMARY KEY (tenant_id,schema_id,revision),
  CHECK ((status = 'draft' AND published_at IS NULL AND retired_at IS NULL) OR (status = 'published' AND published_at IS NOT NULL AND retired_at IS NULL) OR (status = 'retired' AND published_at IS NOT NULL AND retired_at IS NOT NULL))
);
CREATE INDEX runtime_schemas_status_idx ON runtime_schemas(tenant_id,schema_id,status,revision DESC);
ALTER TABLE runtime_schemas ENABLE ROW LEVEL SECURITY;
ALTER TABLE runtime_schemas FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON runtime_schemas USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- +goose Down

DROP TABLE IF EXISTS runtime_schemas;
