-- Reverte 0006_enrollment. enrollment_period aponta para enrollment, que cai por
-- último. Os índices caem junto com as tabelas.
DROP TABLE IF EXISTS enrollment_period;
DROP TABLE IF EXISTS enrollment;
