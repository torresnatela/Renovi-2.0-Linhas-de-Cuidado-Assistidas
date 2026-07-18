import { Link, useParams } from 'react-router-dom';

import { ApiError, type Appointment } from '../../shared/api';
import { formatDateLong, formatDateTimeShort, formatTime } from '../../shared/datetime';
import { openExternal } from '../../shared/navigate';
import { reasonText } from './reasons';
import { useAppointment, useAppointments, useJoinAppointment } from './useScheduling';
import { Carregando, Erro, Vazio } from './ui';

/** /consultas — a lista. */
export function AppointmentsPage() {
  const { data, isLoading, error } = useAppointments();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <div className="mb-6 flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium">Minhas consultas</h2>
        <Link
          to="/agendar"
          className="rounded bg-emerald-700 px-3 py-2 text-sm font-medium text-white"
        >
          Agendar consulta
        </Link>
      </div>

      {isLoading && <Carregando>Carregando suas consultas…</Carregando>}
      {error && <Erro error={error} />}
      {data?.length === 0 && (
        <Vazio>
          Você ainda não tem consultas.{' '}
          <Link to="/agendar" className="text-emerald-700 underline">
            Agendar a primeira
          </Link>
        </Vazio>
      )}

      <ul className="grid gap-3">
        {data?.map((c) => (
          <li key={c.id}>
            <Link
              to={`/consultas/${c.id}`}
              className="block rounded-lg border border-slate-200 bg-white p-4 hover:border-emerald-600"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <span className="block font-medium">{c.specialty.name}</span>
                  <span className="block text-sm text-slate-600">{c.professional.full_name}</span>
                </div>
                <Selo consulta={c} />
              </div>
              <p className="mt-2 text-sm text-slate-700">
                {formatDateTimeShort(c.starts_at, c.time_zone)}
              </p>
            </Link>
          </li>
        ))}
      </ul>
    </main>
  );
}

/** /consultas/:appointmentId — o detalhe, com o botão de entrar. */
export function AppointmentPage() {
  const { appointmentId } = useParams();
  const { data, isLoading, error } = useAppointment(appointmentId);

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <p className="mb-6 text-sm">
        <Link to="/consultas" className="text-emerald-700 underline">
          ← Minhas consultas
        </Link>
      </p>

      {isLoading && <Carregando>Carregando a consulta…</Carregando>}
      {error && <Erro error={error} />}

      {data && (
        <section className="rounded-lg border border-slate-200 bg-white p-6">
          <div className="mb-4 flex items-start justify-between gap-3">
            <div>
              <h2 className="text-lg font-medium">{data.specialty.name}</h2>
              <p className="text-sm text-slate-600">com {data.professional.full_name}</p>
            </div>
            <Selo consulta={data} />
          </div>

          <p className="mb-6 text-sm text-slate-700 first-letter:uppercase">
            {formatDateLong(data.starts_at, data.time_zone)} às{' '}
            {formatTime(data.starts_at, data.time_zone)}
          </p>

          <PainelDeEntrada consulta={data} />
        </section>
      )}
    </main>
  );
}

/**
 * O botão de entrar — ou o MOTIVO.
 *
 * Regra de ouro do apps/web/CLAUDE.md: nunca um botão só desabilitado. E repare
 * que "30 minutos" não aparece neste arquivo: o que a tela mostra é a HORA que o
 * servidor mandou em `join.opens_at`. Mudar a antecedência é uma variável de
 * ambiente no servidor, sem tocar aqui — e um relógio adiantado no cliente não
 * abre a sala mais cedo, porque quem decide é o `join.status`.
 */
function PainelDeEntrada({ consulta }: { consulta: Appointment }) {
  const entrar = useJoinAppointment(consulta.id);
  const { status, opens_at, reason } = consulta.join;

  if (status !== 'OPEN') {
    const texto =
      status === 'TOO_EARLY'
        ? // Uma HORA é melhor que um cronômetro: dá para o paciente pôr um
          // despertador, e não exige timer, re-render por segundo nem teste com
          // relógio falso.
          `Você poderá entrar a partir das ${formatTime(opens_at, consulta.time_zone)} de ${formatDateLong(opens_at, consulta.time_zone)}.`
        : reasonText(reason, 'Esta consulta não está disponível para acesso.');

    return (
      <p role="status" className="rounded bg-slate-100 p-3 text-sm text-slate-700">
        {texto}
      </p>
    );
  }

  return (
    <>
      <button
        type="button"
        onClick={() => {
          // Guarda contra o duplo-clique no mesmo tick, antes de isPending virar:
          // cada clique é um POST que registra acesso no servidor, e dois abririam
          // a sala duas vezes.
          if (entrar.isPending) return;
          entrar.mutate(undefined, { onSuccess: (t) => openExternal(t.url) });
        }}
        disabled={entrar.isPending}
        className="rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
      >
        {entrar.isPending ? 'Abrindo sua sala…' : 'Entrar na consulta'}
      </button>

      {/*
        O 409 aqui é a rede de proteção da corrida (relógio adiantado, cache
        velho): o botão só apareceu porque o `join.status` dizia OPEN, mas quem
        manda é o servidor no momento do clique.
      */}
      {entrar.isError && (
        <p role="alert" className="mt-3 rounded bg-red-50 p-3 text-sm text-red-700">
          {reasonText(
            entrar.error instanceof ApiError ? entrar.error.reason : undefined,
            entrar.error.message,
          )}
        </p>
      )}
    </>
  );
}

/**
 * O selo de status.
 *
 * UNCONFIRMED tem texto próprio e é o mais importante: a consulta PODE existir na
 * Doutor ao Vivo e nunca vamos descobrir sozinhos (ADR-016). Escondê-la seria
 * pior que a incerteza — o paciente pode ter uma consulta de verdade marcada.
 */
function Selo({ consulta }: { consulta: Appointment }) {
  const estilos: Record<Appointment['status'], string> = {
    CONFIRMED: 'bg-emerald-100 text-emerald-800',
    PROCESSING: 'bg-slate-100 text-slate-700',
    UNCONFIRMED: 'bg-amber-100 text-amber-900',
    CANCELLED: 'bg-slate-100 text-slate-500',
  };
  const textos: Record<Appointment['status'], string> = {
    CONFIRMED: 'Confirmada',
    PROCESSING: 'Confirmando…',
    UNCONFIRMED: 'Verificando',
    CANCELLED: 'Cancelada',
  };

  return (
    <span
      className={`shrink-0 rounded-full px-2 py-1 text-xs font-medium ${estilos[consulta.status]}`}
      title={
        consulta.status === 'UNCONFIRMED'
          ? 'A Doutor ao Vivo não confirmou a tempo. Nossa equipe está verificando se a consulta foi marcada.'
          : undefined
      }
    >
      {textos[consulta.status]}
    </span>
  );
}
