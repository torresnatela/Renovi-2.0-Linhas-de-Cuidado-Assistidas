# Decisões de Arquitetura (ADRs)

Registro curto de decisões técnicas. Formato: **contexto → decisão → consequência**.
Novas decisões: adicione ao fim, numere em sequência, nunca reescreva o histórico
(marque como *Substituída por ADR-N* se mudar de ideia).

---

## ADR-001 — MVC pragmático em vez de hexagonal
**Contexto:** o SPEC propunha hexagonal (ports/adapters/app/domain); o time pediu
"MVC simples, sem overengineering".
**Decisão:** camadas `controllers → models → db`. Sem `ports`/`app`/adapters formais.
**Consequência:** menos arquivos e cerimônia; boundaries por pacote, não por interface.
Se um caso de uso crescer, extrai-se função no `models`, não uma nova camada.

## ADR-002 — Motor de elegibilidade puro e isolado (exceção ao MVC)
**Contexto:** a regra de elegibilidade é o núcleo do produto e precisa de testes exaustivos.
**Decisão:** vive em `internal/models/eligibility`, **sem I/O** (recebe template,
eventos e `now` por parâmetro).
**Consequência:** testável com tabelas de casos, sem mocks. É a única "ilha" isolada
do MVC — proposital.

## ADR-003 — sqlc + pgx v5 (não ORM)
**Contexto:** precisamos de SQL explícito (append-only, partial index, JSONB) e tipagem.
**Decisão:** SQL escrito à mão em `internal/db/queries`; sqlc gera Go tipado em `internal/db/gen`.
**Consequência:** zero mágica de ORM; o custo é rodar `make generate` ao mudar queries.

## ADR-004 — PostgreSQL para o renovi_care; MySQL confinado ao legado
**Contexto:** ecossistema novo é Postgres; o único MySQL é o legado da escala.
**Decisão:** banco próprio em PostgreSQL; MySQL acessado só via Adapter Agenda.
**Consequência:** ganhos de JSONB/GIN, DDL transacional, UUID/TIMESTAMPTZ nativos.

## ADR-005 — Idioma: código em inglês, docs em PT-BR
**Decisão:** identificadores/commits em inglês; documentação, comentários de domínio
e mensagens de log de negócio em português.
**Consequência:** padrão de mercado no código; docs acessíveis ao time BR.

## ADR-006 — Escopo da fundação = "só estrutura"
**Contexto:** primeiro entregável deve destravar o desenvolvimento, não entregar features.
**Decisão:** monorepo + infra + CI + testes + docs, **sem** implementar o motor de
elegibilidade nem endpoints de negócio. Uma tabela `example_widget` demonstra o padrão.
**Consequência:** o motor e as rotas do MVP são o próximo passo (ver `PROGRESSO.md`),
já com "casa" pronta.

## ADR-007 — golang-migrate como biblioteca embutida
**Contexto:** evitar depender de um binário `migrate` instalado na máquina/CI.
**Decisão:** migrations embutidas (`//go:embed`) e executadas via `cmd/migrate`
(driver pgx v5). Nenhuma CLI externa.
**Consequência:** binário autossuficiente; `make migrate-up` funciona em qualquer lugar.

## ADR-008 — Ferramentas de geração via `go run tool@version`
**Contexto:** manter o grafo de dependências do módulo enxuto.
**Decisão:** sqlc e oapi-codegen rodam via `go run <pkg>@<versão fixada no Makefile>`,
não como `tool` no go.mod.
**Consequência:** `go.mod` só tem deps de runtime; versões das ferramentas ficam no Makefile.

## ADR-009 — Decisões de produto ASSUMIDAS (aguardam martelo — SPEC §10)
Assumidas para não travar o design; **confirmar antes do go-live**:
- **Janela de cota = calendário civil** (semana ISO, mês civil), sem acúmulo.
- **Auto-conclusão:** consulta não cancelada conta como concluída (no-show real só em P1).
- **Antecedência mínima de cancelamento:** sugerido 4h (abaixo disso, cota não volta).
- **Disparo de ativação no piloto:** manual pelo admin (não sync automático).

Quando batido o martelo, atualize aqui e no SPEC.
