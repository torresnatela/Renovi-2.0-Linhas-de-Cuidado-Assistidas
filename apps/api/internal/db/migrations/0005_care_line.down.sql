-- Reverte 0005_care_line. Ordem inversa das dependências: as FKs apontam para
-- care_line_item e care_line, que caem por último. Os índices caem junto com as
-- tabelas.
DROP TABLE IF EXISTS care_line_rule;
DROP TABLE IF EXISTS care_line_item;
DROP TABLE IF EXISTS care_line;
