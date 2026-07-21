import type { ReactNode } from 'react';

import type { CareAppointment, CareLineItemInfo, JourneyEnrollment } from '../../shared/api';
import { formatDateTimeShort } from '../../shared/datetime';
import { IconCheck } from '../../shared/ui/icons';
import { CtaLink } from './CtaLink';
import { proximaConsulta } from './derivations';
import { SectionLabel } from './JourneyHero';

/**
 * "O mais importante agora": UM cartão em destaque com a próxima ação. A prioridade
 * é do produto, não do motor:
 *  (a) há consulta futura marcada → lembrar dela (e o caminho para entrar);
 *  (b) senão, há item liberado → convidar a agendar;
 *  (c) senão → "tudo em dia" (sem cobrança).
 */
export function MostImportantNow({
  enrollment,
  appointments,
}: {
  enrollment: JourneyEnrollment;
  appointments: CareAppointment[];
}) {
  // A busca de "próxima consulta" é genérica (várias linhas podem repartir a mesma
  // lista de CareAppointments) — por isso filtramos AQUI pelos refs dos itens da
  // linha ATIVA antes de escolher. Sem isto, o card podia carimbar `care_line_name`
  // da linha ativa numa consulta que pertence a outra linha (nome errado) e não
  // reagia à troca de chip.
  const refsDaLinha = new Set(enrollment.items.map((it) => it.item.ref));
  const consultasDaLinha = appointments.filter((a) => refsDaLinha.has(a.item_ref));
  const proxima = proximaConsulta(consultasDaLinha);
  const liberado = enrollment.items.find((it) => it.eligibility.allowed);

  let card: ReactNode;
  if (proxima) {
    card = <ConsultaCard appointment={proxima} linha={enrollment.care_line_name} />;
  } else if (liberado) {
    card = <AgendarCard item={liberado.item} linha={enrollment.care_line_name} />;
  } else {
    card = <TudoEmDiaCard />;
  }

  return (
    <section className="flex flex-col gap-3.5">
      <SectionLabel>O mais importante agora</SectionLabel>
      {card}
    </section>
  );
}

function DestaqueCard({ children }: { children: ReactNode }) {
  // Borda primary-200 e sombra um tom mais forte: o cartão que "puxa o olho".
  return (
    <div className="flex flex-col gap-3.5 rounded-lg border-[1.5px] border-primary-200 bg-white p-5 shadow-[0_4px_24px_rgba(14,25,85,0.10)]">
      {children}
    </div>
  );
}

function Pill({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-pill bg-primary-300 px-2.5 py-1 text-[10.5px] font-bold uppercase tracking-[0.06em] text-white">
      {children}
    </span>
  );
}

function ConsultaCard({ appointment, linha }: { appointment: CareAppointment; linha: string }) {
  return (
    <DestaqueCard>
      <div className="flex items-center gap-2">
        <Pill>Consulta</Pill>
        <span className="truncate text-[12.5px] text-muted">{linha}</span>
      </div>
      <div className="flex flex-col gap-0.5">
        <span className="text-lg font-bold text-primary-300">{appointment.label}</span>
        <span className="text-[13px] text-muted">
          {formatDateTimeShort(appointment.scheduled_at, appointment.time_zone)}
        </span>
      </div>
      <CtaLink to={`/consultas/${appointment.booking_id}`}>Ver consulta</CtaLink>
    </DestaqueCard>
  );
}

function AgendarCard({ item, linha }: { item: CareLineItemInfo; linha: string }) {
  const caption = item.recurrence ?? 'Consulta';
  return (
    <DestaqueCard>
      <div className="flex items-center gap-2">
        <Pill>Agendar</Pill>
        <span className="truncate text-[12.5px] text-muted">{linha}</span>
      </div>
      <div className="flex flex-col gap-0.5">
        <span className="text-lg font-bold text-primary-300">{item.label}</span>
        <span className="text-[13px] text-muted">{caption}</span>
      </div>
      <CtaLink to={`/jornada/agendar/${item.id}`}>Agendar</CtaLink>
    </DestaqueCard>
  );
}

function TudoEmDiaCard() {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-primary-100 bg-white p-5">
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-[rgba(41,176,29,0.12)] text-success">
        <IconCheck size={16} />
      </span>
      <span className="text-sm leading-5 text-ink">
        Tudo em dia por hoje. A gente te avisa quando algo precisar de você.
      </span>
    </div>
  );
}
