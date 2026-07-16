import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { LoginPage } from './features/auth/LoginPage';
import { ProtectedRoute } from './features/auth/ProtectedRoute';
import { RegisterPage } from './features/auth/RegisterPage';
import { HomePage } from './features/home/HomePage';

/**
 * Shell do app. As telas restantes do MVP (Minha Jornada, Agendar, Minha
 * Consulta — SPEC §7) entram como rotas protegidas ao lado de HomePage.
 */
export default function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-slate-50 text-slate-900">
        <header className="border-b border-slate-200 bg-white">
          <div className="mx-auto max-w-3xl px-6 py-4">
            <h1 className="text-xl font-semibold">Renovi 2.0</h1>
            <p className="text-sm text-slate-500">Plataforma do Paciente — Linhas de Cuidado</p>
          </div>
        </header>

        <Routes>
          <Route path="/entrar" element={<LoginPage />} />
          <Route path="/cadastro" element={<RegisterPage />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <HomePage />
              </ProtectedRoute>
            }
          />
        </Routes>
      </div>
    </BrowserRouter>
  );
}
