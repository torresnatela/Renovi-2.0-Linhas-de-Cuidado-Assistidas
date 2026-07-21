import { Avatar } from '../../../shared/ui/Avatar';

interface ResumoBarProps {
  /** Nome do profissional já escolhido. */
  nome: string;
  /** Rótulo do dia já escolhido (passo 3). Ausente no passo 2. */
  dia?: string;
  /** Volta ao passo anterior pela navegação EXISTENTE (limpa seleções posteriores). */
  onTrocar: () => void;
}

/**
 * A faixa-resumo do que já foi escolhido nos passos 2 e 3 do fluxo mobile (mock
 * `Agendar`): avatar de iniciais + o resumo + a ação "Trocar". Presentacional puro —
 * o `onTrocar` é a mesma navegação de passos da página (nada de estado novo aqui).
 */
export function ResumoBar({ nome, dia, onTrocar }: ResumoBarProps) {
  return (
    <div className="flex items-center gap-2.5 rounded-md border border-primary-100 bg-white px-3.5 py-2.5">
      <Avatar name={nome} size="sm" />
      <span className="flex-1 text-sm text-ink first-letter:uppercase">
        {dia ? (
          <>
            <strong className="text-primary-300">{dia}</strong> · com {nome}
          </>
        ) : (
          <>
            com <strong className="text-primary-300">{nome}</strong>
          </>
        )}
      </span>
      <button
        type="button"
        onClick={onTrocar}
        className="shrink-0 rounded-sm text-sm font-bold text-primary-300 transition hover:text-accent-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
      >
        Trocar
      </button>
    </div>
  );
}
