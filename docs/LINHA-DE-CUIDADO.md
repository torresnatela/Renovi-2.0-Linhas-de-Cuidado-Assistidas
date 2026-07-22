# Linha de Cuidado — conceito, modelo de dados e regras

> Documento de referência para explicar **como a linha de cuidado foi modelada** no
> Renovi 2.0 (útil para outros times/projetos). Reconstruído a partir do código-fonte
> (`apps/api/internal/db/migrations`, `apps/api/internal/models/careline`, o contrato
> OpenAPI e o front `apps/web`). Idioma: PT-BR (convenção do repo).
>
> **Nota de precisão:** o `CLAUDE.md` da **raiz** cita o motor em
> `internal/models/eligibility` — **esse diretório não existe**. O motor real vive em
> `apps/api/internal/models/careline/`. O `apps/api/CLAUDE.md` está correto.

---

## 1. O que é uma linha de cuidado (negócio)

Uma **linha de cuidado** é o **desenho versionado de uma jornada clínica** que o
colaborador/paciente percorre: *quais* consultas (ou atividades), *com que
recorrência* e *sob quais regras*. Exemplos de domínio: "gestante", "diabetes",
"saúde mental".

A promessa do produto, em uma frase (`docs/PROGRESSO.md`): *"o colaborador vê a linha
de cuidado, o motor decide o que pode agendar, ele agenda pelo booking existente e a
jornada fica auditada."*

O ponto conceitual mais importante — e o que dá a espinha ao modelo inteiro — é a
separação em **duas naturezas**:

| | O que é | Onde vive |
|---|---|---|
| **Template** | A *definição* reutilizável e versionada da linha (itens + regras) | `care_line` / `care_line_item` / `care_line_rule` |
| **Instância** | A *jornada concreta* de um paciente naquele template | `enrollment` (matrícula) + `care_appointment` + `journey_event` |

Um template é **imutável por versão**: publicar uma revisão **não altera** a versão
anterior — cria a **próxima**. Assim, uma matrícula sabe exatamente qual desenho a
rege, mesmo depois de o admin publicar uma nova versão
(`apps/api/internal/db/migrations/0005_care_line.up.sql:5`).

---

## 2. Vocabulário de domínio

A cadeia central se lê da esquerda para a direita (template) e de cima para baixo
(instância):

```
TEMPLATE     care_line  ──1:N──▶  care_line_item  ──1:N──▶  care_line_rule
(versionado)  (code,ver)          (ref, kind)               (QUOTA, MIN_INTERVAL, …)

INSTÂNCIA    enrollment ──1:N──▶  enrollment_period   (vigências/renovações)
(por paciente)   │
                 ├──1:N──▶  care_appointment   (a consulta realizada da jornada)
                 └──1:N──▶  journey_event       (log append-only da linha do tempo)
```

- **`care_line`** — o template. Versionado por `(code, version)`; estados `draft` → `published`.
- **`care_line_item`** — um passo/etapa dentro da linha. Tem um `ref` (chave lógica
  estável entre versões, ex.: `consulta_ginecologia`, `checkin-humor-diario`) e um
  `kind`: **`CONSULTA`** (aponta para uma especialidade do legado) ou **`ATIVIDADE`**
  (executada dentro da plataforma, ex.: check-in de humor — sem especialidade).
- **`care_line_rule`** — uma regra de elegibilidade **por item**: `QUOTA`,
  `MIN_INTERVAL`, `MAX_ADVANCE`, `PREREQUISITE`, com `params` em JSONB.
- **VIGÊNCIA** — *não* é uma regra armazenada; é pré-condição da matrícula (janela
  `valid_from`/`valid_until`). O motor a avalia à parte.
- **`enrollment` (matrícula)** — amarra um paciente a uma **versão específica** do
  template, com janela de vigência. Estados: `ativa`, `pausada`, `concluida`,
  `encerrada`, `expirada`.
- **Jornada** — a projeção clínica do que aconteceu (`care_appointment`) + o log
  imutável (`journey_event`).
- **Elegibilidade** — o veredito do motor por item: `allowed` + lista de `blocks[]`
  (cada um com `rule_type`, `reason` em PT-BR e `available_from` opcional).

---

## 3. Estrutura de dados

7 tabelas em 3 blocos. Todas seguem as convenções do schema: **PK UUID v7 gerado na
aplicação**, **`TIMESTAMPTZ` sempre**, **enums via `TEXT + CHECK`** (nunca `ENUM`
nativo — evolui com um simples `ALTER … CHECK`, foi assim que `ATIVIDADE` entrou
depois).

### 3.1 Catálogo versionado

**`care_line`** (`0005_care_line.up.sql:12`)

| Coluna | Tipo / regra |
|---|---|
| `id` | UUID PK |
| `code` | TEXT — chave lógica estável entre versões (`btrim <> ''`) |
| `version` | INT ≥ 1 |
| `name`, `description` | TEXT |
| `status` | `draft` \| `published` |
| `published_at` | TIMESTAMPTZ |

Invariantes impostas **no banco**:

- `UNIQUE (code, version)` — não há duas versões iguais do mesmo code.
- `CHECK published_exige_data`: `status <> 'published' OR published_at IS NOT NULL` —
  "publicado sem data" é mentira, e o banco recusa em vez de confiar na disciplina do
  código.
- Índice **único parcial** `ux_care_line_draft (code) WHERE status='draft'` — **no
  máximo um rascunho por code**: dois admins não competem pela mesma versão nova.

**`care_line_item`** (`0005:36`, estendido por `0009_activity_item.up.sql`)

| Coluna | Tipo / regra |
|---|---|
| `care_line_id` | FK → `care_line` **CASCADE** |
| `ref` | TEXT — único por linha (`UNIQUE (care_line_id, ref)`) |
| `kind` | `CONSULTA` \| `ATIVIDADE` |
| `specialty_code` | condicional ao kind (ver abaixo) |
| `label`, `recurrence?`, `sort_order` | |

O `0009` traz um CHECK cross-coluna elegante — `specialty_por_kind`: `CONSULTA`
**exige** especialidade não-vazia; `ATIVIDADE` **tem** `specialty_code = NULL`.

**`care_line_rule`** (`0005:54`)

| Coluna | Tipo / regra |
|---|---|
| `care_line_item_id` | FK → `care_line_item` **CASCADE** |
| `rule_type` | `QUOTA` \| `MIN_INTERVAL` \| `MAX_ADVANCE` \| `PREREQUISITE` |
| `params` | JSONB (forma varia por tipo) |

> Detalhe de modelagem: `VIGENCIA` fica **deliberadamente fora** deste CHECK — ela
> pertence à matrícula (`valid_from/valid_until`), não ao item.

### 3.2 Matrícula / vigência

**`enrollment`** (`0006_enrollment.up.sql:12`)

| Coluna | Tipo / regra |
|---|---|
| `patient_id` | FK → `patient_account` **RESTRICT** |
| `care_line_id` | FK → `care_line` **RESTRICT** (amarra à **versão** exata) |
| `care_line_code` | TEXT — **redundante de propósito** (ver abaixo) |
| `status` | `ativa`/`pausada`/`concluida`/`encerrada`/`expirada` |
| `valid_from`, `valid_until` | TIMESTAMPTZ (`CHECK valid_until > valid_from`) |
| `gestao_contract_id?` | TEXT — vínculo frouxo com a Gestão, **sem FK** |

A trava-chave: índice **único parcial**
`ux_enrollment_viva (patient_id, care_line_code) WHERE status IN ('ativa','pausada')` —
**no máximo uma matrícula viva por (paciente, linha), independente da versão**. É por
isso que `care_line_code` é guardado redundante ao `care_line_id`: a trava precisa
valer **entre versões** do mesmo code. Estados terminais (`concluida`/`encerrada`/
`expirada`) liberam o paciente para nova matrícula.

**`enrollment_period`** (`0006:42`) — as janelas concedidas (renovações):
`starts_at`/`ends_at`, `source` (`admin` no piloto). A contiguidade entre períodos é
validada no caso de uso da renovação, não no banco.

### 3.3 Jornada realizada

**`care_appointment`** (`0007_care_journey.up.sql:13`) — a consulta da jornada,
amarrada à matrícula **e** ao item.

| Coluna | Tipo / regra |
|---|---|
| `enrollment_id` | FK → `enrollment` **RESTRICT** |
| `care_line_item_id` | FK → `care_line_item` **RESTRICT** |
| `booking_id` | UUID → `appointment` (0003) por **id lógico, SEM FK** |
| `status` | `agendada`/`confirmada`/`em_andamento`/`realizada`/`falta`/`cancelada` |
| `idempotency_key?` | TEXT |

Invariantes no banco: par de CHECKs irmãos `cancelada_exige_data` +
`data_exige_cancelada` (status e `cancelled_at` andam juntos); `ux_care_appt_booking
(booking_id)` (um booking técnico projeta no máximo uma consulta de jornada);
`ux_care_appt_idem (enrollment_id, idempotency_key)` parcial (idempotência sem tabela
própria).

**`journey_event`** (`0007:51`) — log **append-only** da linha do tempo (`event_type`,
`actor` ∈ `paciente/sistema/admin`, `payload` JSONB). Ordenado por UUID v7 +
`occurred_at` (paginação keyset). **Sem `updated_at`**: é append-only não por
disciplina, mas por **privilégio de banco** — o `0008` faz
`REVOKE UPDATE, DELETE ON journey_event FROM renovi_app`. Um UPDATE é recusado pelo
Postgres (SQLSTATE 42501).

### 3.4 Diagrama

```
        CATÁLOGO (imutável por versão)                MATRÍCULA / VIGÊNCIA
   ┌───────────────┐                          patient_account ──┐ (RESTRICT)
   │ care_line     │ ux_draft: 1 draft/code                     ▼
   │ (code,version)│◄──(RESTRICT)── care_line_id ──┐   ┌──────────────────┐
   └──────┬────────┘                               └──▶│ enrollment       │
     CASCADE │                                          │ care_line_code   │  ux_enrollment_viva:
          ▼                                             │ valid_from/until │  1 viva por (paciente,
   ┌───────────────┐                                    │ gestao_contract_ │   care_line_code)
   │ care_line_item│  kind: CONSULTA|ATIVIDADE          │  id ····(sem FK)·│····▶ gestao_contract
   │ ref (único)   │                                    └───┬──────────┬───┘
   └──────┬────────┘                              CASCADE │          │ RESTRICT
     CASCADE │                                             ▼          ▼
          ▼                                       enrollment_   ┌──────────────────┐
   ┌───────────────┐                              period        │ care_appointment │
   │ care_line_rule│  QUOTA|MIN_INTERVAL|                       │ booking_id ┈┈┈┈┈┈│┈┈▶ appointment (0003)
   │ params JSONB  │  MAX_ADVANCE|PREREQUISITE                  │ status (PT-BR)   │    saga DAV/legado
   └───────────────┘  (VIGENCIA fica na matrícula)             └──────────────────┘    (status técnico,
                                                                                          não vaza ao cliente)
   RESTRICT: não apaga em silêncio (auditoria/jornada dependem)  ┌──────────────────┐
   CASCADE:  filhos de agregado fechado                          │ journey_event    │ APPEND-ONLY
   ┈┈▶: referência lógica sem FK (desacopla módulos, ADR-012)    │ (REVOKE U/D 0008)│ por privilégio
                                                                 └──────────────────┘
```

---

## 4. As regras — o motor de elegibilidade puro

Este é o coração transferível. O pacote `apps/api/internal/models/careline/` é um
**motor de decisão puro**: proibido importar `db`/`net/http` ou chamar `time.Now()` —
**todo tempo e todo estado entram por parâmetro**. Isso o torna 100% determinístico e
testável por tabela sem banco nem relógio.

### 4.1 Assinatura

```go
func Evaluate(j Journey, item Item, rules []Rule, intendedAt, now time.Time) Eligibility
```

- **`j Journey`** — snapshot da matrícula: `Status`, `ValidFrom`, `ValidUntil`,
  `LineItems` e **todas** as `Appointments` (de todos os itens), + `CancelCountThreshold`.
- **`item`** — o item que se quer agendar.
- **`rules`** — as regras daquele item (`params` cru em JSON).
- **`intendedAt`** — instante pretendido do agendamento.
- **`now`** — "agora" injetado (usado só por `MAX_ADVANCE`).

Saída — `Eligibility { Allowed bool; Blocks []Block }`. O motor avalia **todas** as
regras e devolve **todos** os blocks (não faz curto-circuito): o paciente vê a lista
inteira de motivos. `Allowed == (len(Blocks) == 0)`.

Cada `Block`:

```go
type Block struct {
    RuleType      string     // VIGENCIA | QUOTA | MIN_INTERVAL | MAX_ADVANCE | PREREQUISITE
    Reason        string     // PT-BR, pronto para exibir ao paciente
    AvailableFrom *time.Time // preenchido quando destrava com o TEMPO; nil quando depende de AÇÃO
}
```

> Essa convenção do `AvailableFrom` é sutil e valiosa: **preenchido** = "espere até tal
> data" (cota semanal, intervalo mínimo); **`nil`** = "faça algo" (renovar plano,
> realizar o pré-requisito, cota `total` esgotada).

### 4.2 As cinco regras (com params JSONB)

**VIGENCIA** (pré-condição, sempre avaliada, sempre o 1º block, mas nunca
curto-circuita):

- `status ≠ ativa` → bloqueia com mensagem por estado (`pausada`/`expirada`/…).
  `AvailableFrom` nil.
- `intendedAt < valid_from` → "Seu plano inicia em DD/MM/AAAA",
  `AvailableFrom = valid_from`.
- `intendedAt > valid_until` → "Renove para agendar além dessa data", `AvailableFrom`
  nil (depende de renovar).

**QUOTA** — `{"max": ≥1, "period": "week"|"month"|"total", "window": "rolling"}`

- Janela **móvel** (rolling), durações **fixas**: `week = 7d`, `month = 30d` — nunca
  mês civil. `window:"calendar"` é rejeitada no publish. (Decisão deliberada, ADR-020:
  mês civil permitia o exploit de agendar 31/jan e 01/fev encostados.)
- Bloqueia se **alguma** janela de duração `period` contendo `intendedAt` já tem ≥
  `max` consultas que contam — inclusive uma janela ancorada numa consulta antiga
  (ignora fronteira de ciclo). Janelas semiabertas (distância exata de `period` fica de
  fora).
- `period:"total"` = limite pela vida da matrícula → bloqueio permanente
  (`AvailableFrom` nil).

**MIN_INTERVAL** — `{"days": ≥1}`

- Bidirecional: bloqueia se alguma consulta que conta (mesmo item) está a **menos** de
  `days` do `intendedAt`. Distância exata de `days` é permitida. `AvailableFrom` =
  vizinho conflitante mais tardio + intervalo.

**MAX_ADVANCE** — `{"days": ≥1}`

- Bloqueia agendar além de `days` no futuro a partir de `now`. Única regra que usa
  `now`. `AvailableFrom = intendedAt - horizonte`.

**PREREQUISITE** — `{"item_ref": "...", "status": "realizada", "within_days": ≥1}`

- Exige uma consulta do **item referenciado** com o `status` pedido dentro de uma
  **janela retroativa** `[intendedAt - within_days, intendedAt]`. Uma consulta do
  pré-requisito marcada **depois** do horário pretendido **não** satisfaz "realize
  primeiro". `AvailableFrom` nil (depende de ação). Mensagem: "Realize primeiro:
  «label»".

### 4.3 Política de contagem (o cooldown de cancelamento)

Base compartilhada de QUOTA e MIN_INTERVAL — quais consultas "consomem vaga":

- **Sempre contam**: `agendada`, `confirmada`, `em_andamento`, `realizada`, **`falta`**.
- **`cancelada`**: só **não** conta se o cancelamento foi com ≥ `threshold` de
  antecedência (default **24h**, configurável por journey). Cancelamento tardio (ou sem
  `cancelled_at`) **consome a vaga do mesmo jeito** — evita gaming.

### 4.4 A tabela de testes É a especificação

O cabeçalho do pacote declara: *"a tabela de testes T1–T19 em `evaluate_test.go` é a
especificação normativa deste slice: mudar semântica aqui exige mudar a tabela
primeiro."* É a melhor documentação executável das regras. Casos que valem citar:

- `T02` cota 4/mês estourada → `available_from` = data da mais antiga + 30d
- `T05` min_interval 7, distância exata de 7 dias **permitida**
- `T07` cancelamento tardio (2h antes) **conta** na cota
- `T13`/`T16` QUOTA + MIN_INTERVAL (ou VIGENCIA) simultâneos → **múltiplos blocks**,
  vigência primeiro
- `T18` janela móvel ignora fronteira de ciclo
- `X02` params inválidos em runtime → **falha fechada** (vira block "Regra inválida na
  configuração"), nunca regra ignorada
- `X04` pré-requisito no futuro **não** satisfaz

### 4.5 Validação na publicação (config-time)

`ValidatePublish` (`careline/publish.go`) roda no `publish` e **acumula todos** os
erros (não para no primeiro): (1) pelo menos um item; (2) params de toda regra válidos;
(3) alvo de `PREREQUISITE` existe entre os itens; (4) **sem ciclos de pré-requisito**
(DFS de 3 cores, reporta `A -> B -> A`); (5) especialidade de cada `CONSULTA` existe no
legado (comparação normalizada: maiúsculas, sem acento — `Nutrição` casa `NUTRICAO`).
`ATIVIDADE` pula a checagem de especialidade.

### 4.6 Exemplo concreto de uma linha (cenário real de teste)

A linha `saude-mental-basica` (cenário E2E) ilustra o modelo inteiro:

```
care_line: code="saude-mental-basica", version=1, status=published
├── item ref="psico"  kind=CONSULTA  specialty="Psicologia"
│     ├── rule QUOTA         {"max":4, "period":"month", "window":"rolling"}
│     ├── rule MIN_INTERVAL  {"days":7}
│     └── rule MAX_ADVANCE   {"days":30}
└── item ref="psiq"   kind=CONSULTA  specialty="Psiquiatria"
      └── rule QUOTA         {"max":1, "period":"month", "window":"rolling"}
```

Lido em português: "psicologia até 4×/mês (janela móvel), no mínimo 7 dias entre
sessões, agendável com até 30 dias de antecedência; psiquiatria 1×/mês."

---

## 5. Como a linha flui pela API

Contrato API-first em `packages/contracts/openapi.yaml` (fonte da verdade → gera os
tipos Go). MVC: **controllers finos → models (orquestração) → motor puro → db (sqlc)**.

### 5.1 Endpoints

**Admin** (monta o template; `security: adminToken`):
`POST /admin/care-lines` (draft) → `POST …/items` → `POST …/items/{ref}/rules` →
`POST …/publish` (valida tudo, 400 com `errors[]`). E matrícula:
`POST /admin/enrollments` (na última versão publicada, `months` 1–3), `…/renew`,
`…/end`.

**Paciente** (`security: cookieAuth`, escopo do próprio usuário):

- `GET /me/journey` — a tela-mãe: matrículas + itens **já avaliados** + eventos recentes.
- `GET /me/eligibility?item_id=…&date=…` — veredito de um item (puro, não toca o legado).
- `GET /me/availability?item_id=…` — slots agendáveis, **cada um anotado** com o
  veredito do motor para o instante do slot.
- `POST /me/appointments` — agenda; **header `Idempotency-Key` obrigatório**; motor
  barra → **422 `ELIGIBILITY_BLOCKED`** com `blocks[]`.
- `POST /me/appointments/{id}/cancel`, `GET /me/appointments`, `GET /me/audit`.

**Integração** (`security: integrationToken`): `POST /integration/gestao/contracts`,
`POST /integration/gestao/employees/{cpf_hmac}/resend-invite`.

### 5.2 O agendamento — ordem estrita: motor primeiro, booking depois, projeção por último

`JourneyStore.Schedule` (`models/care_journey.go`):

1. Exige `Idempotency-Key`; carrega snapshot fresco do item. **Replay**: mesma
   key/mesmo item → devolve a **mesma** consulta (HTTP 200); key reusada para outro
   item → `422 IDEMPOTENCY_KEY_REUSE`.
2. **Expiração lazy**: se a matrícula está `ativa` mas `valid_until` já passou,
   transiciona para `expirada` (evento `matricula_expirada`, actor `sistema`) **antes**
   de avaliar — o motor nunca decide sobre um status já falso. Não depende de cron.
3. **O motor decide ANTES de qualquer escrita.** `!Allowed` → `ErrNotEligible{Blocks}`
   → 422.
4. Liberado: `booking.Book(...)` executa a saga (reserva no MySQL legado por CAS +
   `POST` na DAV, **uma tentativa, sem retry**; no desconhecido, **falha fechada**).
5. Desanexa do contexto da request (`context.WithoutCancel`) e grava **projeção +
   evento `consulta_agendada` na MESMA transação**. Corrida no índice único → compensa
   o booking e devolve o vencedor.

O motor é reavaliado **no servidor** em toda leitura de jornada, disponibilidade e
agendamento — o cliente nunca é fonte de verdade de regra.

### 5.3 Front (React) — nunca recalcula, só exibe

Regra de ouro (`apps/web/CLAUDE.md`): **o front nunca recalcula elegibilidade — exibe o
veredito pronto**. A feature `journey/` renderiza a linha como uma **timeline**
ordenada por `sort_order`; cada nó é um `CareItemCard` que mostra o estado do item
(LIBERADO/BLOQUEADO/PRÉ-REQ/FEITO) e só oferece "Agendar" quando `eligibility.allowed`;
senão exibe o `reason` (que **já vem pronto em PT-BR do servidor**). Itens `ATIVIDADE`
(check-in de humor) usam outro card, cujo estado vem do check-in do dia, não da
elegibilidade. A `Idempotency-Key` nasce com a **intenção** (o clique no horário, via
`crypto.randomUUID()`) e é reusada no retry.

---

## 6. Conexões externas e como a matrícula "nasce"

`renovi_care` é o único banco que conhece os três sistemas; **externos nunca ganham
FK** — entram como `TEXT` fotografado no momento do fato.

- **Doutor ao Vivo (DAV)** — onde a teleconsulta acontece. `care_appointment.booking_id`
  referencia a saga técnica (`appointment`) por **id lógico sem FK** (ADR-012/023),
  desacoplando jornada de agendamento. Os status técnicos da saga (`PENDING_SLOT`,
  `DAV_PENDING`, `CONFIRMED`, `DAV_UNKNOWN`, …) **não vazam** ao cliente — este vê o
  vocabulário clínico (`agendada`, `realizada`, …).
- **Gestão 2.0** — origem da elegibilidade organizacional. A ingestão é **push**
  (ADR-043): a Gestão **chama** `POST /integration/gestao/contracts` (idempotente por
  `contract_id`, atrás de `X-Integration-Token`); nós persistimos empresa/pessoa/
  contrato e emitimos convite de onboarding. A pessoa é chaveada por **`cpf_hmac`**
  (HMAC-SHA256 com pepper compartilhado) — **o CPF nunca trafega**. Nunca escrevemos no
  banco da Gestão.

**Como a matrícula chega**, hoje, por dois caminhos:

1. **Automática/universal (ADR-040):** quando a conta vira `ACTIVE`, o `AccountStore`
   matricula o paciente na linha aberta **`saude-mental-aberta`** na **mesma
   transação** de ativação — idempotente e *fail-open* (seed ausente não derruba o
   cadastro). Vigência perpétua via data-sentinela `2999-12-31` (não `infinity` — pgx
   v5 não faz scan de infinity). Essa linha universal contém os 3 itens `ATIVIDADE` do
   Verificador de Humor e é **excluída** da listagem de jornada (não conta como
   "plano").
2. **Por admin:** `POST /admin/enrollments` liga o paciente à última versão publicada —
   único caminho para linhas fechadas no Slice 1.

> A coluna `enrollment.gestao_contract_id` já existe para amarrar matrícula ↔ contrato
> numa fatia futura; hoje a ingestão push só registra contratos e emite convites —
> **não abre matrícula por contrato ainda**.

---

## 7. Princípios de design transferíveis

Se o objetivo é mostrar *como foi feito*, estes são os padrões que fazem o modelo
funcionar — todos reaproveitáveis fora do Renovi:

1. **Template versionado e imutável.** Publicar cria versão nova; a instância aponta
   para a versão que a rege. Nunca edite um desenho já em uso.
2. **Motor de decisão PURO e isolado.** Sem I/O, tempo injetado por parâmetro. Ganha:
   testes table-driven exaustivos (a tabela vira a spec), reavaliação no servidor a
   cada request, e reuso por outros consumidores (o mesmo `Evaluate` serve para
   consultas **e** para a cadência do check-in de humor).
3. **Invariantes no banco, não na disciplina do código.** Enum via `TEXT+CHECK`; CHECKs
   cross-coluna (`published_exige_data`, `cancelada_exige_data`); índices únicos
   parciais para regras de negócio ("1 draft por code", "1 matrícula viva por
   paciente×linha").
4. **Append-only por privilégio.** `REVOKE UPDATE/DELETE` no role da app — auditoria à
   prova de app comprometida.
5. **Sistemas externos sem FK.** Id lógico + texto fotografado no momento do fato →
   desacopla ciclos de vida e esquemas.
6. **Interface declarada no consumidor** (ADR-012) — o model da jornada declara
   `BookingService`/`journeyStorage`, não o fornecedor.
7. **Idempotência por coluna única**, sem tabela de idempotência dedicada; a key nasce
   com a intenção do usuário.
8. **Falha fechada** em runtime (regra inválida bloqueia) e **fail-open** onde a
   ausência não pode derrubar o fluxo (auto-matrícula universal).
9. **`available_from` distingue "espere" de "aja"** — pequena decisão de contrato que
   carrega semântica rica para a UI sem lógica no cliente.

---

## 8. ADRs de referência (`docs/DECISOES.md`)

Para seguir o raciocínio: **ADR-002** (motor puro isolado), **ADR-020** (quota janela
móvel), **ADR-021** (jornada: elegibilidade no servidor + agendamento idempotente +
expiração lazy), **ADR-023** (`care_appointment` sem FK ao booking), **ADR-024**
(append-only por privilégio), **ADR-025** (idempotência por coluna única), **ADR-030**
(item `ATIVIDADE`), **ADR-034** (WHO-5/PHQ-4 reusam o motor), **ADR-040** (linha
universal + matrícula automática), **ADR-043** (ingestão push da Gestão).

---

## Fontes no código

- **Schema:** `apps/api/internal/db/migrations/0005_care_line`, `0006_enrollment`,
  `0007_care_journey`, `0008_app_role`, `0009_activity_item`, `0015_universal_mental_health`,
  `0016_gestao_ingestion`. Mapa completo em `docs/BANCO-DE-DADOS.md`.
- **Motor puro:** `apps/api/internal/models/careline/` (`careline.go`, `evaluate.go`,
  `params.go`, `publish.go`) + testes normativos `evaluate_test.go` (T01–T19, X01–X05).
- **Orquestração:** `apps/api/internal/models/care_journey.go`, `careline_catalog.go`,
  `enrollment.go`, `universal_enrollment.go`.
- **Contrato:** `packages/contracts/openapi.yaml`.
- **Front:** `apps/web/src/features/journey/`, `apps/web/src/shared/api.ts`.
