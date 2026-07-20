# Imagem de produção do front (SPA estática atrás do Caddy de borda da VPS).
# Contexto de build = RAIZ do monorepo (precisa alcançar deploy/Caddyfile).
#
# O dist NÃO é buildado aqui: vem pronto do job `web` do CI (artifact), que já
# rodou typecheck + testes + `vite build`. Assim a imagem embarca exatamente o
# dist que foi testado. Para build local: `make web-build` antes.
# VITE_API_BASE_URL fica vazia de propósito: SPA e API na mesma origem.
FROM caddy:2-alpine
COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY apps/web/dist /srv/web
