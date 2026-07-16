import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { ApiError } from './shared/api';

vi.mock('./shared/api', async () => {
  const actual = await vi.importActual<typeof import('./shared/api')>('./shared/api');
  return { ...actual, getMe: vi.fn(), getHealth: vi.fn() };
});
const api = await import('./shared/api');

function renderApp() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
}

describe('App', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockReset();
    vi.mocked(api.getHealth).mockReset();
    window.history.pushState({}, '', '/');
  });

  it('renderiza o cabeçalho do produto', () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    renderApp();
    expect(screen.getByRole('heading', { level: 1, name: 'Renovi 2.0' })).toBeInTheDocument();
  });

  // Sem sessão, a home não pode aparecer nem por um instante: ela mostra dados
  // do paciente.
  it('manda quem não tem sessão para o login', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    renderApp();

    expect(await screen.findByRole('heading', { level: 1, name: 'Entrar' })).toBeInTheDocument();
    expect(screen.queryByText(/Olá,/)).not.toBeInTheDocument();
  });

  it('mostra a home para quem tem sessão', async () => {
    vi.mocked(api.getMe).mockResolvedValue({
      id: '019f6c75-1ec9-7a93-b852-66a70d765ca6',
      full_name: 'Roberval Juvencio Lazaroti',
      email: 'roberval@example.com',
    });
    vi.mocked(api.getHealth).mockResolvedValue({ status: 'ok', service: 'renovi-care', version: 'dev' });
    renderApp();

    expect(await screen.findByText(/Olá, Roberval/)).toBeInTheDocument();
  });
});
