-- Functions are materialized by the Provisioner-managed release tree. The
-- former configuration field was never authoritative; remove it from stored
-- JSON while retaining historical snapshots and function files/volumes.
UPDATE project_configuration
SET config_json = json_remove(config_json, '$.functions.directory')
WHERE json_valid(config_json);

UPDATE project_configs
SET config_json = json_remove(config_json, '$.functions.directory')
WHERE json_valid(config_json);
