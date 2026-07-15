# Seeds — Linhas de Cuidado

Templates de linha de cuidado como **seeds versionados** (SPEC §3.2). Cada arquivo
JSON descreve um `care_line_template` + seus `care_line_item` e dependências.

## Regras

- **Versionamento:** mudança de regra ⇒ **nova versão** (`version` +1), novo arquivo
  (`<code>-vN.json`). Enrollments em andamento continuam presos à versão em que nasceram.
- **DAG:** as `dependencies` entre itens não podem formar ciclo — o `cmd/seed`
  valida a aciclicidade antes de gravar.
- **MVP:** apenas itens `type: "APPOINTMENT"` entram em uso; os demais tipos
  (`ACTIVITY`, `MEDICATION`, `PROCEDURE`) já são aceitos pelo vocabulário.

## Aplicar

```bash
make seed     # roda cmd/seed (STUB na fundação — ver docs/PROGRESSO.md)
```

## Gramática de um item

| Campo | Significado |
|---|---|
| `recurrence_rule` | `{freq, interval, quota}` — no máximo `quota` por janela de `interval × freq` (janela civil, sem acúmulo). |
| `availability_window` | `{offset_days_from_start, duration_days}` — libera no dia D por N dias (`null` = sem fim). |
| `dependencies[]` | `{requires_item, condition: AT_LEAST_N_COMPLETED, n}` — libera após N ocorrências concluídas de outro item. |
