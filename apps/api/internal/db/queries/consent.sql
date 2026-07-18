-- name: GetActiveConsent :one
SELECT * FROM consent
WHERE patient_id = $1 AND finalidade = $2 AND status = 'ativo';

-- name: InsertConsent :one
INSERT INTO consent (id, patient_id, gestao_contract_id, finalidade, versao_termo)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: RevokeActiveConsent :execrows
UPDATE consent
SET status = 'revogado', revogado_em = $3
WHERE patient_id = $1 AND finalidade = $2 AND status = 'ativo';
