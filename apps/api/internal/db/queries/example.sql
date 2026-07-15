-- Queries de EXEMPLO para a tabela example_widget (ver migration 0001).
-- Demonstram o fluxo sqlc: SQL escrito à mão aqui -> `make generate` -> Go tipado
-- em internal/db/gen. Ao criar as tabelas reais, substitua estas queries.

-- name: GetExampleWidget :one
SELECT id, name, status, config, created_at, updated_at
FROM example_widget
WHERE id = $1;

-- name: CreateExampleWidget :one
INSERT INTO example_widget (id, name, status, config)
VALUES ($1, $2, $3, $4)
RETURNING id, name, status, config, created_at, updated_at;

-- name: ListExampleWidgets :many
SELECT id, name, status, config, created_at, updated_at
FROM example_widget
ORDER BY created_at DESC;
