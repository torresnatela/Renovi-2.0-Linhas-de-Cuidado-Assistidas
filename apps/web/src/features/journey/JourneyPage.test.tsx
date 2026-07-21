import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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

  /**
   * Regressão: com 2+ matrículas, "o mais importante agora" escolhia a próxima
   * consulta GLOBALMENTE entre todas as linhas, mas carimbava o nome da linha
   * ATIVA — mostrando a consulta errada com o nome errado, e sem reagir à troca
   * de chip. O card deve pertencer sempre à linha ativa.
   */
  it('MostImportantNow respeita a linha ativa quando a consulta futura é de outra linha', async () => {
    const duasLinhas: Journey = {
      enrollments: [
        {
          enrollment: {
            id: 'enr-a',
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
                id: 'item-a1',
                ref: 'aval-inicial',
                kind: 'CONSULTA',
                specialty_code: 'PSI',
                label: 'Avaliação inicial (Saúde Mental)',
                sort_order: 1,
              },
              eligibility: { allowed: true, blocks: [] },
            },
          ],
          recent_events: [],
        },
        {
          enrollment: {
            id: 'enr-b',
            care_line_code: 'ortopedia',
            care_line_version: 1,
            status: 'ativa',
            valid_from: '2026-07-01T00:00:00-03:00',
            valid_until: '2026-09-01T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Ortopedia',
          items: [
            {
              item: {
                id: 'item-b1',
                ref: 'consulta-orto',
                kind: 'CONSULTA',
                specialty_code: 'ORTO',
                label: 'Consulta de retorno (Ortopedia)',
                sort_order: 1,
              },
              eligibility: {
                allowed: false,
                blocks: [{ rule_type: 'MIN_INTERVAL', reason: 'Aguarde o intervalo mínimo.' }],
              },
            },
          ],
          recent_events: [],
        },
      ],
    };

    vi.mocked(api.getJourney).mockResolvedValue(duasLinhas);
    vi.mocked(api.listCareAppointments).mockResolvedValue([
      {
        id: 'apt-1',
        item_ref: 'consulta-orto',
        label: 'Consulta Ortopedia — Dr. Fulano',
        status: 'agendada',
        scheduled_at: '2026-08-05T10:00:00-03:00',
        time_zone: 'America/Sao_Paulo',
        booking_id: 'bk-1',
      },
    ]);

    const user = userEvent.setup();
    renderPage();

    const secao1 = (await screen.findByText('O mais importante agora')).closest('section');
    expect(secao1).not.toBeNull();

    // Linha A (Saúde Mental) ativa por padrão: a única consulta futura é da linha B
    // (Ortopedia) — não deve aparecer aqui; cai para o item liberado da própria linha.
    expect(within(secao1!).queryByText(/Consulta Ortopedia/)).not.toBeInTheDocument();
    expect(
      within(secao1!).getByText('Avaliação inicial (Saúde Mental)'),
    ).toBeInTheDocument();

    // Troca para a linha Ortopedia pelo chip: agora a consulta pertence à linha ativa.
    await user.click(screen.getByRole('tab', { name: 'Ortopedia' }));

    const secao2 = (await screen.findByText('O mais importante agora')).closest('section');
    expect(secao2).not.toBeNull();
    expect(
      within(secao2!).getByText('Consulta Ortopedia — Dr. Fulano'),
    ).toBeInTheDocument();
    expect(within(secao2!).getByText('Ortopedia')).toBeInTheDocument();
  });

  /**
   * Regressão (bug ao vivo): `GET /me/journey` devolve TODA matrícula, inclusive
   * encerrada/expirada/concluída — e versões antigas da MESMA linha compartilham o
   * `care_line_code`. Sem filtrar por status e sem chave por `enrollment.id`, os
   * chips mostravam matrículas encerradas, duas linhas com o MESMO nome (key
   * duplicada no React) e a timeline podia cair na versão ERRADA (a primeira do
   * array, não a ativa).
   */
  it('chips mostram só matrículas ativas, keyed por enrollment.id (não por care_line_code)', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const tresMatriculas: Journey = {
      enrollments: [
        {
          // v1 da linha X: ENCERRADA, mesmo care_line_code da v2 abaixo. Vem
          // primeiro no array — é o cenário que expôs o bug do find().
          enrollment: {
            id: 'enr-v1-encerrada',
            care_line_code: 'saude-mental',
            care_line_version: 1,
            status: 'encerrada',
            valid_from: '2026-01-01T00:00:00-03:00',
            valid_until: '2026-06-01T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Linha A',
          items: [
            {
              item: {
                id: 'item-v1',
                ref: 'passo-v1',
                kind: 'CONSULTA',
                specialty_code: 'PSI',
                label: 'Passo da V1 (encerrada)',
                sort_order: 1,
              },
              eligibility: { allowed: true, blocks: [] },
            },
          ],
          recent_events: [],
        },
        {
          // v2 da MESMA linha (mesmo code X): ATIVA — é esta que deve aparecer.
          enrollment: {
            id: 'enr-v2-ativa',
            care_line_code: 'saude-mental',
            care_line_version: 2,
            status: 'ativa',
            valid_from: '2026-06-01T00:00:00-03:00',
            valid_until: '2026-12-01T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Linha A',
          items: [
            {
              item: {
                id: 'item-v2',
                ref: 'passo-v2',
                kind: 'CONSULTA',
                specialty_code: 'PSI',
                label: 'Passo da V2 (ativa)',
                sort_order: 1,
              },
              eligibility: { allowed: true, blocks: [] },
            },
          ],
          recent_events: [],
        },
        {
          // Outra linha (code Y), também ativa.
          enrollment: {
            id: 'enr-y-ativa',
            care_line_code: 'ortopedia',
            care_line_version: 1,
            status: 'ativa',
            valid_from: '2026-01-01T00:00:00-03:00',
            valid_until: '2026-12-01T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Ortopedia',
          items: [
            {
              item: {
                id: 'item-y',
                ref: 'passo-y',
                kind: 'CONSULTA',
                specialty_code: 'ORTO',
                label: 'Passo Ortopedia',
                sort_order: 1,
              },
              eligibility: { allowed: true, blocks: [] },
            },
          ],
          recent_events: [],
        },
      ],
    };

    vi.mocked(api.getJourney).mockResolvedValue(tresMatriculas);

    renderPage();

    // (a) só 2 chips visíveis: a matrícula encerrada não entra.
    const tabs = await screen.findAllByRole('tab');
    expect(tabs).toHaveLength(2);
    expect(tabs.map((t) => t.textContent)).toEqual(['Linha A', 'Ortopedia']);

    // (b) sem seleção, a linha padrão é a matrícula ATIVA v2 — não a encerrada
    // (que vinha primeiro no array e "vencia" no find() antigo por care_line_code).
    // Aparece 2x (destaque + timeline) — daí findAllByText.
    expect((await screen.findAllByText('Passo da V2 (ativa)')).length).toBeGreaterThan(0);
    expect(screen.queryByText('Passo da V1 (encerrada)')).not.toBeInTheDocument();

    // (c) output limpo: nenhum warning do React de key duplicada entre chips.
    const keyDuplicada = consoleErrorSpy.mock.calls.some((args) =>
      String(args[0]).includes('same key'),
    );
    expect(keyDuplicada).toBe(false);

    consoleErrorSpy.mockRestore();
  });
});
