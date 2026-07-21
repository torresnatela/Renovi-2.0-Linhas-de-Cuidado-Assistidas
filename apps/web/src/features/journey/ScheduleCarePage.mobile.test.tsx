import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { UserEvent } from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  ApiError,
  type AnnotatedSlot,
  type AvailabilityPage,
  type CareAppointment,
  type Journey,
} from '../../shared/api';
import { mockViewport, type ViewportHandle } from '../../shared/viewport.testkit';
import { ScheduleCarePage } from './ScheduleCarePage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getAvailability: vi.fn(),
    createCareAppointment: vi.fn(),
    getJourney: vi.fn(),
    // O HelpNowMenu do FlowHeader só toca a rede no clique de "Pedir ajuda"
    // (useMutation, nunca no mount) — os testes não clicam, então não mockamos.
  };
});
const api = await import('../../shared/api');

function slot(over: Partial<AnnotatedSlot> = {}): AnnotatedSlot {
  return {
    id: 'slot-1',
    starts_at: '2026-07-20T09:00:00-03:00',
    ends_at: '2026-07-20T09:25:00-03:00',
    time_zone: 'America/Sao_Paulo',
    professional: { id: 'prof-1', full_name: 'Ana Beatriz Moura' },
    eligibility: { allowed: true, blocks: [] },
    ...over,
  };
}

const pagina: AvailabilityPage = {
  item_id: 'item-1',
  from: '2026-07-20',
  to: '2026-08-19',
  items: [
    slot(),
    slot({
      id: 'slot-2',
      starts_at: '2026-07-20T10:00:00-03:00',
      ends_at: '2026-07-20T10:25:00-03:00',
    }),
  ],
};

const journey: Journey = {
  enrollments: [
    {
      enrollment: {
        id: 'enr-1',
        care_line_code: 'SM',
        care_line_version: 1,
        status: 'ativa',
        valid_from: '2026-07-01T00:00:00-03:00',
        valid_until: '2026-12-31T00:00:00-03:00',
        periods: [],
      },
      care_line_name: 'Linha de Saúde Mental',
      items: [
        {
          item: {
            id: 'item-1',
            ref: 'aval-inicial',
            kind: 'CONSULTA',
            specialty_code: 'PSI',
            label: 'Avaliação inicial',
            recurrence: '4x por mês',
            sort_order: 1,
          },
          eligibility: { allowed: true, blocks: [] },
        },
      ],
      recent_events: [],
    },
  ],
};

const consultaOk: CareAppointment = {
  id: 'care-1',
  item_ref: 'aval-inicial',
  label: 'Avaliação inicial',
  status: 'confirmada',
  scheduled_at: '2026-07-20T09:00:00-03:00',
  time_zone: 'America/Sao_Paulo',
  booking_id: 'book-1',
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/jornada/agendar/item-1']}>
        <Routes>
          <Route path="/jornada/agendar/:itemId" element={<ScheduleCarePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function escolherProfissional(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /Escolher Ana Beatriz Moura/i }));
}
async function escolherDia(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /seg, 20\/07/i }));
}
async function escolherHorario(user: UserEvent, hora: RegExp) {
  await user.click(await screen.findByRole('button', { name: hora }));
}
async function confirmar(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /Confirmar consulta/i }));
}

describe('ScheduleCarePage (mobile)', () => {
  let viewport: ViewportHandle;

  beforeEach(() => {
    vi.clearAllMocks();
    viewport = mockViewport('mobile');
    vi.mocked(api.getAvailability).mockResolvedValue(pagina);
    vi.mocked(api.getJourney).mockResolvedValue(journey);
  });

  afterEach(() => {
    viewport.restore();
  });

  it('mostra o FlowHeader com eyebrow, título do passo e progresso', async () => {
    renderPage();

    await screen.findByRole('button', { name: /Escolher Ana Beatriz Moura/i });
    expect(screen.getByText('Agendar · Avaliação inicial')).toBeInTheDocument();
    expect(screen.getByText('Com quem?')).toBeInTheDocument();
    expect(screen.getByText('Passo 1 de 3')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    // O stepper vertical do desktop NÃO aparece no mobile.
    expect(screen.queryByText(/leva menos de 1 minuto/i)).not.toBeInTheDocument();
  });

  it('avança pelos passos com o título do FlowHeader acompanhando', async () => {
    const user = userEvent.setup();
    renderPage();

    await escolherProfissional(user);
    expect(screen.getByText('Que dia?')).toBeInTheDocument();
    expect(screen.getByText('Passo 2 de 3')).toBeInTheDocument();
    // O calendário mostra o dia derivado da availability.
    expect(screen.getByRole('button', { name: /seg, 20\/07/i })).toBeInTheDocument();

    await escolherDia(user);
    expect(screen.getByText('Que horário?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Horário das 09:00/i })).toBeInTheDocument();
  });

  /**
   * A regra de ouro do calendário sob o fuso da agenda: um slot às 23:30 em −03:00
   * é 02:30 do dia seguinte em UTC. O dia derivado (20/07) tem que aparecer como
   * disponível; 21/07 (o que um cálculo no fuso do runner produziria) fica inerte.
   */
  it('não sofre off-by-one: slot −03:00 perto da meia-noite fica no dia certo', async () => {
    const user = userEvent.setup();
    vi.mocked(api.getAvailability).mockResolvedValue({
      ...pagina,
      items: [slot({ starts_at: '2026-07-20T23:30:00-03:00', ends_at: '2026-07-20T23:55:00-03:00' })],
    });
    renderPage();

    await escolherProfissional(user);
    expect(screen.getByRole('button', { name: /seg, 20\/07/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /21\/07/i })).not.toBeInTheDocument();
  });

  it('"Trocar" volta ao passo anterior e limpa as seleções posteriores', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    expect(await screen.findByRole('button', { name: /Confirmar consulta/i })).toBeInTheDocument();

    // "Trocar" no resumo do passo 3 volta ao passo 2 (navegação existente).
    await user.click(screen.getByRole('button', { name: 'Trocar' }));
    expect(screen.getByText('Que dia?')).toBeInTheDocument();

    // Reentrando no passo 3, a intenção foi limpa — nada a confirmar.
    await escolherDia(user);
    expect(screen.queryByRole('button', { name: /Confirmar consulta/i })).not.toBeInTheDocument();
  });

  /**
   * Espelho da idempotência no mobile: a Idempotency-Key nasce com a INTENÇÃO. Um
   * retry do MESMO horário reusa a MESMA key; outro horário renova a key.
   */
  it('mantém a Idempotency-Key no retry e a troca ao escolher outro horário', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);

    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(1));

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Confirmar consulta/i })).toBeEnabled(),
    );
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(2));

    await escolherHorario(user, /Horário das 10:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(3));

    const calls = vi.mocked(api.createCareAppointment).mock.calls;
    expect(calls[0][1]).toBe(calls[1][1]); // mesma intenção → MESMA key
    expect(calls[2][1]).not.toBe(calls[0][1]); // outro horário → key NOVA
    expect(calls[0][1]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-/i);
  });

  it('no sucesso mostra a oferta; "Agora não" leva à jornada', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockResolvedValue(consultaOk);
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);

    expect(await screen.findByText(/consulta marcada/i)).toBeInTheDocument();
    expect(screen.getByText('Tudo certo')).toBeInTheDocument();
    // Sem progresso no sucesso.
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    // Oferta com cópia verbatim da recorrência (nada de contagem inventada).
    expect(screen.getByText(/quer deixar a próxima marcada/i)).toBeInTheDocument();
    expect(screen.getByText('4x por mês')).toBeInTheDocument();
    // "Agora não" é um link para a jornada.
    expect(screen.getByRole('link', { name: /agora não — voltar à jornada/i })).toHaveAttribute(
      'href',
      '/jornada',
    );
  });

  it('"Agendar a próxima" volta ao passo 2 com nova intenção', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockResolvedValue(consultaOk);
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);
    await screen.findByText(/consulta marcada/i);

    // Captura a key da 1ª intenção para provar que a próxima é DIFERENTE.
    const keyAnterior = vi.mocked(api.createCareAppointment).mock.calls[0][1];

    await user.click(screen.getByRole('button', { name: /Agendar a próxima/i }));

    // Voltou ao passo 2 (mesmo profissional), calendário à mostra.
    expect(screen.getByText('Que dia?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /seg, 20\/07/i })).toBeInTheDocument();

    // Nova intenção: escolher o mesmo horário gera uma key NOVA.
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(2));
    const keyNova = vi.mocked(api.createCareAppointment).mock.calls[1][1];
    expect(keyNova).not.toBe(keyAnterior);
  });

  /**
   * Regressão: reservar o ÚLTIMO horário livre faz o refetch do `onSettled`
   * devolver disponibilidade VAZIA. O sucesso é hasteado acima do gate de slots —
   * a confirmação PERMANECE e o Empty ("Nenhum horário disponível") NÃO toma o seu
   * lugar (o FlowHeader ainda diz "Tudo certo").
   */
  it('o sucesso permanece quando o refetch devolve a lista vazia (último horário)', async () => {
    const user = userEvent.setup();
    // 1ª chamada (mount): há horários; a partir do refetch pós-sucesso: vazio.
    vi.mocked(api.getAvailability)
      .mockResolvedValueOnce(pagina)
      .mockResolvedValue({ ...pagina, items: [] });
    vi.mocked(api.createCareAppointment).mockResolvedValue(consultaOk);
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);

    // O sucesso aparece…
    expect(await screen.findByText(/consulta marcada/i)).toBeInTheDocument();
    // …e SOBREVIVE ao refetch que esvaziou a disponibilidade (2ª chamada+).
    await waitFor(() =>
      expect(vi.mocked(api.getAvailability).mock.calls.length).toBeGreaterThanOrEqual(2),
    );
    expect(screen.getByText(/consulta marcada/i)).toBeInTheDocument();
    // O Empty NÃO substitui a confirmação, e o FlowHeader segue coerente.
    expect(screen.queryByText('Nenhum horário disponível')).not.toBeInTheDocument();
    expect(screen.getByText('Tudo certo')).toBeInTheDocument();
  });
});
