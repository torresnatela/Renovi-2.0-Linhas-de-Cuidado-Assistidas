import { useRef } from 'react';
import type { KeyboardEvent } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface Option {
  value: string;
  label: string;
}

interface SegmentedControlProps {
  options: Option[];
  value: string;
  onChange: (value: string) => void;
  size?: 'sm' | 'md';
}

// Alternador em pill (fundo primary-100; aba ativa branca com sombra). Semântica
// de abas + roving tabindex: ←/→ movem a seleção e o foco.
export function SegmentedControl({ options, value, onChange, size = 'md' }: SegmentedControlProps) {
  const tabs = useRef<Array<HTMLButtonElement | null>>([]);
  const selectedIndex = options.findIndex((o) => o.value === value);
  const pad = size === 'sm' ? 'py-2 text-[13px]' : 'py-[11px] text-sm';

  function move(delta: number) {
    const count = options.length;
    if (count === 0) return;
    const from = selectedIndex < 0 ? 0 : selectedIndex;
    const next = (from + delta + count) % count;
    onChange(options[next].value);
    tabs.current[next]?.focus();
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      move(1);
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault();
      move(-1);
    }
  }

  return (
    <div role="tablist" className="flex gap-1 rounded-pill bg-primary-100 p-1">
      {options.map((option, index) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            ref={(el) => {
              tabs.current[index] = el;
            }}
            type="button"
            role="tab"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(option.value)}
            onKeyDown={onKeyDown}
            className={cx(
              'flex-1 rounded-pill text-center font-bold text-primary-300 transition',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
              pad,
              active
                ? 'bg-white shadow-[0_2px_10px_rgba(14,25,85,0.12)]'
                : 'bg-transparent active:opacity-70',
            )}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
