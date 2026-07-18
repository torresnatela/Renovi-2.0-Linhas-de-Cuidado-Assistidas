-- Consultas da matrícula (tabelas enrollment e enrollment_period — migration 0006).
--
-- Cada UPDATE é estreito e traz o status no WHERE, como no agendamento: um
-- "UpdateEnrollment" genérico deixaria o próximo model gravar qualquer transição.
-- Todas são :execrows quando o model precisa checar que exatamente 1 linha mudou.

-- name: InsertEnrollment :one
INSERT INTO enrollment (
    id, patient_id, care_line_id, care_line_code, status,
    valid_from, valid_until, gestao_contract_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: InsertEnrollmentPeriod :one
-- source usa o default 'admin' (único valor aceito no piloto).
INSERT INTO enrollment_period (id, enrollment_id, starts_at, ends_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetEnrollment :one
SELECT * FROM enrollment WHERE id = $1;

-- name: GetEnrollmentForUpdate :one
-- Trava a linha para uma transição sob concorrência (renovar/encerrar/expirar). O
-- FOR UPDATE serializa dois processos que mexam na mesma matrícula.
SELECT * FROM enrollment WHERE id = $1 FOR UPDATE;

-- name: GetEnrollmentForPatient :one
-- Sempre por (id, dono). Nunca só por id: evita que a próxima rota devolva a
-- matrícula de terceiro por esquecer o WHERE do paciente.
SELECT * FROM enrollment WHERE id = $1 AND patient_id = $2;

-- name: ListEnrollmentsByPatient :many
SELECT * FROM enrollment
WHERE patient_id = $1
ORDER BY created_at DESC;

-- name: ListEnrollmentPeriods :many
SELECT * FROM enrollment_period
WHERE enrollment_id = $1
ORDER BY starts_at;

-- name: RenewEnrollment :execrows
-- Renovação: estende a vigência e reativa. Aceita renovar a partir de viva ou
-- expirada (o caso comum do piloto é renovar o que venceu); concluída/encerrada
-- são terminais e não voltam por aqui.
UPDATE enrollment
SET valid_until = $2, status = 'ativa', updated_at = $3
WHERE id = $1 AND status IN ('ativa', 'pausada', 'expirada');

-- name: EndEnrollment :execrows
-- Encerramento por decisão (concluida | encerrada). O $2 é validado no model — o
-- CHECK do banco aceita ambos, mas só estes dois fazem sentido aqui.
UPDATE enrollment
SET status = $2, updated_at = $3
WHERE id = $1 AND status IN ('ativa', 'pausada', 'expirada');

-- name: ExpireEnrollment :execrows
-- A varredura de vencidas: só expira o que está 'ativa' E já passou da vigência. O
-- `valid_until < $2` no WHERE torna a operação segura contra clock skew do chamador
-- — o banco só expira o que de fato venceu.
UPDATE enrollment
SET status = 'expirada', updated_at = $2
WHERE id = $1 AND status = 'ativa' AND valid_until < $2;
