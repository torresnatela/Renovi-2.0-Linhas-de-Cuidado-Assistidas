-- Queries de autenticação e vínculo com a DAV (ver migration 0002_auth).

-- name: FindAccountByCPF :one
-- O CPF é a chave de identidade: mora em patient_identity (LGPD, ver CLAUDE.md).
SELECT a.*
FROM patient_account a
JOIN patient_identity i ON i.account_id = a.id
WHERE i.cpf = $1;

-- name: InsertAccount :one
-- Nasce PENDING_DAV: a conta só ativa quando a DAV confirmar o vínculo.
INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
VALUES ($1, $2, $3, $4, $5, $6, 'PENDING_DAV')
RETURNING *;

-- name: InsertIdentity :exec
INSERT INTO patient_identity (account_id, cpf) VALUES ($1, $2);

-- name: RefreshPendingAccount :one
-- Reaproveita a linha de uma tentativa que morreu antes da DAV confirmar.
-- O filtro por status é a trava: uma conta ACTIVE nunca pode ser sobrescrita por
-- quem apenas conhece o CPF.
UPDATE patient_account
SET full_name = $2, email = $3, phone = $4, birth_date = $5,
    password_hash = $6, updated_at = now()
WHERE id = $1 AND status = 'PENDING_DAV'
RETURNING *;

-- name: UpsertAddress :exec
INSERT INTO patient_address (account_id, zip_code, street, number, complement,
                             neighborhood, city, state, country)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (account_id) DO UPDATE
SET zip_code = EXCLUDED.zip_code, street = EXCLUDED.street, number = EXCLUDED.number,
    complement = EXCLUDED.complement, neighborhood = EXCLUDED.neighborhood,
    city = EXCLUDED.city, state = EXCLUDED.state, country = EXCLUDED.country,
    updated_at = now();

-- name: LinkAccountToDav :exec
-- Ativa a conta. O CHECK active_exige_vinculo_dav (migration 0002) recusa esta
-- linha se dav_person_id vier nulo — a regra está no banco, não só aqui.
UPDATE patient_account
SET status = 'ACTIVE', dav_person_id = $2, dav_link_origin = $3,
    dav_linked_at = now(), updated_at = now()
WHERE id = $1;

-- name: InsertDavLinkAudit :exec
-- Trilha de todo vínculo. É o que permitirá revisar retroativamente quem anexou
-- prontuário de quem, quando o fator de posse (WhatsApp/e-mail) existir.
INSERT INTO dav_link_audit (id, account_id, dav_person_id, origin, request_ip)
VALUES ($1, $2, $3, $4, $5);

-- name: GetAccountByID :one
SELECT * FROM patient_account WHERE id = $1;

-- name: InsertSession :exec
-- token_hash é o SHA-256 do token opaco. O token em si nunca toca o banco.
INSERT INTO session (id, account_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: FindLiveSession :one
-- "Viva" = não revogada, não expirada E com a conta ainda ACTIVE. O join com o
-- status é o que faz bloquear uma conta derrubar as sessões dela na hora — o
-- ganho concreto de sessão opaca sobre JWT (ADR-010).
SELECT s.id, a.id AS account_id, a.full_name, a.email
FROM session s
JOIN patient_account a ON a.id = s.account_id
WHERE s.token_hash = $1
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND a.status = 'ACTIVE';

-- name: RevokeSessionByTokenHash :exec
UPDATE session SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;
