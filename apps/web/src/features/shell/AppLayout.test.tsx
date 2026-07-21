import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AppLayout } from './AppLayout';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getMe: vi.fn(), moodHelpNow: vi.fn() };
});
const api = await import('../../shared/api');

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/jornada" element={<p>tela jornada</p>} />
            <Route path="/consultas" element={<p>tela consultas</p>} />
            <Route path="/perfil" element={<p>tela perfil</p>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AppLayout', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockResolvedValue({
      id: 'u1',
      full_name: 'Ana Beatriz Souza',
      email: 'ana@example.com',
    });
    vi.mocked(api.moodHelpNow).mockReset();
  });

  it('marca o link ativo conforme a rota', async () => {
    renderAt('/consultas');
    expect(await screen.findByText('tela consultas')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Jornada' })).not.toHaveAttribute('aria-current');
  });

  it('pede ajuda em um clique e exibe o canal retornado', async () => {
    vi.mocked(api.moodHelpNow).mockResolvedValue({
      type: 'phone',
      label: 'Central de Cuidado',
      message: 'Ligue 0800 123 4567 agora.',
    });
    const user = userEvent.setup();
    renderAt('/jornada');

    await user.click(await screen.findByRole('button', { name: 'Pedir ajuda' }));

    expect(await screen.findByText('Ligue 0800 123 4567 agora.')).toBeInTheDocument();
    expect(screen.getByText('Central de Cuidado')).toBeInTheDocument();
    // Guardrail de produto: canal clínico de urgência é 1 clique, sem confirm().
    expect(api.moodHelpNow).toHaveBeenCalledTimes(1);
  });

  it('exibe mensagem neutra quando o canal falha', async () => {
    vi.mocked(api.moodHelpNow).mockRejectedValue(new Error('boom'));
    const user = userEvent.setup();
    renderAt('/jornada');

    await user.click(await screen.findByRole('button', { name: 'Pedir ajuda' }));

    expect(await screen.findByText(/Não foi possível agora; tente novamente\./)).toBeInTheDocument();
  });

  it('leva ao perfil pelo avatar', async () => {
    renderAt('/jornada');
    // O avatar só ganha nome quando a sessão resolve (fallback vazio antes disso).
    const avatar = await screen.findByRole('img', { name: 'Ana' });
    expect(avatar.closest('a')).toHaveAttribute('href', '/perfil');
  });
});
