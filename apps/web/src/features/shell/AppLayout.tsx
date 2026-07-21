import { Outlet, matchPath, useLocation } from 'react-router-dom';

import { AppShell } from '../../shared/ui/AppShell';
import { useSession } from '../auth/useSession';
import { HelpNowMenu } from '../mood/HelpNowMenu';

/** Trata por "você" + primeiro nome (DESIGN-SYSTEM §4.7). */
function primeiroNome(nomeCompleto: string | undefined): string {
  if (!nomeCompleto) return '';
  return nomeCompleto.trim().split(/\s+/)[0] ?? '';
}

/**
 * Fluxos empilhados no mobile: telas de tarefa focada (Agendar, detalhe da
 * consulta, avaliação) que trocam a tab bar pelo próprio FlowHeader — foco na
 * tarefa, sem navegação lateral competindo (DS §4). Cada telas RAIZ (/jornada,
 * /consultas, /perfil) mantém a tab bar.
 */
const ROTAS_DE_FLUXO = [
  '/jornada/agendar/:itemId',
  '/consultas/:appointmentId',
  '/avaliacoes/:codigo',
];

/**
 * O wiring do shell: liga a sessão (nome do usuário) e a afordância de ajuda ao
 * AppShell presentacional. Enquanto o nome não carrega, o shell renderiza com o
 * avatar vazio — o `ProtectedRoute` já garante que só chegamos aqui logados.
 *
 * No mobile, escolhe o chrome pela rota (tabs vs. flow); no desktop o AppShell
 * ignora `mobileVariant` e nada muda.
 */
export function AppLayout() {
  const session = useSession();
  const location = useLocation();

  const emFluxo = ROTAS_DE_FLUXO.some((padrao) => matchPath(padrao, location.pathname) !== null);

  return (
    <AppShell
      userName={primeiroNome(session.data?.full_name)}
      help={<HelpNowMenu />}
      mobileVariant={emFluxo ? 'flow' : 'tabs'}
    >
      <Outlet />
    </AppShell>
  );
}
