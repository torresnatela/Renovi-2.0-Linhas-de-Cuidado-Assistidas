import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, type CareAppointment } from '../../shared/api';
import { CareAppointmentsPage } from './CareAppointmentsPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, listCareAppointments: vi.fn(), cancelCareAppointment: vi.fn() };
});
const api = await import('../../shared/api');

function consulta(over: Partial<CareAppointment> = {}): CareAppointment {
  return {
    id: 'care-1',
    item_ref: 'aval-inicial',
    label: 'Avaliação inicial',
    status: 'agendada',
    scheduled_at: '2026-07-20T09:00:00-03:00',
    time_zone: 'America/Sao_Paulo',
    booking_id: 'book-1',
    ...over,
  };
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/jornada/consultas']}>
        <CareAppointmentsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('CareAppointmentsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('mostra a consulta com data/hora no fuso da agenda', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
    renderPage();
    expect(await screen.findByText('Avaliação inicial')).toBeInTheDocument();
    // Em UTC sairia 12:00 — o runner de CI pegaria a formatação ingênua.
    expect(screen.getByText(/20\/07 às 09:00/)).toBeInTheDocument();
  });

  /**
   * Cancelar fala com a API e a lista se refaz (invalidação): a consulta reaparece
   * cancelada e o botão some. O confirm() é a rede contra o clique acidental num
   * ato destrutivo.
   */
  it('cancela pela API e atualiza a lista', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.mocked(api.listCareAppointments)
      .mockResolvedValueOnce([consulta()])
      .mockResolvedValueOnce([consulta({ status: 'cancelada', cancelled_at: '2026-07-19T10:00:00-03:00' })]);
    vi.mocked(api.cancelCareAppointment).mockResolvedValue(
      consulta({ status: 'cancelada', cancelled_at: '2026-07-19T10:00:00-03:00' }),
    );

    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Cancelar' }));

    await waitFor(() => expect(api.cancelCareAppointment).toHaveBeenCalledWith('care-1'));
    expect(await screen.findByText('Cancelada')).toBeInTheDocument();
    // Cancelada não se cancela de novo: o botão some.
    expect(screen.queryByRole('button', { name: 'Cancelar' })).not.toBeInTheDocument();
  });

  it('não chama a API se o paciente desiste no confirm()', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);

    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Cancelar' }));
    expect(api.cancelCareAppointment).not.toHaveBeenCalled();
  });

  it('traduz o 409 de consulta não cancelável', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
    vi.mocked(api.cancelCareAppointment).mockRejectedValue(
      new ApiError(409, 'Não cancelável', 'texto', { code: 'CANCEL_NOT_ALLOWED' }),
    );

    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Cancelar' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/não pode mais ser cancelada/i);
  });
});
