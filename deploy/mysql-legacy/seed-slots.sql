-- Seed IDEMPOTENTE de horários FUTUROS no mock do legado (MySQL), para rodar o
-- cenário-alvo do Slice 1 MANUALMENTE via `apps/api/docs/slice1.http`.
--
-- Para que serve
-- ---------------------------------------------------------------------------
-- O `deploy/mysql-legacy/init.sql` só semeia plantões de amanhã/depois de amanhã
-- (um deles já no passado) — o bastante para o adapter subir, mas não para
-- percorrer o roteiro inteiro do slice (QUOTA de até 4/mês, MIN_INTERVAL de 7d,
-- MAX_ADVANCE de 30d, vigência de matrícula de 30d). Este script acrescenta os
-- horários futuros que o roteiro (docs/slice1.http) espera, nos MESMOS offsets
-- que `apps/api/internal/e2e` usa para os testes automatizados
-- (`testsupport.SeedFutureSlots`) — só que aqui os dados sobrevivem ao mock
-- PERSISTENTE do `docker compose`, para uma sessão manual de verdade (com a DAV
-- de homologação por trás).
--
-- Como rodar
-- ---------------------------------------------------------------------------
--   make seed-legacy-slots
--
-- (ou, na mão: `docker exec -i renovi-mysql-legacy mysql -uroot -proot
-- renovi_legacy < deploy/mysql-legacy/seed-slots.sql`). Precisa do `make up` já
-- de pé. As datas são RELATIVAS a CURDATE(), então rode de novo sempre que os
-- offsets tiverem "andado" (ex.: depois de um fim de semana parado) — rodar no
-- mesmo dia não duplica nada (ver "Idempotência" abaixo).
--
-- Idempotência
-- ---------------------------------------------------------------------------
-- Todo INSERT é `INSERT IGNORE` e os ids são determinísticos por (profissional,
-- offset, slot): rodar de NOVO no mesmo dia não duplica nada — a chave primária
-- (`id`) já existe e a segunda inserção é ignorada, silenciosa. Isto é verificado
-- mecanicamente pelo teste de integração `TestSeedLegacySlotsIsIdempotent`
-- (`apps/api/internal/testsupport/seed_slots_test.go`), que sobe um MySQL
-- efêmero, executa este arquivo duas vezes seguidas e confere que a contagem de
-- slots livres futuros por profissional não muda. Rode com `make
-- test-integration`.
--
-- Rodar em um dia DIFERENTE do anterior acrescenta um conjunto NOVO de linhas
-- (o offset agora aponta para outra data, logo os ids mudam) — ainda seguro, só
-- acumula mais horários futuros; nada colide.
--
-- Ids
-- ---------------------------------------------------------------------------
-- Prefixo `manual-a-<offset>-*` (Ana) / `manual-b-<offset>-*` (Bruno), para
-- nunca colidir com o `init.sql` (uuids fixos, ex. `a1a1a1a1-...`) nem com
-- `testsupport.SeedFutureSlots` (prefixo `e2e-`, usado pelos testes
-- automatizados contra um MySQL efêmero). Todos ≤ 20 caracteres — bem dentro do
-- limite de `tb_shifts.id`/`tb_slots.id` (varchar(36)).
--
-- Profissionais (já existem via init.sql; ver docs/DESENVOLVIMENTO.md para
-- recriá-los na DAV de homologação, só se ela resetar):
--   Ana Beatriz Moura   (aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa) — Psicologia
--   Bruno Carvalho Lima (bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb) — Psiquiatria
--
-- Por offset: 1 plantão AVAILABLE 09:00–12:00 + 2 slots de 25min (09:00 e
-- 09:30), `booked=0`. Datas via DATE_ADD(CURDATE(), INTERVAL n DAY) — DATETIME
-- ingênuo interpretado como hora de parede de America/Sao_Paulo, mesmo padrão
-- do init.sql (ver o comentário "Fuso" lá).

-- ---------------------------------------------------------------------------
-- Ana (Psicologia) — offsets +2, +9, +10, +16, +23, +30, +44
-- ---------------------------------------------------------------------------
INSERT IGNORE INTO `tb_shifts` (`id`, `professionalId`, `status`, `startsAt`, `endsAt`) VALUES
    ('manual-a-2-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '12:00:00')),
    ('manual-a-9-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '12:00:00')),
    ('manual-a-10-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '12:00:00')),
    ('manual-a-16-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '12:00:00')),
    ('manual-a-23-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '12:00:00')),
    ('manual-a-30-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '12:00:00')),
    ('manual-a-44-shift', 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '12:00:00'));

INSERT IGNORE INTO `tb_slots` (`id`, `shiftId`, `booked`, `startsAt`, `endsAt`) VALUES
    ('manual-a-2-s1', 'manual-a-2-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:25:00')),
    ('manual-a-2-s2', 'manual-a-2-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 2 DAY), '09:55:00')),
    ('manual-a-9-s1', 'manual-a-9-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '09:25:00')),
    ('manual-a-9-s2', 'manual-a-9-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 9 DAY), '09:55:00')),
    ('manual-a-10-s1', 'manual-a-10-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '09:25:00')),
    ('manual-a-10-s2', 'manual-a-10-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 10 DAY), '09:55:00')),
    ('manual-a-16-s1', 'manual-a-16-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '09:25:00')),
    ('manual-a-16-s2', 'manual-a-16-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 16 DAY), '09:55:00')),
    ('manual-a-23-s1', 'manual-a-23-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '09:25:00')),
    ('manual-a-23-s2', 'manual-a-23-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 23 DAY), '09:55:00')),
    ('manual-a-30-s1', 'manual-a-30-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '09:25:00')),
    ('manual-a-30-s2', 'manual-a-30-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 30 DAY), '09:55:00')),
    ('manual-a-44-s1', 'manual-a-44-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '09:25:00')),
    ('manual-a-44-s2', 'manual-a-44-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 44 DAY), '09:55:00'));

-- ---------------------------------------------------------------------------
-- Bruno (Psiquiatria) — offsets +5, +37, +68
-- ---------------------------------------------------------------------------
INSERT IGNORE INTO `tb_shifts` (`id`, `professionalId`, `status`, `startsAt`, `endsAt`) VALUES
    ('manual-b-5-shift', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '12:00:00')),
    ('manual-b-37-shift', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '12:00:00')),
    ('manual-b-68-shift', 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb', 'AVAILABLE',
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '12:00:00'));

INSERT IGNORE INTO `tb_slots` (`id`, `shiftId`, `booked`, `startsAt`, `endsAt`) VALUES
    ('manual-b-5-s1', 'manual-b-5-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '09:25:00')),
    ('manual-b-5-s2', 'manual-b-5-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 5 DAY), '09:55:00')),
    ('manual-b-37-s1', 'manual-b-37-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '09:25:00')),
    ('manual-b-37-s2', 'manual-b-37-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 37 DAY), '09:55:00')),
    ('manual-b-68-s1', 'manual-b-68-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '09:00:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '09:25:00')),
    ('manual-b-68-s2', 'manual-b-68-shift', 0,
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '09:30:00'),
     TIMESTAMP(DATE_ADD(CURDATE(), INTERVAL 68 DAY), '09:55:00'));
