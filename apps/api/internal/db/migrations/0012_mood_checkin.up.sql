-- 0012_mood_checkin — a execução do anel diário (grade valência×energia).
--
-- É o fato de execução da atividade checkin-humor-diario: liga-se ao item da linha
-- (template) e à matrícula/instância. Uma resposta por dia LOCAL (atualizável). O
-- comentário livre foi ADIADO (sem infra de cifra ainda) — MVP só dado estruturado.
-- Convenção: PK UUID v7 (app), TIMESTAMPTZ, enum via TEXT + CHECK.

-- O check-in emite um fato na jornada. Estende o vocabulário de event_type do 0007.
ALTER TABLE journey_event DROP CONSTRAINT journey_event_event_type_check;
ALTER TABLE journey_event ADD CONSTRAINT journey_event_event_type_check CHECK (event_type IN (
    'matricula_criada', 'matricula_renovada', 'matricula_expirada', 'matricula_encerrada',
    'consulta_agendada', 'consulta_cancelada', 'consulta_status_forcado',
    'checkin_humor_registrado'
));

CREATE TABLE mood_checkin (
    id                UUID PRIMARY KEY,
    patient_id        UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    enrollment_id     UUID NOT NULL REFERENCES enrollment (id) ON DELETE RESTRICT,
    care_line_item_id UUID NOT NULL REFERENCES care_line_item (id) ON DELETE RESTRICT,
    consent_id        UUID NOT NULL REFERENCES consent (id) ON DELETE RESTRICT,
    instrument_id     UUID NOT NULL REFERENCES instrument (id) ON DELETE RESTRICT,
    valencia          INT  NOT NULL CHECK (valencia BETWEEN 0 AND 100),
    energia           INT  NOT NULL CHECK (energia  BETWEEN 0 AND 100),
    quadrante         TEXT NOT NULL CHECK (btrim(quadrante) <> ''),  -- derivado (determinístico)
    emotion_label     TEXT,
    context_tags      JSONB,
    -- dia LOCAL (America/Sao_Paulo) calculado na aplicação: o "1 por dia" é do dia
    -- do colaborador, não do dia UTC. Evita a fronteira de meia-noite do fuso.
    dia_ref           DATE NOT NULL,
    respondido_em     TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Uma resposta por dia local (atualizável via upsert em (patient_id, dia_ref)).
CREATE UNIQUE INDEX ux_mood_checkin_dia ON mood_checkin (patient_id, dia_ref);
CREATE INDEX ix_mood_checkin_patient ON mood_checkin (patient_id, respondido_em DESC);
