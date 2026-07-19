# Progresso — estado vivo do projeto

> **Todo agente atualiza este arquivo ao avançar.** É a fonte da verdade de "onde
> estamos". Marque `[x]` o que concluiu e ajuste "Próximo passo".

_Última atualização: 2026-07-18 — **Slice 1 (Linhas de Cuidado), Fase 8 concluída:** artefatos do ambiente manual (seed de slots + `slice1.http` + env)._

## 🚧 Slice 1 — Linhas de Cuidado Assistidas (em andamento)

- [x] **Fase 1 — motor de regras puro** (`internal/models/careline/`, ADR-020):
  `Evaluate` (VIGENCIA, QUOTA janela móvel GERAL, MIN_INTERVAL, MAX_ADVANCE,
  PREREQUISITE), `ParseRuleParams` (params tipados, `DisallowUnknownFields`) e
  `ValidatePublish` (itens, ciclos de pré-requisito, especialidades do legado).
  A tabela T1–T19 em `evaluate_test.go` é a **especificação normativa** do slice.
- [x] **Schema + queries** (migrations 0005–0008): `care_line/care_line_item/
  care_line_rule` (catálogo versionado e imutável por versão), `enrollment` +
  `enrollment_period`, `care_appointment` + `journey_event` (append-only, role
  `renovi_app` sem UPDATE/DELETE) e as queries sqlc de tudo.
- [x] **Contrato**: `/admin/care-lines*`, `/admin/enrollments*`, `/me/journey`,
  `/me/eligibility`, `/me/availability`, `/me/appointments` (+cancel), `/me/audit`
  e `/internal/.../force-status` no `openapi.yaml`, com tipos gerados.
- [x] **Admin** (token estático `X-Admin-Token`): `CareLineStore` (draft → itens
  → regras → publish validando especialidades no legado) e `EnrollmentStore`
  (matricular/renovar/encerrar com eventos), rotas montadas.
- [x] **`BookingStore.Cancel`** (paciente): CANCELLED + devolve o horário ao
  legado + cancel best-effort na DAV (que responde 500 em HML — tolerado).
- [x] **Fase 6 — jornada do paciente** (ADR-021): `models/care_journey.go`
  (`JourneyStore`, interfaces `BookingService`/`journeyStorage` no consumidor) +
  `care_journey_repo.go` (linha+evento SEMPRE na mesma TX). Rotas `/me/journey`
  (matrículas com itens já avaliados + eventos recentes, **expiração lazy**),
  `/me/eligibility` (com `date` simulada), `/me/availability` (slots dos
  profissionais da especialidade anotados com o veredito por instante),
  `POST /me/appointments` (**Idempotency-Key obrigatória**, motor ANTES do
  booking → 422 `blocks[]`, replay 200, corrida de key compensa o booking),
  cancel com bookkeeping de cota, `/me/audit` (keyset por cursor opaco) e
  `POST /internal/appointments/{id}/force-status` (só com
  `RENOVI_TEST_ENDPOINTS`).
- [x] **Fase 7 — E2E do cenário-alvo** (`apps/api/internal/e2e/`, tag
  `integration`): sobe a API real (router de produção, fiação à mão espelhando
  `cmd/api/main.go`) contra Postgres + MySQL reais (testcontainers, role
  restrito `renovi_app`) e uma DAV **fake** (`httptest`, cancel sempre 500 —
  achado #20 tolerado de propósito). `TestE2E_A_SaudeMentalBasica` (23 passos:
  publish/validação, cota/intervalo/antecedência/vigência, cancelamento com
  devolução de vaga, renovação contígua, expiração lazy + reativação,
  auditoria paginada) e `TestE2E_B_ApoioPsicologico` (6 passos: QUOTA
  `period:total` = bloqueio permanente sem `available_from`). Novo
  `testsupport.SeedFutureSlots` (ids `e2e-*`, sem colidir com o `init.sql`).
- [x] **Fase 8 — artefatos do ambiente manual**: `deploy/mysql-legacy/seed-slots.sql`
  (idempotente, `INSERT IGNORE`, ids `manual-a-*`/`manual-b-*` — mesmos offsets
  do E2E, mas sobrevivendo ao mock **persistente** do compose) + alvo
  `make seed-legacy-slots` + `TestSeedLegacySlotsIsIdempotent`
  (`internal/testsupport/seed_slots_test.go`, roda o script duas vezes e confere
  a contagem). `apps/api/docs/slice1.http` espelha 1:1 os passos HTTP dos
  cenários A e B (os 2 passos que mexem direto no Postgres via superusuário —
  expiração/reativação lazy e o teste do grant append-only — ficam de fora, sem
  rota correspondente) + bloco C curto de validação de publish/token.
  `.env.example` ganha `RENOVI_ADMIN_TOKEN`/`RENOVI_TEST_ENDPOINTS` documentados.
- [ ] Próximas fases: front (telas da jornada), auto-conclusão via worker/cron,
  seed `saude-mental-v1`.

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
  - [x] Front de teste da jornada (`apps/web/src/features/journey/`): telas cruas de
    Minha Jornada, Agendar (por item, via `/me/availability` com a Idempotency-Key
    nascida por intenção) e Minhas consultas (`/me/appointments`), com hooks
    (`useJourney`) e testes colocalizados. Estilo mínimo — o design vem depois.
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
