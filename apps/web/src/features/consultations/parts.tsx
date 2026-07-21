import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * "{label}" ou "{label} · {nome}" quando o profissional já carregou.
 *
 * O nome vem de `useBookingProfessionals` (enriquecimento client-side via o
 * booking) e é ENHANCEMENT: enquanto carrega ou se a busca falhar, `nome` chega
 * `undefined` e o título cai de volta para só o label — sem skeleton, sem erro.
 */
export function tituloConsulta(label: string, nomeProfissional?: string): string {
  return nomeProfissional ? `${label} · ${nomeProfissional}` : label;
}

/**
 * Rótulo de seção do design: bold, caixa-alta, tracking largo, muted. 11px no
 * mobile (mock `Consultas.dc.html`, todo `SectionLabel` do arquivo); `lg:text-xs`
 * preserva o 12px do desktop atual — só ESTILO muda por breakpoint (ADR-041).
 */
export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted lg:text-xs">
      {children}
    </span>
  );
}

/**
 * Um `<Link>` com a APARÊNCIA do Button primary do DS.
 *
 * Existe porque o `Button` do DS é um `<button>` — ele age, não navega. Onde a
 * ação é "ir para outra rota" (agendar, ver consulta), o certo é um link de
 * verdade (funciona com o teclado, com "abrir em nova aba", com o back), não um
 * botão que chama `navigate()`.
 */
export function LinkButton({
  to,
  children,
  fullWidth = true,
}: {
  to: string;
  children: ReactNode;
  fullWidth?: boolean;
}) {
  return (
    <Link
      to={to}
      className={cx(
        'inline-flex h-11 items-center justify-center gap-2 rounded-lg bg-primary-300 px-6 text-sm font-bold uppercase text-white transition',
        'active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2',
        fullWidth && 'w-full',
      )}
    >
      {children}
    </Link>
  );
}
