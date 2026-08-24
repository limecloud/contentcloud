-- +goose Up

ALTER TABLE runtime_maintenance_heartbeats
  DROP CONSTRAINT runtime_maintenance_heartbeats_kind_check,
  ADD CONSTRAINT runtime_maintenance_heartbeats_kind_check
    CHECK (kind IN ('runtime_reaper','runtime_delivery','runtime_cleanup'));

-- +goose Down

ALTER TABLE runtime_maintenance_heartbeats
  DROP CONSTRAINT runtime_maintenance_heartbeats_kind_check,
  ADD CONSTRAINT runtime_maintenance_heartbeats_kind_check
    CHECK (kind IN ('runtime_reaper','runtime_delivery'));
