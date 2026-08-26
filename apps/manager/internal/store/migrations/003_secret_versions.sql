CREATE TABLE IF NOT EXISTS project_secret_versions (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL,
  kind TEXT NOT NULL,
  envelope_version INTEGER NOT NULL,
  nonce BLOB NOT NULL,
  ciphertext BLOB NOT NULL,
  PRIMARY KEY(project_id, revision, kind)
);

INSERT OR IGNORE INTO project_secret_versions(project_id, revision, kind, envelope_version, nonce, ciphertext)
SELECT project_id, (SELECT config_revision FROM projects p WHERE p.id = project_secrets.project_id), kind, envelope_version, nonce, ciphertext
FROM project_secrets;
