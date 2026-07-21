import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { ApiError } from './shared/api';

vi.mock('./shared/api', async () => {
  const actual = await vi.importActual<typeof import('./shared/api')>('./shared/api');
  return { ...actual, getMe: vi.fn(), getJourney: vi.fn() };
});
const api = await import('./shared/api');

const conta = {
  id: '019f6c75-1ec9-7a93-b852-66a70d765ca6',
  full_name: 'Roberval Juvencio Lazaroti',
  email: 'roberval@example.com',
};

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
    vi.mocked(api.getJourney).mockReset();
    // A raiz redireciona para a Jornada, que consome getJourney: mock inócuo.
    vi.mocked(api.getJourney).mockResolvedValue({ enrollments: [] });
    window.history.pushState({}, '', '/');
  });

  // Sem sessão, nenhuma tela do paciente pode aparecer nem por um instante.
  it('manda quem não tem sessão para o login', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    renderApp();

    expect(await screen.findByRole('heading', { level: 1, name: 'Entrar' })).toBeInTheDocument();
    expect(screen.queryByText(/Olá,/)).not.toBeInTheDocument();
  });

  // A home foi aposentada: a raiz agora cai na Jornada (dentro do shell).
  it('redireciona a raiz para a Jornada quando há sessão', async () => {
    vi.mocked(api.getMe).mockResolvedValue(conta);
    renderApp();

    expect(await screen.findByRole('heading', { name: 'Minha jornada' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/jornada');
  });

  // O antigo cabeçalho "Renovi 2.0" deu lugar ao shell com a navegação principal.
  it('renderiza o shell do produto com a navegação principal', async () => {
    vi.mocked(api.getMe).mockResolvedValue(conta);
    renderApp();

    expect(await screen.findByRole('navigation')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Jornada' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Consultas' })).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Perfil' }).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Pedir ajuda' })).toBeInTheDocument();
  });
});
