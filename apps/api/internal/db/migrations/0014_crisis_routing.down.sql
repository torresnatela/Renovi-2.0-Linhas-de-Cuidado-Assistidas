ALTER TABLE journey_event DROP CONSTRAINT journey_event_event_type_check;
ALTER TABLE journey_event ADD CONSTRAINT journey_event_event_type_check CHECK (event_type IN (
    'matricula_criada', 'matricula_renovada', 'matricula_expirada', 'matricula_encerrada',
    'consulta_agendada', 'consulta_cancelada', 'consulta_status_forcado',
    'checkin_humor_registrado', 'assessment_respondido'
));
