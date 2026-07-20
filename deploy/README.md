# deploy/

Infraestrutura do Renovi 2.0.

## Desenvolvimento local

`docker-compose.yml` sobe os três bancos do ecossistema como containers locais:

| Serviço | Banco | Porta host | Papel |
|---|---|---|---|
| `pg-care` | Postgres `renovi_care` | 5432 | Nosso banco (escrita/leitura) |
| `pg-gestao` | Postgres `gestao` (mock) | 5433 | Gestão 2.0 — leitura (adapter) |
| `mysql-legacy` | MySQL `renovi_legacy` (mock) | 3306 | Escala/slots legado (adapter) |

`mysql-legacy/init.sql` é **cópia fiel do schema real**, extraída de `homl_renovi`
em 2026-07-16 (só as tabelas do agendamento; o banco real tem 34). Os dados são
sintéticos e as datas dos slots são **relativas a `CURDATE()`** — slot com data
fixa apodrece, que é o estado atual da HML de verdade (nenhum horário futuro
desde 03/2025).

Duas coisas nele merecem atenção:

- **A trava do double-booking é comportamental, não estrutural.** O schema real
  não tem unique nem FK ligando consulta a slot: `booked` é um flag solto. O que
  segura é o app legado virar `booked=1` ao agendar (medido: 84 de 85 consultas
  ativas na HML) — logo o adapter reserva com CAS (`UPDATE ... WHERE booked = 0`)
  e confere `RowsAffected`.
- **O usuário `renovi` é rebaixado pelo próprio init.sql** para `SELECT` em tudo +
  `UPDATE (booked, updatedAt)` só em `tb_slots`. É o ADR-004 virando regra do
  banco em vez de promessa: um `INSERT` em `tb_appointments` recebe
  "ERROR 1142 command denied" na hora, no dev, e não em produção.

`pg-gestao/init.sql` ainda é **inventado** — schema plausível para desenvolver o
adapter antes do levantamento (SPEC §9.3). **Não representa o schema real.**

```bash
make up            # sobe os bancos
make migrate-up    # aplica migrations no renovi_care
make down          # derruba tudo
```

A API e o worker rodam **fora** do compose, via `go run` (`make migrate-up`, etc.),
apontando para os bancos acima (ver `.env.example`).

## Produção (VPS Hostinger compartilhada)

Guia completo (arquitetura, secrets, rollback, runbook): **`docs/DEPLOY.md`**.
Arquivos de produção neste diretório:

| Arquivo | Papel |
|---|---|
| `docker-compose.prod.yml` | Stack `renovi-care` na VPS (`/opt/renovi-care`): serviços `web` + `api` |
| `deploy-remote.sh` | Script que o job `deploy` executa na VPS (pull → migrate → up → readyz) |
| `Caddyfile` | Caddy **interno** do container `web` — só serve a SPA (`/srv/web`) |
| `edge-snippet.Caddyfile` | Referência do bloco `app.renovisaude.com.br` aplicado no Caddy de **borda** da VPS (`/opt/renovi/Caddyfile`, projeto preexistente — mudanças sempre aditivas) |

O banco de produção é o **Neon** (Postgres 17 gerenciado) — nada de Postgres
próprio no compose de prod. O build da imagem `web` usa a raiz do repo como
contexto e espera `apps/web/dist` pronto (no CI vem do artifact do job `web`;
localmente rode `make web-build` antes).
