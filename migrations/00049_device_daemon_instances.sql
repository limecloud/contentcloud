ALTER TABLE devices
  ADD COLUMN machine_id text,
  ADD COLUMN credential_version integer NOT NULL DEFAULT 1 CHECK (credential_version > 0),
  ADD COLUMN credential_rotated_at timestamptz;

UPDATE devices
SET machine_id = 'legacy:' || id::text,
    credential_rotated_at = last_seen_at
WHERE machine_id IS NULL OR credential_rotated_at IS NULL;

ALTER TABLE devices
  ALTER COLUMN machine_id SET NOT NULL,
  ALTER COLUMN credential_rotated_at SET NOT NULL;

CREATE UNIQUE INDEX devices_active_machine_idx
  ON devices(tenant_id,machine_id)
  WHERE revoked_at IS NULL;

ALTER TABLE devices
  ADD CONSTRAINT devices_tenant_id_id_unique UNIQUE (tenant_id,id);

CREATE TABLE daemon_instances (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  device_id uuid NOT NULL,
  connection_epoch bigint NOT NULL DEFAULT 0 CHECK (connection_epoch >= 0),
  report_sequence bigint NOT NULL DEFAULT 0 CHECK (report_sequence >= 0),
  pid integer,
  daemon_version text NOT NULL,
  state text NOT NULL CHECK (state IN ('starting','connected','degraded','stopped')),
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities) = 'object'),
  active_attempts jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(active_attempts) = 'array'),
  started_at timestamptz NOT NULL,
  last_seen_at timestamptz NOT NULL,
  stopped_at timestamptz,
  FOREIGN KEY (tenant_id,device_id) REFERENCES devices(tenant_id,id)
);

CREATE INDEX daemon_instances_device_seen_idx
  ON daemon_instances(tenant_id,device_id,last_seen_at DESC);

ALTER TABLE daemon_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE daemon_instances FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON daemon_instances
  USING (tenant_id = current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id',true)::uuid);

-- +goose Down
DROP TABLE IF EXISTS daemon_instances;
DROP INDEX IF EXISTS devices_active_machine_idx;
ALTER TABLE devices
  DROP CONSTRAINT IF EXISTS devices_tenant_id_id_unique,
  DROP COLUMN IF EXISTS credential_rotated_at,
  DROP COLUMN IF EXISTS credential_version,
  DROP COLUMN IF EXISTS machine_id;
