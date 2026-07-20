# Design System — Front (apps/web)

> Documento vivo do design system do app do paciente (Renovi 2.0), versão **web desktop**.
> Fonte visual: handoff do designer (tokens + telas de referência). Fonte de verdade
> técnica dos valores: `apps/web/src/styles/tokens.css` (CSS custom properties).
> **Idioma:** código/classes em inglês; este doc em PT-BR.

Esta é a **fundação** (Etapa 0): tokens, fontes, theme do Tailwind e diretrizes. A
biblioteca de componentes (`src/shared/ui/`) e as telas vêm nas etapas seguintes.

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

## 7. Inventário de componentes planejados (`src/shared/ui/`)

A construir nas **Etapas 1a/1b**. Nenhum existe ainda; a lista abaixo é o contrato
pretendido (props/uso resumidos) para orientar as próximas etapas.

### Primitivos

| Componente | Uso / props resumidos |
|---|---|
| `Button` | `color` (`primary`\|`accent`\|`ghost`), `size` (`sm`\|`md`\|`lg`), `disabled`, `onClick`. Rótulo UPPERCASE via classe `uppercase`; full-width em formulários. |
| `Card` | Superfície branca padrão: `rounded-lg`, borda `primary-100`, `shadow-card`, padding interno. Slots de título/ação. |
| `Badge` | `tone` (`success`\|`neutral`\|`alert`), texto curto (ex.: "Plano ativo", "Hoje"). |
| `Input` | `label` (acima do campo), `type`, `placeholder`, `value`, `onChange`, `error`, `inputMode`. Altura ~80px com label. |
| `Avatar` | Iniciais ou imagem; tamanhos; usado no header e no Perfil. |
| `ListRow` | Linha clicável (ícone + título + caption + caret); hover = tint claro. |
| `SegmentedControl` | Alternador (ex.: Entrar/Criar conta; Próximas/Histórico); teclado + `aria`. |
| `Toggle` | Switch on/off com `role="switch"` + `aria-checked` (Notificações). |
| `icons` | Set de ícones de linha `currentColor` (grid 24). Sem emoji/icon font. |
| feedback: `Loading` / `Empty` / `ErrorNotice` | Estados transversais de carregamento, vazio e erro real. |

### Padrões proprietários (vocabulário do app)

| Componente | Uso resumido |
|---|---|
| `EligibilityNotice` | Bloco de bloqueio explicável (§4.1): fundo `surface-subtle`, ícone de linha, frase + data/ação em bold. Reutilizável em cards, horários e botões. |
| `CareItemCard` | Card de item de linha de cuidado: ícone + título + quota dots + slot de ação (LIBERADO/BLOQUEADO/PRÉ-REQ/FEITO). |
| `PlanValidityBanner` | Faixa de vigência do plano (Badge + barra de progresso + caption); vira alerta laranja perto de expirar, com CTA "RENOVAR PLANO". |
| `HelpPill` | Pill "Pedir ajuda" do header (branca, borda `primary-200`, texto navy bold, ícone de linha). |
| `LineChips` | Chips de alternância de linha de cuidado (ativa: navy sólido/texto branco; inativa: branca com borda `primary-200`). |
| `DateBadge` | Selo de data curto (ex.: "Hoje", dia/mês) para consultas. |
| `AppShell` | Layout desktop: header sticky 70px + container `max-w-shell` + grid 2 colunas com aside sticky. |

---

## 8. Referências

- Tokens: `apps/web/src/styles/tokens.css` · Fontes: `apps/web/src/styles/fonts.css`
- Config: `apps/web/tailwind.config.js` · Bootstrap: `apps/web/src/index.css`
- Convenções do front: `apps/web/CLAUDE.md`
- Handoff do designer (fora do repo): `design_handoff_webapp_desktop/` — `README.md`
  (tokens) e `design_files/Renovi 2.0 - Design System do App do Paciente.md` (princípios).
