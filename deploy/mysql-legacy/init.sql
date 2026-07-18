-- MOCK do MySQL legado da Renovi (apenas dev/teste).
--
-- O DDL abaixo é CÓPIA FIEL do schema real, extraído de `homl_renovi` em
-- 2026-07-16 via SHOW CREATE TABLE (só as tabelas que o agendamento toca; o
-- banco real tem 34). O mock anterior era inventado (`medico`/`slot`/
-- `agendamento`) e mentia no ponto que mais importa — ver "A trava" abaixo.
--
-- Este arquivo é fixture, não migration: NÓS NÃO somos donos deste banco e não
-- podemos alterá-lo. Se o legado mudar, isto aqui não fica sabendo — por isso o
-- teste de integração do adapter confere as colunas contra o information_schema
-- em vez de confiar nesta cópia.
--
-- Papel (ADR-004): verdade da escala/slots. O renovi-care lê slots e a ÚNICA
-- escrita que faz é virar `tb_slots.booked`. Não inserimos em `tb_appointments`:
-- a consulta vive no nosso Postgres.
--
-- ---------------------------------------------------------------------------
-- A trava (o que o mock antigo errava)
-- ---------------------------------------------------------------------------
-- O mock antigo tinha `UNIQUE KEY uq_agendamento_slot (slot_id)` e dizia ser "a
-- trava real do double-booking". O schema REAL não tem restrição nenhuma:
-- `tb_appointments.slot` é varchar NULL, sem FK e sem unique, e `tb_slots` só
-- tem a PK. `booked` é um flag solto.
--
-- Então a trava é comportamental, não estrutural. Medido na HML (2026-07-16):
--
--     status da consulta | booked=1 | booked=0
--     SCHEDULED          |     1    |    0
--     FINISHED           |    83    |    1
--     CANCELED           |    26    |   49
--
-- Ou seja: o app legado VIRA booked=1 ao agendar (84 de 85 consultas ativas), e
-- nenhum slot jamais teve duas consultas ativas. Logo `booked` é um interlock de
-- verdade e um CAS (`UPDATE ... WHERE booked = 0`) basta para não correr com o
-- legado. Repare também nos 26 CANCELED que ficaram com booked=1: o próprio
-- legado já vaza slot fantasma — é o mesmo tipo de resíduo que a nossa saga
-- aceita produzir quando a DAV não responde, e não um dano novo.
--
-- ---------------------------------------------------------------------------
-- Fuso
-- ---------------------------------------------------------------------------
-- As colunas são DATETIME, que o MySQL guarda LITERAL (ao contrário de
-- TIMESTAMP, que o servidor converte). Não há fuso gravado: é hora de parede de
-- America/Sao_Paulo. Quem resolve para instante é o adapter, com
-- parseTime=true&loc=America%2FSao_Paulo — sem isso, 09:00 vira 09:00Z e a
-- consulta acontece 3h fora, em silêncio.

-- ---------------------------------------------------------------------------
-- Schema real
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS `tb_specialities` (
  `id` varchar(36) NOT NULL,
  `wanted` int NOT NULL,
  `name` varchar(255) NOT NULL,
  `description` varchar(255) DEFAULT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `active` tinyint(1) DEFAULT '1',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `tb_professionals` (
  `id` varchar(36) NOT NULL,
  `firstName` varchar(255) NOT NULL,
  `lastName` varchar(255) NOT NULL,
  `birthday` datetime NOT NULL,
  `email` varchar(255) NOT NULL,
  `cpf` varchar(255) NOT NULL,
  `licenseNumber` varchar(255) NOT NULL,
  `licenseRegion` varchar(255) NOT NULL,
  `licenseCouncil` varchar(255) NOT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `imageUrl` text,
  `rqe` varchar(55) DEFAULT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `tb_professionals_specialities` (
  `professionalId` varchar(255) NOT NULL,
  `specialityId` varchar(255) NOT NULL,
  PRIMARY KEY (`professionalId`,`specialityId`),
  KEY `FK_tb_professionals_specialities_tb_specialities` (`specialityId`),
  CONSTRAINT `FK_tb_professionals_specialities_tb_professionals` FOREIGN KEY (`professionalId`) REFERENCES `tb_professionals` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `FK_tb_professionals_specialities_tb_specialities` FOREIGN KEY (`specialityId`) REFERENCES `tb_specialities` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `tb_shifts` (
  `id` varchar(36) NOT NULL,
  `professionalId` varchar(255) NOT NULL,
  -- Vocabulário observado na HML: só 'AVAILABLE' (129 de 129). Como não
  -- conhecemos os outros valores possíveis, o adapter NÃO filtra por status:
  -- filtrar por um valor chutado esconderia a agenda inteira e pareceria
  -- "não há horários".
  `status` varchar(255) NOT NULL,
  `startsAt` datetime NOT NULL,
  `endsAt` datetime NOT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `FK_tb_shifts_tb_professionals` (`professionalId`),
  CONSTRAINT `FK_tb_shifts_tb_professionals` FOREIGN KEY (`professionalId`) REFERENCES `tb_professionals` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `tb_slots` (
  `id` varchar(36) NOT NULL,
  `shiftId` varchar(255) NOT NULL,
  -- O flag. Não há unique, não há check: a atomicidade vem do CAS do adapter.
  `booked` tinyint(1) NOT NULL DEFAULT '0',
  `startsAt` datetime NOT NULL,
  `endsAt` datetime NOT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `FK_tb_slots_tb_shifts` (`shiftId`),
  CONSTRAINT `FK_tb_slots_tb_shifts` FOREIGN KEY (`shiftId`) REFERENCES `tb_shifts` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- tb_appointments existe para que o mock não minta por omissão: o agendamento do
-- app legado mora aqui, e é dele que vem a concorrência que o nosso CAS enfrenta.
-- O renovi-care NUNCA escreve nesta tabela (ver o GRANT no fim).
CREATE TABLE IF NOT EXISTS `tb_appointments` (
  `id` varchar(36) NOT NULL,
  `userId` varchar(255) NOT NULL,
  `slot` varchar(255) DEFAULT NULL,
  `professionalId` varchar(255) NOT NULL,
  `title` varchar(255) NOT NULL,
  -- Observado na HML: SCHEDULED | FINISHED | CANCELED.
  `status` varchar(255) NOT NULL,
  `startsAt` datetime NOT NULL,
  `endsAt` datetime NOT NULL,
  -- O link de atendimento da DAV, como o legado já o guarda. Formato real:
  -- https://renovisaude.atendimento.hom.dav.med.br/a/{codigo}
  `url` varchar(255) DEFAULT NULL,
  `createdAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updatedAt` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `specialityId` varchar(36) DEFAULT NULL,
  `isCheckup` tinyint(1) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `FK_tb_appointments_tb_professionals` (`professionalId`),
  KEY `FK_tb_appointments_tb_specialities` (`specialityId`),
  CONSTRAINT `FK_tb_appointments_tb_professionals` FOREIGN KEY (`professionalId`) REFERENCES `tb_professionals` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `FK_tb_appointments_tb_specialities` FOREIGN KEY (`specialityId`) REFERENCES `tb_specialities` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
-- NOTA: no banco real há ainda FK de `userId` para `tb_users`, que o mock não
-- traz — `tb_users` é o cadastro de pacientes do legado, que o agendamento não
-- lê (os nossos pacientes vivem no renovi_care).

-- ---------------------------------------------------------------------------
-- Dados de exemplo
-- ---------------------------------------------------------------------------
-- Datas são RELATIVAS a CURDATE(), de propósito. Slot com data fixa apodrece: é
-- exatamente o que aconteceu com a HML de verdade, cujos slots param em
-- 2025-03-29 e não têm um único horário futuro — o que a torna inútil para
-- testar agendamento sem semear antes.

INSERT IGNORE INTO `tb_specialities` (`id`, `wanted`, `name`, `active`) VALUES
    ('11111111-1111-4111-8111-111111111111', 1, 'Psicologia', 1),
    ('22222222-2222-4222-8222-222222222222', 1, 'Psiquiatria', 1),
    -- Inativa: o adapter tem que filtrar por active = 1.
    ('33333333-3333-4333-8333-333333333333', 0, 'Especialidade Desativada', 0);

-- Os ids destes médicos são TAMBÉM o id deles na DAV — no recurso
-- /professional, não /person (sondado em 2026-07-16: GET /professional/{id} de 5
-- médicos reais devolveu 200 com o mesmo id; GET /person/{id} devolveu 204).
-- É por isso que não existe tabela de mapeamento: o participante MMD do
-- appointment é o `tb_professionals.id` direto.
--
-- Para rodar o fluxo ponta a ponta contra a DAV de homologação, estes ids
-- precisam existir lá (POST /professional com o mesmo id). Ver docs/DESENVOLVIMENTO.md.
INSERT IGNORE INTO `tb_professionals`
    (`id`, `firstName`, `lastName`, `birthday`, `email`, `cpf`,
     `licenseNumber`, `licenseRegion`, `licenseCouncil`, `rqe`, `imageUrl`) VALUES
    ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'Ana', 'Beatriz Moura',
     '1985-04-12 00:00:00', 'ana.moura@example.com', '39631974049',
     '06/123456', 'SP', 'CRP', NULL, NULL),
    ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'Bruno', 'Carvalho Lima',
     '1979-11-30 00:00:00', 'bruno.lima@example.com', '80387508376',
     '123456', 'SP', 'CRM', '54321', NULL),
    ('cccccccc-cccc-4ccc-8ccc-cccccccccccc', 'Carla', 'Dias Nogueira',
     '1990-02-08 00:00:00', 'carla.nogueira@example.com', '15350946056',
     '06/654321', 'RJ', 'CRP', NULL, NULL);

INSERT IGNORE INTO `tb_professionals_specialities` (`professionalId`, `specialityId`) VALUES
    ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', '11111111-1111-4111-8111-111111111111'),
    ('cccccccc-cccc-4ccc-8ccc-cccccccccccc', '11111111-1111-4111-8111-111111111111'),
    ('bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', '22222222-2222-4222-8222-222222222222');

-- Plantões: amanhã e depois de amanhã, 09:00-12:00.
INSERT IGNORE INTO `tb_shifts` (`id`, `professionalId`, `status`, `startsAt`, `endsAt`) VALUES
    ('a1a1a1a1-0000-4000-8000-000000000001', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '12:00:00')),
    ('a1a1a1a1-0000-4000-8000-000000000002', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '12:00:00')),
    ('b1b1b1b1-0000-4000-8000-000000000001', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '14:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '17:00:00')),
    -- Plantão que já passou: o adapter não pode ofertar horário no passado (a
    -- DAV recusa start no passado com 422 — achado #18).
    ('c1c1c1c1-0000-4000-8000-000000000001', 'cccccccc-cccc-4ccc-8ccc-cccccccccccc', 'AVAILABLE',
     TIMESTAMP(DATE_SUB(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_SUB(CURDATE(), INTERVAL 2 DAY), '12:00:00'));

-- Slots de 25 minutos (a duração mais comum no banco real: 1350 de 1957).
INSERT IGNORE INTO `tb_slots` (`id`, `shiftId`, `booked`, `startsAt`, `endsAt`) VALUES
    -- Ana, amanhã: dois livres e um já ocupado pelo app legado.
    ('50100000-0000-4000-8000-000000000001', 'a1a1a1a1-0000-4000-8000-000000000001', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '09:25:00')),
    ('50100000-0000-4000-8000-000000000002', 'a1a1a1a1-0000-4000-8000-000000000001', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '09:55:00')),
    ('50100000-0000-4000-8000-000000000003', 'a1a1a1a1-0000-4000-8000-000000000001', 1,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '10:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '10:25:00')),
    -- Ana, depois de amanhã.
    ('50100000-0000-4000-8000-000000000004', 'a1a1a1a1-0000-4000-8000-000000000002', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:25:00')),
    -- Bruno, amanhã à tarde.
    ('50200000-0000-4000-8000-000000000001', 'b1b1b1b1-0000-4000-8000-000000000001', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '14:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '14:25:00')),
    -- Carla: só tem slot no passado. Livre, mas não pode ser ofertado.
    ('50300000-0000-4000-8000-000000000001', 'c1c1c1c1-0000-4000-8000-000000000001', 0,
     TIMESTAMP(DATE_SUB(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_SUB(CURDATE(), INTERVAL 2 DAY), '09:25:00'));

-- A consulta do app legado que ocupa o slot ...0003. Existe para o teste provar
-- que o nosso CAS respeita reserva feita por OUTRO sistema.
INSERT IGNORE INTO `tb_appointments`
    (`id`, `userId`, `slot`, `professionalId`, `specialityId`, `title`, `status`, `startsAt`, `endsAt`, `url`) VALUES
    ('99999999-9999-4999-8999-999999999999', 'usuario-do-app-legado',
     '50100000-0000-4000-8000-000000000003', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
     '11111111-1111-4111-8111-111111111111', 'Consulta marcada pelo app legado', 'SCHEDULED',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '10:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 1 DAY), '10:25:00'),
     'https://renovisaude.atendimento.hom.dav.med.br/a/exemplo01');

-- ---------------------------------------------------------------------------
-- A postura de escrita, no banco e não só no código
-- ---------------------------------------------------------------------------
-- O compose cria `renovi` com todos os privilégios sobre renovi_legacy. Aqui a
-- gente rebaixa para exatamente o que o ADR-004 permite: ler tudo e escrever DUAS
-- colunas de UMA tabela.
--
-- É o mesmo espírito do CHECK `active_exige_vinculo_dav` (0002_auth): regra que
-- importa mora no banco, não na boa memória de quem escreve o próximo model. Com
-- isto, um INSERT em tb_appointments não é "algo que combinamos não fazer" — é
-- algo que o banco recusa. E o dev descobre na hora, não em produção.
--
-- Em produção o DBA do legado aplica o equivalente para o usuário do renovi-care.
-- O seed acima e os testes de integração rodam como root, então isto não os
-- atrapalha: quem fica restrito é só o usuário da APLICAÇÃO.
--
-- Recriamos o usuário em vez de dar REVOKE no que o entrypoint criou. Motivo
-- medido, não estético: o entrypoint do mysql:8 concede em `renovi\_legacy`.*
-- (escapa o `_`, que é curinga em nome de banco no GRANT), então
-- `REVOKE ... ON `renovi_legacy`.*` não casa com a concessão e falha com
-- "ERROR 1141: There is no such grant defined". DROP + CREATE não depende de
-- adivinhar a forma da concessão dele.
--
-- E isto aqui é código de infraestrutura, não decoração: erro em script de
-- initdb.d DERRUBA O CONTAINER. As duas primeiras versões deste bloco fizeram o
-- `make up` parar de subir — a segunda porque concessão por COLUNA exige que o
-- nome do banco RESOLVA (com `\_` ele vira o literal `renovi\_legacy`, que não
-- existe: "ERROR 1049 Unknown database"). Por isso o nome vai simples aqui.
DROP USER IF EXISTS 'renovi'@'%';
CREATE USER 'renovi'@'%' IDENTIFIED BY 'renovi';
GRANT SELECT ON `renovi_legacy`.* TO 'renovi'@'%';
GRANT UPDATE (`booked`, `updatedAt`) ON `renovi_legacy`.`tb_slots` TO 'renovi'@'%';
FLUSH PRIVILEGES;
