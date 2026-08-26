-- Migration 008 was already applied by some installations before it gained
-- its backfill. Keep this as a new, additive migration so those installations
-- receive the revision-1 snapshot as well.
INSERT OR IGNORE INTO project_secret_snapshot_markers(project_id, revision, present)
SELECT p.id, p.config_revision,
       CASE WHEN EXISTS (SELECT 1 FROM project_secrets s WHERE s.project_id = p.id) THEN 1 ELSE 0 END
FROM projects p;

INSERT OR IGNORE INTO project_secret_versions(project_id, revision, kind, envelope_version, nonce, ciphertext)
SELECT s.project_id, p.config_revision, s.kind, s.envelope_version, s.nonce, s.ciphertext
FROM project_secrets s JOIN projects p ON p.id = s.project_id;
