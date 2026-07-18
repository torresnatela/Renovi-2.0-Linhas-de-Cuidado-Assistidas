-- 0010_consent — consentimento livre e informado (LGPD art. 11, dado sensível).
--
-- Pré-condição de GRAVAÇÃO do Verificador de Humor (Anexo C): sem consentimento
-- ativo para a finalidade, nenhuma resposta é persistida. Versionado
-- (versao_termo) e revogável.
--
-- O titular é o paciente (patient_account). O Anexo C fala em colaborador/empresa;
-- no renovi_care o sujeito autenticado é o paciente, e empresa/contrato entram
-- via gestao_contract_id da matrícula quando houver (opcional aqui).
-- Convenção: PK UUID v7 (app), TIMESTAMPTZ, enum via TEXT + CHECK.

CREATE TABLE consent (
    id                 UUID PRIMARY KEY,
    patient_id         UUID NOT NULL REFERENCES patient_account (id) ON DELETE RESTRICT,
    gestao_contract_id TEXT,
    finalidade         TEXT NOT NULL CHECK (btrim(finalidade) <> ''),   -- ex.: 'checkin_humor'
    versao_termo       TEXT NOT NULL CHECK (btrim(versao_termo) <> ''),
    status             TEXT NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'revogado')),
    concedido_em       TIMESTAMPTZ NOT NULL DEFAULT now(),
    revogado_em        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- 'revogado' exige data de revogação; 'ativo' não a tem. Invariante no banco.
    CONSTRAINT revogado_exige_data CHECK (
        (status = 'revogado' AND revogado_em IS NOT NULL)
        OR (status = 'ativo' AND revogado_em IS NULL)
    )
);

-- No máximo UM consentimento ativo por (paciente, finalidade). Reconceder com um
-- termo novo exige revogar o anterior antes — o model faz isso numa transação.
CREATE UNIQUE INDEX ux_consent_ativo ON consent (patient_id, finalidade) WHERE status = 'ativo';

CREATE INDEX ix_consent_patient ON consent (patient_id, finalidade);
