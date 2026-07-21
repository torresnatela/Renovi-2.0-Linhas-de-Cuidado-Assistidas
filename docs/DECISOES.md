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

**Atualização (2026-07-19) — pendência de infra resolvida na borda:** o bloco
`app.renovisaude.com.br` do Caddy de borda (`deploy/edge-snippet.Caddyfile`)
**deleta `True-Client-IP`** e **sobrescreve `X-Real-IP` com `{client_ip}`**; o
`X-Forwarded-For` de cliente não confiável o Caddy ≥ 2.5 já descarta sozinho.
Com isso o `middleware.RealIP` do chi passa a ler valor controlado pela borda e
os rate limits por IP (login/cadastro) e o IP da auditoria deixam de ser
spoofáveis. Trocar o `RealIP` deprecated por derivação com allowlist explícita
fica como melhoria futura (defesa em profundidade), não bloqueia o go-live.

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

**Consequência — LIMITAÇÃO ACEITA (corrida de quota entre keys DIFERENTES):**
a idempotência (ADR-025) só neutraliza o duplo-clique da MESMA `Idempotency-Key`.
Duas requisições `Schedule` CONCORRENTES com keys DIFERENTES **passam as duas pelo
`Evaluate` antes de qualquer uma commitar o `CreateScheduled`** — cada uma lê um
estado da jornada em que a outra consulta ainda não existe. Resultado: o paciente
pode acabar com **max+1 numa janela de QUOTA**, ou **furar o MIN_INTERVAL** entre
duas consultas do mesmo item. É uma corrida real, não coberta pelo índice único
(que casa por key, e as keys são distintas). **Por que aceita:** serializar de
verdade (um lock por matrícula segurado do `Evaluate` até o commit) conflita com a
disciplina de **não segurar lock atravessando a chamada de ~29s à DAV** — o
`Book` é lento e insondável, e prender uma linha esse tempo todo estrangularia o
agendamento e arriscaria conexões presas. O **rate limit por conta**
(`rateLimitByAccount`) REDUZ a janela — dois POSTs precisam chegar quase juntos —
mas **não elimina**. **Opção do Slice 2:** reavaliar a elegibilidade DENTRO da TX
do `CreateScheduled`, contra um snapshot `FOR UPDATE` das consultas da matrícula,
e **compensar o booking** (`BookingStore.Cancel`) quando a segunda a commitar
estourar a regra — exatamente como a corrida da MESMA key já compensa hoje.

**Revisão pós-review (2026-07-19):**

- **Renovação decide por TEMPO, não pelo flag `status`.** A expiração lazy só roda
  nas leituras da JORNADA do paciente; o caminho ADMIN (`EnrollmentStore.Renew`)
  nunca a dispara. Uma matrícula podia então estar `ativa`/`pausada` com
  `valid_until` no passado, e renovar de forma CONTÍGUA a partir desse
  `valid_until` velho gerava um período INTEIRO no passado (o paciente pagava por
  dias já vencidos, sem que o CHECK `valid_until > valid_from` pegasse). `Renew`
  agora reativa a partir de `now` sempre que a vigência já venceu
  (`status == expirada` **ou** `valid_until <= now`). A afirmação "o cron vira
  otimização, não requisito de corretude" só se sustenta porque essa decisão
  passou a ser por tempo — antes era falsa no admin. Coberto por
  `TestRenew_MatriculaVencidaSemMarcarExpirada_ReativaDeNow`.
- **`Idempotency-Key` vinculada ao ITEM (ver ADR-025).** Reusar a MESMA key para
  outro item da matrícula devolvia a consulta ERRADA como replay 200; agora vira
  `422 IDEMPOTENCY_KEY_REUSE`.
- **Seção crítica pós-`Book` desanexa do ctx da requisição** (`context.WithoutCancel`):
  gravar a projeção e compensar a corrida de key rodavam no ctx do request, então
  uma DESCONEXÃO do cliente (o duplo-clique num POST lento) deixava o booking REAL
  órfão e travava o retry da mesma key em `ErrSlotTaken`. Com o detach a projeção e
  a compensação completam mesmo sem o cliente. (Um CRASH do processo nessa janela
  continua sem cobertura — só uma *intent* persistida ANTES do `Book` resolveria, o
  que é mudança de schema; fica para o Slice 2.)
- **`available_from` de QUOTA em janela super-lotada** (`n > max`) usava a consulta
  mais antiga da janela + period — um instante que ainda estaria bloqueado. Agora
  usa a `(n-max+1)`-ésima mais antiga da janela de âncora mais antiga (com `n == max`
  é idêntico ao anterior, o que preserva a tabela T e o E2E semanal). Coberto por
  `X05_quota_janela_super_lotada...` e pela mutação da janela do cenário C.

## ADR-022 — Admin por token estático (RISCO ACEITO)

**Contexto:** o Slice 1 não tem back-office. As rotas `/admin/care-lines*` e
`/admin/enrollments*` (montar catálogo, publicar, matricular/renovar/encerrar)
são operadas por gente do time via `curl`/`apps/api/docs/slice1.http`, fora de
banda — nunca pela sessão do paciente.

**Decisão:** um único token estático no header `X-Admin-Token`
(`controllers/admin_middleware.go`), comparado em tempo constante
(`crypto/subtle.ConstantTimeCompare`) com `RENOVI_ADMIN_TOKEN`. **Token vazio =
rotas desligadas**: o middleware recusa qualquer requisição (inclusive header
vazio, que casaria por acaso num compare de dois vazios). Ausente e errado
respondem IGUAL (401 `ADMIN_TOKEN_INVALID`), para não dar oráculo pelo tempo nem
pela mensagem. **Não há role de operador no schema** — o token é o único fator; o
admin não é um usuário do banco.

**Consequência — risco aceito:** é um segredo compartilhado, sem rotação, sem
escopo por operação e **sem auditoria de QUEM operou** (o efeito fica no
`journey_event` com `actor='admin'` genérico — `enrollment.go`). Mesma dívida do
ADR-013 (fator de posse): aceitável no piloto, **revisar quando houver
back-office** com login de operador. O token NUNCA é logado (`config.LogValue` só
expõe `admin_token_set`).

## ADR-023 — `care_appointment` separado da saga de booking (referência lógica, sem FK)

**Contexto:** a tabela `appointment` (migration 0003) é o **espelho técnico** da
saga MySQL+DAV do agendamento (ADR-011/016). A jornada clínica tem vocabulário
(status em PT-BR) e ciclo de vida próprios, e não deve acoplar seu esquema ao da
saga.

**Decisão:** `care_appointment` (migration 0007) é a projeção clínica: status
PT-BR (`agendada/confirmada/em_andamento/realizada/falta/cancelada`) e
**`booking_id UUID NOT NULL` SEM FK** para `appointment`. A garantia "um booking,
uma consulta de jornada" vem do índice único `ux_care_appt_booking`, não de uma
FK que acoplaria os dois esquemas. O booking é consumido atrás da interface
`BookingService`, **declarada no consumidor** (`models/care_journey.go`, ADR-012)
e implementada pelo `*BookingStore` real.

**Consequência — LIMITAÇÕES CONHECIDAS:**
- **(a) Espelhos podem divergir** se a TX final falhar depois do `Book`: em
  `Schedule`, se `CreateScheduled` falha após o booking já existir, há uma
  consulta REAL na DAV sem projeção de jornada. Não há compensação segura
  genérica — fica log ERROR com os ids (filosofia falha-fechada: gente resolve).
- **(b) `CANCELLED` com slot retido fica órfão.** A fila do worker
  (`ListPendingSlotRelease`, `queries/scheduling.sql`) hoje varre **só `FAILED`**
  com `slot_held_at` e sem `slot_released_at`. Mas `releaseCancelledSlot`
  (`appointment.go`) pode deixar uma consulta **`CANCELLED` com `slot_held_at` e
  sem `slot_released_at`** quando o `ReleaseSlot` no legado falha — e essa linha
  **não** entra na fila varrida. Fica órfã até o worker do Slice 2 cobrir
  `CANCELLED`+held+not-released (o comentário do código já promete essa varredura;
  a query ainda não a faz).

## ADR-024 — Role `renovi_app` e `journey_event` append-only por PRIVILÉGIO de banco

**Contexto:** o `journey_event` é a trilha de auditoria da jornada. Append-only
por desenho (sem `updated_at`) não basta: um bug — ou um app comprometido — que
emita `UPDATE`/`DELETE` reescreveria a história.

**Decisão:** a migration 0008 cria o role `renovi_app` (num bloco `DO $$` — o DDL
de role é string opaca para o sqlc) com os privilégios de DML e então **`REVOKE
UPDATE, DELETE ON journey_event`**. A aplicação em runtime conecta como
`renovi_app` (`RENOVI_CARE_DATABASE_URL`); as **migrations** rodam como OWNER
(`renovi`), porque criam tabelas e o próprio role — via
`RENOVI_CARE_MIGRATE_DATABASE_URL`, que **cai em `RENOVI_CARE_DATABASE_URL`**
quando não definida (`config.go`; `cmd/migrate` usa a de migração, `cmd/api` a do
app).

**Consequência:** a trilha fica à prova de app comprometida — um `UPDATE`/`DELETE`
em `journey_event` recebe SQLSTATE 42501, no banco, não por disciplina de código
(mesmo espírito do CHECK `active_exige_vinculo_dav` e do grant do legado, ADR-014).
Custo: a senha do role na migration é de DEV (bate com o compose/`.env`); em
staging/produção o operador roda `ALTER ROLE renovi_app PASSWORD ...` com um
segredo fora do controle de versão — **documentado no README do slice**
(`apps/api/docs/SLICE1.md`).

## ADR-025 — Idempotência do agendamento por coluna UNIQUE (sem tabela própria)

**Contexto:** o `POST /me/appointments` fala com a DAV, que é lenta e insondável;
o duplo-clique criaria duas consultas REAIS. O ADR-021 já registra o comportamento
em runtime (motor antes do booking, replay 200, corrida compensada). Esta ADR
registra a **decisão de schema** e o **limite** da idempotência.

**Decisão:** nenhuma tabela de idempotency-keys. A chave mora numa **coluna** —
`care_appointment.idempotency_key` — com **índice único PARCIAL por matrícula**
(`ux_care_appt_idem (enrollment_id, idempotency_key) WHERE idempotency_key IS NOT
NULL`, migration 0007). Replay da mesma key devolve a MESMA consulta com 200. A
corrida de dois requests com a mesma key é decidida pelo próprio índice dentro do
`CreateScheduled`; o perdedor **compensa o booking que criou** (`BookingStore.Cancel`,
log ERROR se falhar) e devolve o vencedor como replay. A corrida é reconhecida
PELO NOME da constraint — outra unique violation é bug, não replay.

**Consequência:** a idempotência garante **um agendamento de jornada por key**;
ela **não** torna a escrita na DAV idempotente — `POST /appointment` continua
não-idempotente e insondável, e o fail-closed do ADR-011b/016 segue INTACTO
(quem estoura no `Book` sobe o erro; a jornada não reescreve nem retenta a DAV).
Parcial porque a maioria das linhas não traz key (nulos não colidem).

**Revisão pós-review (2026-07-19):** o replay é VINCULADO ao item. A key vale por
operação; reusá-la para OUTRO item da mesma matrícula devolvia a consulta do item
A quando o cliente pediu o item B (confirmação de agendamento silenciosamente
errada). O `Schedule` agora confere `care_appointment.care_line_item_id` contra o
item pedido antes de replayar e, se divergir, responde `422 IDEMPOTENCY_KEY_REUSE`.

## ADR-026 — Deploy: GitHub Actions + GHCR + compose numa VPS compartilhada

**Contexto:** ir a produção em `app.renovisaude.com.br` usando a VPS Hostinger
já existente (2.25.184.35, AlmaLinux 10, 1 vCPU/4 GB), que roda outros dois
sistemas da empresa (o "renovi" antigo e o renovi-gestao) atrás de um Caddy de
borda comum (projeto `/opt/renovi`, portas 80/443). Requisitos: build no GitHub,
segredos no GitHub, não afetar os vizinhos nem o SSH.

**Decisão:**
- **Pipeline única** (`.github/workflows/ci.yml`): testes → (PR) build-check das
  imagens sem push → (main, com **aprovação manual** no environment
  `production`) build+push no **GHCR** (`renovi-care-api`/`renovi-care-web`,
  tags `<sha>` + `latest`) → deploy via **SSH** (chave dedicada; host key
  fixada) → `deploy/deploy-remote.sh` na VPS. `workflow_run` foi descartado
  (mesmo SHA e environment ficam naturais com `needs` no mesmo workflow).
- **Migrations antes do `up -d`**: se falharem, a versão antiga segue no ar.
- **Projeto isolado `renovi-care`** em `/opt/renovi-care` (o nome `renovi` já
  pertence ao sistema antigo): rede própria + entrada na rede externa
  `renovi_default` só com aliases `care-api`/`care-web`; portas de smoke
  `127.0.0.1:8083/8084`; `mem_limit` e log com rotação (padrão da VM).
- **A imagem web embarca o dist do artifact do CI** (não rebuilda no Docker):
  o que foi testado é o que vai pro ar.
- **1 instância de API sempre** (rate limiter em memória — ADR-019). Escalar
  exige antes mover o rate limit para armazenamento compartilhado.
- **Rollback por tag**: `IMAGE_TAG=<sha anterior>` no `.env` da VPS + `up -d`;
  imagens de 7 dias mantidas na VM (`prune --filter until=168h`).
- Concurrency do CI: PRs cancelam runs antigos; na main **enfileira**
  (`cancel-in-progress` condicional) — cancelar na main mataria deploy no meio.

**Consequência:** deploy auditável e reversível com um clique de aprovação;
custo: dependência do GHCR e do SSH da VPS. Runbook em `docs/DEPLOY.md`.

**Revisão (2026-07-20):** a 1ª conexão SSH do job (o `scp`) era feita sem retry e
estourava `connect ... port 22: Connection timed out` quando o anti-DDoS da borda
Hostinger descartava o SYN do IP efêmero do runner — intermitente, **não** allowlist
(os outros sistemas da VPS deployam pelo mesmo caminho; o `renovi_saude_publica`
já repetia a 1ª conexão). Adicionado **warm-up com retry** (6× · `ConnectTimeout=10`)
antes do `scp`, sem mudança de firewall nem de postura de segurança.

## ADR-027 — Banco de produção no Neon (Postgres 17), endpoint direto

**Contexto:** o `renovi_care` de produção precisa de Postgres gerenciado com
backup; a VPS de 4 GB compartilhados não é lugar para o banco do piloto.

**Decisão:** **Neon** (Postgres 17, TLS obrigatório). A URL vai em
`RENOVI_CARE_DATABASE_URL` com `?sslmode=require` — o pgx usa o DSN como está,
zero mudança de código. **Endpoint direto (host sem `-pooler`) para tudo**: o
endpoint pooled (PgBouncer em transaction pooling) quebra o advisory lock do
golang-migrate, e na escala do piloto (1 instância, pool pequeno do pgx) o
pooler não agrega nada. Autosuspend aceito (cold start ~1 s pós-ociosidade).
A separação de roles do ADR-024 vale no Neon: a app conecta como `renovi_app`
(criado pela migration 0008; senha trocada via `ALTER ROLE` no provisionamento)
e as migrations como o owner, via `RENOVI_CARE_MIGRATE_DATABASE_URL`.

**Consequência:** backup/PITR e upgrades por conta do Neon; latência app↔banco
vira dependência de internet (mitigada por timeouts já existentes). Se um dia
houver muitas instâncias, revisitar o pooler (aí migrations continuam no direto).

## ADR-028 — Borda: vhost aditivo no Caddy existente; Cloudflare DNS-only

**Contexto:** as portas 80/443 da VPS pertencem ao Caddy de borda do projeto
`/opt/renovi`, que já serve os outros dois sistemas e detém o TLS de todos.

**Decisão:**
- O Renovi 2.0 entra como **mais um bloco de site** (`app.renovisaude.com.br`)
  no `/opt/renovi/Caddyfile` — cópia versionada em
  `deploy/edge-snippet.Caddyfile`. Mudanças na borda: sempre aditivas, com
  backup `Caddyfile.bak.<ts>`, `caddy validate` antes do reload gracioso, e
  edição **in-place** (o arquivo é bind-mount único; `mv` troca o inode).
- **Cloudflare em DNS only** (nuvem cinza): o Caddy emite/renova Let's Encrypt
  via HTTP-01 sozinho — zero gestão de certificado. Nuvem laranja é topologia
  diferente (cert de origem, headers CF) e exigiria novo ADR.
- O container `web` (`renovi-care-web`) tem um Caddy **interno** só de SPA
  estática; roteamento de path e headers de segurança (HSTS etc.) ficam no
  bloco da borda, seguindo o padrão da VM (é assim que a gestao funciona).

**Consequência:** um único ponto de TLS na VM; o Renovi 2.0 depende do
container de borda do projeto antigo (aceito: já era verdade para os demais).
Se a borda um dia for extraída para um projeto próprio "edge", todos os blocos
migram juntos (fora do escopo).

---

> **Nota de numeração — Verificador Diário de Humor (Anexo C).** A feature do
> Anexo C foi desenvolvida no branch `feat/verificador-humor` (ramo do Slice 1).
> O Slice 1 usou ADR-021..025; para não colidir, os ADRs do Verificador de Humor
> começam em **ADR-030**.

## ADR-030 — `care_line_item.kind` ganha ATIVIDADE (item não-consulta)

**Contexto:** o Verificador Diário de Humor (Anexo C) é a primeira ATIVIDADE de
uma linha de cuidado: uma execução DENTRO da plataforma (check-in de humor),
sem especialidade do legado nem agendamento na DAV. A fundação do Slice 1 travava
`care_line_item.kind` em `CHECK (kind IN ('CONSULTA'))` e exigia `specialty_code`
NOT NULL — moldado só para consulta.

**Decisão:**
- Migration `0009_activity_item` estende o vocabulário: `kind IN ('CONSULTA',
  'ATIVIDADE')` e torna `specialty_code` **condicional ao kind** via CHECK
  `specialty_por_kind` (CONSULTA exige especialidade não vazia; ATIVIDADE tem
  `NULL`). Invariante no banco, não em disciplina de código.
- No domínio/motor, `SpecialtyCode` segue `string` (`""` = sem especialidade); a
  conversão para `NULL`/`pgtype.Text` fica na fronteira `gen`, como já é feito com
  `recurrence`. Assim o contrato de resposta admin não muda (atividade retorna
  `specialty_code: ""`) e o ripple é mínimo.
- `careline.ValidatePublish` **pula** a checagem de especialidade quando
  `item.Kind == ATIVIDADE` — a tabela normativa T1–T19 permanece intacta.

**Consequência:** o catálogo agora comporta atividades da linha; o Verificador de
Humor entra como itens `ATIVIDADE` (`checkin-humor-diario`, `who5-semanal`,
`phq4-gatilhado`). O único resíduo é `specialty_code: ""` na resposta admin de uma
atividade (em vez de omitido) — aceitável no piloto, refinável depois.

## ADR-031 — Consentimento LGPD: titular é o paciente, um ativo por finalidade

**Contexto:** o Verificador de Humor (Anexo C) exige consentimento livre e
informado (LGPD art. 11, dado sensível) como pré-condição de gravação. O Anexo C
modela `consent` com `colaborador_id`/`empresa_id` pseudonimizados. No renovi_care,
porém, o sujeito autenticado é o **paciente** (`patient_account`), e não há
entidade de colaborador/empresa integrada (a Gestão 2.0 é somente leitura e ainda
não conectada).

**Decisão:**
- `consent.patient_id` referencia `patient_account` (ON DELETE RESTRICT: a trilha
  de consentimento sobrevive). `empresa_id` do Anexo vira `gestao_contract_id`
  opcional (preenchido a partir da matrícula quando houver).
- **Allowlist de finalidade:** só finalidades conhecidas (`checkin_humor`) são
  aceitas no model — não se grava consentimento para propósito arbitrário.
- **Um ativo por (paciente, finalidade):** índice único parcial
  `ux_consent_ativo ... WHERE status='ativo'`. Reconceder é versionado: mesmo
  `versao_termo` é idempotente (não recria); versão nova revoga o anterior e cria
  um novo, numa transação. Revogação é idempotente.
- `Active(patient, finalidade)` devolve `ErrNoActiveConsent` quando não há ativo —
  é o gate que as capturas (Módulos 3–5) consultam antes de persistir; sem ele,
  respondem 403.

**Consequência:** o consentimento é rastreável e revogável, com histórico
preservado (linhas revogadas não são apagadas). O front do check-in coleta o
aceite do termo antes da primeira captura; sem consentimento ativo, nada é gravado.

## ADR-032 — Pontuação pura com cortes versionados como reference data

**Contexto:** o Verificador de Humor pontua grade (quadrante), WHO-5 e PHQ-4. O
Anexo C exige pontuação DETERMINÍSTICA e cortes da validação BRASILEIRA mantidos
como parâmetro configurável — nunca constante de código.

**Decisão:**
- Pacote **puro** `internal/models/mood/scoring` (sem I/O, sem `time.Now()`):
  `Quadrant`/`IsQuadranteRisco` (circumplexo, corte determinístico em 50, >= é o
  lado alto), `ScoreWHO5` (índice = bruto×4), `ScorePHQ4` (subescalas PHQ-2/GAD-2).
  Os **cortes entram por parâmetro** (`WHO5Cutoffs`/`PHQ4Cutoffs`). A tabela de
  testes é a especificação, como no motor `careline`.
- Cortes, dimensões e polaridades vivem em **reference data VERSIONADA**
  (`instrument`/`instrument_dimension`/`instrument_cutoff`), semeada na migration
  `0011` com os valores BR (WHO-5 <50/<28 — de Souza & Hidalgo 2012; PHQ-4
  subescala ≥3 — Santos 2013/Moreno 2016; total ≥6 — Kroenke 2009). Mudar um corte
  = migration nova, não deploy de código.
- Paleta e vocabulário (rótulos de emoção por quadrante) são **próprios da Renovi**
  — a nomenclatura do Mood Meter é marca da Yale (Anexo C.4).

**Consequência:** o algoritmo (pacote puro) e os parâmetros (banco) têm fontes da
verdade separadas: o Módulo 4 carregará os cortes do banco e os passará ao scorer.
Instrumentos são semeados em toda migração/ambiente (inclusive testcontainers),
então o feature funciona sem passo de seed manual.

## ADR-033 — O "1 por dia" do check-in é o dia LOCAL (America/Sao_Paulo)

**Contexto:** o anel diário aceita uma resposta por dia (atualizável). "Dia" tem
de ser o dia do colaborador no Brasil, não o dia UTC: um check-in às 22h de
Brasília (01h UTC do dia seguinte) tem de contar como HOJE, não amanhã.

**Decisão:**
- `mood_checkin.dia_ref DATE` é calculado na APLICAÇÃO a partir de `respondido_em`
  convertido para `America/Sao_Paulo` (`models.localDay`). A unicidade é
  `ux_mood_checkin_dia (patient_id, dia_ref)`; o upsert é `ON CONFLICT
  (patient_id, dia_ref)`. Não se usa `respondido_em::date` (que dependeria do fuso
  da sessão do banco e quebraria na fronteira da meia-noite).
- Pré-condição do check-in é DERIVADA sob demanda dos fatos imutáveis
  (`FindActivityEnrollment`: matrícula ativa+vigente numa linha publicada que
  contém o item `checkin-humor-diario`), não materializada. O anel diário NÃO usa
  o motor de agendamento (não trava); só as pré-condições de matrícula + consentimento.
- O comentário livre cifrado foi **adiado** (sem infra de cifra ainda): MVP grava
  só dado estruturado (coordenadas, quadrante derivado, rótulo, context_tags) —
  princípio de minimização (LGPD).

**Consequência:** a série temporal é do dia vivido pelo colaborador. O front usa
paleta e vocabulário PRÓPRIOS da Renovi na grade (não os do Mood Meter/Yale) e
nunca recalcula o quadrante — exibe o que o servidor derivou. Rotas `/me/*` só
sobem quando o Auth está montado (dependem de RequireSession), então a verificação
no browser exige o stack de dev com credenciais DAV.

## ADR-034 — WHO-5/PHQ-4 reusam o motor de cadência via fatos de atividade

**Contexto:** os anéis periódicos (WHO-5 semanal, PHQ-4 gatilhado) têm cadência
mínima (`MIN_INTERVAL`). Reimplementar a regra seria duplicar o que o motor puro
`careline` já faz — e divergir dele. Mas o motor foi escrito para CONSULTAS
(`JourneyAppointment`), e sua tabela T1–T19 é normativa.

**Decisão:**
- O `AssessmentStore` monta uma `careline.Journey` cujos `Appointments` são os
  fatos de execução da ATIVIDADE (aplicações passadas do instrumento) mapeados
  para o shape `JourneyAppointment{ItemRef, Status: "realizada", ScheduledAt:
  respondido_em}`, e chama `careline.Evaluate(journey, item, rules, now, now)`.
  Assim `MIN_INTERVAL` (e futuramente QUOTA) "simplesmente funcionam" — **sem
  alterar o motor nem a tabela T1–T19**. As regras vêm de `care_line_rule` do item.
- A vigência (VIGENCIA) é avaliada pelo mesmo motor, com `valid_from/valid_until`
  da matrícula (`FindActivityEnrollmentDetail`). Se não há matrícula elegível, o
  store devolve uma `Eligibility` não-permitida com um bloco VIGENCIA — a
  elegibilidade continua **derivada sob demanda dos fatos imutáveis**.
- **Alternativa preterida:** generalizar `JourneyAppointment`→`JourneyFact` no
  motor. Exigiria reescrever a especificação normativa T1–T19; adiada até haver
  ganho que justifique.
- A pontuação (WHO-5) usa os cortes carregados de `instrument_cutoff` (ADR-032),
  passados ao pacote puro `scoring`. `flag_encaminhar` (índice < 28) marca o
  rastreio positivo que o Módulo 6 roteia à trilha clínica.

**Consequência:** um só motor decide cadência para consultas e atividades. O anel
diário (que não trava) fica de fora do motor de propósito; só os periódicos o usam.

## ADR-035 — Gatilho de aprofundamento fora-de-banda e puro (não no motor)

**Contexto:** a deterioração sustentada no anel diário deve OFERECER o WHO-5;
WHO-5 sinalizando oferece o PHQ-4; PHQ-4 positivo escala à trilha clínica
(Anexo C.5.4). Isso NÃO é elegibilidade de agendamento — é "oferecer aprofundar".
Havia duas modelagens (C.9.1): novo `rule_type` no motor `careline` vs. avaliador
fora-de-banda.

**Decisão (recomendada e adotada):** avaliador **fora-de-banda**, em pacote PURO
`internal/models/mood/trigger`, separado do motor `careline`:
- `Evaluate(Snapshot, Params) State` — máquina de estados NORMAL / OFERECER_WHO5 /
  OFERECER_PHQ4 / ESCALAR_CLINICO. Precedência: o anel mais PROFUNDO respondido
  recentemente decide; sem resposta recente, `RiskStreak ≥ N` oferece o WHO-5.
  Parâmetro `N` (dias consecutivos em risco) = **default 4**, versionável.
- O `Snapshot` é DERIVADO sob demanda do histórico imutável (sequência de dias em
  quadrante de risco no `mood_checkin` + últimas aplicações de WHO-5/PHQ-4 dentro
  de uma janela de 14 dias). O `MoodCheckinStore.Today` monta o Snapshot e expõe
  `offer`/`escalate` — nada materializado (sem `trigger_state`).
- Justificativa: manter o motor de agendamento (e a tabela normativa T1–T19)
  intocado; o gatilho tem semântica e ciclo próprios. Cortes entram via a `faixa`
  já pontuada (ADR-032/034), não reimplementados.

**Consequência:** `escalate=true` (PHQ-4 positivo) roteia à trilha CLÍNICA — nunca
ao gestor (Módulo 6). A tabela de testes de `trigger` é a especificação; mudar a
máquina de estados começa por ela.

## ADR-036 — Roteamento de crise e escalonamento vão à trilha clínica, nunca ao gestor

**Contexto:** o anel diário NÃO é detector de crise (sinaliza tendência). Mas o
módulo precisa de (a) uma afordância permanente "preciso de ajuda agora" e (b)
escalonamento de rastreio positivo — ambos SEMPRE ao canal clínico/urgência,
jamais ao gestor (guardrails 6.1/6.2/6.5 do documento central; Anexo C.5.5).

**Decisão:**
- `POST /me/mood/help-now` registra `pedido_ajuda` na jornada (quando há matrícula)
  e devolve um `HelpChannel` (`type: care_navigation`) — triagem, não tratamento.
  Sem informação de contato falsa no MVP: o front roteia pelo `type`.
- Rastreio positivo (`flag_encaminhar`: WHO-5 índice < 28 / PHQ-4 subescala ≥ corte)
  emite `escalonamento_clinico` (`actor: sistema`) na MESMA transação do
  `assessment_respondido`. A trilha clínica efetiva entra quando existir; hoje grava
  o fato/flag (auditoria) e o `Today.escalate` expõe o sinal ao paciente.
- **Muralha:** todo evento em `journey_event` é escopado ao paciente
  (`patient_id NOT NULL`) e append-only (role sem UPDATE/DELETE, 0008). Não há
  superfície agregada/gestor no schema — a camada agregada/anonimizada (índice
  coletivo, k-anonimato) é documento próprio (C.8) e não recebe dado individual.

**Consequência:** o dado individual do Anexo C nunca transita para o gestor por
construção (não existe caminho). O escalonamento é um fato na trilha clínica do
paciente; a integração de roteamento real é o próximo passo (ver PROGRESSO).

## ADR-037 — Correções pós-review (PR #13) do Verificador de Humor

**Contexto:** a revisão da PR #13 apontou três ajustes de correção/robustez sobre
os ADRs 034/035, sem mudar o contrato nem o schema.

**Decisão:**
- **Streak = dias de CALENDÁRIO consecutivos (ADR-035).** O gatilho contava
  *check-ins* em risco em sequência, ignorando lacunas de dia — quem só registrava
  nos dias ruins acumulava um streak falso. `riskStreak` (helper puro em
  `mood_checkin.go`, table-driven) agora quebra a sequência numa lacuna de dia,
  fiel a "N dias consecutivos" (Anexo C.5.4).
- **Guard de concorrência no `Submit` do assessment (ADR-034).** A cadência era
  checada FORA da transação (janela TOCTOU: dois submits simultâneos podiam gravar
  duas aplicações dentro do `MIN_INTERVAL`). Agora o `Submit` adquire um
  `pg_advisory_xact_lock` por (paciente, instrumento) no início da transação e
  **reavalia a cadência dentro dela** — o segundo submit vê a aplicação já
  commitada e é barrado. Coberto por teste de integração determinístico.
- **Nits:** `History` capa o limite em 120 (teto do contrato) em vez de zerar para
  30; `getMoodHistory` (front) passa `limit`; grade de humor operável por teclado
  (setas), com teste; comentário fixando a invariante dos cortes de subescala do
  PHQ-4 (as duas linhas `subescala_positiva` devem compartilhar o corte).

**Consequência:** o gatilho fica fiel ao spec, o anel semanal ganha um backstop de
concorrência no servidor (além do botão desabilitado no front) e a captura diária
fica acessível por teclado.

### Segunda rodada — revisão do CodeRabbit (PR #13)

O CodeRabbit revisou o push das correções acima e apontou 10 itens; os válidos
foram aplicados:

- **Cortes na conexão da tx (Major):** `AssessmentStore.score`/`who5Cutoffs`/
  `phq4Cutoffs` liam por `s.q` (pool) enquanto o `Submit` segurava a tx + advisory
  lock — pediria uma 2ª conexão por Submit (risco de pool-starvation sob
  concorrência). Passam a receber o `q` da tx.
- **Streak precisa ser RECENTE (Major):** `riskStreak` agora exige que o check-in
  mais novo seja hoje ou ontem — um streak antigo e interrompido não oferece mais o
  WHO-5 indefinidamente (o `Snapshot` fala em dias "recentes").
- **Consentimento serializado (Major):** `Grant`, `Revoke` e o `Record` do check-in
  passam a compartilhar um `pg_advisory_xact_lock` por (paciente, finalidade), e as
  pré-condições do `Record` foram movidas para DENTRO da tx. Fecha a janela em que
  uma revogação se intrometeria entre a checagem e a gravação, e evita a
  unique-violation crua de duas concessões concorrentes.
- **Acessibilidade (Major):** a grade anuncia o ponto escolhido por uma região
  `aria-live` (leitor de tela); botões-toggle (Likert e tags de contexto) ganham
  `aria-pressed`.
- **Nits:** o controller valida/capa o `limit` do histórico (controller fino); a
  resposta de `getMoodHistory` ganha `maxItems: 120` no contrato (sem drift de
  código gerado — é validação, não muda o codegen).
- **Bug de teste pré-existente (achado na verificação):** `TestMoodCheckinStore_Fluxo`
  usava `time.Now()` e assumia `now` e `now+1h` no mesmo dia local — perto da
  meia-noite de Brasília caíam em dias diferentes (2 linhas). Ancorado a um `now`
  fixo ao meio-dia UTC.
- **Bug de teste pré-existente (achado no CI Linux, E2E do Slice 1):**
  `requireInstant` comparava instantes por igualdade EXATA; a coluna `timestamptz`
  trunca a microssegundo, mas o payload de evento (RFC3339Nano) guarda nanossegundos
  — no runner Linux (onde `time.Now()` tem resolução de nanossegundo) a comparação
  falhava por uma diferença que o banco sequer distingue. Passa a truncar a
  microssegundo (a tolerância zero correta para instantes ancorados no banco).

**Não aplicado — `CHECK` sem `NOT VALID` nas migrations (o CodeRabbit marcou como
Crítico):** trocar o `CHECK` de `journey_event` sem `NOT VALID` faz um scan sob
`ACCESS EXCLUSIVE` (bloqueia escrita). Porém o `golang-migrate` (usado via
`NewWithSourceInstance`) roda cada arquivo de migration como UMA transação
implícita, então `ADD ... NOT VALID` + `VALIDATE` no mesmo arquivo mantêm o
`ACCESS EXCLUSIVE` por toda a transação — **não** entregam zero-downtime. O fix real
exigiria dividir cada extensão de `event_type` em duas migrations (add / validate),
desproporcional para o piloto (tabela `journey_event` minúscula). Registrado como
follow-up se/quando a tabela crescer.

## ADR-040 — Verificador de Humor para todos (Degrau 1) via linha de cuidado universal + matrícula automática (2026-07-20)

**Contexto:** o Verificador de Humor (GRID diário, WHO-5, PHQ-4) nasceu como
**Degrau 2** — só funciona para quem tem matrícula ativa numa linha de cuidado que
contenha os itens de atividade. O produto passou a exigir saúde mental **para todo
colaborador, com ou sem plano** (o "Degrau 1", antes adiado). O obstáculo é
estrutural: `mood_checkin`, `wellbeing_assessment` e `journey_event` amarram
`enrollment_id` **e** `care_line_item_id` como **NOT NULL** — toda escrita de humor
pressupõe uma matrícula.

**Decisão:**
- **Linha de cuidado universal, não desacoplamento.** Em vez de tornar as FKs
  opcionais (mudança grande no schema, no motor e em dezenas de testes), semeamos uma
  **linha publicada `saude-mental-aberta`** (migration `0015`) com os 3 itens
  ATIVIDADE (`checkin-humor-diario`, `who5-semanal`, `phq4-gatilhado`) e matriculamos
  **todo colaborador** nela. O modelo de dados, o motor puro, a jornada, o gatilho e a
  suíte de testes ficam intactos. Humor continua sendo uma ATIVIDADE de linha de cuidado.
- **Matrícula automática.** Contas novas: hook em `AccountStore.commitLink`, **na
  mesma transação** de ativação (idempotente e *fail-open* — seed ausente não bloqueia
  o cadastro). Contas existentes: **backfill** na própria `0015` (só contas `ACTIVE`,
  com `NOT EXISTS`).
- **Vigência PERPÉTUA via data-sentinela (`2999-12-31`), não `infinity`.** pgx v5 não
  faz scan de `infinity` em `time.Time`, e o `valid_until` entra no motor puro. Com a
  sentinela, o `vigenciaBlock` nunca bloqueia e a expiração lazy nunca dispara —
  **zero mudança no motor** (mantém a pureza). Descartadas: auto-renovação por cron
  (worker ainda é stub) e caso-especial no motor (violaria a pureza).
- **A linha universal NÃO é "plano".** Fica **fora** da listagem da jornada, filtrada
  em `JourneyRepo.ListEnrollmentsByPatient` (só no wrapper Go — `SnapshotByItem`/
  `labelIndex` seguem enxergando-a para a auditoria). Assim o perfil mantém "Sem plano
  ativo" para quem não tem matrícula real, e o card de humor (endpoint
  `/me/mood/today`, separado da jornada) continua liberado.
- **Consentimento LGPD permanece.** `finalidade = checkin_humor` (art. 11) segue como
  pré-condição de gravação — liberar por plano não é liberar por consentimento.
- **Cadência preservada.** WHO-5/PHQ-4 levam `MIN_INTERVAL` (7d/14d) na linha
  universal; o check-in diário **não** leva regra — o "1 por dia" é do upsert em
  `(patient_id, dia_ref)`.

**Consequência:** qualquer conta ativa, após consentir, faz GRID + WHO-5 + PHQ-4 sem
matrícula em linha real. O ramo `not_enrolled`/`NotEnrolledCard` do front fica como
fallback defensivo (raro: seed ausente). A reversão da `0015` é **forward-only** após
o primeiro check-in (FKs `ON DELETE RESTRICT` protegem o dado de saúde). Migration
validada contra Postgres 16 (seed, backfill idempotente, cadeia de FKs e down).
A auto-matrícula emite um `matricula_criada` (actor=`sistema`) no `journey_event`, que
aparece em `/me/audit` como o evento **mais antigo** da conta — consistente com os
eventos de humor (`checkin_humor_registrado`/`assessment_respondido`), que também são
gravados na matrícula universal. Os E2E de auditoria (`scenario_a` A22, `scenario_c`
C10) contam esse evento no total.
