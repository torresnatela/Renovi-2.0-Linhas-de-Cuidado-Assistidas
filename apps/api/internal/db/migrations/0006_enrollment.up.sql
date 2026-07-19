-- 0006_enrollment — a matrícula do paciente numa linha de cuidado.
--
-- A matrícula amarra um paciente a uma VERSÃO específica do catálogo (care_line_id)
-- e carrega a janela de vigência (valid_from/valid_until) que a rege. Guardamos
-- também o care_line_code redundante ao id: a trava de "uma matrícula viva por
-- linha" precisa valer entre versões diferentes do mesmo code, então ela indexa o
-- code, não o id da versão.
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação, TIMESTAMPTZ
-- sempre, enum via TEXT + CHECK.

CREATE TABLE enrollment (
    id             UUID PRIMARY KEY,
    -- RESTRICT (não CASCADE): apagar paciente ou linha não pode apagar a matrícula
    -- em silêncio — a jornada e a auditoria dependem dela. A exclusão vira decisão
    -- explícita de quem implementar remoção (inclusive por LGPD).
    patient_id     UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    care_line_id   UUID NOT NULL REFERENCES care_line (id) ON DELETE RESTRICT,
    -- Redundante ao care_line_id de propósito: a trava ux_enrollment_viva precisa
    -- valer independente de versão (ver abaixo).
    care_line_code TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('ativa', 'pausada', 'concluida', 'encerrada', 'expirada')),
    valid_from  TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    -- Contrato correspondente na Gestão 2.0, quando houver. Opcional: o piloto
    -- matricula pela mão do admin antes da integração da Gestão.
    gestao_contract_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT vigencia_valida CHECK (valid_until > valid_from)
);

-- A trava de matrícula: no máximo UMA viva por (paciente, linha), independente da
-- versão do catálogo. Parcial porque só 'ativa' e 'pausada' ocupam a linha —
-- concluida/encerrada/expirada liberam o paciente para uma nova matrícula.
CREATE UNIQUE INDEX ux_enrollment_viva ON enrollment (patient_id, care_line_code)
    WHERE status IN ('ativa', 'pausada');

CREATE INDEX ix_enrollment_patient ON enrollment (patient_id, created_at DESC);

CREATE TABLE enrollment_period (
    id            UUID PRIMARY KEY,
    enrollment_id UUID NOT NULL REFERENCES enrollment (id) ON DELETE CASCADE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at   TIMESTAMPTZ NOT NULL,
    -- Só 'admin' no piloto (a renovação é manual). TEXT+CHECK deixa 'gestao' entrar
    -- depois sem migration de tipo.
    source    TEXT NOT NULL DEFAULT 'admin' CHECK (source IN ('admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A contiguidade entre períodos (o próximo começa onde o anterior termina) é
    -- validada no CASO DE USO da renovação, não aqui: exigiria varrer os irmãos e
    -- um CHECK não enxerga outras linhas. O banco garante só o período individual.
    CONSTRAINT periodo_valido CHECK (ends_at > starts_at)
);

CREATE INDEX ix_enrollment_period ON enrollment_period (enrollment_id, starts_at);
