import { useQuery } from '@tanstack/react-query';
import { getHealth } from '../../shared/api';

/**
 * HealthBadge demonstra o caminho front -> API (TanStack Query + fetch com
 * proxy /api). É um exemplo do padrão de feature; as features reais do MVP
 * (jornada, agendamento) seguem esta estrutura em src/features/.
 */
export function HealthBadge() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['health'],
    queryFn: getHealth,
    retry: false,
  });

  let label = 'Verificando API…';
  let dot = 'bg-amber-400';

  if (isError) {
    label = 'API indisponível (suba com `make up` + `go run ./cmd/api`)';
    dot = 'bg-red-500';
  } else if (!isLoading && data) {
    label = `API ${data.status} · ${data.service} ${data.version}`;
    dot = 'bg-emerald-500';
  }

  return (
    <div className="inline-flex items-center gap-2 rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-sm">
      <span className={`h-2 w-2 rounded-full ${dot}`} aria-hidden />
      <span>{label}</span>
    </div>
  );
}
