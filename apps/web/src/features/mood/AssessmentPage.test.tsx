import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AssessmentPage } from './AssessmentPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getAssessmentAvailability: vi.fn(), submitAssessment: vi.fn() };
});
const api = await import('../../shared/api');

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/avaliacoes/:codigo" element={<AssessmentPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AssessmentPage', () => {
  beforeEach(() => {
    vi.mocked(api.getAssessmentAvailability).mockResolvedValue({
      codigo: 'WHO5',
      eligibility: { allowed: true, blocks: [] },
      item_count: 5,
      value_min: 0,
      value_max: 5,
    });
  });

  it('avisa e oferece volta quando o código é inválido', () => {
    renderAt('/avaliacoes/XPTO');
    expect(screen.getByText('Avaliação não encontrada')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /voltar/i })).toHaveAttribute('href', '/jornada');
    // Não renderiza o formulário de instrumento.
    expect(screen.queryByText(/Nas últimas duas semanas/)).not.toBeInTheDocument();
  });

  it('renderiza o formulário do WHO-5 quando o código é válido', async () => {
    renderAt('/avaliacoes/WHO5');
    expect(await screen.findByText(/Nas últimas duas semanas/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /voltar/i })).toHaveAttribute('href', '/jornada');
  });

  // Intenção migrada de MoodPage.test (aposentada na Etapa 8): responder o
  // instrumento até o resultado e ver o encaminhamento — em tom INFORMATIVO,
  // nunca vermelho de erro (o encaminhamento não é uma falha do paciente).
  it('envia o WHO-5, mostra o resultado e o encaminhamento sem tom de erro', async () => {
    vi.mocked(api.submitAssessment).mockResolvedValue({
      codigo: 'WHO5',
      index_score: 20,
      raw_score: 5,
      faixa: 'encaminha',
      flag_encaminhar: true,
      respondido_em: '2026-07-20T10:00:00-03:00',
    });

    const user = userEvent.setup();
    renderAt('/avaliacoes/WHO5');

    await screen.findByText(/Nas últimas duas semanas/);
    // Responde os 5 itens (o primeiro rótulo de cada: "Em nenhum momento").
    for (const botao of screen.getAllByRole('button', { name: /Em nenhum momento/i })) {
      await user.click(botao);
    }
    await user.click(screen.getByRole('button', { name: /Enviar respostas/i }));

    expect(await screen.findByText(/Índice de bem-estar:/)).toBeInTheDocument();
    expect(screen.getByText(/vale conversar com a equipe de cuidado/i)).toBeInTheDocument();
    // Encaminhamento é informativo, não erro: nenhum role=alert.
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
