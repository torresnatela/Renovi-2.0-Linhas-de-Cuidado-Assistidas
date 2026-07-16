-- 0002_auth — contas de paciente, identidade, endereço, sessões e auditoria do
-- vínculo com a Doutor ao Vivo (DAV).
--
-- Convenções (docs/ARQUITETURA.md): PK UUID v7 gerado na aplicação, TIMESTAMPTZ
-- sempre, enum via TEXT + CHECK.
--
-- LGPD (CLAUDE.md): "CPF só em tabelas de identidade" — por isso ele mora
-- sozinho em patient_identity, e não em patient_account.

CREATE TABLE patient_account (
    id            UUID PRIMARY KEY,
    full_name     TEXT NOT NULL CHECK (btrim(full_name) <> ''),
    email         TEXT NOT NULL,
    phone         TEXT NOT NULL,
    birth_date    DATE NOT NULL,
    password_hash TEXT NOT NULL, -- Argon2id em formato PHC (ver models/credential)

    -- PENDING_DAV: reservou o CPF mas a DAV ainda não confirmou. NÃO autentica.
    -- ACTIVE: vinculada à DAV e utilizável.
    -- BLOCKED: desativada por nós.
    status TEXT NOT NULL CHECK (status IN ('PENDING_DAV', 'ACTIVE', 'BLOCKED')),

    -- Id da pessoa na DAV. É o nosso `id` quando nós a criamos, ou o id que já
    -- existia lá quando anexamos uma pessoa preexistente.
    dav_person_id   TEXT UNIQUE,
    dav_link_origin TEXT CHECK (dav_link_origin IN ('CREATED', 'ATTACHED')),
    dav_linked_at   TIMESTAMPTZ,

    -- Encaixe do fator de posse (verificação por WhatsApp/e-mail), que entra
    -- depois. Nasce NULL para não exigir migration nova quando chegar.
    verified_at TIMESTAMPTZ,

    -- Trava progressiva de login (força bruta).
    failed_login_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
    locked_until       TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A regra de negócio central, gravada no banco em vez de confiada ao código:
    -- não existe conta utilizável sem vínculo com a DAV. Se a DAV falhar, a
    -- conta fica PENDING_DAV e o login recusa. Um bug futuro no model não
    -- consegue burlar isto.
    CONSTRAINT active_exige_vinculo_dav
        CHECK (status <> 'ACTIVE' OR (dav_person_id IS NOT NULL AND dav_link_origin IS NOT NULL))
);

-- Unicidade de e-mail sem depender de o app lembrar de normalizar. Índice
-- funcional em vez de coluna extra: não há como os dois divergirem.
-- (A DAV também exige e-mail único na base dela — ver docs/DAV-API-NOTAS.md.)
CREATE UNIQUE INDEX ux_patient_account_email ON patient_account (lower(btrim(email)));

-- Identidade fiscal, isolada. O CPF é a chave de identidade do paciente e o que
-- casa nossa conta com a pessoa na DAV.
CREATE TABLE patient_identity (
    account_id UUID PRIMARY KEY REFERENCES patient_account (id) ON DELETE CASCADE,
    cpf        CHAR(11) NOT NULL UNIQUE CHECK (cpf ~ '^[0-9]{11}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE patient_address (
    account_id   UUID PRIMARY KEY REFERENCES patient_account (id) ON DELETE CASCADE,
    zip_code     CHAR(8) NOT NULL CHECK (zip_code ~ '^[0-9]{8}$'),
    street       TEXT NOT NULL,
    number       TEXT NOT NULL,
    complement   TEXT,
    neighborhood TEXT NOT NULL,
    -- Nome do município (não o código IBGE): a DAV aceita ambos e o nome
    -- dispensa um lookup de CEP -> IBGE no cadastro.
    city       TEXT NOT NULL,
    state      CHAR(2) NOT NULL CHECK (state ~ '^[A-Z]{2}$'),
    country    CHAR(2) NOT NULL DEFAULT 'BR' CHECK (country ~ '^[A-Z]{2}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessão opaca (ADR-010). Guardamos o SHA-256 do token, nunca o token: quem ler
-- um dump do banco não consegue se passar por ninguém.
CREATE TABLE session (
    id           UUID PRIMARY KEY,
    account_id   UUID NOT NULL REFERENCES patient_account (id) ON DELETE CASCADE,
    token_hash   BYTEA NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_session_account ON session (account_id);
-- Para a varredura de expiradas; parcial porque só as vivas interessam.
CREATE INDEX ix_session_expires ON session (expires_at) WHERE revoked_at IS NULL;

-- Auditoria de todo vínculo com a DAV.
--
-- Por que existe: no piloto o cadastro é confiado (sem fator de posse), então um
-- CPF de terceiro permite anexar o prontuário alheio. Quando a verificação por
-- WhatsApp/e-mail entrar, esta tabela é o que permite revisar RETROATIVAMENTE
-- quem anexou o quê, e de onde. Sem ela, o histórico se perde.
-- RESTRICT, não CASCADE: apagar a conta NÃO pode apagar a trilha. Com CASCADE,
-- quem anexou o prontuário de um terceiro sumiria da auditoria bastando excluir a
-- própria conta — destruindo exatamente a evidência que o ADR-013 promete revisar.
-- Nada exclui conta hoje; o RESTRICT força a decisão a ser explícita quando
-- alguém implementar exclusão (inclusive por pedido de LGPD).
CREATE TABLE dav_link_audit (
    id            UUID PRIMARY KEY,
    account_id    UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    dav_person_id TEXT NOT NULL,
    origin        TEXT NOT NULL CHECK (origin IN ('CREATED', 'ATTACHED')),
    request_ip    INET,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_dav_link_audit_account ON dav_link_audit (account_id);
-- ATTACHED é o caso sensível: é o vínculo a um prontuário que já existia.
CREATE INDEX ix_dav_link_audit_attached ON dav_link_audit (created_at) WHERE origin = 'ATTACHED';
