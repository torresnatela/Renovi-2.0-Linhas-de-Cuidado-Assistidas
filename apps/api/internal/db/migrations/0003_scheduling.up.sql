-- 0003_scheduling — o espelho local das consultas.
--
-- A consulta vive AQUI e só aqui (ADR-004 + a decisão desta feature): no MySQL
-- legado nós apenas viramos o flag `tb_slots.booked`, e na DAV existe o
-- appointment com o link do paciente. Este é o único lugar que amarra os três.
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação, TIMESTAMPTZ
-- sempre, enum via TEXT + CHECK.

CREATE TABLE appointment (
    id         UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,

    -- Chaves do legado. TEXT e não UUID: são varchar(36) lá e NADA no schema
    -- deles garante que sejam UUID. Um cast quebraria na primeira linha torta de
    -- um banco que não é nosso.
    legacy_slot_id         TEXT NOT NULL CHECK (btrim(legacy_slot_id) <> ''),
    -- É também o id do profissional na DAV (recurso /professional). Por isso não
    -- existe tabela de mapeamento — ver docs/DAV-API-NOTAS.md.
    legacy_professional_id TEXT NOT NULL CHECK (btrim(legacy_professional_id) <> ''),
    legacy_specialty_id    TEXT NOT NULL CHECK (btrim(legacy_specialty_id) <> ''),

    -- Fotografia do legado no momento do agendamento. Não é desnormalização
    -- preguiçosa: as FKs de lá são ON DELETE CASCADE e o schema é de terceiro. Se
    -- o profissional for renomeado ou removido, a consulta antiga ainda precisa
    -- saber se descrever ao paciente.
    professional_name TEXT NOT NULL CHECK (btrim(professional_name) <> ''),
    specialty_name    TEXT NOT NULL CHECK (btrim(specialty_name) <> ''),

    -- Instantes. O legado guarda DATETIME ingênuo (hora de parede de São Paulo);
    -- quem resolve para instante é o adapter da agenda.
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,

    -- Os estados da saga. Repare que são MAIS que os do contrato: o paciente vê
    -- PROCESSING/CONFIRMED/UNCONFIRMED/CANCELLED, e o mapeamento é do model.
    -- Vocabulário interno não vaza para o cliente.
    --
    --   PENDING_SLOT : intenção registrada; o horário ainda não é nosso.
    --   DAV_PENDING  : horário travado; a escrita insondável está em voo.
    --   CONFIRMED    : existe na DAV e temos o link do paciente.
    --   FAILED       : comprovadamente não aconteceu. Se travamos o horário, ele
    --                  volta ao mercado.
    --   DAV_UNKNOWN  : o ErrMaybeApplied persistido. NÃO sabemos se a consulta
    --                  existe lá, e (ao contrário do cadastro) NUNCA vamos saber
    --                  sozinhos: a DAV recusa id nosso no appointment e não tem
    --                  rota de busca. Só gente resolve.
    --   CANCELLED
    status TEXT NOT NULL CHECK (status IN (
        'PENDING_SLOT', 'DAV_PENDING', 'CONFIRMED', 'FAILED', 'DAV_UNKNOWN', 'CANCELLED'
    )),

    -- O id DELES: só o conhecemos se o POST responder.
    dav_appointment_id TEXT UNIQUE,
    -- O link de entrada do PACIENTE. É CREDENCIAL: nunca em listagem, nunca em
    -- log (ver JoinTicket no openapi.yaml).
    patient_join_url TEXT,

    -- A trilha da saga. Existe para o worker saber o que compensar sem adivinhar.
    slot_held_at     TIMESTAMPTZ, -- provamos que o horário é nosso
    slot_released_at TIMESTAMPTZ, -- devolvemos ao mercado
    dav_attempted_at TIMESTAMPTZ,
    confirmed_at     TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT janela_valida CHECK (ends_at > starts_at),

    -- Irmão do active_exige_vinculo_dav (0002_auth): consulta CONFIRMED sem link
    -- de entrada é MENTIRA — o paciente só descobre no horário da consulta, que é
    -- o pior momento possível. O banco recusa.
    CONSTRAINT confirmed_exige_dav CHECK (
        status <> 'CONFIRMED'
        OR (dav_appointment_id IS NOT NULL
            AND patient_join_url IS NOT NULL
            AND slot_held_at IS NOT NULL)
    ),

    CONSTRAINT liberado_exige_travado CHECK (
        slot_released_at IS NULL OR slot_held_at IS NOT NULL
    ),

    -- A trava mais importante do desenho, gravada no banco em vez de confiada à
    -- boa memória de quem escrever o próximo worker: enquanto não sabemos se a
    -- DAV criou a consulta, o horário NÃO volta.
    --
    -- Devolvê-lo deixaria outro paciente marcar em cima de uma consulta que
    -- talvez exista — e a DAV aceita duas consultas no mesmo horário para o mesmo
    -- profissional (achado #17), então ninguém barraria. Dois pacientes, um
    -- médico, um horário. Perder um horário é problema operacional; double-booking
    -- é problema clínico.
    CONSTRAINT desconhecido_nao_libera CHECK (
        status <> 'DAV_UNKNOWN' OR slot_released_at IS NULL
    )
);

-- A trava do NOSSO double-booking (contra o app legado quem defende é o CAS no
-- MySQL — são adversários diferentes e são precisos os dois).
--
-- Parcial: só as reservas VIVAS ocupam o horário. Uma tentativa que falhou de
-- verdade não pode impedir o próximo paciente de marcar o mesmo horário — mas
-- DAV_UNKNOWN ocupa, porque a consulta talvez exista.
CREATE UNIQUE INDEX ux_appointment_slot_vivo ON appointment (legacy_slot_id)
    WHERE status IN ('PENDING_SLOT', 'DAV_PENDING', 'CONFIRMED', 'DAV_UNKNOWN');

-- "Minhas consultas", a rota mais chamada da feature.
CREATE INDEX ix_appointment_account ON appointment (account_id, starts_at DESC);

-- A fila do worker: o que ainda precisa de compensação ou de conclusão.
CREATE INDEX ix_appointment_pendente ON appointment (updated_at)
    WHERE status IN ('PENDING_SLOT', 'DAV_PENDING', 'FAILED');

-- O que precisa de GENTE. Se esta lista cresce, alguém tem que olhar — e é a
-- única forma de descobrir, porque a máquina não consegue resolver sozinha.
CREATE INDEX ix_appointment_revisao ON appointment (created_at)
    WHERE status = 'DAV_UNKNOWN';
