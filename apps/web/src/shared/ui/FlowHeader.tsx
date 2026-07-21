import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

import logoIcon from '../../assets/logos/logo-icon.svg';
import { IconBack } from './icons';

interface FlowHeaderProps {
  /** Rótulo de contexto acima do título — renderizado UPPERCASE via classe. */
  eyebrow: string;
  /** Título do passo/fluxo — o `<h1>` do fluxo empilhado (20px bold navy). */
  title: string;
  /** Destino do botão voltar como `<Link>`. Use OU `backTo` OU `onBack`. */
  backTo?: string;
  /** Handler do botão voltar como `<button>`. Use OU `onBack` OU `backTo`. */
  onBack?: () => void;
  /** Slot da afordância de ajuda (HelpNowMenu) à direita. */
  help?: ReactNode;
  /** Barra de progresso opcional (trilho + caption). `pct` de 0 a 100. */
  progress?: { pct: number; caption: string };
}

/**
 * Header dos fluxos empilhados no mobile (< lg) — mock `Agendar`. Presentacional
 * puro: a página decide `backTo` (Link) vs. `onBack` (button) — exatamente um —,
 * passa o slot de ajuda e, opcionalmente, o progresso.
 */
export function FlowHeader({ eyebrow, title, backTo, onBack, help, progress }: FlowHeaderProps) {
  // Botão voltar circular de 36px (mock): press esmaece via `active:opacity-70`,
  // padrão do app. Compartilhado entre a variante Link e a variante button.
  const backClass =
    'inline-flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full border border-primary-200 bg-white text-primary-300 transition active:opacity-70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300';

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3 pt-2">
        {backTo ? (
          <Link to={backTo} aria-label="Voltar" className={backClass}>
            <IconBack size={16} />
          </Link>
        ) : (
          <button type="button" aria-label="Voltar" onClick={onBack} className={backClass}>
            <IconBack size={16} />
          </button>
        )}

        <div className="flex min-w-0 flex-1 flex-col">
          <img src={logoIcon} alt="Renovi" className="mb-0.5 h-3.5 w-auto self-start" />
          <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">
            {eyebrow}
          </span>
          {/* O título do fluxo é o `<h1>` da tela no mobile: cada fluxo empilhado
              precisa de exatamente um h1, e é aqui que ele mora (a a11y de heading
              não pode depender de a página lembrar de criar o seu). */}
          <h1 className="text-xl font-bold text-primary-300">{title}</h1>
        </div>

        {help}
      </div>

      {progress && (
        <div className="flex flex-col gap-1.5">
          {/* Trilho decorativo: role progressbar + aria-valuenow são bem-vindos,
              sem inventar semântica além disso. Largura dinâmica via style inline. */}
          <div className="h-1.5 overflow-hidden rounded-pill bg-primary-100">
            <div
              role="progressbar"
              aria-valuenow={progress.pct}
              aria-valuemin={0}
              aria-valuemax={100}
              className="h-full rounded-pill bg-primary-300"
              style={{ width: `${progress.pct}%` }}
            />
          </div>
          <span className="text-xs text-muted">{progress.caption}</span>
        </div>
      )}
    </div>
  );
}
