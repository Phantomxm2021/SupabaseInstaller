ALTER TABLE operations ADD COLUMN compensation_phase TEXT NOT NULL DEFAULT '';
ALTER TABLE operations ADD COLUMN compensation_idempotency_key TEXT NOT NULL DEFAULT '';
