import type { ReactNode } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface ListRowProps {
  icon?: ReactNode;
  title: string;
  caption?: string;
  right?: ReactNode;
  onClick?: () => void;
}

function Inner({ icon, title, caption, right }: Omit<ListRowProps, 'onClick'>) {
  return (
    <>
      {icon && <span className="shrink-0 text-primary-300">{icon}</span>}
      <span className="min-w-0 flex-1 text-left">
        <span className="block truncate font-semibold text-ink">{title}</span>
        {caption && <span className="block truncate text-sm text-muted">{caption}</span>}
      </span>
      {right && <span className="shrink-0">{right}</span>}
    </>
  );
}

// Linha de lista. Clicável => `<button>` full-width (teclado/foco nativos +
// hover tint). Estática => `<div>` sem role de botão.
export function ListRow({ icon, title, caption, right, onClick }: ListRowProps) {
  const base = 'flex w-full items-center gap-3 rounded-md px-3 py-3';
  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={cx(
          base,
          'text-left transition hover:bg-page',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
        )}
      >
        <Inner icon={icon} title={title} caption={caption} right={right} />
      </button>
    );
  }
  return (
    <div className={base}>
      <Inner icon={icon} title={title} caption={caption} right={right} />
    </div>
  );
}
