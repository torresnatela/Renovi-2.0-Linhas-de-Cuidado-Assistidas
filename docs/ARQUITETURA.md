# Arquitetura — Renovi 2.0

Resumo executável da arquitetura da fundação. O contexto completo de produto está
no **SPEC v1** (documento de estratégia). Aqui foca-se em *como o código está
organizado* e *por quê*.

## 1. Decisão central: MVC pragmático + domínio puro

O SPEC original descrevia uma arquitetura hexagonal (ports/adapters). Optamos por
um **MVC clássico** para evitar overengineering, com **uma exceção deliberada**:
o motor de elegibilidade fica isolado em um pacote **puro** (sem I/O), porque é o
coração testável do sistema.

```
Requisição HTTP
   │
   ▼
controllers/         (C) recebe req, valida entrada, chama models, responde JSON
   │
   ▼
models/              (M) regra de negócio + acesso a dados (via sqlc)
   ├── eligibility/  ← PURO: motor de decisão, sem banco/HTTP/relógio implícito
   └── ...           ← repositórios sobre o renovi_care
   │
   ▼
db/                  pool pgx + migrations + código sqlc gerado (gen/)
```

- **Views** = respostas JSON (é uma API, não HTML).
- **Não há camada `service`/`ports`/`app`** — os controllers orquestram direto os
  models. Se um caso de uso crescer demais, extraia uma função no próprio `models`.
- **`models/eligibility` é sagrado:** nunca importe banco, HTTP ou `time.Now()` lá
  dentro. Tudo entra por parâmetro. Isso o torna 100% testável com tabelas de casos.

## 2. Estrutura do backend (`apps/api`)

```
cmd/
  api/        # servidor HTTP
  worker/     # cron (reconciliação, auto-conclusão) — STUB
  seed/       # aplica templates de linha de cuidado — STUB
  migrate/    # CLI de migrations (up/down/version)
internal/
  config/     # 12-factor: lê env, valida
  http/       # router chi + middleware (request-id, recover, timeout, log)
  controllers/# handlers HTTP
  models/     # regra de negócio + dados
    eligibility/  # motor PURO (reservado na fundação)
  db/         # pgx pool, migrate (embed), migrations/, queries/ (sqlc), gen/ (gerado)
  testsupport/# helpers de teste de integração (testcontainers)
seeds/care_lines/  # templates versionados (JSON)
```

## 3. Os três bancos (regra inegociável)

| Banco | Papel | Acesso do renovi-care |
|---|---|---|
| **Postgres `renovi_care`** (nosso) | Todo o domínio novo (contas, templates, jornada, consultas) | Escrita + leitura. Migrations versionadas. |
| **MySQL legado** | Verdade da escala/slots + trava de agenda | Leitura de slots + escrita **restrita** à tabela de agendamento, via Adapter Agenda. |
| **Postgres Gestão 2.0** | Empresa, contrato, colaborador elegível | **Somente leitura**, via Adapter Gestão. Nunca escreve. |

No dev local, os três sobem via `make up` (os externos como **mocks** — ver
`deploy/`). Os schemas reais serão mapeados na Sprint 0 (SPEC §9).

## 4. Modelo de domínio (linha de cuidado)

Separa **template** (definição versionada, reutilizável) de **instância**
(`enrollment`, a jornada de um paciente). O motor lê apenas o `journey_event`
(log append-only) e devolve, por item, um **veredito**: `AVAILABLE`, `BLOCKED`,
`QUOTA_EXHAUSTED`, `NOT_YET_OPEN`, `OVERDUE` — com `reasons[]` máquina-legíveis
que o front traduz. O contrato desses tipos já está em
`internal/models/eligibility` (Go) e no OpenAPI (`ItemVerdict`).

As regras de cada item (periodicidade/cota, pré-requisito, janela de liberação)
são declarativas (JSONB). Ver SPEC §3 para a gramática completa.

> **Na fundação**, o schema de domínio ainda **não** existe — há apenas uma tabela
> `example_widget` demonstrando as convenções. A implementação real segue o SPEC §4.

## 5. Fluxos críticos (referência — implementação no MVP)

- **Ativação** (Gestão → paciente): token por CPF/e-mail → conta ACTIVE → enrollment → cadastro na DAV.
- **Agendamento** (3 sistemas, sem transação global): intenção registrada no
  `renovi_care` (`PENDING_SLOT`) → **CAS** do slot no legado (`booked=0 → 1`) →
  `DAV_PENDING` → `POST /appointment` na DAV, **uma tentativa, sem retry** →
  `CONFIRMED` com o link do paciente. No desconhecido, **falha fechada**: o horário
  fica retido e a consulta vai para revisão humana (ADR-016).

  > Este parágrafo já disse "lock pessimista → espelho local `CONFIRMED` →
  > agendamento na DAV (best-effort + retry)". As três coisas estavam erradas e
  > ficam registradas aqui porque a versão antiga é a intuição que qualquer um
  > teria: **lock pessimista** virou CAS (ADR-015), **`CONFIRMED` antes da DAV**
  > seria uma consulta confirmada sem link — mentira que o paciente descobriria no
  > horário da consulta — e **retry** é exatamente o que o ADR-011b proíbe: repetir
  > uma escrita que talvez tenha pegado cria uma segunda consulta de verdade.
- **Teleconsulta + auto-conclusão**: paciente entra na sala DAV; job marca `COMPLETED` após o horário, avançando a jornada.

Detalhes e ordem exata: SPEC §5.

## 6. Geração de código (API-first)

```
packages/contracts/openapi.yaml   ── oapi-codegen ─▶  internal/http/api/*.gen.go (tipos Go)
                                  └─ orval ────────▶  packages/api-client (hooks TS)
internal/db/queries/*.sql         ── sqlc ─────────▶  internal/db/gen (repositórios Go)
```

Rode sempre `make generate` após mexer no contrato ou nas queries. O CI valida que
o código gerado está commitado e atualizado.

## Ver também

- `docs/DESENVOLVIMENTO.md` — como rodar tudo localmente.
- `docs/DECISOES.md` — decisões de arquitetura (ADRs).
- `docs/PROGRESSO.md` — estado atual e backlog.
