-- +goose Up

REVOKE UPDATE, DELETE ON creative_execution_bundles FROM contentcloud_runtime;

CREATE TRIGGER creative_execution_bundles_immutable
  BEFORE UPDATE OR DELETE ON creative_execution_bundles
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();

-- +goose Down

DROP TRIGGER IF EXISTS creative_execution_bundles_immutable ON creative_execution_bundles;
GRANT UPDATE, DELETE ON creative_execution_bundles TO contentcloud_runtime;
