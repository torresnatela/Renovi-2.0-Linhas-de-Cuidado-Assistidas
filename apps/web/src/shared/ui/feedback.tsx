import { ApiError } from '../api';
import { Button } from './Button';
import { IconClock } from './icons';

/**
 * Estados transversais de carregamento, vazio e erro.
 *
 * Porta a semântica de `features/scheduling/ui.tsx`: indisponibilidade temporária
 * (503 ou `reason.code` LEGACY_UNAVAILABLE) NÃO é erro do usuário — é tom
 * informativo âmbar, "tente de novo em instantes". Erro real é vermelho. E
 * `Empty` (vazio) ≠ erro: nem alerta, nem vermelho.
 */

export function Loading({ label = 'Carregando…' }: { label?: string }) {
  return (
    <p role="status" className="flex items-center gap-2 text-sm text-muted">
      <span
        className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-200 border-t-primary-300"
        aria-hidden="true"
      />
      {label}
    </p>
  );
}

export function Empty({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-primary-100 bg-white p-8 text-center">
      <p className="font-bold text-ink">{title}</p>
      {hint && <p className="mt-1 text-sm text-muted">{hint}</p>}
    </div>
  );
}

const UNAVAILABLE_TEXT =
  'Não conseguimos consultar a agenda agora. Isso é um problema nosso, não seu — tente de novo em instantes.';

function isUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 503 || error.reason?.code === 'LEGACY_UNAVAILABLE')
  );
}

export function ErrorNotice({ error, retry }: { error: unknown; retry?: () => void }) {
  const unavailable = isUnavailable(error);
  const message = unavailable
    ? UNAVAILABLE_TEXT
    : error instanceof Error
      ? error.message
      : 'Não foi possível carregar.';

  return (
    <div
      // Indisponibilidade é informativa (status); erro real é alerta.
      role={unavailable ? 'status' : 'alert'}
      className={
        unavailable
          ? 'flex items-start gap-2 rounded-md border border-accent-200 bg-[rgba(250,143,27,0.1)] p-4 text-sm text-ink'
          : 'flex items-start gap-2 rounded-md bg-[rgba(205,25,25,0.08)] p-4 text-sm text-error'
      }
    >
      {unavailable && (
        <span className="mt-0.5 shrink-0 text-accent-300">
          <IconClock size={18} />
        </span>
      )}
      <div className="flex flex-col gap-2">
        <span>{message}</span>
        {retry && (
          <span>
            <Button color="ghost" size="sm" onClick={retry}>
              Tentar de novo
            </Button>
          </span>
        )}
      </div>
    </div>
  );
}
