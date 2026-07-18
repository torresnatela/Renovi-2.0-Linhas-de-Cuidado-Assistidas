-- Consultas do log da jornada (tabela journey_event — migration 0007).
--
-- É append-only: só INSERT e SELECT. O role renovi_app (0008) NÃO tem UPDATE/DELETE
-- aqui, então nem existe query de alteração — não teria como rodar.

-- name: InsertJourneyEvent :one
-- O id é UUID v7 gerado na app: ordena junto com occurred_at e é a chave de
-- desempate do keyset de paginação.
INSERT INTO journey_event (
    id, enrollment_id, patient_id, event_type, actor, ref_table, ref_id, payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListJourneyEventsByPatient :many
-- A tela de jornada, paginada por KEYSET (não OFFSET): a página seguinte pede tudo
-- ANTERIOR ao último (occurred_at, id) que já viu. O índice
-- ix_journey_event_patient (patient_id, occurred_at DESC, id DESC) serve isto
-- diretamente. Para a primeira página, o model passa o maior instante/uuid
-- possível como cursor.
--
-- A comparação é a forma EXPANDIDA do row-value `(occurred_at, id) < (cursor)` —
-- idêntica em semântica, mas o sqlc infere o tipo de cada parâmetro corretamente
-- (o row-value constructor faz o sqlc tipar o id como timestamptz).
SELECT * FROM journey_event
WHERE patient_id = sqlc.arg('patient_id')
  AND (
    occurred_at < sqlc.arg('before_occurred_at')
    OR (occurred_at = sqlc.arg('before_occurred_at') AND id < sqlc.arg('before_id'))
  )
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('limit');

-- name: ListRecentJourneyEventsByEnrollment :many
-- Os eventos mais recentes de UMA matrícula (ex.: para montar o resumo da linha).
SELECT * FROM journey_event
WHERE enrollment_id = $1
ORDER BY occurred_at DESC, id DESC
LIMIT $2;
