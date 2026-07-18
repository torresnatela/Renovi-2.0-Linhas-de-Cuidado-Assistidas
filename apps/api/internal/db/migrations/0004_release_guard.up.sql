-- 0004_release_guard — endurece a liberação do horário.
--
-- O 0003 já impedia soltar o slot de uma consulta DAV_UNKNOWN
-- (desconhecido_nao_libera). Isto é mais forte e fecha o buraco de ordenação da
-- compensação: só uma consulta em estado TERMINAL pode ter o horário registrado
-- como devolvido. Assim, se por qualquer motivo o `status = 'FAILED'` não ficou
-- durável, o banco RECUSA gravar `slot_released_at` — em vez de o Postgres
-- registrar uma reserva ainda viva enquanto o MySQL já liberou o slot.
--
-- `NOT VALID` + `VALIDATE`: adiciona a constraint sem varrer a tabela sob lock
-- pesado (ela é nova e pequena, mas é o hábito certo para ALTER em produção).
ALTER TABLE appointment
    ADD CONSTRAINT liberado_exige_terminal
    CHECK (slot_released_at IS NULL OR status IN ('FAILED', 'CANCELLED'))
    NOT VALID;

ALTER TABLE appointment VALIDATE CONSTRAINT liberado_exige_terminal;
