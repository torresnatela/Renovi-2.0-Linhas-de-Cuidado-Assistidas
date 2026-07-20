import type { ButtonHTMLAttributes } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

type Color = 'primary' | 'accent' | 'ghost';
type Size = 'sm' | 'md' | 'lg';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  color?: Color;
  size?: Size;
  loading?: boolean;
  fullWidth?: boolean;
}

const SIZES: Record<Size, string> = {
  sm: 'h-9 px-4 text-sm', // ~36px
  md: 'h-11 px-6 text-sm', // ~44px
  lg: 'h-14 px-8 text-base', // ~56px
};

// Disabled = tint -200 da própria cor (nunca cinza apagado). A REGRA de por que
// está desabilitado é da tela; aqui é só o visual.
function colorClasses(color: Color, disabled: boolean): string {
  switch (color) {
    case 'accent':
      return disabled ? 'bg-accent-200 text-white' : 'bg-accent-300 text-white shadow-button';
    case 'ghost':
      return disabled ? 'bg-transparent text-primary-200' : 'bg-transparent text-primary-300';
    case 'primary':
    default:
      return disabled ? 'bg-primary-200 text-white' : 'bg-primary-300 text-white';
  }
}

function Spinner() {
  return (
    <svg
      className="animate-spin"
      width={18}
      height={18}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.3" strokeWidth="2.5" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  );
}

export function Button({
  color = 'primary',
  size = 'md',
  loading = false,
  fullWidth = false,
  disabled = false,
  className,
  children,
  ...rest
}: ButtonProps) {
  const isDisabled = disabled || loading;
  return (
    <button
      type="button"
      disabled={isDisabled}
      aria-busy={loading || undefined}
      className={cx(
        'inline-flex items-center justify-center gap-2 rounded-lg font-bold uppercase transition',
        'active:opacity-70 disabled:cursor-not-allowed',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2',
        SIZES[size],
        colorClasses(color, isDisabled),
        fullWidth && 'w-full',
        className,
      )}
      {...rest}
    >
      {loading && <Spinner />}
      {children}
    </button>
  );
}
