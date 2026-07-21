import { useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { ApiError, type AnnotatedSlot, type CareAppointment } from '../../shared/api';
import { FUSO_PADRAO, dayKey, formatDateLong, formatTime } from '../../shared/datetime';
import { Button } from '../../shared/ui/Button';
import { Card } from '../../shared/ui/Card';
import { EligibilityNotice } from '../../shared/ui/EligibilityNotice';
import { FlowHeader } from '../../shared/ui/FlowHeader';
import { Empty, ErrorNotice, Loading } from '../../shared/ui/feedback';
import { IconBack, IconCheck } from '../../shared/ui/icons';
import { useIsDesktop } from '../../shared/viewport';
import { HelpNowMenu } from '../mood/HelpNowMenu';
import { reasonText } from '../scheduling/reasons';
import { CalendarGrid } from './schedule/CalendarGrid';
import { ConfirmCard } from './schedule/ConfirmCard';
import { DateStep, type DiaResumo } from './schedule/DateStep';
import { ProfessionalStep, type ProfissionalResumo } from './schedule/ProfessionalStep';
import { ResumoBar } from './schedule/ResumoBar';
import { Stepper, type PassoEstado, type PassoInfo } from './schedule/Stepper';
import { TimeStep } from './schedule/TimeStep';
import { useAvailability, useJourney, useScheduleCare } from './useJourney';

/**
 * A intenção viva: o horário que o paciente escolheu, a key que o identifica, e
 * o fuso do slot NO MOMENTO da escolha. O fuso viaja com a intenção (e não só
 * via `slotEscolhido`) porque o `onSettled` do agendamento refaz a
 * disponibilidade mesmo no ERRO — e num 409 SLOT_TAKEN o refetch legitimamente
 * remove o horário da lista. Sem essa cópia, o feedback de erro perderia o fuso
 * para formatar a data de desbloqueio junto com o slot que sumiu.
 */
type Intencao = { slotId: string; key: string; timeZone: string };
type Passo = 1 | 2 | 3;

/**
 * Agendar a consulta de um item da linha (SPEC §7), como wizard de 3 passos —
 * Profissional → Data → Horário — sobre `useAvailability`. A regra de negócio mais
 * crítica do app vive aqui: a Idempotency-Key nasce com a INTENÇÃO (ADR-025), não
 * com a chamada. Escolher um horário cria a key; um retry do MESMO horário reusa a
 * mesma key (o servidor devolve a mesma consulta sem duplicar); escolher OUTRO
 * horário é outra intenção, e nasce outra key. No sucesso a tela FICA: mostra a
 * confirmação e a disponibilidade se refaz (o horário tomado some).
 */
export function ScheduleCarePage() {
  const { itemId } = useParams();
  const { data, isLoading, error } = useAvailability(itemId);
  const journey = useJourney();
  const agendar = useScheduleCare();
  const isDesktop = useIsDesktop();

  const [passo, setPasso] = useState<Passo>(1);
  const [profId, setProfId] = useState<string | null>(null);
  const [diaKey, setDiaKey] = useState<string | null>(null);
  const [intencao, setIntencao] = useState<Intencao | null>(null);

  const slots = useMemo(() => data?.items ?? [], [data]);

  // O rótulo do item não vem na disponibilidade — a jornada é a fonte. Uso apenas
  // de leitura (a tela anterior já a aqueceu); se faltar, um termo neutro serve.
  const label = useMemo(() => rotuloDoItem(journey.data, itemId), [journey.data, itemId]);
  // A recorrência (texto livre da API) alimenta SÓ a oferta de sucesso no mobile —
  // exibida verbatim; nada de contagem inventada ("você ainda tem 1 consulta").
  const recurrence = useMemo(() => recorrenciaDoItem(journey.data, itemId), [journey.data, itemId]);

  const profissionais = useMemo(() => derivarProfissionais(slots), [slots]);
  const dias = useMemo(() => derivarDias(slots, profId), [slots, profId]);
  const slotsDoDia = useMemo(() => derivarSlotsDoDia(slots, profId, diaKey), [slots, profId, diaKey]);
  const profEscolhido = profissionais.find((p) => p.id === profId) ?? null;
  const slotEscolhido = slots.find((s) => s.id === intencao?.slotId) ?? null;

  function escolherProfissional(id: string) {
    setProfId(id);
    setDiaKey(null);
    setIntencao(null);
    agendar.reset();
    setPasso(2);
  }

  function escolherDia(k: string) {
    setDiaKey(k);
    setIntencao(null);
    agendar.reset();
    setPasso(3);
  }

  function escolherHorario(slot: AnnotatedSlot) {
    // Re-clicar o MESMO horário não é uma nova intenção: preserva a key (o retry
    // reusa exatamente ela). Um horário diferente é outra intenção → key nova.
    if (intencao?.slotId === slot.id) return;
    setIntencao({ slotId: slot.id, key: crypto.randomUUID(), timeZone: slot.time_zone });
    agendar.reset();
  }

  // Voltar a um passo já feito limpa as escolhas POSTERIORES (e a intenção/key
  // junto). Em voo, a navegação trava — não se abandona um POST no meio.
  function irParaPasso(n: number) {
    if (agendar.isPending) return;
    if (n <= 2) {
      setIntencao(null);
      agendar.reset();
    }
    if (n <= 1) setDiaKey(null);
    setPasso(n as Passo);
  }

  function confirmar() {
    if (!itemId || !intencao) return;
    agendar.mutate({ body: { item_id: itemId, slot_id: intencao.slotId }, idemKey: intencao.key });
  }

  const passos: PassoInfo[] = [
    {
      titulo: 'Profissional',
      caption: profEscolhido?.full_name ?? 'Escolha o profissional',
      estado: estado(1, passo),
    },
    {
      titulo: 'Data',
      caption: captionDia(dias, diaKey),
      estado: estado(2, passo),
    },
    {
      titulo: 'Horário',
      caption: slotEscolhido ? formatTime(slotEscolhido.starts_at, slotEscolhido.time_zone) : 'Escolha o horário',
      estado: estado(3, passo),
    },
  ];

  // ── Mobile (< lg): fluxo linear empilhado sob o FlowHeader (mock `Agendar`). ──
  // Bifurcação SÓ de apresentação: reusa o MESMO estado, as MESMAS derivações e a
  // MESMA idempotência do desktop — nenhuma regra de agendamento muda aqui.
  if (!isDesktop) {
    const sucesso = agendar.isSuccess && Boolean(agendar.data);
    const tituloPasso = passo === 1 ? 'Com quem?' : passo === 2 ? 'Que dia?' : 'Que horário?';
    const voltar =
      sucesso || passo === 1
        ? { backTo: '/jornada' }
        : { onBack: () => irParaPasso(passo - 1) };

    return (
      <div className="flex flex-col gap-5">
        <FlowHeader
          eyebrow={`Agendar · ${label}`}
          title={sucesso ? 'Tudo certo' : tituloPasso}
          help={<HelpNowMenu />}
          progress={
            sucesso
              ? undefined
              : { pct: Math.round((passo / 3) * 100), caption: `Passo ${passo} de 3` }
          }
          {...voltar}
        />

        {isLoading && <Loading label="Carregando horários…" />}
        {error && <ErrorNotice error={error} />}

        {/* O sucesso é HASTEADO acima do gate de slots: reservar o ÚLTIMO horário
            livre faz o refetch do `onSettled` voltar lista vazia — e a confirmação
            não pode sumir dando lugar ao Empty enquanto o FlowHeader ainda diz
            "Tudo certo". Sucesso → MobileSuccess (haja ou não slots); senão, lista
            vazia → Empty; senão, os passos. */}
        {sucesso && agendar.data ? (
          <MobileSuccess
            consulta={agendar.data}
            profNome={profEscolhido?.full_name ?? 'seu profissional'}
            recurrence={recurrence}
            onAgendarProxima={() => profEscolhido && escolherProfissional(profEscolhido.id)}
          />
        ) : data && slots.length === 0 ? (
          <Empty
            title="Nenhum horário disponível"
            hint="Não há horários agendáveis para este item nos próximos dias."
          />
        ) : (
          data &&
          slots.length > 0 && (
            <>
              {passo === 1 && (
                <ProfessionalStep
                  profissionais={profissionais}
                  onEscolher={escolherProfissional}
                  colunaUnica
                />
              )}

              {passo === 2 && profEscolhido && (
                <div className="flex flex-col gap-3.5">
                  <ResumoBar nome={profEscolhido.full_name} onTrocar={() => irParaPasso(1)} />
                  <CalendarGrid dias={dias} selecionado={diaKey} onEscolher={escolherDia} />
                </div>
              )}

              {passo === 3 && profEscolhido && (
                <div className="flex flex-col gap-3.5">
                  <ResumoBar
                    nome={profEscolhido.full_name}
                    dia={captionDia(dias, diaKey)}
                    onTrocar={() => irParaPasso(2)}
                  />
                  <TimeStep
                    slots={slotsDoDia}
                    intencaoSlotId={intencao?.slotId ?? null}
                    emVoo={agendar.isPending}
                    onEscolher={escolherHorario}
                  />
                  <div className="flex flex-col gap-3">
                    {slotEscolhido && (
                      <ConfirmCard
                        label={label}
                        slot={slotEscolhido}
                        profissional={profEscolhido}
                        loading={agendar.isPending}
                        onConfirmar={confirmar}
                      />
                    )}
                    {agendar.isPending && <AvisoEspera />}
                    {agendar.isError && (
                      <FeedbackErro
                        error={agendar.error}
                        timeZone={intencao?.timeZone ?? FUSO_PADRAO}
                      />
                    )}
                  </div>
                </div>
              )}
            </>
          )
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <Link
          to="/jornada"
          className="inline-flex items-center gap-1 rounded-sm text-sm font-bold text-primary-300 transition hover:text-accent-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        >
          <IconBack size={18} />
          Voltar
        </Link>
      </div>

      <header className="flex flex-col gap-1">
        <span className="text-xs font-bold uppercase tracking-[0.08em] text-muted">
          Agendar · {label}
        </span>
        <h1 className="text-[38px] font-bold leading-[44px] text-primary-300">Agendar consulta</h1>
      </header>

      {isLoading && <Loading label="Carregando horários…" />}
      {error && <ErrorNotice error={error} />}
      {data && slots.length === 0 && (
        <Empty
          title="Nenhum horário disponível"
          hint="Não há horários agendáveis para este item nos próximos dias."
        />
      )}

      {data && slots.length > 0 && (
        <div className="grid gap-8 lg:grid-cols-[320px_minmax(0,1fr)] lg:items-start">
          <Card padding="lg" className="lg:sticky lg:top-[102px]">
            <Stepper passos={passos} atual={passo} onIr={irParaPasso} navegavel={!agendar.isPending} />
          </Card>

          <div className="min-w-0">
            {passo === 1 && (
              <ProfessionalStep profissionais={profissionais} onEscolher={escolherProfissional} />
            )}

            {passo === 2 && (
              <DateStep dias={dias} selecionado={diaKey} onEscolher={escolherDia} />
            )}

            {passo === 3 && (
              <div className="flex flex-col gap-6">
                <TimeStep
                  slots={slotsDoDia}
                  intencaoSlotId={intencao?.slotId ?? null}
                  emVoo={agendar.isPending}
                  onEscolher={escolherHorario}
                />

                {agendar.isSuccess && agendar.data ? (
                  <SucessoCard consulta={agendar.data} />
                ) : (
                  <div className="flex flex-col gap-3">
                    {/* O ConfirmCard precisa dos dados completos do slot (data,
                        horário) e por isso segue dependendo dele estar na
                        disponibilidade — sem o que confirmar, não há card. */}
                    {slotEscolhido && (
                      <ConfirmCard
                        label={label}
                        slot={slotEscolhido}
                        profissional={profEscolhido}
                        loading={agendar.isPending}
                        onConfirmar={confirmar}
                      />
                    )}
                    {agendar.isPending && <AvisoEspera />}
                    {/* FORA do gate de slotEscolhido: o `onSettled` refaz a
                        disponibilidade mesmo no erro, e num 409 SLOT_TAKEN o
                        refetch legitimamente remove o horário da lista — o
                        paciente não pode perder a explicação por isso. O fuso
                        vem da intenção (sobrevive ao slot sumir da lista). */}
                    {agendar.isError && (
                      <FeedbackErro
                        error={agendar.error}
                        timeZone={intencao?.timeZone ?? FUSO_PADRAO}
                      />
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/** Enquanto a DAV cria a consulta (45–90s), o paciente não pode fechar a página. */
function AvisoEspera() {
  return (
    <p role="status" className="rounded-md bg-primary-100 p-3 text-sm text-primary-300">
      Estamos reservando seu horário na Doutor ao Vivo. Isso pode levar de 45 a 90 segundos —{' '}
      <strong>não feche nem recarregue esta página.</strong>
    </p>
  );
}

/**
 * O desfecho de um erro na mutação. 422 = o horário existe e está livre, mas as
 * regras da linha barram ESTE paciente agora: mostramos os `blocks` do motor (a
 * frase pronta + a data de desbloqueio, no fuso do slot). Os demais erros viram a
 * frase do `reason.code` (ou o `detail` da API como recuo).
 */
function FeedbackErro({ error, timeZone }: { error: unknown; timeZone: string }) {
  if (error instanceof ApiError && error.status === 422 && error.blocks && error.blocks.length > 0) {
    return (
      <div role="alert" className="flex flex-col gap-2">
        <p className="text-sm font-bold text-primary-300">Não foi possível agendar este horário:</p>
        <EligibilityNotice blocks={error.blocks} timeZone={timeZone} />
      </div>
    );
  }
  return (
    <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-3 text-sm text-error">
      {reasonText(
        error instanceof ApiError ? error.reason : undefined,
        error instanceof Error ? error.message : 'Não foi possível agendar.',
      )}
    </p>
  );
}

/** No sucesso a tela FICA: confirmação + link para as consultas (não navega para fora). */
function SucessoCard({ consulta }: { consulta: CareAppointment }) {
  return (
    <Card padding="lg" className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-success text-white">
          <IconCheck size={20} />
        </span>
        <div className="flex flex-col">
          <h3 className="font-bold text-primary-300">Consulta agendada!</h3>
          <p className="text-sm text-muted first-letter:uppercase">
            {consulta.label} · {formatDateLong(consulta.scheduled_at, consulta.time_zone)} ·{' '}
            {formatTime(consulta.scheduled_at, consulta.time_zone)}
          </p>
        </div>
      </div>
      <Link
        to="/consultas"
        className="inline-flex w-fit rounded-sm text-sm font-bold text-primary-300 transition hover:text-accent-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
      >
        Ver minhas consultas
      </Link>
    </Card>
  );
}

/**
 * O sucesso MOBILE (mock `Agendar`, linhas 132–156): confirmação centrada + a
 * oferta de já marcar a próxima. A cópia da oferta vem SÓ de dados reais — a
 * recorrência da API, verbatim; nada de "você ainda tem N consultas" calculado.
 * "Agendar a próxima" reusa `escolherProfissional` (mesmo profissional → passo 2,
 * dia/intenção limpos, mutação resetada) — a disponibilidade já se refez no
 * `onSettled`. Sem os dots do mock (decisão herdada).
 */
function MobileSuccess({
  consulta,
  profNome,
  recurrence,
  onAgendarProxima,
}: {
  consulta: CareAppointment;
  profNome: string;
  recurrence: string | null;
  onAgendarProxima: () => void;
}) {
  const dia = formatDateLong(consulta.scheduled_at, consulta.time_zone);
  const hora = formatTime(consulta.scheduled_at, consulta.time_zone);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col items-center gap-3 px-3 pb-2 pt-6 text-center">
        {/* Tinte suave por rgba literal do DS (NÃO `/alpha` sobre token) — mesmo
            padrão já mergeado em consultations/HistorySection.tsx e documentado em
            DESIGN-SYSTEM.md §7 (Badge: "fundos rgba literais do DS"). */}
        <span className="inline-flex h-14 w-14 items-center justify-center rounded-full bg-[rgba(41,176,29,0.12)] text-success">
          <IconCheck size={26} />
        </span>
        {/* h2, não h1: o FlowHeader ("Tudo certo") já é o h1 da tela de sucesso —
            esta é a seção abaixo dele (mesma hierarquia do SucessoCard no desktop,
            que fica sob o h1 "Agendar consulta"). Um h1 por branch de viewport. */}
        <h2 className="text-[22px] font-bold text-primary-300">Consulta marcada</h2>
        <p className="text-sm leading-relaxed text-ink">
          {consulta.label} com {profNome}
          <br />
          <strong className="text-primary-300 first-letter:uppercase">
            {dia} · {hora}
          </strong>{' '}
          · por vídeo
        </p>
      </div>

      <div className="flex flex-col gap-3 rounded-lg border border-primary-100 bg-white p-4">
        <span className="text-[15px] font-bold text-primary-300">
          Quer deixar a próxima marcada?
        </span>
        {recurrence && <p className="text-sm text-ink">{recurrence}</p>}
        <Button color="primary" size="md" fullWidth onClick={onAgendarProxima}>
          Agendar a próxima
        </Button>
        <Link
          to="/jornada"
          className="rounded-sm p-1 text-center text-sm font-bold text-primary-300 transition hover:text-accent-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        >
          Agora não — voltar à jornada
        </Link>
      </div>
    </div>
  );
}

// --- derivações client-side (exibição) -------------------------------------

function estado(n: number, atual: number): PassoEstado {
  if (n < atual) return 'completo';
  if (n === atual) return 'ativo';
  return 'pendente';
}

function captionDia(dias: DiaResumo[], diaKey: string | null): string {
  const d = dias.find((x) => x.key === diaKey);
  return d ? formatDateLong(d.inicio, d.timeZone) : 'Escolha o dia';
}

function rotuloDoItem(
  journey: { enrollments: { items: { item: { id: string; label: string } }[] }[] } | undefined,
  itemId: string | undefined,
): string {
  for (const e of journey?.enrollments ?? []) {
    for (const it of e.items) {
      if (it.item.id === itemId) return it.item.label;
    }
  }
  return 'Consulta';
}

/** A recorrência (texto livre) do item, se a jornada a trouxer — exibida verbatim. */
function recorrenciaDoItem(
  journey: { enrollments: { items: { item: { id: string; recurrence?: string | null } }[] }[] } | undefined,
  itemId: string | undefined,
): string | null {
  for (const e of journey?.enrollments ?? []) {
    for (const it of e.items) {
      if (it.item.id === itemId) return it.item.recurrence ?? null;
    }
  }
  return null;
}

/** Profissionais únicos dos slots, cada um com seu primeiro horário livre. */
function derivarProfissionais(slots: AnnotatedSlot[]): ProfissionalResumo[] {
  const map = new Map<string, ProfissionalResumo>();
  for (const s of slots) {
    const ex = map.get(s.professional.id);
    if (!ex) {
      map.set(s.professional.id, {
        id: s.professional.id,
        full_name: s.professional.full_name,
        primeiroInicio: s.starts_at,
        timeZone: s.time_zone,
      });
    } else if (new Date(s.starts_at) < new Date(ex.primeiroInicio)) {
      map.set(s.professional.id, { ...ex, primeiroInicio: s.starts_at, timeZone: s.time_zone });
    }
  }
  return [...map.values()];
}

/** Dias únicos do profissional escolhido (chave e primeiro horário no fuso do slot). */
function derivarDias(slots: AnnotatedSlot[], profId: string | null): DiaResumo[] {
  if (!profId) return [];
  const map = new Map<string, DiaResumo>();
  for (const s of slots) {
    if (s.professional.id !== profId) continue;
    const k = dayKey(s.starts_at, s.time_zone);
    const ex = map.get(k);
    if (!ex || new Date(s.starts_at) < new Date(ex.inicio)) {
      map.set(k, { key: k, inicio: s.starts_at, timeZone: s.time_zone });
    }
  }
  return [...map.values()];
}

/** Horários do profissional no dia escolhido, em ordem cronológica. */
function derivarSlotsDoDia(
  slots: AnnotatedSlot[],
  profId: string | null,
  diaKey: string | null,
): AnnotatedSlot[] {
  if (!profId || !diaKey) return [];
  return slots
    .filter((s) => s.professional.id === profId && dayKey(s.starts_at, s.time_zone) === diaKey)
    .sort((a, b) => new Date(a.starts_at).getTime() - new Date(b.starts_at).getTime());
}
