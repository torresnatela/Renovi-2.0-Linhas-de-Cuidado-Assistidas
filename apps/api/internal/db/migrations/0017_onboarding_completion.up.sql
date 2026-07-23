-- 0017_onboarding_completion — a conclusão do onboarding via convite.
--
-- O colaborador abre o link do convite, finaliza o cadastro (vira patient_account)
-- e confirma o vínculo com a empresa: o gestao_employee_link fecha ('vinculado',
-- link_method='convite') e o gestao_contract ganha accepted_at. Se recusar a empresa
-- (dupla confirmação no front), registramos a recusa no próprio vínculo.
--
-- Esta migration só ABRE espaço no vocabulário: um novo status de pessoa ('recusado')
-- e dois novos tipos de evento de auditoria. As colunas do fechamento (patient_id,
-- link_method, linked_at em gestao_employee_link; accepted_at em gestao_contract;
-- used_at em onboarding_token) já existem desde a 0016.

-- 'recusado': a pessoa abriu o convite e disse que NÃO faz parte da empresa. Distinto
-- de 'cancelado' (ação da Gestão). Como 'recusado' <> 'cancelado', continua ocupando
-- a trava ux_gestao_employee_ativo — não recebe novo convite automático (o que fazer
-- depois com um vínculo recusado é fatia futura).
ALTER TABLE gestao_employee_link DROP CONSTRAINT gestao_employee_link_status_check;
ALTER TABLE gestao_employee_link ADD CONSTRAINT gestao_employee_link_status_check
    CHECK (status IN ('pendente', 'vinculado', 'cancelado', 'recusado'));

-- Novos eventos da trilha append-only: a conclusão e a recusa do onboarding.
ALTER TABLE gestao_ingestion_event DROP CONSTRAINT gestao_ingestion_event_event_type_check;
ALTER TABLE gestao_ingestion_event ADD CONSTRAINT gestao_ingestion_event_event_type_check
    CHECK (event_type IN (
        'contrato_recebido', 'convite_emitido', 'convite_reenviado', 'cpf_match_pendente',
        'onboarding_concluido', 'onboarding_recusado'
    ));
