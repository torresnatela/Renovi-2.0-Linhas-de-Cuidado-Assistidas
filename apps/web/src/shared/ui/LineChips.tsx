import { useRef } from 'react';
import type { KeyboardEvent } from 'react';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

interface Line {
  code: string;
  name: string;
}

interface LineChipsProps {
  lines: Line[];
  active: string;
  onSelect: (code: string) => void;
}

/**
 * Chips de alternância entre linhas de cuidado (o paciente pode ter mais de uma).
 * Semântica de abas + roving tabindex (←/→ movem seleção e foco), igual ao
 * SegmentedControl — mas NÃO o reusa: o visual é outro (chips soltos, ativo navy
 * sólido; o SegmentedControl é um trilho com aba branca). Duplicar o punhado de
 * linhas do roving é mais barato que uma abstração compartilhada de um só uso.
 */
export function LineChips({ lines, active, onSelect }: LineChipsProps) {
  const tabs = useRef<Array<HTMLButtonElement | null>>([]);
  const selectedIndex = lines.findIndex((l) => l.code === active);

  function move(delta: number) {
    const count = lines.length;
    if (count === 0) return;
    const from = selectedIndex < 0 ? 0 : selectedIndex;
    const next = (from + delta + count) % count;
    onSelect(lines[next].code);
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
    <div role="tablist" className="flex flex-wrap gap-2">
      {lines.map((line, index) => {
        const isActive = line.code === active;
        return (
          <button
            key={line.code}
            ref={(el) => {
              tabs.current[index] = el;
            }}
            type="button"
            role="tab"
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onSelect(line.code)}
            onKeyDown={onKeyDown}
            className={cx(
              'rounded-pill px-4 py-2 text-[13.5px] font-bold transition',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
              isActive
                ? 'bg-primary-300 text-white'
                : 'border border-primary-200 bg-white text-primary-300 active:opacity-70',
            )}
          >
            {line.name}
          </button>
        );
      })}
    </div>
  );
}
