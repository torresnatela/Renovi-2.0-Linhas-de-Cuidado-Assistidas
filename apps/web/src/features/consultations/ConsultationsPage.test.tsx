import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, type CareAppointment, type Journey, type JourneyItem } from '../../shared/api';
import { ConsultationsPage } from './ConsultationsPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    listCareAppointments: vi.fn(),
    cancelCareAppointment: vi.fn(),
    getJourney: vi.fn(),
  };
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

function journeyItem(over: Partial<JourneyItem['item']> = {}, allowed = true): JourneyItem {
  return {
    item: {
      id: 'item-1',
      ref: 'psico',
      kind: 'CONSULTA',
      specialty_code: 'PSI',
      label: 'Psicologia',
      recurrence: '4x por mês',
      sort_order: 1,
      ...over,
    },
    eligibility: allowed
      ? { allowed: true, blocks: [] }
      : {
          allowed: false,
          blocks: [{ rule_type: 'QUOTA', reason: 'Você já agendou sua consulta deste mês.' }],
        },
  };
}

function journeyWith(items: JourneyItem[]): Journey {
  return {
    enrollments: [
      {
        enrollment: {
          id: 'enr-1',
          care_line_code: 'saude-mental',
          care_line_version: 1,
          status: 'ativa',
          valid_from: '2026-01-01T00:00:00-03:00',
          valid_until: '2026-12-31T00:00:00-03:00',
          periods: [],
        },
        care_line_name: 'Saúde mental',
        items,
        recent_events: [],
      },
    ],
  };
}

// Um "agora" fixo bem longe da agenda de teste (jul/2026): assim a consulta
// padrão NUNCA cai em "hoje", a menos que o teste peça — sem depender do relógio
// do runner (que roda em UTC).
const NAO_HOJE = new Date('2026-01-01T12:00:00Z');

function renderPage(opts: { now?: Date } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/consultas']}>
        <ConsultationsPage now={opts.now ?? NAO_HOJE} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('ConsultationsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getJourney).mockResolvedValue({ enrollments: [] });
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
  });

  // --- As 4 intenções migradas de CareAppointmentsPage.test ---

  it('mostra a consulta com data/hora no fuso da agenda', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
    renderPage();
    expect(await screen.findByText('Avaliação inicial')).toBeInTheDocument();
    // Em UTC sairia 12:00 — o runner de CI pegaria a formatação ingênua.
    expect(screen.getByText(/20\/07 às 09:00/)).toBeInTheDocument();
    expect(screen.queryByText(/12:00/)).not.toBeInTheDocument();
  });

  /**
   * Cancelar fala com a API e a lista se refaz (invalidação). No redesign a
   * consulta cancelada SAI de "Próximas" (deixou de ser status ativo) e reaparece
   * em "Histórico". O confirm() é a rede contra o clique acidental num ato
   * destrutivo.
   */
  it('cancela pela API e atualiza a lista', async () => {
    const user = userEvent.setup();
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.mocked(api.listCareAppointments)
      .mockResolvedValueOnce([consulta()])
      .mockResolvedValueOnce([
        consulta({ status: 'cancelada', cancelled_at: '2026-07-19T10:00:00-03:00' }),
      ]);
    vi.mocked(api.cancelCareAppointment).mockResolvedValue(
      consulta({ status: 'cancelada', cancelled_at: '2026-07-19T10:00:00-03:00' }),
    );

    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Cancelar' }));
    await waitFor(() => expect(api.cancelCareAppointment).toHaveBeenCalledWith('care-1'));

    // Saiu de Próximas: o botão Cancelar some.
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Cancelar' })).not.toBeInTheDocument(),
    );

    // E reaparece cancelada no Histórico.
    await user.click(screen.getByRole('tab', { name: 'Histórico' }));
    expect(await screen.findByText('Cancelada')).toBeInTheDocument();
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

  // --- Novas intenções do redesign ---

  it('alterna entre as abas Próximas e Histórico', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listCareAppointments).mockResolvedValue([
      consulta({ id: 'a', label: 'Consulta futura', status: 'agendada' }),
      consulta({
        id: 'b',
        label: 'Consulta antiga',
        status: 'realizada',
        scheduled_at: '2026-06-10T09:00:00-03:00',
      }),
    ]);
    renderPage();

    // Próximas: a agendada aparece; a realizada não.
    expect(await screen.findByText('Consulta futura')).toBeInTheDocument();
    expect(screen.queryByText('Consulta antiga')).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Histórico' }));
    expect(await screen.findByText('Consulta antiga')).toBeInTheDocument();
    expect(screen.queryByText('Consulta futura')).not.toBeInTheDocument();
  });

  it('marca "Hoje" só na consulta do dia (data controlada, no fuso da agenda)', async () => {
    // 2026-07-20T12:00:00Z = 09:00 em São Paulo → "hoje" é 20/07 em SP.
    const now = new Date('2026-07-20T12:00:00Z');
    vi.mocked(api.listCareAppointments).mockResolvedValue([
      consulta({ id: 'hoje', label: 'Consulta de hoje', scheduled_at: '2026-07-20T16:00:00-03:00' }),
      consulta({
        id: 'depois',
        label: 'Consulta futura',
        scheduled_at: '2026-07-23T10:00:00-03:00',
      }),
    ]);
    renderPage({ now });

    await screen.findByText('Consulta de hoje');
    expect(screen.getAllByText('Hoje')).toHaveLength(1);
  });

  it('agrupa o histórico por mês, do mais recente ao mais antigo', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listCareAppointments).mockResolvedValue([
      consulta({
        id: 'jul',
        label: 'Consulta de julho',
        status: 'realizada',
        scheduled_at: '2026-07-16T10:00:00-03:00',
      }),
      consulta({
        id: 'jun',
        label: 'Consulta de junho',
        status: 'realizada',
        scheduled_at: '2026-06-25T10:00:00-03:00',
      }),
    ]);
    renderPage();

    await user.click(screen.getByRole('tab', { name: 'Histórico' }));
    const cabecalhos = await screen.findAllByText(/de 2026/);
    expect(cabecalhos.map((n) => n.textContent)).toEqual(['Julho de 2026', 'Junho de 2026']);
  });

  it('filtra o histórico por Canceladas', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listCareAppointments).mockResolvedValue([
      consulta({
        id: 'r',
        label: 'Consulta realizada',
        status: 'realizada',
        scheduled_at: '2026-07-16T10:00:00-03:00',
      }),
      consulta({
        id: 'c',
        label: 'Consulta cancelada',
        status: 'cancelada',
        scheduled_at: '2026-07-09T10:00:00-03:00',
      }),
    ]);
    renderPage();

    await user.click(screen.getByRole('tab', { name: 'Histórico' }));
    expect(await screen.findByText('Consulta realizada')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Canceladas' }));
    expect(screen.getByText('Consulta cancelada')).toBeInTheDocument();
    expect(screen.queryByText('Consulta realizada')).not.toBeInTheDocument();
  });

  it('mostra no aside o item liberado (Agendar) e o motivo do bloqueado', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
    vi.mocked(api.getJourney).mockResolvedValue(
      journeyWith([
        journeyItem({ id: 'i1', label: 'Psicologia' }, true),
        journeyItem({ id: 'i2', label: 'Psiquiatria', recurrence: '1x por mês' }, false),
      ]),
    );
    renderPage();

    expect(await screen.findByRole('link', { name: 'Agendar' })).toHaveAttribute(
      'href',
      '/jornada/agendar/i1',
    );
    expect(screen.getByText('Você já agendou sua consulta deste mês.')).toBeInTheDocument();
  });

  it('aba Próximas vazia mostra CTA para agendar na jornada', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText(/não tem consultas agendadas/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Agendar' })).toHaveAttribute('href', '/jornada');
  });
});
