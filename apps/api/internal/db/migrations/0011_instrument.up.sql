-- 0011_instrument — catálogo de instrumentos do Verificador de Humor (Anexo C.6.2).
--
-- Cortes, dimensões e polaridades vivem em DADOS versionados, não em código
-- (o pacote puro models/mood/scoring recebe os cortes por parâmetro). Mudar um
-- corte = migration nova, não deploy de código.
-- Convenção: PK UUID v7 (app), TIMESTAMPTZ, enum via TEXT + CHECK.

CREATE TABLE instrument (
    id         UUID PRIMARY KEY,
    codigo     TEXT NOT NULL CHECK (btrim(codigo) <> ''),  -- 'GRID' | 'WHO5' | 'PHQ4'
    versao     TEXT NOT NULL CHECK (btrim(versao) <> ''),
    anel       TEXT NOT NULL CHECK (anel IN ('diario', 'semanal', 'gatilhado')),
    licenca    TEXT NOT NULL CHECK (licenca IN ('livre', 'restrita')),
    ativo      BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ux_instrument_codigo_versao UNIQUE (codigo, versao)
);
-- Um instrumento ATIVO por código (a versão em vigor).
CREATE UNIQUE INDEX ux_instrument_ativo ON instrument (codigo) WHERE ativo;

CREATE TABLE instrument_dimension (
    id            UUID PRIMARY KEY,
    instrument_id UUID NOT NULL REFERENCES instrument (id) ON DELETE CASCADE,
    dimensao      TEXT NOT NULL,   -- 'valencia'|'energia'|'depressao'|'ansiedade'|'bem_estar'
    polaridade    TEXT NOT NULL CHECK (polaridade IN ('positiva', 'negativa')),
    min_score     NUMERIC NOT NULL,
    max_score     NUMERIC NOT NULL,
    CONSTRAINT ux_instrument_dimension UNIQUE (instrument_id, dimensao)
);

CREATE TABLE instrument_cutoff (
    id               UUID PRIMARY KEY,
    instrument_id    UUID NOT NULL REFERENCES instrument (id) ON DELETE CASCADE,
    dimensao         TEXT NOT NULL,
    faixa            TEXT NOT NULL,   -- 'sinaliza'|'encaminha'|'moderado'|'subescala_positiva'
    operador         TEXT NOT NULL CHECK (operador IN ('<', '>=', 'entre')),
    valor            NUMERIC NOT NULL,
    valor_max        NUMERIC,
    origem_validacao TEXT NOT NULL    -- referência da validação BR usada
);
CREATE INDEX ix_instrument_cutoff ON instrument_cutoff (instrument_id, dimensao);

CREATE TABLE emotion_label (
    id        UUID PRIMARY KEY,
    quadrante TEXT NOT NULL,
    rotulo    TEXT NOT NULL,
    ativo     BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE context_tag (
    id     UUID PRIMARY KEY,
    chave  TEXT NOT NULL,
    rotulo TEXT NOT NULL,
    ativo  BOOLEAN NOT NULL DEFAULT true,
    CONSTRAINT ux_context_tag_chave UNIQUE (chave)
);

-- ---------------------------------------------------------------------------
-- Seed dos 3 instrumentos (reference data versionada). IDs fixos para os
-- instrumentos (referência estável); filhos usam gen_random_uuid().
-- Cortes = validação BRASILEIRA (Anexo C.4/C.10):
--   WHO-5:  <50 sinaliza, <28 encaminha        (de Souza & Hidalgo, 2012)
--   PHQ-4:  subescala >=3 rastreio positivo     (Santos 2013 / Moreno 2016)
--           total >=6 sofrimento moderado       (Kroenke et al., 2009)
-- Paleta e vocabulário PRÓPRIOS da Renovi (Mood Meter é marca da Yale).
-- ---------------------------------------------------------------------------
INSERT INTO instrument (id, codigo, versao, anel, licenca) VALUES
  ('11111111-1111-4111-8111-000000000001', 'GRID', '1', 'diario',    'livre'),
  ('11111111-1111-4111-8111-000000000002', 'WHO5', '1', 'semanal',   'livre'),
  ('11111111-1111-4111-8111-000000000003', 'PHQ4', '1', 'gatilhado', 'livre');

INSERT INTO instrument_dimension (id, instrument_id, dimensao, polaridade, min_score, max_score) VALUES
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000001', 'valencia',  'positiva', 0, 100),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000001', 'energia',   'positiva', 0, 100),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000002', 'bem_estar', 'positiva', 0, 100),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000003', 'depressao', 'negativa', 0, 6),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000003', 'ansiedade', 'negativa', 0, 6);

INSERT INTO instrument_cutoff (id, instrument_id, dimensao, faixa, operador, valor, origem_validacao) VALUES
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000002', 'bem_estar', 'sinaliza',           '<',  50, 'de Souza & Hidalgo, 2012'),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000002', 'bem_estar', 'encaminha',          '<',  28, 'de Souza & Hidalgo, 2012'),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000003', 'depressao', 'subescala_positiva', '>=', 3,  'Santos et al., 2013'),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000003', 'ansiedade', 'subescala_positiva', '>=', 3,  'Moreno et al., 2016'),
  (gen_random_uuid(), '11111111-1111-4111-8111-000000000003', 'total',     'moderado',           '>=', 6,  'Kroenke et al., 2009');

INSERT INTO emotion_label (id, quadrante, rotulo) VALUES
  (gen_random_uuid(), 'agradavel_ativado',    'Animado'),
  (gen_random_uuid(), 'agradavel_ativado',    'Motivado'),
  (gen_random_uuid(), 'agradavel_ativado',    'Empolgado'),
  (gen_random_uuid(), 'agradavel_calmo',      'Tranquilo'),
  (gen_random_uuid(), 'agradavel_calmo',      'Sereno'),
  (gen_random_uuid(), 'agradavel_calmo',      'Grato'),
  (gen_random_uuid(), 'desagradavel_ativado', 'Ansioso'),
  (gen_random_uuid(), 'desagradavel_ativado', 'Tenso'),
  (gen_random_uuid(), 'desagradavel_ativado', 'Irritado'),
  (gen_random_uuid(), 'desagradavel_calmo',   'Desanimado'),
  (gen_random_uuid(), 'desagradavel_calmo',   'Cansado'),
  (gen_random_uuid(), 'desagradavel_calmo',   'Triste');

INSERT INTO context_tag (id, chave, rotulo) VALUES
  (gen_random_uuid(), 'trabalho',   'Trabalho'),
  (gen_random_uuid(), 'sono',       'Sono'),
  (gen_random_uuid(), 'relacoes',   'Relações'),
  (gen_random_uuid(), 'saude',      'Saúde'),
  (gen_random_uuid(), 'financeiro', 'Financeiro'),
  (gen_random_uuid(), 'pessoal',    'Pessoal');
