ALTER TABLE projects ADD COLUMN last_good_revision INTEGER NOT NULL DEFAULT 1;

UPDATE projects SET last_good_revision = config_revision;

INSERT INTO project_configs(project_id, section, revision, config_json, created_at)
SELECT p.id, 'aggregate', p.config_revision,
       json_object(
         'revision', p.config_revision,
         'general', json_object('domain', p.domain, 'siteUrl', p.site_url, 'supabaseVersion', p.supabase_version),
         'services', json_object(
           'database', json('true'),
           'gateway', json(CASE WHEN json_extract(p.services_json, '$.auth') OR json_extract(p.services_json, '$.rest') OR json_extract(p.services_json, '$.studio') OR json_extract(p.services_json, '$.realtime') OR json_extract(p.services_json, '$.storage') THEN 'true' ELSE CASE WHEN json_extract(p.services_json, '$.gateway') THEN 'true' ELSE 'false' END END),
           'auth', json(CASE WHEN json_extract(p.services_json, '$.auth') THEN 'true' ELSE 'false' END),
           'rest', json(CASE WHEN json_extract(p.services_json, '$.rest') THEN 'true' ELSE 'false' END),
           'studio', json(CASE WHEN json_extract(p.services_json, '$.studio') THEN 'true' ELSE 'false' END),
           'postgresMeta', json(CASE WHEN json_extract(p.services_json, '$.studio') OR json_extract(p.services_json, '$.postgresMeta') THEN 'true' ELSE 'false' END),
           'realtime', json(CASE WHEN json_extract(p.services_json, '$.realtime') THEN 'true' ELSE 'false' END),
           'storage', json(CASE WHEN json_extract(p.services_json, '$.imgproxy') OR json_extract(p.services_json, '$.storage') THEN 'true' ELSE 'false' END),
           'imgproxy', json(CASE WHEN json_extract(p.services_json, '$.imgproxy') THEN 'true' ELSE 'false' END),
           'functions', json(CASE WHEN json_extract(p.services_json, '$.functions') THEN 'true' ELSE 'false' END),
           'supavisor', json(CASE WHEN json_extract(p.services_json, '$.supavisor') THEN 'true' ELSE 'false' END),
           'logs', json(CASE WHEN json_extract(p.services_json, '$.logs') THEN 'true' ELSE 'false' END),
           'vector', json(CASE WHEN json_extract(p.services_json, '$.logs') THEN 'true' ELSE 'false' END),
           'directDb', json(CASE WHEN json_extract(p.services_json, '$.directDb') THEN 'true' ELSE 'false' END)),
         'auth', json_object('enabled', json('true'), 'email', json_object('enabled', json('true'), 'allowSignup', json('true')), 'smtp', json_object('port', 587)),
         'storage', json_object('backend', 'local'),
         'realtime', json_object('maxConnections', 100, 'databasePoolSize', 5, 'logLevel', 'info'),
         'functions', json_object('defaultJwtVerification', json('true'), 'directory', './functions'),
         'database', json_object('version', '15', 'maxConnections', 100),
         'pooler', json_object('transactionPort', 6543, 'sessionPort', 6544, 'poolSize', 20, 'maxClientConnections', 100),
         'network', json_object('gateway', 'envoy', 'httpsMode', 'external')),
       p.created_at
FROM projects AS p
WHERE NOT EXISTS (
  SELECT 1 FROM project_configs AS c
  WHERE c.project_id = p.id AND c.section = 'aggregate' AND c.revision = p.config_revision
);
