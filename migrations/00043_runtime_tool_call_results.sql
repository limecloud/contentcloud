-- +goose Up

ALTER TABLE runtime_tool_calls
  ADD COLUMN safe_result jsonb NOT NULL DEFAULT '{}'::jsonb
  CHECK (jsonb_typeof(safe_result) = 'object');

-- +goose Down

ALTER TABLE runtime_tool_calls DROP COLUMN IF EXISTS safe_result;
