-- Reverte 0015: remove a linha universal e as matrículas automáticas nela.
--
-- ATENÇÃO — reversão FORWARD-ONLY após o primeiro uso: mood_checkin e
-- wellbeing_assessment referenciam enrollment/care_line_item com ON DELETE RESTRICT.
-- Se algum colaborador já registrou humor na linha universal, o DELETE da matrícula
-- (e da linha) FALHA de propósito — apagar aqui destruiria dado sensível de saúde.
-- Neste caso, trate a linha como permanente. Este down só completa num banco onde a
-- linha universal ainda não recebeu nenhum check-in/assessment.

DELETE FROM journey_event
WHERE enrollment_id IN (SELECT id FROM enrollment WHERE care_line_code = 'saude-mental-aberta');

DELETE FROM enrollment_period
WHERE enrollment_id IN (SELECT id FROM enrollment WHERE care_line_code = 'saude-mental-aberta');

DELETE FROM enrollment WHERE care_line_code = 'saude-mental-aberta';

-- care_line_item e care_line_rule caem por ON DELETE CASCADE (0005).
DELETE FROM care_line WHERE code = 'saude-mental-aberta';
