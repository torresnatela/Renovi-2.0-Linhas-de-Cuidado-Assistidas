import { Link } from 'react-router-dom';

import { HealthBadge } from '../health/HealthBadge';
import { useLogout, useSession } from '../auth/useSession';

/**
 * Tela pós-login. Hoje só confirma a sessão; é aqui que a Minha Jornada entra
 * quando o motor de elegibilidade existir (ver docs/PROGRESSO.md).
 */
export function HomePage() {
  const session = useSession();
  const logout = useLogout();

  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <section className="rounded-lg border border-slate-200 bg-white p-6">
        <div className="mb-4 flex items-start justify-between gap-4">
          <div>
            <h2 className="text-lg font-medium">Olá, {session.data?.full_name}</h2>
            <p className="text-sm text-slate-600">{session.data?.email}</p>
          </div>
          <button
            onClick={() => logout.mutate()}
            disabled={logout.isPending}
            className="rounded border px-3 py-1 text-sm disabled:opacity-60"
          >
            {logout.isPending ? 'Saindo…' : 'Sair'}
          </button>
        </div>

        <p className="mb-4 text-sm text-slate-600">
          Sua conta está criada e vinculada à Doutor ao Vivo. Suas linhas de cuidado aparecem aqui
          assim que forem liberadas.
        </p>

        <div className="mb-6 flex flex-wrap gap-3">
          <Link
            to="/jornada"
            className="rounded bg-emerald-700 px-4 py-2 text-sm font-medium text-white"
          >
            Minha jornada
          </Link>
          <Link to="/agendar" className="rounded border px-4 py-2 text-sm">
            Agendar consulta
          </Link>
          <Link to="/consultas" className="rounded border px-4 py-2 text-sm">
            Minhas consultas
          </Link>
        </div>

        <HealthBadge />
      </section>
    </main>
  );
}
