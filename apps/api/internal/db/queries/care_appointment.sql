-- Consultas da jornada realizada (tabela care_appointment — migration 0007).
--
-- É a projeção clínica do agendamento (appointment, 0003), amarrada à matrícula.
-- As leituras por paciente passam SEMPRE por um JOIN em enrollment que confere o
-- dono — a consulta carrega dado clínico e não pode vazar por id.

-- name: InsertCareAppointment :one
-- O status inicial entra como parâmetro (tipicamente 'agendada'); a idempotência é
-- garantida pelo índice único ux_care_appt_idem quando idempotency_key não é nula.
INSERT INTO care_appointment (
    id, enrollment_id, care_line_item_id, item_ref,
    booking_id, scheduled_at, status, idempotency_key
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetCareAppointmentByIdemKey :one
-- O outro lado da idempotência: se a key já existe na matrícula, o handler devolve
-- a consulta que já foi criada em vez de tentar (e falhar no índice único).
SELECT * FROM care_appointment
WHERE enrollment_id = $1 AND idempotency_key = $2;

-- name: GetCareAppointmentForPatient :one
-- Por (id, dono), via a matrícula. Nunca só por id.
SELECT ca.* FROM care_appointment ca
JOIN enrollment e ON e.id = ca.enrollment_id
WHERE ca.id = $1 AND e.patient_id = $2;

-- name: GetCareAppointment :one
-- Leitura por id SEM dono: EXCLUSIVA do endpoint interno de teste (force-status),
-- que é gated por ambiente (RENOVI_TEST_ENDPOINTS), não por sessão — lá não há
-- paciente para conferir. Toda rota de paciente usa a versão ForPatient acima.
SELECT * FROM care_appointment WHERE id = $1;

-- name: ListCareAppointmentsByEnrollment :many
SELECT * FROM care_appointment
WHERE enrollment_id = $1
ORDER BY scheduled_at;

-- name: ListCareAppointmentsByPatient :many
-- "Minhas consultas da jornada", com filtro OPCIONAL por status: quando
-- sqlc.narg('status') é nulo, o predicado some e a lista vem inteira.
SELECT ca.* FROM care_appointment ca
JOIN enrollment e ON e.id = ca.enrollment_id
WHERE e.patient_id = sqlc.arg('patient_id')
  AND (sqlc.narg('status')::text IS NULL OR ca.status = sqlc.narg('status'))
ORDER BY ca.scheduled_at DESC;

-- name: CancelCareAppointment :execrows
-- Cancela (grava a data no mesmo passo — o CHECK cancelada_exige_data recusaria
-- 'cancelada' sem cancelled_at). O guard só deixa cancelar o que ainda está por
-- acontecer; consulta já realizada/faltada/cancelada casa 0 linhas.
UPDATE care_appointment
SET status = 'cancelada', cancelled_at = $2, updated_at = $2
WHERE id = $1 AND status IN ('agendada', 'confirmada');

-- name: ForceCareAppointmentStatus :execrows
-- A correção manual (admin) do status clínico quando o fluxo automático não deu
-- conta. O $2 é validado no model. Não alcança consulta já em estado terminal
-- (realizada/falta/cancelada): o guard casa 0 linhas.
UPDATE care_appointment
SET status = $2, updated_at = $3
WHERE id = $1 AND status IN ('agendada', 'confirmada', 'em_andamento');
