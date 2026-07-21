import { formatDateLong } from '../../../shared/datetime';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/** Um dia agendável do profissional, representado pelo seu primeiro horário. */
export interface DiaResumo {
  /** `dayKey` no fuso do slot — a identidade estável do dia. */
  key: string;
  inicio: string;
  timeZone: string;
}

interface DateStepProps {
  dias: DiaResumo[];
  selecionado: string | null;
  onEscolher: (key: string) => void;
}

export function DateStep({ dias, selecionado, onEscolher }: DateStepProps) {
  return (
    <section className="flex flex-col gap-5">
      <header>
        <h2 className="text-xl font-bold text-primary-300">Escolha o dia</h2>
      </header>

      <div className="flex flex-wrap gap-3">
        {dias.map((d) => {
          const ativo = d.key === selecionado;
          return (
            <button
              key={d.key}
              type="button"
              aria-pressed={ativo}
              onClick={() => onEscolher(d.key)}
              className={cx(
                'rounded-lg border px-4 py-3 text-sm font-bold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
                ativo
                  ? 'border-primary-300 bg-primary-300 text-white'
                  : 'border-primary-200 bg-white text-primary-300 hover:border-primary-300',
              )}
            >
              <span className="first-letter:uppercase">
                {formatDateLong(d.inicio, d.timeZone)}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
