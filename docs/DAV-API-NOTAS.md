# Notas da API Doutor ao Vivo (DAV) — achados da sondagem

> **Gerado por `make dav-probe`** (`apps/api/internal/adapters/dav/probe_test.go`).
> Não edite à mão: rode a bateria de novo.

- Ambiente sondado: `https://api.v2.hom.doutoraovivo.com.br`
- Data: 2026-07-16 17:11:31 -03:00

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
| 10 | Qual a latência real da DAV? | `GET` p50 **511ms** · máx **548ms** (n=8). `POST` mediana **2.241s** · máx **2.459s** (n=3). |
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
  "cpf": "62074184760",
  "name": "RENOVI PROBE Minimo"
}
```

Resposta:

```json
{
  "id": "65b9b43d-73ce-4563-9a1e-05abd7d1e499"
}
```

### 3. `cpf` é opcional no POST /person, como diz o spec?

**Veredito:** HTTP **400** — `cpf` é obrigatório na prática (o spec diz que é opcional). Bom: o lookup por CPF cobre toda a base.

Requisição: `POST /person` **sem** `cpf`:

```json
{
  "birth_date": "1990-05-14",
  "name": "RENOVI PROBE Sem CPF",
  "status": true
}
```

Resposta:

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "aa06502a14d6b96fc38b6d4361347c82ef240635",
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

Requisição: `POST /person` com `id` = UUIDv7 nosso (`019f6c8d-a99d-796a-bbf7-75b5c297adca`):

```json
{
  "birth_date": "1990-05-14",
  "cpf": "39631974049",
  "id": "019f6c8d-a99d-796a-bbf7-75b5c297adca",
  "name": "RENOVI PROBE Id Proprio",
  "status": true
}
```

Resposta:

```json
{
  "id": "019f6c8d-a99d-796a-bbf7-75b5c297adca"
}
```

Confirmação: `GET /person/019f6c8d-a99d-796a-bbf7-75b5c297adca` → **200**

```json
{
  "id": "019f6c8d-a99d-796a-bbf7-75b5c297adca",
  "name": "RENOVI PROBE Id Proprio",
  "email": "39631974049@dav.med.br",
  "cpf": "39631974049",
  "birth_date": "1990-05-14",
  "timezone": "America/Sao_Paulo",
  "father_not_informed": false,
  "mother_not_informed": false,
  "status": true
}
```

### 5. A DAV rejeita CPF duplicado?

**Veredito:** HTTP **422** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` (nunca retriável). O lookup por CPF é determinístico.

Duas pessoas, mesmo CPF (`82240907959`), **e-mails diferentes** — para que uma eventual
recusa seja pelo CPF, e não pelo e-mail sintetizado.

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Person invalid",
  "trace": "30a6e631de5122c19135a542746455b618e9890e",
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

Duas pessoas, **CPFs diferentes**, mesmo e-mail (`renovi-probe+5b64ba52@example.com`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Email já cadastrado.",
  "trace": "6151e44517a26dbbec623ca97dca18d8864799bb"
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
  "trace": "39a550e098718e05f650cbf07c334f0b940a9b22",
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
  "id": "99c3bfaa-4956-4224-87f3-c5bb11591058"
}
```

Por código IBGE (`3505708`): HTTP **201**

```json
{
  "id": "f1b1c9b0-777a-4104-ad55-6c46eee1d83e"
}
```

### 9. Que PII o `GET /person/cpf/{cpf}` expõe?

**Veredito:** HTTP **200**, **12 campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. Nada disso pode sair na resposta do nosso `/auth/register`.

Campos devolvidos: `address`, `birth_date`, `cell_phone`, `cpf`, `email`, `father_not_informed`, `id`, `mother_name`, `mother_not_informed`, `name`, `status`, `timezone`

Resposta completa:

```json
{
  "id": "47eb08d4-676a-4e3e-b8ad-38b9766f0123",
  "name": "RENOVI PROBE Pii",
  "email": "renovi-probe+e6566191@example.com",
  "cpf": "80387508376",
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

**Veredito:** `GET` p50 **511ms** · máx **548ms** (n=8). `POST` mediana **2.241s** · máx **2.459s** (n=3).

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
| `GET /person/cpf` | 1 | 477ms | — |
| `GET /person/cpf` | 2 | 481ms | — |
| `GET /person/cpf` | 3 | 500ms | — |
| `GET /person/cpf` | 4 | 500ms | — |
| `GET /person/cpf` | 5 | 511ms | — |
| `GET /person/cpf` | 6 | 517ms | — |
| `GET /person/cpf` | 7 | 524ms | — |
| `GET /person/cpf` | 8 | 548ms | — |
| `POST /person` | 1 | 2.157s | 201 |
| `POST /person` | 2 | 2.241s | 201 |
| `POST /person` | 3 | 2.459s | 201 |


### 11. Repetir o POST com o MESMO id devolve o quê?

**Veredito:** HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete.

O cenário do retry cego: dois `POST /person` idênticos, mesmo `id` (`019f6c8e-1365-7088-a3c3-34acbcdebb17`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 409,
  "message": "id already exists.",
  "trace": "46df07d8bc3d397746c219b3d1ba6f1a01a857b2"
}
```

> Este achado veio de um cadastro real que falhou, não da lista original de
> perguntas. O 504 do gateway mente: ele diz que falhou depois de ter criado.

