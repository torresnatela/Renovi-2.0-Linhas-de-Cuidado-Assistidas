import type { ReactNode } from 'react';

import { LineChips } from '../../shared/ui/LineChips';
import { primeiroNome } from './derivations';

/** Rótulo de seção (eyebrow): 12px, bold, uppercase, tracking largo, cinza. */
export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-xs font-bold uppercase tracking-[0.08em] text-muted">{children}</span>
  );
}

interface JourneyHeroProps {
  fullName: string | undefined;
  resumo: string;
  lines: Array<{ code: string; name: string }>;
  activeCode: string;
  onSelect: (code: string) => void;
}

/**
 * O topo da Jornada: saudação pelo primeiro nome + uma frase de resumo do dia, com
 * os chips de linha de cuidado à direita. Os chips só aparecem quando há mais de uma
 * linha — para uma só, o nome já está em destaque no card de vigência abaixo.
 */
export function JourneyHero({ fullName, resumo, lines, activeCode, onSelect }: JourneyHeroProps) {
  return (
    <div className="mb-7 flex flex-wrap items-end justify-between gap-8">
      <div className="flex min-w-0 flex-col gap-1">
        <SectionLabel>Sua jornada</SectionLabel>
        <span className="text-[38px] font-bold leading-[44px] text-primary-300">
          Olá, {primeiroNome(fullName)}
        </span>
        <span className="mt-1 text-base leading-6 text-ink">{resumo}</span>
      </div>
      {lines.length > 1 && <LineChips lines={lines} active={activeCode} onSelect={onSelect} />}
    </div>
  );
}
