import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../../shared/api';
import { LoginPage } from './LoginPage';
import { ProtectedRoute } from './ProtectedRoute';
import { useLogout, useSession } from './useSession';

/**
 * Fixture mínima de tela logada. A HomePage foi aposentada no redesign (Etapa 8);
 * o que este teste trava é a SESSÃO — depois do logout, nenhum dado do paciente
 * pode sobreviver na tela. Uma tela enxuta preserva exatamente essa intenção.
 */
function ContaFixture() {
  const session = useSession();
  const logout = useLogout();
  if (!session.data) return <p>sem sessão</p>;
  return (
    <div>
      <p>Olá, {session.data.full_name}</p>
      <p>{session.data.email}</p>
      <button onClick={() => logout.mutate()}>Sair</button>
    </div>
  );
}

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getMe: vi.fn(), getHealth: vi.fn(), logout: vi.fn(), login: vi.fn() };
});
const api = await import('../../shared/api');

const conta = {
  id: '019f6c75-1ec9-7a93-b852-66a70d765ca6',
  full_name: 'Roberval Juvencio Lazaroti',
  email: 'roberval@example.com',
};

function renderApp() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/entrar" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <ContaFixture />
              </ProtectedRoute>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('logout', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockReset();
    vi.mocked(api.logout).mockReset();
    vi.mocked(api.getHealth).mockResolvedValue({ status: 'ok', service: 'renovi-care', version: 'dev' });
  });

  // O ponto sensível: depois do logout a home NÃO pode continuar mostrando o
  // nome e o e-mail do paciente. É dado de quem acabou de sair, num browser que
  // pode ser compartilhado.
  it('some com os dados do paciente da tela assim que o logout conclui', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getMe).mockResolvedValue(conta);
    vi.mocked(api.logout).mockResolvedValue(undefined);
    renderApp();

    expect(await screen.findByText(/Olá, Roberval/)).toBeInTheDocument();

    // Depois do logout o servidor passa a recusar a sessão.
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    await user.click(screen.getByRole('button', { name: /Sair/i }));

    await waitFor(() => {
      expect(screen.queryByText(/Olá, Roberval/)).not.toBeInTheDocument();
      expect(screen.queryByText(conta.email)).not.toBeInTheDocument();
    });
  });
});
