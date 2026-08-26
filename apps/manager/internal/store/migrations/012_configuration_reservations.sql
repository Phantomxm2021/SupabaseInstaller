-- Pending desired revisions reserve globally unique domains and host ports
-- before runtime publication. Reservations are released only on a confirmed
-- rollback or MarkConfigurationGood.
CREATE TABLE IF NOT EXISTS configuration_reservations (
  resource_kind TEXT NOT NULL,
  resource_key TEXT NOT NULL,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(resource_kind, resource_key),
  UNIQUE(project_id, operation_id, resource_kind, resource_key)
);
