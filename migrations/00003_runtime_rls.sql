-- +goose Up

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'contentcloud_runtime') THEN
    CREATE ROLE contentcloud_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS;
  END IF;
  EXECUTE format('GRANT contentcloud_runtime TO %I', current_user);
END $$;

GRANT USAGE ON SCHEMA public TO contentcloud_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO contentcloud_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO contentcloud_runtime;
REVOKE ALL ON users, tenants, memberships, sessions, user_device_flows, contentcloud_schema_migrations FROM contentcloud_runtime;

ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO contentcloud_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO contentcloud_runtime;

-- The runtime role is intentionally not granted access to global identity and
-- token lookup paths outside their existing SECURITY DEFINER functions.

-- +goose Down

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM contentcloud_runtime;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM contentcloud_runtime;
REVOKE USAGE ON SCHEMA public FROM contentcloud_runtime;
