import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { HelpNowMenu } from './HelpNowMenu';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, moodHelpNow: vi.fn() };
});
const api = await import('../../shared/api');

/** A menu + um gatilho de navegação, para exercitar o fechar-ao-trocar-de-rota. */
function Harness() {
  const navigate = useNavigate();
  return (
    <>
      <HelpNowMenu />
      <button onClick={() => navigate('/perfil')}>navegar</button>
    </>
  );
}

function renderMenu() {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/jornada']}>
        <Harness />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('HelpNowMenu', () => {
  beforeEach(() => {
    // Pendente de propósito: o popover fica aberto ("Conectando…") durante o teste.
    vi.mocked(api.moodHelpNow).mockImplementation(() => new Promise(() => {}));
  });

  it('fecha o popover com Escape', async () => {
    const user = userEvent.setup();
    renderMenu();

    await user.click(screen.getByRole('button', { name: /pedir ajuda/i }));
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('button', { name: 'Fechar' })).not.toBeInTheDocument();
  });

  it('fecha o popover ao trocar de rota', async () => {
    const user = userEvent.setup();
    renderMenu();

    await user.click(screen.getByRole('button', { name: /pedir ajuda/i }));
    expect(screen.getByRole('button', { name: 'Fechar' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'navegar' }));
    expect(screen.queryByRole('button', { name: 'Fechar' })).not.toBeInTheDocument();
  });
});
