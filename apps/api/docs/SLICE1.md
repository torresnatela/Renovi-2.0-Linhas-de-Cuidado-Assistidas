# Slice 1 — Linhas de Cuidado Assistidas

README operacional do slice. Decisões em `docs/DECISOES.md` (ADR-020 a ADR-025);
estado vivo em `docs/PROGRESSO.md`.

## O que é

O colaborador tem uma **linha de cuidado** (jornada) com itens (consultas). Um
**motor de regras puro** (`internal/models/careline/`) decide, para um instante, se
ele pode agendar cada item — quota em janela móvel, intervalo mínimo, antecedência
máxima, pré-requisito e vigência da matrícula. O agendamento em si reusa o booking
já existente (saga MySQL legado + DAV); a jornada é uma **projeção clínica** por
cima (`care_appointment`), com trilha **append-only** (`journey_event`).

Rotas: `/admin/care-lines*` e `/admin/enrollments*` (operação por token, sem
back-office) e `/me/journey`, `/me/eligibility`, `/me/availability`,
`/me/appointments` (+cancel), `/me/audit` (paciente).

## Subir do zero

```bash
# 1. Infra local (Postgres renovi_care + mocks de MySQL legado e Gestão)
make up

# 2. Migrations — rodam como OWNER (renovi), pois criam tabelas e o próprio role
#    renovi_app (migration 0008). Aponte a URL de migração para o owner:
RENOVI_CARE_MIGRATE_DATABASE_URL=postgres://renovi:renovi@localhost:5432/renovi_care?sslmode=disable \
  make migrate-up

# 3. Slots futuros no mock do legado (idempotente; ids manual-a-*/manual-b-*)
make seed-legacy-slots

# 4. API — conecta como renovi_app (role restrito). Para rodar o roteiro manual,
#    ligue o admin e as rotas de teste (NUNCA em produção):
RENOVI_ADMIN_TOKEN=dev-admin-token RENOVI_TEST_ENDPOINTS=true \
  go -C apps/api run ./cmd/api
```

Envs em `.env.example` (copie para `.env`). Os dois papéis do Postgres:
`RENOVI_CARE_DATABASE_URL` = **app** (renovi_app); `RENOVI_CARE_MIGRATE_DATABASE_URL`
= **owner** (renovi) — se vazia, cai na de app (só serve em setup sem split de
privilégio). Em **produção** o operador troca a senha do role:
`ALTER ROLE renovi_app PASSWORD '<segredo>'` (a da migration é de DEV — ADR-024).

Com tudo no ar, rode o roteiro ponta a ponta em **`apps/api/docs/slice1.http`**
(cenários A e B: publicar linha, matricular, agendar, esbarrar em cota/intervalo/
vigência, cancelar, renovar).

## Testes

```bash
make test              # unitários (rápidos, sem Docker)
make test-integration  # integração (testcontainers; exige Docker)
```

Camadas cobertas:
- **Motor puro** — `careline/evaluate_test.go`, tabela **T1–T19**: a especificação
  normativa do slice (mudança de semântica começa por ela).
- **Casos de uso** — testes de unidade do `JourneyStore` com fakes: 422 de
  elegibilidade, replay 200 da idempotência, compensação da corrida de key.
- **Schema + role** (integração) — todas as queries contra Postgres real, com o
  role restrito `renovi_app`; prova que `UPDATE`/`DELETE` em `journey_event` é
  recusado no banco (append-only por privilégio).
- **E2E** (`internal/e2e/`, tag `integration`, **29 passos em 2 cenários**) — API
  real contra Postgres + MySQL (testcontainers) e uma DAV fake: cenário-alvo A
  (23 passos) e B (6 passos).

## Onde ficam as constantes de política

| Política | Onde | Env |
|---|---|---|
| Antecedência mínima p/ o cancelamento devolver a cota | `careline.DefaultCancelCountThreshold` (24h) | `RENOVI_CANCEL_COUNT_THRESHOLD` (default 24h); por journey, sobrescreve |
| Período `week` da quota | `careline.WeekWindow` (7d fixo) | — |
| Período `month` da quota | `careline.MonthWindow` (**30d fixo**, não mês civil — ADR-020) | — |
| Janela default da disponibilidade | `Availability`: hoje → +30 dias | via `from`/`to` da query |
| Teto da janela de disponibilidade | `maxAvailabilityWindow` (60d) | — |
| Antecedência **máxima** do agendamento | regra `MAX_ADVANCE` (`days`), **por item** no catálogo | — (dado do catálogo) |

A quota é **janela móvel geral**, não calendário civil — ADR-020 detalha o porquê.

## Segurança

- **`renovi_app`** — a app roda com role restrito; `journey_event` é append-only
  por privilégio de banco, não por disciplina de código (ADR-024).
- **Admin por token estático** (`X-Admin-Token` × `RENOVI_ADMIN_TOKEN`,
  constant-time; vazio = rotas desligadas). Sem rotação nem auditoria de operador —
  risco aceito no piloto (ADR-022).
- **`RENOVI_TEST_ENDPOINTS`** liga a rota interna `force-status` (avançar consulta
  para `realizada`/`falta` no teste). A API **recusa subir** com ela `true` em
  `RENOVI_ENV=production` — **nunca em produção**.
- Rate limit do `POST /me/appointments` é **por CONTA** da sessão (não por IP),
  pela razão do ADR-019.

## Limitações conhecidas

- **`CANCELLED` com slot retido fica órfão** (ADR-023): se o `ReleaseSlot` no legado
  falha, a consulta fica `CANCELLED` com `slot_held_at` e a fila do worker
  (`ListPendingSlotRelease`) hoje só varre `FAILED`. Cobre no worker do Slice 2.
- **Expiração de matrícula é LAZY** — só na leitura da jornada; o cron vem depois
  (ADR-021).
- **DAV cancel responde 500** em HML (achado #20): o cancelamento local segue e é
  auditado, mas a consulta pode continuar viva na DAV. Sem reconciliação possível
  (ADR-016).
- **DAV HML instável no limite dos 29s**: o `POST /appointment` estoura o teto do
  gateway na cauda → 502 fail-closed (o horário fica retido, sem consulta fantasma).
