-- Reverte 0008_app_role. Revoga os privilégios e tenta remover o role.
--
-- O DROP ROLE é protegido: se ainda houver objetos de que o role depende (ou
-- privilégios em outros bancos), `dependent_objects_still_exist` é engolido em vez
-- de derrubar o down inteiro. E se o role já não existir, `undefined_object`
-- também — o down é idempotente.
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'renovi_app') THEN
        ALTER DEFAULT PRIVILEGES IN SCHEMA public
            REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM renovi_app;
        REVOKE ALL ON ALL TABLES IN SCHEMA public FROM renovi_app;
        REVOKE ALL ON SCHEMA public FROM renovi_app;

        BEGIN
            DROP ROLE renovi_app;
        EXCEPTION
            WHEN dependent_objects_still_exist THEN NULL;
            WHEN undefined_object THEN NULL;
        END;
    END IF;
END $$;
