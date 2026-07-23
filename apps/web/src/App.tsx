import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom';

import { LoginPage } from './features/auth/LoginPage';
import { ProtectedRoute } from './features/auth/ProtectedRoute';
import { RegisterPage } from './features/auth/RegisterPage';
import { OnboardingPage } from './features/onboarding/OnboardingPage';
import { ConsultationsPage } from './features/consultations/ConsultationsPage';
import { JourneyPage } from './features/journey/JourneyPage';
import { ScheduleCarePage } from './features/journey/ScheduleCarePage';
import { AssessmentPage } from './features/mood/AssessmentPage';
import { ProfilePage } from './features/profile/ProfilePage';
import { AppLayout } from './features/shell/AppLayout';
import { AppointmentPage } from './features/scheduling/AppointmentPage';
import { ErrorBoundary } from './shared/ErrorBoundary';
import { Button } from './shared/ui/Button';
import { Card } from './shared/ui/Card';

/**
 * Tabela de rotas final do app do paciente. As telas logadas vivem sob um único
 * layout (`AppLayout`) que traz o shell desktop (top nav) e a guarda de sessão —
 * envolver cada tela seria repetir a guarda seis vezes, e uma acaba esquecida.
 * `/entrar` e `/cadastro` ficam fora do shell (a Etapa 3 traz o layout próprio).
 */
export default function App() {
  return (
    <BrowserRouter>
      {/*
        ErrorBoundary: um erro síncrono no render de qualquer tela (ex.: um
        time_zone malformado da API fazendo toLocaleTimeString lançar) deixaria o
        app inteiro em branco. O boundary vira isso numa mensagem com saída.
      */}
      <ErrorBoundary>
        <Routes>
          <Route path="/entrar" element={<LoginPage />} />
          <Route path="/cadastro" element={<RegisterPage />} />
          {/* Onboarding pelo convite da Gestão: público (o token na URL é a
              credencial), fora do shell — cadastro pré-preenchido + confirmação
              da empresa (ADR-044). */}
          <Route path="/onboarding/:token" element={<OnboardingPage />} />

          {/* Rota de layout: shell + guarda de sessão em UM lugar. */}
          <Route
            element={
              <ProtectedRoute>
                <AppLayout />
              </ProtectedRoute>
            }
          >
            <Route path="/" element={<Navigate to="/jornada" replace />} />

            {/* Minha jornada (SPEC §7): a linha de cuidado do paciente. Agendar
                é por ITEM da linha (passa pelo motor de elegibilidade). */}
            <Route path="/jornada" element={<JourneyPage />} />
            <Route path="/jornada/agendar/:itemId" element={<ScheduleCarePage />} />
            {/* Consultas migraram para /consultas: mantém links antigos vivos. */}
            <Route path="/jornada/consultas" element={<Navigate to="/consultas" replace />} />

            <Route path="/consultas" element={<ConsultationsPage />} />
            <Route path="/consultas/:appointmentId" element={<AppointmentPage />} />

            <Route path="/perfil" element={<ProfilePage />} />

            {/* /humor aposentado no redesign: o check-in de humor virou o card da
                Jornada (MoodCheckinCard). Mantém links antigos vivos. */}
            <Route path="/humor" element={<Navigate to="/jornada" replace />} />
            {/* Instrumentos periódicos (WHO-5/PHQ-4) por link direto. */}
            <Route path="/avaliacoes/:codigo" element={<AssessmentPage />} />
          </Route>

          <Route path="*" element={<NaoEncontrada />} />
        </Routes>
      </ErrorBoundary>
    </BrowserRouter>
  );
}

/**
 * Um id ou endereço errado renderiza uma página com saída, nunca uma tela em
 * branco. Fica fora do shell (rota `*`): pode ser atingida sem sessão.
 */
function NaoEncontrada() {
  const navigate = useNavigate();
  return (
    <main className="mx-auto flex max-w-shell flex-col items-center px-10 py-16">
      <Card as="section" padding="lg" className="w-full max-w-md text-center">
        <h1 className="text-lg font-bold text-primary-300">Página não encontrada</h1>
        <p className="mt-2 text-sm text-ink">
          O endereço que você acessou não existe ou mudou de lugar.
        </p>
        <Button className="mt-6" onClick={() => navigate('/jornada')}>
          Voltar para a jornada
        </Button>
      </Card>
    </main>
  );
}
