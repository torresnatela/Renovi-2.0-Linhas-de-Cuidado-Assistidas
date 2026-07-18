# Desenvolvimento — setup local

## Pré-requisitos

- **Go 1.25+** (a geração com sqlc baixa automaticamente o toolchain 1.26 na 1ª vez)
- **Node 20+** (testado com Node 24)
- **Docker** (para os bancos locais e testes de integração)
- **make**

## Primeira vez

```bash
# 1. Variáveis de ambiente
cp .env.example .env

# 2. Sobe os bancos (Postgres renovi_care + mocks de legado/Gestão)
make up

# 3. Aplica as migrations no renovi_care
make migrate-up

# 4. Backend
make build            # compila
make test             # testes unitários
go -C apps/api run ./cmd/api     # sobe a API em :8090  (ou: cd apps/api && go run ./cmd/api)

# 5. Front (outro terminal)
make web-install
make web-dev          # http://localhost:5173  (proxy /api -> :8090)
```

Abra `http://localhost:5173`: o badge deve ficar verde ("API ok") quando a API
estiver rodando.

## Comandos do dia a dia

| Objetivo | Comando |
|---|---|
| Ver todos os alvos | `make help` |
| Testes unitários (rápidos) | `make test` |
| Testes de integração (Docker) | `make test-integration` |
| Regenerar código (sqlc + oapi) | `make generate` |
| Checar formatação + vet | `make lint` |
| Formatar Go | `make fmt` |
| Nova migration | crie `NNNN_nome.up.sql` e `.down.sql` em `apps/api/internal/db/migrations` |
| Aplicar/reverter migration | `make migrate-up` / `make migrate-down` |
| Espelhar o CI | `make ci` |

## Como adicionar uma rota (fluxo API-first)

1. Edite `packages/contracts/openapi.yaml` (defina path, params, schemas).
2. `make generate` (gera tipos Go e, quando habilitado, hooks TS).
3. Escreva o teste do handler (`internal/controllers/..._test.go`).
4. Implemente o handler em `internal/controllers/` e monte a rota em `internal/http/router.go`.
5. `make test && make lint`.

## Como adicionar uma tabela + query

1. Nova migration em `internal/db/migrations` (`NNNN_nome.up.sql` / `.down.sql`).
2. Escreva a query em `internal/db/queries/*.sql` (anotação `-- name: ... :one|:many|:exec`).
3. `make generate-sqlc` → gera o repositório tipado em `internal/db/gen`.
4. Envolva num model em `internal/models/` e teste com `make test-integration`.

## Rodar o agendamento localmente

O agendamento fala com DOIS sistemas externos, e os dois precisam de um passo
manual na primeira vez.

**1. O mock do legado tem que estar com o schema novo.** O `init.sql` do MySQL só
roda em volume VAZIO — quem já tinha rodado `make up` antes de 2026-07-16 continua
com o schema antigo (`medico`/`slot`/`agendamento`) e o adapter não acha
`tb_slots`. Recriar:

```bash
docker rm -f renovi-mysql-legacy
docker volume rm deploy_mysql_legacy_data
make up
```

**2. Os médicos do mock precisam existir na DAV de homologação**, com o MESMO id
do `tb_professionals.id` — é ele que vira o participante `MMD`. Sem isso, o
`POST /appointment` não tem médico e o agendamento não fecha:

```bash
KEY=$(awk -F= '/^RENOVI_DAV_API_KEY=/{print $2}' .env)
BASE=$(awk -F= '/^RENOVI_DAV_BASE_URL=/{print $2}' .env)
curl -X POST "$BASE/professional" -H "x-api-key: $KEY" -H 'Content-Type: application/json' -d '{
  "id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","name":"Bruno Carvalho Lima",
  "cpf":"80387508376","birth_date":"1979-11-30","email":"bruno.lima@example.com",
  "license_number":"123456","license_region":"SP","license_council":"CRM"}'
```

⚠️ **Se este POST devolver 504, NÃO o repita cegamente.** Confira com
`GET /professional/{id}` primeiro: o 504 da DAV mente — ele já criou o registro em
todas as vezes que medimos. Pior: um profissional criado por um POST que estourou
parece ficar **quebrado** lá — todo `POST /appointment` com ele como MMD estoura os
29s do gateway, enquanto o mesmo agendamento com um profissional criado com 201
limpo fecha em ~22s. Se cair nisso, use outro id.

**3. Suba a API com o legado ligado** (a aplicação NÃO lê o `.env` sozinha —
config é 12-factor):

```bash
export RENOVI_CARE_DATABASE_URL='postgres://renovi:renovi@localhost:5432/renovi_care?sslmode=disable'
export RENOVI_LEGACY_DATABASE_URL='renovi:renovi@tcp(localhost:3306)/renovi_legacy'
export RENOVI_DAV_BASE_URL=... RENOVI_DAV_API_KEY=...
export RENOVI_SESSION_COOKIE_SECURE=false
go -C apps/api run ./cmd/api
```

Sem `RENOVI_LEGACY_DATABASE_URL`, as rotas de agendamento simplesmente não sobem
(e o log avisa) — mesma política das rotas de auth sem credencial da DAV.

**Para testar a entrada na sala sem esperar o horário**, aumente a janela em vez de
mexer no relógio da máquina:

```bash
RENOVI_JOIN_OPENS_BEFORE=48h go -C apps/api run ./cmd/api
```


## Troubleshooting

- **`make generate` baixa Go 1.26:** normal — o sqlc exige. É automático (`GOTOOLCHAIN=auto`).
- **Testes de integração falham sem Docker:** eles exigem o daemon rodando (`make up` não é necessário, o testcontainers sobe containers próprios).
- **Front com erro de tipo em `test`:** garanta que `vite.config.ts` importa `defineConfig` de `vitest/config` (não de `vite`).
- **Porta 5432/3306 ocupada:** ajuste as portas no `deploy/docker-compose.yml` e no `.env`.
- **Porta da API (`:8090`) ocupada:** rode em outra porta sem editar código — `RENOVI_HTTP_ADDR=:8095 go -C apps/api run ./cmd/api` (e ajuste o proxy em `apps/web/vite.config.ts` se usar o front).
