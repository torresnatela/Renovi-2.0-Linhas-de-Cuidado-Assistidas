-- Reverte 0002_auth. Ordem inversa das dependências (as FKs apontam todas para
-- patient_account, que cai por último).
DROP TABLE IF EXISTS dav_link_audit;
DROP TABLE IF EXISTS session;
DROP TABLE IF EXISTS patient_address;
DROP TABLE IF EXISTS patient_identity;
DROP TABLE IF EXISTS patient_account;
