# Notas da API Doutor ao Vivo (DAV) — achados da sondagem

> **Gerado por `make dav-probe`** (`apps/api/internal/adapters/dav/probe_test.go`).
> Não edite à mão: rode a bateria de novo.

- Ambiente sondado: `https://api.v2.hom.doutoraovivo.com.br`
- Data: 2026-07-16 19:37:02 -03:00

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
| 10 | Qual a latência real da DAV? | `GET` p50 **499ms** · máx **525ms** (n=8). `POST` mediana **1.858s** · máx **1.977s** (n=3). |
| 11 | Repetir o POST com o MESMO id devolve o quê? | HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete. |
| 12 | A DAV aceita um `id` nosso no POST /appointment? | HTTP 400 — a DAV RECUSOU o id do integrador. **Sem idempotência**: um 504 no POST deixa a consulta órfã e sem id para sondar. Ver Risco 1 do plano. |
| 13 | `participants[].url` é obrigatório no request, como diz o spec? | HTTP 201 — **não é obrigatório; o spec mente.** Mandar `{id, role}` e ler a `url` da RESPOSTA. |
| 14 | De onde sai a url de atendimento do paciente? | HTTP 201 — a resposta traz 2 participante(s). **Cada participante tem a SUA url.** A do PAT é o link que o paciente clica. O spec declara a resposta como `{id, url}`, mas ela **também traz `role`**. Mesmo assim casamos pelo ID que enviamos, e não pelo role: o id é o que NÓS controlamos, e depender de um campo que o spec diz não existir é construir sobre algo que eles podem remover sem avisar. |
| 15 | `GET /appointment/{id}` serve de sonda de reconciliação? | ⚠️ **Serve pela metade.** 200 para o que existe, mas o inexistente devolveu 500 (esperado 204). Dá para reconciliar (200 = existe), mas o negativo é ambíguo: não distinguir 'não existe' de 'a DAV caiu' obriga a errar para o lado seguro (deixar PENDING_DAV, não liberar o slot). |
| 16 | A DAV respeita o offset `-03:00` ou reinterpreta como UTC? | Enviamos `2026-07-18T00:35:00-03:00`; a DAV devolveu `2026-07-18T00:35:00-03:00`. ✅ **Mesmo instante** — o offset é respeitado. Mandar sempre RFC3339 com offset. |
| 17 | A DAV recusa dois appointments no mesmo horário para o mesmo médico? | HTTP 201 — ⚠️ **a DAV ACEITA sobreposição.** Não há segunda rede: o CAS do adapter da agenda (`UPDATE tb_slots SET booked=1 WHERE id=? AND booked=0`) é a ÚNICA trava de double-booking do sistema — o MySQL real não tem constraint alguma ligando consulta a horário. O teste de concorrência do adapter não é opcional. |
| 18 | A DAV recusa `start_date_time` no passado? | HTTP 422 — **recusa, como o spec diz.** Ainda assim validamos antes: falha rápido, sem gastar um POST de ~2s. |
| 19 | Qual a latência do POST /appointment? | Média **3.261s**, máx **3.324s** (n=3) NESTA execução. ⚠️ **Varia muito entre execuções**: sondagens do mesmo dia deram média de 3,3s e de 10,5s, com máximo de 17,2s — ou seja, o número de uma rodada só não dimensiona nada. Tratar como "alguns segundos, às vezes mais de 15". O teto do gateway (29s) continua valendo: `RENOVI_DAV_TIMEOUT` acima disso é inútil, e com essa variância o 504 não é hipótese remota. A rota de POST /appointments precisa do mesmo tratamento do register: timeout próprio derivado do orçamento da DAV + SetWriteDeadline. |
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
  "cpf": "89973962184",
  "id": "019f6d11-aa88-7372-a683-7122ffd8b659",
  "name": "RENOVI PROBE Minimo"
}
```

Resposta:

```json
{
  "id": "019f6d11-aa88-7372-a683-7122ffd8b659"
}
```

### 3. `cpf` é opcional no POST /person, como diz o spec?

**Veredito:** HTTP **400** — `cpf` é obrigatório na prática (o spec diz que é opcional). Bom: o lookup por CPF cobre toda a base.

Requisição: `POST /person` **sem** `cpf`:

```json
{
  "birth_date": "1990-05-14",
  "id": "019f6d11-b759-7b52-8aa2-bfaedcc11d46",
  "name": "RENOVI PROBE Sem CPF",
  "status": true
}
```

Resposta:

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "e04e7ebedc6a44015bad6b4143fd35fad6e55c7a",
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

Requisição: `POST /person` com `id` = UUIDv7 nosso (`019f6d11-b93e-7dc7-9b3a-1ee46203a6e6`):

```json
{
  "birth_date": "1990-05-14",
  "cpf": "98314402265",
  "id": "019f6d11-b93e-7dc7-9b3a-1ee46203a6e6",
  "name": "RENOVI PROBE Id Proprio",
  "status": true
}
```

Resposta:

```json
{
  "id": "019f6d11-b93e-7dc7-9b3a-1ee46203a6e6"
}
```

Confirmação: `GET /person/019f6d11-b93e-7dc7-9b3a-1ee46203a6e6` → **200**

```json
{
  "id": "019f6d11-b93e-7dc7-9b3a-1ee46203a6e6",
  "name": "RENOVI PROBE Id Proprio",
  "email": "98314402265@dav.med.br",
  "cpf": "98314402265",
  "birth_date": "1990-05-14",
  "timezone": "America/Sao_Paulo",
  "father_not_informed": false,
  "mother_not_informed": false,
  "status": true
}
```

### 5. A DAV rejeita CPF duplicado?

**Veredito:** HTTP **422** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` (nunca retriável). O lookup por CPF é determinístico.

Duas pessoas, mesmo CPF (`11557620377`), **e-mails diferentes** — para que uma eventual
recusa seja pelo CPF, e não pelo e-mail sintetizado.

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Person invalid",
  "trace": "0bd98f38daa2faaa171db8ef32f2297086102879",
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

Duas pessoas, **CPFs diferentes**, mesmo e-mail (`renovi-probe+bcc9b47b@example.com`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 422,
  "message": "Email já cadastrado.",
  "trace": "76a0843ad0660d7c3ab5de1b8b7eee4cfe7ebe91"
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
  "trace": "73509ae317337e145aab6caae3a1d77fa36f6ab0",
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
  "id": "019f6d11-dba5-7b46-9ed0-2da488be7b3d"
}
```

Por código IBGE (`3505708`): HTTP **201**

```json
{
  "id": "019f6d11-e3a7-71cc-bd17-09155adb250a"
}
```

### 9. Que PII o `GET /person/cpf/{cpf}` expõe?

**Veredito:** HTTP **200**, **12 campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. Nada disso pode sair na resposta do nosso `/auth/register`.

Campos devolvidos: `address`, `birth_date`, `cell_phone`, `cpf`, `email`, `father_not_informed`, `id`, `mother_name`, `mother_not_informed`, `name`, `status`, `timezone`

Resposta completa:

```json
{
  "id": "019f6d11-eb3f-7ec5-9060-bc2451f0daa8",
  "name": "RENOVI PROBE Pii",
  "email": "renovi-probe+24110b9f@example.com",
  "cpf": "15125246034",
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

**Veredito:** `GET` p50 **499ms** · máx **525ms** (n=8). `POST` mediana **1.858s** · máx **1.977s** (n=3).

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
| `GET /person/cpf` | 1 | 456ms | — |
| `GET /person/cpf` | 2 | 478ms | — |
| `GET /person/cpf` | 3 | 489ms | — |
| `GET /person/cpf` | 4 | 492ms | — |
| `GET /person/cpf` | 5 | 499ms | — |
| `GET /person/cpf` | 6 | 511ms | — |
| `GET /person/cpf` | 7 | 522ms | — |
| `GET /person/cpf` | 8 | 525ms | — |
| `POST /person` | 1 | 1.833s | 201 |
| `POST /person` | 2 | 1.858s | 201 |
| `POST /person` | 3 | 1.977s | 201 |


### 11. Repetir o POST com o MESMO id devolve o quê?

**Veredito:** HTTP **409** — **409 `id already exists`** — prova de que um POST nosso anterior pegou. NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` antes de concluir. É por isso que `CreatePerson` nunca repete.

O cenário do retry cego: dois `POST /person` idênticos, mesmo `id` (`019f6d12-1ae7-7b46-9899-4e8065729949`).

Primeira: HTTP 201

Segunda:

```json
{
  "code": 409,
  "message": "id already exists.",
  "trace": "10a177e6ea50356d0dca479c962e5a651a570a5e"
}
```

> Este achado veio de um cadastro real que falhou, não da lista original de
> perguntas. O 504 do gateway mente: ele diz que falhou depois de ter criado.

### 12. A DAV aceita um `id` nosso no POST /appointment?

**Veredito:** HTTP 400 — a DAV RECUSOU o id do integrador. **Sem idempotência**: um 504 no POST deixa a consulta órfã e sem id para sondar. Ver Risco 1 do plano.

```json
{
  "appointment_reason": "elective",
  "end_date_time": "2026-07-17T20:00:00-03:00",
  "id": "7e643ba7-3a44-401a-81ee-4ac94090aa73",
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT"
    }
  ],
  "start_date_time": "2026-07-17T19:35:00-03:00",
  "title": "RENOVI PROBE Consulta"
}
```

```json
{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "b7cf221ea17f5d89274385f112448e04972c5981",
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
  "end_date_time": "2026-07-17T21:00:00-03:00",
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT"
    }
  ],
  "start_date_time": "2026-07-17T20:35:00-03:00",
  "title": "RENOVI PROBE Consulta"
}
```

```json
{
  "id": "bb335561-566e-4bd7-ad89-455b39451b8c",
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/pmg3q1ur85"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/uh1n16cph0"
    }
  ]
}
```

### 14. De onde sai a url de atendimento do paciente?

**Veredito:** HTTP 201 — a resposta traz 2 participante(s). **Cada participante tem a SUA url.** A do PAT é o link que o paciente clica. O spec declara a resposta como `{id, url}`, mas ela **também traz `role`**. Mesmo assim casamos pelo ID que enviamos, e não pelo role: o id é o que NÓS controlamos, e depender de um campo que o spec diz não existir é construir sobre algo que eles podem remover sem avisar.

```json
{
  "id": "a1812c86-ca67-4989-9ba4-5b5f9839f4f6",
  "participants": [
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/s7es683qqv"
    },
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/sde0kpxf70"
    }
  ]
}
```

### 15. `GET /appointment/{id}` serve de sonda de reconciliação?

**Veredito:** ⚠️ **Serve pela metade.** 200 para o que existe, mas o inexistente devolveu 500 (esperado 204). Dá para reconciliar (200 = existe), mas o negativo é ambíguo: não distinguir 'não existe' de 'a DAV caiu' obriga a errar para o lado seguro (deixar PENDING_DAV, não liberar o slot).

Appointment recém-criado (`bcd4cdf6-d865-47c4-87bc-60c2c728960b`):

```json
{
  "id": "bcd4cdf6-d865-47c4-87bc-60c2c728960b",
  "title": "RENOVI PROBE Consulta",
  "status_appointment": "AGE",
  "start_date_time": "2026-07-17T22:35:00-03:00",
  "end_date_time": "2026-07-17T23:00:00-03:00",
  "appointment_reason": "elective",
  "appointment_specialty": null,
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/8wev8ncshs"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/d32b07u70o"
    }
  ]
}
```

UUID inexistente (controle):

```json
{
  "code": 500,
  "message": "Unexpected end of JSON input",
  "trace": "6f51474cacc1689ac0ac10a52bc92879825a8682"
}
```

### 16. A DAV respeita o offset `-03:00` ou reinterpreta como UTC?

**Veredito:** Enviamos `2026-07-18T00:35:00-03:00`; a DAV devolveu `2026-07-18T00:35:00-03:00`. ✅ **Mesmo instante** — o offset é respeitado. Mandar sempre RFC3339 com offset.

```json
{
  "id": "c9a429f7-0d9e-4a28-a226-c39e6fc79f64",
  "title": "RENOVI PROBE Consulta",
  "status_appointment": "AGE",
  "start_date_time": "2026-07-18T00:35:00-03:00",
  "end_date_time": "2026-07-18T01:00:00-03:00",
  "appointment_reason": "elective",
  "appointment_specialty": null,
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/e1pb66hfa3"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/5u5fjgfsbr"
    }
  ]
}
```

### 17. A DAV recusa dois appointments no mesmo horário para o mesmo médico?

**Veredito:** HTTP 201 — ⚠️ **a DAV ACEITA sobreposição.** Não há segunda rede: o CAS do adapter da agenda (`UPDATE tb_slots SET booked=1 WHERE id=? AND booked=0`) é a ÚNICA trava de double-booking do sistema — o MySQL real não tem constraint alguma ligando consulta a horário. O teste de concorrência do adapter não é opcional.

Segundo POST no mesmo horário/MMD:

```json
{
  "id": "161662e1-8cae-4929-8b20-43007989cf94",
  "participants": [
    {
      "id": "981eae5c-4f4a-459b-af86-eae26a0f0887",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/0np0rpetjx"
    },
    {
      "id": "019f6d12-332f-7280-9e17-088e16ccc4bf",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/9u4cqlitee"
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
  "trace": "6a9a1d006e3822f0f16e19c1cdd99d1d7a9af942",
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

**Veredito:** Média **3.261s**, máx **3.324s** (n=3) NESTA execução. ⚠️ **Varia muito entre execuções**: sondagens do mesmo dia deram média de 3,3s e de 10,5s, com máximo de 17,2s — ou seja, o número de uma rodada só não dimensiona nada. Tratar como "alguns segundos, às vezes mais de 15". O teto do gateway (29s) continua valendo: `RENOVI_DAV_TIMEOUT` acima disso é inútil, e com essa variância o 504 não é hipótese remota. A rota de POST /appointments precisa do mesmo tratamento do register: timeout próprio derivado do orçamento da DAV + SetWriteDeadline.

```
POST /appointment #1: 3.14s
POST /appointment #2: 3.32s
POST /appointment #3: 3.324s
```

### 20. `PUT /appointment/{id}/cancel` funciona?

**Veredito:** HTTP 500.

Cancel:

```json
{
  "code": 500,
  "message": "Unexpected token '<', \"<!DOCTYPE \"... is not valid JSON",
  "trace": "50295b57912d79f8e4dd5d78b275f7e14f77b081"
}
```

