-- Queries da conclusão do onboarding via convite (ver migrations 0016 e 0017).
-- Consomem o token cunhado na ingestão e fecham (ou recusam) o vínculo.

-- name: FindTokenByHash :one
-- O convite pelo hash do token (SEM filtro de vivo). Devolve usado/revogado/expira
-- para o model classificar e dar a mensagem certa (inexistente/expirado/usado/revogado).
SELECT * FROM onboarding_token WHERE token_hash = $1;

-- name: GetEmployeeLinkByID :one
-- A pessoa por id (status, cpf_hmac e o snapshot invite_* para o pré-preenchimento).
SELECT * FROM gestao_employee_link WHERE id = $1;

-- name: ListLiveContractCompaniesByEmployeeLink :many
-- Nomes distintos das empresas dos contratos VIVOS (ativo/afastado) da pessoa —
-- o que o passo "Você faz parte da empresa X?" exibe.
SELECT DISTINCT c.display_name
FROM gestao_contract gc
JOIN gestao_company_link c ON c.id = gc.gestao_company_link_id
WHERE gc.gestao_employee_link_id = $1 AND gc.status IN ('ativo', 'afastado')
ORDER BY c.display_name;

-- name: CloseEmployeeLink :execrows
-- Fecha o vínculo pelo convite: seta patient_id, status='vinculado', link_method e
-- linked_at JUNTOS (o CHECK vinculado_completo exige os três). Guard status='pendente'
-- torna idempotente e serve de trava contra dupla conclusão.
UPDATE gestao_employee_link
SET patient_id  = $2,
    status      = 'vinculado',
    link_method = 'convite',
    linked_at   = $3,
    updated_at  = $3
WHERE id = $1 AND status = 'pendente';

-- name: SetLiveContractsAcceptedByEmployeeLink :execrows
-- Marca o consentimento (accepted_at) nos contratos vivos da pessoa. Só os que ainda
-- não têm accepted_at (idempotente) e só ativo/afastado (desligado não é aceitável).
UPDATE gestao_contract
SET accepted_at = $2, updated_at = $2
WHERE gestao_employee_link_id = $1 AND accepted_at IS NULL AND status IN ('ativo', 'afastado');

-- name: MarkTokenUsed :execrows
-- Consome o convite. Guard used_at/revoked_at IS NULL: um token já usado/revogado
-- não é reusado (execrows = 0 sinaliza a corrida ao model).
UPDATE onboarding_token
SET used_at = $2
WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL;

-- name: MarkEmployeeLinkDeclined :execrows
-- A pessoa abriu o convite e disse que NÃO faz parte da empresa: registra a recusa no
-- próprio vínculo (visível na tabela). Guard status='pendente' (idempotente).
UPDATE gestao_employee_link
SET status = 'recusado', updated_at = $2
WHERE id = $1 AND status = 'pendente';
