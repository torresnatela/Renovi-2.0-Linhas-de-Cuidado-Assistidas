type Size = 'sm' | 'md' | 'lg';

interface AvatarProps {
  name: string;
  size?: Size;
}

const SIZES: Record<Size, string> = {
  sm: 'h-8 w-8 text-xs', // 32px
  md: 'h-10 w-10 text-sm', // 40px
  lg: 'h-[84px] w-[84px] text-3xl', // 84px
};

// Iniciais = primeira letra do primeiro nome + primeira do último (regra única).
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '';
  if (parts.length === 1) return parts[0][0].toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function Avatar({ name, size = 'md' }: AvatarProps) {
  return (
    <span
      role="img"
      aria-label={name}
      className={`inline-flex shrink-0 items-center justify-center rounded-full bg-primary-300 font-bold text-white ${SIZES[size]}`}
    >
      <span aria-hidden="true">{initials(name)}</span>
    </span>
  );
}
