# Progresso — estado vivo do projeto

> **Todo agente atualiza este arquivo ao avançar.** É a fonte da verdade de "onde
> estamos". Marque `[x]` o que concluiu e ajuste "Próximo passo".

_Última atualização: 2026-07-15 — fundação criada._

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

## ⏳ Próximo passo (início do MVP)

**1. Implementar o motor de elegibilidade (primeiro código de negócio, TDD).**
Arquivo: `apps/api/internal/models/eligibility/eligibility.go` (hoje é stub).
Escreva a suíte table-driven antes: cota semanal (semana ISO), dependências N,
"cancelou → cota volta", `NOT_YET_OPEN`, `OVERDUE`. Ver SPEC §3.3.

## 🗺️ Backlog por fase (resumo — detalhe no SPEC §11)

### P0 (MVP) — linha de cuidado + agendamento
- [ ] Schema real de domínio (patient_account, care_line_template/item/dependency, enrollment, journey_event, appointment, idempotency_key) — migrations
- [ ] Motor de elegibilidade implementado + `GET /me/eligibility`
- [ ] Ativação de conta (token por CPF/e-mail) + Adapter Gestão (leitura)
- [ ] Adapter Agenda (legado): leitura de slots + escrita da reserva (lock pessimista)
- [ ] Fluxo de agendamento distribuído (idempotência, reconciliação) + Adapter DAV
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

- [ ] Schema real do MySQL legado (slots/escala/agenda) + como evita double-booking hoje
- [ ] Doc e credenciais da API Doutor ao Vivo (cadastro, agendamento, sala, histórico)
- [ ] Confiabilidade do CPF na tabela de colaboradores da Gestão 2.0
- [ ] Acesso de rede da VM aos bancos legado e Gestão

## ❓ Decisões de produto aguardando martelo

Ver `docs/DECISOES.md` → ADR-009.
