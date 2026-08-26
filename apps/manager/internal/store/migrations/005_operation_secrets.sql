CREATE TABLE IF NOT EXISTS operation_secrets (
  operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  envelope_version INTEGER NOT NULL,
  nonce BLOB NOT NULL,
  ciphertext BLOB NOT NULL,
  PRIMARY KEY(operation_id, kind)
);
