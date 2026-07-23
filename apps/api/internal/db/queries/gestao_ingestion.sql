-- Queries da ingestão de contratos da Gestão (ver migration 0016_gestao_ingestion).
-- Todos os upserts são idempotentes pela chave do Gestão, para o push poder repetir.

-- name: UpsertGestaoCompany :one
-- Idempotente por gestao_company_id: o mesmo id do Gestão devolve a mesma empresa.
INSERT INTO gestao_company_link (id, gestao_company_id, display_name)
VALUES ($1, $2, $3)
ON CONFLICT (gestao_company_id) DO UPDATE
SET display_name = EXCLUDED.display_name, updated_at = now()
RETURNING *;

-- name: FindIdentityByCPFHmac :one
-- Detecção (sem vincular): existe um paciente para este cpf_hmac? Funciona porque o
-- cadastro grava patient_identity.cpf_hmac com o MESMO pepper que a Gestão usou.
SELECT account_id FROM patient_identity WHERE cpf_hmac = $1;

-- name: UpsertGestaoEmployeeLink :one
-- Upsert da PESSOA pelo cpf_hmac, arbitrado pelo índice parcial ux_gestao_employee_ativo
-- (WHERE status <> 'cancelado'). O DO UPDATE só atualiza o snapshot do convite —
-- NUNCA mexe em status: um 'vinculado' jamais é rebaixado por um novo push.
INSERT INTO gestao_employee_link (id, cpf_hmac, invite_name, invite_email, invite_phone, status)
VALUES ($1, $2, $3, $4, $5, 'pendente')
ON CONFLICT (cpf_hmac) WHERE status <> 'cancelado' DO UPDATE
SET invite_name  = EXCLUDED.invite_name,
    invite_email = EXCLUDED.invite_email,
    invite_phone = EXCLUDED.invite_phone,
    updated_at   = now()
RETURNING *;

-- name: GetLiveEmployeeLinkByCPFHmacForUpdate :one
-- A pessoa VIVA (não cancelada) pelo cpf_hmac, travada para o reenvio de convite
-- (FOR UPDATE serializa reenvios concorrentes, como o Expire da jornada).
SELECT * FROM gestao_employee_link
WHERE cpf_hmac = $1 AND status <> 'cancelado'
FOR UPDATE;

-- name: UpsertGestaoContract :one
-- Idempotente por gestao_contract_id. A transição ativo -> afastado -> desligado é
-- um simples UPDATE de status; ended_at vem calculado pelo model (não-nulo só em
-- desligado, senão o CHECK desligado_exige_data recusa). accepted_at e started_at
-- não são tocados no update (consentimento e início não mudam por re-push).
INSERT INTO gestao_contract (
    id, gestao_contract_id, gestao_employee_id,
    gestao_employee_link_id, gestao_company_link_id,
    status, started_at, ended_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (gestao_contract_id) DO UPDATE
SET gestao_employee_id      = EXCLUDED.gestao_employee_id,
    gestao_employee_link_id = EXCLUDED.gestao_employee_link_id,
    gestao_company_link_id  = EXCLUDED.gestao_company_link_id,
    status                  = EXCLUDED.status,
    ended_at                = EXCLUDED.ended_at,
    updated_at              = now()
RETURNING *;

-- name: FindLiveTokenByLink :one
-- O convite vivo desta pessoa (usado/revogado não conta). Guardamos só o hash, então
-- serve para DETECTAR que já há convite — não para reconstruir a URL.
SELECT * FROM onboarding_token
WHERE gestao_employee_link_id = $1 AND used_at IS NULL AND revoked_at IS NULL;

-- name: InsertOnboardingToken :one
-- Cunha um convite. Guarda o SHA-256 do token (o token cru nunca toca o banco). Uma
-- corrida no ux_token_vivo devolve 23505, que o model trata relendo o vivo.
INSERT INTO onboarding_token (id, gestao_employee_link_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: RevokeLiveTokensByLink :exec
-- Revoga o convite vivo antes de cunhar outro (reenvio). Idempotente.
UPDATE onboarding_token SET revoked_at = now()
WHERE gestao_employee_link_id = $1 AND used_at IS NULL AND revoked_at IS NULL;

-- name: InsertIngestionEvent :exec
-- Trilha append-only da ingestão. NUNCA recebe CPF em claro — só o cpf_hmac.
INSERT INTO gestao_ingestion_event
    (id, event_type, gestao_contract_id, gestao_employee_link_id, cpf_hmac, payload)
VALUES ($1, $2, $3, $4, $5, $6);
