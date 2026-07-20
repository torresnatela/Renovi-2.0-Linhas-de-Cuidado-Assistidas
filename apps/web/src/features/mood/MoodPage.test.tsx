import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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
    getAssessmentAvailability: vi.fn(),
    submitAssessment: vi.fn(),
    moodHelpNow: vi.fn(),
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

  it('permite escolher o humor pelo teclado (acessibilidade)', async () => {
    const today: MoodToday = { dia: '2026-07-18', can_checkin: true, checkin: null };
    vi.mocked(api.getMoodToday).mockResolvedValue(today);
    const salvo: MoodCheckin = {
      valencia: 55,
      energia: 55,
      quadrante: 'agradavel_ativado',
      respondido_em: '2026-07-18T10:00:00-03:00',
    };
    vi.mocked(api.recordMoodCheckin).mockResolvedValue(salvo);

    renderPage();

    const grade = await screen.findByRole('button', {
      name: 'Grade de humor: valência por energia',
    });
    grade.focus();
    // Do centro (50,50): uma seta para a direita e uma para cima => (55, 55).
    fireEvent.keyDown(grade, { key: 'ArrowRight' });
    fireEvent.keyDown(grade, { key: 'ArrowUp' });
    expect(screen.getByTestId('mood-marker')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Registrar meu humor' }));

    expect(await screen.findByText(/Humor de hoje registrado/)).toBeInTheDocument();
    expect(vi.mocked(api.recordMoodCheckin).mock.calls[0][0]).toMatchObject({
      valencia: 55,
      energia: 55,
    });
  });

  it('oferece o WHO-5 quando o gatilho pede e mostra o resultado', async () => {
    const today: MoodToday = { dia: '2026-07-18', can_checkin: true, checkin: null, offer: 'WHO5' };
    vi.mocked(api.getMoodToday).mockResolvedValue(today);
    vi.mocked(api.getAssessmentAvailability).mockResolvedValue({
      codigo: 'WHO5',
      eligibility: { allowed: true, blocks: [] },
      item_count: 5,
      value_min: 0,
      value_max: 5,
    });
    vi.mocked(api.submitAssessment).mockResolvedValue({
      codigo: 'WHO5',
      index_score: 20,
      raw_score: 5,
      faixa: 'encaminha',
      flag_encaminhar: true,
      respondido_em: '2026-07-18T10:00:00-03:00',
    });

    const user = userEvent.setup();
    renderPage();

    // O gatilho oferece o WHO-5 → abrir o formulário.
    await user.click(await screen.findByRole('button', { name: 'Responder WHO-5' }));

    // Responder os 5 itens: em cada um, escolher o valor 1 (botão "1 · ...").
    await screen.findByText(/Nas últimas duas semanas/);
    const botoes1 = screen.getAllByRole('button').filter((b) => b.textContent?.startsWith('1 ·'));
    expect(botoes1).toHaveLength(5);
    for (const b of botoes1) {
      await user.click(b);
    }
    const enviar = screen.getByRole('button', { name: 'Enviar respostas' });
    expect(enviar).toBeEnabled();
    await user.click(enviar);
    await waitFor(() => expect(api.submitAssessment).toHaveBeenCalled());

    expect(await screen.findByRole('heading', { name: /— resultado/ })).toBeInTheDocument();
    expect(vi.mocked(api.submitAssessment).mock.calls[0]).toEqual(['WHO5', [1, 1, 1, 1, 1]]);
    expect(screen.getByText(/Índice de bem-estar:/)).toBeInTheDocument();
    // Rastreio positivo → mensagem de encaminhamento à trilha clínica.
    expect(screen.getByText(/vale conversar com a equipe de cuidado/)).toBeInTheDocument();
  });
});
