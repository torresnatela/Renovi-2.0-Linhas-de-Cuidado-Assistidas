-- Reverte 0004_release_guard.
ALTER TABLE appointment DROP CONSTRAINT IF EXISTS liberado_exige_terminal;
