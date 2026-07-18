-- 0007_care_journey — a jornada realizada da matrícula.
--
-- Duas tabelas com propósitos distintos:
--   care_appointment: a consulta da jornada AMARRADA à matrícula e ao item do
--     catálogo. É a projeção clínica do agendamento (appointment, 0003), que é a
--     saga técnica. Ela referencia o booking por id LÓGICO, sem FK.
--   journey_event: o log append-only da linha do tempo do paciente. É o que
--     alimenta a tela de jornada e a auditoria.
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação, TIMESTAMPTZ
-- sempre, enum via TEXT + CHECK.

CREATE TABLE care_appointment (
    id                UUID PRIMARY KEY,
    -- RESTRICT: a jornada não some quando a matrícula ou o item some por engano.
    enrollment_id     UUID NOT NULL REFERENCES enrollment (id) ON DELETE RESTRICT,
    care_line_item_id UUID NOT NULL REFERENCES care_line_item (id) ON DELETE RESTRICT,
    item_ref          TEXT NOT NULL,

    -- Referência LÓGICA à saga (tabela appointment, 0003). SEM FK, de propósito:
    -- o agendamento é um módulo atrás de interface (ADR-012), com ciclo de vida
    -- independente. Amarrá-los por FK acoplaria os dois esquemas; o id lógico +
    -- índice único abaixo dá a garantia (um booking, uma consulta de jornada) sem
    -- o acoplamento.
    booking_id   UUID NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'agendada', 'confirmada', 'em_andamento', 'realizada', 'falta', 'cancelada'
    )),
    cancelled_at TIMESTAMPTZ,
    -- Idempotência sem tabela própria: mesma key na mesma matrícula = mesma
    -- consulta (ver índice único parcial). Nulo quando o chamador não trouxe key.
    idempotency_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Par de irmãos: cancelada exige data, e ter data exige estar cancelada. Um sem
    -- o outro é estado incoerente, e o banco recusa em vez de confiar no model.
    CONSTRAINT cancelada_exige_data CHECK (status <> 'cancelada' OR cancelled_at IS NOT NULL),
    CONSTRAINT data_exige_cancelada CHECK (cancelled_at IS NULL OR status = 'cancelada')
);

-- Idempotência: mesma key na mesma matrícula não cria duas consultas. Parcial
-- porque a maioria das linhas não traz key (nulos não colidem).
CREATE UNIQUE INDEX ux_care_appt_idem ON care_appointment (enrollment_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
-- Um booking técnico projeta no máximo uma consulta de jornada.
CREATE UNIQUE INDEX ux_care_appt_booking ON care_appointment (booking_id);
CREATE INDEX ix_care_appt_enrollment ON care_appointment (enrollment_id, scheduled_at);

CREATE TABLE journey_event (
    id            UUID PRIMARY KEY,  -- UUID v7: ordena junto com occurred_at
    -- RESTRICT: a trilha da jornada não some quando a matrícula/paciente some.
    enrollment_id UUID NOT NULL REFERENCES enrollment (id) ON DELETE RESTRICT,
    patient_id    UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'matricula_criada', 'matricula_renovada', 'matricula_expirada', 'matricula_encerrada',
        'consulta_agendada', 'consulta_cancelada', 'consulta_status_forcado'
    )),
    actor TEXT NOT NULL CHECK (actor IN ('paciente', 'sistema', 'admin')),
    -- Ponteiro fraco para o objeto que originou o evento (ex.: 'care_appointment').
    ref_table TEXT,
    ref_id    UUID,
    payload   JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
    -- SEM updated_at: append-only por desenho E por privilégio de banco (0008
    -- revoga UPDATE/DELETE do role da aplicação). Um evento nunca muda; a correção
    -- é um novo evento.
);

-- A tela de jornada: os eventos do paciente do mais recente ao mais antigo. O id
-- (v7) desempata occurred_at empatado, e é a chave do keyset de paginação.
CREATE INDEX ix_journey_event_patient ON journey_event (patient_id, occurred_at DESC, id DESC);
CREATE INDEX ix_journey_event_enrollment ON journey_event (enrollment_id, occurred_at DESC, id DESC);
