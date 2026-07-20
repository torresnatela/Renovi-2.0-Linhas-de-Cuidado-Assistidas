import type { ElementType, ReactNode } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface CardProps {
  as?: ElementType;
  padding?: 'md' | 'lg';
  className?: string;
  children?: ReactNode;
}

// Superfície branca padrão do DS: raio 16, borda leve navy e a sombra única.
export function Card({ as, padding = 'md', className, children }: CardProps) {
  const Comp = as ?? 'div';
  const pad = padding === 'lg' ? 'p-5' : 'p-[18px]';
  return (
    <Comp
      className={cx('bg-white rounded-lg border border-primary-100 shadow-card', pad, className)}
    >
      {children}
    </Comp>
  );
}
