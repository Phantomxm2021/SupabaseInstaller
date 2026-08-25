ALTER TABLE projects ADD COLUMN last_good_revision INTEGER NOT NULL DEFAULT 1;

INSERT INTO project_configs(project_id, section, revision, config_json, created_at)
SELECT p.id, 'aggregate', 1,
       '{"revision":1,"general":{"domain":' || json_quote(p.domain) ||
       ',"siteUrl":' || json_quote(p.site_url) ||
       ',"supabaseVersion":' || json_quote(p.supabase_version) ||
       '},"services":' || p.services_json || '}',
       p.created_at
FROM projects AS p
WHERE NOT EXISTS (
  SELECT 1 FROM project_configs AS c
  WHERE c.project_id = p.id AND c.section = 'aggregate'
);
