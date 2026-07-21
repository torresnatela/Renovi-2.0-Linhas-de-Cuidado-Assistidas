import type { CareAppointment, JourneyEnrollment } from '../../shared/api';
import { CareItemCard } from '../../shared/ui/CareItemCard';
import { IconCheck } from '../../shared/ui/icons';
import { PlanValidityBanner } from '../../shared/ui/PlanValidityBanner';
import { CtaLink } from './CtaLink';
import { SectionLabel } from './JourneyHero';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * "Sua jornada nesta linha": a faixa de vigência no topo e, abaixo, o fluxo da linha
 * como uma timeline — um nó numerado (ou check quando feito) ligado por conectores,
 * ao lado do CareItemCard de cada passo. O card já sabe exibir o veredito do motor
 * (ação quando liberado, EligibilityNotice quando bloqueado); aqui só ordenamos os
 * itens e desenhamos os nós.
 */
export function JourneyTimeline({
  enrollment,
  appointments,
}: {
  enrollment: JourneyEnrollment;
  appointments: CareAppointment[];
}) {
  const items = [...enrollment.items].sort((a, b) => a.item.sort_order - b.item.sort_order);

  // "feito" é AÇÚCAR DE EXIBIÇÃO: existe uma consulta 'realizada' cujo item_ref bate
  // com o ref do passo. Não é regra de negócio — o motor decide elegibilidade; isto
  // só marca o passo como cumprido na linha do tempo.
  const realizados = new Set(
    appointments.filter((a) => a.status === 'realizada').map((a) => a.item_ref),
  );

  return (
    <section className="flex flex-col gap-3.5">
      <SectionLabel>Sua jornada nesta linha</SectionLabel>
      <PlanValidityBanner
        enrollment={enrollment.enrollment}
        careLineName={enrollment.care_line_name}
      />

      <ol className="mt-1.5 flex flex-col">
        {items.map((it, i) => {
          const done = realizados.has(it.item.ref);
          const allowed = it.eligibility.allowed;
          return (
            <li key={it.item.id} className="flex gap-4">
              <TimelineNode step={i + 1} isLast={i === items.length - 1} done={done} allowed={allowed} />
              <div className="min-w-0 flex-1 pb-4">
                <CareItemCard
                  item={it.item}
                  eligibility={it.eligibility}
                  done={done}
                  action={
                    allowed ? (
                      <CtaLink to={`/jornada/agendar/${it.item.id}`}>Agendar</CtaLink>
                    ) : undefined
                  }
                />
              </div>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

// Nó 34px + conector vertical. Verde+check quando feito; navy quando liberado;
// branco com borda cinza quando bloqueado — o mesmo código-cor dos estados do card.
function TimelineNode({
  step,
  isLast,
  done,
  allowed,
}: {
  step: number;
  isLast: boolean;
  done: boolean;
  allowed: boolean;
}) {
  const node = done
    ? 'bg-success border-success text-white'
    : allowed
      ? 'bg-primary-300 border-primary-300 text-white'
      : 'bg-white border-primary-200 text-muted';

  return (
    <div className="flex w-[34px] shrink-0 flex-col items-center">
      <span
        className={cx(
          'flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-full border-2 text-[13.5px] font-bold',
          node,
        )}
      >
        {done ? <IconCheck size={15} /> : step}
      </span>
      {!isLast && (
        <span className={cx('my-1 w-0.5 flex-1 rounded-full', done ? 'bg-success' : 'bg-primary-100')} />
      )}
    </div>
  );
}
