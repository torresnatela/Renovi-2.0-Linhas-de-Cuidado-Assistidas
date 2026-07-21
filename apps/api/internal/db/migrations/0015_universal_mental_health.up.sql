-- 0015_universal_mental_health — Verificador de Humor para TODOS (Degrau 1).
--
-- Até aqui o Verificador de Humor (GRID diário, WHO-5 semanal, PHQ-4 gatilhado) só
-- funcionava para quem tinha matrícula ativa numa linha de cuidado que contivesse os
-- itens de atividade (Degrau 2). Esta migration entrega o Degrau 1 — saúde mental
-- disponível independentemente de plano — SEM desacoplar o modelo de dados: em vez
-- disso, semeia uma LINHA DE CUIDADO UNIVERSAL (`saude-mental-aberta`) que carrega os
-- três itens ATIVIDADE, e matricula todo colaborador nela (contas novas: hook em
-- AccountStore.commitLink; contas existentes: o backfill ao final deste arquivo).
--
-- Decisões (ver ADR-040):
--   * Vigência PERPÉTUA via `valid_until` distante (2999-12-31), NÃO 'infinity' — pgx v5
--     não faz scan de infinity em time.Time, e o valid_until entra no motor puro. Com
--     data-sentinela o motor `vigenciaBlock` nunca bloqueia e a expiração lazy nunca
--     dispara. A linha universal é excluída da listagem da jornada no código (repo), então
--     não vira "plano" no perfil.
--   * `checkin-humor-diario` NÃO leva regra: o "1 por dia" é imposto pelo upsert em
--     (patient_id, dia_ref) do mood_checkin, não pelo motor. WHO-5/PHQ-4 levam MIN_INTERVAL.
--   * Seed direto (como o 0011), não pela API admin: uma linha só de ATIVIDADE não precisa
--     de validação de especialidade (legado), indisponível numa migration.
-- Convenção: IDs fixos para os "pais" (linha + itens, referenciados abaixo);
-- gen_random_uuid() para regras e para as linhas do backfill (mesma exceção do 0011).

-- (1) A linha de cuidado universal, já publicada.
INSERT INTO care_line (id, code, version, name, description, status, published_at) VALUES
  ('15000000-0000-4000-8000-000000000001', 'saude-mental-aberta', 1,
   'Saúde mental (aberto)',
   'Verificador de humor disponível a todos os colaboradores, com ou sem plano.',
   'published', now());

-- (2) Os três itens ATIVIDADE. specialty_code fica NULL (CHECK specialty_por_kind, 0009).
--     Os refs DEVEM casar com as constantes Go (mood_checkin.go, assessment.go).
INSERT INTO care_line_item (id, care_line_id, ref, kind, label, sort_order) VALUES
  ('15000000-0000-4000-8000-000000000011', '15000000-0000-4000-8000-000000000001',
   'checkin-humor-diario', 'ATIVIDADE', 'Check-in de humor', 1),
  ('15000000-0000-4000-8000-000000000012', '15000000-0000-4000-8000-000000000001',
   'who5-semanal', 'ATIVIDADE', 'Índice de bem-estar (WHO-5)', 2),
  ('15000000-0000-4000-8000-000000000013', '15000000-0000-4000-8000-000000000001',
   'phq4-gatilhado', 'ATIVIDADE', 'Rastreio de humor (PHQ-4)', 3);

-- (3) Cadência dos anéis periódicos (o motor lê MIN_INTERVAL sobre o histórico imutável).
--     O anel diário NÃO tem regra (upsert por dia_ref cuida do "1 por dia").
INSERT INTO care_line_rule (id, care_line_item_id, rule_type, params) VALUES
  (gen_random_uuid(), '15000000-0000-4000-8000-000000000012', 'MIN_INTERVAL', '{"days":7}'),
  (gen_random_uuid(), '15000000-0000-4000-8000-000000000013', 'MIN_INTERVAL', '{"days":14}');

-- (4) Backfill: matricula toda conta ACTIVE ainda sem matrícula viva na linha universal.
--     valid_until = sentinela distante (perpétua). Roda como owner (migrate), então pode
--     inserir em journey_event (append-only só vale para o role renovi_app, ADR-024/0008).
INSERT INTO enrollment (id, patient_id, care_line_id, care_line_code, status, valid_from, valid_until)
SELECT gen_random_uuid(), a.id, '15000000-0000-4000-8000-000000000001', 'saude-mental-aberta',
       'ativa', now(), TIMESTAMPTZ '2999-12-31 00:00:00+00'
FROM patient_account a
WHERE a.status = 'ACTIVE'
  AND NOT EXISTS (
    SELECT 1 FROM enrollment e
    WHERE e.patient_id = a.id
      AND e.care_line_code = 'saude-mental-aberta'
      AND e.status IN ('ativa', 'pausada')
  );

-- Um período de vigência por matrícula recém-criada (source usa o default 'admin').
INSERT INTO enrollment_period (id, enrollment_id, starts_at, ends_at)
SELECT gen_random_uuid(), e.id, e.valid_from, e.valid_until
FROM enrollment e
WHERE e.care_line_code = 'saude-mental-aberta'
  AND NOT EXISTS (SELECT 1 FROM enrollment_period p WHERE p.enrollment_id = e.id);

-- O fato matricula_criada na jornada (actor=sistema: foi automática, não um admin).
INSERT INTO journey_event (id, enrollment_id, patient_id, event_type, actor, ref_table, ref_id, payload)
SELECT gen_random_uuid(), e.id, e.patient_id, 'matricula_criada', 'sistema',
       'enrollment', e.id, jsonb_build_object('valid_until', e.valid_until, 'source', 'backfill')
FROM enrollment e
WHERE e.care_line_code = 'saude-mental-aberta'
  AND NOT EXISTS (
    SELECT 1 FROM journey_event je
    WHERE je.enrollment_id = e.id AND je.event_type = 'matricula_criada'
  );
