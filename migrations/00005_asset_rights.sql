-- +goose Up

CREATE TABLE assets (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  name text NOT NULL,
  asset_type text NOT NULL CHECK (asset_type IN ('product_image','brand_mark','packaging','person','location','audio','other')),
  source_revision_id uuid NOT NULL REFERENCES source_revisions(id),
  usage_mode text NOT NULL CHECK (usage_mode IN ('analysis_only','generation_reference','owned')),
  status text NOT NULL CHECK (status IN ('needs_review','approved','rejected','review_required','expired')),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rights_records (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  asset_id uuid NOT NULL REFERENCES assets(id),
  rights_holder text NOT NULL,
  rights_type text NOT NULL CHECK (rights_type IN ('owned','licensed_generation','licensed_edit','public_domain')),
  territories jsonb NOT NULL,
  channels jsonb NOT NULL,
  valid_from timestamptz,
  valid_until timestamptz,
  proof_source_revision_id uuid NOT NULL REFERENCES source_revisions(id),
  restrictions jsonb NOT NULL DEFAULT '[]',
  status text NOT NULL CHECK (status IN ('needs_review','approved','rejected','review_required','expired')),
  reviewed_by uuid REFERENCES users(id),
  reviewed_at timestamptz,
  row_version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until > valid_from)
);

CREATE INDEX assets_project_idx ON assets(tenant_id,project_id,status,created_at DESC);
CREATE INDEX rights_asset_idx ON rights_records(tenant_id,asset_id,status,created_at DESC);

ALTER TABLE assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE rights_records ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_assets ON assets USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_rights_records ON rights_records USING (tenant_id = current_setting('app.tenant_id',true)::uuid) WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS rights_records, assets CASCADE;
