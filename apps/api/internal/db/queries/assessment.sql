-- name: FindActivityEnrollmentDetail :one
-- Como FindActivityEnrollment, mas traz a vigência da matrícula — o motor puro
-- precisa dela para a regra VIGENCIA ao avaliar a cadência do instrumento.
SELECT e.id AS enrollment_id, e.status, e.valid_from, e.valid_until,
       i.id AS care_line_item_id
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

-- name: ListItemRules :many
SELECT rule_type, params FROM care_line_rule WHERE care_line_item_id = $1 ORDER BY id;

-- name: ListAssessmentTimes :many
-- Os instantes das aplicações passadas do paciente para um item — o histórico
-- imutável que o motor lê para MIN_INTERVAL (cadência derivada sob demanda).
SELECT respondido_em FROM wellbeing_assessment
WHERE patient_id = $1 AND care_line_item_id = $2
ORDER BY respondido_em DESC;

-- name: InsertWellbeingAssessment :one
INSERT INTO wellbeing_assessment (
    id, patient_id, enrollment_id, care_line_item_id, consent_id, instrument_id,
    raw_score, index_score, subscores, faixa, flag_encaminhar, respondido_em
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: InsertAssessmentItemResponse :exec
INSERT INTO assessment_item_response (id, assessment_id, item_ordem, valor)
VALUES ($1, $2, $3, $4);
