import { Link } from 'react-router-dom';

import type {
  Enrollment,
  JourneyEnrollment,
  JourneyEvent,
  JourneyItem,
} from '../../shared/api';
import { formatDateLong, formatDateTimeShort } from '../../shared/datetime';
import { Carregando, Erro, Vazio } from '../scheduling/ui';
import { Blocos } from './ui';
import { FUSO_PADRAO, useJourney } from './useJourney';

const STATUS_MATRICULA: Record<Enrollment['status'], string> = {
  ativa: 'Ativa',
  pausada: 'Pausada',
  concluida: 'Concluída',
  encerrada: 'Encerrada',
  expirada: 'Expirada',
};

const TIPO_EVENTO: Record<JourneyEvent['event_type'], string> = {
  matricula_criada: 'Matrícula criada',
  matricula_renovada: 'Matrícula renovada',
  matricula_expirada: 'Matrícula expirada',
  matricula_encerrada: 'Matrícula encerrada',
  consulta_agendada: 'Consulta agendada',
  consulta_cancelada: 'Consulta cancelada',
  consulta_status_forcado: 'Status da consulta atualizado',
};

/**
 * A tela-mãe do paciente (SPEC §7): cada matrícula com seus itens JÁ avaliados
 * pelo motor. O front NÃO recalcula elegibilidade — ele exibe o veredito (`allowed`
 * e os `blocks`) que a API mandou pronto.
 */
export function JourneyPage() {
  const { data, isLoading, error } = useJourney();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <div className="mb-6 flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium">Minha jornada</h2>
        <Link to="/jornada/consultas" className="rounded border px-3 py-2 text-sm">
          Minhas consultas
        </Link>
      </div>

      {isLoading && <Carregando>Carregando sua jornada…</Carregando>}
      {error && <Erro error={error} />}
      {data?.enrollments.length === 0 && (
        <Vazio>Você ainda não está em nenhuma linha de cuidado.</Vazio>
      )}

      <div className="grid gap-6">
        {data?.enrollments.map((je) => <Matricula key={je.enrollment.id} matricula={je} />)}
      </div>
    </main>
  );
}

function Matricula({ matricula }: { matricula: JourneyEnrollment }) {
  const { enrollment, care_line_name, items, recent_events } = matricula;

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-6">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <h3 className="text-base font-medium">{care_line_name}</h3>
          <p className="text-sm text-slate-600 first-letter:uppercase">
            Vigência: {formatDateLong(enrollment.valid_from, FUSO_PADRAO)} até{' '}
            {formatDateLong(enrollment.valid_until, FUSO_PADRAO)}
          </p>
        </div>
        <span className="shrink-0 rounded-full bg-slate-100 px-2 py-1 text-xs font-medium text-slate-700">
          {STATUS_MATRICULA[enrollment.status]}
        </span>
      </div>

      <ul className="grid gap-3">
        {items.map((it) => (
          <li key={it.item.id}>
            <ItemDaLinha item={it} />
          </li>
        ))}
      </ul>

      {recent_events.length > 0 && <EventosRecentes eventos={recent_events} />}
    </section>
  );
}

function ItemDaLinha({ item }: { item: JourneyItem }) {
  const { eligibility } = item;

  return (
    <div className="rounded border border-slate-200 p-3">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">{item.item.label}</span>
        {eligibility.allowed ? (
          <div className="flex shrink-0 items-center gap-3">
            <span className="rounded-full bg-emerald-100 px-2 py-1 text-xs font-medium text-emerald-800">
              Disponível
            </span>
            <Link
              to={`/jornada/agendar/${item.item.id}`}
              className="rounded bg-emerald-700 px-3 py-1 text-sm font-medium text-white"
            >
              Agendar
            </Link>
          </div>
        ) : (
          <span className="shrink-0 rounded-full bg-amber-100 px-2 py-1 text-xs font-medium text-amber-900">
            Indisponível
          </span>
        )}
      </div>

      {item.item.recurrence && (
        <p className="mt-1 text-xs text-slate-500">{item.item.recurrence}</p>
      )}

      {/* Regra de ouro: nunca só "indisponível" — o porquê, pronto do servidor. */}
      {!eligibility.allowed && eligibility.blocks.length > 0 && (
        <div className="mt-2">
          <Blocos blocks={eligibility.blocks} />
        </div>
      )}
    </div>
  );
}

function EventosRecentes({ eventos }: { eventos: JourneyEvent[] }) {
  return (
    <div className="mt-4 border-t border-slate-100 pt-4">
      <h4 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">
        Atividade recente
      </h4>
      <ul className="grid gap-1">
        {eventos.map((ev) => (
          <li key={ev.id} className="text-sm text-slate-600">
            {TIPO_EVENTO[ev.event_type]} — {formatDateTimeShort(ev.occurred_at, FUSO_PADRAO)}
          </li>
        ))}
      </ul>
    </div>
  );
}
