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
conta em `PENDING_DAV`; (2) chamada à DAV **fora de transação** (com retry apenas
nas LEITURAS — ver ADR-011b); (3) TX
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

---

## ADR-014 — Escrita no legado: só `tb_slots.booked`, e a permissão garante

**Contexto:** o ADR-004 dizia "leitura de slots + escrita **restrita** à tabela de
agendamento". Ao levantar o schema real (2026-07-16), a premissa caiu: o mock que
o repo tinha era inventado, e o `tb_appointments` real não tem unique nem FK
ligando consulta a horário.

**Decisão (do time):** a nossa plataforma escreve **apenas** `tb_slots.booked`.
Não inserimos em `tb_appointments`. A consulta vive só no `renovi_care`. É mais
restrito que o ADR-004 — considere aquele trecho **substituído por este**.

E a restrição não é disciplina: o usuário do banco só tem `SELECT` + `UPDATE
(booked, updatedAt)` em `tb_slots` (ver `deploy/mysql-legacy/init.sql`). Um INSERT
em `tb_appointments` recebe "ERROR 1142 command denied" — no dev, na hora. Mesmo
espírito do CHECK `active_exige_vinculo_dav`: a regra que importa mora no banco.

**Consequência — risco aceito:** o médico recebe a consulta **pela DAV** (ele é
participante MMD e é notificado por lá), mas ela **não aparece na agenda do Renovi
legado**, que verá apenas um horário ocupado sem dono. Precisa ser combinado com
quem opera o legado antes do go-live.

**Por que `booked` basta:** medido na HML — o app legado vira `booked=1` ao
agendar em 84 das 85 consultas ativas, e nenhum horário jamais teve duas consultas
ativas. Logo é um interlock de verdade, ainda que só por comportamento.

---

## ADR-015 — A reserva do horário é um CAS, não `SELECT … FOR UPDATE`

**Contexto:** o desenho original previa lock pessimista. O schema real não tem
constraint nenhuma protegendo o horário: `booked` é um flag solto.

**Decisão:** reservar com compare-and-set —
`UPDATE tb_slots SET booked=1 WHERE id=? AND booked=0` — e decidir por
`RowsAffected`.

**Consequência:** uma ida ao banco em vez de duas, atômica sob InnoDB, sem janela
entre ler e decidir e sem lock aberto enquanto pensamos. `FOR UPDATE` só se
pagaria se precisássemos ler outras colunas **dentro** do lock.

São **duas travas contra adversários diferentes**, e precisamos das duas:

| Trava | Defende contra |
|---|---|
| CAS no MySQL | o **app legado** (e nós mesmos, em último caso) |
| Índice único parcial `ux_appointment_slot_vivo` no Postgres | **dois pacientes nossos** |

A DAV **não** ajuda: ela aceita dois appointments no mesmo horário para o mesmo
profissional (achado #17). Não há terceira rede. Por isso o teste de concorrência
(12 goroutines, exatamente 1 vencedor) não é zelo — é o que sustenta a afirmação.

---

## ADR-016 — O agendamento falha FECHADO: no desconhecido, o horário não volta

**Contexto:** o ADR-011b ensinou que escrita na DAV que estoura é "desconhecido",
não "falhou", e que quem recebe isso **sonda** antes de concluir. No agendamento a
sondagem **não existe**, e isso foi medido (`make dav-probe`, achados #12/#15/#17/#20):

- `POST /appointment` **recusa id nosso** (400 "property id should not exist") —
  ela aceita em `/person` e `/professional`, mas não aqui. O id é dela e só chega
  na resposta.
- **Não há rota de busca/listagem** de appointment. Sondei a última saída
  plausível, `GET /professional/{id}/schedule`: devolve disponibilidade, não
  consultas.
- `PUT /appointment/{id}/cancel` responde **500**. Não dá nem para desfazer.
- Ela **aceita sobreposição** para o mesmo profissional.

Ou seja: se a DAV não responde, a consulta pode existir e **nunca saberemos**.

**Decisão:** na dúvida, **segura o horário**. O estado `DAV_UNKNOWN` retém o slot,
ninguém repete (repetir criaria uma segunda consulta real) e a consulta entra em
fila de revisão humana. Para o paciente ela aparece como `UNCONFIRMED` — **não é
escondida**, porque ele pode ter uma consulta de verdade marcada.

Isto está gravado como CHECK (`desconhecido_nao_libera`), não como comentário:
devolver o horário deixaria outro paciente marcar em cima de uma consulta fantasma,
e a DAV não barraria. **Perder um horário é problema operacional; double-booking é
problema clínico.**

**Consequência:** vaza horário quando a DAV oscila, e alguém precisa olhar a fila.
Aceito — e é o mesmo tipo de resíduo que o legado já produz sozinho (26 consultas
canceladas na HML ficaram com `booked=1`).

**Isto não é teórico.** No **primeiro agendamento real** deste código, o
`POST /appointment` morreu em **29,2s** no teto do AWS API Gateway. O horário ficou
retido, a consulta ficou `DAV_UNKNOWN` e o paciente recebeu 502 dizendo a verdade.
Exatamente como o ADR-011b nasceu — só que desta vez a armadilha já estava mapeada.

**Chamado a abrir na DAV** (temos o `trace` dos três): aceitar id do integrador no
`POST /appointment`; o 500 do cancel; e o GET devolvendo 500 em vez de 204.

---

## ADR-017 — O link da sala é capacidade, não dado

**Contexto:** o paciente entra na teleconsulta até 30 min antes. A forma óbvia
seria devolver a url em `GET /appointments`.

**Decisão:** o payload da consulta leva o **estado** da janela
(`join.status` + `join.opens_at`); a url sai **só** do `POST /appointments/{id}/join`,
e só com a janela aberta pelo relógio **do servidor**.

**Consequência:**

- A regra dos 30 minutos deixa de ser decoração: com a url no payload, bastaria
  abrir o DevTools — ou ter o relógio errado — para entrar a qualquer hora.
- O cache do cliente não guarda N links de teleconsulta que ninguém pediu.
- É POST porque não é leitura pura (o acesso vai alimentar a auto-conclusão) e
  porque POST não entra em cache de proxy, histórico nem prefetch de link.
- **"30 minutos" não existe no contrato nem no front**: viaja `opens_at`, já
  calculado. Mudar a antecedência é `RENOVI_JOIN_OPENS_BEFORE`, sem deploy do
  front. Verificado ponta a ponta: com `48h`, a janela abriu.
- O struct `models.Appointment` **não tem** o campo da url, de propósito: se
  tivesse, mais cedo ou mais tarde alguém o serializaria numa listagem.

---

## ADR-018 — Adapter da agenda escrito à mão, sem sqlc (exceção ao ADR-003)

**Contexto:** o ADR-003 manda usar sqlc. O MySQL legado é a exceção.

**Decisão:** `internal/adapters/agenda` usa `database/sql` à mão.

**Consequência:** o valor do sqlc é conferir a query contra o **schema**. Do lado
do Postgres, o schema são as nossas migrations — ele se paga. No legado, o "schema"
seria `deploy/mysql-legacy/init.sql`, que é uma **cópia** de um banco que não é
nosso e que não podemos migrar. Apontar codegen para ele inverte a relação: a cópia
vira autoridade, e no dia em que a produção divergir o `generate-check` continua
verde. Isso não é segurança, é **aparência** de segurança.

Em troca vieram dois controles melhores: um teste de integração que executa
**todas** as queries contra o schema real (o que o sqlc faria) e um que enumera as
colunas de que dependemos via `information_schema` (o que o sqlc nunca faria, e que
dá para apontar para uma réplica num job noturno).

O fuso entra na mesma conta: as colunas são `DATETIME` ingênuo em
`America/Sao_Paulo`, e o adapter **força** `parseTime=true&loc=America/Sao_Paulo`
em vez de confiar no DSN — recusando subir se alguém pedir outro fuso. Com o
default (UTC), 09:00 viraria 06:00 em silêncio.

---

## ADR-019 — Rate limit do agendamento por CONTA; RealIP por header é risco aberto

**Contexto:** revisão de segurança do PR #11 (CodeRabbit + passe adversarial)
apontou que o `middleware.RealIP` do chi (usado em `internal/http/router.go`)
sobrescreve o IP do cliente a partir de cabeçalhos que o próprio cliente controla
(`True-Client-IP`, `X-Real-IP`, `X-Forwarded-For`), **sem allowlist de proxy
confiável**. O chi v5.3.1 marca essa função como *deprecated: vulnerable to IP
spoofing*. O `Caddyfile` da topologia (Caddy → API) não remove esses cabeçalhos
de entrada. Como o `rateLimitByIP` e o IP da auditoria do vínculo
(`dav_link_audit`) leem esse valor, um atacante que rotaciona o cabeçalho:
- fura o rate limit de **brute-force de login** (a senha é o único fator — não há
  2FA), de **enumeração de CPF** no cadastro e de **flood de agendamento**;
- **falsifica o IP** que vai para a auditoria LGPD do vínculo.

**Decisão (parcial, aplicada agora):** a rota mais cara e perigosa
(`POST /appointments`) passa a ser limitada por **CONTA da sessão**
(`rateLimitByAccount`), não por IP. A conta vem da sessão validada
(`RequireSession` roda antes), então **não é spoofável** — o flood de agendamento
fica fechado independentemente do RealIP. É também mais justo sob NAT corporativo.

**Decisão (pendente, precisa de infra):** trocar o `middleware.RealIP` por
derivação de IP com **proxy confiável explícito** (o hop que o Caddy de fato
adiciona ao `X-Forwarded-For`, da direita para a esquerda) **e** configurar o
Caddy para **remover** `True-Client-IP`/`X-Real-IP` de entrada. Enquanto isso não
for feito, os limites de **login** e **cadastro** (rotas não autenticadas, que
só têm o IP como chave) continuam contornáveis, e o IP da auditoria continua
forjável. **Tratar antes do go-live**, junto do ajuste do `deploy/Caddyfile`.

**Consequência:** o agendamento está protegido; auth e a auditoria não, até o
ajuste do Caddy. Registrado aqui para não se perder — é o maior item aberto de
segurança do piloto, ao lado do fator de posse do ADR-013.

## ADR-020 — Quota das linhas de cuidado é janela MÓVEL GERAL, não mês civil

**Contexto:** o Slice 1 (Linhas de Cuidado) precisa de um motor de elegibilidade
puro (`internal/models/careline/`) que decida se o paciente pode agendar um item
num instante. A regra mais delicada é a QUOTA "N por mês": mês civil cria o
exploit clássico de fronteira (4 consultas na última semana de agosto + 4 na
primeira de setembro = 8 em 15 dias).

**Decisão:**
- **Janela móvel GERAL:** `QUOTA {max, period}` bloqueia se **existe alguma**
  janela de duração `period` (week=7d, month=30d — durações fixas) contendo o
  `intendedAt` com ≥ max consultas que contam. Não são só as duas janelas
  ancoradas no `intendedAt`: uma janela começando numa consulta antiga também
  bloqueia (caso normativo T18). Janelas são semiabertas — distância exata de
  `period` fica fora (T17). `period=total` = vida da matrícula, bloqueio
  permanente. `window:"calendar"` é **rejeitada no publish**.
- **Contagem de canceladas:** cancelou com ≥ 24h de antecedência
  (`CancelCountThreshold`, configurável por journey) → a vaga volta; cancelamento
  tardio, falta e status ativos contam. Vale para QUOTA e MIN_INTERVAL.
- **Composição sem curto-circuito:** vigência da matrícula é pré-condição sempre
  avaliada, mas NÃO impede as demais regras de rodarem — a resposta traz TODOS os
  `Blocks` (vigência primeiro), cada um com `Reason` PT-BR e `AvailableFrom`
  quando o desbloqueio é por tempo (nil = depende de ação: renovar, realizar
  pré-requisito).
- **Falha fechada:** params de regra inválidos em runtime viram Block, nunca
  regra ignorada.

**Consequência:** a tabela T1–T19 de `evaluate_test.go` é a especificação
normativa do slice — mudança de semântica começa por ela. O motor é O(n²) na
quota, aceitável para listas de consultas de uma matrícula. O front nunca
recalcula regra: exibe `Reason`/`AvailableFrom` que o motor mandou.

**Supersede o ADR-009 (parcial):** a decisão assumida "janela de cota =
calendário civil (semana ISO/mês civil, sem acúmulo)" caiu — o martelo bateu em
janela móvel. Os demais pontos do ADR-009 (auto-conclusão, antecedência de
cancelamento, ativação manual) seguem valendo até serem martelados.

## ADR-021 — Jornada do paciente: elegibilidade no servidor, agendamento idempotente com compensação, expiração lazy

**Contexto:** a Fase 6 do Slice 1 liga o motor puro (`models/careline`) ao
booking existente (`models/appointment.go`) nas rotas `/me/*`. Três problemas
inevitáveis: (1) o front poderia "confiar" no veredito que ele mesmo exibiu;
(2) o POST de agendamento fala com a DAV (lenta, insondável) e o duplo-clique
criaria duas consultas REAIS; (3) matrícula vencida só é verdade depois que
alguém a expira — e não há cron ainda.

**Decisão:**
- **Elegibilidade é SEMPRE reavaliada no servidor**, no instante do slot
  (`Evaluate(intendedAt = slot.StartsAt)`), ANTES de tocar o booking. Motor
  barrou → 422 com `blocks[]` completos e reason `ELIGIBILITY_BLOCKED`; o front
  só exibe.
- **Idempotência por `Idempotency-Key`** (header obrigatório, 400 sem ele) via
  índice único parcial `ux_care_appt_idem (enrollment_id, idempotency_key)`.
  Replay → 200 com a MESMA consulta. **Corrida** de duas requisições com a mesma
  key: o índice escolhe o vencedor dentro do `CreateScheduled`; o perdedor
  **compensa o booking que criou** (`BookingStore.Cancel`, log ERROR se falhar)
  e devolve o vencedor como replay. A corrida é reconhecida PELO NOME da
  constraint — outra unique violation é bug, não replay.
- **Expiração LAZY**: toda leitura da jornada que encontra matrícula `ativa` com
  `valid_until` no passado a expira na hora (`ExpireEnrollment` + evento
  `matricula_expirada` actor=`sistema`, na MESMA TX, `FOR UPDATE` contra
  corrida; idempotente — 0 linhas = sem evento). O futuro cron do worker vira
  otimização, não requisito de corretude.
- **Toda escrita da jornada grava linha+evento na MESMA transação**
  (`consulta_agendada`, `consulta_cancelada`, `consulta_status_forcado`), com
  payloads como structs nomeadas (contrato de auditoria). O cancelamento grava o
  bookkeeping (`hours_before`, `counts_for_quota` — a MESMA semântica do
  `counts()` do motor, `dav_cancelled`/`dav_error`); dessincronia com o booking
  (já cancelado lá) é tolerada com log ERROR: o paciente não fica preso.
- **`/internal/appointments/{id}/force-status`** só EXISTE quando
  `RENOVI_TEST_ENDPOINTS` (proibido em produção pela config); sem sessão — o
  gate é o ambiente. Ganhou a única query por id sem dono
  (`GetCareAppointment`), exclusiva dessa rota.
- **Rate limit do POST /me/appointments por CONTA** (`rateLimitByAccount`,
  mesmos parâmetros do POST /appointments), pela razão do ADR-019 — a rota é
  autenticada e IP sob NAT pune inocentes.

**Consequência:** `JourneyStore` declara as duas interfaces que consome
(ADR-012): `BookingService` (implementada pelo `*BookingStore` real — a jornada
agenda PELO booking, um store só) e `journeyStorage` (implementada por
`JourneyRepo` e por um fake em memória nos testes). O 422 de elegibilidade, o
replay 200 e a compensação da corrida têm testes de unidade; atomicidade
linha+evento, keyset de auditoria com empate de `occurred_at` e a expiração
idempotente têm testes de integração contra Postgres real.
