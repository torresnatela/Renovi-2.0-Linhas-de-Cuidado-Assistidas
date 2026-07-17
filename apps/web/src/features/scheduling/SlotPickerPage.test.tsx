import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, type SlotPage } from '../../shared/api';
import { SlotPickerPage } from './SlotPickerPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, listSlots: vi.fn(), createAppointment: vi.fn() };
});
const api = await import('../../shared/api');

const pagina: SlotPage = {
  professional: {
    id: 'prof-1',
    full_name: 'Ana Beatriz Moura',
    image_url: null,
    license: { council: 'CRP', number: '06/123456', region: 'SP', rqe: null },
  },
  from: '2026-07-20',
  to: '2026-08-19',
  items: [
    {
      id: 'slot-1',
      starts_at: '2026-07-20T09:00:00-03:00',
      ends_at: '2026-07-20T09:25:00-03:00',
      time_zone: 'America/Sao_Paulo',
    },
    {
      id: 'slot-2',
      starts_at: '2026-07-21T14:00:00-03:00',
      ends_at: '2026-07-21T14:25:00-03:00',
      time_zone: 'America/Sao_Paulo',
    },
  ],
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/agendar/esp-1/prof-1']}>
        <Routes>
          <Route path="/agendar/:specialtyId/:professionalId" element={<SlotPickerPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function escolherPrimeiroHorario(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: '09:00' }));
}

describe('SlotPickerPage', () => {
  beforeEach(() => {
    vi.mocked(api.listSlots).mockResolvedValue(pagina);
    vi.mocked(api.createAppointment).mockReset();
  });

  /**
   * O runner de CI roda em UTC. Uma implementação que formate no fuso do browser
   * mostraria 12:00 aqui — e a segunda asserção é o que a pega.
   */
  it('mostra o horário no fuso da agenda, não no do browser', async () => {
    renderPage();
    expect(await screen.findByRole('button', { name: '09:00' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '12:00' })).not.toBeInTheDocument();
  });

  it('agrupa os horários por dia', async () => {
    renderPage();
    expect(await screen.findByText(/segunda-feira, 20 de julho/i)).toBeInTheDocument();
    expect(screen.getByText(/terça-feira, 21 de julho/i)).toBeInTheDocument();
  });

  it('avisa que não há horário sem tratar isso como erro', async () => {
    vi.mocked(api.listSlots).mockResolvedValue({ ...pagina, items: [] });
    renderPage();
    expect(await screen.findByText(/não tem horário livre/i)).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  // 503 é "não conseguimos ler a agenda", não "não há horários". Confundir os dois
  // faz o paciente desistir de um profissional que estava livre.
  it('diferencia agenda vazia de agenda indisponível', async () => {
    vi.mocked(api.listSlots).mockRejectedValue(new ApiError(503, 'indisponível'));
    renderPage();
    expect(await screen.findByRole('alert')).toHaveTextContent(/problema nosso, não seu/i);
  });

  it('manda o horário e a especialidade escolhidos', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createAppointment).mockResolvedValue({} as never);
    renderPage();

    await escolherPrimeiroHorario(user);
    await user.click(screen.getByRole('button', { name: /Confirmar consulta/i }));

    // Afirma o PRIMEIRO argumento, e não a chamada inteira: o TanStack Query v5
    // passa um objeto de contexto como segundo argumento para a mutationFn, e um
    // toHaveBeenCalledWith cravaria esse detalhe da biblioteca no teste.
    expect(vi.mocked(api.createAppointment).mock.calls[0][0]).toEqual({
      slot_id: 'slot-1',
      // A especialidade vem da URL: o slot não a determina (o vínculo
      // profissional-especialidade é muitos-para-muitos no legado).
      specialty_id: 'esp-1',
    });
  });

  /**
   * Espelha o teste do RegisterPage: o POST fala com a DAV, que mediu de 3s a
   * 29s. Sem o aviso, o usuário acha que travou e recarrega no meio.
   */
  it('avisa que a reserva demora enquanto ela corre', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createAppointment).mockReturnValue(new Promise(() => {}) as never);
    renderPage();

    await escolherPrimeiroHorario(user);
    await user.click(screen.getByRole('button', { name: /Confirmar consulta/i }));

    const aviso = await screen.findByRole('status');
    expect(aviso).toHaveTextContent(/pode levar até um minuto/i);
    expect(aviso).toHaveTextContent(/não feche nem recarregue/i);
  });

  it('traduz o 409 de horário tomado', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createAppointment).mockRejectedValue(
      new ApiError(409, 'Horário indisponível', 'texto da api', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    await escolherPrimeiroHorario(user);
    await user.click(screen.getByRole('button', { name: /Confirmar consulta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/reservado por outra pessoa/i);
  });

  /**
   * O teste que codifica o ADR-016. No 502 o resultado é DESCONHECIDO: a consulta
   * pode existir, e repetir criaria uma SEGUNDA de verdade. Dizer "falhou" ou
   * oferecer "tentar de novo" seria mentira perigosa.
   */
  it('no 502 não diz que falhou nem oferece tentar de novo', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createAppointment).mockRejectedValue(
      new ApiError(502, 'Não conseguimos confirmar', 'texto da api', {
        code: 'BOOKING_UNCONFIRMED',
      }),
    );
    renderPage();

    await escolherPrimeiroHorario(user);
    await user.click(screen.getByRole('button', { name: /Confirmar consulta/i }));

    const alerta = await screen.findByRole('alert');
    expect(alerta).toHaveTextContent(/pode ter sido marcada/i);
    expect(screen.getByRole('link', { name: /Ver minhas consultas/i })).toBeInTheDocument();

    expect(screen.queryByText(/não foi marcada/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Confirmar consulta/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /tentar de novo/i })).not.toBeInTheDocument();
  });
});
