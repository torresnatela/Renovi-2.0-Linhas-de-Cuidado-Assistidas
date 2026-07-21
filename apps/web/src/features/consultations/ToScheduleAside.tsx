import type { Journey } from '../../shared/api';
import { EligibilityNotice } from '../../shared/ui/EligibilityNotice';
import { LinkButton, SectionLabel } from './parts';

/**
 * O aside sticky "Para agendar": os itens da linha de cuidado que ainda cabem,
 * cada um com o veredito do motor JÁ pronto do servidor.
 *
 * SEM dots de cota (a API não expõe contagem de uso — o front não a inventa,
 * decisão de produto). Item liberado → botão Agendar (a rota por ITEM passa pelo
 * motor de novo). Item bloqueado → `EligibilityNotice`, que mostra O QUÊ, POR QUÊ
 * e A PARTIR DE QUANDO em tom neutro, nunca vermelho.
 */
export function ToScheduleAside({ journey }: { journey: Journey | undefined }) {
  // `GET /me/journey` devolve TODO o histórico de matrículas (inclusive
  // encerrada/expirada/concluída). "Para agendar" é o PRESENTE — igual à
  // Jornada (`JourneyPage.tsx`) e ao Perfil (`PlanSection.tsx`), só matrículas
  // `ativa` entram aqui. Sem este filtro, itens de matrícula encerrada
  // apareciam duplicados/mortos no funil de agendamento.
  const enrollmentsAtivas = (journey?.enrollments ?? []).filter(
    (e) => e.enrollment.status === 'ativa',
  );
  // Só CONSULTA é agendável (tem especialidade + slots no legado). ATIVIDADE
  // (check-in de humor, WHO-5, PHQ-4) vive na Jornada — aqui o clique em
  // "Agendar" levaria a uma agenda vazia, então o item nem aparece no funil.
  const itens = enrollmentsAtivas
    .flatMap((e) => e.items)
    .filter(({ item }) => item.kind === 'CONSULTA');
  if (itens.length === 0) return null;

  return (
    <aside className="flex flex-col gap-3.5 lg:sticky lg:top-[102px]">
      <SectionLabel>Para agendar</SectionLabel>
      {itens.map(({ item, eligibility }) => (
        <div
          key={item.id}
          className="flex flex-col gap-3 rounded-lg border-[1.5px] border-dashed border-primary-200 bg-white p-[18px]"
        >
          <div className="flex flex-col gap-px">
            {/* 15px/12.5px no mock mobile (Consultas.dc.html:86-87), espelhando
                AppointmentCard; `lg:` devolve o tamanho de sempre do desktop
                (base/13px). */}
            <span className="text-[15px] font-bold text-primary-300 lg:text-base">
              {item.label}
            </span>
            <span className="text-[12.5px] text-muted lg:text-[13px]">
              {item.recurrence ?? 'Consulta'}
            </span>
          </div>

          {eligibility.allowed ? (
            <LinkButton to={`/jornada/agendar/${item.id}`}>Agendar</LinkButton>
          ) : (
            <EligibilityNotice blocks={eligibility.blocks} />
          )}
        </div>
      ))}
    </aside>
  );
}
