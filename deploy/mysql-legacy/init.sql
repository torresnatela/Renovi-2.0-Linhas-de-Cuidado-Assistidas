-- MOCK do MySQL legado (apenas dev/teste).
-- Fonte da verdade da escala/slots e da trava de agenda (SPEC §2.1). O renovi-care
-- só o acessa via Adapter Agenda: leitura de slots + escrita RESTRITA à tabela de
-- agendamento. O schema real será mapeado na Sprint 0 (SPEC §9.1).
--
-- Ponto crítico ilustrado abaixo: o slot ocupado precisa de uma restrição única
-- para tornar o double-booking impossível (passo 4 do fluxo de agendamento).

CREATE TABLE IF NOT EXISTS medico (
    id          BIGINT PRIMARY KEY,
    nome        VARCHAR(255) NOT NULL,
    especialidade VARCHAR(64) NOT NULL   -- 'PSICOLOGIA' | 'PSIQUIATRIA' | ...
);

CREATE TABLE IF NOT EXISTS slot (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    medico_id   BIGINT NOT NULL,
    inicio      DATETIME NOT NULL,
    fim         DATETIME NOT NULL,
    ocupado     TINYINT(1) NOT NULL DEFAULT 0,
    CONSTRAINT fk_slot_medico FOREIGN KEY (medico_id) REFERENCES medico(id),
    -- Impede dois slots idênticos para o mesmo médico/horário.
    UNIQUE KEY uq_slot_medico_inicio (medico_id, inicio)
);

CREATE TABLE IF NOT EXISTS agendamento (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    slot_id     BIGINT NOT NULL,
    paciente_ref VARCHAR(64) NOT NULL,   -- referência ao paciente (CPF/id externo)
    criado_em   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- A trava real do double-booking: um slot só pode ter UM agendamento ativo.
    UNIQUE KEY uq_agendamento_slot (slot_id),
    CONSTRAINT fk_agendamento_slot FOREIGN KEY (slot_id) REFERENCES slot(id)
);

-- Dados de exemplo.
INSERT IGNORE INTO medico (id, nome, especialidade) VALUES
    (1, 'Dra. Ana Psicóloga', 'PSICOLOGIA'),
    (2, 'Dr. Bruno Psiquiatra', 'PSIQUIATRIA');

INSERT IGNORE INTO slot (id, medico_id, inicio, fim, ocupado) VALUES
    (1, 1, '2026-07-20 09:00:00', '2026-07-20 09:50:00', 0),
    (2, 1, '2026-07-20 10:00:00', '2026-07-20 10:50:00', 0),
    (3, 2, '2026-07-22 14:00:00', '2026-07-22 14:50:00', 0);
