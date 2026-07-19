# Progresso — estado vivo do projeto

> **Todo agente atualiza este arquivo ao avançar.** É a fonte da verdade de "onde
> estamos". Marque `[x]` o que concluiu e ajuste "Próximo passo".

_Última atualização: 2026-07-18 — **Verificador Diário de Humor (Anexo C), Módulo 0 concluído:** `care_line_item.kind` ganha ATIVIDADE (branch `feat/verificador-humor`)._

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
- [ ] Módulo 5 — Anel gatilhado PHQ-4 + gatilho puro (`models/mood/trigger`).
- [ ] Módulo 6 — Roteamento de crise/escalonamento + fechamento.

## 🚧 Slice 1 — Linhas de Cuidado Assistidas (em andamento)

- [x] **Fase 1 — motor de regras puro** (`internal/models/careline/`, ADR-020):
  `Evaluate` (VIGENCIA, QUOTA janela móvel GERAL, MIN_INTERVAL, MAX_ADVANCE,
  PREREQUISITE), `ParseRuleParams` (params tipados, `DisallowUnknownFields`) e
  `ValidatePublish` (itens, ciclos de pré-requisito, especialidades do legado).
  A tabela T1–T19 em `evaluate_test.go` é a **especificação normativa** do slice.
- [ ] Próximas fases: schema/migrations do domínio, queries, wiring nos
  controllers e front (o pacote ainda não é importado por ninguém — de propósito).

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

**1. `cmd/worker`** — hoje stub, e a saga já produz as filas que ele deveria varrer
(compensação e revisão). Sem ele, horário que a compensação não devolveu fica
retido até alguém olhar.

**2. Wiring do motor de linhas de cuidado.** O motor puro está pronto
(`models/careline`, Fase 1 do Slice 1); faltam schema, queries e os controllers
que o alimentam com `Journey`/`Rule` e expõem os `Blocks` ao front. Ele filtra
ANTES do agendamento, sem mudar as rotas já contratadas.

## 🗺️ Backlog por fase (resumo — detalhe no SPEC §11)

### P0 (MVP) — linha de cuidado + agendamento
- [ ] Schema real de domínio (patient_account, care_line_template/item/dependency, enrollment, journey_event, appointment, idempotency_key) — migrations
- [ ] Motor de elegibilidade implementado + `GET /me/eligibility`
- [ ] Ativação de conta (token por CPF/e-mail) + Adapter Gestão (leitura)
- [x] Adapter Agenda (legado): leitura de slots + reserva por CAS (ADR-015)
- [x] Fluxo de agendamento distribuído + Adapter DAV — **sem** reconciliação: ela é impossível hoje (ADR-016)
- [ ] Auto-conclusão (cron) + jornada avançando
- [ ] `cmd/seed` real (aplica `saude-mental-v1`, valida DAG)
- [ ] Telas: Ativação/Login, Minha Jornada, Agendar, Minha Consulta
- [ ] E2E (Playwright): fluxo feliz + 2 bloqueios

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
