# deploy/

Infraestrutura do Renovi 2.0.

## Desenvolvimento local

`docker-compose.yml` sobe os três bancos do ecossistema como containers locais:

| Serviço | Banco | Porta host | Papel |
|---|---|---|---|
| `pg-care` | Postgres `renovi_care` | 5432 | Nosso banco (escrita/leitura) |
| `pg-gestao` | Postgres `gestao` (mock) | 5433 | Gestão 2.0 — leitura (adapter) |
| `mysql-legacy` | MySQL `renovi_legacy` (mock) | 3306 | Escala/slots legado (adapter) |

Os mocks (`pg-gestao/init.sql`, `mysql-legacy/init.sql`) têm schemas plausíveis +
dados de exemplo para desenvolver os adapters antes do levantamento da Sprint 0
(SPEC §9). **Não representam o schema real** — serão substituídos.

```bash
make up            # sobe os bancos
make migrate-up    # aplica migrations no renovi_care
make down          # derruba tudo
```

A API e o worker rodam **fora** do compose, via `go run` (`make migrate-up`, etc.),
apontando para os bancos acima (ver `.env.example`).

## Produção (VM Hostinger)

Alvo: `docker compose up -d` com os serviços `caddy` + `api` + `worker` (ver
`Caddyfile`). O `renovi_care` de produção pode ser um container Postgres com
volume ou um Postgres gerenciado; os bancos legado e Gestão são externos
(acesso por rede — pendência §9.4). Este compose de produção entra na fase P1
(observabilidade/robustez) e ainda não está versionado aqui.
