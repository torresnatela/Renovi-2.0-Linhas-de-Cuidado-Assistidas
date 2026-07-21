import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import type { CareAppointment, JourneyEvent } from '../../shared/api';
import { formatDateTimeShort, FUSO_PADRAO } from '../../shared/datetime';
import { Card } from '../../shared/ui/Card';
import { Empty, ErrorNotice, Loading } from '../../shared/ui/feedback';
import { IconCalendar, IconCaretRight } from '../../shared/ui/icons';
import { ListRow } from '../../shared/ui/ListRow';
import { PlanValidityBanner } from '../../shared/ui/PlanValidityBanner';
import { useSession } from '../auth/useSession';
import { MoodCheckinCard } from '../mood/MoodCheckinCard';
import { proximaConsulta, resumoDoDia, vigenciaPertoDoFim } from './derivations';
import { JourneyHero, SectionLabel } from './JourneyHero';
import { JourneyTimeline } from './JourneyTimeline';
import { MostImportantNow } from './MostImportantNow';
import { useCareAppointments, useJourney } from './useJourney';
import { useMoodToday } from '../mood/useMood';

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
 * A tela-mãe do paciente (SPEC §7) e nova home do app: a linha de cuidado como uma
 * jornada. O front NÃO recalcula elegibilidade — exibe o veredito pronto do motor.
 * Renderiza DENTRO do AppShell (que já tem o <main> e o header), então a raiz é uma
 * div de conteúdo, nunca um <main> próprio.
 */
export function JourneyPage() {
  const journey = useJourney();
  const appointmentsQuery = useCareAppointments();
  const session = useSession();
  const mood = useMoodToday();
  // Guarda o enrollment.id selecionado nos chips — NUNCA o care_line_code (ver
  // filtro abaixo: duas matrículas podem compartilhar o mesmo code).
  const [matriculaAtivaId, setMatriculaAtivaId] = useState<string | null>(null);

  if (journey.isLoading) return <Loading label="Carregando sua jornada…" />;
  if (journey.isError) return <ErrorNotice error={journey.error} retry={() => journey.refetch()} />;

  // `GET /me/journey` devolve TODO o histórico de matrículas (inclusive
  // encerrada/expirada/concluída), e uma renovação de linha gera uma NOVA
  // matrícula com o MESMO care_line_code da anterior. A Jornada é "o presente":
  // só mostra chips e conteúdo das matrículas ATIVAS — histórico não é papel
  // desta tela. Sem este filtro, linhas encerradas apareciam como chips
  // duplicados (mesmo nome) e o find por care_line_code podia escolher a versão
  // errada.
  const todasMatriculas = journey.data?.enrollments ?? [];
  const enrollments = todasMatriculas.filter((e) => e.enrollment.status === 'ativa');
  if (enrollments.length === 0) {
    return (
      <Empty
        title="Você ainda não está em nenhuma linha de cuidado."
        hint="Quando a equipe ativar a sua, ela aparece aqui com os próximos passos."
      />
    );
  }

  // A linha ativa é a selecionada nos chips (por enrollment.id, que é único);
  // sem seleção, a primeira matrícula ativa.
  const active =
    enrollments.find((e) => e.enrollment.id === matriculaAtivaId) ?? enrollments[0];
  const appointments = appointmentsQuery.data ?? [];
  // `code` aqui é só a chave opaca que o LineChips usa para destacar/selecionar
  // (não o care_line_code de domínio, que pode se repetir entre versões).
  const lines = enrollments.map((e) => ({
    code: e.enrollment.id,
    name: e.care_line_name,
  }));

  // Resumo do dia derivado do que já carregamos (sem chamadas novas).
  const checkinPendente = mood.data
    ? Boolean(mood.data.can_checkin && !mood.data.checkin)
    : false;
  const temItemLiberado = active.items.some((it) => it.eligibility.allowed);
  const resumo = resumoDoDia(temItemLiberado, checkinPendente);

  return (
    <div>
      <JourneyHero
        fullName={session.data?.full_name}
        resumo={resumo}
        lines={lines}
        activeCode={active.enrollment.id}
        onSelect={setMatriculaAtivaId}
      />

      <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,1fr)_388px]">
        {/* Coluna principal */}
        <div className="flex min-w-0 flex-col gap-8">
          <MostImportantNow enrollment={active} appointments={appointments} />
          <JourneyTimeline enrollment={active} appointments={appointments} />
          {active.recent_events.length > 0 && <EventosRecentes eventos={active.recent_events} />}
        </div>

        {/* Aside sticky sob o header de 70px (top 102 = 70 + respiro). */}
        <aside className="flex flex-col gap-5 self-start lg:sticky lg:top-[102px]">
          <MoodCheckinCard />
          {vigenciaPertoDoFim(active.enrollment.valid_until) && (
            <PlanValidityBanner
              enrollment={active.enrollment}
              careLineName={active.care_line_name}
              nearExpiry
            />
          )}
          <ProximaConsulta appointments={appointments} />
        </aside>
      </div>
    </div>
  );
}

function ProximaConsulta({ appointments }: { appointments: CareAppointment[] }) {
  const navigate = useNavigate();
  const proxima = proximaConsulta(appointments);
  // Sem consulta futura, a seção some (nada de espaço vazio).
  if (!proxima) return null;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-baseline justify-between">
        <SectionLabel>Próxima consulta</SectionLabel>
        <Link
          to="/consultas"
          className="rounded-sm text-[13px] font-bold text-primary-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        >
          Ver todas
        </Link>
      </div>
      <Card className="!p-0">
        <ListRow
          icon={<IconCalendar size={22} />}
          title={proxima.label}
          caption={formatDateTimeShort(proxima.scheduled_at, proxima.time_zone)}
          right={<IconCaretRight size={16} />}
          onClick={() => navigate(`/consultas/${proxima.booking_id}`)}
        />
      </Card>
      <span className="text-xs text-muted">Cancelamento gratuito até 24h antes da consulta.</span>
    </div>
  );
}

// Uma seção discreta no fim da coluna principal — a jornada é um event log, e ver a
// atividade recente ajuda o paciente a se situar sem virar timeline social.
function EventosRecentes({ eventos }: { eventos: JourneyEvent[] }) {
  return (
    <section className="flex flex-col gap-2.5">
      <SectionLabel>Atividade recente</SectionLabel>
      <ul className="flex flex-col gap-1">
        {eventos.map((ev) => (
          <li key={ev.id} className="text-[13px] text-muted">
            {TIPO_EVENTO[ev.event_type]} · {formatDateTimeShort(ev.occurred_at, FUSO_PADRAO)}
          </li>
        ))}
      </ul>
    </section>
  );
}
