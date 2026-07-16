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

## ADR-010 — Sessão opaca em cookie httpOnly (não JWT)
**Contexto:** o `openapi.yaml` da fundação declarava `bearerAuth`/JWT, mas nada o
implementava — era placeholder, não decisão. Precisávamos escolher de fato.
**Decisão:** sessão **opaca** (32 bytes aleatórios) em cookie `HttpOnly` + `Secure`
+ `SameSite=Lax`. O banco guarda só o **SHA-256** do token. O securityScheme virou
`cookieAuth`.
**Consequência:**
- Revogação é **instantânea** e real: `logout`, troca de senha ou bloqueio de conta
  derrubam a sessão na requisição seguinte (o `SELECT` da sessão faz `JOIN` com
  `status = 'ACTIVE'`). Com JWT stateless isso exigiria blacklist — ou seja, estado,
  que era justamente o que o JWT prometia evitar.
- Sem segredo de assinatura para vazar ou rotacionar.
- Um dump do banco **não** permite se passar por ninguém: o token não está lá.
- O front nunca guarda token; logo, XSS não o rouba.
- Custo: um `SELECT` por requisição autenticada. Aceito — é um índice único em
  32 bytes.
- O hash da sessão é SHA-256 **sem salt**, ao contrário da senha (Argon2id). Não é
  descuido: o token tem 256 bits de entropia real e vida curta, então não há
  dicionário a construir. Salt e custo alto compensam entropia baixa de senha
  humana; aqui só custariam latência.

## ADR-011 — Vínculo com a DAV é síncrono, em duas transações curtas
**Contexto:** o cadastro só vale se a pessoa existir na Doutor ao Vivo (é onde ficam
os dados de saúde e a teleconsulta). A DAV é lenta e instável: medimos `GET` entre
0,5s e 6,3s, e `POST` batendo no teto de **29s do AWS API Gateway** na frente dela.
**Decisão:** `Register` roda em três passos — (1) TX curta reservando o CPF com a
conta em `PENDING_DAV`; (2) chamada à DAV **fora de transação**, com retry; (3) TX
curta gravando o vínculo e ativando a conta.
**Consequência:**
- "Só cadastra se tiver tudo certo lá" vale porque **`PENDING_DAV` não autentica** —
  e isso está no banco (`CHECK active_exige_vinculo_dav`), não só no código.
- Nunca seguramos uma conexão do pool durante uma chamada HTTP de dezenas de
  segundos (o que derrubaria a API sob carga).
- O passo 2 **começa pelo lookup por CPF**, então o retry é idempotente de graça:
  uma tentativa que criou a pessoa e morreu é reencontrada na seguinte. Isto não é
  teórico — aconteceu no primeiro cadastro real: os POSTs "falharam" com timeout mas
  tinham criado a pessoa, e a segunda tentativa a reconheceu.
- Se o id encontrado é o **nosso**, a origem é `CREATED`, não `ATTACHED`. Marcar
  errado mentiria na trilha de auditoria justamente no caso sensível.
- Timeout por tentativa = **30s**, deliberadamente ACIMA dos 29s do gateway: assim o
  504 *deles* chega até nós (prova que a requisição chegou lá) em vez de o nosso
  cliente desistir antes. O timeout da rota deriva de `config.DAVBudget()` — quando
  os dois números eram independentes, divergiram e cortavam a última tentativa.
  Dado real: um `POST /person` legítimo levou **19,6s** e deu 201. Com o timeout
  antigo de 10s, esse cadastro teria sido reprovado.

## ADR-011b — `CreatePerson` NUNCA repete; quem reconcilia é o model
**Contexto:** correção do ADR-011, aprendida quebrando a cara. O client repetia o
POST em 5xx/timeout. No primeiro cadastro real pelo browser: o POST estourou os 29s
(504) **mas tinha criado a pessoa**; o retry levou **409 `id already exists.`** — um
status que a sondagem não conhecia — e caiu no `default` do mapeamento, virando
"indisponível". Resultado: cadastro reprovado com a pessoa existindo na DAV, e o
usuário preso (toda nova tentativa esbarraria nela).
**Decisão:**
- **Leituras** (`FindPersonByCPF`, `GetPerson`) repetem: GET é seguro e a DAV oscila.
- **Escritas** (`CreatePerson`) **não repetem, nunca**. Falha de escrita vira
  **`dav.ErrMaybeApplied`** — "o resultado é desconhecido", nunca "falhou".
- Quem recebe `ErrMaybeApplied` **sonda** antes de concluir: `models.reconcile`
  chama `GET /person/{nosso-id}`. Achou → `CREATED`; não achou → falhou de verdade;
  sonda caiu → `PENDING_DAV` e o usuário tenta de novo.
**Consequência:** repetir POST não-idempotente era a causa raiz — é ele que
transforma sucesso em 409. A distinção "falhou" × "não sei" passou a ser explícita no
tipo do erro, não no julgamento de quem lê o código. O `GetPerson` já existia para
isto desde o início e não estava sendo usado.
**Lição:** nenhum teste com mock encontraria isso. Só rodar contra a API real, com a
lentidão real dela, encontrou.

## ADR-012 — `internal/adapters/` para sistemas externos
**Contexto:** o `docs/ARQUITETURA.md` já falava em "Adapter DAV", "Adapter Agenda" e
"Adapter Gestão", mas a tabela do `apps/api/CLAUDE.md` não tinha onde pô-los.
**Decisão:** `internal/adapters/<sistema>/`. A **interface fica no consumidor**
(ex.: `models.DAVClient`), não no adapter.
**Consequência:** quem usa declara só o que precisa e testa com fake sem subir HTTP;
abre caminho para `adapters/gestao` e `adapters/agenda` sem discussão nova.

## ADR-013 — Cadastro confiado no piloto (SEM fator de posse) — RISCO ACEITO
**Contexto:** o item 2 do escopo manda anexar a pessoa que já existir na DAV com o
mesmo CPF. Mas `GET /person/cpf/{cpf}` devolve o cadastro completo (12 campos:
e-mail, celular, endereço, nome da mãe) de qualquer pessoa, com o **CPF como única
chave** — e CPF no Brasil é dado vazado, não segredo.
**Decisão (do time, 2026-07-16):** confiar no cadastro no piloto; verificação por
WhatsApp/e-mail vem depois.
**Consequência — riscos aceitos:** quem souber o CPF de outra pessoa se cadastra e
**herda o prontuário dela**. Mitigações que já estão pagas:
- A resposta do `/auth/register` **nunca** ecoa nada vindo da DAV, nem revela se a
  pessoa já existia lá (o cliente não distingue `CREATED` de `ATTACHED`).
- Todo vínculo é gravado em **`dav_link_audit`** (origem, IP, timestamp), com índice
  parcial em `origin = 'ATTACHED'` — é o que permitirá **revisar retroativamente** as
  anexações quando o fator de posse existir.
- `patient_account.verified_at` já nasce (NULL): o fator entra sem migration nova.
- Validamos o dígito verificador do CPF e há rate limit por IP.
**Revisar antes do go-live.**
