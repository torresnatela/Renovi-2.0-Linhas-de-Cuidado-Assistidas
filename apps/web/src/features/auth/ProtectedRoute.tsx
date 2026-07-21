import { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';

import { useSession } from './useSession';

/**
 * ProtectedRoute segura a tela até saber se há sessão.
 *
 * O estado de carregando NÃO é opcional: sem ele, a primeira renderização (com
 * `data` ainda undefined) redirecionaria para o login todo mundo que já está
 * logado, e a sessão só é conhecida depois do GET /me — o cookie é httpOnly e
 * o JS não consegue espiá-lo.
 */
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const session = useSession();

  if (session.isLoading) {
    return <p className="p-6 text-sm text-muted">Carregando…</p>;
  }
  if (!session.data) {
    return <Navigate to="/entrar" replace />;
  }
  return <>{children}</>;
}
