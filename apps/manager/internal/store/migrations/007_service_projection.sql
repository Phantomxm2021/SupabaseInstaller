INSERT OR IGNORE INTO project_services(project_id, service, enabled, status)
SELECT p.id, j.key, CASE WHEN json_extract(c.config_json, '$.services.' || j.key) THEN 1 ELSE 0 END, 'UNKNOWN'
FROM projects p
JOIN project_configs c ON c.project_id = p.id AND c.section = 'aggregate' AND c.revision = p.config_revision
JOIN json_each('{"database":1,"gateway":1,"auth":1,"rest":1,"studio":1,"postgresMeta":1,"realtime":1,"storage":1,"imgproxy":1,"functions":1,"supavisor":1,"logs":1,"vector":1,"directDb":1}') j;
