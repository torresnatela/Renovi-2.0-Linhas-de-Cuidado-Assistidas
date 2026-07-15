# Renovi 2.0 — Plataforma do Paciente

Monorepo da plataforma onde o colaborador executa sua **linha de cuidado assistida**:
vê a jornada, agenda as consultas que as regras permitem e realiza a teleconsulta.

> **Estado:** fundação (estrutura + infra + testes). Veja `docs/PROGRESSO.md` para o
> que vem a seguir e `docs/ARQUITETURA.md` para o desenho.

## Stack

Go 1.25 (MVC · chi · pgx · sqlc) · PostgreSQL · React 18 + Vite + TypeScript ·
Docker Compose · OpenAPI (API-first) · TDD.

## Quickstart

```bash
cp .env.example .env
make up            # Postgres + mocks dos bancos externos (Docker)
make migrate-up    # migrations do renovi_care
make test          # testes unitários do backend
go -C apps/api run ./cmd/api    # API em http://localhost:8090

# front (outro terminal)
make web-install && make web-dev   # http://localhost:5173
```

`make help` lista todos os comandos.

## Estrutura

```
apps/api/              backend Go (MVC)          → apps/api/CLAUDE.md
apps/web/              front React               → apps/web/CLAUDE.md
packages/contracts/    openapi.yaml (verdade)
packages/api-client/   cliente TS gerado (orval)
deploy/                docker-compose, Caddyfile, mocks
docs/                  arquitetura, desenvolvimento, decisões, progresso
Makefile               todos os comandos
```

## Para desenvolver com Claude Code

Leia o `CLAUDE.md` da raiz — ele define stack, fluxo (API-first + TDD), convenções
e a regra de manter os docs vivos. Cada app tem seu próprio `CLAUDE.md` com o
contexto local.

## Documentação

| Doc | Conteúdo |
|---|---|
| [docs/ARQUITETURA.md](docs/ARQUITETURA.md) | Desenho, camadas, bancos, domínio |
| [docs/DESENVOLVIMENTO.md](docs/DESENVOLVIMENTO.md) | Setup local, comandos, troubleshooting |
| [docs/DECISOES.md](docs/DECISOES.md) | ADRs (decisões técnicas) |
| [docs/PROGRESSO.md](docs/PROGRESSO.md) | Estado atual e backlog |
