# packages/api-client

Cliente TypeScript **gerado** a partir do `packages/contracts/openapi.yaml` (via
[orval](https://orval.dev)), com hooks TanStack Query prontos para o front.

> **Placeholder na fundação.** Ainda não instalado/gerado — a API só tem rotas de
> saúde. Ative quando as rotas do MVP entrarem no contrato.

## Como ativar (na fase MVP)

```bash
npm --prefix packages/api-client install
npm --prefix packages/api-client run generate   # gera src/generated a partir do OpenAPI
```

Depois, no front, importe os hooks de `@renovi/api-client` em vez do `shared/api.ts`
manual. O código gerado (`src/generated`) **não deve ser editado à mão** e é
validado pelo CI.
