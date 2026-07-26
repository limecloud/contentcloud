-- +goose Up
CREATE OR REPLACE FUNCTION contentcloud_backfill_v1_approved_snapshots(p_tenant_id uuid DEFAULT NULL)
RETURNS jsonb
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
  inserted_count integer;
  skipped_invalid_hash_count integer;
BEGIN
  SELECT count(*) INTO skipped_invalid_hash_count
  FROM script_versions sv
  WHERE sv.status IN ('approved','superseded')
    AND (p_tenant_id IS NULL OR sv.tenant_id=p_tenant_id)
    AND sv.content_hash !~ '^[0-9a-f]{64}$';

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
    AND (p_tenant_id IS NULL OR sv.tenant_id=p_tenant_id)
    AND sv.content_hash ~ '^[0-9a-f]{64}$'
  ON CONFLICT DO NOTHING;

  GET DIAGNOSTICS inserted_count = ROW_COUNT;
  RETURN jsonb_build_object(
    'inserted', inserted_count,
    'skipped_invalid_hash', skipped_invalid_hash_count
  );
END;
$$;

SELECT contentcloud_backfill_v1_approved_snapshots();

-- +goose Down
DROP FUNCTION IF EXISTS contentcloud_backfill_v1_approved_snapshots(uuid);
