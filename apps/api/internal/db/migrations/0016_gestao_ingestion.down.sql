-- Reverte 0016_gestao_ingestion. Ordem: filhos (FK) antes dos pais. Os índices e o
-- REVOKE caem junto com as tabelas.
DROP TABLE IF EXISTS gestao_ingestion_event;
DROP TABLE IF EXISTS onboarding_token;
DROP TABLE IF EXISTS gestao_contract;
DROP TABLE IF EXISTS gestao_employee_link;
DROP TABLE IF EXISTS gestao_company_link;
ALTER TABLE patient_identity DROP COLUMN IF EXISTS cpf_hmac;
