import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { ApiError, type AnnotatedSlot } from '../../shared/api';
import { dayKey, formatDateLong, formatDateTimeShort, formatTime } from '../../shared/datetime';
import { reasonText } from '../scheduling/reasons';
import { Carregando, Erro, Vazio } from '../scheduling/ui';
import { Blocos } from './ui';
import { useAvailability, useScheduleCare } from './useJourney';

/** A intenção viva: qual horário o paciente escolheu e a key que a identifica. */
type Intencao = { slotId: string; key: string };

type Agendamento = ReturnType<typeof useScheduleCare>;

/**
 * Agenda um item da linha. Diferente do wizard de booking, aqui a tela NÃO navega
 * para fora no sucesso: agrega os horários de vários profissionais e deixa o
 * paciente marcar vários de uma vez ("o mês todo"), refazendo a lista a cada
 * agendamento (o horário tomado some).
 */
export function ScheduleCarePage() {
  const { itemId } = useParams();
  const { data, isLoading, error } = useAvailability(itemId);
  const agendar = useScheduleCare();

  // A Idempotency-Key nasce com a INTENÇÃO (o clique no horário), não com a
  // chamada: um retry da MESMA intenção reusa a MESMA key e o servidor devolve a
  // mesma consulta sem duplicar. Escolher OUTRO horário é outra intenção → key nova.
  const [intencao, setIntencao] = useState<Intencao | null>(null);

  function marcar(slot: AnnotatedSlot) {
    if (!itemId) return;
    const mesmaIntencao = intencao?.slotId === slot.id;
    const key = mesmaIntencao ? intencao!.key : crypto.randomUUID();
    if (!mesmaIntencao) setIntencao({ slotId: slot.id, key });
    agendar.mutate({ body: { item_id: itemId, slot_id: slot.id }, idemKey: key });
  }

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <p className="mb-6 text-sm">
        <Link to="/jornada" className="text-emerald-700 underline">
          ← Minha jornada
        </Link>
      </p>
      <h2 className="mb-1 text-lg font-medium">Escolha um horário</h2>
      <p className="mb-6 text-sm text-slate-600">
        Os horários vêm de todos os profissionais da especialidade deste item. Marque quantos
        precisar — a lista se atualiza a cada agendamento.
      </p>

      {isLoading && <Carregando>Carregando horários…</Carregando>}
      {error && <Erro error={error} />}
      {data?.items.length === 0 && (
        <Vazio>Não há horários agendáveis para este item nos próximos dias.</Vazio>
      )}

      <ResultadoAgendamento agendar={agendar} />

      {data && data.items.length > 0 && (
        <GradeDeHorarios slots={data.items} onMarcar={marcar} agendar={agendar} />
      )}
    </main>
  );
}

/** O que aconteceu com a última tentativa: confirmação, bloqueio (422) ou erro. */
function ResultadoAgendamento({ agendar }: { agendar: Agendamento }) {
  if (agendar.isPending) {
    return (
      <p role="status" className="mb-4 rounded bg-emerald-50 p-3 text-sm text-emerald-900">
        Estamos reservando seu horário na Doutor ao Vivo. Isso pode levar até um minuto —
        <strong> não feche nem recarregue esta página.</strong>
      </p>
    );
  }

  if (agendar.isSuccess && agendar.data) {
    const c = agendar.data;
    return (
      <p role="status" className="mb-4 rounded bg-emerald-50 p-3 text-sm text-emerald-900">
        Consulta agendada: <strong>{c.label}</strong> —{' '}
        {formatDateTimeShort(c.scheduled_at, c.time_zone)}. Você pode marcar o próximo horário
        abaixo.
      </p>
    );
  }

  if (agendar.isError) {
    const err = agendar.error;
    // 422: o horário existe e está livre — quem não pode é ESTE paciente agendar
    // ESTE item agora. Mostramos a lista inteira de motivos que o motor mandou.
    if (err instanceof ApiError && err.status === 422 && err.blocks && err.blocks.length > 0) {
      return (
        <div role="alert" className="mb-4 rounded bg-amber-50 p-3 text-sm text-amber-900">
          <p className="mb-2 font-medium">Não foi possível agendar este horário:</p>
          <Blocos blocks={err.blocks} />
        </div>
      );
    }
    return (
      <p role="alert" className="mb-4 rounded bg-red-50 p-3 text-sm text-red-700">
        {reasonText(err instanceof ApiError ? err.reason : undefined, err.message)}
      </p>
    );
  }

  return null;
}

/**
 * Agrupa por dia usando o fuso DO SLOT (não o do browser): agrupar pelo dia local
 * jogaria o horário das 23:00 de segunda no balde de terça para quem está a leste.
 */
function GradeDeHorarios({
  slots,
  onMarcar,
  agendar,
}: {
  slots: AnnotatedSlot[];
  onMarcar: (s: AnnotatedSlot) => void;
  agendar: Agendamento;
}) {
  const dias = new Map<string, AnnotatedSlot[]>();
  for (const s of slots) {
    const chave = dayKey(s.starts_at, s.time_zone);
    dias.set(chave, [...(dias.get(chave) ?? []), s]);
  }

  return (
    <div className="grid gap-6">
      {[...dias.entries()].map(([chave, doDia]) => (
        <section key={chave}>
          <h3 className="mb-2 text-sm font-medium text-slate-700 first-letter:uppercase">
            {formatDateLong(doDia[0].starts_at, doDia[0].time_zone)}
          </h3>
          <ul className="grid gap-2">
            {doDia.map((s) => (
              <li key={s.id}>
                <Horario slot={s} onMarcar={onMarcar} agendar={agendar} />
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function Horario({
  slot,
  onMarcar,
  agendar,
}: {
  slot: AnnotatedSlot;
  onMarcar: (s: AnnotatedSlot) => void;
  agendar: Agendamento;
}) {
  const hora = formatTime(slot.starts_at, slot.time_zone);
  const permitido = slot.eligibility.allowed;
  const estePendente = agendar.isPending && agendar.variables?.body.slot_id === slot.id;

  // Enquanto QUALQUER agendamento está em voo, todos os botões travam: o hook é
  // uma única useMutation e a chamada à DAV leva ~29s. Clicar outro horário no
  // meio dispararia um segundo POST concorrente — o observer só rastrearia o
  // segundo (o banner do primeiro sumiria) e ainda alargaria a corrida de cota do
  // servidor. O paciente espera este confirmar e marca o próximo.
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <span className="block font-medium">{hora}</span>
          <span className="block text-sm text-slate-600">{slot.professional.full_name}</span>
        </div>
        <button
          type="button"
          aria-label={`Agendar horário das ${hora}`}
          onClick={() => onMarcar(slot)}
          disabled={agendar.isPending || !permitido}
          className="shrink-0 rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
        >
          {estePendente ? 'Agendando…' : 'Agendar'}
        </button>
      </div>

      {/*
        Regra de ouro: nunca um botão só desabilitado — quando o motor barra este
        horário, o paciente lê o porquê aqui embaixo (o `reason` pronto do servidor).
      */}
      {!permitido && slot.eligibility.blocks.length > 0 && (
        <div className="mt-2">
          <Blocos blocks={slot.eligibility.blocks} />
        </div>
      )}
    </div>
  );
}
