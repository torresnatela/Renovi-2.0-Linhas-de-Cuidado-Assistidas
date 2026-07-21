import { IconCheck } from '../../../shared/ui/icons';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

export type PassoEstado = 'completo' | 'ativo' | 'pendente';

export interface PassoInfo {
  titulo: string;
  /** O valor já escolhido (ou o convite para escolher). */
  caption: string;
  estado: PassoEstado;
}

interface StepperProps {
  passos: PassoInfo[];
  /** 1-based. */
  atual: number;
  /** Volta para um passo já COMPLETO. */
  onIr: (n: number) => void;
  /** Falso enquanto um agendamento está em voo: trava a navegação para trás. */
  navegavel: boolean;
}

/**
 * A trilha vertical do wizard (Etapa 5). Cada passo é um círculo numerado (ou um
 * check verde quando completo) + título + o valor escolhido. O passo ativo ganha
 * `aria-current="step"` e fundo `primary-100`; os já feitos viram botões de volta.
 */
export function Stepper({ passos, atual, onIr, navegavel }: StepperProps) {
  const total = passos.length;

  return (
    <div className="flex flex-col gap-5">
      <ol className="flex flex-col gap-1">
        {passos.map((p, i) => {
          const n = i + 1;
          const podeVoltar = p.estado === 'completo' && navegavel;

          const linha = (
            <span
              className={cx(
                'flex items-center gap-3 rounded-md px-3 py-2.5',
                p.estado === 'ativo' && 'bg-primary-100',
              )}
            >
              <span className={circulo(p.estado)} aria-hidden="true">
                {p.estado === 'completo' ? <IconCheck size={18} /> : n}
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="text-sm font-bold text-primary-300">{p.titulo}</span>
                <span className="truncate text-xs text-muted first-letter:uppercase">
                  {p.caption}
                </span>
              </span>
            </span>
          );

          return (
            <li key={p.titulo} aria-current={p.estado === 'ativo' ? 'step' : undefined}>
              {podeVoltar ? (
                <button
                  type="button"
                  aria-label={p.titulo}
                  onClick={() => onIr(n)}
                  className="w-full rounded-md text-left transition hover:bg-page focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
                >
                  {linha}
                </button>
              ) : (
                linha
              )}
            </li>
          );
        })}
      </ol>

      <div>
        <div className="h-1 overflow-hidden rounded-pill bg-primary-100">
          <div
            className="h-full rounded-pill bg-primary-300 transition-all"
            style={{ width: `${(atual / total) * 100}%` }}
          />
        </div>
        <p className="mt-2 text-xs text-muted">
          Passo {atual} de {total} · leva menos de 1 minuto
        </p>
      </div>
    </div>
  );
}

function circulo(estado: PassoEstado): string {
  const base =
    'inline-flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-full text-sm font-bold';
  if (estado === 'completo') return `${base} bg-success text-white`;
  if (estado === 'ativo') return `${base} bg-primary-300 text-white`;
  return `${base} border-2 border-primary-200 text-muted`;
}
