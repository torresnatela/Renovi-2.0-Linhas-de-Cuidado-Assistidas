-- Reverte 0017: volta os CHECKs aos conjuntos da 0016. Best-effort — se já houver
-- linhas 'recusado' ou eventos onboarding_*, o re-ADD do CHECK mais restrito falha
-- (esperado; o down só faz sentido antes de a feature gerar dados).
ALTER TABLE gestao_ingestion_event DROP CONSTRAINT gestao_ingestion_event_event_type_check;
ALTER TABLE gestao_ingestion_event ADD CONSTRAINT gestao_ingestion_event_event_type_check
    CHECK (event_type IN (
        'contrato_recebido', 'convite_emitido', 'convite_reenviado', 'cpf_match_pendente'
    ));

ALTER TABLE gestao_employee_link DROP CONSTRAINT gestao_employee_link_status_check;
ALTER TABLE gestao_employee_link ADD CONSTRAINT gestao_employee_link_status_check
    CHECK (status IN ('pendente', 'vinculado', 'cancelado'));
