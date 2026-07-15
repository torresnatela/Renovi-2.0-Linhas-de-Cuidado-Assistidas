import { HealthBadge } from './features/health/HealthBadge';

/**
 * Shell mínimo do app (fundação). O mapa de telas do MVP (Ativação/Login,
 * Minha Jornada, Agendar, Minha Consulta — SPEC §7) entra nas fases seguintes.
 */
export default function App() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto max-w-3xl px-6 py-4">
          <h1 className="text-xl font-semibold">Renovi 2.0</h1>
          <p className="text-sm text-slate-500">Plataforma do Paciente — Linhas de Cuidado</p>
        </div>
      </header>

      <main className="mx-auto max-w-3xl px-6 py-10">
        <section className="rounded-lg border border-slate-200 bg-white p-6">
          <h2 className="mb-2 text-lg font-medium">Fundação</h2>
          <p className="mb-4 text-sm text-slate-600">
            Estrutura inicial pronta. As telas do MVP (Minha Jornada, Agendar,
            Minha Consulta) serão construídas sobre esta base.
          </p>
          <HealthBadge />
        </section>
      </main>
    </div>
  );
}
