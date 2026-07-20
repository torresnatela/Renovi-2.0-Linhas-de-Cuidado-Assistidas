-- Reverte 0009: volta ao vocabulário só-CONSULTA e specialty_code NOT NULL.
-- Pressupõe que não há itens ATIVIDADE (rollback de catálogo em piloto): o
-- SET NOT NULL falharia se houvesse specialty_code NULL de alguma atividade.

ALTER TABLE care_line_item DROP CONSTRAINT specialty_por_kind;
ALTER TABLE care_line_item ALTER COLUMN specialty_code SET NOT NULL;
ALTER TABLE care_line_item ADD CONSTRAINT care_line_item_specialty_code_check
    CHECK (btrim(specialty_code) <> '');

ALTER TABLE care_line_item DROP CONSTRAINT care_line_item_kind_check;
ALTER TABLE care_line_item ADD CONSTRAINT care_line_item_kind_check
    CHECK (kind IN ('CONSULTA'));
