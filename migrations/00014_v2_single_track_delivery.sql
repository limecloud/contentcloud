-- +goose Up
ALTER TABLE approval_decisions
  ADD COLUMN decision_stage text NOT NULL DEFAULT 'legacy'
  CHECK (decision_stage IN ('internal','client','legacy'));

ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_status_check
  CHECK (status IN ('preparing','submitted','in_review','internally_approved','client_review','changes_requested','approved','rejected','withdrawn','superseded'));

ALTER TABLE approved_snapshots
  ADD COLUMN origin text NOT NULL DEFAULT 'current' CHECK (origin IN ('current','v1_import')),
  ADD COLUMN external_ref text;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_submission_revision_id_key;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_workspace_id_fkey;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_submission_id_fkey;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_submission_revision_id_fkey;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_decision_id_fkey;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_created_by_fkey;
ALTER TABLE approved_snapshots
  ALTER COLUMN workspace_id DROP NOT NULL,
  ALTER COLUMN submission_id DROP NOT NULL,
  ALTER COLUMN submission_revision_id DROP NOT NULL,
  ALTER COLUMN decision_id DROP NOT NULL,
  ALTER COLUMN created_by TYPE text USING created_by::text;
ALTER TABLE approved_snapshots
  ADD CONSTRAINT approved_snapshots_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspace_bindings(id),
  ADD CONSTRAINT approved_snapshots_submission_id_fkey FOREIGN KEY (submission_id) REFERENCES submissions(id),
  ADD CONSTRAINT approved_snapshots_submission_revision_id_fkey FOREIGN KEY (submission_revision_id) REFERENCES submission_revisions(id),
  ADD CONSTRAINT approved_snapshots_decision_id_fkey FOREIGN KEY (decision_id) REFERENCES approval_decisions(id),
  ADD CONSTRAINT approved_snapshots_origin_shape_check CHECK (
    (origin='current' AND workspace_id IS NOT NULL AND submission_id IS NOT NULL AND submission_revision_id IS NOT NULL AND decision_id IS NOT NULL AND external_ref IS NULL)
    OR (origin='v1_import' AND workspace_id IS NULL AND submission_id IS NULL AND submission_revision_id IS NULL AND external_ref IS NOT NULL)
  );
CREATE UNIQUE INDEX approved_snapshots_current_revision_unique
  ON approved_snapshots(submission_revision_id) WHERE origin='current';
CREATE UNIQUE INDEX approved_snapshots_v1_external_ref_unique
  ON approved_snapshots(tenant_id,external_ref) WHERE origin='v1_import';

ALTER TABLE artifacts
  ADD COLUMN approved_snapshot_id uuid REFERENCES approved_snapshots(id),
  ALTER COLUMN script_version_id DROP NOT NULL;
ALTER TABLE artifacts ADD CONSTRAINT artifacts_version_source_check
  CHECK ((script_version_id IS NOT NULL) <> (approved_snapshot_id IS NOT NULL));
CREATE INDEX artifacts_approved_snapshot_idx ON artifacts(tenant_id,approved_snapshot_id,created_at DESC);

ALTER TABLE performance_observations
  ADD COLUMN approved_snapshot_id uuid REFERENCES approved_snapshots(id),
  ALTER COLUMN script_version_id DROP NOT NULL;
ALTER TABLE performance_observations ADD CONSTRAINT performance_observations_version_source_check
  CHECK ((script_version_id IS NOT NULL) <> (approved_snapshot_id IS NOT NULL));
CREATE INDEX performance_observations_snapshot_idx ON performance_observations(tenant_id,approved_snapshot_id,published_at DESC);

ALTER TABLE rating_decisions DROP CONSTRAINT IF EXISTS rating_decisions_subject_type_check;
ALTER TABLE rating_decisions ADD CONSTRAINT rating_decisions_subject_type_check
  CHECK (subject_type IN ('approved_snapshot','script_version','content_framework','shot_pattern'));

CREATE TABLE delivery_packages (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  project_id uuid NOT NULL REFERENCES brand_projects(id),
  script_id text NOT NULL CHECK (char_length(btrim(script_id)) > 0),
  status text NOT NULL CHECK (status = 'ready'),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL
);

CREATE TABLE delivery_package_snapshots (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  delivery_package_id uuid NOT NULL REFERENCES delivery_packages(id),
  approved_snapshot_id uuid NOT NULL REFERENCES approved_snapshots(id),
  position integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (delivery_package_id,approved_snapshot_id)
);

CREATE TABLE delivery_package_artifacts (
  tenant_id uuid NOT NULL REFERENCES tenants(id),
  delivery_package_id uuid NOT NULL REFERENCES delivery_packages(id),
  artifact_id uuid NOT NULL REFERENCES artifacts(id),
  position integer NOT NULL CHECK (position >= 0),
  PRIMARY KEY (delivery_package_id,artifact_id)
);

CREATE INDEX delivery_packages_project_idx ON delivery_packages(tenant_id,project_id,created_at DESC);
CREATE INDEX delivery_package_snapshots_snapshot_idx ON delivery_package_snapshots(tenant_id,approved_snapshot_id);

ALTER TABLE delivery_packages ENABLE ROW LEVEL SECURITY;
ALTER TABLE delivery_package_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE delivery_package_artifacts ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_delivery_packages ON delivery_packages
  USING (tenant_id=current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id=current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_delivery_package_snapshots ON delivery_package_snapshots
  USING (tenant_id=current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id=current_setting('app.tenant_id',true)::uuid);
CREATE POLICY tenant_delivery_package_artifacts ON delivery_package_artifacts
  USING (tenant_id=current_setting('app.tenant_id',true)::uuid)
  WITH CHECK (tenant_id=current_setting('app.tenant_id',true)::uuid);

CREATE TRIGGER delivery_packages_immutable BEFORE UPDATE OR DELETE ON delivery_packages
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();
CREATE TRIGGER delivery_package_snapshots_immutable BEFORE UPDATE OR DELETE ON delivery_package_snapshots
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();
CREATE TRIGGER delivery_package_artifacts_immutable BEFORE UPDATE OR DELETE ON delivery_package_artifacts
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();

INSERT INTO approved_snapshots(
  id,tenant_id,project_id,workspace_id,submission_id,submission_revision_id,
  submission_type,schema_version,content_hash,subject_hash,canonical_content,
  eligible_ids,artifacts,decision_id,created_by,created_at,origin,external_ref
)
SELECT
  (substr(md5('v1:'||sv.id::text),1,8)||'-'||substr(md5('v1:'||sv.id::text),9,4)||'-'||substr(md5('v1:'||sv.id::text),13,4)||'-'||substr(md5('v1:'||sv.id::text),17,4)||'-'||substr(md5('v1:'||sv.id::text),21,12))::uuid,
  sv.tenant_id,sv.project_id,NULL,NULL,NULL,'script','1.x',sv.content_hash,sv.content_hash,
  jsonb_build_object('schema_version','1.x','submission_type','script','objects',jsonb_build_array(sv.canonical_json),'source_disclosures','[]'::jsonb,'artifacts','[]'::jsonb,'local_run_summary','{}'::jsonb),
  jsonb_build_array(sv.id::text),'[]'::jsonb,
  (SELECT ad.id FROM approval_decisions ad WHERE ad.tenant_id=sv.tenant_id AND ad.subject_id=sv.id AND ad.decision='approve' ORDER BY ad.created_at DESC LIMIT 1),
  COALESCE((SELECT ad.actor_id FROM approval_decisions ad WHERE ad.tenant_id=sv.tenant_id AND ad.subject_id=sv.id AND ad.decision='approve' ORDER BY ad.created_at DESC LIMIT 1),'v1_import'),
  sv.created_at,'v1_import',sv.id::text
FROM script_versions sv
WHERE sv.status IN ('approved','superseded')
  AND sv.content_hash ~ '^[0-9a-f]{64}$'
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TRIGGER IF EXISTS delivery_package_artifacts_immutable ON delivery_package_artifacts;
DROP TRIGGER IF EXISTS delivery_package_snapshots_immutable ON delivery_package_snapshots;
DROP TRIGGER IF EXISTS delivery_packages_immutable ON delivery_packages;
DROP TABLE IF EXISTS delivery_package_artifacts;
DROP TABLE IF EXISTS delivery_package_snapshots;
DROP TABLE IF EXISTS delivery_packages;

ALTER TABLE rating_decisions DROP CONSTRAINT IF EXISTS rating_decisions_subject_type_check;
ALTER TABLE rating_decisions ADD CONSTRAINT rating_decisions_subject_type_check
  CHECK (subject_type IN ('script_version','content_framework','shot_pattern'));

DROP TRIGGER IF EXISTS performance_observations_immutable ON performance_observations;
DELETE FROM performance_observations WHERE script_version_id IS NULL;
ALTER TABLE performance_observations DROP CONSTRAINT IF EXISTS performance_observations_version_source_check;
DROP INDEX IF EXISTS performance_observations_snapshot_idx;
ALTER TABLE performance_observations DROP COLUMN IF EXISTS approved_snapshot_id;
ALTER TABLE performance_observations ALTER COLUMN script_version_id SET NOT NULL;
CREATE TRIGGER performance_observations_immutable BEFORE UPDATE OR DELETE ON performance_observations
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_performance_mutation();

DELETE FROM artifacts WHERE script_version_id IS NULL;
ALTER TABLE artifacts DROP CONSTRAINT IF EXISTS artifacts_version_source_check;
DROP INDEX IF EXISTS artifacts_approved_snapshot_idx;
ALTER TABLE artifacts DROP COLUMN IF EXISTS approved_snapshot_id;
ALTER TABLE artifacts ALTER COLUMN script_version_id SET NOT NULL;

DROP TRIGGER IF EXISTS approved_snapshots_immutable ON approved_snapshots;
DELETE FROM approved_snapshots WHERE origin='v1_import' OR created_by !~ '^[0-9a-fA-F-]{36}$';
DROP INDEX IF EXISTS approved_snapshots_v1_external_ref_unique;
DROP INDEX IF EXISTS approved_snapshots_current_revision_unique;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_origin_shape_check;
ALTER TABLE approved_snapshots DROP CONSTRAINT IF EXISTS approved_snapshots_created_by_fkey;
ALTER TABLE approved_snapshots
  ALTER COLUMN workspace_id SET NOT NULL,
  ALTER COLUMN submission_id SET NOT NULL,
  ALTER COLUMN submission_revision_id SET NOT NULL,
  ALTER COLUMN decision_id SET NOT NULL,
  ALTER COLUMN created_by TYPE uuid USING created_by::uuid;
ALTER TABLE approved_snapshots
  ADD CONSTRAINT approved_snapshots_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id),
  ADD CONSTRAINT approved_snapshots_submission_revision_id_key UNIQUE (submission_revision_id);
ALTER TABLE approved_snapshots DROP COLUMN IF EXISTS external_ref;
ALTER TABLE approved_snapshots DROP COLUMN IF EXISTS origin;
CREATE TRIGGER approved_snapshots_immutable BEFORE UPDATE OR DELETE ON approved_snapshots
  FOR EACH ROW EXECUTE FUNCTION contentcloud_reject_v2_immutable_mutation();

UPDATE submissions SET status='submitted' WHERE status IN ('internally_approved','client_review');
ALTER TABLE submissions DROP CONSTRAINT IF EXISTS submissions_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_status_check
  CHECK (status IN ('preparing','submitted','in_review','changes_requested','approved','rejected','withdrawn','superseded'));
ALTER TABLE approval_decisions DROP COLUMN IF EXISTS decision_stage;
