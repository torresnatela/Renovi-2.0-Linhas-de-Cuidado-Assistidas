import { Link } from 'react-router-dom';

import { ApiError, type CareAppointment } from '../../shared/api';
import { formatDateTimeShort } from '../../shared/datetime';
import { reasonText } from '../scheduling/reasons';
import { Carregando, Erro, Vazio } from '../scheduling/ui';
import { useCancelCare, useCareAppointments } from './useJourney';

const STATUS_CONSULTA: Record<CareAppointment['status'], { rotulo: string; classe: string }> = {
  agendada: { rotulo: 'Agendada', classe: 'bg-slate-100 text-slate-700' },
  confirmada: { rotulo: 'Confirmada', classe: 'bg-emerald-100 text-emerald-800' },
  em_andamento: { rotulo: 'Em andamento', classe: 'bg-blue-100 text-blue-800' },
  realizada: { rotulo: 'Realizada', classe: 'bg-emerald-50 text-emerald-700' },
  falta: { rotulo: 'Falta', classe: 'bg-red-100 text-red-800' },
  cancelada: { rotulo: 'Cancelada', classe: 'bg-slate-100 text-slate-500' },
};

// Só destes dois estados dá para cancelar; realizada/falta/cancelada não (o
// servidor confirma com 409, mas o botão nem aparece para não prometer o que não há).
const CANCELAVEIS: ReadonlySet<CareAppointment['status']> = new Set(['agendada', 'confirmada']);

type Cancelamento = ReturnType<typeof useCancelCare>;

/** /jornada/consultas — as consultas de linha de cuidado do paciente. */
export function CareAppointmentsPage() {
  const { data, isLoading, error } = useCareAppointments();
  const cancelar = useCancelCare();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <p className="mb-6 text-sm">
        <Link to="/jornada" className="text-emerald-700 underline">
          ← Minha jornada
        </Link>
      </p>
      <h2 className="mb-6 text-lg font-medium">Minhas consultas</h2>

      {isLoading && <Carregando>Carregando suas consultas…</Carregando>}
      {error && <Erro error={error} />}

      {cancelar.isError && (
        <p role="alert" className="mb-4 rounded bg-red-50 p-3 text-sm text-red-700">
          {reasonText(
            cancelar.error instanceof ApiError ? cancelar.error.reason : undefined,
            cancelar.error.message,
          )}
        </p>
      )}

      {data?.length === 0 && <Vazio>Você ainda não tem consultas de linha de cuidado.</Vazio>}

      <ul className="grid gap-3">
        {data?.map((c) => (
          <li key={c.id}>
            <ConsultaCard consulta={c} cancelar={cancelar} />
          </li>
        ))}
      </ul>
    </main>
  );
}

function ConsultaCard({
  consulta,
  cancelar,
}: {
  consulta: CareAppointment;
  cancelar: Cancelamento;
}) {
  const status = STATUS_CONSULTA[consulta.status];
  const podeCancelar = CANCELAVEIS.has(consulta.status);
  const cancelando = cancelar.isPending && cancelar.variables === consulta.id;

  function pedirCancelamento() {
    if (cancelar.isPending) return;
    // confirm() cru: é uma tela de teste, e o clique é destrutivo (fala com a DAV).
    if (!window.confirm(`Cancelar a consulta "${consulta.label}"?`)) return;
    cancelar.mutate(consulta.id);
  }

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <span className="block font-medium">{consulta.label}</span>
          <span className="block text-sm text-slate-600">
            {formatDateTimeShort(consulta.scheduled_at, consulta.time_zone)}
          </span>
          {consulta.cancelled_at && (
            <span className="mt-1 block text-xs text-slate-500">
              Cancelada em {formatDateTimeShort(consulta.cancelled_at, consulta.time_zone)}
            </span>
          )}
        </div>
        <span
          className={`shrink-0 rounded-full px-2 py-1 text-xs font-medium ${status.classe}`}
        >
          {status.rotulo}
        </span>
      </div>

      {podeCancelar && (
        <div className="mt-3">
          <button
            type="button"
            onClick={pedirCancelamento}
            disabled={cancelando}
            className="rounded border px-3 py-1 text-sm disabled:opacity-60"
          >
            {cancelando ? 'Cancelando…' : 'Cancelar'}
          </button>
        </div>
      )}
    </div>
  );
}
