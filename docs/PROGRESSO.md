# Progresso — estado vivo do projeto

> **Todo agente atualiza este arquivo ao avançar.** É a fonte da verdade de "onde
> estamos". Marque `[x]` o que concluiu e ajuste "Próximo passo".

_Última atualização: 2026-07-20 — **Revisão do CodeRabbit na PR #13** endereçada (ADR-037, 2ª rodada): cortes na conexão da tx, streak recente, locks de consentimento (Grant/Revoke/Record), a11y (aria-live + aria-pressed), clamp de limit, `maxItems: 120` no contrato, e fix de teste flaky (meia-noite BR). Não aplicado: `NOT VALID` nas migrations (golang-migrate roda em tx única; desproporcional no piloto). `make ci` + integração completa (incl. e2e) + front verdes.
Antes: 2026-07-19 — **Correções pós-review (PR #13)** do Verificador de Humor: streak = dias consecutivos de calendário (gatilho), guard de concorrência (advisory lock + rechecagem na tx) no Submit do assessment, e nits (teto do History em 120, `limit` no `getMoodHistory`, grade operável por teclado). `make ci` + integração verdes. Ver ADR-037.
Antes: 2026-07-19 — **Verificador Diário de Humor (Anexo C): completo (Módulos 0–6 + telas de WHO-5/PHQ-4)**, pronto para merge na main, sobre o Slice 1 concluído._

## 🌡️ Verificador Diário de Humor — Anexo C (em andamento, branch `feat/verificador-humor`)

Check-in emocional contínuo como **atividade** da linha de cuidado (3 anéis:
diário → WHO-5 semanal → PHQ-4 gatilhado). Só Degrau 2 (exige matrícula ativa).
Execução em loops orientados a `/goal` (plano aprovado). Ramo do Slice 1.

- [x] **Módulo 0 — item ATIVIDADE na fundação** (ADR-030): migration
  `0009_activity_item` (kind `IN ('CONSULTA','ATIVIDADE')`, `specialty_code`
  condicional ao kind), `careline_catalog.AddItem` aceita ATIVIDADE,
  `ValidatePublish` pula especialidade para atividade. Unit + integração verdes
  (fluxo de consulta sem regressão).
- [x] **Módulo 1 — Consentimento** (ADR-031): migration `0010_consent`
  (índice parcial `ux_consent_ativo` = um ativo por paciente+finalidade), model
  `ConsentStore` (Grant idempotente por termo, reconcessão versionada, Revoke,
  `Active`), controller + rotas `/me/consent` (GET/POST/revoke) sob `RequireSession`.
  Testes de controller (fakes) + integração verdes.
- [x] **Módulo 2 — Instrumentos + pontuação pura** (ADR-032): pacote PURO
  `models/mood/scoring` (`Quadrant`/`IsQuadranteRisco`, `ScoreWHO5`, `ScorePHQ4`,
  cortes por parâmetro) table-driven; migration `0011_instrument`
  (instrument/dimension/cutoff + lookups) com seed dos 3 instrumentos e cortes BR;
  `InstrumentStore.Config` + `GET /me/mood/instruments/{codigo}`. Unit + controller
  + integração verdes.
- [x] **Módulo 3 — Anel diário (grade) ponta a ponta** (ADR-033): migration
  `0012_mood_checkin` (dia_ref local, sem comentario), `MoodCheckinStore`
  (pré-condições derivadas, upsert do dia, fato na jornada), rotas
  `POST /me/mood/checkin`, `GET /me/mood/today`, `/history`. Front
  `apps/web/src/features/mood/` (grade valência×energia + fluxo de consentimento,
  paleta própria da Renovi). Verificado: scoring/model (integração), controllers
  (fakes), front (Vitest) + typecheck + build. **Browser: pendente** — precisa do
  stack de dev com credenciais DAV (rotas /me só montam com Auth).
- [x] **Módulo 4 — Anel semanal WHO-5 via MIN_INTERVAL** (ADR-034):
  `0013_wellbeing_assessment` + `assessment_item_response`. `AssessmentStore`
  **reusa `careline.Evaluate`** montando a `Journey` com os fatos de atividade
  (Status=realizada, ScheduledAt=respondido_em) — cadência derivada sob demanda,
  sem tocar T1–T19. Pontuação WHO-5 com cortes do banco. `GET /me/assessments/{codigo}`,
  `POST /me/assessments`. Integração (cadência 7d) + controller (409 blocks, 403) verdes.
- [x] **Módulo 5 — Anel gatilhado PHQ-4 + gatilho puro** (ADR-035): pacote PURO
  `models/mood/trigger` (máquina de estados C.5.4, `N=4` default) table-driven;
  PHQ-4 no `AssessmentStore.score` (subescalas PHQ-2/GAD-2, cortes do banco);
  wiring do gatilho no `MoodCheckinStore.Today` (oferta `offer` + `escalate`
  derivados do histórico). Integração do caminho completo NORMAL→WHO5→PHQ4→ESCALAR verde.
- [x] **Módulo 6 — Crise/escalonamento + fechamento** (ADR-036): `0014_crisis_routing`
  (event_types `pedido_ajuda`, `escalonamento_clinico`). `POST /me/mood/help-now`
  (canal de urgência + registro na jornada); rastreio positivo emite
  `escalonamento_clinico` (actor=sistema) → trilha CLÍNICA, nunca gestor. Integração
  (help-now + escalonamento + muralha) verde.

**Próximos passos do Verificador de Humor** (fora do backend entregue):
- **Verificação no browser** do fluxo (precisa do stack de dev com credenciais DAV;
  rotas `/me/*` só montam com Auth).
- [x] **Front dos anéis periódicos** (WHO-5/PHQ-4) + oferta do gatilho, escalonamento
  e "preciso de ajuda agora" na `/humor` (`AssessmentForm` + `MoodPage`).
- **Comentário livre cifrado** (adiado — exige pacote de cifra em repouso).
- **Degrau 1** (termômetro populacional fora de linha de cuidado) — fork adiado.
- **Integração real da trilha clínica** e **worker de retenção** (hoje o escalonamento
  grava o fato/flag; o roteamento efetivo entra quando a trilha existir).
- **Camada agregada/gestor** (índice coletivo, k-anonimato) — documento próprio (C.8).

## ✅ Slice 1 — Linhas de Cuidado Assistidas (CONCLUÍDO)

O colaborador vê a linha de cuidado, o motor decide o que pode agendar, ele agenda
pelo booking existente e a jornada fica auditada. Ativação (Gestão), preço/billing
e o worker de auto-conclusão ficam **fora** deste slice.

- [x] **Motor de regras puro** (`models/careline/`, ADR-020): `Evaluate` (VIGENCIA,
  QUOTA em **janela móvel GERAL**, MIN_INTERVAL, MAX_ADVANCE, PREREQUISITE),
  `ParseRuleParams` (tipado, `DisallowUnknownFields`) e `ValidatePublish`. A tabela
  **T1–T19** em `evaluate_test.go` é a especificação normativa do slice.
- [x] **Catálogo versionado via API + publish validado** (migration 0005,
  `careline_catalog.go`, rotas `/admin/care-lines*`): draft → itens → regras →
  publish que valida ciclos de pré-requisito e as especialidades contra o legado
  (legado indisponível = 503). Versão publicada é **imutável** (novo item/regra em
  linha publicada = 409).
- [x] **Matrícula** (migration 0006, `enrollment.go`, `/admin/enrollments*`):
  vigência por período, **renovação contígua** (o novo período emenda no fim do
  atual), **reativação** de uma matrícula expirada (a partir de agora) e
  **expiração LAZY** — toda leitura da jornada expira a matrícula vencida na hora,
  com evento `matricula_expirada`, sem depender de cron.
- [x] **Jornada do paciente `/me/*`** (ADR-021, `care_journey.go` +
  `care_journey_repo.go`): `/me/journey`, `/me/eligibility`, `/me/availability`
  (slots dos profissionais da especialidade anotados com o veredito por instante),
  `POST /me/appointments` com **elegibilidade reavaliada no servidor** (motor ANTES
  do booking → 422 `blocks[]`), **idempotência por `Idempotency-Key`** (replay 200,
  corrida compensa o booking — ADR-025), cancel com bookkeeping de cota e
  `/me/audit` (keyset por cursor opaco). Toda escrita grava **linha + evento
  append-only na mesma TX**; `journey_event` é append-only por PRIVILÉGIO de banco
  (role `renovi_app`, ADR-024).
- [x] **Cancelamento best-effort na DAV**: `BookingStore.Cancel` marca CANCELLED,
  devolve o horário ao legado (CAS) e tenta o cancel na DAV — que responde **500 em
  HML** (achado #20), tolerado e auditado.
- [x] **E2E de integração** (`internal/e2e/`, tag `integration`, **39 passos em 3
  cenários**): sobe a API real contra Postgres + MySQL (testcontainers, role
  restrito `renovi_app`) e uma DAV fake (cancel sempre 500). `TestE2E_A_SaudeMentalBasica`
  (23 passos: publish/validação, cota/intervalo/antecedência/vigência, cancelamento
  com devolução de vaga, renovação contígua, expiração lazy + reativação, auditoria
  paginada), `TestE2E_B_ApoioPsicologico` (6 passos: QUOTA `period:total` =
  bloqueio permanente sem `available_from`) e `TestE2E_C_SaudeMentalSemanal`
  (10 passos, 2026-07-19: os casos de uso do marco "linha semanal" — psico QUOTA
  1/semana + psiq QUOTA 1/mês, matrícula de 1 mês. UC1 ativação, UC2 as 4
  semanais a exatos 7d + 1 psiq, UC3 nada mais agendável — QUOTA na vigência,
  VIGENCIA além, disponibilidade anotada 100% bloqueada —, UC4 mesma semana em
  qualquer horário = 422, UC5 cancela uma semana e reagenda nela, UC6 psiq
  remarca para antes E para depois; extras: idempotência do reagendamento,
  cancel duplo 409, 404, 401 e auditoria com 12 eventos. O teste foi validado
  por mutação: janela da QUOTA `<` → `<=` derruba o C04).
- [x] **Percurso ao vivo contra a DAV de homologação** (2026-07-18): o cenário-alvo
  rodado ponta a ponta pela API real. Comprovado: **2 estouros reais de 29s** no
  teto do gateway → **502 fail-closed** (o horário fica retido, nenhuma consulta
  fantasma é solta — ADR-016); **cancel da DAV em 500 tolerado e auditado**;
  **renovação antecipada liberando o ciclo 2** ao vivo.
- [x] **Front de teste** (`apps/web/src/features/journey/`, 3 telas): Minha Jornada,
  Agendar (Idempotency-Key nascida com a intenção) e Minhas consultas. Estilo
  mínimo — o design vem depois.
- [x] **Ambiente manual**: `apps/api/docs/slice1.http` (espelha os cenários A/B),
  `deploy/mysql-legacy/seed-slots.sql` + `make seed-legacy-slots` (idempotente) e
  `apps/api/docs/SLICE1.md` (README operacional do slice).

### 🔧 Correções pós-review (2026-07-19)

Achados de review corrigidos, cada um com teste (unit/integração/web). `make ci` e
a integração completa (E2E A/B/C) verdes; front 47/47 + typecheck.

- [x] **Renovação de matrícula vencida sem status expirado** (`enrollment.go`): o
  admin renovava o que venceu antes de a expiração lazy rodar e o período novo caía
  no passado; `Renew` agora reativa a partir de `now` quando `valid_until <= now`
  (ADR-021, `TestRenew_MatriculaVencidaSemMarcarExpirada_ReativaDeNow`).
- [x] **PREREQUISITE com limite superior** (`careline/evaluate.go`): consulta futura
  não satisfaz mais "realize primeiro" (`X04`).
- [x] **`available_from` de QUOTA super-lotada** (`careline/evaluate.go`): usa a
  `(n-max+1)`-ésima consulta da janela, não a mais antiga (`X05`; validado pela
  mutação da janela do cenário C).
- [x] **`Idempotency-Key` vinculada ao item** (`care_journey.go`): reúso para outro
  item vira `422 IDEMPOTENCY_KEY_REUSE` em vez de replay errado (ADR-025).
- [x] **Detach do ctx pós-`Book`** (`care_journey.go`): desconexão do cliente não
  deixa o booking órfão (projeção + compensação em `context.WithoutCancel`; ADR-021).
- [x] **Front sem POST concorrente** (`ScheduleCarePage.tsx`): todos os botões
  Agendar travam enquanto um agendamento está em voo.

Deixados fora (decisão de escopo, não bug de código): corrida de cota entre keys
diferentes (LIMITAÇÃO ACEITA, ADR-021); `RENOVI_ENV` fail-open (decisão de ops);
defesa em profundidade do append-only e demais itens que exigem migration nova.

### 🔴 Pendências e riscos conhecidos do Slice 1

- **`cmd/worker` continua stub** — e agora com uma lacuna NOVA: um `CANCELLED` cujo
  `ReleaseSlot` no legado falhou fica com o slot retido e **não** entra na fila que
  o worker varre (`ListPendingSlotRelease` só varre `FAILED`). Órfão até o worker
  do Slice 2 cobrir `CANCELLED`+held+not-released (ADR-023).
- **Expiração de matrícula é LAZY**: só acontece quando alguém lê a JORNADA do
  paciente. A renovação já não depende disso (decide por tempo — ver Correções
  pós-review); resta que as leituras ADMIN (`EnrollmentStore.Get`, dashboard) ainda
  mostram `ativa` para uma vigência vencida até uma leitura da jornada rodar. O cron
  do Slice 2 vira otimização, não requisito de corretude (ADR-021).
- **Admin por token estático, sem rotação nem auditoria de operador** (ADR-022):
  revisar quando houver back-office.
- **DAV HML instável no limite dos 29s** (comprovado ao vivo): o `POST /appointment`
  estoura o teto do gateway na cauda. O fail-closed cobre, mas é operacionalmente
  ruim — mesmo chamado já aberto na DAV (ADR-016).

### ⏭️ Próximo passo (fora deste slice)

**Worker** (compensação + reconciliação, INCLUINDO a varredura de `CANCELLED` com
slot retido) + **ativação via Gestão** (Adapter de leitura) + **preço/billing** da
matrícula.

## ✅ Agendamento — CONCLUÍDO (backend + front)

Fluxo: especialidade → profissional → horário → consulta na DAV → link de entrada.
**Sem** motor de elegibilidade e sem linha de cuidado (decisão de escopo).

- Sondagem da DAV para agendamento (`make dav-probe`) — 9 achados novos (#12 a
  #20) em `docs/DAV-API-NOTAS.md`. Derrubou mais 3 afirmações do spec deles.
- `deploy/mysql-legacy/init.sql` com o **schema REAL** do legado (o anterior era
  inventado e mentia na trava de double-booking).
- Contrato: `/specialties`, `/specialties/{id}/professionals`,
  `/professionals/{id}/slots`, `/appointments` (GET/POST), `/appointments/{id}`,
  `/appointments/{id}/join`.
- `models/scheduling` — janela de entrada, pacote **puro**, table-driven.
- `adapters/agenda` — MySQL legado à mão (ADR-018), CAS de reserva, fuso forçado.
- `adapters/dav` — `CreateAppointment`, uma tentativa, `ErrMaybeApplied`.
- Migration `0003_scheduling` + saga em `models/appointment.go`.
- Controllers + rotas atrás de `RequireSession`, com timeout próprio no POST.
- Front (`apps/web/src/features/scheduling/`): wizard, minhas consultas e entrar
  na consulta. `shared/datetime` (fuso obrigatório) e `shared/navigate`.

**Verificado ponta a ponta NO BROWSER** (Chrome + API real + mock do legado + DAV
de homologação): cadastro → login → especialidade → profissional → horário (09:00
do legado saindo como `09:00-03:00` na tela) → **agendamento 201 CONFIRMED** →
minhas consultas → entrar → **a sala da Doutor ao Vivo abriu**.

Rodar de verdade encontrou dois bugs que os testes não pegavam, os dois do mesmo
tipo — contrato e mock concordando entre si e discordando da realidade:
`SlotPage` sem o profissional (tela em branco) e `Appointment.professional`
prometendo um registro no conselho que a consulta não guarda.

### 🔴 Pendências e riscos conhecidos do agendamento

- **`cmd/worker` continua stub.** A saga já grava a fila de compensação
  (`FAILED` + `slot_held_at` + `slot_released_at IS NULL`) e a de revisão
  (`DAV_UNKNOWN`), mas **ninguém as varre ainda**. Enquanto isso, um horário que a
  compensação não conseguiu devolver fica retido até alguém olhar.
- **Não há reconciliação possível** (ADR-016): a DAV recusa id nosso no
  appointment e não tem rota de busca. Um 504 deixa a consulta possivelmente
  criada e inalcançável. **Chamado a abrir na DAV**, com os `trace` que temos:
  (1) aceitar id do integrador no `POST /appointment`, (2) o `cancel` devolvendo
  500, (3) o `GET` devolvendo 500 em vez de 204 para id inexistente.
- **A DAV de homologação está oscilando muito**: o `POST /appointment` mediu 3,3s
  e 10,5s de média em sondagens do MESMO dia, com máximo de 17,2s — e no primeiro
  agendamento real estourou os 29s do gateway. Não é cauda rara.
- **Profissional criado por POST que estourou parece ficar quebrado na DAV**: todo
  agendamento com ele como MMD estoura o gateway. Reproduzido duas vezes. Merece
  entrar no mesmo chamado.
- **Cancelar consulta ainda não existe** no nosso produto — e nem poderia hoje: o
  `cancel` da DAV responde 500.
- **A escrita no legado precisa de acordo com quem o opera** (ADR-014): o médico
  não vê a consulta na agenda do Renovi legado, só pela DAV.


## ✅ Autenticação — cadastro, login e vínculo com a Doutor ao Vivo (CONCLUÍDA)

Primeira feature de negócio. Ver `docs/DAV-API-NOTAS.md` (comportamento real da DAV)
e ADR-010 a ADR-013 em `docs/DECISOES.md`.

- [x] **Sondagem da API da DAV** (`make dav-probe`) — 10 achados contra a HML, com
      evidência. Derrubou 3 afirmações do spec publicado deles.
- [x] Contrato: `/auth/register`, `/auth/login`, `/auth/logout`, `/me`; `bearerAuth` → `cookieAuth`
- [x] Pacotes puros: `models/cpf` (DV) e `models/credential` (Argon2id)
- [x] `internal/adapters/dav` — client com retry, mapeamento de erro por `i18n.phrase`
- [x] Migration `0002_auth` — `patient_account`, `patient_identity` (CPF isolado),
      `patient_address`, `session`, `dav_link_audit`
- [x] `models.AccountStore.Register` (2 TX curtas + DAV no meio) e `models.SessionStore`
- [x] Controllers `/auth/*` + `/me`, `RequireSession`, rate limit por IP, `config.LogValue`
- [x] Front: `react-router-dom`, telas de cadastro e login, `useSession`, `ProtectedRoute`
- [x] **Verificado ponta a ponta contra a DAV de homologação**: cadastro real → pessoa
      criada lá com o nosso UUIDv7 → login → `/me` → logout revoga a sessão.

### ⚠️ Pendências conhecidas desta feature

- [ ] **Fator de posse (WhatsApp/e-mail)** — sem ele o cadastro é confiado e um CPF
      alheio dá acesso ao prontuário daquela pessoa. **ADR-013, revisar antes do go-live.**
- [ ] **E-mail único na DAV** (achado #6): casal/família que compartilha e-mail não
      consegue cadastrar o segundo. Há mensagem própria na UI, mas é decisão de produto.
- [ ] **Divergência de dados com a DAV**: numa reconciliação (retry após 504), o
      cadastro de lá fica com os dados da tentativa anterior. Não sincronizamos — no
      caminho `ATTACHED` isso sobrescreveria dados de terceiro.
- [ ] Reset de senha; 2FA; lockout progressivo por conta (as colunas
      `failed_login_count`/`locked_until` já existem, a lógica não).
- [ ] Rate limit é **em memória**: só serve para instância única. Escalou → Redis.

## ✅ Sprint 0 — Fundação (CONCLUÍDA)

- [x] Monorepo (apps/api, apps/web, packages, deploy, docs)
- [x] Backend Go em MVC (config, http+chi, controllers, models, db)
- [x] Motor de elegibilidade — **contrato/tipos reservados** (puro, sem implementação)
- [x] PostgreSQL renovi_care + migrations (golang-migrate embutido) + tabela-exemplo
- [x] sqlc configurado e gerando (`internal/db/gen`)
- [x] OpenAPI inicial (`packages/contracts/openapi.yaml`) + oapi-codegen gerando tipos
- [x] Docker Compose (Postgres + mocks de MySQL legado e Postgres Gestão)
- [x] Testes: unitários (config, http, controllers, eligibility smoke) + integração (testcontainers)
- [x] Front React+Vite+TS+Tailwind+TanStack Query + Vitest (badge de saúde da API)
- [x] CI **configurado** (lint → generate-check → test → build, para api e web) — workflow escrito; ainda não executado em runner (repo sem remoto/commits)
- [x] Docs para Claude Code (este PROGRESSO, ARQUITETURA, DESENVOLVIMENTO, DECISOES, CLAUDE.md)

## ⏳ Próximo passo

**`cmd/worker`** — hoje stub, e a saga já produz as filas que ele deveria varrer
(compensação e revisão). O Slice 1 acrescentou uma fila NOVA: `CANCELLED` com slot
retido que o `ReleaseSlot` não devolveu (ADR-023). Sem o worker, esse horário fica
preso até alguém olhar. Depois dele: **ativação via Gestão** e **preço/billing** da
matrícula.

_(O wiring do motor de linhas de cuidado — schema, queries, controllers `/me/*` que
expõem os `Blocks` — foi CONCLUÍDO no Slice 1; ver a seção acima.)_

## 🗺️ Backlog por fase (resumo — detalhe no SPEC §11)

### P0 (MVP) — linha de cuidado + agendamento
- [x] Schema real de domínio (care_line/item/rule, enrollment + período, care_appointment, journey_event, idempotency por coluna UNIQUE — migrations 0005–0008; patient_account/appointment já vinham da auth/agendamento)
- [x] Motor de elegibilidade implementado + `GET /me/eligibility` (Slice 1)
- [ ] Ativação de conta (token por CPF/e-mail) + Adapter Gestão (leitura)
- [x] Adapter Agenda (legado): leitura de slots + reserva por CAS (ADR-015)
- [x] Fluxo de agendamento distribuído + Adapter DAV — **sem** reconciliação: ela é impossível hoje (ADR-016)
- [ ] Auto-conclusão (cron) — jornada avançando já funciona (eventos + expiração
  lazy); o `realizada`/`falta` só existe hoje pela rota de teste
  `force-status` (Slice 1). O cron real fica no worker (Slice 2).
- [x] Catálogo montado 100% pela admin API (`/admin/care-lines*`, ADR-022): não há `cmd/seed` — ele foi removido na Fase 0. As linhas do E2E/piloto são criadas pela própria API (Create → AddItem/AddRule → Publish, com validação do DAG no `Publish`).
- [ ] Telas: Ativação/Login, Minha Jornada, Agendar, Minha Consulta
  - [x] Front de teste da jornada (`apps/web/src/features/journey/`): telas cruas de
    Minha Jornada, Agendar (por item, via `/me/availability` com a Idempotency-Key
    nascida por intenção) e Minhas consultas (`/me/appointments`), com hooks
    (`useJourney`) e testes colocalizados. Estilo mínimo — o design vem depois.
- [ ] E2E (Playwright): fluxo feliz + 2 bloqueios
  - [x] E2E de integração em Go (`internal/e2e/`, 29 passos em 2 cenários) já cobre o
    cenário-alvo do Slice 1 (feliz + bloqueios de cota/intervalo/vigência);
    Playwright no browser fica para quando as telas de produto existirem.

### P1 — robustez
- [ ] Conciliação via histórico DAV (no-show real)
- [ ] Lembretes por e-mail · reagendamento · observabilidade (logs/healthchecks)

### P2 — pós-consulta
- [ ] Receitas, medicamentos, atividades + portal do profissional

## 🔎 Levantamentos pendentes (Sprint 0 técnica — SPEC §9)

- [x] Schema real do MySQL legado + como evita double-booking hoje (o app legado vira `booked=1`: 84 de 85 consultas ativas na HML)
- [x] Doc da API Doutor ao Vivo: cadastro, agendamento e sala mapeados (`docs/DAV-API-NOTAS.md`). Falta **histórico**.
- [ ] Confiabilidade do CPF na tabela de colaboradores da Gestão 2.0
- [ ] Acesso de rede da VM aos bancos legado e Gestão

## ❓ Decisões de produto aguardando martelo

Ver `docs/DECISOES.md` → ADR-009.
