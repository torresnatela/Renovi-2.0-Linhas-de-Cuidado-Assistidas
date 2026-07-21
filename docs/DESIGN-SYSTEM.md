# Design System — Front (apps/web)

> Documento vivo do design system do app do paciente (Renovi 2.0) — **desktop e
> mobile responsivo, mesma base de código (same-codebase)**.
> Fonte visual: handoff do designer (tokens + telas de referência). Fonte de verdade
> técnica dos valores: `apps/web/src/styles/tokens.css` (CSS custom properties).
> **Idioma:** código/classes em inglês; este doc em PT-BR.

O redesign desktop foi concluído nas **Etapas 0–8** (2026-07-20): a fundação
(tokens, fontes, theme, diretrizes), a biblioteca de componentes em
`src/shared/ui/` (§7, inventário REAL) e as telas do produto. Decisões em
**ADR-038** (design system) e **ADR-039** (produto). Este doc é a referência
antes de estilizar qualquer tela nova.

O mobile responsivo chegou por cima do MESMO design system (Etapas 0–7,
2026-07-21, branch `feat/mobile-ui`) — nenhum token novo, mesma paleta/tipografia/
raios. Só o **layout** muda por viewport (container, chrome, headers — §9).
Decisões em **ADR-041** (hook de viewport) e **ADR-042** (produto mobile).

---

## 1. Arquitetura de tokens

- **CSS custom properties são a fonte de verdade** (`src/styles/tokens.css`), portadas
  do handoff e diffáveis contra pacotes futuros do designer.
- **`tailwind.config.js` estende o theme referenciando `var(--…)`** (cores, sombras).
  Assim a classe Tailwind e o token nunca divergem.
- **Carregamento:** `src/index.css` importa `styles/fonts.css` e `styles/tokens.css`
  **antes** das diretivas `@tailwind`; o `@layer base` aplica Nunito + cores de corpo
  no `body`.

### Trade-off documentado: nada de `/alpha` sobre cores de token

Modificadores de alpha do Tailwind (`bg-primary-300/50`) **não funcionam** com valores
`var()` simples. **Regra: proibido `/alpha` sobre cores de token.** Para transparência,
use os **tints** que o DS já fornece (`primary-100/200`, `accent-100/200`). Estados
`pressed`/`disabled` usam **opacidade do elemento** via tokens
`--opacity-pressed: 0.7` e `--opacity-disabled: 0.5`.

---

## 2. Fundação visual

- **Tipografia:** **Nunito** local (`400` regular, `600` semibold, `700` bold),
  `font-display: swap`. Headings em *sentence case*; rótulos de botão em **UPPERCASE**
  (via classe, ver §4). Fallback: `-apple-system, 'Segoe UI', Roboto, sans-serif`.
  A escala tipográfica (h1–h6, textN, caption) **não** é portada como CSS — o app usa
  utilitários Tailwind por tela. A escala de referência do designer vive em
  `design_system/tokens/typography.css` (handoff).
- **Raios:** `8` (inputs / blocos internos), `12` (ícone-container, blocos de motivo),
  `16` (cards e botões), `999` (pills/chips). Mapeados no Tailwind como
  `rounded-sm / rounded-md / rounded-lg / rounded-pill` (ver §3.3).
- **Elevação:** **uma sombra única navy** — `--shadow-card: 0 4px 24px rgba(14,25,85,0.08)`.
  Variantes `--shadow-raised` (hover/modal) e `--shadow-button` (CTA laranja).
- **Borda + sombra, não fill pesado:** cards brancos com borda `1px` `--border-default`
  (`primary-100`) e a sombra navy. Evite blocos de cor sólida "pesados".
- **Ícones:** SVG de **linha**, `currentColor`, stroke ~`1.5–1.8`, caps arredondados,
  grid `24`. **Sem emoji, sem icon font.**
- **Animação:** contida. Press = opacidade `0.7`. Hover em rows = tint claro. Sem
  bounce/parallax. `--transition-base: 150ms ease`.

---

## 3. Tokens → classes Tailwind

### 3.1 Cores

Prefira os **aliases semânticos** (coluna "Alias") nas telas quando existirem; caiam
para as escalas quando precisar de um tom específico.

| Token CSS | Hex | Alias semântico | Classe Tailwind | Uso |
|---|---|---|---|---|
| `--color-primary-300` | `#0E1955` | `--text-strong`, `--surface-brand`, `--border-strong` | `text-primary-300` / `bg-primary-300` / `border-primary-300` | Navy da marca — ações primárias, títulos, estados ativos |
| `--color-primary-200` | `#C0CDDE` | `--border-input` | `border-primary-200` / `bg-primary-200` | Bordas de input, chips inativos, disabled |
| `--color-primary-100` | `#E9EDF1` | `--border-default`, `--surface-subtle` | `border-primary-100` / `bg-primary-100` | Bordas de card, fills leves, **bloco de bloqueio** |

> **Equivalência `--surface-subtle` ≡ `bg-primary-100`** (o MESMO `#E9EDF1`): é ao
> mesmo tempo a **borda leve de card** e o **fill do bloqueio explicável** (estado
> do plano, nunca vermelho — §4.1). São o mesmo token de propósito — não crie um
> segundo para "fundo de aviso" (ADR-038).
| `--color-accent-300` | `#FA8F1B` | `--surface-accent`, `--focus-ring` | `text-accent-300` / `bg-accent-300` | Laranja — ênfase, CTA de destaque (com parcimônia) |
| `--color-accent-200` | `#FFC88E` | — | `bg-accent-200` / `border-accent-200` | Laranja suave — accent disabled, borda de alerta |
| `--color-accent-100` | `#FEDDBC` | — | `bg-accent-100` | Laranja pálido — fills de destaque |
| `--color-text` | `#37474F` | `--text-body` | `text-ink` | Texto de corpo (blue-gray) |
| `--color-gray-300` | `#838383` | `--text-muted` | `text-muted` | Labels, captions, placeholder |
| `--color-gray-50` | `#FBFBFB` | `--surface-page` | `bg-page` | Fundo de página/superfície |
| `--color-white` | `#FFFFFF` | `--surface-card`, `--text-on-brand` | `bg-white` / `text-white` | Cards, texto sobre navy |
| `--color-success` | `#29B01D` | — | `text-success` / `bg-success` | Sucesso (Badge "Plano ativo", "Feito") |
| `--color-error` | `#CD1919` | — | `text-error` / `bg-error` | **Erro real** — nunca bloqueio de regra |
| `--color-alert` | `#FBC70F` | — | `text-alert` / `bg-alert` | Alerta |

> As escalas default do Tailwind (`slate`, `emerald`, `rose`, `sky`, …) **continuam
> existindo** porque o theme é estendido, não substituído — mas **não são para telas
> novas** (ver §5).

### 3.2 Sombras

| Token | Classe | Valor |
|---|---|---|
| `--shadow-card` | `shadow-card` | `0 4px 24px rgba(14,25,85,0.08)` |
| `--shadow-raised` | `shadow-raised` | `0 8px 32px rgba(14,25,85,0.12)` |
| `--shadow-button` | `shadow-button` | `0 4px 12px rgba(250,143,27,0.24)` |

### 3.3 Raios

| Classe | Valor | Uso |
|---|---|---|
| `rounded-sm` | `8px` | inputs, blocos internos |
| `rounded-md` | `12px` | ícone-container, blocos de motivo |
| `rounded-lg` | `16px` | cards e botões |
| `rounded-pill` | `999px` | pills, chips |

> **Atenção:** o theme sobrescreve `rounded-sm/md/lg` do Tailwind (defaults 2/6/8px).
> É intencional — o DS usa só esses raios. `rounded-full`, `rounded-xl`, etc. seguem os
> defaults. Estes raios são literais no config (o `md`=12 não tem token equivalente na
> escala do handoff, que é 8/16/24/40/999); os valores em `tokens.css`
> (`--radius-small/medium/large/…`) preservam a escala original do designer para diff.

### 3.4 Layout

| Chave | Classe | Valor |
|---|---|---|
| `maxWidth.shell` | `max-w-shell` | `1240px` |

Spacing **não** é customizado — a escala default de 4px do Tailwind já cobre os valores
do app (`5,8,15,16,24,32,40,48,64`).

---

## 4. Princípios invioláveis (UX)

1. **Bloqueio explicável.** Nada é só "indisponível". Todo bloqueio diz *o quê, por quê
   e a partir de quando*, em tom neutro. Bloqueio por regra é **estado do plano, não
   erro** → **NUNCA em vermelho** (`error`) nem apagado a ponto de sumir. Padrão visual:
   fundo `--surface-subtle`, ícone de linha navy, data/ação em **bold**.
2. **Sigilo LGPD visível.** Dados individuais não vão ao RH. Reforce em microcopy nos
   momentos de captura: *"Só você vê o que registra aqui."*
3. **Sem diagnóstico na UI.** Exiba faixas, sinais, encaminhamentos — jamais rótulo
   clínico.
4. **"Pedir ajuda" permanente no header.** Sempre no mesmo lugar (canto direito),
   discreto em navy — distinto do CTA laranja e do vermelho de erro. Roteia para canal
   clínico, nunca engajamento.
5. **Anti-engajamento agressivo.** Sem streaks, culpa ou gamificação. Check-in perdido
   = silêncio gentil, nunca cobrança.
6. **Botões em UPPERCASE via classe CSS `uppercase`, NUNCA no texto-fonte.** Escreva
   `AGENDAR` como `<Button>Agendar</Button>` com a classe `uppercase`. Isso preserva o
   nome acessível ("Agendar") para leitores de tela e mantém `getByRole('button',
   { name: /agendar/i })` funcionando nos testes.
7. **Microcopy pt-BR:** trate por **"você"** + **primeiro nome** (`Olá, Ana`). Frases
   curtas, causa antes da consequência, sem exclamação em momentos sensíveis.

---

## 5. Proibições

- **Sem `emerald`/`slate`** (nem `rose`/`sky`) em tela nova — use os tokens do DS.
- **Sem hex hardcoded** — sempre token (`var(--…)`) ou classe Tailwind do theme.
- **Sem `/alpha` sobre cores de token** (`bg-primary-300/50` não funciona com `var()`)
  — use os tints `100/200` ou a opacidade do elemento (`--opacity-*`).
- **Sem recalcular regra de negócio no front.** O servidor calcula (elegibilidade,
  janelas, cotas), o front **exibe** o que vem pronto. Ver ADR-017 e o
  `apps/web/CLAUDE.md`.

---

## 6. Layout desktop

- **Container:** `max-w-shell` (1240px) centralizado, com `px-10` (40px lateral).
- **Header sticky:** altura ~`70px`, `position: sticky`, borda inferior `border-default`.
  Conteúdo: logo, nav (Jornada / Consultas / Perfil com estado ativo), pill "Pedir
  ajuda", avatar.
- **Grid de duas colunas:** coluna principal + **aside `sticky`** (check-in, renovação,
  próxima consulta). Colapsa para coluna única abaixo de ~960px.
- **Cards:** `rounded-lg` (16px), `border border-primary-100`, `shadow-card`, fundo
  branco.

Logos disponíveis em `src/assets/logos/`: `logo-blue.svg` (marca completa) e
`logo-icon.svg` (símbolo).

---

## 7. Inventário de componentes (`src/shared/ui/`) — REAL

**19 componentes.** Os 17 originais das Etapas 1a/1b (desktop) + `FlowHeader` e
`TabBar` (chrome mobile, §9). Cada um tem teste colocalizado (`*.test.tsx`).

### Primitivos

| Componente | Uso / props resumidos |
|---|---|
| `Button` | `color` (`primary`\|`accent`\|`ghost`), `size` (`sm`\|`md`\|`lg`), `loading`, `fullWidth`, `disabled`. Rótulo UPPERCASE via classe `uppercase` (nome acessível preservado). Disabled = tint `-200` da própria cor, nunca cinza. |
| `Card` | Superfície branca padrão: `rounded-lg`, borda `primary-100`, `shadow-card`, `padding` (`md`\|`lg`), `as` (tag). |
| `Badge` | `tone` (`success`\|`neutral`\|`accent`\|`alert`); fundos rgba literais do DS (não `/alpha`). NUNCA sinaliza bloqueio de regra — isso é `EligibilityNotice`. |
| `Input` | `label` acima do campo, `error`, `inputMode`, etc. |
| `Avatar` | Iniciais a partir do nome; usado no header e no Perfil. |
| `ListRow` | Linha clicável (ícone + título + caption + caret); hover = tint claro. |
| `SegmentedControl` | Alternador (Entrar/Criar conta; Próximas/Histórico); teclado + `aria`. |
| `Toggle` | Switch on/off com `role="switch"` + `aria-checked`. |
| `icons` | Set de ícones de linha `currentColor` (grid 24). Sem emoji/icon font. |
| `feedback`: `Loading` / `Empty` / `ErrorNotice` | Estados transversais. `ErrorNotice`: indisponibilidade (503/`LEGACY_UNAVAILABLE`) é informativa (âmbar, `role=status`); erro real é `error` (`role=alert`). |

### Padrões proprietários (vocabulário do app)

| Componente | Uso resumido |
|---|---|
| `EligibilityNotice` | Bloco de bloqueio explicável (§4.1): fundo `surface-subtle` (= `bg-primary-100`), ícone de linha por `rule_type`, frase + data/ação em bold. NUNCA vermelho. |
| `CareItemCard` | Card de item de linha de cuidado: ícone + título + slot de ação (LIBERADO/BLOQUEADO/PRÉ-REQ/FEITO). |
| `PlanValidityBanner` | Faixa de vigência do plano (Badge + progresso + caption); vira alerta laranja perto de expirar. |
| `HelpPill` | Pill "Pedir ajuda" do header (branca, borda `primary-200`, texto navy bold). O popover e a lógica são do `HelpNowMenu` (feature). |
| `LineChips` | Chips de alternância de linha de cuidado (ativa: navy sólido; inativa: branca com borda `primary-200`). |
| `DateBadge` | Selo de data curto (mês + dia) para consultas. `timeZone` **obrigatório** (lê o dia no fuso da agenda). **Zero-pad:** o dia usa `day: '2-digit'` (single-digit vira `05`, alinhando o selo); o mês abrevia com `month: 'short'` e tira o ponto final do pt-BR (`jul.` → `jul`). |
| `AppShell` | Chrome do app (desktop E mobile — ADR-041): **skip-to-content** ("Pular para o conteúdo", visually-hidden até o foco, alvo `<main id="conteudo">`) + no desktop header sticky 70px/`max-w-shell`, no mobile container `max-w-[430px]` + `mobileVariant` (`tabs`\|`flow`, §9.2). Presentacional puro (o wiring é do `AppLayout`). |
| `TabBar` | Tab bar fixa das telas raiz no mobile (§9.3): 3 abas, ícone outline→filled no ativo, `z-30`, `safe-area-inset-bottom`. |
| `FlowHeader` | Header dos fluxos empilhados no mobile (§9.4): voltar 36px + logo-icon + eyebrow + título + slot de ajuda + progresso opcional (`{ pct, caption }`). |

### Superfícies de DS que vivem na feature (não em `shared/ui/`)

Ficam em `features/` por acoplamento a hooks/domínio, mas seguem o DS:

| Componente | Onde | Uso |
|---|---|---|
| `MoodGrid` | `features/mood/` | Grade valência×energia (gradientes oklch): clique/arraste por Pointer Events (com `onPointerCancel`), teclado (setas, passo 5) e o ponto anunciado por `aria-live`. Controlado (`value`/`onChange`). |
| `MoodCheckinCard` | `features/mood/` | O check-in de humor do aside da Jornada — máquina de estados (consentimento/elegibilidade/feito/aprofundamento). ÚNICA superfície do check-in diário (a `/humor` foi aposentada — ADR-039). |
| `HelpNowMenu` | `features/mood/` | O popover do "Pedir ajuda": um clique dispara a API; fecha com **Escape** e ao **trocar de rota**. |

---

## 8. Referências

- Tokens: `apps/web/src/styles/tokens.css` · Fontes: `apps/web/src/styles/fonts.css`
- Config: `apps/web/tailwind.config.js` · Bootstrap: `apps/web/src/index.css`
- Convenções do front: `apps/web/CLAUDE.md`
- Handoff do designer (fora do repo): `design_handoff_webapp_desktop/` — `README.md`
  (tokens) e `design_files/Renovi 2.0 - Design System do App do Paciente.md` (princípios).

---

## 9. Layout mobile

O breakpoint estrutural é `lg` (**1024px**, `DESKTOP_QUERY` em `shared/viewport.ts`):
abaixo dele o app troca de CHROME (tab bar em vez de header+nav), não só de
espaçamento. Ver ADR-041.

### 9.1 Regra: hook vs. classes `lg:`

- **Estrutura muda → hook.** Quando o mobile precisa de um elemento a MAIS/A MENOS,
  ou de um COMPONENTE diferente (tab bar em vez de nav, `FlowHeader` em vez de
  header sticky), a decisão vem de `useIsDesktop()` (`shared/viewport.ts`) — nunca
  de duas árvores JSX condicionadas só por classe.
- **Só estilo muda → classes `lg:`.** Espaçamento, tamanho de fonte, ordem visual
  dentro do MESMO elemento seguem os utilitários responsivos do Tailwind.
- **Proibido dual-render do mesmo accessible name.** Renderizar o MESMO elemento
  (ex.: um botão "Agendar") duas vezes — uma escondida em mobile, outra em
  desktop, só com `hidden lg:block` — quebra `getByRole` nos testes: o jsdom não
  computa CSS, então as duas cópias "aparecem" simultaneamente e colidem. Um
  componente com estado (inputs, toggles) duplicaria a fonte da verdade. É por
  isso que o chrome é decidido no `AppShell`/`AppLayout`, uma única árvore.
- `useIsDesktop()` usa `useSyncExternalStore` sobre `window.matchMedia`; o jsdom
  do projeto **não implementa** `matchMedia` — o default quando ele não existe é
  `true` (DESKTOP), o que preserva os ~200 testes escritos antes do mobile sem
  editar nenhum. Telas que chamam o hook diretamente simulam o viewport via
  `shared/viewport.testkit.ts` (`mockViewport('mobile' | 'desktop')` →
  `{ set, restore }`).

### 9.2 Chrome: `tabs` × `flow`

`AppShell` recebe `mobileVariant: 'tabs' | 'flow'` (ignorado no desktop). Quem
decide é o `AppLayout`, por `matchPath` contra as rotas de fluxo:

```
'/jornada/agendar/:itemId'
'/consultas/:appointmentId'
'/avaliacoes/:codigo'
```

- **`tabs`** (telas raiz — Jornada/Consultas/Perfil): `main` em
  `max-w-[430px] px-5` (container 430px, gutter 20px) com `pb-[110px]` de
  clearance para a `TabBar`; faixa de logo **não-sticky** no topo (rola com o
  conteúdo, ao contrário do header desktop); `TabBar` fixa no rodapé.
- **`flow`** (fluxos empilhados — Agendar, detalhe da consulta, avaliação): sem
  faixa de logo, sem `TabBar` (`pb-10` de respiro); a própria página renderiza um
  `FlowHeader`.

### 9.3 `TabBar`

Fixa (`fixed bottom-0`), largura `max-w-[430px]` centralizada, `z-30` (mesma
camada do header sticky do desktop — nunca coexistem). Três abas (Jornada,
Consultas, Perfil); o item ativo troca o ícone outline pelo par **filled** (não
só cor) e ganha `font-bold`; o inativo fica com `opacity-55`. Padding inferior
`pb-[calc(18px+env(safe-area-inset-bottom))]` — respeita a home indicator do
iOS sem inflar demais o espaço em telas sem notch.

### 9.4 `FlowHeader`

Header dos fluxos empilhados: botão voltar circular de **36px**
(`backTo` via `Link` OU `onBack` via `button` — exatamente um) + `logo-icon` +
`eyebrow` (rótulo UPPERCASE acima do título) + `title` (20px bold navy) + slot
`help` opcional à direita + barra de progresso opcional
(`progress: { pct, caption }`, ex.: "Passo 2 de 3"). Presentacional puro — a
página decide o conteúdo.

### 9.5 Exceção: ícones filled do tab bar

`IconHomeFilled` / `IconAppointmentsFilled` / `IconProfileFilled` (`shared/ui/icons.tsx`)
são transcritos **verbatim** do handoff, não redesenhados a partir do outline. Duas
exceções documentadas às regras de §2 ("SVG de linha", grid 24):
- **`viewBox` nativo `0 0 21 21`** (não o grid 24 dos demais ícones).
- O detalhe interno das formas preenchidas (contorno/pontos sobre o fundo sólido
  navy) usa o token `var(--color-white)` em vez de `currentColor` — é
  literalmente branco por cima do preenchimento, não uma cor de tema.

### 9.6 Escala de z-index

| Camada | z-index | Onde |
|---|---|---|
| Header sticky desktop | `z-30` | `AppShell` |
| `TabBar` mobile | `z-30` | `shared/ui/TabBar.tsx` (mesma camada do header — nunca ambos montados) |
| Popover "Pedir ajuda" (`HelpNowMenu`) | `z-40` | `features/mood/HelpNowMenu.tsx` — sempre por cima do chrome |

### 9.7 Tints translúcidos: rgba literal é o padrão sancionado

Vários blocos "de sucesso" no mobile (ex.: badge de "Feito" na jornada, resumo do
perfil, histórico de consultas) usam `bg-[rgba(41,176,29,0.12)]` — o MESMO valor
do tone `success` do `Badge` (§7). Não é gambiarra: é o padrão já sancionado no
ADR-038 (rgba literal, nunca `/alpha` sobre token) repetido onde o `Badge` em si
não serve (o bloco não é uma pill de status). **Melhoria futura:** extrair um
token `--surface-success-subtle` (ou classe utilitária) para não repetir o
literal em múltiplos arquivos — não bloqueia nada hoje.
