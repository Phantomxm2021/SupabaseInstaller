CREATE TABLE IF NOT EXISTS project_configuration_leases (
  project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  owner TEXT NOT NULL,
  acquired_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
