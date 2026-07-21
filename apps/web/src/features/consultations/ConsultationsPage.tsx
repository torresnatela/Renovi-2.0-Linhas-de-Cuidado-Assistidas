import { useState } from 'react';

import { ApiError, type CareAppointment, type CareAppointmentStatus } from '../../shared/api';
import { Empty, ErrorNotice, Loading } from '../../shared/ui/feedback';
import { SegmentedControl } from '../../shared/ui/SegmentedControl';
import { useCancelCare, useCareAppointments, useJourney } from '../journey/useJourney';
import { reasonText } from '../scheduling/reasons';
import { AppointmentCard } from './AppointmentCard';
import { HistorySection } from './HistorySection';
import { LinkButton, SectionLabel } from './parts';
import { ToScheduleAside } from './ToScheduleAside';
import { useBookingProfessionals } from './useBookingProfessionals';

type Tab = 'proximas' | 'historico';

// Próximas = o que ainda vai acontecer; Histórico = o que já é passado. O split é
// por STATUS (a mesma fonte de verdade das duas abas), não por relógio — evita
// esconder uma consulta `em_andamento` só porque o horário marcado já passou.
const ATIVAS: ReadonlySet<CareAppointmentStatus> = new Set([
  'agendada',
  'confirmada',
  'em_andamento',
]);
const HISTORICO: ReadonlySet<CareAppointmentStatus> = new Set(['realizada', 'falta', 'cancelada']);

const porDataAsc = (a: CareAppointment, b: CareAppointment) =>
  new Date(a.scheduled_at).getTime() - new Date(b.scheduled_at).getTime();
const porDataDesc = (a: CareAppointment, b: CareAppointment) =>
  new Date(b.scheduled_at).getTime() - new Date(a.scheduled_at).getTime();

/**
 * /consultas — as consultas da linha de cuidado do paciente, em duas abas.
 *
 * `now` chega por prop (default `new Date()`) só para o selo "Hoje": é um seam de
 * teste, para cravar o dia sem depender do relógio do runner (que roda em UTC).
 * Ambas as abas leem da MESMA query (`useCareAppointments`) e filtram no cliente.
 */
export function ConsultationsPage({ now = new Date() }: { now?: Date } = {}) {
  const { data: consultas, isLoading, error } = useCareAppointments();
  const { data: journey } = useJourney();
  const cancelar = useCancelCare();
  const [tab, setTab] = useState<Tab>('proximas');

  const proximas = (consultas ?? []).filter((c) => ATIVAS.has(c.status)).sort(porDataAsc);
  const historico = (consultas ?? []).filter((c) => HISTORICO.has(c.status)).sort(porDataDesc);

  // Só os bookings da aba VISÍVEL: não dispara o histórico inteiro contra a DAV
  // enquanto o paciente está em Próximas (e vice-versa).
  const idsVisiveis = (tab === 'proximas' ? proximas : historico).map((c) => c.booking_id);
  const profissionais = useBookingProfessionals(idsVisiveis);

  function pedirCancelamento(consulta: CareAppointment) {
    if (cancelar.isPending) return;
    // confirm() cru: o clique é destrutivo (fala com a DAV) e a rede contra o
    // clique acidental basta — não é uma tela que precise de modal próprio.
    if (!window.confirm(`Cancelar a consulta "${consulta.label}"?`)) return;
    cancelar.mutate(consulta.id);
  }

  return (
    <div className="flex flex-col gap-7">
      <div className="flex flex-wrap items-end justify-between gap-8">
        <div className="flex flex-col gap-1">
          <SectionLabel>Suas consultas</SectionLabel>
          <h1 className="text-[38px] font-bold leading-[44px] text-primary-300">Consultas</h1>
        </div>
        <div className="w-[300px] max-w-full">
          <SegmentedControl
            options={[
              { value: 'proximas', label: 'Próximas' },
              { value: 'historico', label: 'Histórico' },
            ]}
            value={tab}
            onChange={(v) => setTab(v as Tab)}
          />
        </div>
      </div>

      {isLoading && <Loading label="Carregando suas consultas…" />}
      {error && <ErrorNotice error={error} />}

      {cancelar.isError && (
        <p role="alert" className="rounded-md bg-[rgba(205,25,25,0.08)] p-4 text-sm text-error">
          {reasonText(
            cancelar.error instanceof ApiError ? cancelar.error.reason : undefined,
            cancelar.error.message,
          )}
        </p>
      )}

      {!isLoading && tab === 'proximas' && (
        <div className="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_388px] lg:items-start">
          <section className="flex min-w-0 flex-col gap-3.5">
            <SectionLabel>Agendadas</SectionLabel>
            {proximas.length === 0 ? (
              <div className="flex flex-col items-start gap-4">
                <Empty
                  title="Você não tem consultas agendadas."
                  hint="Agende pela sua jornada de cuidado."
                />
                <LinkButton to="/jornada" fullWidth={false}>
                  Agendar
                </LinkButton>
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                {proximas.map((c) => (
                  <AppointmentCard
                    key={c.id}
                    consulta={c}
                    now={now}
                    onCancel={pedirCancelamento}
                    cancelando={cancelar.isPending && cancelar.variables === c.id}
                    nomeProfissional={profissionais[c.booking_id]}
                  />
                ))}
              </div>
            )}
          </section>

          <ToScheduleAside journey={journey} />
        </div>
      )}

      {!isLoading && tab === 'historico' && (
        <HistorySection consultas={historico} profissionais={profissionais} />
      )}
    </div>
  );
}
