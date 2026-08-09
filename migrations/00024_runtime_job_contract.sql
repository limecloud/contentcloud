-- +goose Up

ALTER TABLE runtime_job_runs ADD COLUMN binding_digest text;
ALTER TABLE runtime_job_runs ADD COLUMN input_digest text;
ALTER TABLE runtime_job_runs ADD COLUMN runtime_policy_id text;
ALTER TABLE runtime_job_runs ADD COLUMN contract_major integer;
ALTER TABLE runtime_job_runs ADD COLUMN contract_minor integer;
ALTER TABLE runtime_job_runs ADD COLUMN root_job_run_id text;

-- Pre-contract rows cannot be reconstructed after the fact. Keep them
-- readable and explicitly marked instead of manufacturing admission facts.
UPDATE runtime_job_runs
SET binding_digest = 'sha256:829742971be6c4928681d6d52c835bb36a2cbb6dcbfdd0b15b9690c6dc59630c',
    input_digest = 'sha256:829742971be6c4928681d6d52c835bb36a2cbb6dcbfdd0b15b9690c6dc59630c',
    runtime_policy_id = 'runtime.legacy-unfrozen/1',
    contract_major = 1,
    contract_minor = 0,
    root_job_run_id = id;

ALTER TABLE runtime_job_runs ALTER COLUMN binding_digest SET NOT NULL;
ALTER TABLE runtime_job_runs ALTER COLUMN input_digest SET NOT NULL;
ALTER TABLE runtime_job_runs ALTER COLUMN runtime_policy_id SET NOT NULL;
ALTER TABLE runtime_job_runs ALTER COLUMN contract_major SET NOT NULL;
ALTER TABLE runtime_job_runs ALTER COLUMN contract_minor SET NOT NULL;
ALTER TABLE runtime_job_runs ALTER COLUMN root_job_run_id SET NOT NULL;
ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_binding_digest_ck CHECK (binding_digest ~ '^sha256:[0-9a-f]{64}$');
ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_input_digest_ck CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$');
ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_policy_ck CHECK (btrim(runtime_policy_id) <> '');
ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_contract_ck CHECK (contract_major > 0 AND contract_minor >= 0);

ALTER TABLE runtime_job_runs ADD CONSTRAINT runtime_job_runs_root_job_fk
  FOREIGN KEY (tenant_id,root_job_run_id) REFERENCES runtime_job_runs(tenant_id,id);

CREATE INDEX runtime_job_runs_root_idx ON runtime_job_runs(tenant_id,root_job_run_id,created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS runtime_job_runs_root_idx;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_root_job_fk;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_contract_ck;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_policy_ck;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_input_digest_ck;
ALTER TABLE runtime_job_runs DROP CONSTRAINT IF EXISTS runtime_job_runs_binding_digest_ck;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS root_job_run_id;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS contract_minor;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS contract_major;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS runtime_policy_id;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS input_digest;
ALTER TABLE runtime_job_runs DROP COLUMN IF EXISTS binding_digest;
