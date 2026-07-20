-- name: FindActivityEnrollment :one
-- A matrícula ATIVA e VIGENTE do paciente numa linha publicada que contém o item
-- de atividade pedido. É a elegibilidade do check-in, derivada sob demanda dos
-- fatos imutáveis (matrícula + item do template). Sem linha => não elegível.
SELECT e.id AS enrollment_id, i.id AS care_line_item_id
FROM enrollment e
JOIN care_line_item i ON i.care_line_id = e.care_line_id
WHERE e.patient_id = $1
  AND e.status = 'ativa'
  AND e.valid_from <= $2
  AND e.valid_until >= $2
  AND i.ref = $3
  AND i.kind = 'ATIVIDADE'
ORDER BY e.valid_from DESC
LIMIT 1;

-- name: UpsertMoodCheckin :one
-- Uma resposta por dia local (dia_ref): nova resposta no mesmo dia ATUALIZA a do
-- dia. O id da linha original é preservado no update.
INSERT INTO mood_checkin (
    id, patient_id, enrollment_id, care_line_item_id, consent_id, instrument_id,
    valencia, energia, quadrante, emotion_label, context_tags, dia_ref, respondido_em
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (patient_id, dia_ref) DO UPDATE SET
    enrollment_id     = EXCLUDED.enrollment_id,
    care_line_item_id = EXCLUDED.care_line_item_id,
    consent_id        = EXCLUDED.consent_id,
    instrument_id     = EXCLUDED.instrument_id,
    valencia          = EXCLUDED.valencia,
    energia           = EXCLUDED.energia,
    quadrante         = EXCLUDED.quadrante,
    emotion_label     = EXCLUDED.emotion_label,
    context_tags      = EXCLUDED.context_tags,
    respondido_em     = EXCLUDED.respondido_em,
    updated_at        = now()
RETURNING *;

-- name: GetMoodCheckinByDay :one
SELECT * FROM mood_checkin WHERE patient_id = $1 AND dia_ref = $2;

-- name: ListMoodCheckins :many
SELECT * FROM mood_checkin WHERE patient_id = $1 ORDER BY respondido_em DESC LIMIT $2;

-- name: ListRecentCheckinQuadrants :many
-- Quadrantes dos check-ins recentes (mais novo primeiro) — o gatilho conta a
-- sequência de dias em risco a partir daqui.
SELECT dia_ref, quadrante FROM mood_checkin
WHERE patient_id = $1 ORDER BY dia_ref DESC LIMIT $2;

-- name: LatestAssessmentByInstrument :one
-- A aplicação mais recente de um instrumento (por código) para o paciente — o
-- gatilho usa a faixa/flag e o instante para decidir o estado.
SELECT wa.faixa, wa.flag_encaminhar, wa.respondido_em
FROM wellbeing_assessment wa
JOIN instrument i ON i.id = wa.instrument_id
WHERE wa.patient_id = $1 AND i.codigo = $2
ORDER BY wa.respondido_em DESC
LIMIT 1;
