# Deploy e infraestrutura de produção

> Como o Renovi 2.0 roda em produção, como publicar, como reverter e como agir
> em incidente. Decisões formais: ADR-026/027/028 em `DECISOES.md`.

## Visão geral

```
Cloudflare (DNS only, nuvem cinza)
  A app.renovisaude.com.br → 2.25.184.35 (VPS Hostinger, AlmaLinux 10)
        │
        ▼  portas 80/443 da VM
  Caddy de BORDA (container renovi-caddy-1, projeto preexistente /opt/renovi)
  TLS automático Let's Encrypt · um bloco de site por sistema:
        ├── {$DOMAIN}                    → sistema antigo   (INTOCADO)
        ├── gestao.renovisaude.com.br    → renovi-gestao    (INTOCADO)
        └── app.renovisaude.com.br       → RENOVI 2.0 (este repo)
              ├── /api/*, /healthz, /readyz → care-api:8090
              └── restante                  → care-web:80 (SPA)
        │  (rede docker externa renovi_default, aliases care-api/care-web)
        ▼
  /opt/renovi-care — docker compose project "renovi-care"
        ├── web  ghcr.io/torresnatela/renovi-care-web:<sha>   (Caddy interno, SPA)
        └── api  ghcr.io/torresnatela/renovi-care-api:<sha>   (Go, 1 instância)
              ├── Neon Postgres 17 (database neondb) — TLS, endpoint direto
              ├── MySQL legado (escala/slots)     — agendamento
              └── Doutor ao Vivo (api.v2.doutoraovivo.com.br)
```

A VPS é **compartilhada** por três sistemas. Regras de convivência:

- O projeto deste repo chama-se **`renovi-care`** (o nome `renovi` pertence ao
  sistema antigo). Diretório: `/opt/renovi-care`. Rede interna: `renovi-care`.
- Toda mudança no Caddy de borda (`/opt/renovi/Caddyfile`) é **aditiva**, com
  backup `Caddyfile.bak.<timestamp>` antes, `caddy validate` antes do reload, e
  **nunca** altera blocos dos outros sistemas. O arquivo é bind-mount de arquivo
  único (`ro,Z`): edite **in-place** (`tee -a`, `cp`) — `mv`/`sed -i` trocam o
  inode e o container continua lendo o arquivo velho.
- **⚠️ Antes de qualquer edição na borda, confira se o bind não está órfão**
  (aconteceu no go-live de 2026-07-20: um fluxo antigo trocou o arquivo com `mv`
  e o container ficou preso ao inode velho — edições in-place e `caddy reload`
  passavam a valer para um arquivo que o Caddy não lia):
  `ls -i /opt/renovi/Caddyfile` vs `docker exec renovi-caddy-1 ls -i /etc/caddy/Caddyfile`.
  **Inodes diferentes ⇒ recriar o container** (`docker compose -f
  docker-compose.prod.yml up -d --force-recreate --no-deps caddy` em
  `/opt/renovi`, ~5 s de borda fora; certificados sobrevivem no volume
  `caddy_data`) e SÓ ENTÃO editar.
- Portas localhost em uso na VM: 8081/8082 (gestao), 9000 (portainer),
  **8083 (care-web)** e **8084 (care-api)** — só para smoke test via SSH.
- **Nunca escalar a API para >1 réplica**: o rate limiter é em memória por
  processo (ver ADR-019/ADR-026).

## Como o deploy funciona

Pipeline único em `.github/workflows/ci.yml`:

1. **PR**: jobs `api`, `api-integration`, `web` + `docker-build-check` (builda as
   duas imagens sem push — Dockerfile quebrado não chega à main).
2. **Push na main** (merge): os mesmos testes rodam e o job **`deploy`** fica
   aguardando **aprovação manual** no environment `production` do GitHub.
3. Aprovado, o job:
   - builda e publica `renovi-care-api` e `renovi-care-web` no GHCR com tags
     `<sha>` e `latest` (o dist do front vem do artifact do job `web` — a imagem
     embarca exatamente o que foi testado);
   - copia para a VPS (`scp`): `docker-compose.prod.yml`, `deploy-remote.sh`,
     `.env` (IMAGE_TAG/GHCR_OWNER) e `.env.api` (segredos, chmod 600);
   - executa `deploy-remote.sh` na VPS: `compose pull` → **migrations antes de
     trocar a API** (`docker run --rm --env-file .env.api <img> /migrate up`;
     se falharem, a versão antiga continua no ar) → `compose up -d` → espera
     `/readyz` 200 pela porta de smoke;
   - verificação externa: `https://app.renovisaude.com.br/readyz` **e** smoke dos
     sistemas vizinhos (site antigo e gestao) — o deploy falha se algum quebrou.

Deploy manual (re-run): aba Actions → workflow CI → `Run workflow` na main, ou
re-executar só o job `deploy` de um run verde.

## Segredos (GitHub → environment `production`)

| Secret | Conteúdo |
|---|---|
| `SSH_HOST` | `2.25.184.35` |
| `SSH_USER` | `deploy` |
| `SSH_PRIVATE_KEY` | chave ed25519 dedicada de deploy (par local `~/.ssh/renovi_ci`) |
| `GHCR_PULL_TOKEN` | PAT classic com só `read:packages` (a VPS puxa as imagens; logout ao fim do deploy) |
| `RENOVI_CARE_DATABASE_URL` | Neon, **endpoint direto** (host sem `-pooler`) + `?sslmode=require`, autenticando como **`renovi_app`** (role restrito — ADR-024) |
| `RENOVI_CARE_MIGRATE_DATABASE_URL` | mesma base, autenticando como **owner** (usuário padrão do Neon) — só as migrations usam |
| `RENOVI_DAV_BASE_URL` | `https://api.v2.doutoraovivo.com.br` |
| `RENOVI_DAV_API_KEY` | chave de produção da Doutor ao Vivo |
| `RENOVI_LEGACY_DATABASE_URL` | `user:pass@tcp(host:3306)/db` — **sem** `parseTime`/`loc` (o adapter os força). ⚠️ Hoje usa o usuário `admin` (pleno); criar um usuário restrito a `SELECT` + `UPDATE(booked, updatedAt)` como no mock de dev (ADR-004) é pendência de hardening |
| `RENOVI_CPF_PEPPER` | pepper do `cpf_hmac` (HMAC-SHA256 do CPF, ADR-043). **Segredo**; **o MESMO valor** configurado na Renovi Gestão — os dois lados só casam a pessoa se o pepper for idêntico. Vazio DESLIGA a ingestão. Setar **antes** do backfill (ver "Ligar a ingestão da Gestão") |
| `RENOVI_GESTAO_INTEGRATION_TOKEN` | token estático da integração Gestão→API (header `X-Integration-Token`, ADR-043). **Segredo**; nunca logado. Vazio DESLIGA as rotas `/integration/gestao/*`. **Setar por ÚLTIMO**, depois do backfill (ver "Ligar a ingestão da Gestão") |

Regras:
- `RENOVI_ENV=production` e `RENOVI_LOG_LEVEL=info` são fixos no workflow, não
  são secrets. `RENOVI_SESSION_COOKIE_SECURE` **não** é definido (default `true`;
  `false` é recusado pela config em produção).
- Ingestão da Gestão (ADR-043): `RENOVI_WEB_BASE_URL=https://app.renovisaude.com.br`
  (base do `invite_url`; **obrigatória** quando o token de integração está setado —
  a config recusa subir sem ela) e `RENOVI_INVITE_TTL` (default `168h` = 7 dias) —
  **não** são segredos. Só o pepper e o token de integração são secrets.
- Neon: o endpoint **pooled** (`-pooler`) quebra as migrations (advisory lock do
  golang-migrate × transaction pooling). Na escala do piloto, endpoint direto
  para tudo (ADR-027).
- A host key da VPS está **fixada** no workflow (não usa `ssh-keyscan` cego).
- A borda Hostinger roda **anti-DDoS** que descarta *intermitentemente* o 1º SYN
  do IP (efêmero) do runner. O job faz **warm-up com retry** (6× · `ConnectTimeout=10`)
  da conexão SSH **antes** do `scp` — senão um único pacote perdido derruba o deploy
  com `connect ... port 22: Connection timed out`. **Não é allowlist de IP**: os
  outros sistemas da VPS deployam pelo mesmo caminho (o `renovi_saude_publica`
  também repete a 1ª conexão) e a 22 responde da máquina do dev (IP "quente").

## Ligar a ingestão da Gestão (push, ADR-043)

As rotas `/integration/gestao/*` nascem **desligadas** e só sobem quando as três
variáveis estão presentes juntas (`RENOVI_GESTAO_INTEGRATION_TOKEN`,
`RENOVI_CPF_PEPPER`, `RENOVI_WEB_BASE_URL`) — falta de qualquer uma responde 404 e
loga o motivo. Isso torna a **ordem de ativação** a parte que importa.

### Gerando os segredos

Ambos são strings aleatórias fortes (32 bytes em base64url ≈ 43 chars):

```bash
# RENOVI_CPF_PEPPER — a MESMA string vai para a Renovi Gestão (combine o valor com
# o time da Gestão e guarde nos secrets dos DOIS sistemas). Gere UMA vez:
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='

# RENOVI_GESTAO_INTEGRATION_TOKEN — só nosso; a Gestão o manda no X-Integration-Token:
openssl rand -base64 32 | tr '+/' '-_' | tr -d '='
```

- Guarde-os nos **secrets do environment `production`** (viram `.env.api`, chmod 600),
  como os demais segredos da tabela acima. **Nunca** os comite nem os cole em chat/log.
- O **pepper é rotação difícil**: mudar seu valor invalida todo `cpf_hmac` já gravado
  (nosso e o da Gestão). Trate-o como chave de longa vida; se um dia rotacionar, é um
  reprocessamento coordenado dos dois lados + novo backfill, não um simples swap.
- O **token de integração é rotação simples** (manual, como o `RENOVI_ADMIN_TOKEN`):
  gere um novo, atualize o secret e avise a Gestão. Ausente/errado respondem igual (401).

### Ordem obrigatória (fecha a janela de conta duplicada)

A detecção de `cpf_match` — não convidar quem **já tem** conta Renovi — depende de
`patient_identity.cpf_hmac` estar preenchido. Essa coluna nasce `NULL`; o cadastro só
passa a gravá-la depois que o pepper existe, e as linhas **antigas** só recebem valor
pelo backfill. Se o token de integração for ligado antes disso, um push para alguém que
já é paciente **não casa**, e nós cunhamos um convite de onboarding indevido (semente de
conta duplicada quando a fatia de conclusão do onboarding entrar). Por isso:

1. **Deploy** da versão com a migration `0016` aplicada (o pipeline roda as migrations
   antes de trocar a API).
2. **Setar `RENOVI_CPF_PEPPER`** (secret) e redeploy. A partir daqui, todo **cadastro
   novo** já grava `cpf_hmac`.
3. **Rodar o backfill** das linhas antigas (one-shot, idempotente, exige o pepper no
   ambiente) — cobre quem se cadastrou antes do passo 2. O binário
   (`cmd/backfill-cpf-hmac`) vai na imagem ao lado do `/migrate`:

   ```bash
   ssh renovi-prod
   docker run --rm --env-file /opt/renovi-care/.env.api \
     ghcr.io/torresnatela/renovi-care-api:<sha> /backfill-cpf-hmac
   # -> "backfill concluído: N atualizada(s), K ignorada(s) por cpf inválido"
   ```

   Confira que zerou os pendentes:
   `psql "<owner>" -c "SELECT count(*) FROM patient_identity WHERE cpf_hmac IS NULL"`
   deve dar **0** (fora eventuais CPFs inválidos legados, que o backfill loga e pula).
   Rode-o quantas vezes precisar: só toca linhas ainda `NULL`.
4. **Só então setar `RENOVI_GESTAO_INTEGRATION_TOKEN`** (secret) e `RENOVI_WEB_BASE_URL`,
   e redeploy. É esse passo que **abre** as rotas — fazê-lo por último garante que toda
   pessoa já casa por `cpf_hmac` antes do primeiro push chegar.
5. Entregar o token à Gestão e validar com `apps/api/docs/ingestion.http` (o `@cpfHmac`
   de lá é calculado com o MESMO pepper de produção — recalcule).

## Rollback

```bash
ssh renovi-prod                      # alias local → deploy@2.25.184.35
cd /opt/renovi-care
# fixe o SHA anterior (veja tags no GHCR ou o histórico de runs verdes):
sed -i 's/^IMAGE_TAG=.*/IMAGE_TAG=<SHA_ANTERIOR>/' .env
docker compose -f docker-compose.prod.yml up -d
curl -fsS http://127.0.0.1:8084/readyz
```

- Imagens de até **7 dias** ficam na VM (`prune --filter until=168h`); mais
  antigas, o `pull` baixa de novo do GHCR.
- **Migrations não são revertidas** no rollback. Regra do repo: migrations
  compatíveis com a versão anterior da API (expand/contract). `/migrate down` é
  decisão manual e consciente.
- Borda: restaurar backup é `sudo cp /opt/renovi/Caddyfile.bak.<ts> /opt/renovi/Caddyfile`
  + `docker exec -w /etc/caddy renovi-caddy-1 caddy reload --config /etc/caddy/Caddyfile`.

## Runbook de incidentes

| Sintoma | Diagnóstico | Ação |
|---|---|---|
| Deploy falha no `scp`/`ssh` com `connect ... port 22: Connection timed out` | anti-DDoS da borda Hostinger descartou o 1º SYN do IP novo do runner — **não é allowlist** (os vizinhos deployam pelo mesmo caminho) | o job já faz warm-up com retry (6×); se estourar as 6, **re-executar o job `deploy`** (novo IP/janela); da sua máquina `nc -zv 2.25.184.35 22` deve abrir |
| Site fora (`app.renovisaude.com.br` não resolve/TLS) | `dig +short app.renovisaude.com.br` deve dar `2.25.184.35` (não IPs 104.x — nuvem laranja da Cloudflare é topologia errada); logs do cert: `docker logs renovi-caddy-1 \| grep -i acme` | corrigir DNS na Cloudflare (DNS only); aguardar retry do Caddy |
| SPA abre, API não (502 em `/api`) | `ssh renovi-prod`; `docker ps` — `renovi-care-api-1` de pé? `docker compose -f /opt/renovi-care/docker-compose.prod.yml logs --tail 100 api` | se crash-loop por config: conferir `.env.api`; se OOM: `docker stats` |
| `/readyz` 503 | corpo não diz a causa (de propósito — LGPD); ver logs da api: falha de ping no Postgres (Neon) ou no MySQL legado | Neon: status.neon.tech + testar `psql` da VM; legado: `nc -zv <host> 3306` da VM — se o firewall do legado bloqueou o IP da VPS, o **login continua ok e o agendamento fica fora** até liberar |
| Migration falhou no deploy | o job falha ANTES do `up -d` (versão antiga segue no ar); `docker run --rm --env-file /opt/renovi-care/.env.api <img> /migrate version` mostra versão/dirty | corrigir a migration, novo commit/deploy; dirty: resolver manualmente antes |
| Outro sistema da VM caiu após mexer na borda | `docker exec renovi-caddy-1 caddy validate --config /etc/caddy/Caddyfile` | restaurar o backup mais recente (ver Rollback) |
| Mudança na borda "não pega" (site novo sem certificado, `SSL alert internal error`; config carregada não lista o domínio) | comparar inodes host×container (ver regra de convivência acima) — bind órfão por `mv` antigo | recriar o container caddy (`up -d --force-recreate --no-deps caddy`) e reconferir `docker exec renovi-caddy-1 grep app.renovisaude /etc/caddy/Caddyfile` |
| Migration `dirty` no Neon | `cmd/migrate version` mostra `dirty=true`; causa típica: 0008 recusada pela política de senha do Neon | corrigir a causa, `psql "<owner>" -c "UPDATE schema_migrations SET version=<anterior>, dirty=false"` e rodar `migrate up` de novo |
| VM sem recursos | `free -m`, `df -h`, `docker stats` — a VM tem 1 vCPU/4 GB compartilhados | limites: api 512m, web 128m; se sistêmico, avaliar upgrade do plano Hostinger |
| Push da Gestão gera convite para quem JÁ tem conta Renovi | `cpf_match` não detectou: `patient_identity.cpf_hmac` estava `NULL` (backfill não rodou, ou pepper ligado depois da conta) — `psql "<owner>" -c "SELECT count(*) FROM patient_identity WHERE cpf_hmac IS NULL"` > 0 | rodar `/backfill-cpf-hmac` (ver "Ligar a ingestão da Gestão"); enquanto a conclusão de onboarding não existe, o convite indevido não cria conta, mas revise a ordem de ativação |

## LGPD e logs

- Logs da API em produção: JSON, nível `info`. Corpo de auth e dados de saúde
  nunca são logados (garantido no código; ver `apps/api/CLAUDE.md`).
- **Dado pessoal nunca vai em query string** (o access log da borda registra o
  URI). Hoje nenhuma rota do contrato viola isso; toda rota nova deve manter.
- O cookie de sessão nunca deve aparecer em log de proxy. O Caddy de borda hoje
  não loga access log por site; se for ligar, filtrar os headers `Cookie` e
  `Authorization`.
- `dav_link_audit` guarda IP (dado pessoal): política de retenção pendente (P1).

## Banco (Neon)

- Postgres 17 gerenciado, região `sa-east-1`, database **`neondb`** (nome padrão
  do projeto Neon; o "renovi_care" é o nome lógico do sistema, não do database).
  TLS obrigatório (`sslmode=require`; remover `channel_binding` da URL do
  console — o pgx não precisa). Host **sem `-pooler`** (o console oferece o
  pooled por padrão; trocar).
- **Dois roles** (ADR-024/ADR-027): a app conecta como `renovi_app` (restrito;
  `journey_event` append-only por privilégio) e as migrations como o owner
  (`neondb_owner`).
- **Provisionamento inicial** (uma vez, por ambiente novo — como foi feito em
  2026-07-20): o Neon **recusa** o `CREATE ROLE ... PASSWORD 'renovi_app'` da
  migration 0008 (política de senha do control plane). Ordem correta:
  1. `psql "<owner>" -c "CREATE ROLE renovi_app LOGIN PASSWORD '<senha forte>'"`
     (o `IF NOT EXISTS` da 0008 então só aplica os GRANTs);
  2. `RENOVI_CARE_MIGRATE_DATABASE_URL=<owner> RENOVI_CARE_DATABASE_URL=<owner>
     go -C apps/api run ./cmd/migrate up` → conferir `version` = 8, limpo;
  3. `RENOVI_CARE_DATABASE_URL` da app = mesma URL trocando `neondb_owner:...`
     por `renovi_app:<senha forte>`.
  Se a 0008 rodar antes do passo 1, ela falha e deixa `dirty` — recuperação no
  runbook acima.
- **Backup/PITR**: conferir a retenção do plano contratado (default do plano
  gratuito ~1 dia; anotar aqui o valor real quando confirmado).
- **Autosuspend**: se ligado, o primeiro request após ociosidade tem cold start
  (~1 s). Aceito no piloto; se incomodar, desligar no console do Neon.

## Checklist pré-go-live

> Concluído integralmente no go-live de **2026-07-20** (SHA `fd6c7d9`). Mantido
> como template para um eventual novo ambiente.

- [ ] DNS `A app.renovisaude.com.br → 2.25.184.35`, **DNS only** (nuvem cinza)
- [ ] 8 secrets do environment `production` preenchidos
- [ ] Bloco `app.renovisaude.com.br` aplicado na borda (backup + validate + reload)
- [ ] Conectividade da VPS: Neon (`select 1`), MySQL legado (porta 3306 liberada
      para `2.25.184.35`), DAV (HTTP 403 sem chave = alcançável)
- [ ] Deploy aprovado, `/readyz` 200 externo, login e agendamento reais testados
- [ ] Cookie de sessão com `Secure; HttpOnly; SameSite=Lax` (conferir no browser)
- [ ] Sistemas vizinhos intactos (site antigo + gestao) e containers antigos `running`
- [ ] ADR-019: `True-Client-IP` deletado e `X-Real-IP` sobrescrito na borda
      (já vem no bloco de `deploy/edge-snippet.Caddyfile`)

## Fora do escopo desta fase

`cmd/worker` (reconciliação/auto-conclusão/lembretes) não é deployado — é STUB.
Quando ganhar jobs reais, entra como serviço `worker` no
`deploy/docker-compose.prod.yml` (mesma imagem da api, `CMD ["/worker"]`).
HTTP/3 (443/udp) desligado — o firewall Hostinger só abre TCP.
