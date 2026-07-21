import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  ApiError,
  type Appointment,
  type CareAppointment,
  type Journey,
  type JourneyItem,
} from '../../shared/api';
import { mockViewport } from '../../shared/viewport.testkit';
import { ConsultationsPage } from './ConsultationsPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    listCareAppointments: vi.fn(),
    cancelCareAppointment: vi.fn(),
    getJourney: vi.fn(),
    getAppointment: vi.fn(),
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

// O detalhe do booking (`getAppointment`) — fonte do nome do profissional que os
// cards de "Minhas Consultas" mostram ao lado do label (enriquecimento client-side).
function appointment(over: Partial<Appointment> = {}): Appointment {
  return {
    id: 'book-1',
    status: 'CONFIRMED',
    starts_at: '2026-07-20T09:00:00-03:00',
    ends_at: '2026-07-20T09:50:00-03:00',
    time_zone: 'America/Sao_Paulo',
    specialty: { id: 'psi', name: 'Psicologia' },
    professional: { id: 'prof-1', full_name: 'Dra. Marina Costa' },
    join: {
      status: 'TOO_EARLY',
      opens_at: '2026-07-20T08:30:00-03:00',
      closes_at: '2026-07-20T09:50:00-03:00',
    },
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
    // Default: sem profissional configurado — os testes que não mexem nisto
    // continuam vendo só o label (o enriquecimento é opt-in por teste).
    vi.mocked(api.getAppointment).mockRejectedValue(new Error('getAppointment não configurado'));
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

  /**
   * Regressão (achado real na conta de teste): `GET /me/journey` devolve TODO o
   * histórico de matrículas (inclusive encerrada/expirada/concluída), e o aside
   * fazia `flatMap` de TODAS sem filtrar por `enrollment.status`. Mesma regra já
   * aplicada em `JourneyPage.tsx` e `PlanSection.tsx`: "Para agendar" é o
   * PRESENTE — só itens de matrícula `ativa` aparecem aqui.
   */
  it('mostra no aside só itens de matrícula ativa; a encerrada não aparece', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
    vi.mocked(api.getJourney).mockResolvedValue({
      enrollments: [
        {
          enrollment: {
            id: 'enr-ativa',
            care_line_code: 'saude-mental',
            care_line_version: 1,
            status: 'ativa',
            valid_from: '2026-01-01T00:00:00-03:00',
            valid_until: '2026-12-31T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Saúde mental',
          items: [journeyItem({ id: 'i1', label: 'Psicologia' }, true)],
          recent_events: [],
        },
        {
          enrollment: {
            id: 'enr-encerrada',
            care_line_code: 'ortopedia',
            care_line_version: 1,
            status: 'encerrada',
            valid_from: '2025-01-01T00:00:00-03:00',
            valid_until: '2025-06-30T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Ortopedia (encerrada)',
          items: [journeyItem({ id: 'i2', label: 'Ortopedia' }, true)],
          recent_events: [],
        },
      ],
    });

    renderPage();

    expect(await screen.findByText('Psicologia')).toBeInTheDocument();
    expect(screen.queryByText('Ortopedia')).not.toBeInTheDocument();
  });

  // Atividades (check-in de humor, WHO-5, PHQ-4) não têm especialidade nem
  // slots no legado — não são agendáveis. O aside "Para agendar" existe para
  // o funil de CONSULTA; atividade vive na Jornada, não aqui.
  it('no aside, lista só itens agendáveis (CONSULTA) e omite ATIVIDADE', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
    vi.mocked(api.getJourney).mockResolvedValue(
      journeyWith([
        journeyItem({ id: 'i1', label: 'Psicologia' }, true),
        journeyItem(
          { id: 'i2', kind: 'ATIVIDADE', label: 'Check-in de humor', specialty_code: '' },
          true,
        ),
      ]),
    );
    renderPage();

    expect(await screen.findByRole('link', { name: 'Agendar' })).toHaveAttribute(
      'href',
      '/jornada/agendar/i1',
    );
    expect(screen.queryByText('Check-in de humor')).not.toBeInTheDocument();
  });

  it('aba Próximas vazia mostra CTA para agendar na jornada', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText(/não tem consultas agendadas/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Agendar' })).toHaveAttribute('href', '/jornada');
  });

  // --- Nome do profissional ao lado do label (enriquecimento client-side via booking) ---

  it('mostra o nome do profissional ao lado do label quando o booking carrega', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta({ label: 'Psicologia' })]);
    vi.mocked(api.getAppointment).mockResolvedValue(
      appointment({ professional: { id: 'prof-1', full_name: 'Dra. Marina Costa' } }),
    );
    renderPage();

    expect(await screen.findByText('Psicologia · Dra. Marina Costa')).toBeInTheDocument();
    expect(api.getAppointment).toHaveBeenCalledWith('book-1');
  });

  it('mostra só o label quando o booking falha — enhancement, sem erro na tela', async () => {
    vi.mocked(api.listCareAppointments).mockResolvedValue([consulta({ label: 'Psicologia' })]);
    vi.mocked(api.getAppointment).mockRejectedValue(new Error('sem acesso ao booking'));
    renderPage();

    expect(await screen.findByText('Psicologia')).toBeInTheDocument();
    expect(screen.queryByText(/Psicologia ·/)).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  // --- Mobile (Etapa 3): mock em design_files/Consultas.dc.html ---

  describe('mobile', () => {
    let viewport: ReturnType<typeof mockViewport>;

    beforeEach(() => {
      viewport = mockViewport('mobile');
    });
    afterEach(() => {
      viewport.restore();
    });

    it('mostra o header mobile: eyebrow "Suas consultas", título 26px e o HelpNowMenu', async () => {
      vi.mocked(api.listCareAppointments).mockResolvedValue([]);
      renderPage();

      expect(await screen.findByText('Suas consultas')).toBeInTheDocument();
      const titulo = screen.getByRole('heading', { level: 1, name: 'Consultas' });
      expect(titulo).toHaveClass('text-[26px]');
      expect(screen.getByRole('button', { name: /pedir ajuda/i })).toBeInTheDocument();
    });

    it('a aba (SegmentedControl) ocupa a largura toda', async () => {
      vi.mocked(api.listCareAppointments).mockResolvedValue([]);
      renderPage();

      const tablist = await screen.findByRole('tablist');
      expect(tablist.parentElement).toHaveClass('w-full');
    });

    // Adaptação registrada no report: o mock rotula "Entrar na consulta", mas o
    // clique navega ao DETALHE (o join vive lá, atrás do gate de pré-consulta) —
    // por isso o rótulo é "Ver consulta", que não mente sobre o que o botão faz.
    it('CTA da consulta de hoje usa o rótulo honesto "Ver consulta", nunca "Entrar na consulta"', async () => {
      const now = new Date('2026-07-20T12:00:00Z'); // 09:00 em São Paulo
      vi.mocked(api.listCareAppointments).mockResolvedValue([
        consulta({ scheduled_at: '2026-07-20T16:00:00-03:00' }),
      ]);
      renderPage({ now });

      expect(await screen.findByText('Hoje')).toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Ver consulta' })).toHaveAttribute(
        'href',
        '/consultas/book-1',
      );
      expect(screen.queryByText(/entrar na consulta/i)).not.toBeInTheDocument();
    });

    it('a ação Cancelar aparece no card; a tela nunca menciona "Remarcar"', async () => {
      vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
      renderPage();

      expect(await screen.findByRole('button', { name: 'Cancelar' })).toBeInTheDocument();
      expect(screen.queryByText(/remarcar/i)).not.toBeInTheDocument();
    });

    it('Para agendar aparece depois de Agendadas na ordem do documento', async () => {
      vi.mocked(api.listCareAppointments).mockResolvedValue([consulta()]);
      vi.mocked(api.getJourney).mockResolvedValue(
        journeyWith([journeyItem({ id: 'i1', label: 'Psicologia' }, true)]),
      );
      renderPage();

      const agendadas = await screen.findByText('Agendadas');
      const paraAgendar = await screen.findByText('Para agendar');
      // eslint-disable-next-line no-bitwise
      expect(
        agendadas.compareDocumentPosition(paraAgendar) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    });

    it('mostra a nota final do histórico, centralizada', async () => {
      const user = userEvent.setup();
      vi.mocked(api.listCareAppointments).mockResolvedValue([]);
      renderPage();

      await user.click(screen.getByRole('tab', { name: 'Histórico' }));
      const nota = await screen.findByText(
        /consultas canceladas com mais de 24h de antecedência não contam na sua cota/i,
      );
      expect(nota).toHaveClass('text-center');
    });
  });
});
