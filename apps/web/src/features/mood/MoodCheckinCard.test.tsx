import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { MoodCheckin } from '../../shared/api';
import { MoodCheckinCard } from './MoodCheckinCard';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getMoodToday: vi.fn(),
    grantConsent: vi.fn(),
    recordMoodCheckin: vi.fn(),
  };
});
const api = await import('../../shared/api');

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/jornada']}>
        <MoodCheckinCard />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('MoodCheckinCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('pede consentimento, aceita e então permite registrar o check-in', async () => {
    // Antes: consentimento pendente. Depois de aceitar, o dia revalida e libera o check-in.
    vi.mocked(api.getMoodToday)
      .mockResolvedValueOnce({ dia: '2026-07-20', can_checkin: false, reason: 'consent_required' })
      .mockResolvedValue({ dia: '2026-07-20', can_checkin: true, checkin: null });
    vi.mocked(api.grantConsent).mockResolvedValue({ finalidade: 'checkin_humor', active: true });
    const salvo: MoodCheckin = {
      valencia: 50,
      energia: 50,
      quadrante: 'x',
      emotion_label: 'Neutro(a)',
      respondido_em: '2026-07-20T10:00:00-03:00',
    };
    vi.mocked(api.recordMoodCheckin).mockResolvedValue(salvo);

    const user = userEvent.setup();
    renderCard();

    await user.click(await screen.findByRole('button', { name: 'Aceitar e continuar' }));
    // Aceita com a MESMA versão de termo que a MoodPage usa.
    expect(api.grantConsent).toHaveBeenCalledWith('v1');

    // Revalidado o dia, a grade aparece e o check-in pode ser registrado.
    await screen.findByRole('button', { name: 'Grade de humor: valência por energia' });
    await user.click(screen.getByRole('button', { name: 'Registrar' }));
    await waitFor(() => expect(api.recordMoodCheckin).toHaveBeenCalled());
  });

  it('seleciona no grid pelo teclado e envia o shape correto ao registrar', async () => {
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: true,
      checkin: null,
    });
    const salvo: MoodCheckin = {
      valencia: 55,
      energia: 55,
      quadrante: 'x',
      emotion_label: 'Neutro(a)',
      respondido_em: '2026-07-20T10:00:00-03:00',
    };
    vi.mocked(api.recordMoodCheckin).mockResolvedValue(salvo);

    renderCard();

    const grade = await screen.findByRole('button', {
      name: 'Grade de humor: valência por energia',
    });
    grade.focus();
    // Do centro (50,50): uma seta à direita e uma para cima => (55, 55).
    fireEvent.keyDown(grade, { key: 'ArrowRight' });
    fireEvent.keyDown(grade, { key: 'ArrowUp' });
    expect(screen.getByTestId('mood-value')).toHaveTextContent(
      'valência 55 de 100, energia 55 de 100',
    );

    fireEvent.click(screen.getByRole('button', { name: 'Registrar' }));

    await waitFor(() => expect(api.recordMoodCheckin).toHaveBeenCalled());
    // Envia valência/energia e o rótulo da região como emotion_label.
    expect(vi.mocked(api.recordMoodCheckin).mock.calls[0][0]).toMatchObject({
      valencia: 55,
      energia: 55,
      emotion_label: 'Neutro(a)',
    });
  });

  it('mostra o estado feito colapsado (sem "Refazer" quando a API não permite)', async () => {
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: false,
      checkin: {
        valencia: 80,
        energia: 20,
        quadrante: 'agradavel_calmo',
        emotion_label: 'Tranquilo(a)',
        respondido_em: '2026-07-20T09:00:00-03:00',
      },
    });

    renderCard();

    expect(
      await screen.findByText(/Check-in de hoje feito: Tranquilo\(a\)/),
    ).toBeInTheDocument();
    expect(screen.getByText('Amanhã a gente se fala de novo.')).toBeInTheDocument();
    // can_checkin=false → honestidade: não oferece refazer o que a API não permite.
    expect(screen.queryByRole('button', { name: /refazer/i })).not.toBeInTheDocument();
  });

  it('após Refazer + registrar com sucesso, o card colapsa de volta ao estado feito', async () => {
    // Regressão: "Refazer" setava um estado local que nunca era resetado, então
    // mesmo depois de um novo registro bem-sucedido o card ficava preso na grade.
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: true,
      checkin: {
        valencia: 80,
        energia: 20,
        quadrante: 'agradavel_calmo',
        emotion_label: 'Tranquilo(a)',
        respondido_em: '2026-07-20T09:00:00-03:00',
      },
    });
    const novoSalvo: MoodCheckin = {
      valencia: 55,
      energia: 55,
      quadrante: 'x',
      emotion_label: 'Bem',
      respondido_em: '2026-07-20T10:00:00-03:00',
    };
    vi.mocked(api.recordMoodCheckin).mockResolvedValue(novoSalvo);

    const user = userEvent.setup();
    renderCard();

    expect(
      await screen.findByText(/Check-in de hoje feito: Tranquilo\(a\)/),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /refazer/i }));

    // Refazendo: a grade reaparece.
    const grade = await screen.findByRole('button', {
      name: 'Grade de humor: valência por energia',
    });
    grade.focus();
    fireEvent.keyDown(grade, { key: 'ArrowRight' });
    fireEvent.keyDown(grade, { key: 'ArrowUp' });

    await user.click(screen.getByRole('button', { name: 'Registrar' }));
    await waitFor(() => expect(api.recordMoodCheckin).toHaveBeenCalled());

    // O card COLAPSA de volta ao estado feito com o novo rótulo — não fica
    // preso na grade parecendo que o registro falhou.
    expect(await screen.findByText(/Check-in de hoje feito: Bem/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Registrar' })).not.toBeInTheDocument();
  });

  it('oferece o aprofundamento com link para /avaliacoes/WHO5', async () => {
    vi.mocked(api.getMoodToday).mockResolvedValue({
      dia: '2026-07-20',
      can_checkin: false,
      checkin: {
        valencia: 60,
        energia: 60,
        quadrante: 'agradavel_ativado',
        respondido_em: '2026-07-20T09:00:00-03:00',
      },
      offer: 'WHO5',
    });

    renderCard();

    const link = await screen.findByRole('link', { name: /responder agora/i });
    expect(link).toHaveAttribute('href', '/avaliacoes/WHO5');
  });
});
