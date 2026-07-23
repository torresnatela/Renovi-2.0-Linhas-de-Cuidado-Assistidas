import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError } from '../../shared/api';
import { OnboardingPage } from './OnboardingPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getOnboardingInfo: vi.fn(),
    completeOnboarding: vi.fn(),
    declineOnboarding: vi.fn(),
  };
});
vi.mock('../auth/viaCep');

const api = await import('../../shared/api');
const viaCep = await import('../auth/viaCep');

const TOKEN = 'tok-abc';
const INFO = {
  invite_name: 'Maria Silva',
  invite_email: 'maria@e.test',
  invite_phone: '11999999999',
  companies: ['ACME'],
};

function renderPage(token = TOKEN) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/onboarding/${token}`]}>
        <Routes>
          <Route path="/onboarding/:token" element={<OnboardingPage />} />
          <Route path="/" element={<div>HOME</div>} />
          <Route path="/cadastro" element={<div>CADASTRO</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

type User = ReturnType<typeof userEvent.setup>;

// nome e celular já vêm pré-preenchidos do convite: o passo 1 só digita CPF e nascimento.
async function fillStep1(user: User) {
  await user.type(screen.getByLabelText(/CPF/i), '11144477735');
  await user.type(screen.getByLabelText(/Nascimento/i), '01011990');
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

// e-mail já vem pré-preenchido: o passo 3 só define a senha e aceita os termos.
async function fillStep3(user: User, senha = 'cavalo-bateria-grampo') {
  await user.type(screen.getByLabelText(/^Senha/i), senha);
  await user.type(screen.getByLabelText(/Confirmar senha/i), senha);
  await user.click(screen.getByRole('checkbox'));
  await user.click(screen.getByRole('button', { name: /Continuar/i }));
}

describe('OnboardingPage', () => {
  beforeEach(() => {
    vi.mocked(api.getOnboardingInfo).mockReset().mockResolvedValue(INFO);
    vi.mocked(api.completeOnboarding).mockReset();
    vi.mocked(api.declineOnboarding).mockReset().mockResolvedValue(undefined);
    vi.mocked(viaCep.lookupCep).mockReset().mockResolvedValue(null);
  });

  it('pré-preenche o cadastro com os dados do convite (nome, e-mail, celular)', async () => {
    renderPage();
    expect(await screen.findByLabelText(/Nome completo/i)).toHaveValue('Maria Silva');
    // O CPF NÃO vem pré-preenchido (só temos o cpf_hmac).
    expect(screen.getByLabelText(/CPF/i)).toHaveValue('');
    expect(screen.getByLabelText(/Celular/i)).toHaveValue('(11) 99999-9999');
  });

  it('token inválido/expirado mostra o motivo com saída, não o formulário', async () => {
    vi.mocked(api.getOnboardingInfo).mockRejectedValue(
      new ApiError(410, 'convite expirado', 'peça um novo link ao RH da sua empresa'),
    );
    renderPage();

    expect(await screen.findByText(/Convite indisponível/i)).toBeInTheDocument();
    expect(screen.getByText(/peça um novo link ao RH/i)).toBeInTheDocument();
    expect(screen.queryByLabelText(/Nome completo/i)).not.toBeInTheDocument();
  });

  it('conclui: confirma a empresa e envia o cadastro pré-preenchido + token', async () => {
    const user = userEvent.setup();
    vi.mocked(api.completeOnboarding).mockResolvedValue({
      id: 'id',
      full_name: 'Maria Silva',
      email: 'maria@e.test',
    });
    renderPage();
    await screen.findByLabelText(/Nome completo/i);

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);

    // Passo da empresa: SIM é o caminho fácil.
    expect(await screen.findByRole('heading', { name: /faz parte da empresa ACME/i })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Sim, faço parte/i }));

    await waitFor(() => expect(api.completeOnboarding).toHaveBeenCalledOnce());
    const [token, body] = vi.mocked(api.completeOnboarding).mock.calls[0];
    expect(token).toBe(TOKEN);
    expect(body).toEqual({
      full_name: 'Maria Silva',
      cpf: '111.444.777-35',
      birth_date: '1990-01-01',
      email: 'maria@e.test',
      phone: '11999999999',
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
    // Sessão aberta → tela de sucesso.
    expect(await screen.findByText(/Conta criada, Maria/i)).toBeInTheDocument();
  }, 20000);

  it('recusar a empresa exige DUPLA confirmação e não cria conta', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByLabelText(/Nome completo/i);

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);
    await screen.findByRole('heading', { name: /faz parte da empresa ACME/i });

    // 1º "não": só revela o aviso — NÃO chama a API ainda.
    await user.click(screen.getByRole('button', { name: /Não faço parte dessa empresa/i }));
    expect(api.declineOnboarding).not.toHaveBeenCalled();

    // 2ª confirmação explícita: registra a recusa e mostra o beco sem saída.
    await user.click(screen.getByRole('button', { name: /Confirmar que não/i }));
    await waitFor(() => expect(api.declineOnboarding).toHaveBeenCalledOnce());
    expect(api.completeOnboarding).not.toHaveBeenCalled();
    expect(await screen.findByText(/Tudo bem/i)).toBeInTheDocument();
    expect(screen.getByText(/fale com o RH da sua empresa/i)).toBeInTheDocument();
  }, 20000);

  it('mostra a mensagem da API quando o CPF não corresponde ao convite', async () => {
    const user = userEvent.setup();
    vi.mocked(api.completeOnboarding).mockRejectedValue(
      new ApiError(400, 'CPF não confere', 'o CPF informado não corresponde ao convite'),
    );
    renderPage();
    await screen.findByLabelText(/Nome completo/i);

    await fillStep1(user);
    await fillStep2(user);
    await fillStep3(user);
    await screen.findByRole('heading', { name: /faz parte da empresa ACME/i });
    await user.click(screen.getByRole('button', { name: /Sim, faço parte/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/não corresponde ao convite/i);
  }, 20000);
});
