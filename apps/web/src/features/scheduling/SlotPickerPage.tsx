import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import { ApiError, type Slot } from '../../shared/api';
import { dayKey, formatDateLong, formatTime } from '../../shared/datetime';
import { reasonText } from './reasons';
import { useCreateAppointment, useSlots } from './useScheduling';
import { Carregando, Erro, Passos, Vazio } from './ui';

/** Passo 3 — escolher o horário e confirmar. */
export function SlotPickerPage() {
  const { specialtyId, professionalId } = useParams();
  const { data, isLoading, error } = useSlots(professionalId);

  // Guardamos o ID, não o Slot inteiro, e derivamos o slot da lista atual. Assim,
  // quando a lista faz refetch (staleTime de 30s) e o horário escolhido some
  // porque outra pessoa o pegou, o painel de confirmação some junto — em vez de
  // seguir oferecendo um fantasma que só falharia no clique (409).
  const [escolhidoId, setEscolhidoId] = useState<string | null>(null);
  const escolhido = data?.items.find((s) => s.id === escolhidoId) ?? null;

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <Passos atual={3} />
      <h2 className="mb-1 text-lg font-medium">
        {data ? `Horários de ${data.professional.full_name}` : 'Horários'}
      </h2>
      <p className="mb-6 text-sm text-slate-600">
        <Link to={`/agendar/${specialtyId}`} className="text-emerald-700 underline">
          Trocar de profissional
        </Link>
      </p>

      {isLoading && <Carregando>Carregando horários…</Carregando>}
      {error && <Erro error={error} />}
      {data?.items.length === 0 && (
        <Vazio>
          Este profissional não tem horário livre nos próximos 30 dias.{' '}
          <Link to={`/agendar/${specialtyId}`} className="text-emerald-700 underline">
            Ver outros profissionais
          </Link>
        </Vazio>
      )}

      {data && data.items.length > 0 && (
        <GradeDeHorarios
          slots={data.items}
          escolhido={escolhido}
          onEscolher={(s) => setEscolhidoId(s.id)}
        />
      )}

      {escolhido && data && specialtyId && (
        <PainelDeConfirmacao
          slot={escolhido}
          profissional={data.professional.full_name}
          specialtyId={specialtyId}
          onCancelar={() => setEscolhidoId(null)}
        />
      )}
    </main>
  );
}

/**
 * Agrupa por dia usando o fuso DO SLOT, não o do browser: agrupar pelo dia local
 * jogaria o horário das 23:00 de segunda no balde de terça para quem estiver a
 * leste.
 */
function GradeDeHorarios({
  slots,
  escolhido,
  onEscolher,
}: {
  slots: Slot[];
  escolhido: Slot | null;
  onEscolher: (s: Slot) => void;
}) {
  const dias = new Map<string, Slot[]>();
  for (const s of slots) {
    const chave = dayKey(s.starts_at, s.time_zone);
    dias.set(chave, [...(dias.get(chave) ?? []), s]);
  }

  return (
    <div className="grid gap-6">
      {[...dias.entries()].map(([chave, doDia]) => (
        <section key={chave}>
          {/*
            first-letter:uppercase, e NÃO o `capitalize` do Tailwind: aquele é
            text-transform: capitalize, que deixa toda palavra maiúscula —
            "Sexta-Feira, 17 De Julho". Em português só a primeira letra sobe.
          */}
          <h3 className="mb-2 text-sm font-medium text-slate-700 first-letter:uppercase">
            {formatDateLong(doDia[0].starts_at, doDia[0].time_zone)}
          </h3>
          <ul className="flex flex-wrap gap-2">
            {doDia.map((s) => {
              const ativo = escolhido?.id === s.id;
              return (
                <li key={s.id}>
                  <button
                    type="button"
                    aria-pressed={ativo}
                    onClick={() => onEscolher(s)}
                    className={
                      ativo
                        ? 'rounded border border-emerald-700 bg-emerald-700 px-3 py-2 text-sm text-white'
                        : 'rounded border border-slate-300 bg-white px-3 py-2 text-sm hover:border-emerald-600'
                    }
                  >
                    {formatTime(s.starts_at, s.time_zone)}
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
    </div>
  );
}

function PainelDeConfirmacao({
  slot,
  profissional,
  specialtyId,
  onCancelar,
}: {
  slot: Slot;
  profissional: string;
  specialtyId: string;
  onCancelar: () => void;
}) {
  const navigate = useNavigate();
  const agendar = useCreateAppointment();

  const confirmar = () =>
    agendar.mutate(
      { slot_id: slot.id, specialty_id: specialtyId },
      { onSuccess: (appt) => navigate(`/consultas/${appt.id}`) },
    );

  // 502/504: o resultado é DESCONHECIDO, nunca "falhou" (ADR-016). A consulta
  // pode existir de verdade, e repetir criaria uma SEGUNDA — a DAV não aceita id
  // nosso, então não há como reconciliar. Por isso este caso tem tela própria e
  // NÃO oferece "tentar de novo".
  const resultadoDesconhecido =
    agendar.error instanceof ApiError &&
    (agendar.error.status === 502 || agendar.error.status === 504);

  return (
    <section className="mt-8 rounded-lg border border-slate-200 bg-white p-6">
      <h3 className="mb-2 text-base font-medium">Confirmar consulta</h3>
      <p className="mb-4 text-sm text-slate-700">
        {profissional} — <strong>{formatDateLong(slot.starts_at, slot.time_zone)}</strong> às{' '}
        <strong>{formatTime(slot.starts_at, slot.time_zone)}</strong>
      </p>

      {/*
        O agendamento é síncrono contra a Doutor ao Vivo, que mediu de 3s a 29s.
        Sem esta frase o usuário acha que travou e recarrega — e recarregar aqui é
        péssimo: ele fica sem saber se marcou.
      */}
      {agendar.isPending && (
        <p role="status" className="mb-4 rounded bg-emerald-50 p-3 text-sm text-emerald-900">
          Estamos reservando seu horário na Doutor ao Vivo. Isso pode levar até um minuto —
          <strong> não feche nem recarregue esta página.</strong>
        </p>
      )}

      {resultadoDesconhecido && (
        <div role="alert" className="mb-4 rounded bg-amber-50 p-3 text-sm text-amber-900">
          <p className="mb-2">
            Reservamos seu horário, mas a Doutor ao Vivo não confirmou a tempo. Sua consulta{' '}
            <strong>pode ter sido marcada</strong> — nossa equipe está verificando.
          </p>
          <Link to="/consultas" className="font-medium text-emerald-700 underline">
            Ver minhas consultas
          </Link>
        </div>
      )}

      {agendar.isError && !resultadoDesconhecido && (
        <p role="alert" className="mb-4 rounded bg-red-50 p-3 text-sm text-red-700">
          {reasonText(
            agendar.error instanceof ApiError ? agendar.error.reason : undefined,
            agendar.error.message,
          )}
        </p>
      )}

      {!resultadoDesconhecido && (
        <div className="flex flex-wrap gap-3">
          <button
            type="button"
            onClick={confirmar}
            disabled={agendar.isPending}
            className="rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
          >
            {agendar.isPending ? 'Reservando seu horário…' : 'Confirmar consulta'}
          </button>
          <button
            type="button"
            onClick={onCancelar}
            disabled={agendar.isPending}
            className="rounded border px-4 py-2 text-sm disabled:opacity-60"
          >
            Escolher outro horário
          </button>
        </div>
      )}
    </section>
  );
}
