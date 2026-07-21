import type { EligibilityBlock, EligibilityRuleType } from '../api';
import { formatDateTimeShort, FUSO_PADRAO } from '../datetime';
import { IconArrowRight, IconCalendar, IconClock } from './icons';

/**
 * O bloco de bloqueio explicável — a regra de ouro de UX em forma de componente:
 * quando um item não pode ser agendado, o paciente lê O QUÊ, POR QUÊ e A PARTIR
 * DE QUANDO, em tom neutro.
 *
 * Dois invariantes de produto vivem aqui:
 *  - `reason` é escrito pelo SERVIDOR pensando no paciente e é exibido VERBATIM —
 *    NÃO se re-traduz (é a exceção à tabela do reasons.ts, que é para `Reason.code`).
 *  - Bloqueio de regra é ESTADO DO PLANO, não falha: fundo `surface-subtle`, ícone
 *    de linha navy, data/ação em bold. NUNCA vermelho, nunca tom de erro.
 *
 * `available_from` só vem quando o desbloqueio depende do relógio (cota, intervalo,
 * antecedência) — formatado no fuso da agenda, nunca no do browser.
 */

interface EligibilityNoticeProps {
  blocks: EligibilityBlock[];
  /** Padding e ícone menores, para caber dentro de pills de horário (Etapa 5). */
  compact?: boolean;
  timeZone?: string;
}

// VIGENCIA/MAX_ADVANCE são de calendário; QUOTA/MIN_INTERVAL são de espera; o
// pré-requisito aponta para outro item. O `name` é só para o smoke test de ícone.
function iconFor(rule: EligibilityRuleType): { Icon: typeof IconClock; name: string } {
  switch (rule) {
    case 'PREREQUISITE':
      return { Icon: IconArrowRight, name: 'arrow' };
    case 'VIGENCIA':
    case 'MAX_ADVANCE':
      return { Icon: IconCalendar, name: 'calendar' };
    case 'QUOTA':
    case 'MIN_INTERVAL':
    default:
      return { Icon: IconClock, name: 'clock' };
  }
}

export function EligibilityNotice({
  blocks,
  compact = false,
  timeZone = FUSO_PADRAO,
}: EligibilityNoticeProps) {
  const box = compact ? 'gap-2 px-2.5 py-1.5' : 'gap-2.5 px-[13px] py-[11px]';
  const text = compact ? 'text-xs leading-[17px]' : 'text-[13px] leading-[19px]';
  const iconSize = compact ? 14 : 16;

  return (
    <ul className="grid gap-2">
      {blocks.map((block, i) => {
        const { Icon, name } = iconFor(block.rule_type);
        return (
          <li
            key={`${block.rule_type}-${i}`}
            className={`flex items-start rounded-md bg-primary-100 text-primary-300 ${box}`}
          >
            <span
              data-testid="eligibility-icon"
              data-icon={name}
              className="mt-px shrink-0"
              aria-hidden="true"
            >
              <Icon size={iconSize} />
            </span>
            <p className={text}>
              <span>{block.reason}</span>
              {block.available_from && (
                <>
                  {' '}
                  <strong>
                    Disponível a partir de {formatDateTimeShort(block.available_from, timeZone)}
                  </strong>
                </>
              )}
            </p>
          </li>
        );
      })}
    </ul>
  );
}
