import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { InstrumentConfig, MoodCheckin, MoodToday } from '../../shared/api';
import { MoodPage } from './MoodPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return {
    ...actual,
    getMoodToday: vi.fn(),
    grantConsent: vi.fn(),
    getMoodInstrument: vi.fn(),
    recordMoodCheckin: vi.fn(),
  };
});
const api = await import('../../shared/api');

const instrumento: InstrumentConfig = {
  codigo: 'GRID',
  versao: '1',
  anel: 'diario',
  dimensions: [],
  emotion_labels: [],
  context_tags: [{ chave: 'sono', rotulo: 'Sono' }],
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/humor']}>
        <MoodPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('MoodPage', () => {
  beforeEach(() => {
    vi.mocked(api.getMoodInstrument).mockResolvedValue(instrumento);
    vi.mocked(api.grantConsent).mockReset();
    vi.mocked(api.recordMoodCheckin).mockReset();
  });

  it('pede consentimento antes de permitir o check-in', async () => {
    const today: MoodToday = { dia: '2026-07-18', can_checkin: false, reason: 'consent_required' };
    vi.mocked(api.getMoodToday).mockResolvedValue(today);
    vi.mocked(api.grantConsent).mockResolvedValue({ finalidade: 'checkin_humor', active: true });

    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Aceitar e continuar' }));
    expect(api.grantConsent).toHaveBeenCalledWith('v1');
  });

  it('avisa quando não há matrícula elegível', async () => {
    const today: MoodToday = { dia: '2026-07-18', can_checkin: false, reason: 'not_enrolled' };
    vi.mocked(api.getMoodToday).mockResolvedValue(today);

    renderPage();
    expect(await screen.findByText('Check-in indisponível')).toBeInTheDocument();
  });

  it('registra o humor ao tocar na grade e confirmar', async () => {
    const today: MoodToday = { dia: '2026-07-18', can_checkin: true, checkin: null };
    vi.mocked(api.getMoodToday).mockResolvedValue(today);
    const salvo: MoodCheckin = {
      valencia: 75,
      energia: 75,
      quadrante: 'agradavel_ativado',
      respondido_em: '2026-07-18T10:00:00-03:00',
    };
    vi.mocked(api.recordMoodCheckin).mockResolvedValue(salvo);

    renderPage();

    const grade = await screen.findByRole('button', {
      name: 'Grade de humor: valência por energia',
    });
    // jsdom devolve rect zerado: stub para o cálculo de coordenadas fazer sentido.
    grade.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 200, height: 200, right: 200, bottom: 200, x: 0, y: 0 }) as DOMRect;

    // (150, 50) numa grade 200×200 => valência 75, energia 75 (topo = mais energia).
    fireEvent.click(grade, { clientX: 150, clientY: 50 });
    expect(screen.getByTestId('mood-marker')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Registrar meu humor' }));

    expect(await screen.findByText(/Humor de hoje registrado/)).toBeInTheDocument();
    expect(vi.mocked(api.recordMoodCheckin).mock.calls[0][0]).toMatchObject({
      valencia: 75,
      energia: 75,
    });
    expect(screen.getByText('Agradável e com energia')).toBeInTheDocument();
  });
});
