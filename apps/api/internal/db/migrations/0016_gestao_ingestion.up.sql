-- 0016_gestao_ingestion — a ingestão (push) de contratos vindos da Renovi Gestão.
--
-- O Gestão faz POST na nossa API com empresa + colaborador + contrato; nós
-- persistimos aqui, cunhamos um token de onboarding e emitimos o convite. Nunca
-- escrevemos no banco do Gestão (ADR-043 — o mecanismo virou push, superando o
-- pull "Adapter Gestão" do ARQUITETURA §3 e o "manual pelo admin" do ADR-009).
--
-- Chave da pessoa: cpf_hmac = HMAC-SHA256(cpf, CPF_PEPPER), com pepper
-- compartilhado com o Gestão. O CPF em claro nunca trafega e nunca sai de
-- patient_identity (LGPD, CLAUDE.md).
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação (sem default no
-- banco), TIMESTAMPTZ sempre, enum via TEXT + CHECK, bytea de hash com
-- CHECK(octet_length = 32) como session.token_hash (0002).

-- A EMPRESA. Espelho mínimo do que o Gestão nos conta; unicidade por id do Gestão
-- é o que torna o upsert idempotente.
CREATE TABLE gestao_company_link (
    id                UUID PRIMARY KEY,
    gestao_company_id TEXT NOT NULL UNIQUE,
    display_name      TEXT NOT NULL CHECK (btrim(display_name) <> ''),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A PESSOA: identidade do colaborador <-> patient (1 por CPF, atravessa empresas).
-- invite_* são um snapshot para o e-mail e o pré-preenchimento do cadastro.
-- patient_id nasce NULL e só é preenchido quando o onboarding fecha (fatia futura).
CREATE TABLE gestao_employee_link (
    id           UUID PRIMARY KEY,
    cpf_hmac     BYTEA NOT NULL CHECK (octet_length(cpf_hmac) = 32),
    invite_name  TEXT NOT NULL CHECK (btrim(invite_name) <> ''),
    invite_email TEXT,
    invite_phone TEXT,
    -- RESTRICT: apagar a conta não pode apagar em silêncio o vínculo do Gestão.
    patient_id   UUID REFERENCES patient_account (id) ON DELETE RESTRICT,
    status       TEXT NOT NULL DEFAULT 'pendente'
                 CHECK (status IN ('pendente', 'vinculado', 'cancelado')),
    link_method  TEXT CHECK (link_method IN ('convite', 'cpf_match')),
    linked_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A regra "vinculado só existe completo" gravada no banco, não confiada ao
    -- model (mesmo espírito do active_exige_vinculo_dav, 0002): um vínculo fechado
    -- exige paciente, método e data juntos.
    CONSTRAINT vinculado_completo CHECK (
        status <> 'vinculado'
        OR (patient_id IS NOT NULL AND link_method IS NOT NULL AND linked_at IS NOT NULL)
    )
);

-- Uma trava só, parcial, no padrão da casa (ux_enrollment_viva/ux_consent_ativo):
-- no máximo uma pessoa VIVA por cpf_hmac. Um vínculo 'cancelado' libera o CPF para
-- um novo onboarding no futuro, e é este índice o arbiter do upsert por CPF.
CREATE UNIQUE INDEX ux_gestao_employee_ativo ON gestao_employee_link (cpf_hmac)
    WHERE status <> 'cancelado';

-- O VÍNCULO pessoa x empresa (N:N no tempo). A linha não morre: o status transita
-- ativo -> afastado -> desligado. accepted_at é o consentimento do titular para
-- ESTA empresa (fica NULL até a conclusão do onboarding — fatia futura).
CREATE TABLE gestao_contract (
    id                      UUID PRIMARY KEY,
    gestao_contract_id      TEXT NOT NULL UNIQUE,
    -- employee do Gestão (por tenant), snapshot; sem FK (é id de outro banco).
    gestao_employee_id      TEXT NOT NULL,
    gestao_employee_link_id UUID NOT NULL REFERENCES gestao_employee_link (id) ON DELETE RESTRICT,
    gestao_company_link_id  UUID NOT NULL REFERENCES gestao_company_link (id) ON DELETE RESTRICT,
    status                  TEXT NOT NULL CHECK (status IN ('ativo', 'afastado', 'desligado')),
    accepted_at             TIMESTAMPTZ,
    started_at              TIMESTAMPTZ NOT NULL,
    ended_at                TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Desligar exige a data do desligamento (espelha cancelada_exige_data, 0007).
    CONSTRAINT desligado_exige_data CHECK (status <> 'desligado' OR ended_at IS NOT NULL)
);
CREATE INDEX ix_gestao_contract_employee ON gestao_contract (gestao_employee_link_id);
CREATE INDEX ix_gestao_contract_company  ON gestao_contract (gestao_company_link_id);

-- O TOKEN de onboarding (TTL por INVITE_TTL, default 7 dias). Guardamos o SHA-256
-- do token, nunca o token — mesmo desenho da sessão (0002): quem lê um dump não se
-- passa por ninguém.
CREATE TABLE onboarding_token (
    id                      UUID PRIMARY KEY,
    gestao_employee_link_id UUID NOT NULL REFERENCES gestao_employee_link (id) ON DELETE RESTRICT,
    token_hash              BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at              TIMESTAMPTZ NOT NULL,
    used_at                 TIMESTAMPTZ,
    revoked_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- No máximo UM convite vivo por pessoa. Parcial porque usado/revogado não conta —
-- é o que faz o reenvio (revoga + cunha novo) e o push idempotente conviverem.
CREATE UNIQUE INDEX ux_token_vivo ON onboarding_token (gestao_employee_link_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX ix_onboarding_token_link ON onboarding_token (gestao_employee_link_id);

-- Trilha append-only da ingestão (cultura de auditoria do projeto: dav_link_audit,
-- journey_event). NUNCA guarda CPF em claro — só o cpf_hmac. Append-only imposto
-- pelo privilégio de banco no bloco DO abaixo (ADR-024).
CREATE TABLE gestao_ingestion_event (
    id                      UUID PRIMARY KEY,
    event_type              TEXT NOT NULL CHECK (event_type IN (
        'contrato_recebido', 'convite_emitido', 'convite_reenviado', 'cpf_match_pendente'
    )),
    gestao_contract_id      TEXT,
    gestao_employee_link_id UUID,
    cpf_hmac                BYTEA,
    payload                 JSONB NOT NULL DEFAULT '{}',
    occurred_at             TIMESTAMPTZ NOT NULL DEFAULT now()
    -- SEM updated_at: um evento nunca muda; a correção é um novo evento.
);
CREATE INDEX ix_gestao_ingestion_event_link
    ON gestao_ingestion_event (gestao_employee_link_id, occurred_at DESC);

-- cpf_hmac na identidade: coluna ADICIONAL ao cpf em claro (que o login por CPF e o
-- vínculo DAV continuam usando). É o que permite casar, sem CPF em claro, a pessoa
-- que o Gestão manda contra um paciente que já existe. Nasce NULL; o cadastro passa
-- a preenchê-la e o backfill (cmd/backfill-cpf-hmac) cobre as linhas antigas.
ALTER TABLE patient_identity
    ADD COLUMN cpf_hmac BYTEA CHECK (cpf_hmac IS NULL OR octet_length(cpf_hmac) = 32);
CREATE UNIQUE INDEX ux_patient_identity_cpf_hmac ON patient_identity (cpf_hmac)
    WHERE cpf_hmac IS NOT NULL;

-- Append-only de gestao_ingestion_event, imposto pelo BANCO (ADR-024, como 0008).
-- Dentro de um DO $$ para o sqlc tratar o corpo como string (ele não modela GRANT/
-- REVOKE). A tabela nasceu com o GRANT completo via ALTER DEFAULT PRIVILEGES (0008);
-- aqui revogamos a mutação, deixando só INSERT + SELECT ao role da aplicação.
DO $$
BEGIN
    REVOKE UPDATE, DELETE ON gestao_ingestion_event FROM renovi_app;
END $$;
