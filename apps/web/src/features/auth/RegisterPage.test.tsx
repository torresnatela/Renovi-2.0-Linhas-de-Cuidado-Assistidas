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
// A ViaCEP é isolada e sempre mockada aqui: nenhum teste toca a rede.
vi.mock('./viaCep');

const api = await import('../../shared/api');
const viaCep = await import('./viaCep');

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

type User = ReturnType<typeof userEvent.setup>;

async function fillStep1(user: User) {
  await user.type(screen.getByLabelText(/Nome completo/i), 'Roberval Juvencio Lazaroti');
  await user.type(screen.getByLabelText(/CPF/i), '94819089846');
  await user.type(screen.getByLabelText(/Nascimento/i), '23011976');
  await user.type(screen.getByLabelText(/Celular/i), '11912345678');
  await user.click(screen.getByRole('button', { name: /Continuar/i }));
}

async function fillStep2(user: User) {
  await user.type(screen.getByLabelText(/CEP/i), '06472000');
  await user.type(screen.getByLabelText(/Endereço/i), 'Avenida Copacabana');
  await user.type(screen.getByLabelText(/Número/i), '238');
  await user.type(screen.getByLabelText(/Bairro/i), 'Dezoito do Forte');
  await user.type(screen.getByLabelText(/Cidade/i), 'Barueri');
  await user.type(screen.getByLabelText(/UF/i), 'SP');
  await user.click(screen.getByRole('button', { name: /Continuar/i }));
}

async function fillStep3(
  user: User,
  {
    senha = 'cavalo-bateria-grampo',
    senha2 = senha,
    consent = true,
  }: { senha?: string; senha2?: string; consent?: boolean } = {},
) {
  await user.type(screen.getByLabelText(/E-mail/i), 'roberval@example.com');
  await user.type(screen.getByLabelText(/^Senha/i), senha);
  await user.type(screen.getByLabelText(/Confirmar senha/i), senha2);
  if (consent) await user.click(screen.getByRole('checkbox'));
}

describe('RegisterPage', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    vi.mocked(api.registerAccount).mockReset();
    vi.mocked(viaCep.lookupCep).mockReset();
    vi.mocked(viaCep.lookupCep).mockResolvedValue(null);
  });

  it('envia o cadastro com o payload completo (address embutido)', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockResolvedValue({
      id: 'id',
      full_name: 'Roberval Juvencio Lazaroti',
      email: 'roberval@example.com',
    });
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    await waitFor(() => expect(api.registerAccount).toHaveBeenCalledOnce());
    // Payload EXATO: birth_date BR→ISO, phone só dígitos, cpf/CEP mascarados, sem `country`.
    expect(vi.mocked(api.registerAccount).mock.calls[0][0]).toEqual({
      full_name: 'Roberval Juvencio Lazaroti',
      cpf: '948.190.898-46',
      birth_date: '1976-01-23',
      email: 'roberval@example.com',
      phone: '11912345678',
      password: 'cavalo-bateria-grampo',
      address: {
        zip_code: '06472-000',
        street: 'Avenida Copacabana',
        number: '238',
        complement: '',
        neighborhood: 'Dezoito do Forte',
        city: 'Barueri',
        state: 'SP',
      },
    });
  });

  // A espera é longa e real (cadastro síncrono contra a Doutor ao Vivo).
  // Sem aviso, o usuário acha que travou e recarrega no meio.
  it('avisa que a espera é longa enquanto o cadastro corre', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockImplementation(() => new Promise(() => {})); // nunca resolve
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    const aviso = await screen.findByRole('status');
    expect(aviso).toHaveTextContent(/pode levar até um minuto/i);
    expect(aviso).toHaveTextContent(/não feche nem recarregue/i);
  });

  it('barra senhas divergentes sem chamar a API', async () => {
    const user = userEvent.setup();
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user, { senha: 'cavalo-bateria-grampo', senha2: 'outra-senha-diferente' });

    expect(await screen.findByText(/não coincidem/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));
    expect(api.registerAccount).not.toHaveBeenCalled();
  });

  it('barra senha curta (<12) sem chamar a API', async () => {
    const user = userEvent.setup();
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user, { senha: 'curta', senha2: 'curta' });

    expect(await screen.findByText(/pelo menos 12 caracteres/i)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));
    expect(api.registerAccount).not.toHaveBeenCalled();
  });

  // O 409 de e-mail já usado na DAV precisa chegar com a frase que a API escreveu —
  // é o casal que compartilha e-mail, e um "erro inesperado" o deixaria sem saída.
  it('mostra a mensagem da API quando o e-mail já está em uso', async () => {
    const user = userEvent.setup();
    vi.mocked(api.registerAccount).mockRejectedValue(
      new ApiError(
        409,
        'e-mail indisponível',
        'este e-mail já está vinculado a outro paciente. Use um e-mail pessoal, não compartilhado.',
      ),
    );
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);
    await user.click(screen.getByRole('button', { name: /Criar conta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      /já está vinculado a outro paciente/i,
    );
  });

  it('mantém "Criar conta" desabilitado até aceitar os termos, com o motivo visível', async () => {
    const user = userEvent.setup();
    renderPage();

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user, { consent: false });

    const criar = screen.getByRole('button', { name: /Criar conta/i });
    expect(criar).toBeDisabled();
    expect(screen.getByText(/Aceite os termos para continuar/i)).toBeInTheDocument();

    await user.click(screen.getByRole('checkbox'));
    expect(criar).toBeEnabled();
  });

  it('preenche o endereço pelo CEP (ViaCEP) e mantém os campos editáveis', async () => {
    const user = userEvent.setup();
    vi.mocked(viaCep.lookupCep).mockResolvedValue({
      street: 'Avenida Copacabana',
      neighborhood: 'Dezoito do Forte',
      city: 'Barueri',
      state: 'SP',
    });
    renderPage();

    await fillStep1(user);
    await user.type(screen.getByLabelText(/CEP/i), '06472000');

    // autofill assíncrono
    await screen.findByDisplayValue('Avenida Copacabana');
    expect(screen.getByLabelText(/Cidade/i)).toHaveValue('Barueri');
    expect(screen.getByLabelText(/UF/i)).toHaveValue('SP');

    // continuam editáveis: o usuário corrige a rua
    const rua = screen.getByLabelText(/Endereço/i);
    await user.clear(rua);
    await user.type(rua, 'Rua das Flores');
    expect(rua).toHaveValue('Rua das Flores');
  });

  // Foco: avançar/voltar de passo desmonta o botão focado (Continuar/Voltar) e o
  // foco cai no <body>, deixando quem usa leitor de tela sem contexto do passo novo.
  it('move o foco para o heading do passo ao avançar no wizard', async () => {
    const user = userEvent.setup();
    renderPage();

    await fillStep1(user);

    expect(await screen.findByRole('heading', { name: 'Seu endereço' })).toHaveFocus();
  });

  // Guard: a máscara do CEP satura em 8 dígitos — continuar digitando não muda o
  // valor exibido, mas o onChange do input ainda dispara. Sem a guarda, cada
  // keystroke extra repetiria a consulta à ViaCEP.
  it('não repete a consulta à ViaCEP quando o CEP de 8 dígitos não muda', async () => {
    const user = userEvent.setup();
    vi.mocked(viaCep.lookupCep).mockResolvedValue({
      street: 'Avenida Copacabana',
      neighborhood: 'Dezoito do Forte',
      city: 'Barueri',
      state: 'SP',
    });
    renderPage();

    await fillStep1(user);
    const cepInput = screen.getByLabelText(/CEP/i);
    await user.type(cepInput, '06472000');
    await screen.findByDisplayValue('Avenida Copacabana');

    // keystroke extra: 9 dígitos digitados, mas a máscara continua devolvendo os
    // mesmos 8 — antes do fix, isso disparava uma segunda consulta.
    await user.type(cepInput, '9');

    await waitFor(() => expect(viaCep.lookupCep).toHaveBeenCalledTimes(1));
  });
});
