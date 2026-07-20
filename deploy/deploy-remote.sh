#!/usr/bin/env bash
# Executado NA VPS pelo job de deploy (via SSH), a partir de /opt/renovi-care.
# Pré-condições (preparadas pelo workflow antes de chamar este script):
#   - docker login no ghcr.io já feito;
#   - .env (IMAGE_TAG/GHCR_OWNER) e .env.api (segredos, 600) atualizados;
#   - docker-compose.prod.yml e este script copiados via scp.
set -euo pipefail

cd /opt/renovi-care
set -a
# shellcheck source=/dev/null
. ./.env
set +a

COMPOSE="docker compose -f docker-compose.prod.yml"
API_IMAGE="ghcr.io/${GHCR_OWNER}/renovi-care-api:${IMAGE_TAG}"

echo "==> pull das imagens (${IMAGE_TAG})"
$COMPOSE pull

# Migrations ANTES de trocar a API: se falharem, a versão antiga segue no ar.
echo "==> migrations (${API_IMAGE})"
docker run --rm --env-file .env.api "${API_IMAGE}" /migrate up

echo "==> subindo containers"
$COMPOSE up -d

# Readiness real (Neon + MySQL legado) pela porta de smoke em localhost.
echo "==> aguardando /readyz"
ok=""
for _ in $(seq 1 12); do
  if curl -fsS http://127.0.0.1:8084/readyz >/dev/null 2>&1; then ok=1; break; fi
  sleep 5
done
if [ -z "$ok" ]; then
  echo "ERRO: /readyz não respondeu 200 em 60s; últimos logs da api:" >&2
  $COMPOSE logs --tail 50 api >&2
  exit 1
fi
curl -fsS -o /dev/null http://127.0.0.1:8083/
echo "==> api pronta e web servindo"

# Mantém 7 dias de imagens (janela de rollback); não toca em volumes.
docker image prune -af --filter "until=168h" >/dev/null

# Não deixar credencial do GHCR persistida na VM.
docker logout ghcr.io >/dev/null 2>&1 || true

echo "==> deploy ${IMAGE_TAG} concluído"
