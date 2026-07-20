import { IconHelpTarget } from './icons';

const cx = (...c: Array<string | false | null | undefined>) => c.filter(Boolean).join(' ');

/**
 * A pill "Pedir ajuda" do header — presente sempre no mesmo lugar (princípio de
 * UX §4.4). Discreta em navy: distinta do CTA laranja e do vermelho de erro, para
 * não competir com eles nem parecer alarme. Roteia para canal clínico, nunca
 * engajamento — mas a rota é de quem usa (o `onClick`); aqui é só a aparência.
 *
 * `busy` cobre a espera de quem clicou (abrir o canal pode ser assíncrono):
 * desabilita e troca o alvo por um spinner discreto, sem tirar o rótulo — spinner
 * mudo faz o usuário reclicar.
 */
function Spinner() {
  return (
    <svg
      data-testid="help-spinner"
      className="animate-spin"
      width={16}
      height={16}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.3" strokeWidth="2.5" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  );
}

export function HelpPill({ onClick, busy = false }: { onClick: () => void; busy?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy}
      aria-busy={busy || undefined}
      className={cx(
        'inline-flex items-center gap-2 whitespace-nowrap rounded-pill border border-primary-200 bg-white',
        'px-4 py-2.5 text-[13.5px] font-bold text-primary-300 shadow-card transition',
        'hover:bg-primary-100 active:opacity-70 disabled:cursor-not-allowed',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300',
      )}
    >
      {busy ? <Spinner /> : <IconHelpTarget size={18} />}
      Pedir ajuda
    </button>
  );
}
