import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../../shared/api';
import { RegisterPage } from './RegisterPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getMe: vi.fn(), registerAccount: vi.fn() };
});
const api = await import('../../shared/api');

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <RegisterPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function preencherFormulario(user: ReturnType<typeof userEvent.setup>, senha = 'cavalo-bateria-grampo', confirmacao = senha) {
  await user.type(screen.getByLabelText(/Nome completo/i), 'Roberval Juvencio Lazaroti');
  await user.type(screen.getByLabelText(/^CPF/i), '948.190.898-46');
  await user.type(screen.getByLabelText(/Data de nascimento/i), '1976-01-23');
  await user.type(screen.getByLabelText(/E-mail/i), 'roberval@example.com');
  await user.type(screen.getByLabelText(/Celular/i), '11912345678');
  await user.type(screen.getByLabelText(/CEP/i), '06472000');
  await user.type(screen.getByLabelText(/Logradouro/i), 'Avenida Copacabana');
  await user.type(screen.getByLabelText(/Número/i), '238');
  await user.type(screen.getByLabelText(/Bairro/i), 'Dezoito do Forte');
  await user.type(screen.getByLabelText(/Cidade/i), 'Barueri');
  await user.type(screen.getByLabelText(/UF/i), 'SP');
  await user.type(screen.getByLabelText(/^Senha \(/i), senha);
  await user.type(screen.getByLabelText(/Confirme a senha/i), confirmacao);
}

describe('RegisterPage', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    vi.mocked(api.registerAccount).mockReset();
  });

  it('envia o cadastro com os dados preenchidos', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockResolvedValue({
      id: 'id', full_name: 'Roberval Juvencio Lazaroti', email: 'roberval@example.com',
    });
    renderPage();

    await preencherFormulario(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    await waitFor(() => expect(api.registerAccount).toHaveBeenCalledOnce());
    expect(vi.mocked(api.registerAccount).mock.calls[0][0]).toMatchObject({
      cpf: '948.190.898-46',
      email: 'roberval@example.com',
      address: { city: 'Barueri', state: 'SP' },
    });
  });

  // A espera é longa e real (o cadastro é síncrono contra a Doutor ao Vivo).
  // Sem aviso, o usuário acha que travou e recarrega no meio.
  it('avisa que a espera é longa enquanto o cadastro corre', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockImplementation(() => new Promise(() => {})); // nunca resolve
    renderPage();

    await preencherFormulario(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    const aviso = await screen.findByRole('status');
    expect(aviso).toHaveTextContent(/pode levar até um minuto/i);
    expect(aviso).toHaveTextContent(/não feche nem recarregue/i);
  });

  it('barra senhas divergentes sem chamar a API', async () => {
    const user = userEvent.setup();
    renderPage();

    await preencherFormulario(user, 'cavalo-bateria-grampo', 'outra-senha-qualquer');
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/não conferem/i);
    expect(api.registerAccount).not.toHaveBeenCalled();
  });

  it('barra senha curta sem chamar a API', async () => {
    const user = userEvent.setup();
    renderPage();

    await preencherFormulario(user, 'curta');
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/12 caracteres/i);
    expect(api.registerAccount).not.toHaveBeenCalled();
  });

  // O 409 de e-mail já usado na DAV precisa chegar ao usuário com a frase que a
  // API escreveu — é o caso do casal que compartilha e-mail, e um "erro
  // inesperado" genérico o deixaria sem saber o que fazer.
  it('mostra a mensagem da API quando o e-mail já está em uso', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockRejectedValue(
      new ApiError(409, 'e-mail indisponível',
        'este e-mail já está vinculado a outro paciente. Use um e-mail pessoal, não compartilhado.'),
    );
    renderPage();

    await preencherFormulario(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/já está vinculado a outro paciente/i);
  });
});
