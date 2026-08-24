-- +goose Up

-- Platform-scoped customer surface declarations. This table owns only the
-- customer UI contract and lifecycle assignment; execution and artifact
-- semantics remain in SOP/Runtime/Review/Delivery tables.
CREATE TABLE workbench_plugin_versions (
  id text NOT NULL,
  version text NOT NULL CHECK (version ~ '^[0-9]+[.][0-9]+[.][0-9]+$'),
  manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
  digest text NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
  status text NOT NULL CHECK (status IN ('draft','published','retired','revoked')),
  lifecycle_reason text NOT NULL DEFAULT '',
  template_aliases text[] NOT NULL DEFAULT '{}'::text[],
  tenant_ids text[] NOT NULL DEFAULT '{}'::text[],
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id,version),
  CHECK (status <> 'revoked' OR btrim(lifecycle_reason) <> '')
);

CREATE INDEX workbench_plugin_versions_status_idx ON workbench_plugin_versions(status,id,version);

GRANT SELECT,INSERT ON workbench_plugin_versions TO contentcloud_runtime;
GRANT UPDATE (status,lifecycle_reason,tenant_ids,updated_at) ON workbench_plugin_versions TO contentcloud_runtime;

-- +goose Down

DROP TABLE IF EXISTS workbench_plugin_versions;
