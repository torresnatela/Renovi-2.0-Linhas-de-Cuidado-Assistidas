-- 0009_activity_item — generaliza care_line_item para além de CONSULTA.
--
-- O Verificador Diário de Humor (Anexo C) é a primeira ATIVIDADE de uma linha de
-- cuidado: uma execução DENTRO da própria plataforma, sem especialidade do
-- legado. Esta migration:
--   1. estende o vocabulário de kind para incluir ATIVIDADE;
--   2. torna specialty_code condicional ao kind — CONSULTA exige especialidade,
--      ATIVIDADE não tem (NULL).
-- Convenção (docs/ARQUITETURA.md): enum via TEXT + CHECK, invariante no banco.

-- (1) kind ganha ATIVIDADE. O CHECK inline do 0005 tem o nome padrão do Postgres.
ALTER TABLE care_line_item DROP CONSTRAINT care_line_item_kind_check;
ALTER TABLE care_line_item ADD CONSTRAINT care_line_item_kind_check
    CHECK (kind IN ('CONSULTA', 'ATIVIDADE'));

-- (2) specialty_code deixa de ser NOT NULL global; a exigência passa a depender
-- do kind: CONSULTA precisa de especialidade não vazia, ATIVIDADE tem NULL.
ALTER TABLE care_line_item DROP CONSTRAINT care_line_item_specialty_code_check;
ALTER TABLE care_line_item ALTER COLUMN specialty_code DROP NOT NULL;
ALTER TABLE care_line_item ADD CONSTRAINT specialty_por_kind CHECK (
    (kind = 'CONSULTA' AND specialty_code IS NOT NULL AND btrim(specialty_code) <> '')
    OR (kind <> 'CONSULTA' AND specialty_code IS NULL)
);
