CREATE TABLE IF NOT EXISTS project_secret_versions (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL,
  kind TEXT NOT NULL,
  envelope_version INTEGER NOT NULL,
  nonce BLOB NOT NULL,
  ciphertext BLOB NOT NULL,
  PRIMARY KEY(project_id, revision, kind)
);
