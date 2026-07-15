-- MOCK do Postgres Gestão 2.0 (apenas dev/teste).
-- No produto real, este banco é de terceiro e o renovi-care o acessa SOMENTE
-- LEITURA via Adapter Gestão. O schema real será confirmado na Sprint 0 (SPEC §9.3).
--
-- Objetivo aqui: dar ao adapter algo plausível para ler durante o desenvolvimento.

CREATE TABLE IF NOT EXISTS empresa (
    id         BIGINT PRIMARY KEY,
    nome       TEXT NOT NULL,
    cnpj       CHAR(14) NOT NULL
);

CREATE TABLE IF NOT EXISTS contrato (
    id         BIGINT PRIMARY KEY,
    empresa_id BIGINT NOT NULL REFERENCES empresa(id),
    plano      TEXT NOT NULL,          -- ex.: 'saude-mental' (mapeia p/ template)
    ativo      BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS colaborador (
    id           BIGINT PRIMARY KEY,
    contrato_id  BIGINT NOT NULL REFERENCES contrato(id),
    nome         TEXT NOT NULL,
    cpf          CHAR(11) NOT NULL UNIQUE,  -- chave de ativação (SPEC §9.3)
    email        TEXT,
    elegivel     BOOLEAN NOT NULL DEFAULT true
);

-- Dados de exemplo (um colaborador elegível à linha de Saúde Mental).
INSERT INTO empresa (id, nome, cnpj) VALUES
    (1, 'Empresa Piloto LTDA', '00000000000191')
ON CONFLICT DO NOTHING;

INSERT INTO contrato (id, empresa_id, plano, ativo) VALUES
    (1, 1, 'saude-mental', true)
ON CONFLICT DO NOTHING;

INSERT INTO colaborador (id, contrato_id, nome, cpf, email, elegivel) VALUES
    (1, 1, 'Maria de Teste', '12345678901', 'maria.teste@example.com', true)
ON CONFLICT DO NOTHING;
