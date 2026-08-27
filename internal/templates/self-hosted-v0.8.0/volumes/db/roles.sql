-- NOTE: change to your own passwords for production environments
\set pgpass `echo "$POSTGRES_PASSWORD"`

-- Optional services are disabled in some presets, so their roles may not
-- exist. Update only roles present in this database.
DO $$
DECLARE
  role_name text;
BEGIN
  FOR role_name IN
    SELECT rolname
    FROM pg_roles
    WHERE rolname IN (
      'authenticator',
      'pgbouncer',
      'supabase_auth_admin',
      'supabase_functions_admin',
      'supabase_storage_admin'
    )
  LOOP
    EXECUTE format('ALTER ROLE %I WITH PASSWORD %L', role_name, :'pgpass');
  END LOOP;
END
$$;
