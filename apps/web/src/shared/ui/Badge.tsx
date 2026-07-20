import type { ReactNode } from 'react';

type Tone = 'success' | 'neutral' | 'accent' | 'alert';

/**
 * Pill de status curto (ex.: "Plano ativo", "Hoje"). Fundo suave + texto forte.
 * Os fundos são rgba LITERAIS do DS (não `/alpha` sobre token, que é proibido).
 * Badge NUNCA sinaliza bloqueio de regra — isso é o EligibilityNotice (Etapa 1b).
 */
const TONES: Record<Tone, string> = {
  success: 'bg-[rgba(41,176,29,0.12)] text-success',
  neutral: 'bg-primary-100 text-primary-300',
  accent: 'bg-[rgba(250,143,27,0.12)] text-accent-300',
  alert: 'bg-[rgba(251,199,15,0.18)] text-primary-300',
};

interface BadgeProps {
  tone: Tone;
  children: ReactNode;
}

export function Badge({ tone, children }: BadgeProps) {
  return (
    <span
      className={`inline-flex items-center whitespace-nowrap rounded-pill px-3 py-1 text-xs font-bold ${TONES[tone]}`}
    >
      {children}
    </span>
  );
}
