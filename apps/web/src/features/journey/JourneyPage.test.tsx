import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Account, Journey } from '../../shared/api';
import { JourneyPage } from './JourneyPage';

// A página agora orquestra várias fontes: a jornada, as consultas (para "o mais
// importante agora" e a timeline), a sessão (saudação) e o dia de humor (card do
// aside). Todas passam pelo mesmo shared/api mockado.
vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getJourney: vi.fn(),
    listCareAppointments: vi.fn(),
    getMe: vi.fn(),
    getMoodToday: vi.fn(),
  };
});
const api = await import('../../shared/api');

const conta: Account = { id: 'acc-1', full_name: 'Ana Paula', email: 'ana@example.com' };

const jornada: Journey = {
  enrollments: [
    {
      enrollment: {
        id: 'enr-1',
        care_line_code: 'saude-mental',
        care_line_version: 1,
        status: 'ativa',
        valid_from: '2026-07-01T00:00:00-03:00',
        valid_until: '2026-09-01T00:00:00-03:00',
        periods: [],
      },
      care_line_name: 'Saúde Mental',
      items: [
        {
          item: {
            id: 'item-1',
            ref: 'aval-inicial',
            kind: 'CONSULTA',
            specialty_code: 'PSI',
            label: 'Avaliação inicial',
            sort_order: 1,
          },
          eligibility: { allowed: true, blocks: [] },
        },
        {
          item: {
            id: 'item-2',
            ref: 'retorno',
            kind: 'CONSULTA',
            specialty_code: 'PSI',
            label: 'Consulta de retorno',
            sort_order: 2,
          },
          eligibility: {
            allowed: false,
            blocks: [
              {
                rule_type: 'MIN_INTERVAL',
                reason: 'Aguarde o intervalo mínimo entre consultas.',
                available_from: '2026-08-01T00:00:00-03:00',
              },
            ],
          },
        },
      ],
      recent_events: [
        {
          id: 'ev-1',
          event_type: 'matricula_criada',
          actor: 'admin',
          occurred_at: '2026-07-01T10:00:00-03:00',
          payload: {},
        },
      ],
    },
  ],
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/jornada']}>
        <JourneyPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('JourneyPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getJourney).mockResolvedValue(jornada);
    vi.mocked(api.getMe).mockResolvedValue(conta);
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
    // not_enrolled mantém o card do aside num estado neutro (sem grade a operar).
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: false,
      reason: 'not_enrolled',
    });
  });

  it('mostra a matrícula com sua vigência e o status', async () => {
    renderPage();
    // Status via faixa de vigência (estado do plano, não texto cru).
    expect(await screen.findByText('Plano ativo')).toBeInTheDocument();
    expect(screen.getByText(/Vigente até/)).toBeInTheDocument();
    // O nome da linha aparece ao menos uma vez (faixa de vigência).
    expect(screen.getAllByText('Saúde Mental').length).toBeGreaterThan(0);
  });

  it('no item liberado, oferece o link de agendar por item', async () => {
    renderPage();
    // Há pelo menos um link "Agendar" que leva ao agendar POR ITEM (não ao booking).
    const links = await screen.findAllByRole('link', { name: /agendar/i });
    expect(links.some((l) => l.getAttribute('href') === '/jornada/agendar/item-1')).toBe(true);
  });

  /**
   * A regra de ouro: o item barrado não some nem vira um "indisponível" mudo —
   * mostra o motivo pronto do servidor e, quando há, a data de desbloqueio.
   */
  it('no item bloqueado, mostra o motivo e a data de desbloqueio', async () => {
    renderPage();
    expect(await screen.findByText('Consulta de retorno')).toBeInTheDocument();
    expect(screen.getByText(/aguarde o intervalo mínimo/i)).toBeInTheDocument();
    // available_from formatado no fuso da agenda — em UTC cairia noutro dia.
    expect(screen.getByText(/01\/08/)).toBeInTheDocument();
    // E NÃO oferece agendar este item.
    const links = screen.getAllByRole('link', { name: /agendar/i });
    expect(links.some((l) => l.getAttribute('href') === '/jornada/agendar/item-2')).toBe(false);
  });

  it('lista os eventos recentes da jornada', async () => {
    renderPage();
    expect(await screen.findByText(/Matrícula criada/)).toBeInTheDocument();
  });
});
