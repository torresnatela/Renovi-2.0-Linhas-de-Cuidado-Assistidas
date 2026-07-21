import type { ReactNode } from 'react';

import { LineChips } from '../../shared/ui/LineChips';
import { useIsDesktop } from '../../shared/viewport';
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
  /**
   * Afordância de ajuda à direita do título — no mobile a página passa o
   * `HelpNowMenu` (no desktop ele vive no chrome do AppShell, então fica vazio).
   */
  help?: ReactNode;
}

/**
 * O topo da Jornada: saudação pelo primeiro nome + uma frase de resumo do dia, com
 * os chips de linha de cuidado à direita. Os chips só aparecem quando há mais de uma
 * linha — para uma só, o nome já está em destaque no card de vigência abaixo.
 *
 * O CHROME muda por viewport (ADR-041, "estrutura → hook"): no mobile o mock põe a
 * ajuda na mesma linha do título, o resumo abaixo e os chips por último; no desktop
 * o layout histórico (coluna + chips à direita, alinhados à base) fica intocado.
 */
export function JourneyHero({
  fullName,
  resumo,
  lines,
  activeCode,
  onSelect,
  help,
}: JourneyHeroProps) {
  const isDesktop = useIsDesktop();
  const chips = lines.length > 1 && (
    <LineChips lines={lines} active={activeCode} onSelect={onSelect} />
  );

  if (!isDesktop) {
    return (
      <div className="mb-5 flex flex-col gap-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-col gap-0.5">
            {/* Eyebrow 11px (mock) — desktop mantém 12px via SectionLabel. */}
            <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">
              Sua jornada
            </span>
            <span className="text-[26px] font-bold leading-8 text-primary-300">
              Olá, {primeiroNome(fullName)}
            </span>
          </div>
          {help}
        </div>
        <span className="text-sm leading-[21px] text-ink">{resumo}</span>
        {chips}
      </div>
    );
  }

  return (
    <div className="mb-7 flex flex-wrap items-end justify-between gap-8">
      <div className="flex min-w-0 flex-col gap-1">
        <SectionLabel>Sua jornada</SectionLabel>
        <span className="text-[38px] font-bold leading-[44px] text-primary-300">
          Olá, {primeiroNome(fullName)}
        </span>
        <span className="mt-1 text-base leading-6 text-ink">{resumo}</span>
      </div>
      {chips}
    </div>
  );
}
