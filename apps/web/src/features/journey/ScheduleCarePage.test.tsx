import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { UserEvent } from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import {
  ApiError,
  type AnnotatedSlot,
  type AvailabilityPage,
  type CareAppointment,
  type Journey,
} from '../../shared/api';
import { ScheduleCarePage } from './ScheduleCarePage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getAvailability: vi.fn(),
    createCareAppointment: vi.fn(),
    getJourney: vi.fn(),
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

// Dois horários do MESMO profissional no MESMO dia: assim a grade de horários
// mostra ambos sem trocar de dia — o que permite testar retry, troca de intenção
// e trava-em-voo na mesma tela.
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
            sort_order: 1,
          },
          eligibility: { allowed: true, blocks: [] },
        },
      ],
      recent_events: [],
    },
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

// Navegação do wizard: cada passo é um clique. Os defaults batem com a `pagina`.
async function escolherProfissional(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /Escolher Ana Beatriz Moura/i }));
}
async function escolherDia(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /20 de julho/i }));
}
async function escolherHorario(user: UserEvent, hora: RegExp) {
  await user.click(await screen.findByRole('button', { name: hora }));
}
async function confirmar(user: UserEvent) {
  await user.click(await screen.findByRole('button', { name: /Confirmar consulta/i }));
}

describe('ScheduleCarePage', () => {
  beforeEach(() => {
    // clearAllMocks zera o HISTÓRICO de chamadas entre os testes — sem isso o
    // toHaveBeenCalledTimes do teste de sucesso somaria as chamadas dos anteriores.
    vi.clearAllMocks();
    vi.mocked(api.getAvailability).mockResolvedValue(pagina);
    vi.mocked(api.getJourney).mockResolvedValue(journey);
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

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);

    expect(await screen.findByText(/já usou sua consulta desta semana/i)).toBeInTheDocument();
    expect(screen.getByText(/aguarde o intervalo mínimo/i)).toBeInTheDocument();
    // A data de desbloqueio, formatada no fuso da agenda (em UTC sairia outro dia).
    expect(screen.getByText(/01\/08/)).toBeInTheDocument();
  });

  /**
   * O coração da idempotência: a Idempotency-Key nasce com a INTENÇÃO (o clique no
   * horário), não com a chamada. Um retry da MESMA intenção (confirmar de novo)
   * reusa a MESMA key; escolher OUTRO horário é outra intenção, e nasce outra key.
   */
  it('mantém a Idempotency-Key no retry e a troca ao escolher outro horário', async () => {
    const user = userEvent.setup();
    // Falha sempre: o que importa aqui são as KEYS de cada tentativa, não o desfecho.
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);

    // 1ª tentativa no horário das 09:00.
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(1));

    // Retry da MESMA intenção (confirmar de novo).
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /Confirmar consulta/i })).toBeEnabled(),
    );
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(2));

    // Agora um horário DIFERENTE (outra intenção) e confirma.
    await escolherHorario(user, /Horário das 10:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(3));

    const calls = vi.mocked(api.createCareAppointment).mock.calls;
    // createCareAppointment(body, idemKey) — a key é o 2º argumento.
    expect(calls[0][1]).toBe(calls[1][1]); // mesma intenção → MESMA key
    expect(calls[2][1]).not.toBe(calls[0][1]); // outro horário → key NOVA
    expect(calls[0][1]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-/i); // é um UUID gerado no cliente
  });

  /**
   * Regressão: no 409 SLOT_TAKEN, o `onSettled` refaz a disponibilidade — e o
   * refetch LEGITIMAMENTE remove o horário reservado por outra pessoa da lista.
   * Isso não pode apagar a mensagem de erro (ela vivia dentro do gate de
   * `slotEscolhido`, derivado da própria lista): o paciente precisa continuar
   * vendo o porquê mesmo depois do horário sumir da grade.
   */
  it('mantém o erro visível quando o refetch remove o horário escolhido da lista', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    // 1ª chamada: os dois horários. Depois do 409, o refetch (onSettled) não
    // devolve mais o das 09:00 — outra pessoa acabou de ocupá-lo.
    vi.mocked(api.getAvailability).mockResolvedValueOnce(pagina).mockResolvedValue({
      ...pagina,
      items: [
        slot({
          id: 'slot-2',
          starts_at: '2026-07-20T10:00:00-03:00',
          ends_at: '2026-07-20T10:25:00-03:00',
        }),
      ],
    });
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);

    expect(await screen.findByText(/acabou de ser reservado por outra pessoa/i)).toBeInTheDocument();

    // O refetch aconteceu e removeu o horário escolhido da lista.
    await waitFor(() => expect(api.getAvailability).toHaveBeenCalledTimes(2));

    // A mensagem de erro CONTINUA visível — não pode sumir com o slot.
    expect(screen.getByText(/acabou de ser reservado por outra pessoa/i)).toBeInTheDocument();
  });

  /**
   * Enquanto um agendamento está em voo (a DAV leva ~29s), TODOS os horários
   * travam: clicar outro dispararia um segundo POST concorrente — o banner do
   * primeiro sumiria e a corrida de cota do servidor se alargaria.
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

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);
    await waitFor(() => expect(api.createCareAppointment).toHaveBeenCalledTimes(1));

    // Com o 1º em voo, o OUTRO horário está desabilitado — não há como disparar 2º POST.
    expect(screen.getByRole('button', { name: /Horário das 10:00/i })).toBeDisabled();
    expect(api.createCareAppointment).toHaveBeenCalledTimes(1);

    // Concluído o 1º, os horários voltam.
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
      expect(screen.getByRole('button', { name: /Horário das 10:00/i })).toBeEnabled(),
    );
  });

  /**
   * Após o sucesso a tela FICA, mostra a confirmação e refaz a disponibilidade (o
   * horário tomado some), para o paciente seguir marcando sem navegar para fora.
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

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    await confirmar(user);

    expect(await screen.findByText(/consulta agendada/i)).toBeInTheDocument();
    // A disponibilidade é refeita (invalidação): a 1ª carga + o refetch = 2 chamadas.
    await waitFor(() => expect(api.getAvailability).toHaveBeenCalledTimes(2));
    // E a tela continua ali — o paciente pode marcar o próximo horário.
    expect(screen.getByRole('button', { name: /Horário das 10:00/i })).toBeInTheDocument();
    // Com link para as consultas, sem navegar para fora.
    expect(screen.getByRole('link', { name: /ver minhas consultas/i })).toHaveAttribute(
      'href',
      '/consultas',
    );
  });

  /**
   * Regra de ouro por slot: um horário que o motor barra vira pill riscada
   * NÃO-clicável e o paciente lê o porquê (o `reason` pronto do servidor).
   */
  it('horário inelegível vira pill riscada não-clicável e mostra o motivo', async () => {
    const user = userEvent.setup();
    const bloqueado = slot({
      id: 'slot-3',
      starts_at: '2026-07-20T11:00:00-03:00',
      ends_at: '2026-07-20T11:25:00-03:00',
      eligibility: {
        allowed: false,
        blocks: [{ rule_type: 'QUOTA', reason: 'Você já usou sua consulta desta semana.' }],
      },
    });
    vi.mocked(api.getAvailability).mockResolvedValue({ ...pagina, items: [slot(), bloqueado] });
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);

    const pill = await screen.findByRole('button', { name: /Horário das 11:00, indisponível/i });
    expect(pill).toBeDisabled();
    expect(pill).toHaveClass('line-through');
    expect(screen.getByText(/já usou sua consulta desta semana/i)).toBeInTheDocument();
  });

  /**
   * Dedupe: o MESMO motivo repetido em vários horários aparece UMA vez (acima da
   * grade), não uma cópia por horário barrado.
   */
  it('deduplica o motivo repetido entre vários horários', async () => {
    const user = userEvent.setup();
    const barrado = (id: string, hh: string): AnnotatedSlot =>
      slot({
        id,
        starts_at: `2026-07-20T${hh}:00:00-03:00`,
        ends_at: `2026-07-20T${hh}:25:00-03:00`,
        eligibility: {
          allowed: false,
          blocks: [{ rule_type: 'QUOTA', reason: 'Você já usou sua consulta desta semana.' }],
        },
      });
    vi.mocked(api.getAvailability).mockResolvedValue({
      ...pagina,
      items: [barrado('slot-a', '11'), barrado('slot-b', '12')],
    });
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await screen.findByRole('button', { name: /Horário das 11:00, indisponível/i });

    expect(screen.getAllByText(/já usou sua consulta desta semana/i)).toHaveLength(1);
  });

  /**
   * O passo ativo é anunciado com `aria-current="step"` — e só ele.
   */
  it('marca o passo ativo com aria-current="step"', async () => {
    const user = userEvent.setup();
    renderPage();

    await screen.findByRole('button', { name: /Escolher Ana Beatriz Moura/i });
    expect(screen.getByText('Profissional').closest('[aria-current="step"]')).not.toBeNull();

    await escolherProfissional(user);
    expect(screen.getByText('Data').closest('[aria-current="step"]')).not.toBeNull();
    expect(screen.getByText('Profissional').closest('[aria-current="step"]')).toBeNull();
  });

  /**
   * Voltar a um passo anterior pelo stepper limpa as escolhas POSTERIORES (e a
   * intenção/key junto) — nada de confirmar um horário que já foi abandonado.
   */
  it('voltar ao passo anterior limpa as escolhas posteriores', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createCareAppointment).mockRejectedValue(
      new ApiError(409, 'Ocupado', 'texto', { code: 'SLOT_TAKEN' }),
    );
    renderPage();

    await escolherProfissional(user);
    await escolherDia(user);
    await escolherHorario(user, /Horário das 09:00/i);
    expect(await screen.findByRole('button', { name: /Confirmar consulta/i })).toBeInTheDocument();

    // Volta ao passo 1 pelo stepper.
    await user.click(screen.getByRole('button', { name: 'Profissional' }));

    // De volta à pergunta do passo 1, e sem card de confirmação (intenção limpa).
    expect(screen.getByText(/com quem você quer se consultar/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Confirmar consulta/i })).not.toBeInTheDocument();
  });
});
