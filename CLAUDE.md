# CLAUDE.md — Guia mestre para agentes (Renovi 2.0)

> Este arquivo é lido pelo Claude Code em toda sessão. Mantenha-o curto e atual.
> **Documentação detalhada vive em `docs/`.** Idioma: **código em inglês, docs em PT-BR**.

## O que é

Renovi 2.0 — **Plataforma do Paciente**: o colaborador vê sua **linha de cuidado**
(jornada), agenda as consultas que as regras permitem e realiza a teleconsulta.
Escopo atual = **fundação** (estrutura + infra + testes). Detalhes do produto no
SPEC v1 (documento de contexto) e em `docs/ARQUITETURA.md`.

## Stack

| Camada | Escolha |
|---|---|
| Backend | Go 1.25 · chi · pgx v5 · **sqlc** · golang-migrate · slog · Argon2id (x/crypto) |
| Arquitetura | **MVC pragmático** (`controllers` → `models` → `db`) + motor de elegibilidade **puro** isolado + `internal/adapters/` para sistemas externos |
| Auth | Sessão **opaca** em cookie httpOnly (ADR-010) — não JWT |
| Externo | **Doutor ao Vivo** (dados de saúde + teleconsulta) — comportamento real em `docs/DAV-API-NOTAS.md` |
| Contrato | OpenAPI (`packages/contracts/openapi.yaml`) — **fonte da verdade** · oapi-codegen |
| Front | React 18 · TypeScript · Vite · Tailwind · TanStack Query · Vitest |
| Banco próprio | PostgreSQL `renovi_care` — produção no **Neon** (Postgres 17, ADR-021) |
| Infra local | Docker Compose (Postgres + mocks do legado/Gestão) |
| Infra prod | VPS Hostinger compartilhada · GHCR + deploy via GitHub Actions (aprovação manual) · Caddy de borda — ver `docs/DEPLOY.md` |

## Estrutura do monorepo

```
apps/api/        # backend Go (MVC)  — ver apps/api/CLAUDE.md
apps/web/        # front React       — ver apps/web/CLAUDE.md
packages/contracts/   # openapi.yaml (fonte da verdade)
packages/api-client/  # cliente TS gerado (orval) — placeholder
deploy/          # docker-compose, Caddyfile, mocks dos bancos
docs/            # ARQUITETURA, DESENVOLVIMENTO, DECISOES, PROGRESSO, DEPLOY, LINHA-DE-CUIDADO
Makefile         # todos os comandos
```

## Comandos essenciais

```bash
make help              # lista todos os alvos
make up                # sobe Postgres + mocks (Docker)
make migrate-up        # aplica migrations no renovi_care
make seed-legacy-slots # semeia slots futuros no mock do legado (idempotente; Slice 1)
make test              # testes unitários Go (rápidos)
make test-integration  # testes de integração (testcontainers; exige Docker)
make dav-probe         # sonda a API da Doutor ao Vivo (HML) -> docs/DAV-API-NOTAS.md
                       # CRIA pessoas de teste lá. Fora do CI. Nunca aponte para produção.
make generate          # regenera código Go (sqlc + oapi-codegen)
make lint              # gofmt + go vet
make web-dev           # front em dev (proxy /api -> :8090)
make ci                # espelha o pipeline de CI localmente
```

## Fluxo de trabalho (OBRIGATÓRIO)

1. **API-first:** mudou a API? Edite `packages/contracts/openapi.yaml` **primeiro**,
   rode `make generate`, e só então implemente. O CI falha se houver código gerado
   desatualizado (`make generate-check`).
2. **TDD:** escreva o teste (vermelho) → implemente (verde) → refatore. O motor de
   elegibilidade (`apps/api/internal/models/eligibility`) é **puro** e o alvo
   principal de testes table-driven.
3. **Migrations:** toda mudança de schema é uma migration nova em
   `apps/api/internal/db/migrations` (nunca edite uma já aplicada).
4. **Env nova = fiar no deploy (automático, não manual):** adicionou uma variável de
   ambiente? Além de `.env.example` e `internal/config/config.go`, **renderize-a no
   passo "Render .env e .env.api" do `.github/workflows/ci.yml`** (bloco
   `.env.api.render`). O deploy **sobrescreve** o `.env.api` do container a cada vez —
   o que não estiver nesse passo NUNCA chega à produção (setar na mão na VPS é apagado
   no deploy seguinte). Vars cuja ausência derruba o boot (`config.validate()`) ou que
   ligam uma feature (as `RENOVI_*` da ingestão da Gestão, p. ex.) são as mais
   críticas: sem esse passo, a feature sobe DESLIGADA em produção sem erro nenhum.

## Convenções

- **Código em inglês** (identificadores, commits); **docs e comentários de domínio em PT-BR**.
- **Não edite código gerado** (`internal/db/gen`, `internal/http/api/*.gen.go`, `packages/api-client/src/generated`) — rode `make generate`.
- **LGPD:** nunca logar corpo de request de autenticação nem dados de saúde. CPF só em tabelas de identidade.
- Enums no banco = `TEXT + CHECK`; timestamps = `TIMESTAMPTZ`; PKs = `UUID` (v7 na aplicação).

## 🔴 Regra para todo agente (docs vivos)

**Ao concluir qualquer avanço, ATUALIZE a documentação antes de encerrar:**

- `docs/PROGRESSO.md` — marque o que foi feito e o próximo passo.
- `docs/DECISOES.md` — registre qualquer decisão técnica nova (formato ADR).
- O `CLAUDE.md` local do app tocado (`apps/api/CLAUDE.md` ou `apps/web/CLAUDE.md`) se mudou padrão/estrutura.
- Este arquivo, se mudou stack ou comando.

Docs desatualizados são bug. Trate como parte da "definição de pronto".
