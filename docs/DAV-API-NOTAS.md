# Notas da API Doutor ao Vivo (DAV) — achados da sondagem

> **Gerado por `make dav-probe`** (`apps/api/internal/adapters/dav/probe_test.go`).
> Não edite à mão: rode a bateria de novo.

- Ambiente sondado: `https://api.v2.hom.doutoraovivo.com.br`
- Data: 2026-07-16 18:30:08 -03:00

O spec publicado (`/person/_/api-docs`) se contradiz em pontos que mudam o
adapter. Esta é a fonte da verdade sobre o comportamento REAL.

## Resumo

| # | Pergunta | Veredito |
|---|---|---|
| 1 | Lookup de CPF inexistente devolve o quê? | HTTP **204** — confirma o spec. `FindPersonByCPF` deve mapear 204 → não encontrado. |
| 2 | `status` é mesmo obrigatório no POST /person? | HTTP **201** — `status` é **opcional na prática** (os exemplos estão certos; o `required` do spec mente). Mandamos sempre, mesmo assim. |
| 3 | `cpf` é opcional no POST /person, como diz o spec? | HTTP **400** — `cpf` é obrigatório na prática (o spec diz que é opcional). Bom: o lookup por CPF cobre toda a base. |
| 4 | A DAV aceita um `id` gerado por nós (UUIDv7)? | HTTP **201** — **aceito e confirmado** pelo GET. `POST /person` fica sondável → idempotência via `GET /person/{nosso-id}`. |
| 5 | A DAV rejeita CPF duplicado? | HTTP **422** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` (nunca retriável). O lookup por CPF é determinístico. |
| 6 | E-mail duplicado é rejeitado? | HTTP **422** — e-mail **é único na DAV**. ⚠️ Risco de produto: duas pessoas com o mesmo e-mail (casal, e-mail compartilhado) — a segunda não se cadastra. Precisa de mensagem própria na UI. |
| 7 | A DAV valida o dígito verificador do CPF? | HTTP **422** — a DAV valida o DV. Nossa validação continua valendo: falha rápido, sem gastar um round-trip lento. |
| 8 | `address.city` aceita nome do município ou exige código IBGE? | **Ambos funcionam.** Usar o **nome** da cidade — dispensa lookup de código IBGE no cadastro. |
| 9 | Que PII o `GET /person/cpf/{cpf}` expõe? | HTTP **200**, **12 campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. Nada disso pode sair na resposta do nosso `/auth/register`. |
| 10 | Qual a latência real da DAV? | `GET` p50 **488ms** · máx **604ms** (n=8). `POST` mediana **1.88s** · máx **2.058s** (n=3). |
| 11 | Repetir o POST com o MESMO id devolve o quê? | HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete. |

## Evidências

### 1. Lookup de CPF inexistente devolve o quê?

**Veredito:** HTTP **204** — confirma o spec. `FindPersonByCPF` deve mapear 204 → não encontrado.

Requisição: `GET /person/cpf/{cpf-sintetico-inexistente}`

Resposta:

```json
(corpo vazio)
```

### 2. `status` é mesmo obrigatório no POST /person?

**Veredito:** HTTP **201** — `status` é **opcional na prática** (os exemplos estão certos; o `required` do spec mente). Mandamos sempre, mesmo assim.

Requisição: `POST /person` **sem** `status`:

```json
{
  "birth_date": "1990-05-14",
  "cpf": "51345127014",
  "id": "019f6cd5-484b-781e-b595-1330269dd3c0",
  "name": "RENOVI PROBE Minimo"
}
```

Resposta:

```json
{
  "id": "019f6cd5-484b-781e-b595-1330269dd3c0"
}
```

### 3. `cpf` é opcional no POST /person, como diz o spec?

**Veredito:** HTTP **400** — `cpf` é obrigatório na prática (o spec diz que é opcional). Bom: o lookup por CPF cobre toda a base.

Requisição: `POST /person` **sem** `cpf`:

```json
{
  "birth_date": "1990-05-14",
  "id": "019f6cd5-5032-75ff-ac93-e942eec758b5",
  "name": "RENOVI PROBE Sem CPF",
  "status": true
}
```

Resposta:

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "934ff1d48dc05c489a120d3b0fd0f3ff5822cbc8",
  "detail": [
    {
      "message": "cpf must match ^\\d{3}\\d{3}\\d{3}\\d{2}$ regular expression"
    },
    {
      "message": "CPF is mandatory when email is not provided"
    },
    {
      "message": "cpf must be a string"
    }
  ]
}
```

### 4. A DAV aceita um `id` gerado por nós (UUIDv7)?

**Veredito:** HTTP **201** — **aceito e confirmado** pelo GET. `POST /person` fica sondável → idempotência via `GET /person/{nosso-id}`.

Requisição: `POST /person` com `id` = UUIDv7 nosso (`019f6cd5-51e5-74f1-b4e2-6c685a87ed7a`):

```json
{
  "birth_date": "1990-05-14",
  "cpf": "79885694811",
  "id": "019f6cd5-51e5-74f1-b4e2-6c685a87ed7a",
  "name": "RENOVI PROBE Id Proprio",
  "status": true
}
```

Resposta:

```json
{
  "id": "019f6cd5-51e5-74f1-b4e2-6c685a87ed7a"
}
```

Confirmação: `GET /person/019f6cd5-51e5-74f1-b4e2-6c685a87ed7a` → **200**

```json
{
  "id": "019f6cd5-51e5-74f1-b4e2-6c685a87ed7a",
  "name": "RENOVI PROBE Id Proprio",
  "email": "79885694811@dav.med.br",
  "cpf": "79885694811",
  "birth_date": "1990-05-14",
  "timezone": "America/Sao_Paulo",
  "father_not_informed": false,
  "mother_not_informed": false,
  "status": true
}
```

### 5. A DAV rejeita CPF duplicado?

**Veredito:** HTTP **422** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` (nunca retriável). O lookup por CPF é determinístico.

Duas pessoas, mesmo CPF (`74279479674`), **e-mails diferentes** — para que uma eventual
recusa seja pelo CPF, e não pelo e-mail sintetizado.

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Person invalid",
  "trace": "369b7618dbd2d9b792395d41b9712837729119d8",
  "i18n": {
    "phrase": "entity.validation.exception",
    "mustache": {
      "entity": "Person"
    }
  },
  "detail": [
    {
      "message": "A person with same CPF already exists",
      "i18n": {
        "phrase": "entity.unique.attribute.already.exists",
        "mustache": {
          "field": "cpf"
        }
      }
    }
  ]
}
```

### 6. E-mail duplicado é rejeitado?

**Veredito:** HTTP **422** — e-mail **é único na DAV**. ⚠️ Risco de produto: duas pessoas com o mesmo e-mail (casal, e-mail compartilhado) — a segunda não se cadastra. Precisa de mensagem própria na UI.

Duas pessoas, **CPFs diferentes**, mesmo e-mail (`renovi-probe+0704c4a8@example.com`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Email já cadastrado.",
  "trace": "6bf482225e3413fc3b54a1d44b2ba9d405b45786"
}
```

### 7. A DAV valida o dígito verificador do CPF?

**Veredito:** HTTP **422** — a DAV valida o DV. Nossa validação continua valendo: falha rápido, sem gastar um round-trip lento.

Requisição: `POST /person` com `cpf` = `12345678900` (DV inválido).

Resposta:

```json
{
  "code": 422,
  "message": "Person invalid",
  "trace": "cffbbe129f2893ac70e955dac69ac2b1f8c43819",
  "i18n": {
    "phrase": "entity.validation.exception",
    "mustache": {
      "entity": "Person"
    }
  },
  "detail": [
    {
      "message": "Invalid CPF",
      "i18n": {
        "phrase": "attribute.is.not.valid",
        "mustache": {
          "field": "cpf"
        }
      }
    }
  ]
}
```

### 8. `address.city` aceita nome do município ou exige código IBGE?

**Veredito:** **Ambos funcionam.** Usar o **nome** da cidade — dispensa lookup de código IBGE no cadastro.

Por nome (`"Barueri"`): HTTP **201**

```json
{
  "id": "019f6cd5-741f-7128-9e7a-bd8141960f48"
}
```

Por código IBGE (`3505708`): HTTP **201**

```json
{
  "id": "019f6cd5-7efb-71b9-9331-1575941494b9"
}
```

### 9. Que PII o `GET /person/cpf/{cpf}` expõe?

**Veredito:** HTTP **200**, **12 campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. Nada disso pode sair na resposta do nosso `/auth/register`.

Campos devolvidos: `address`, `birth_date`, `cell_phone`, `cpf`, `email`, `father_not_informed`, `id`, `mother_name`, `mother_not_informed`, `name`, `status`, `timezone`

Resposta completa:

```json
{
  "id": "019f6cd5-871d-70a0-a551-2f1d3fc8174d",
  "name": "RENOVI PROBE Pii",
  "email": "renovi-probe+2690a0e5@example.com",
  "cpf": "07960769109",
  "birth_date": "1990-05-14",
  "cell_phone": "11912345678",
  "address": {
    "country": "BR",
    "number": "238",
    "city": "Barueri",
    "street": "Avenida Copacabana",
    "state": "SP",
    "neighborhood": "Dezoito do Forte",
    "zip_code": "06472000"
  },
  "timezone": "America/Sao_Paulo",
  "father_not_informed": false,
  "mother_name": "Mae Da Probe",
  "mother_not_informed": false,
  "status": true
}
```

### 10. Qual a latência real da DAV?

**Veredito:** `GET` p50 **488ms** · máx **604ms** (n=8). `POST` mediana **1.88s** · máx **2.058s** (n=3).

**O teto é do AWS API Gateway, não da DAV.** A sondagem já pegou um `POST /person`
morrer em ~29s com `{"message": "Endpoint request timed out"}` — o limite rígido de
integração do API Gateway. Consequências para o adapter:

- Timeout por tentativa **acima de ~29s é inútil**: o gateway desiste antes.
- Um **504 não significa que falhou** — a criação pode ter chegado ao backend deles.
  Por isso o retry precisa sondar com `GET /person/{nosso-id}` antes de repetir o POST
  (viável graças ao achado #4).
- O orçamento total precisa caber no `RENOVI_HTTP_WRITE_TIMEOUT` (hoje **15s**) — daí o
  `SetWriteDeadline` no handler de register.

| Operação | Amostra | Duração | HTTP |
|---|---|---|---|
| `GET /person/cpf` | 1 | 468ms | — |
| `GET /person/cpf` | 2 | 478ms | — |
| `GET /person/cpf` | 3 | 480ms | — |
| `GET /person/cpf` | 4 | 480ms | — |
| `GET /person/cpf` | 5 | 488ms | — |
| `GET /person/cpf` | 6 | 500ms | — |
| `GET /person/cpf` | 7 | 580ms | — |
| `GET /person/cpf` | 8 | 604ms | — |
| `POST /person` | 1 | 1.742s | 201 |
| `POST /person` | 2 | 1.88s | 201 |
| `POST /person` | 3 | 2.058s | 201 |


### 11. Repetir o POST com o MESMO id devolve o quê?

**Veredito:** HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete.

O cenário do retry cego: dois `POST /person` idênticos, mesmo `id` (`019f6cd5-b6c3-7124-8911-c5fb52e634d0`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 409,
  "message": "id already exists.",
  "trace": "d76e0710d66b1de2c1602c4a2b0aeddc6865d14d"
}
```

> Este achado veio de um cadastro real que falhou, não da lista original de
> perguntas. O 504 do gateway mente: ele diz que falhou depois de ter criado.

