import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * Um `Link` com aparência de Button do DS. A Jornada navega por rota (agendar por
 * item, ver consulta), então precisa da SEMÂNTICA de link — mas o visual é o do
 * botão. O rótulo em UPPERCASE vem da classe `uppercase` (nunca do texto-fonte),
 * preservando o nome acessível para leitores de tela e para os testes.
 */
export function CtaLink({
  to,
  color = 'primary',
  children,
}: {
  to: string;
  color?: 'primary' | 'accent';
  children: ReactNode;
}) {
  return (
    <Link
      to={to}
      className={cx(
        'inline-flex h-11 items-center justify-center rounded-lg px-6 text-sm font-bold uppercase transition',
        'active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2',
        color === 'accent'
          ? 'bg-accent-300 text-white shadow-button'
          : 'bg-primary-300 text-white',
      )}
    >
      {children}
    </Link>
  );
}
