# CLAUDE.md — Backend (apps/api)

Contexto local do backend Go. Regras gerais no `CLAUDE.md` da raiz; arquitetura em
`docs/ARQUITETURA.md`.

## Padrão MVC (onde colocar o quê)

| Preciso... | Vai em... |
|---|---|
| Handler de uma rota | `internal/controllers/<área>_controller.go` |
| Regra de negócio + acesso a dados | `internal/models/<entidade>.go` |
| Decisão pura (linha de cuidado) | `internal/models/careline/` — **sem I/O, nunca** |
| Decisão pura (humor — Anexo C) | `internal/models/mood/scoring/` (pontuação) e `internal/models/mood/trigger/` (gatilho) — **sem I/O, nunca** |
| Cliente de sistema externo (DAV, Gestão, legado) | `internal/adapters/<sistema>/` — interface no **consumidor** (ADR-012) |
| Query SQL | `internal/db/queries/*.sql` → `make generate-sqlc` |
| Migration | `internal/db/migrations/NNNN_nome.up.sql` + `.down.sql` |
| Montar rota | `internal/http/router.go` |
| Config/env | `internal/config/config.go` |
| Job de cron | `cmd/worker/` |

## Regras de ouro

- **`models/careline` é puro.** Proibido importar `db`, `net/http` ou chamar
  `time.Now()` lá dentro. Tudo entra por parâmetro. É o alvo #1 de testes table-driven.
- **Nunca edite** `internal/db/gen/` nem `internal/http/api/*.gen.go` — são gerados.
- **API-first:** rota nova começa no `openapi.yaml` (raiz `packages/contracts`), depois `make generate`.
- **Controllers finos:** validam entrada, chamam model, respondem (`WriteJSON`/`WriteProblem`). Sem SQL no controller.
- **Erros HTTP:** use `controllers.WriteProblem` (RFC 7807).
- **Testes:** unitários por padrão (`_test.go`); integração com `//go:build integration` + testcontainers (`make test-integration`).
- **Sondagem da DAV:** `make dav-probe` (tag `davprobe`) bate na API real de HML e **cria pessoas de teste**. Fora do CI. Achados em `docs/DAV-API-NOTAS.md` — ele vale mais que o spec publicado deles, que se contradiz.
- **Segredos:** `config.Config` tem `LogValue()` que redige a API key e a senha do banco. Ao somar campo sensível na config, some-o lá também.

## Comandos

```bash
make test              # unitários
make test-integration  # integração (Docker)
make generate          # sqlc + oapi-codegen
make lint              # gofmt + vet
go -C apps/api run ./cmd/api      # sobe a API
go -C apps/api run ./cmd/migrate up
```

## Dependências principais

chi (router) · pgx v5 (Postgres) · sqlc (gen) · golang-migrate (embed) ·
google/uuid · slog (log) · testify + testcontainers-go (testes).

## Ao terminar

Atualize `docs/PROGRESSO.md` e, se criou decisão nova, `docs/DECISOES.md`.
