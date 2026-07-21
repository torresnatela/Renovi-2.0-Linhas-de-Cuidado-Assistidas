import { useRef } from 'react';
import type { KeyboardEvent, MouseEvent, PointerEvent } from 'react';

/**
 * Um ponto na grade de humor, em escala 0–100 nos dois eixos:
 *  - `valencia`: horizontal — 0 = desagradável, 100 = agradável;
 *  - `energia`:  vertical   — 100 = mais energia (topo), 0 = menos energia.
 *
 * O quadrante/rótulo é DERIVADO deste ponto por quem consome (MoodPage usa o do
 * servidor; o card do aside usa o mapa 3×3 do design) — a grade não os conhece.
 */
export interface MoodPoint {
  valencia: number;
  energia: number;
}

interface MoodGridProps {
  /** Ponto atual (componente CONTROLADO). `null` = ainda sem seleção (sem marcador). */
  value: MoodPoint | null;
  onChange: (point: MoodPoint) => void;
  disabled?: boolean;
}

const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const clamp100 = (v: number) => Math.min(100, Math.max(0, v));

/**
 * A grade valência×energia, extraída da MoodPage para servir também o check-in do
 * aside da Jornada. Preserva o contrato de acessibilidade que os testes fixam:
 * papel de botão com rótulo, operação por teclado (setas, passo 5) e o valor
 * anunciado a leitores de tela — e acrescenta o arraste contínuo por Pointer
 * Events e o visual do design (4 gradientes oklch nos cantos + cruz central).
 */
export function MoodGrid({ value, onChange, disabled = false }: MoodGridProps) {
  const gridRef = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);

  // Converte a posição do ponteiro na grade em valência/energia (0–100). O topo da
  // grade é MAIS energia, por isso energia = (1 − y). O jsdom devolve um rect
  // zerado; os testes injetam um getBoundingClientRect para o cálculo fazer sentido.
  function fromPointer(clientX: number, clientY: number) {
    const el = gridRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const x = clamp01((clientX - rect.left) / rect.width);
    const y = clamp01((clientY - rect.top) / rect.height);
    onChange({ valencia: Math.round(x * 100), energia: Math.round((1 - y) * 100) });
  }

  function onClick(e: MouseEvent<HTMLDivElement>) {
    if (disabled) return;
    fromPointer(e.clientX, e.clientY);
  }

  // Teclado: as setas movem o ponto em passos de 5, partindo do centro (50,50)
  // quando ainda não há seleção. Mantém a grade operável sem ponteiro.
  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (disabled) return;
    const passo = 5;
    const movimentos: Record<string, [number, number]> = {
      ArrowLeft: [-passo, 0],
      ArrowRight: [passo, 0],
      ArrowUp: [0, passo],
      ArrowDown: [0, -passo],
    };
    const mov = movimentos[e.key];
    if (!mov) return;
    e.preventDefault();
    const base = value ?? { valencia: 50, energia: 50 };
    onChange({
      valencia: clamp100(base.valencia + mov[0]),
      energia: clamp100(base.energia + mov[1]),
    });
  }

  // Arraste contínuo: segura o ponteiro na grade e desliza. `setPointerCapture?.`
  // é optional chaining porque o jsdom não implementa a API (nos testes bastam
  // clique e teclado); num browser real garante que o move/up sigam vindo aqui.
  function onPointerDown(e: PointerEvent<HTMLDivElement>) {
    if (disabled) return;
    dragging.current = true;
    e.currentTarget.setPointerCapture?.(e.pointerId);
    fromPointer(e.clientX, e.clientY);
  }
  function onPointerMove(e: PointerEvent<HTMLDivElement>) {
    if (disabled || !dragging.current) return;
    fromPointer(e.clientX, e.clientY);
  }
  function onPointerUp() {
    dragging.current = false;
  }

  return (
    <div>
      <div
        ref={gridRef}
        role="button"
        tabIndex={0}
        aria-label="Grade de humor: valência por energia"
        aria-disabled={disabled || undefined}
        onClick={onClick}
        onKeyDown={onKeyDown}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        className="relative h-[210px] cursor-crosshair overflow-hidden rounded-md border border-primary-100 [touch-action:none] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
        style={{
          // Um gradiente radial por canto (energia/valência), sobre branco.
          background:
            'radial-gradient(120% 120% at 100% 0%, oklch(0.88 0.08 75 / 0.9), transparent 62%),' +
            'radial-gradient(120% 120% at 0% 0%, oklch(0.84 0.06 25 / 0.75), transparent 62%),' +
            'radial-gradient(120% 120% at 0% 100%, oklch(0.86 0.045 265 / 0.75), transparent 62%),' +
            'radial-gradient(120% 120% at 100% 100%, oklch(0.89 0.06 175 / 0.85), transparent 62%),' +
            'white',
        }}
      >
        {/* Cruz central hairline + rótulos de eixo — decorativos (pointer-events: none). */}
        <span className="pointer-events-none absolute bottom-1.5 left-1/2 top-1.5 w-px bg-[rgba(14,25,85,0.10)]" />
        <span className="pointer-events-none absolute left-1.5 right-1.5 top-1/2 h-px bg-[rgba(14,25,85,0.10)]" />
        <span className="pointer-events-none absolute left-1/2 top-2 -translate-x-1/2 text-[10px] font-bold uppercase tracking-[0.07em] text-[rgba(14,25,85,0.45)]">
          + energia
        </span>
        <span className="pointer-events-none absolute bottom-2 left-1/2 -translate-x-1/2 text-[10px] font-bold uppercase tracking-[0.07em] text-[rgba(14,25,85,0.45)]">
          − energia
        </span>
        <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 rotate-180 text-[10px] font-bold uppercase tracking-[0.07em] text-[rgba(14,25,85,0.45)] [writing-mode:vertical-rl]">
          Desagradável
        </span>
        <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] font-bold uppercase tracking-[0.07em] text-[rgba(14,25,85,0.45)] [writing-mode:vertical-rl]">
          Agradável
        </span>

        {value && (
          <span
            data-testid="mood-marker"
            className="pointer-events-none absolute h-[30px] w-[30px] -translate-x-1/2 -translate-y-1/2 rounded-full border-[2.5px] border-primary-300 bg-white shadow-[0_4px_14px_rgba(14,25,85,0.25)]"
            style={{ left: `${value.valencia}%`, top: `${100 - value.energia}%` }}
          />
        )}
      </div>

      {/* A grade é visual: o ponto escolhido é anunciado a leitores de tela. */}
      {value && (
        <p className="sr-only" aria-live="polite" data-testid="mood-value">
          Selecionado: valência {value.valencia} de 100, energia {value.energia} de 100.
        </p>
      )}
    </div>
  );
}
