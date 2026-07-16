# Notas da API Doutor ao Vivo (DAV) — achados da sondagem

> **Gerado por `make dav-probe`** (`apps/api/internal/adapters/dav/probe_test.go`).
> Não edite à mão: rode a bateria de novo.

- Ambiente sondado: `https://api.v2.hom.doutoraovivo.com.br`
- Data: 2026-07-16 18:55:18 -03:00

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
| 10 | Qual a latência real da DAV? | `GET` p50 **501ms** · máx **532ms** (n=8). `POST` mediana **2.036s** · máx **2.063s** (n=3). |
| 11 | Repetir o POST com o MESMO id devolve o quê? | HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete. |
| 12 | A DAV aceita um `id` nosso no POST /appointment? | HTTP 400 — a DAV RECUSOU o id do integrador. **Sem idempotência**: um 504 no POST deixa a consulta órfã e sem id para sondar. Ver Risco 1 do plano. |
| 13 | `participants[].url` é obrigatório no request, como diz o spec? | HTTP 201 — **não é obrigatório; o spec mente.** Mandar `{id, role}` e ler a `url` da RESPOSTA. |
| 14 | De onde sai a url de atendimento do paciente? | HTTP 201 — a resposta traz 2 participante(s). **Cada participante tem a SUA url**, e dá para casar pelo id que enviamos (a resposta não traz `role`). A url do PAT é o link que o paciente clica. |
| 15 | `GET /appointment/{id}` serve de sonda de reconciliação? | ⚠️ **Serve pela metade.** 200 para o que existe, mas o inexistente devolveu 500 (esperado 204). Dá para reconciliar (200 = existe), mas o negativo é ambíguo: não distinguir 'não existe' de 'a DAV caiu' obriga a errar para o lado seguro (deixar PENDING_DAV, não liberar o slot). |
| 16 | A DAV respeita o offset `-03:00` ou reinterpreta como UTC? | Enviamos `2026-07-17T23:53:00-03:00`; a DAV devolveu `2026-07-17T23:53:00-03:00`. ✅ **Mesmo instante** — o offset é respeitado. Mandar sempre RFC3339 com offset. |
| 17 | A DAV recusa dois appointments no mesmo horário para o mesmo médico? | HTTP 201 — ⚠️ **a DAV ACEITA sobreposição.** O `SELECT..FOR UPDATE` em `tb_slots` é a ÚNICA trava de double-booking que existe (o MySQL real não tem constraint alguma). O teste de concorrência do adapter não é opcional. |
| 18 | A DAV recusa `start_date_time` no passado? | HTTP 422 — **recusa, como o spec diz.** Ainda assim validamos antes: falha rápido, sem gastar um POST de ~2s. |
| 19 | Qual a latência do POST /appointment? | Média **10.536s**, máx **17.242s** (n=3). O teto do gateway (29s) continua valendo: `RENOVI_DAV_TIMEOUT` acima disso é inútil. A rota de POST /appointments precisa do mesmo tratamento que o register: timeout próprio derivado do `config.DAVBudget()`. |
| 20 | `PUT /appointment/{id}/cancel` funciona? | HTTP 500. |

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
  "cpf": "24100814950",
  "id": "019f6cea-2e4d-7718-878e-d10227dc8f1a",
  "name": "RENOVI PROBE Minimo"
}
```

Resposta:

```json
{
  "id": "019f6cea-2e4d-7718-878e-d10227dc8f1a"
}
```

### 3. `cpf` é opcional no POST /person, como diz o spec?

**Veredito:** HTTP **400** — `cpf` é obrigatório na prática (o spec diz que é opcional). Bom: o lookup por CPF cobre toda a base.

Requisição: `POST /person` **sem** `cpf`:

```json
{
  "birth_date": "1990-05-14",
  "id": "019f6cea-460c-7c38-9b18-068e31050bb3",
  "name": "RENOVI PROBE Sem CPF",
  "status": true
}
```

Resposta:

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "99dd5dc76b485c5476f3ad93f10283956cf4bd70",
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

Requisição: `POST /person` com `id` = UUIDv7 nosso (`019f6cea-484d-77d0-8d76-505f69939d9c`):

```json
{
  "birth_date": "1990-05-14",
  "cpf": "60501309900",
  "id": "019f6cea-484d-77d0-8d76-505f69939d9c",
  "name": "RENOVI PROBE Id Proprio",
  "status": true
}
```

Resposta:

```json
{
  "id": "019f6cea-484d-77d0-8d76-505f69939d9c"
}
```

Confirmação: `GET /person/019f6cea-484d-77d0-8d76-505f69939d9c` → **200**

```json
{
  "id": "019f6cea-484d-77d0-8d76-505f69939d9c",
  "name": "RENOVI PROBE Id Proprio",
  "email": "60501309900@dav.med.br",
  "cpf": "60501309900",
  "birth_date": "1990-05-14",
  "timezone": "America/Sao_Paulo",
  "father_not_informed": false,
  "mother_not_informed": false,
  "status": true
}
```

### 5. A DAV rejeita CPF duplicado?

**Veredito:** HTTP **422** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` (nunca retriável). O lookup por CPF é determinístico.

Duas pessoas, mesmo CPF (`22647577102`), **e-mails diferentes** — para que uma eventual
recusa seja pelo CPF, e não pelo e-mail sintetizado.

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Person invalid",
  "trace": "e71ba5cbb8c567731e831a5b869ebd28bd82e5c8",
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

Duas pessoas, **CPFs diferentes**, mesmo e-mail (`renovi-probe+71557e1e@example.com`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Email já cadastrado.",
  "trace": "79b780ace02d28409f2d807c428dfda1433c71af"
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
  "trace": "c982a76a398ef235bb8d7a515221cc186adc924d",
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
  "id": "019f6cea-7138-7766-8d20-f3a24396133d"
}
```

Por código IBGE (`3505708`): HTTP **201**

```json
{
  "id": "019f6cea-78fb-757a-b2a0-34949af49b80"
}
```

### 9. Que PII o `GET /person/cpf/{cpf}` expõe?

**Veredito:** HTTP **200**, **12 campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. Nada disso pode sair na resposta do nosso `/auth/register`.

Campos devolvidos: `address`, `birth_date`, `cell_phone`, `cpf`, `email`, `father_not_informed`, `id`, `mother_name`, `mother_not_informed`, `name`, `status`, `timezone`

Resposta completa:

```json
{
  "id": "019f6cea-810c-791f-b6a9-0ef8f80aeda2",
  "name": "RENOVI PROBE Pii",
  "email": "renovi-probe+57100c62@example.com",
  "cpf": "83182415417",
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

**Veredito:** `GET` p50 **501ms** · máx **532ms** (n=8). `POST` mediana **2.036s** · máx **2.063s** (n=3).

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
| `GET /person/cpf` | 1 | 438ms | — |
| `GET /person/cpf` | 2 | 467ms | — |
| `GET /person/cpf` | 3 | 480ms | — |
| `GET /person/cpf` | 4 | 492ms | — |
| `GET /person/cpf` | 5 | 501ms | — |
| `GET /person/cpf` | 6 | 518ms | — |
| `GET /person/cpf` | 7 | 528ms | — |
| `GET /person/cpf` | 8 | 532ms | — |
| `POST /person` | 1 | 1.845s | 201 |
| `POST /person` | 2 | 2.036s | 201 |
| `POST /person` | 3 | 2.063s | 201 |


### 11. Repetir o POST com o MESMO id devolve o quê?

**Veredito:** HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete.

O cenário do retry cego: dois `POST /person` idênticos, mesmo `id` (`019f6cea-b21a-748c-b764-4e7e3dddce57`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 409,
  "message": "id already exists.",
  "trace": "e7804d5f872ca323d56a1ab9725573fc6ab5cd52"
}
```

> Este achado veio de um cadastro real que falhou, não da lista original de
> perguntas. O 504 do gateway mente: ele diz que falhou depois de ter criado.

### 12. A DAV aceita um `id` nosso no POST /appointment?

**Veredito:** HTTP 400 — a DAV RECUSOU o id do integrador. **Sem idempotência**: um 504 no POST deixa a consulta órfã e sem id para sondar. Ver Risco 1 do plano.

```json
{
  "appointment_reason": "elective",
  "end_date_time": "2026-07-17T19:17:00-03:00",
  "id": "9ecb9fae-5609-415e-a221-56b5c3f9ea56",
  "participants": [
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD"
    },
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT"
    }
  ],
  "start_date_time": "2026-07-17T18:52:00-03:00",
  "title": "RENOVI PROBE Consulta"
}
```

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "0699ae63fd701a2003ca94b2f6eda92c6d2cd7a3",
  "detail": [
    {
      "message": "property id should not exist"
    }
  ]
}
```

### 13. `participants[].url` é obrigatório no request, como diz o spec?

**Veredito:** HTTP 201 — **não é obrigatório; o spec mente.** Mandar `{id, role}` e ler a `url` da RESPOSTA.

```json
{
  "appointment_reason": "elective",
  "end_date_time": "2026-07-17T20:18:00-03:00",
  "participants": [
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD"
    },
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT"
    }
  ],
  "start_date_time": "2026-07-17T19:53:00-03:00",
  "title": "RENOVI PROBE Consulta"
}
```

```json
{
  "id": "e33a63c7-9729-43fb-b049-8f44289ebe31",
  "participants": [
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/0c91hxsz7z"
    },
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/2qu5h6mf4f"
    }
  ]
}
```

### 14. De onde sai a url de atendimento do paciente?

**Veredito:** HTTP 201 — a resposta traz 2 participante(s). **Cada participante tem a SUA url**, e dá para casar pelo id que enviamos (a resposta não traz `role`). A url do PAT é o link que o paciente clica.

```json
{
  "id": "13cd147e-68a7-45da-a65b-80b826cf674a",
  "participants": [
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz"
    },
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/xucz7cx8cc"
    }
  ]
}
```

### 15. `GET /appointment/{id}` serve de sonda de reconciliação?

**Veredito:** ⚠️ **Serve pela metade.** 200 para o que existe, mas o inexistente devolveu 500 (esperado 204). Dá para reconciliar (200 = existe), mas o negativo é ambíguo: não distinguir 'não existe' de 'a DAV caiu' obriga a errar para o lado seguro (deixar PENDING_DAV, não liberar o slot).

Appointment recém-criado (`804ca440-434b-45e4-87a2-cfa6dff7e5ed`):

```json
{
  "id": "804ca440-434b-45e4-87a2-cfa6dff7e5ed",
  "title": "RENOVI PROBE Consulta",
  "status_appointment": "AGE",
  "start_date_time": "2026-07-17T21:53:00-03:00",
  "end_date_time": "2026-07-17T22:18:00-03:00",
  "appointment_reason": "elective",
  "appointment_specialty": null,
  "participants": [
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/1triniuwsq"
    },
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/7q4dtr5o22"
    }
  ]
}
```

UUID inexistente (controle):

```json
{
  "code": 500,
  "message": "Unexpected end of JSON input",
  "trace": "7cc4a290c12b5bfed96c7fda7c43cfd126646e05"
}
```

### 16. A DAV respeita o offset `-03:00` ou reinterpreta como UTC?

**Veredito:** Enviamos `2026-07-17T23:53:00-03:00`; a DAV devolveu `2026-07-17T23:53:00-03:00`. ✅ **Mesmo instante** — o offset é respeitado. Mandar sempre RFC3339 com offset.

```json
{
  "id": "21c7b319-6f10-4f63-a124-f5aeffb87d5a",
  "title": "RENOVI PROBE Consulta",
  "status_appointment": "AGE",
  "start_date_time": "2026-07-17T23:53:00-03:00",
  "end_date_time": "2026-07-18T00:18:00-03:00",
  "appointment_reason": "elective",
  "appointment_specialty": null,
  "participants": [
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/1z5ojpylmy"
    },
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/5ynlqhh8ny"
    }
  ]
}
```

### 17. A DAV recusa dois appointments no mesmo horário para o mesmo médico?

**Veredito:** HTTP 201 — ⚠️ **a DAV ACEITA sobreposição.** O `SELECT..FOR UPDATE` em `tb_slots` é a ÚNICA trava de double-booking que existe (o MySQL real não tem constraint alguma). O teste de concorrência do adapter não é opcional.

Segundo POST no mesmo horário/MMD:

```json
{
  "id": "b35d6ff4-66f6-4b09-be15-0a5e149f9d8e",
  "participants": [
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/gzamvtuw8g"
    },
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/jpjsmh3264"
    }
  ]
}
```

### 18. A DAV recusa `start_date_time` no passado?

**Veredito:** HTTP 422 — **recusa, como o spec diz.** Ainda assim validamos antes: falha rápido, sem gastar um POST de ~2s.

```json
{
  "code": 422,
  "message": "Appointment invalid",
  "trace": "5b4f1ecadfd54e478744a916527817c426e7a47f",
  "i18n": {
    "phrase": "entity.validation.exception",
    "mustache": {
      "entity": "Appointment"
    }
  },
  "detail": [
    {
      "message": "start date time should not be in the past",
      "i18n": {
        "phrase": "attribute.is.not.valid",
        "mustache": {
          "field": "start_date_time"
        }
      }
    }
  ]
}
```

### 19. Qual a latência do POST /appointment?

**Veredito:** Média **10.536s**, máx **17.242s** (n=3). O teto do gateway (29s) continua valendo: `RENOVI_DAV_TIMEOUT` acima disso é inútil. A rota de POST /appointments precisa do mesmo tratamento que o register: timeout próprio derivado do `config.DAVBudget()`.

```
POST /appointment #1: 10.239s
POST /appointment #2: 17.242s
POST /appointment #3: 4.127s
```

### 20. `PUT /appointment/{id}/cancel` funciona?

**Veredito:** HTTP 500.

Cancel:

```json
{
  "code": 500,
  "message": "Unexpected token '<', \"<!DOCTYPE \"... is not valid JSON",
  "trace": "4448e38923ca17fdda6bead3948264ae4f6e2cec"
}
```

