import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  ApiError,
  type AnnotatedSlot,
  type AvailabilityPage,
  type CareAppointment,
} from '../../shared/api';
import { ScheduleCarePage } from './ScheduleCarePage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getAvailability: vi.fn(), createCareAppointment: vi.fn() };
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
      starts_at: '2026-07-21T14:00:00-03:00',
      ends_at: '2026-07-21T14:25:00-03:00',
    }),
  ],
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

describe('ScheduleCarePage', () => {
  beforeEach(() => {
    // clearAllMocks zera o HISTÓRICO de chamadas entre os testes — sem isso o
    // toHaveBeenCalledTimes do teste de sucesso somaria as chamadas dos anteriores.
    vi.clearAllMocks();
    vi.mocked(api.getAvailability).mockResolvedValue(pagina);
  });

  /**
   * A regra de ouro: um horário barrado pelo motor NO POST (422) não vira erro
   * mudo — mostra os `blocks` que o servidor mandou, com a frase pronta e a data
   * de desbloqueio quando há.
   */
  it('no 422 mostra os blocks retornados pelo motor', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(422, 'Bloqueado', 'texto', { code: 'ELIGIBILITY_BLOCKED' }, [
        { rule_type: 'QUOTA', reason: 'Você já usou sua consulta desta semana.' },
        {
          rule_type: 'MIN_INTERVAL',
          reason: 'Aguarde o intervalo mínimo entre consultas.',
          available_from: '2026-08-01T00:00:00-03:00',
        },
      ]),
    );
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Agendar horário das 09:00/i }));

    expect(await screen.findByText(/já usou sua consulta desta semana/i)).toBeInTheDocument();
    expect(screen.getByText(/aguarde o intervalo mínimo/i)).toBeInTheDocument();
    // A data de desbloqueio, formatada no fuso da agenda (em UTC sairia outro dia).
    expect(screen.getByText(/01\/08/)).toBeInTheDocument();
  });

  /**
   * O coração da idempotência: a Idempotency-Key nasce com a INTENÇÃO, não com a
   * chamada. Um retry da MESMA intenção reusa a MESMA key (o servidor então
   * responde a mesma consulta, sem duplicar); escolher OUTRO horário é outra
   * intenção, e nasce outra key.
   */
  it('mantém a Idempotency-Key no retry e a troca ao escolher outro horário', async () => {
    const user = userEvent.setup();
    // Falha sempre: o que importa aqui são as KEYS de cada tentativa, não o desfecho.
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    // 1ª tentativa no slot das 09:00.
    await user.click(await screen.findByRole('button', { name: /Agendar horário das 09:00/i }));
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(1));

    // Retry do MESMO horário (mesma intenção).
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Agendar horário das 09:00/i })).toBeEnabled(),
    );
    await user.click(screen.getByRole('button', { name: /Agendar horário das 09:00/i }));
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(2));

    // Agora um horário DIFERENTE (outra intenção).
    await user.click(screen.getByRole('button', { name: /Agendar horário das 14:00/i }));
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(3));

    const calls = vi.mocked(api.createCareAppointment).mock.calls;
    // createCareAppointment(body, idemKey) — a key é o 2º argumento.
    expect(calls[0][1]).toBe(calls[1][1]); // mesma intenção → MESMA key
    expect(calls[2][1]).not.toBe(calls[0][1]); // outro horário → key NOVA
    expect(calls[0][1]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-/i); // é um UUID gerado no cliente
  });

  /**
   * Enquanto um agendamento está em voo (a DAV leva ~29s), TODOS os botões
   * travam: como o hook é uma única useMutation, clicar outro horário dispararia
   * um segundo POST concorrente — o banner do primeiro sumiria e a corrida de cota
   * do servidor se alargaria. O paciente espera este confirmar e marca o próximo.
   */
  it('trava os demais horários enquanto um agendamento está em voo', async () => {
    const user = userEvent.setup();
    let resolver!: (v: CareAppointment) => void;
    vi.mocked(api.createCareAppointment).mockImplementation(
      () =>
        new Promise<CareAppointment>((res) => {
          resolver = res;
        }),
    );
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Agendar horário das 09:00/i }));
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(1));

    // Com o 1º em voo, o OUTRO horário está desabilitado — não há como disparar 2º POST.
    expect(screen.getByRole('button', { name: /Agendar horário das 14:00/i })).toBeDisabled();
    expect(api.createCareAppointment).toHaveBeenCalledTimes(1);

    // Concluído o 1º, os botões voltam e o paciente segue marcando.
    resolver({
      id: 'care-1',
      item_ref: 'aval',
      label: 'Avaliação',
      status: 'confirmada',
      scheduled_at: '2026-07-20T09:00:00-03:00',
      time_zone: 'America/Sao_Paulo',
      booking_id: 'book-1',
    });
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Agendar horário das 14:00/i })).toBeEnabled(),
    );
  });

  /**
   * O caso "agendar o mês de uma vez": após o sucesso a tela FICA, mostra a
   * confirmação e refaz a disponibilidade (o horário tomado some), para o paciente
   * seguir marcando sem navegar para fora.
   */
  it('no sucesso fica na tela e refaz a disponibilidade', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockResolvedValue({
      id: 'care-1',
      item_ref: 'aval-inicial',
      label: 'Avaliação inicial',
      status: 'confirmada',
      scheduled_at: '2026-07-20T09:00:00-03:00',
      time_zone: 'America/Sao_Paulo',
      booking_id: 'book-1',
    });
    renderPage();

    await user.click(await screen.findByRole('button', { name: /Agendar horário das 09:00/i }));

    expect(await screen.findByText(/consulta agendada/i)).toBeInTheDocument();
    // A disponibilidade é refeita (invalidação): a 1ª carga + o refetch = 2 chamadas.
    await waitFor(() => expect(api.getAvailability).toHaveBeenCalledTimes(2));
    // E a tela continua ali — o paciente pode marcar o próximo.
    expect(screen.getByRole('button', { name: /Agendar horário das 14:00/i })).toBeInTheDocument();
  });
});
