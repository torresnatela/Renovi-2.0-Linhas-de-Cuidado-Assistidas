import type { ReactNode } from 'react';

import type { CareAppointment, CareLineItemInfo, JourneyEnrollment, MoodToday } from '../../shared/api';
import { Card } from '../../shared/ui/Card';
import { CareItemCard } from '../../shared/ui/CareItemCard';
import { IconCheck } from '../../shared/ui/icons';
import { PlanValidityBanner } from '../../shared/ui/PlanValidityBanner';
import { CtaLink } from './CtaLink';
import { ehAtividade, estadoAtividade, type EstadoAtividade } from './derivations';
import { SectionLabel } from './JourneyHero';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * "Sua jornada nesta linha": a faixa de vigência no topo e, abaixo, o fluxo da linha
 * como uma timeline — um nó numerado (ou check quando feito) ligado por conectores,
 * ao lado do card de cada passo.
 *
 * Dois eixos de estado bem diferentes, por `kind`:
 *  - CONSULTA usa o `CareItemCard`, que já sabe exibir o veredito do motor (ação
 *    quando liberado, EligibilityNotice quando bloqueado).
 *  - ATIVIDADE (check-in de humor, WHO-5/PHQ-4) NÃO tem especialidade nem slots —
 *    nunca é agendável, mesmo quando o motor a avalia como "liberada" (ela só não
 *    tem regras). Por isso usa `AtividadeCard`, cujo estado vem do check-in do dia
 *    (`useMoodToday`), não da elegibilidade.
 */
export function JourneyTimeline({
  enrollment,
  appointments,
  mood,
}: {
  enrollment: JourneyEnrollment;
  appointments: CareAppointment[];
  mood?: MoodToday;
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
          const isLast = i === items.length - 1;

          if (ehAtividade(it.item)) {
            const estado = estadoAtividade(it.item.ref, mood);
            const done = estado.tipo === 'feito_hoje';
            // Nó "aceso" (mesma cor de um item liberado) só quando ESTE card tem
            // uma ação de verdade (Responder agora) — nunca por a atividade estar
            // "liberada" no motor.
            const hasAction = estado.tipo === 'responder_agora';
            return (
              <li key={it.item.id} className="flex gap-4">
                <TimelineNode step={i + 1} isLast={isLast} done={done} allowed={hasAction} />
                <div className="min-w-0 flex-1 pb-4">
                  <AtividadeCard item={it.item} estado={estado} />
                </div>
              </li>
            );
          }

          const done = realizados.has(it.item.ref);
          const allowed = it.eligibility.allowed;
          return (
            <li key={it.item.id} className="flex gap-4">
              <TimelineNode step={i + 1} isLast={isLast} done={done} allowed={allowed} />
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

// O card de um passo ATIVIDADE. Mesmo cabeçalho do CareItemCard (título +
// legenda) — mas o corpo NUNCA é um link de agendar: uma atividade não tem
// especialidade nem slots (SPEC Anexo C).
function AtividadeCard({ item, estado }: { item: CareLineItemInfo; estado: EstadoAtividade }) {
  return (
    <Card className="flex flex-col gap-[11px]">
      <div className="flex flex-col gap-px">
        <span className="text-base font-bold text-primary-300">{item.label}</span>
        <span className="text-[13px] text-muted">{item.recurrence ?? 'Atividade'}</span>
      </div>
      <AtividadeCorpo estado={estado} />
    </Card>
  );
}

function AtividadeCorpo({ estado }: { estado: EstadoAtividade }) {
  switch (estado.tipo) {
    case 'feito_hoje':
      return (
        <div className="flex items-center gap-2 text-[13px] font-bold text-success">
          <IconCheck size={15} />
          Feito hoje
        </div>
      );
    case 'checkin_pendente':
      return (
        <BlocoNeutro>
          Ainda não feito hoje — <strong>registre no painel ao lado.</strong>
        </BlocoNeutro>
      );
    case 'responder_agora':
      return <CtaLink to={`/avaliacoes/${estado.codigo}`}>Responder agora</CtaLink>;
    case 'ofertado_quando_fizer_sentido':
      return (
        <BlocoNeutro>
          Oferecido pela equipe <strong>quando fizer sentido na sua jornada.</strong>
        </BlocoNeutro>
      );
    case 'sem_acao':
      return null;
  }
}

// Mesmo tom do EligibilityNotice (bloqueio de regra é estado do plano, não
// falha): fundo primary-100, texto navy — nunca vermelho, nunca tom de erro.
function BlocoNeutro({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-md bg-primary-100 px-[13px] py-[11px] text-[13px] leading-[19px] text-primary-300">
      {children}
    </p>
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
