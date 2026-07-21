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
  const itens = journey?.enrollments.flatMap((e) => e.items) ?? [];
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
            <span className="text-base font-bold text-primary-300">{item.label}</span>
            <span className="text-[13px] text-muted">{item.recurrence ?? 'Consulta'}</span>
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
