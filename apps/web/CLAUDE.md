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
  shared/ui/         # 17 componentes do design system (ver docs/DESIGN-SYSTEM.md §7)
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
