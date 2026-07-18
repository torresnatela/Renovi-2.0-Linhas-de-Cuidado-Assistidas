-- 0005_care_line — o catálogo VERSIONADO das linhas de cuidado.
--
-- Uma linha de cuidado (ex.: "gestante", "diabetes") descreve a jornada que o
-- paciente vai percorrer: quais consultas, com que recorrência, sob quais regras.
-- O catálogo é imutável por VERSÃO: publicar não altera a versão anterior, cria a
-- próxima. Assim uma matrícula sabe exatamente qual desenho a regia, mesmo depois
-- de o admin publicar uma revisão.
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação, TIMESTAMPTZ
-- sempre, enum via TEXT + CHECK.

CREATE TABLE care_line (
    id           UUID PRIMARY KEY,
    code         TEXT NOT NULL CHECK (btrim(code) <> ''),
    version      INT  NOT NULL CHECK (version >= 1),
    name         TEXT NOT NULL CHECK (btrim(name) <> ''),
    description  TEXT NOT NULL DEFAULT '',
    -- draft: em edição, um por code (ver índice parcial abaixo).
    -- published: imutável e elegível para matrícula.
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT ux_care_line_code_version UNIQUE (code, version),

    -- Irmão do active_exige_vinculo_dav (0002): estado 'published' sem data é
    -- mentira. O banco recusa em vez de confiar na disciplina do model.
    CONSTRAINT published_exige_data CHECK (status <> 'published' OR published_at IS NOT NULL)
);

-- Um rascunho por code: dois admins não competem pela mesma versão nova. O
-- índice parcial é a trava — não há como abrir dois drafts do mesmo code.
CREATE UNIQUE INDEX ux_care_line_draft ON care_line (code) WHERE status = 'draft';

CREATE TABLE care_line_item (
    id             UUID PRIMARY KEY,
    care_line_id   UUID NOT NULL REFERENCES care_line (id) ON DELETE CASCADE,
    -- Chave lógica do item DENTRO da linha (ex.: "consulta_ginecologia"). É o que
    -- as regras PREREQUISITE e a jornada referenciam, estável entre versões.
    ref            TEXT NOT NULL CHECK (btrim(ref) <> ''),
    -- Só CONSULTA no piloto; TEXT+CHECK deixa o vocabulário crescer sem migration
    -- de tipo enum nativo.
    kind           TEXT NOT NULL CHECK (kind IN ('CONSULTA')),
    specialty_code TEXT NOT NULL CHECK (btrim(specialty_code) <> ''),
    label          TEXT NOT NULL CHECK (btrim(label) <> ''),
    recurrence     TEXT,
    sort_order     INT  NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT ux_care_line_item_ref UNIQUE (care_line_id, ref)
);

CREATE TABLE care_line_rule (
    id                UUID PRIMARY KEY,
    care_line_item_id UUID NOT NULL REFERENCES care_line_item (id) ON DELETE CASCADE,
    -- VIGENCIA fica DELIBERADAMENTE fora deste CHECK: ela é pré-condição da
    -- matrícula (a janela valid_from/valid_until vive em enrollment), não uma
    -- regra armazenada por item. O motor puro (models/careline) a avalia à parte.
    rule_type TEXT NOT NULL CHECK (rule_type IN ('QUOTA', 'MIN_INTERVAL', 'MAX_ADVANCE', 'PREREQUISITE')),
    params    JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_care_line_rule_item ON care_line_rule (care_line_item_id);
