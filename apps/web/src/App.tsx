import { BrowserRouter, Link, Outlet, Route, Routes } from 'react-router-dom';

import { LoginPage } from './features/auth/LoginPage';
import { ProtectedRoute } from './features/auth/ProtectedRoute';
import { RegisterPage } from './features/auth/RegisterPage';
import { HomePage } from './features/home/HomePage';
import { MoodPage } from './features/mood/MoodPage';
import { AppointmentPage, AppointmentsPage } from './features/scheduling/AppointmentsPage';
import { ProfessionalPickerPage, SpecialtyPickerPage } from './features/scheduling/SchedulingPages';
import { SlotPickerPage } from './features/scheduling/SlotPickerPage';
import { ErrorBoundary } from './shared/ErrorBoundary';

/**
 * Shell do app. A Minha Jornada (SPEC §7) entra ao lado destas quando o motor de
 * elegibilidade existir (ver docs/PROGRESSO.md).
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

        {/*
          ErrorBoundary: um erro síncrono no render de qualquer tela (ex.: um
          time_zone malformado da API fazendo toLocaleTimeString lançar) deixaria
          o app inteiro em branco. O boundary vira isso numa mensagem com saída.
        */}
        <ErrorBoundary>
          <Routes>
            <Route path="/entrar" element={<LoginPage />} />
            <Route path="/cadastro" element={<RegisterPage />} />

            {/*
              Rota de layout sem path: a guarda de sessão fica em UM lugar em vez
              de envolver cada uma das seis telas — e envolver seis vezes é como
              uma acaba esquecida.
            */}
            <Route
              element={
                <ProtectedRoute>
                  <Outlet />
                </ProtectedRoute>
              }
            >
              <Route path="/" element={<HomePage />} />

              {/*
                A URL É o estado do wizard: voltar, recarregar e compartilhar
                funcionam de graça, e cada passo é testável isolado.
              */}
              <Route path="/agendar" element={<SpecialtyPickerPage />} />
              <Route path="/agendar/:specialtyId" element={<ProfessionalPickerPage />} />
              <Route path="/agendar/:specialtyId/:professionalId" element={<SlotPickerPage />} />

              <Route path="/consultas" element={<AppointmentsPage />} />
              <Route path="/consultas/:appointmentId" element={<AppointmentPage />} />

              {/* Verificador Diário de Humor (Anexo C). */}
              <Route path="/humor" element={<MoodPage />} />
            </Route>

            <Route path="*" element={<NaoEncontrada />} />
          </Routes>
        </ErrorBoundary>
      </div>
    </BrowserRouter>
  );
}

/**
 * Esta feature levou o app de 3 para 8 rotas e trouxe o primeiro link de verdade
 * compartilhável (/consultas/:id). Sem isto, um id errado renderiza uma página em
 * branco sob o cabeçalho.
 */
function NaoEncontrada() {
  return (
    <main className="mx-auto max-w-3xl px-6 py-10">
      <h2 className="mb-2 text-lg font-medium">Página não encontrada</h2>
      <Link to="/" className="text-emerald-700 underline">
        Voltar ao início
      </Link>
    </main>
  );
}
