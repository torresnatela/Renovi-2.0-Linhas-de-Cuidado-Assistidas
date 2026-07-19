import type { EligibilityBlock } from '../../shared/api';
import { formatDateTimeShort } from '../../shared/datetime';
import { FUSO_PADRAO } from './useJourney';

/**
 * A lista de motivos de um bloqueio do motor — a regra de ouro de UX em forma de
 * componente: quando um item não pode ser agendado, o paciente lê POR QUÊ.
 *
 * O `reason` vem PRONTO do servidor e NÃO se re-traduz: é a exceção à tabela do
 * reasons.ts (essa é para `Reason.code`). O `available_from`, quando presente, diz
 * quando o item destrava — formatado no fuso da agenda, nunca no do browser.
 */
export function Blocos({ blocks }: { blocks: EligibilityBlock[] }) {
  return (
    <ul className="grid gap-1">
      {blocks.map((b, i) => (
        <li key={`${b.rule_type}-${i}`} className="text-sm text-amber-900">
          {b.reason}
          {b.available_from && (
            <span className="text-amber-700">
              {' '}
              (disponível a partir de {formatDateTimeShort(b.available_from, FUSO_PADRAO)})
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}
