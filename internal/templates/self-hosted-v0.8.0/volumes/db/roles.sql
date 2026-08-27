-- NOTE: change to your own passwords for production environments
\set pgpass `echo "$POSTGRES_PASSWORD"`

-- Optional services are disabled in some presets, so their roles may not
-- exist. Generate statements only for roles present in this database.
SELECT format('ALTER ROLE %I WITH PASSWORD %L;', rolname, :'pgpass')
FROM pg_roles
WHERE rolname IN (
  'authenticator',
  'pgbouncer',
  'supabase_auth_admin',
  'supabase_functions_admin',
  'supabase_storage_admin'
)
\gexec
