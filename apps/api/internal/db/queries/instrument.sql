-- name: GetActiveInstrument :one
SELECT * FROM instrument WHERE codigo = $1 AND ativo;

-- name: ListInstrumentDimensions :many
SELECT * FROM instrument_dimension WHERE instrument_id = $1 ORDER BY dimensao;

-- name: ListInstrumentCutoffs :many
SELECT * FROM instrument_cutoff WHERE instrument_id = $1 ORDER BY dimensao, faixa;

-- name: ListEmotionLabels :many
SELECT * FROM emotion_label WHERE ativo ORDER BY quadrante, rotulo;

-- name: ListContextTags :many
SELECT * FROM context_tag WHERE ativo ORDER BY rotulo;
