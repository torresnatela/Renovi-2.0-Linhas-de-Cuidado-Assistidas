import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../../shared/api';
import { LoginPage } from './LoginPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getMe: vi.fn(), login: vi.fn() };
});
const api = await import('../../shared/api');

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    vi.mocked(api.login).mockReset();
  });

  it('mascara o CPF e submete cpf + senha', async () => {
    const user = userEvent.setup();
    vi.mocked(api.login).mockResolvedValue({
      id: 'id',
      full_name: 'Roberval',
      email: 'roberval@example.com',
    });
    renderPage();

    const cpf = screen.getByLabelText(/CPF/i);
    await user.type(cpf, '94819089846');
    expect(cpf).toHaveValue('948.190.898-46');

    await user.type(screen.getByLabelText(/Senha/i), 'cavalo-bateria-grampo');
    await user.click(screen.getByRole('button', { name: /^Entrar/i }));

    await waitFor(() => expect(api.login).toHaveBeenCalledOnce());
    expect(vi.mocked(api.login).mock.calls[0]).toEqual([
      '948.190.898-46',
      'cavalo-bateria-grampo',
    ]);
  });

  it('mostra a mensagem de erro de credencial em role="alert"', async () => {
    const user = userEvent.setup();
    vi.mocked(api.login).mockRejectedValue(
      new ApiError(401, 'credenciais inválidas', 'CPF ou senha incorretos'),
    );
    renderPage();

    await user.type(screen.getByLabelText(/CPF/i), '94819089846');
    await user.type(screen.getByLabelText(/Senha/i), 'senha-qualquer-longa');
    await user.click(screen.getByRole('button', { name: /^Entrar/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/CPF ou senha incorretos/i);
  });

  it('leva ao cadastro pelo link', () => {
    renderPage();
    expect(screen.getByRole('link', { name: /Crie sua conta/i })).toHaveAttribute(
      'href',
      '/cadastro',
    );
  });
});
