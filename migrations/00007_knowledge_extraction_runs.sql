-- +goose Up

ALTER TABLE context_snapshots ALTER COLUMN brief_version_id DROP NOT NULL;
ALTER TABLE task_runs ALTER COLUMN brief_version_id DROP NOT NULL;

ALTER TABLE task_runs ADD COLUMN capability_id text NOT NULL DEFAULT 'contentcloud.script.generate';
ALTER TABLE task_runs ADD COLUMN capability_version text NOT NULL DEFAULT '1.1.0';
ALTER TABLE task_runs ADD COLUMN input_schema text NOT NULL DEFAULT 'task-contract/1.0';
ALTER TABLE task_runs ADD COLUMN output_schema text NOT NULL DEFAULT 'script-package/1.1';
ALTER TABLE task_runs ADD COLUMN output_count integer NOT NULL DEFAULT 1 CHECK (output_count BETWEEN 1 AND 20);
ALTER TABLE task_runs ADD COLUMN delivery_profiles jsonb NOT NULL DEFAULT '["review_projection/1.0","text"]';

ALTER TABLE knowledge_items ADD COLUMN origin_run_id uuid REFERENCES task_runs(id);
CREATE INDEX knowledge_origin_run_idx ON knowledge_items(tenant_id,origin_run_id) WHERE origin_run_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS knowledge_origin_run_idx;
ALTER TABLE knowledge_items DROP COLUMN IF EXISTS origin_run_id;
ALTER TABLE task_runs DROP COLUMN IF EXISTS delivery_profiles, DROP COLUMN IF EXISTS output_count, DROP COLUMN IF EXISTS output_schema, DROP COLUMN IF EXISTS input_schema, DROP COLUMN IF EXISTS capability_version, DROP COLUMN IF EXISTS capability_id;
ALTER TABLE task_runs ALTER COLUMN brief_version_id SET NOT NULL;
ALTER TABLE context_snapshots ALTER COLUMN brief_version_id SET NOT NULL;
