import { render, screen, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import App from './App';
import { ApiError, type Appointment, type Journey } from './shared/api';
import { mockViewport } from './shared/viewport.testkit';

vi.mock('./shared/api', async () => {
  const actual = await vi.importActual<typeof import('./shared/api')>('./shared/api');
  return {
    ...actual,
    getMe: vi.fn(),
    getJourney: vi.fn(),
    listCareAppointments: vi.fn(),
    getMoodToday: vi.fn(),
    getAppointment: vi.fn(),
  };
});
const api = await import('./shared/api');

const conta = {
  id: '019f6c75-1ec9-7a93-b852-66a70d765ca6',
  full_name: 'Roberval Juvencio Lazaroti',
  email: 'roberval@example.com',
};

function renderApp() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
}

describe('App', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockReset();
    vi.mocked(api.getJourney).mockReset();
    // A raiz redireciona para a Jornada, que consome getJourney: mock inócuo.
    vi.mocked(api.getJourney).mockResolvedValue({ enrollments: [] });
    // A Jornada também lê consultas e o dia de humor (card do aside): mocks inócuos.
    vi.mocked(api.listCareAppointments).mockResolvedValue([]);
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: false,
      reason: 'not_enrolled',
    });
    window.history.pushState({}, '', '/');
  });

  // Sem sessão, nenhuma tela do paciente pode aparecer nem por um instante.
  it('manda quem não tem sessão para o login', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'não autenticado'));
    renderApp();

    expect(
      await screen.findByRole(
        'heading',
        { level: 1, name: 'Que bom te ver' },
        { timeout: 10000 },
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Olá,/)).not.toBeInTheDocument();
  });

  // A home foi aposentada: a raiz agora cai na Jornada (dentro do shell). Sem
  // matrículas, a Jornada mostra o estado vazio informativo — prova de que a tela
  // (e não a home antiga) renderizou no caminho certo.
  it('redireciona a raiz para a Jornada quando há sessão', async () => {
    vi.mocked(api.getMe).mockResolvedValue(conta);
    renderApp();

    expect(
      await screen.findByText('Você ainda não está em nenhuma linha de cuidado.'),
    ).toBeInTheDocument();
    expect(window.location.pathname).toBe('/jornada');
  });

  // O antigo cabeçalho "Renovi 2.0" deu lugar ao shell com a navegação principal.
  it('renderiza o shell do produto com a navegação principal', async () => {
    vi.mocked(api.getMe).mockResolvedValue(conta);
    renderApp();

    expect(await screen.findByRole('navigation')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Jornada' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Consultas' })).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Perfil' }).length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Pedir ajuda' })).toBeInTheDocument();
  });

  // Etapa 7 (integração): smoke mobile no nível do App — roteador real + shell +
  // página juntos, não peças isoladas. `mockViewport('mobile')` força
  // `useIsDesktop()` para false; `restore()` no afterEach evita vazar para os
  // `it()`s desktop acima (que rodam com o default do jsdom).
  describe('mobile', () => {
    let viewport: ReturnType<typeof mockViewport>;

    beforeEach(() => {
      viewport = mockViewport('mobile');
    });

    afterEach(() => {
      viewport.restore();
    });

    // Uma matrícula ativa evita o estado vazio da Jornada: só com o hero
    // renderizado o "Pedir ajuda" mobile (que vive na própria tela, não no
    // AppShell) aparece — é o que este smoke precisa provar único.
    const jornadaComMatricula: Journey = {
      enrollments: [
        {
          enrollment: {
            id: 'enr-mobile-1',
            care_line_code: 'saude-mental',
            care_line_version: 1,
            status: 'ativa',
            valid_from: '2026-07-01T00:00:00-03:00',
            valid_until: '2026-09-01T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Saúde Mental',
          items: [],
          recent_events: [],
        },
      ],
    };

    // Smoke de chrome raiz (/jornada): TabBar presente com os 3 destinos, SEM o
    // header sticky do desktop (role banner — a asserção menos frágil: o mock
    // inteiro dele não existe no mobile), e o "Pedir ajuda" único. No mobile o
    // AppShell descarta o `help` que a AppLayout repassa (só o desktop o usa) —
    // quem mostra a afordância é a própria Jornada, uma vez só. Duas fontes
    // renderizando ao mesmo tempo é exatamente o bug de dual-render que este
    // smoke fecha.
    it('smoke de chrome raiz: TabBar com os 3 destinos, sem header desktop, "Pedir ajuda" único', async () => {
      vi.mocked(api.getMe).mockResolvedValue(conta);
      vi.mocked(api.getJourney).mockResolvedValue(jornadaComMatricula);
      window.history.pushState({}, '', '/jornada');
      renderApp();

      // A TabBar (do AppShell) monta de cara, mas o "Pedir ajuda" só existe
      // depois da Jornada sair do loading (é a própria tela quem o renderiza no
      // mobile) — espera nele em vez do nav garante que a matrícula já resolveu.
      await screen.findByRole('button', { name: /pedir ajuda/i });

      const tabBar = screen.getByRole('navigation', { name: 'Principal' });
      expect(within(tabBar).getByRole('link', { name: 'Jornada' })).toBeInTheDocument();
      expect(within(tabBar).getByRole('link', { name: 'Consultas' })).toBeInTheDocument();
      expect(within(tabBar).getByRole('link', { name: 'Perfil' })).toBeInTheDocument();

      expect(screen.queryByRole('banner')).not.toBeInTheDocument();
      expect(screen.getAllByRole('button', { name: /pedir ajuda/i })).toHaveLength(1);
    });

    // Smoke de fluxo empilhado (/consultas/:id): a rota está em ROTAS_DE_FLUXO
    // (AppLayout), então a TabBar dá lugar ao FlowHeader da própria página — sem
    // navegação lateral competindo com a tarefa (ADR-041). `join.status:
    // 'UNAVAILABLE'` é o mock mais barato: não abre o JoinGate (só monta em
    // OPEN) nem agenda o poll da janela de entrada (só em TOO_EARLY).
    it('smoke de fluxo empilhado: TabBar ausente em /consultas/:id', async () => {
      vi.mocked(api.getMe).mockResolvedValue(conta);
      vi.mocked(api.getAppointment).mockResolvedValue({
        id: 'appt-mobile-1',
        status: 'CONFIRMED',
        starts_at: '2026-07-25T09:00:00-03:00',
        ends_at: '2026-07-25T09:25:00-03:00',
        time_zone: 'America/Sao_Paulo',
        specialty: { id: 'esp-1', name: 'Psicologia' },
        professional: { id: 'prof-1', full_name: 'Ana Beatriz Moura' },
        join: {
          status: 'UNAVAILABLE',
          opens_at: '2026-07-25T08:30:00-03:00',
          closes_at: '2026-07-25T09:25:00-03:00',
        },
      } satisfies Appointment);
      window.history.pushState({}, '', '/consultas/appt-mobile-1');
      renderApp();

      expect(await screen.findByRole('link', { name: 'Voltar' })).toBeInTheDocument();
      expect(screen.queryByRole('navigation', { name: 'Principal' })).not.toBeInTheDocument();
    });
  });
});
