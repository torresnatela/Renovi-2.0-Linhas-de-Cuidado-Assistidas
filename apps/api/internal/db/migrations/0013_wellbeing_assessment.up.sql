-- 0013_wellbeing_assessment — execução dos anéis periódicos (WHO-5 semanal e,
-- adiante, PHQ-4 gatilhado). É o fato de execução dessas atividades: o motor de
-- linhas de cuidado o lê (via respondido_em) para avaliar MIN_INTERVAL/cadência.
-- Convenção: PK UUID v7 (app), TIMESTAMPTZ, enum via TEXT + CHECK.

-- Cada aplicação emite um fato na jornada. Estende o event_type do 0012.
ALTER TABLE journey_event DROP CONSTRAINT journey_event_event_type_check;
ALTER TABLE journey_event ADD CONSTRAINT journey_event_event_type_check CHECK (event_type IN (
    'matricula_criada', 'matricula_renovada', 'matricula_expirada', 'matricula_encerrada',
    'consulta_agendada', 'consulta_cancelada', 'consulta_status_forcado',
    'checkin_humor_registrado', 'assessment_respondido'
));

CREATE TABLE wellbeing_assessment (
    id                UUID PRIMARY KEY,
    patient_id        UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    enrollment_id     UUID NOT NULL REFERENCES enrollment (id) ON DELETE RESTRICT,
    care_line_item_id UUID NOT NULL REFERENCES care_line_item (id) ON DELETE RESTRICT,
    consent_id        UUID NOT NULL REFERENCES consent (id) ON DELETE RESTRICT,
    instrument_id     UUID NOT NULL REFERENCES instrument (id) ON DELETE RESTRICT,
    raw_score         NUMERIC NOT NULL,
    index_score       NUMERIC,          -- WHO-5: 0–100; PHQ-4: NULL
    subscores         JSONB,            -- PHQ-4: {"phq2": x, "gad2": y}
    faixa             TEXT NOT NULL CHECK (btrim(faixa) <> ''),  -- derivada dos cortes
    flag_encaminhar   BOOLEAN NOT NULL DEFAULT false,
    respondido_em     TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_wellbeing_patient_item ON wellbeing_assessment (patient_id, care_line_item_id, respondido_em DESC);

CREATE TABLE assessment_item_response (
    id            UUID PRIMARY KEY,
    assessment_id UUID NOT NULL REFERENCES wellbeing_assessment (id) ON DELETE CASCADE,
    item_ordem    INT NOT NULL CHECK (item_ordem >= 1),
    valor         INT NOT NULL,
    CONSTRAINT ux_assessment_item UNIQUE (assessment_id, item_ordem)
);
