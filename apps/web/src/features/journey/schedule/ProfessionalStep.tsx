import { formatDateLong } from '../../../shared/datetime';
import { Avatar } from '../../../shared/ui/Avatar';
import { IconCaretRight, IconClock } from '../../../shared/ui/icons';

/**
 * O profissional como a DISPONIBILIDADE o traz: id + nome, e o primeiro horário
 * livre dele. Sem registro no conselho nem bio — a agenda de uma LINHA não os
 * fornece (o `professional` do slot é o `AppointmentProfessional`), então o card
 * mostra só o nome. Nada de badge "Cuida de você": inventar dado é pior que omiti-lo.
 */
export interface ProfissionalResumo {
  id: string;
  full_name: string;
  primeiroInicio: string;
  timeZone: string;
}

interface ProfessionalStepProps {
  profissionais: ProfissionalResumo[];
  onEscolher: (id: string) => void;
}

export function ProfessionalStep({ profissionais, onEscolher }: ProfessionalStepProps) {
  return (
    <section className="flex flex-col gap-5">
      <header className="flex flex-col gap-1">
        <h2 className="text-xl font-bold text-primary-300">Com quem você quer se consultar?</h2>
        <p className="text-sm text-muted">
          Continuar com quem já te acompanha ajuda na continuidade do cuidado.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2">
        {profissionais.map((p) => (
          <button
            key={p.id}
            type="button"
            aria-label={`Escolher ${p.full_name}`}
            onClick={() => onEscolher(p.id)}
            className="flex w-full flex-col gap-3 rounded-lg border border-primary-100 bg-white p-[18px] text-left shadow-card transition hover:border-primary-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
          >
            <span className="flex items-center gap-3">
              <Avatar name={p.full_name} />
              <span className="block font-bold text-primary-300">{p.full_name}</span>
            </span>
            <span className="flex items-center justify-between gap-2 border-t border-primary-100 pt-3">
              <span className="inline-flex items-center gap-1.5 text-sm text-accent-300">
                <IconClock size={16} />
                <span>
                  Horários a partir de{' '}
                  <span className="first-letter:uppercase">
                    {formatDateLong(p.primeiroInicio, p.timeZone)}
                  </span>
                </span>
              </span>
              <span className="inline-flex shrink-0 items-center gap-1 text-sm font-bold text-primary-300">
                Escolher
                <IconCaretRight size={16} />
              </span>
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}
