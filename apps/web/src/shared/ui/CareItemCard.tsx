import type { ReactNode } from 'react';

import type { CareLineItemInfo, Eligibility } from '../api';
import { Card } from './Card';
import { EligibilityNotice } from './EligibilityNotice';
import { IconCheck } from './icons';

/**
 * O card de um passo da linha de cuidado. O slot de ação obedece ao motor:
 *  - `done`        → linha verde "Feito" (item já cumprido);
 *  - `allowed`     → renderiza a `action` que a TELA passa (o card não conhece
 *                    rotas: quem sabe agendar é a tela);
 *  - bloqueado     → o EligibilityNotice com os motivos do servidor.
 *
 * SEM dots de cota: a API não expõe contagem de uso, então o front não a inventa
 * (decisão de produto). E sem corpo/descrição: `CareLineItemInfo` não tem esse
 * campo — não se preenche com texto imaginado.
 */

interface CareItemCardProps {
  item: CareLineItemInfo;
  eligibility: Eligibility;
  /** O botão/link de agendar, montado pela tela (que conhece a rota). */
  action?: ReactNode;
  done?: boolean;
  compact?: boolean;
}

// `kind` é hoje só 'CONSULTA' (Slice 1); o map deixa explícito o rótulo humano
// e evita mostrar o enum cru caso a linha não traga recorrência.
const KIND_LABEL: Record<CareLineItemInfo['kind'], string> = {
  CONSULTA: 'Consulta',
};

export function CareItemCard({
  item,
  eligibility,
  action,
  done = false,
  compact = false,
}: CareItemCardProps) {
  const caption = item.recurrence ? item.recurrence : KIND_LABEL[item.kind];

  return (
    <Card className={`flex flex-col ${compact ? 'gap-2.5 !p-4' : 'gap-[11px]'}`}>
      <div className="flex flex-col gap-px">
        <span className="text-base font-bold text-primary-300">{item.label}</span>
        <span className="text-[13px] text-muted">{caption}</span>
      </div>

      {done ? (
        <div className="flex items-center gap-2 text-[13px] font-bold text-success">
          <IconCheck size={15} />
          Feito
        </div>
      ) : eligibility.allowed ? (
        action
      ) : (
        <EligibilityNotice blocks={eligibility.blocks} compact={compact} />
      )}
    </Card>
  );
}
