# CLAUDE.md — Front (apps/web)

Contexto local do front React. Regras gerais no `CLAUDE.md` da raiz.

## Stack

React 18 · TypeScript · Vite · Tailwind · **TanStack Query** (cache/refetch da
jornada e elegibilidade) · Vitest + Testing Library. Consumo da API via proxy
`/api` (dev) → API Go em `:8090`.

## Estrutura

```
src/
  main.tsx           # bootstrap + QueryClientProvider (<StrictMode>)
  App.tsx            # tabela de rotas do paciente
  features/<feat>/   # cada tela/feature (colocation: componente + hooks + teste)
  shared/            # api client (shared/api.ts), helpers (datetime, masks, navigate)
  shared/viewport.ts # useIsDesktop() (hook de chrome mobile×desktop) + viewport.testkit.ts (mockViewport) — ADR-041
  shared/ui/         # 19 componentes do design system (ver docs/DESIGN-SYSTEM.md §7; layout mobile §9)
  styles/            # tokens.css + fonts.css (fonte de verdade do DS)
  assets/            # fonts/ (Nunito .ttf) + logos/ (logo-blue, logo-icon)
  index.css          # imports de fonts/tokens ANTES do @tailwind + @layer base
  setupTests.ts      # matchers do Testing Library
```

**Features VIVAS após o redesign desktop (Etapas 0–8):**

| Feature | Tela / papel |
|---|---|
| `auth/` | Acesso: login por CPF + cadastro 3 passos (ViaCEP); `useSession`, `ProtectedRoute` |
| `journey/` | Minha Jornada (hero, timeline, aside) + Agendar por item (stepper, motor) |
| `consultations/` | Consultas: abas Próximas/Histórico (+ CTA de agendar do aside) |
| `scheduling/` | Só o **detalhe** da consulta (`AppointmentPage`) com o **gate de pré-consulta**; hooks `useAppointment`/`useJoinAppointment`/`proximoPoll` |
| `mood/` | Check-in no aside da Jornada (`MoodCheckinCard`/`MoodGrid`), instrumentos (`AssessmentForm`/`AssessmentPage`), `HelpNowMenu` |
| `profile/` | Perfil reduzido: conta + plano + privacidade (revogar consentimento) |
| `shell/` | `AppLayout` — liga sessão + "Pedir ajuda" ao `AppShell` |

**Aposentadas no redesign (removidas — ADR-039):** `home/` (HomePage) e
`health/` (HealthBadge); o wizard de especialidade (`SchedulingPages`,
`SlotPickerPage`) e a **lista** `AppointmentsPage`; `journey/CareAppointmentsPage`
(virou `ConsultationsPage`); a `MoodPage` (a `/humor` redireciona para `/jornada`).

Todas as features acima são **same-codebase**: mobile (`< lg`) e desktop
(`≥ lg`) compartilham componente e hooks — só o layout muda por viewport
(Etapas 0–8 do mobile, ADR-041/042). Nenhuma feature nova; ver "Mobile
responsivo" nas Convenções.

## Convenções

- **Uma pasta por feature** em `src/features/` (colocation: componente + hooks + teste juntos).
- **Dados da API sempre via TanStack Query** (`useQuery`/`useMutation`), nunca `fetch` solto num componente.
- **Cliente HTTP:** o cliente manual (`shared/api.ts`) **permanece** como a camada de
  API do front. A geração via **orval** (a partir do OpenAPI, `packages/api-client`)
  está **adiada** (decisão de 2026-07-20); não é a fonte atual. Ao trocar, migrar de uma vez.
- **Estilo:** Tailwind com o **design system** — tokens em `src/styles/tokens.css`,
  theme em `tailwind.config.js`. Regras completas em **`docs/DESIGN-SYSTEM.md`**
  (leia antes de estilizar tela nova) e ADR-038. Sem hex hardcoded, sem
  `emerald`/`slate`/`rose` em código vivo, sem `/alpha` sobre cores de token
  (use tints `100/200`); sem CSS solto salvo necessidade. O gate é o sweep
  `grep -rn "emerald\|slate\|rose-" apps/web/src` **zerado** (fora comentários e o
  falso-positivo `translate`).
- **Mobile responsivo (ADR-041/042):** abaixo de `lg` (1024px) o chrome muda —
  `AppShell` vira `tabs` (tela raiz + `TabBar`) ou `flow` (fluxo empilhado com
  `FlowHeader`), decidido pelo `AppLayout` via `matchPath` contra as rotas de
  fluxo (`/jornada/agendar/:itemId`, `/consultas/:appointmentId`,
  `/avaliacoes/:codigo`). **Regra do hook:** estrutura muda (elemento a
  mais/a menos, componente diferente) → `useIsDesktop()` (`shared/viewport.ts`);
  só estilo muda (espaçamento, fonte, ordem) → classes `lg:` no MESMO elemento.
  **Proibido dual-render** do mesmo *accessible name* entre mobile/desktop — o
  jsdom não computa CSS, então as duas cópias colidem no `getByRole`, e um
  componente com estado duplicaria a fonte da verdade. **Testes que dependem do
  viewport** usam `mockViewport('mobile' | 'desktop')`
  (`shared/viewport.testkit.ts`, `{ set, restore }`) — sem ele, o hook assume
  DESKTOP (default quando o jsdom não implementa `matchMedia`, o que preserva os
  testes anteriores ao mobile sem edição). Detalhe completo em
  `docs/DESIGN-SYSTEM.md` §9.
- **Ícones filled do tab bar são exceção ao grid de ícones (§9.5):**
  `IconHomeFilled`/`IconAppointmentsFilled`/`IconProfileFilled`
  (`shared/ui/icons.tsx`) usam `viewBox` nativo `0 0 21 21` (não o grid 24 dos
  demais ícones), nos três — transcritos **verbatim** do handoff, não
  redesenhados a partir do outline. O detalhe interno NÃO é uniforme:
  `IconAppointmentsFilled`/`IconProfileFilled` têm preenchimento sólido navy com
  o contorno/pontos internos em `var(--color-white)` por cima; `IconHomeFilled`
  não tem preenchimento sólido (só os traços do teto/parede), então seus dois
  `<path>` internos ficam em `stroke="currentColor"` mesmo — não há
  branco-sobre-navy para resolver ali.
- **Gate de pré-consulta (ADR-039):** em `scheduling/AppointmentPage`, ao clicar
  "Entrar" com uma oferta ativa (`today.offer` = WHO-5/PHQ-4), o `AssessmentForm`
  aparece ANTES de abrir a sala. Ele **nunca prende** o paciente: avaliado só no
  clique; trava após a 1ª vez; erro/"Fechar" liberam a entrada. As props do
  `AssessmentForm` (`{ codigo, onDone }`) são contrato — a página e o gate dependem.
- **Sessão:** cookie `httpOnly` — o JS NÃO o lê. Todo fetch usa `credentials: 'include'`;
  nunca guarde token em `localStorage`. Saber quem está logado = perguntar ao servidor (`useSession`).
- **O cadastro demora de verdade** (é síncrono contra a Doutor ao Vivo, que já levou dezenas
  de segundos). Toda tela que o dispare precisa dizer isso — spinner mudo faz o usuário
  recarregar no meio.
- **Regra de ouro de UX (do motor):** nunca mostre botão só desabilitado — mostre o
  motivo traduzido do `reason` (ex.: "Você já usou sua consulta desta semana").
  A tabela de tradução é `features/scheduling/reasons.ts` — uma só, servindo erro e veredito.
- **Data/hora SEMPRE via `shared/datetime`**, que exige o `timeZone` como parâmetro.
  Nunca `toLocaleTimeString()` solto: ele usa o fuso do BROWSER, e os horários vêm
  do legado como hora de parede de São Paulo — um paciente viajando veria 12:00
  numa consulta das 09:00, sem erro nenhum aparecer.
- **Regra que o servidor calcula, o front NÃO recalcula.** A janela de entrada
  chega como `join.opens_at`, já pronto: "30 minutos" não existe no front (ADR-017).
  Vale para tudo o que vier do motor de elegibilidade também.
- **`Idempotency-Key` do agendamento nasce com a INTENÇÃO** (o clique no horário),
  não com a requisição: o retry do mesmo agendamento reusa a key (replay 200, sem
  consulta duplicada); escolher outro horário gera uma key nova. Ver
  `features/journey/ScheduleCarePage.tsx` (ADR-025).

## Comandos

```bash
make web-install   # npm install
make web-dev       # dev server (http://localhost:5173)
make web-test      # vitest
npm --prefix apps/web run build       # build de produção
npm --prefix apps/web run typecheck   # tsc --noEmit
```

## Ao terminar

Atualize `docs/PROGRESSO.md` (telas concluídas) e este arquivo se mudou o padrão de estrutura.
