import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { Account, ConsentStatus, Journey } from '../../shared/api';
import { mockViewport } from '../../shared/viewport.testkit';
import { ProfilePage } from './ProfilePage';

// Só o useNavigate é espionado; o resto do router (MemoryRouter, âncoras) é real.
const navigateSpy = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigateSpy };
});

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getMe: vi.fn(),
    getJourney: vi.fn(),
    getConsent: vi.fn(),
    revokeConsent: vi.fn(),
    logout: vi.fn(),
  };
});
const api = await import('../../shared/api');

const conta: Account = {
  id: 'u1',
  full_name: 'Ana Beatriz Silva',
  email: 'ana.silva@email.com',
};

const journey: Journey = {
  enrollments: [
    {
      enrollment: {
        id: 'e1',
        care_line_code: 'SAUDE_MENTAL',
        care_line_version: 1,
        status: 'ativa',
        valid_from: '2026-01-01T00:00:00-03:00',
        valid_until: '2026-09-30T00:00:00-03:00',
        periods: [],
      },
      care_line_name: 'Saúde Mental',
      items: [
        {
          item: {
            id: 'i1',
            ref: 'psico',
            kind: 'CONSULTA',
            specialty_code: 'PSICOLOGIA',
            label: 'Psicologia',
            recurrence: '4x por mês',
            sort_order: 1,
          },
          eligibility: { allowed: true, blocks: [] },
        },
        {
          item: {
            id: 'i2',
            ref: 'psiq',
            kind: 'CONSULTA',
            specialty_code: 'PSIQUIATRIA',
            label: 'Psiquiatria',
            recurrence: '1x por mês',
            sort_order: 2,
          },
          eligibility: {
            allowed: false,
            blocks: [
              { rule_type: 'QUOTA', reason: 'Você já usou sua consulta deste mês.' },
            ],
          },
        },
      ],
      recent_events: [],
    },
  ],
};

const consentAtivo: ConsentStatus = {
  finalidade: 'checkin_humor',
  active: true,
  versao_termo: 'v1',
  concedido_em: '2026-07-01T10:00:00-03:00',
};

const consentInativo: ConsentStatus = {
  finalidade: 'checkin_humor',
  active: false,
  versao_termo: null,
  concedido_em: null,
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/perfil']}>
        <ProfilePage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('ProfilePage', () => {
  beforeEach(() => {
    vi.mocked(api.getMe).mockResolvedValue(conta);
    vi.mocked(api.getJourney).mockResolvedValue(journey);
    vi.mocked(api.getConsent).mockResolvedValue(consentAtivo);
    vi.mocked(api.revokeConsent).mockReset();
    vi.mocked(api.logout).mockReset();
    navigateSpy.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('mostra nome e e-mail reais da conta', async () => {
    renderPage();
    expect(await screen.findByRole('heading', { name: 'Ana Beatriz Silva' })).toBeInTheDocument();
    expect(screen.getAllByText('ana.silva@email.com').length).toBeGreaterThan(0);
  });

  it('mostra o plano com o nome da linha e a vigência formatada no fuso', async () => {
    renderPage();
    // Vigência 30/09/2026 é formatada no fuso de São Paulo (nunca no do browser).
    expect(await screen.findByText('30/09/2026')).toBeInTheDocument();
    expect(screen.getAllByText('Saúde Mental').length).toBeGreaterThan(0);
    expect(screen.getByText('Psicologia')).toBeInTheDocument();
  });

  it('exibe o motivo do bloqueio de um item da linha', async () => {
    renderPage();
    expect(
      await screen.findByText('Você já usou sua consulta deste mês.'),
    ).toBeInTheDocument();
  });

  /**
   * Regressão (bug ao vivo): `GET /me/journey` devolve TODA matrícula, inclusive
   * encerrada/expirada — a seção "Plano e cobertura" mostrava um bloco completo
   * (banner + "Sua linha inclui") por matrícula, ativa ou não. Mesma regra já
   * aplicada nos chips da Jornada (commit 4449cb2): histórico de planos não é
   * papel do Perfil v1 — só a(s) matrícula(s) `ativa` aparecem aqui.
   */
  it('mostra só a matrícula ativa; a encerrada não aparece', async () => {
    const duasMatriculas: Journey = {
      enrollments: [
        ...journey.enrollments,
        {
          enrollment: {
            id: 'e2-encerrada',
            care_line_code: 'ORTOPEDIA',
            care_line_version: 1,
            status: 'encerrada',
            valid_from: '2025-01-01T00:00:00-03:00',
            valid_until: '2025-06-30T00:00:00-03:00',
            periods: [],
          },
          care_line_name: 'Ortopedia (encerrada)',
          items: [
            {
              item: {
                id: 'i3',
                ref: 'orto',
                kind: 'CONSULTA',
                specialty_code: 'ORTOPEDIA',
                label: 'Ortopedia',
                sort_order: 1,
              },
              eligibility: { allowed: true, blocks: [] },
            },
          ],
          recent_events: [],
        },
      ],
    };
    vi.mocked(api.getJourney).mockResolvedValue(duasMatriculas);

    renderPage();

    expect(await screen.findAllByText('Saúde Mental')).not.toHaveLength(0);
    expect(screen.queryByText('Ortopedia (encerrada)')).not.toBeInTheDocument();
  });

  it('revoga o consentimento quando o usuário confirma', async () => {
    // Após revogar, o status relido volta inativo (invalidação → refetch).
    vi.mocked(api.getConsent)
      .mockResolvedValueOnce(consentAtivo)
      .mockResolvedValue(consentInativo);
    vi.mocked(api.revokeConsent).mockResolvedValue(consentInativo);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: /revogar consentimento/i }));

    expect(confirmSpy).toHaveBeenCalled();
    expect(api.revokeConsent).toHaveBeenCalledWith('checkin_humor');
    // A revogação reflete no estado: a ação some quando o consentimento fica inativo.
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: /revogar consentimento/i }),
      ).not.toBeInTheDocument(),
    );
  });

  // Falha silenciosa numa ação LGPD é inaceitável: se a revogação falha, o
  // paciente precisa ver que NÃO foi revogado (senão pode achar que saiu do
  // registro sem ter saído).
  it('mostra erro visível quando a revogação falha', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.mocked(api.revokeConsent).mockRejectedValue(new Error('rede caiu'));

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: /revogar consentimento/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/não foi possível revogar/i);
  });

  it('não revoga quando o usuário cancela o confirm', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: /revogar consentimento/i }));

    expect(confirmSpy).toHaveBeenCalled();
    expect(api.revokeConsent).not.toHaveBeenCalled();
  });

  it('sai da conta e redireciona para o login', async () => {
    vi.mocked(api.logout).mockResolvedValue(undefined);

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: /sair da conta/i }));

    await waitFor(() => expect(api.logout).toHaveBeenCalled());
    await waitFor(() => expect(navigateSpy).toHaveBeenCalledWith('/entrar'));
  });

  // --- Mobile: tela raiz (Etapa 5) ---

  describe('no mobile (tela raiz)', () => {
    let viewport: ReturnType<typeof mockViewport>;

    beforeEach(() => {
      viewport = mockViewport('mobile');
    });

    afterEach(() => {
      viewport.restore();
    });

    /**
     * O cabeçalho de duas linhas do desktop ("Sua conta" / "Perfil" 32px) dá
     * lugar ao padrão de tela raiz: eyebrow "Seu perfil" + título 26px + Pedir
     * ajuda — mesmo padrão do mock da Jornada.
     */
    it('mostra o cabeçalho padrão de tela raiz com Pedir ajuda', async () => {
      renderPage();

      expect(await screen.findByText('Seu perfil')).toBeInTheDocument();
      expect(screen.getByText('Perfil')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /Pedir ajuda/i })).toBeInTheDocument();
      // O cabeçalho de desktop não coexiste com o mobile.
      expect(screen.queryByText('Sua conta')).not.toBeInTheDocument();
    });

    /**
     * Ordem mobile sensata (regra do brief): resumo → dados → plano →
     * privacidade → Sair. No desktop o Sair mora dentro do aside (logo depois
     * do resumo); no mobile ele precisa ir para o FIM, depois de Privacidade.
     */
    it('reordena o Sair da conta para o fim da pilha de seções', async () => {
      renderPage();

      await screen.findByRole('heading', { name: 'Ana Beatriz Silva' });
      const texto = document.body.textContent ?? '';

      const posResumo = texto.indexOf('Ana Beatriz Silva');
      const posDados = texto.indexOf('Dados pessoais');
      const posPlano = texto.indexOf('Plano e cobertura');
      const posPrivacidade = texto.indexOf('Privacidade e segurança');
      const posSair = texto.indexOf('Sair da conta');

      expect(posResumo).toBeGreaterThanOrEqual(0);
      expect(posDados).toBeGreaterThan(posResumo);
      expect(posPlano).toBeGreaterThan(posDados);
      expect(posPrivacidade).toBeGreaterThan(posPlano);
      expect(posSair).toBeGreaterThan(posPrivacidade);
    });

    // A revogação real ainda precisa funcionar dentro do novo layout mobile —
    // o gate de LGPD não é intocável só no desktop.
    it('revoga o consentimento no mobile igual ao desktop', async () => {
      vi.mocked(api.getConsent)
        .mockResolvedValueOnce(consentAtivo)
        .mockResolvedValue(consentInativo);
      vi.mocked(api.revokeConsent).mockResolvedValue(consentInativo);
      vi.spyOn(window, 'confirm').mockReturnValue(true);

      const user = userEvent.setup();
      renderPage();

      await user.click(await screen.findByRole('button', { name: /revogar consentimento/i }));

      expect(api.revokeConsent).toHaveBeenCalledWith('checkin_humor');
      await waitFor(() =>
        expect(
          screen.queryByRole('button', { name: /revogar consentimento/i }),
        ).not.toBeInTheDocument(),
      );
    });
  });
});
