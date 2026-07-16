-- Reverte 0003_scheduling.
--
-- Os índices caem junto com a tabela (Postgres derruba os dependentes), então
-- basta o DROP.
DROP TABLE IF EXISTS appointment;
