-- +goose Up

CREATE TABLE knowledge_objects (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  id text NOT NULL,
  version integer NOT NULL CHECK (version > 0),
  object_type text NOT NULL,
  layer text NOT NULL CHECK (layer IN ('identity','product','market','expression','operations','content_engine','compliance')),
  status text NOT NULL,
  title text NOT NULL DEFAULT '',
  statement text NOT NULL DEFAULT '',
  payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
  dimensions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(dimensions) = 'array'),
  allowed_channels jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(allowed_channels) = 'array'),
  evidence_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(evidence_refs) = 'array'),
  relation_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(relation_refs) = 'array'),
  rights_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rights_refs) = 'array'),
  conflict_refs jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(conflict_refs) = 'array'),
  decision_ref text NOT NULL DEFAULT '',
  next_action text NOT NULL DEFAULT '',
  impact text NOT NULL DEFAULT '',
  valid_from timestamptz,
  valid_until timestamptz,
  expires_at timestamptz,
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id,version),
  CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from)
);

CREATE TABLE knowledge_packs (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  name text NOT NULL,
  purpose text NOT NULL,
  version integer NOT NULL CHECK (version > 0),
  status text NOT NULL CHECK (status IN ('draft','published','retired')),
  object_refs jsonb NOT NULL CHECK (jsonb_typeof(object_refs) = 'array'),
  query_policy jsonb NOT NULL CHECK (jsonb_typeof(query_policy) = 'object'),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_by text NOT NULL DEFAULT '',
  published_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  PRIMARY KEY (tenant_id,id),
  UNIQUE (tenant_id,project_id,id,version)
);

CREATE TABLE knowledge_decisions (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  object_id text NOT NULL,
  previous_version integer NOT NULL CHECK (previous_version > 0),
  result_version integer NOT NULL CHECK (result_version = previous_version + 1),
  subject_digest text NOT NULL CHECK (subject_digest ~ '^sha256:[0-9a-f]{64}$'),
  decision text NOT NULL CHECK (decision IN ('approve','reject')),
  reason text NOT NULL,
  actor_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,object_id,previous_version) REFERENCES knowledge_objects(tenant_id,id,version),
  FOREIGN KEY (tenant_id,object_id,result_version) REFERENCES knowledge_objects(tenant_id,id,version)
);

CREATE TABLE knowledge_snapshots (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  id text NOT NULL,
  project_id uuid NOT NULL REFERENCES brand_projects(id) ON DELETE CASCADE,
  pack_id text NOT NULL,
  pack_version integer NOT NULL CHECK (pack_version > 0),
  pack_digest text NOT NULL CHECK (pack_digest ~ '^sha256:[0-9a-f]{64}$'),
  objects jsonb NOT NULL CHECK (jsonb_typeof(objects) = 'array'),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,id),
  FOREIGN KEY (tenant_id,pack_id) REFERENCES knowledge_packs(tenant_id,id)
);

CREATE INDEX knowledge_objects_project_idx ON knowledge_objects(tenant_id,project_id,layer,status,id,version);
CREATE INDEX knowledge_objects_id_idx ON knowledge_objects(tenant_id,id,version DESC);
CREATE INDEX knowledge_decisions_object_idx ON knowledge_decisions(tenant_id,object_id,created_at DESC);
CREATE INDEX knowledge_packs_project_idx ON knowledge_packs(tenant_id,project_id,status,created_at DESC);
CREATE INDEX knowledge_snapshots_pack_idx ON knowledge_snapshots(tenant_id,project_id,pack_id,created_at DESC);

ALTER TABLE knowledge_objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_objects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON knowledge_objects
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE knowledge_packs ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_packs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON knowledge_packs
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE knowledge_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_decisions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON knowledge_decisions
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE knowledge_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE knowledge_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON knowledge_snapshots
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TRIGGER knowledge_snapshots_immutable
  BEFORE UPDATE OR DELETE ON knowledge_snapshots
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_immutable_mutation();

CREATE TRIGGER knowledge_objects_immutable
  BEFORE UPDATE OR DELETE ON knowledge_objects
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_immutable_mutation();

CREATE TRIGGER knowledge_decisions_immutable
  BEFORE UPDATE OR DELETE ON knowledge_decisions
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_immutable_mutation();

GRANT SELECT,INSERT,UPDATE,DELETE ON knowledge_objects,knowledge_decisions,knowledge_packs,knowledge_snapshots TO contentcloud_runtime;
REVOKE UPDATE,DELETE ON knowledge_objects,knowledge_decisions,knowledge_snapshots FROM contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS knowledge_snapshots;
DROP TABLE IF EXISTS knowledge_packs;
DROP TABLE IF EXISTS knowledge_decisions;
DROP TABLE IF EXISTS knowledge_objects;
