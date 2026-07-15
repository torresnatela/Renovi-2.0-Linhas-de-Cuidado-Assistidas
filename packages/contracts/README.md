# packages/contracts

**`openapi.yaml` é a FONTE DA VERDADE da API.** Todo o resto é gerado a partir daqui:

- Backend Go (tipos) via **oapi-codegen** → `apps/api/internal/http/api`
- Cliente TS (hooks) via **orval** → `packages/api-client/src/generated`

## Fluxo

1. Edite `openapi.yaml`.
2. `make generate` (na raiz).
3. Implemente contra os tipos gerados.

O CI (`make generate-check`) falha se o código gerado não estiver atualizado no commit.

## Estado

Na fundação, apenas `/healthz`, `/readyz` e `/me` (stub) estão contratados, além do
vocabulário de domínio (`ItemVerdict`, `Reason`, `Problem`). As rotas do MVP
(`/me/eligibility`, `/slots`, `/appointments`, `/auth/*`) entram nas fases seguintes
sob o mesmo `/api/v1`, sem breaking change.
