-- +goose Up

ALTER TABLE sop_definitions
  ADD COLUMN IF NOT EXISTS template_key text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS built_in boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS source_ref text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS sop_definitions_builtin_template_idx
  ON sop_definitions(tenant_id,template_key)
  WHERE built_in = true AND template_key <> '';

-- +goose Down

DROP INDEX IF EXISTS sop_definitions_builtin_template_idx;
ALTER TABLE sop_definitions
  DROP COLUMN IF EXISTS source_ref,
  DROP COLUMN IF EXISTS built_in,
  DROP COLUMN IF EXISTS template_key;
