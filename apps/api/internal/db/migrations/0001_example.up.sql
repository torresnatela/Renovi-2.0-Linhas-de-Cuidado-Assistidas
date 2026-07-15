-- 0001_example — EXEMPLO DE REFERÊNCIA (não é tabela de domínio).
--
-- Demonstra as convenções do banco renovi_care descritas em docs/ARQUITETURA.md:
--   * PRIMARY KEY UUID (gerado na aplicação, UUID v7);
--   * TIMESTAMPTZ sempre;
--   * enum via TEXT + CHECK (evolução sem ALTER TYPE);
--   * regras/config declarativas em JSONB.
--
-- Ao criar as tabelas reais (patient_account, care_line_template, enrollment,
-- journey_event, appointment, ...), adicione novas migrations e REMOVA esta.
CREATE TABLE example_widget (
    id         UUID PRIMARY KEY,
    name       TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('DRAFT', 'ACTIVE', 'RETIRED')),
    config     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
