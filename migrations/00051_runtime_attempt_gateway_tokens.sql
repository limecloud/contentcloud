ALTER TABLE runtime_attempts
  ADD COLUMN gateway_token_hash text NOT NULL DEFAULT '',
  ADD COLUMN gateway_expires_at timestamptz;

ALTER TABLE runtime_attempts
  ADD CONSTRAINT runtime_attempts_gateway_token_hash_format
  CHECK (gateway_token_hash = '' OR gateway_token_hash ~ '^[0-9a-f]{64}$');

CREATE UNIQUE INDEX runtime_attempts_gateway_token_hash_unique
  ON runtime_attempts(gateway_token_hash)
  WHERE gateway_token_hash <> '';

CREATE OR REPLACE FUNCTION contentcloud_lookup_runtime_gateway_token(p_hash text)
RETURNS TABLE(tenant_id uuid, attempt_id text)
LANGUAGE sql SECURITY DEFINER SET search_path = public AS $$
  SELECT a.tenant_id,a.id
  FROM runtime_attempts a
  WHERE a.gateway_token_hash = p_hash
    AND a.gateway_expires_at > now()
    AND a.state IN ('prepared','running')
  LIMIT 1
$$;

REVOKE ALL ON FUNCTION contentcloud_lookup_runtime_gateway_token(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION contentcloud_lookup_runtime_gateway_token(text) TO contentcloud_runtime;
