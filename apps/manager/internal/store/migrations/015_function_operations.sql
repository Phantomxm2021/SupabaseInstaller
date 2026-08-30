CREATE TABLE IF NOT EXISTS function_operations (
  operation_id TEXT PRIMARY KEY REFERENCES operations(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  function_name TEXT NOT NULL,
  archive_sha256 TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_function_operations_project ON function_operations(project_id, created_at);
