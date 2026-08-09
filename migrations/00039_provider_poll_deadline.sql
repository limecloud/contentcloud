-- A missing poll deadline means an unknown submission that requires manual
-- reconciliation, not an immediate re-submit loop.
CREATE OR REPLACE FUNCTION contentcloud_pending_media_generation_jobs(p_limit integer)
RETURNS TABLE(tenant_id uuid, job_id text)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT j.tenant_id,j.id
  FROM media_generation_jobs j
  LEFT JOIN LATERAL (
    SELECT a.next_poll_at
    FROM provider_attempts a
    WHERE a.tenant_id=j.tenant_id AND a.generation_job_id=j.id
    ORDER BY a.attempt_number DESC
    LIMIT 1
  ) latest ON true
  WHERE j.state IN ('queued','retry_wait')
     OR (j.state = 'awaiting_external_result' AND latest.next_poll_at IS NOT NULL AND latest.next_poll_at <= now())
  ORDER BY j.updated_at,j.created_at
  LIMIT greatest(1,least(p_limit,100))
$$;
