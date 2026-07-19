-- 0014_crisis_routing — fatos de roteamento de crise/escalonamento (Anexo C.5.5).
--
-- Dois novos tipos de evento na jornada, ambos roteados ao canal CLÍNICO/urgência,
-- NUNCA ao gestor (guardrails 6.1/6.5):
--   pedido_ajuda        — afordância "preciso de ajuda agora" (triagem, não tratamento)
--   escalonamento_clinico — rastreio positivo (WHO-5 <28 / PHQ-4 subescala ≥ corte)
-- É só estender o CHECK de event_type (append-only, imposto pela role do 0008).

ALTER TABLE journey_event DROP CONSTRAINT journey_event_event_type_check;
ALTER TABLE journey_event ADD CONSTRAINT journey_event_event_type_check CHECK (event_type IN (
    'matricula_criada', 'matricula_renovada', 'matricula_expirada', 'matricula_encerrada',
    'consulta_agendada', 'consulta_cancelada', 'consulta_status_forcado',
    'checkin_humor_registrado', 'assessment_respondido',
    'pedido_ajuda', 'escalonamento_clinico'
));
