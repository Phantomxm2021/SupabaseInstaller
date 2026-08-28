-- Canonical configuration storage.  The aggregate JSON in this table is the
-- only value that new Manager code reads and applies.  project_configs remains
-- available during the upgrade window solely as historical migration input.
CREATE TABLE IF NOT EXISTS project_configuration (
  project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  config_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_configuration_keys (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (project_id, kind),
  UNIQUE (kind, value)
);

INSERT INTO project_configuration(project_id, config_json, updated_at)
SELECT p.id, c.config_json, COALESCE(c.created_at, p.updated_at)
FROM projects p
JOIN project_configs c
  ON c.project_id = p.id
 AND c.section = 'aggregate'
 AND c.revision = (
   SELECT MAX(c2.revision)
   FROM project_configs c2
   WHERE c2.project_id = p.id AND c2.section = 'aggregate'
 )
 AND json_valid(c.config_json)
WHERE NOT EXISTS (
  SELECT 1 FROM project_configuration pc WHERE pc.project_id = p.id
);

INSERT OR IGNORE INTO project_configuration_keys(project_id, kind, value)
SELECT project_id, 'domain', json_extract(config_json, '$.general.domain')
FROM project_configuration
WHERE NULLIF(json_extract(config_json, '$.general.domain'), '') IS NOT NULL;
