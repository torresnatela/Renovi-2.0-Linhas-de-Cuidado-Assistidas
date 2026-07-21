import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ApiError, type Appointment, type MoodToday } from '../../shared/api';
import { moodKeys } from '../mood/useMood';
import { AppointmentPage } from './AppointmentsPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getAppointment: vi.fn(),
    joinAppointment: vi.fn(),
    // O gate de pré-consulta consulta o humor de hoje e (se houver oferta) o
    // instrumento periódico. Mockados para não bater na rede nos testes de join.
    getMoodToday: vi.fn(),
    getAssessmentAvailability: vi.fn(),
    submitAssessment: vi.fn(),
  };
});
// O jsdom não deixa espionar window.location — por isso a navegação externa é um
// módulo, e não uma linha solta dentro do componente.
vi.mock('../../shared/navigate', () => ({ openExternal: vi.fn() }));

const api = await import('../../shared/api');
const nav = await import('../../shared/navigate');

const LINK = 'https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz';

function today(over: Partial<MoodToday> = {}): MoodToday {
  return { dia: '2026-07-20', can_checkin: false, offer: null, ...over };
}

function consulta(over: Partial<Appointment> = {}): Appointment {
  return {
    id: 'appt-1',
    status: 'CONFIRMED',
    starts_at: '2026-07-20T09:00:00-03:00',
    ends_at: '2026-07-20T09:25:00-03:00',
    time_zone: 'America/Sao_Paulo',
    specialty: { id: 'esp-1', name: 'Psicologia' },
    professional: { id: 'prof-1', full_name: 'Ana Beatriz Moura' },
    join: {
      status: 'OPEN',
      opens_at: '2026-07-20T08:30:00-03:00',
      closes_at: '2026-07-20T09:25:00-03:00',
    },
    ...over,
  };
}

function renderPage(opts: { today?: MoodToday } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // Semeia o humor de hoje ANTES do render: garante que `today.data` já esteja
  // presente no clique (o gate é avaliado no clique, e sem semear haveria corrida
  // entre o resolve da query e o userEvent).
  if (opts.today) client.setQueryData(moodKeys.today(), opts.today);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/consultas/appt-1']}>
        <Routes>
          <Route path="/consultas/:appointmentId" element={<AppointmentPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AppointmentPage', () => {
  beforeEach(() => {
    vi.mocked(api.joinAppointment).mockReset();
    vi.mocked(nav.openExternal).mockReset();
    // Padrão sem oferta: o gate fica inerte e o join corre direto — é o que os
    // testes de janela/join abaixo assumem.
    vi.mocked(api.getMoodToday).mockResolvedValue(today({ offer: null }));
  });

  /**
   * A regra de ouro do apps/web/CLAUDE.md, como teste: nunca um botão só
   * desabilitado — sempre o motivo. E o motivo é uma HORA que o servidor mandou.
   */
  it('fora da janela, mostra a hora de abertura em vez de um botão morto', async () => {
    vi.mocked(api.getAppointment).mockResolvedValue(
      consulta({
        join: {
          status: 'TOO_EARLY',
          opens_at: '2026-07-20T08:30:00-03:00',
          closes_at: '2026-07-20T09:25:00-03:00',
          reason: { code: 'JOIN_TOO_EARLY' },
        },
      }),
    );
    renderPage();

    expect(await screen.findByText(/a partir das 08:30/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Entrar na consulta/i })).not.toBeInTheDocument();
  });

  /**
   * "30 minutos" não pode existir no front: o que a tela usa é o `opens_at` que
   * veio pronto. Se o servidor mudar a antecedência, a tela acompanha sem deploy —
   * é o que este teste crava, mandando uma janela de 2h.
   */
  it('obedece ao opens_at do servidor, não a uma regra decorada', async () => {
    vi.mocked(api.getAppointment).mockResolvedValue(
      consulta({
        join: {
          status: 'TOO_EARLY',
          opens_at: '2026-07-20T07:00:00-03:00',
          closes_at: '2026-07-20T09:25:00-03:00',
          reason: { code: 'JOIN_TOO_EARLY' },
        },
      }),
    );
    renderPage();

    expect(await screen.findByText(/a partir das 07:00/i)).toBeInTheDocument();
  });

  it('com a janela aberta, entrar abre a sala', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    vi.mocked(api.joinAppointment).mockResolvedValue({ url: LINK });
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Entrar na consulta/i }));

    await waitFor(() => expect(nav.openExternal).toHaveBeenCalledWith(LINK));
  });

  // O botão só apareceu porque o join.status dizia OPEN — mas quem manda é o
  // servidor no momento do clique (relógio adiantado, cache velho).
  it('traduz o 409 de janela fechada no clique', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    vi.mocked(api.joinAppointment).mockRejectedValue(
      new ApiError(409, 'Fora da janela', 'texto da api', { code: 'JOIN_TOO_LATE' }),
    );
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Entrar na consulta/i }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/já terminou/i);
    expect(nav.openExternal).not.toHaveBeenCalled();
  });

  /**
   * UNCONFIRMED é o estado que o ADR-016 criou: a consulta PODE existir na DAV e
   * nunca vamos saber sozinhos. Escondê-la seria pior que a incerteza — o paciente
   * pode ter uma consulta de verdade marcada.
   */
  it('explica a consulta que não conseguimos confirmar', async () => {
    vi.mocked(api.getAppointment).mockResolvedValue(
      consulta({
        status: 'UNCONFIRMED',
        join: {
          status: 'UNAVAILABLE',
          opens_at: '2026-07-20T08:30:00-03:00',
          closes_at: '2026-07-20T09:25:00-03:00',
          reason: { code: 'JOIN_UNAVAILABLE' },
        },
      }),
    );
    renderPage();

    expect(await screen.findByText('Verificando')).toBeInTheDocument();
    expect(screen.getByText(/ainda estamos confirmando/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Entrar na consulta/i })).not.toBeInTheDocument();
  });

  it('mostra a consulta cancelada sem oferecer entrada', async () => {
    vi.mocked(api.getAppointment).mockResolvedValue(
      consulta({
        status: 'CANCELLED',
        join: {
          status: 'UNAVAILABLE',
          opens_at: '2026-07-20T08:30:00-03:00',
          closes_at: '2026-07-20T09:25:00-03:00',
          reason: { code: 'JOIN_CANCELLED' },
        },
      }),
    );
    renderPage();

    expect(await screen.findByText('Cancelada')).toBeInTheDocument();
    expect(screen.getByText(/foi cancelada/i)).toBeInTheDocument();
  });

  it('mostra data e hora no fuso da agenda', async () => {
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    renderPage();
    // Em UTC isto sairia 12:00 — o runner de CI pegaria a implementação ingênua.
    expect(await screen.findByText(/segunda-feira, 20 de julho às 09:00/i)).toBeInTheDocument();
  });

  // --- Gate de pré-consulta (decisão de produto, 2026-07-20) ---

  /**
   * Com uma oferta ativa (WHO-5), o clique em "Entrar" mostra o instrumento
   * PRIMEIRO — a sala não abre ainda. Só depois de responder e concluir é que o
   * join corre. O gate coleta antes da consulta, mas nunca no lugar dela.
   */
  it('com oferta, mostra o instrumento antes e só entra após concluir', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    vi.mocked(api.getMoodToday).mockResolvedValue(today({ offer: 'WHO5' }));
    vi.mocked(api.getAssessmentAvailability).mockResolvedValue({
      codigo: 'WHO5',
      eligibility: { allowed: true, blocks: [] },
      item_count: 5,
      value_min: 0,
      value_max: 5,
    });
    vi.mocked(api.submitAssessment).mockResolvedValue({
      codigo: 'WHO5',
      index_score: 80,
      faixa: 'normal',
      flag_encaminhar: false,
      respondido_em: '2026-07-20T08:40:00-03:00',
    });
    vi.mocked(api.joinAppointment).mockResolvedValue({ url: LINK });
    renderPage({ today: today({ offer: 'WHO5' }) });

    await user.click(await screen.findByRole('button', { name: /Entrar na consulta/i }));

    // O instrumento aparece; a sala NÃO abre ainda.
    expect(await screen.findByText(/Antes da consulta/i)).toBeInTheDocument();
    expect(api.joinAppointment).not.toHaveBeenCalled();

    // Responde os 5 itens do WHO-5 (o primeiro rótulo de cada) e envia.
    for (const botao of screen.getAllByRole('button', { name: /Em nenhum momento/i })) {
      await user.click(botao);
    }
    await user.click(screen.getByRole('button', { name: /Enviar respostas/i }));

    // Concluído o instrumento, a sala abre.
    await user.click(await screen.findByRole('button', { name: /Concluir/i }));
    await waitFor(() => expect(nav.openExternal).toHaveBeenCalledWith(LINK));
  });

  it('sem oferta, entra direto sem instrumento', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    vi.mocked(api.joinAppointment).mockResolvedValue({ url: LINK });
    renderPage({ today: today({ offer: null }) });

    await user.click(await screen.findByRole('button', { name: /Entrar na consulta/i }));

    await waitFor(() => expect(nav.openExternal).toHaveBeenCalledWith(LINK));
    expect(screen.queryByText(/Antes da consulta/i)).not.toBeInTheDocument();
  });

  /**
   * Falha ao determinar a oferta (o `today` erra) NÃO pode prender o paciente: o
   * gate é opcional, a consulta não. Sem `offer`, o clique entra direto.
   */
  it('se o instrumento falha, não bloqueia a entrada', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAppointment).mockResolvedValue(consulta());
    vi.mocked(api.getMoodToday).mockRejectedValue(new Error('humor indisponível'));
    vi.mocked(api.joinAppointment).mockResolvedValue({ url: LINK });
    renderPage(); // sem semear today → a query erra e `offer` fica indefinido

    await user.click(await screen.findByRole('button', { name: /Entrar na consulta/i }));

    await waitFor(() => expect(nav.openExternal).toHaveBeenCalledWith(LINK));
    expect(screen.queryByText(/Antes da consulta/i)).not.toBeInTheDocument();
  });
});
