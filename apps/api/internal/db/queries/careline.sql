-- Consultas do catálogo de linhas de cuidado (tabelas care_line, care_line_item,
-- care_line_rule — migration 0005).
--
-- O catálogo é versionado e imutável por versão: criar é sempre um rascunho novo,
-- publicar sela a versão. As queries refletem isso — não há UPDATE de conteúdo,
-- só o PublishCareLine que promove um draft.

-- name: CreateCareLine :one
-- Cria um rascunho (status default 'draft', published_at NULL). A versão é
-- calculada antes, pela NextCareLineVersion, e entra como parâmetro.
INSERT INTO care_line (id, code, version, name, description)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: NextCareLineVersion :one
-- A próxima versão de um code: MAX+1, ou 1 se ainda não existe nenhuma. É o número
-- que o CreateCareLine grava — calculá-lo aqui evita uma corrida no model.
SELECT COALESCE(MAX(version), 0) + 1 AS next_version
FROM care_line
WHERE code = $1;

-- name: GetCareLine :one
SELECT * FROM care_line WHERE id = $1;

-- name: ListCareLinesByCode :many
-- Todas as versões de um code, da mais nova para a mais antiga.
SELECT * FROM care_line
WHERE code = $1
ORDER BY version DESC;

-- name: ListCareLines :many
-- O catálogo inteiro, agrupado por code e com a versão mais nova primeiro.
SELECT * FROM care_line
ORDER BY code, version DESC;

-- name: GetLatestPublishedCareLine :one
-- A versão publicada mais recente de um code: é a que rege uma nova matrícula.
SELECT * FROM care_line
WHERE code = $1 AND status = 'published'
ORDER BY version DESC
LIMIT 1;

-- name: InsertCareLineItem :one
INSERT INTO care_line_item (
    id, care_line_id, ref, kind, specialty_code, label, recurrence, sort_order
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: InsertCareLineRule :one
INSERT INTO care_line_rule (id, care_line_item_id, rule_type, params)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListItemsByCareLine :many
SELECT * FROM care_line_item
WHERE care_line_id = $1
ORDER BY sort_order, ref;

-- name: ListRulesByCareLine :many
-- As regras de todos os itens de uma linha, com o ref do item junto (o motor puro
-- resolve PREREQUISITE por ref, então trazê-lo aqui evita um segundo lookup).
SELECT sqlc.embed(r), i.ref AS item_ref
FROM care_line_rule r
JOIN care_line_item i ON i.id = r.care_line_item_id
WHERE i.care_line_id = $1
ORDER BY i.sort_order, i.ref, r.created_at;

-- name: PublishCareLine :execrows
-- Promove o rascunho a publicado. O guard `status = 'draft'` no WHERE torna a
-- operação idempotente-segura: republicar (ou publicar o que não é draft) casa 0
-- linhas, e o model trata como falha em vez de sobrescrever published_at.
UPDATE care_line
SET status = 'published', published_at = $2, updated_at = $2
WHERE id = $1 AND status = 'draft';
