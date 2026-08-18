CREATE TABLE workspace_objects (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL,
  content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  byte_size bigint NOT NULL CHECK (byte_size >= 0 AND byte_size <= 536870912),
  object_key text NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,project_id,content_digest),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE
);

CREATE TABLE workspace_upload_sessions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  device_id uuid NOT NULL,
  file_ref text NOT NULL,
  content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  byte_size bigint NOT NULL CHECK (byte_size >= 0 AND byte_size <= 536870912),
  chunk_size bigint NOT NULL CHECK (chunk_size > 0),
  part_count integer NOT NULL CHECK (part_count >= 0),
  state text NOT NULL CHECK (state IN ('initiated','uploading','completed','failed','expired')),
  object_key text NOT NULL,
  idempotency_key text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  UNIQUE (tenant_id,workspace_id,idempotency_key),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id) REFERENCES workspace_bindings(id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,device_id) REFERENCES devices(tenant_id,id)
);

CREATE TABLE workspace_upload_parts (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  session_id uuid NOT NULL REFERENCES workspace_upload_sessions(id) ON DELETE CASCADE,
  part_no integer NOT NULL CHECK (part_no >= 0),
  content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  byte_size bigint NOT NULL CHECK (byte_size >= 0),
  object_key text NOT NULL,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id,session_id,part_no)
);

CREATE TABLE workspace_revisions (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL,
  workspace_id uuid NOT NULL,
  device_id uuid NOT NULL,
  schema_version text NOT NULL CHECK (schema_version = 'contentcloud.workspace-revision/1.0'),
  revision_no bigint NOT NULL CHECK (revision_no > 0),
  base_revision_id uuid,
  content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
  files jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(files) = 'array'),
  client_mutation_id text NOT NULL CHECK (client_mutation_id <> ''),
  idempotency_key text NOT NULL CHECK (idempotency_key <> ''),
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id,workspace_id,revision_no),
  UNIQUE (tenant_id,workspace_id,idempotency_key),
  FOREIGN KEY (tenant_id,project_id) REFERENCES brand_projects(tenant_id,id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id) REFERENCES workspace_bindings(id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id,device_id) REFERENCES devices(tenant_id,id),
  FOREIGN KEY (base_revision_id) REFERENCES workspace_revisions(id)
);

CREATE INDEX workspace_revisions_project_created_idx
  ON workspace_revisions(tenant_id,project_id,created_at DESC);

ALTER TABLE workspace_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_revisions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_revisions
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

ALTER TABLE workspace_objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_objects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_objects USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
ALTER TABLE workspace_upload_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_upload_sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_upload_sessions USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
ALTER TABLE workspace_upload_parts ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_upload_parts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON workspace_upload_parts USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

GRANT SELECT,INSERT ON workspace_revisions,workspace_objects TO contentcloud_runtime;
GRANT SELECT,INSERT,UPDATE ON workspace_upload_sessions,workspace_upload_parts TO contentcloud_runtime;

-- +goose Down
DROP TABLE IF EXISTS workspace_revisions;
DROP TABLE IF EXISTS workspace_upload_parts;
DROP TABLE IF EXISTS workspace_upload_sessions;
DROP TABLE IF EXISTS workspace_objects;
