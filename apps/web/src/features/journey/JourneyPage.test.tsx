import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Journey } from '../../shared/api';
import { JourneyPage } from './JourneyPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getJourney: vi.fn() };
});
const api = await import('../../shared/api');

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
  });

  it('mostra a matrícula com sua vigência e o status', async () => {
    renderPage();
    expect(await screen.findByText('Saúde Mental')).toBeInTheDocument();
    expect(screen.getByText('Ativa')).toBeInTheDocument();
    expect(screen.getByText(/Vigência:/)).toBeInTheDocument();
  });

  it('no item liberado, oferece o link de agendar', async () => {
    renderPage();
    expect(await screen.findByText('Avaliação inicial')).toBeInTheDocument();
    expect(screen.getByText('Disponível')).toBeInTheDocument();
    // O link leva ao agendar POR ITEM (não ao wizard de booking).
    expect(screen.getByRole('link', { name: 'Agendar' })).toHaveAttribute(
      'href',
      '/jornada/agendar/item-1',
    );
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
    expect(screen.queryByRole('link', { name: /item-2/ })).not.toBeInTheDocument();
  });

  it('lista os eventos recentes da jornada', async () => {
    renderPage();
    expect(await screen.findByText(/Matrícula criada/)).toBeInTheDocument();
  });
});
