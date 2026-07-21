import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';

import { Card } from '../../shared/ui/Card';
import { HelpPill } from '../../shared/ui/HelpPill';
import { useHelpNow } from './useMood';

/**
 * "Pedir ajuda" do header (DESIGN-SYSTEM §4.4): afordância permanente para o
 * canal clínico de urgência. Guardrail de produto: sem `confirm()`, sem fricção —
 * UM clique dispara a API e o popover mostra o canal retornado. O erro vira uma
 * mensagem neutra (nunca vermelho de alarme): quem precisa de ajuda não pode
 * bater num beco sem saída.
 */
export function HelpNowMenu() {
  const help = useHelpNow();
  const [open, setOpen] = useState(false);
  const location = useLocation();

  // Um popover aberto não sobrevive à navegação: trocar de rota fecha.
  useEffect(() => {
    setOpen(false);
  }, [location.pathname]);

  // Escape fecha (padrão de popover): quem abriu por engano sai sem o mouse.
  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);

  function pedirAjuda() {
    setOpen(true);
    help.mutate();
  }

  return (
    <div className="relative">
      <HelpPill onClick={pedirAjuda} busy={help.isPending} />

      {open && (
        <Card className="absolute right-0 top-full z-40 mt-2 w-72">
          <div className="flex items-start justify-between gap-3">
            <div role="status" className="text-sm">
              {help.isPending && <p className="text-muted">Conectando você ao canal…</p>}
              {help.isError && (
                <p className="text-ink">Não foi possível agora; tente novamente.</p>
              )}
              {help.data && (
                <>
                  <p className="font-bold text-primary-300">{help.data.label}</p>
                  <p className="mt-1 text-ink">{help.data.message}</p>
                </>
              )}
            </div>
            <button
              type="button"
              aria-label="Fechar"
              onClick={() => setOpen(false)}
              className="shrink-0 rounded-sm p-1 text-muted transition hover:bg-page focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
            >
              <svg
                width={18}
                height={18}
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1.8}
                strokeLinecap="round"
                aria-hidden="true"
              >
                <path d="M6 6l12 12M18 6L6 18" />
              </svg>
            </button>
          </div>
        </Card>
      )}
    </div>
  );
}
