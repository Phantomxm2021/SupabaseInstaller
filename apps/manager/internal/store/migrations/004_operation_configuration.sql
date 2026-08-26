CREATE TABLE IF NOT EXISTS operation_configurations (
  operation_id TEXT PRIMARY KEY REFERENCES operations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL,
  config_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);
