-- 0008_app_role — o role restrito da aplicação (renovi_app).
--
-- As migrations rodam como OWNER (renovi); a aplicação em runtime roda como
-- renovi_app, um role com menos privilégio. É esse role que impõe o append-only de
-- journey_event NO BANCO, não por disciplina de código: mesmo um bug que emita um
-- UPDATE/DELETE contra a tabela é RECUSADO pelo Postgres (SQLSTATE 42501).
--
-- CUIDADO (sqlc): todo o DDL de role vive dentro de UM bloco `DO $$ ... $$`. O
-- corpo dollar-quoted é uma STRING para o parser do sqlc — ele não tenta entender
-- CREATE ROLE / GRANT (que não são DML e o sqlc não modela). PL/pgSQL ainda nos dá
-- idempotência (o IF NOT EXISTS no CREATE ROLE).
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'renovi_app') THEN
        -- Senha de DEV, batendo com o docker-compose e o .env local. Em
        -- staging/produção o operador roda `ALTER ROLE renovi_app PASSWORD ...`
        -- com um segredo real fora do controle de versão.
        CREATE ROLE renovi_app LOGIN PASSWORD 'renovi_app';
    END IF;

    GRANT USAGE ON SCHEMA public TO renovi_app;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO renovi_app;
    -- Tabelas futuras (migrations posteriores rodadas como owner) já nascem com o
    -- privilégio, sem precisar relembrar de conceder.
    ALTER DEFAULT PRIVILEGES IN SCHEMA public
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO renovi_app;

    -- O append-only imposto no BANCO: journey_event só aceita INSERT e SELECT.
    REVOKE UPDATE, DELETE ON journey_event FROM renovi_app;
    -- A tabela de controle do golang-migrate não é da aplicação; o app não a toca.
    REVOKE ALL ON schema_migrations FROM renovi_app;
END $$;
