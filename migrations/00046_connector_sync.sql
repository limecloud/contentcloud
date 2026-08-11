-- +goose Up

CREATE TABLE connector_bindings (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  connector_id text NOT NULL,
  authorization_ref text NOT NULL CHECK (authorization_ref ~ '^(secret|vault|env)://'),
  region text NOT NULL DEFAULT 'global',
  status text NOT NULL CHECK (status IN ('active','disabled')),
  cursor text NOT NULL DEFAULT '',
  sync_lease_owner text NOT NULL DEFAULT '',
  sync_lease_expires_at timestamptz NOT NULL DEFAULT 'epoch'::timestamptz,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,project_id,connector_id,region),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE connector_record_mappings (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL,
  binding_id text NOT NULL,
  external_id text NOT NULL,
  external_version text NOT NULL,
  source_id uuid,
  revision_id uuid,
  digest text NOT NULL DEFAULT '' CHECK (digest = '' OR digest ~ '^sha256:[0-9a-f]{64}$'),
  source_url text NOT NULL DEFAULT '',
  deleted boolean NOT NULL DEFAULT false,
  deleted_at timestamptz,
  rights jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(rights) = 'object'),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  observed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,binding_id,external_id),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,binding_id) REFERENCES connector_bindings(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (source_id) REFERENCES sources(id),
  FOREIGN KEY (revision_id) REFERENCES source_revisions(id),
  CHECK ((deleted AND deleted_at IS NOT NULL) OR (NOT deleted AND source_id IS NOT NULL AND revision_id IS NOT NULL AND digest <> ''))
);

CREATE TABLE connector_sync_receipts (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL,
  binding_id text NOT NULL,
  connector_id text NOT NULL,
  previous_cursor text NOT NULL,
  next_cursor text NOT NULL,
  records jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(records) = 'array'),
  upsert_count integer NOT NULL CHECK (upsert_count >= 0),
  tombstone_count integer NOT NULL CHECK (tombstone_count >= 0),
  has_more boolean NOT NULL,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  observed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,binding_id,previous_cursor,next_cursor,digest),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,binding_id) REFERENCES connector_bindings(tenant_id,id) ON DELETE CASCADE
);

CREATE INDEX connector_records_source_idx ON connector_record_mappings(tenant_id,source_id) WHERE deleted = false;
CREATE INDEX connector_receipts_binding_idx ON connector_sync_receipts(tenant_id,binding_id,observed_at DESC);

ALTER TABLE connector_bindings ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_bindings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON connector_bindings USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE connector_record_mappings ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_record_mappings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON connector_record_mappings USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
ALTER TABLE connector_sync_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE connector_sync_receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON connector_sync_receipts USING (tenant_id = current_setting('app.tenant_id', true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT,INSERT,UPDATE,DELETE ON connector_bindings,connector_record_mappings,connector_sync_receipts TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS connector_sync_receipts;
DROP TABLE IF EXISTS connector_record_mappings;
DROP TABLE IF EXISTS connector_bindings;
