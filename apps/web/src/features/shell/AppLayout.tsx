import { Outlet } from 'react-router-dom';

import { AppShell } from '../../shared/ui/AppShell';
import { useSession } from '../auth/useSession';
import { HelpNowMenu } from '../mood/HelpNowMenu';

/** Trata por "você" + primeiro nome (DESIGN-SYSTEM §4.7). */
function primeiroNome(nomeCompleto: string | undefined): string {
  if (!nomeCompleto) return '';
  return nomeCompleto.trim().split(/\s+/)[0] ?? '';
}

/**
 * O wiring do shell: liga a sessão (nome do usuário) e a afordância de ajuda ao
 * AppShell presentacional. Enquanto o nome não carrega, o shell renderiza com o
 * avatar vazio — o `ProtectedRoute` já garante que só chegamos aqui logados.
 */
export function AppLayout() {
  const session = useSession();

  return (
    <AppShell userName={primeiroNome(session.data?.full_name)} help={<HelpNowMenu />}>
      <Outlet />
    </AppShell>
  );
}
